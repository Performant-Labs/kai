package translate

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// The request registry (issue #109).
//
// Every fan-out of a translation (TranslateMulti, and the two screenshot flows) is a request: it
// has an id, belongs to a session, and owns one cancel-only context per engine. The registry is
// what lets a cancel reach a running engine (CancelTranslate) and what lets a newer request of the
// same session replace the running one (superseding). Nothing in it ever times out: a request ends
// when each of its engines has reported once, and only then is it removed.
//
// Contexts form a tree rooted at context.Background(): a request context, and under it one context
// per engine. Cancelling the request cancels every engine; cancelling one engine touches only that
// engine's context, so the others keep running.

// The reason a request or engine context ended, read with context.Cause. What an engine's ending
// means is decided from these causes and never from error text: the LLM engines wrap what they get
// with "%s" and lose the error chain, so errors.Is(err, context.Canceled) is not reliable.
var (
	// errUserCancelled is the user's Cancel. The engine reports a payload with Cancelled set.
	errUserCancelled = errors.New("translation cancelled by the user")
	// errSuperseded means a newer request of the same session replaced this one. Its engines emit
	// nothing and write no history.
	errSuperseded = errors.New("translation superseded by a newer request")
	// errRequestDone releases a context once everything that used it has reported. Nothing is
	// waiting on it any more, so it is never the reason an outcome is decided.
	errRequestDone = errors.New("translation request finished")
)

// sessionTranslate is the session of the translate window's requests (TranslateMulti). The
// screenshot flows are keyed by events.ScreenshotSessionScreenshot / ScreenshotSessionInput, which
// this value must not collide with.
const sessionTranslate = "translate"

// requestRegistry holds the running requests. The zero value is ready to use.
type requestRegistry struct {
	mu        sync.Mutex
	byID      map[string]*activeRequest
	bySession map[string]string // session -> id of that session's newest request
}

// activeRequest is one running request.
type activeRequest struct {
	id      string
	session string
	ctx     context.Context
	cancel  context.CancelCauseFunc

	// The next three are guarded by requestRegistry.mu.
	engines map[string]*engineRun
	open    int  // engines started that have not finished
	sealed  bool // the engines are known: the request is removed once open drops to 0

	// emitMu makes "still live? then emit" atomic against superseding: an emit holds the read
	// side, supersede takes the write side, so once supersede has returned nothing more of this
	// request is emitted, and whatever it emitted before has already been handed to the event
	// system, ahead of anything the request that replaced it sends.
	emitMu     sync.RWMutex
	superseded bool
}

// engineRun is one engine of a request.
type engineRun struct {
	name   string
	ctx    context.Context
	cancel context.CancelCauseFunc
	// startedAtMs is the backend clock (epoch ms) of the started event; 0 until it was emitted.
	startedAtMs atomic.Int64
	// claimed is set by the first terminal report of this engine (guarded by requestRegistry.mu).
	claimed bool
}

// open registers a new request of session and supersedes the session's running one, if any. The
// caller then calls start with the engines, or release when it turns out there are none. open
// comes before the first thing the request sends, so nothing of the request it replaces can arrive
// after it.
func (rr *requestRegistry) open(session, id string) *activeRequest {
	ctx, cancel := context.WithCancelCause(context.Background())
	ar := &activeRequest{
		id:      id,
		session: session,
		ctx:     ctx,
		cancel:  cancel,
		engines: make(map[string]*engineRun),
	}
	rr.mu.Lock()
	if rr.byID == nil {
		rr.byID = make(map[string]*activeRequest)
		rr.bySession = make(map[string]string)
	}
	var replaced []*activeRequest
	if prev := rr.byID[rr.bySession[session]]; prev != nil {
		replaced = append(replaced, prev)
	}
	// A caller that reuses an id replaces the request that still holds it too, so one id never
	// names two live requests.
	if dup := rr.byID[id]; dup != nil && (len(replaced) == 0 || replaced[0] != dup) {
		replaced = append(replaced, dup)
	}
	rr.byID[id] = ar
	rr.bySession[session] = id
	rr.mu.Unlock()
	for _, old := range replaced {
		old.supersede()
	}
	return ar
}

// start gives each named engine of ar its own context and seals the request: it is removed once
// every one of them has finished. With no engines it is removed at once. The runs come back in the
// order of names.
func (rr *requestRegistry) start(ar *activeRequest, names []string) []*engineRun {
	runs := make([]*engineRun, 0, len(names))
	rr.mu.Lock()
	for _, n := range names {
		ctx, cancel := context.WithCancelCause(ar.ctx)
		run := &engineRun{name: n, ctx: ctx, cancel: cancel}
		ar.engines[n] = run
		runs = append(runs, run)
	}
	ar.open = len(runs)
	ar.sealed = true
	empty := ar.open == 0
	if empty {
		rr.removeLocked(ar)
	}
	rr.mu.Unlock()
	if empty {
		ar.cancel(errRequestDone)
	}
	return runs
}

// finish ends one engine's part in ar, after its terminal report (or its silent supersede). The
// last engine to finish removes the request and releases every context; the registry is empty
// only after every emit of the request has been made.
func (rr *requestRegistry) finish(ar *activeRequest, run *engineRun) {
	rr.mu.Lock()
	ar.open--
	last := ar.sealed && ar.open == 0
	if last {
		rr.removeLocked(ar)
	}
	rr.mu.Unlock()
	run.cancel(errRequestDone)
	if last {
		ar.cancel(errRequestDone)
	}
}

// release drops a request that never got engines (the screenshot flow failed before it reached the
// translation stage). It is a no-op once start has been called: the engines own the request then.
func (rr *requestRegistry) release(ar *activeRequest) {
	rr.mu.Lock()
	if ar.sealed {
		rr.mu.Unlock()
		return
	}
	ar.sealed = true
	rr.removeLocked(ar)
	rr.mu.Unlock()
	ar.cancel(errRequestDone)
}

// claim reports whether the caller is the first to report run's terminal outcome. It is the
// once-only guard behind "exactly one terminal event per started engine": the terminal emit runs
// only for the caller that gets true.
func (rr *requestRegistry) claim(run *engineRun) bool {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	if run.claimed {
		return false
	}
	run.claimed = true
	return true
}

// cancel ends a running request, or only its engine when engine is not empty, as the user's
// Cancel. It reports whether it found something still running: false for an unknown or finished
// request, an unknown engine, and an engine that has already reported. Cancelling twice is
// harmless (the first cause stands).
func (rr *requestRegistry) cancel(id, engine string) bool {
	rr.mu.Lock()
	ar := rr.byID[id]
	if ar == nil {
		rr.mu.Unlock()
		return false
	}
	if engine == "" {
		rr.mu.Unlock()
		ar.cancel(errUserCancelled)
		return true
	}
	run := ar.engines[engine]
	if run == nil || run.claimed {
		rr.mu.Unlock()
		return false
	}
	rr.mu.Unlock()
	run.cancel(errUserCancelled)
	return true
}

// running returns the request and engine when the engine is still working: known, not yet
// reported and not cancelled. It is nil otherwise.
func (rr *requestRegistry) running(id, engine string) (*activeRequest, *engineRun) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	ar := rr.byID[id]
	if ar == nil {
		return nil, nil
	}
	run := ar.engines[engine]
	if run == nil || run.claimed || run.ctx.Err() != nil {
		return nil, nil
	}
	return ar, run
}

// count is the number of requests the registry still holds.
func (rr *requestRegistry) count() int {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	return len(rr.byID)
}

// removeLocked drops ar from both maps, unless a newer request already took its place in either.
// The caller holds rr.mu.
func (rr *requestRegistry) removeLocked(ar *activeRequest) {
	// Only the request that still owns its id may drop the id and the session's pointer to it. A
	// superseded request that reused the id of its replacement (same session, same id) must not
	// delete the session mapping the replacement now holds.
	if rr.byID[ar.id] != ar {
		return
	}
	delete(rr.byID, ar.id)
	if rr.bySession[ar.session] == ar.id {
		delete(rr.bySession, ar.session)
	}
}

// supersede marks ar replaced and cancels it. It waits for the emits in flight, so when it returns
// nothing more of ar is delivered.
func (ar *activeRequest) supersede() {
	ar.emitMu.Lock()
	ar.superseded = true
	ar.emitMu.Unlock()
	ar.cancel(errSuperseded)
}

// isSuperseded reports whether a newer request replaced ar.
func (ar *activeRequest) isSuperseded() bool {
	ar.emitMu.RLock()
	defer ar.emitMu.RUnlock()
	return ar.superseded
}

// emitIfLive runs emit unless ar was superseded, and reports whether it ran. Every emit of a
// request goes through it, which is how "a superseded request emits nothing" holds even for an
// engine that finished a moment before the newer request began.
func (ar *activeRequest) emitIfLive(emit func()) bool {
	ar.emitMu.RLock()
	defer ar.emitMu.RUnlock()
	if ar.superseded {
		return false
	}
	emit()
	return true
}

// requestSeq numbers the ids newRequestID hands out.
var requestSeq atomic.Uint64

// newRequestID returns an id for a request nobody named: the screenshot flows, and callers of
// TranslateMulti that send no id. The frontend names its own requests, so ids from here carry a
// prefix no frontend id has.
func newRequestID() string {
	return fmt.Sprintf("srv-%x-%d", time.Now().UnixNano(), requestSeq.Add(1))
}
