package translate

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/historystore"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #109 Part A (T, RED). The tests here drive the service through its public entry points
// (Translate, TranslateMulti, CancelTranslate, ScreenshotRetranslate) and observe it through the
// emitter seam (D8). The names the F implementation must provide, all in package translate:
//
//	type emitter interface { Emit(name string, data ...any) bool }   // (*application.EventManager satisfies it)
//	func (s *Service) SetEmitter(e emitter)
//	func (s *Service) CancelTranslate(requestID, engine string) bool
//	func (s *Service) activeRequestCount() int
//	func (s *Service) reportProgress(requestID, engine string, done, total int)
//
// Emitted payloads are VALUES: model.TranslateResult, model.ScreenshotResult and
// events.TranslateProgressPayload (never pointers). No test sleeps for more than a few
// milliseconds: engines block on channels and the waits below poll with a 1 ms tick.

// ---- recording emitter -------------------------------------------------------------------

type recorded struct {
	name string
	data any
}

type recEmitter struct {
	mu  sync.Mutex
	evs []recorded
}

func (r *recEmitter) Emit(name string, data ...any) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	var d any
	if len(data) > 0 {
		d = data[0]
	}
	r.evs = append(r.evs, recorded{name: name, data: d})
	return true
}

func (r *recEmitter) all() []recorded {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]recorded(nil), r.evs...)
}

// results returns every EventTranslateResult payload, in emit order.
func (r *recEmitter) results() []model.TranslateResult {
	var out []model.TranslateResult
	for _, e := range r.all() {
		if e.name != events.EventTranslateResult {
			continue
		}
		switch v := e.data.(type) {
		case model.TranslateResult:
			out = append(out, v)
		case *model.TranslateResult:
			out = append(out, *v)
		}
	}
	return out
}

func (r *recEmitter) resultsFor(requestID, eng string) []model.TranslateResult {
	var out []model.TranslateResult
	for _, x := range r.results() {
		if x.RequestID == requestID && x.Engine == eng {
			out = append(out, x)
		}
	}
	return out
}

func (r *recEmitter) progress() []events.TranslateProgressPayload {
	var out []events.TranslateProgressPayload
	for _, e := range r.all() {
		if e.name != events.EventTranslateProgress {
			continue
		}
		switch v := e.data.(type) {
		case events.TranslateProgressPayload:
			out = append(out, v)
		case *events.TranslateProgressPayload:
			out = append(out, *v)
		}
	}
	return out
}

func (r *recEmitter) screenshots() []model.ScreenshotResult {
	var out []model.ScreenshotResult
	for _, e := range r.all() {
		if e.name != events.EventScreenshotOCR {
			continue
		}
		switch v := e.data.(type) {
		case model.ScreenshotResult:
			out = append(out, v)
		case *model.ScreenshotResult:
			out = append(out, *v)
		}
	}
	return out
}

// waitUntil polls cond on a 1 ms tick until it holds or 5 s pass (a failure, not a sleep).
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// ---- fake engines ------------------------------------------------------------------------

type fakeEngine struct {
	name string
	run  func(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error)
}

func (f *fakeEngine) Name() string { return f.name }
func (f *fakeEngine) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	return f.run(ctx, req)
}

func okResult(text string) *model.TranslateResult {
	return &model.TranslateResult{Result: text}
}

// honoring blocks until released (success) or ctx ends. Like the LLM engines it loses the error
// chain on the way out ("%s"), so the service cannot rely on errors.Is(err, context.Canceled).
type honoring struct {
	*fakeEngine
	started  chan struct{}
	release  chan struct{}
	sawCtxEr atomic.Value // error
	once     sync.Once
}

func newHonoring(name string) *honoring {
	h := &honoring{started: make(chan struct{}), release: make(chan struct{})}
	h.fakeEngine = &fakeEngine{name: name, run: func(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		h.once.Do(func() { close(h.started) })
		select {
		case <-h.release:
			return okResult("done-" + name), nil
		case <-ctx.Done():
			h.sawCtxEr.Store(ctx.Err())
			return nil, fmt.Errorf("%s", ctx.Err().Error())
		}
	}}
	return h
}

// ignoring blocks on release and never looks at its ctx: it stands for any engine that ignores
// its ctx. The Apple engine is not one (it honours its ctx); for its shape see
// TestCancelledAppleShapedEngineIsNotAFailure.
type ignoring struct {
	*fakeEngine
	started  chan struct{}
	release  chan struct{}
	returned chan struct{}
}

func newIgnoring(name string) *ignoring {
	g := &ignoring{started: make(chan struct{}), release: make(chan struct{}), returned: make(chan struct{})}
	g.fakeEngine = &fakeEngine{name: name, run: func(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		close(g.started)
		<-g.release
		defer close(g.returned)
		return okResult("late-" + name), nil
	}}
	return g
}

// ---- service builder ---------------------------------------------------------------------

func newCancelService(t *testing.T, engs ...engine.Translator) (*Service, *recEmitter, *historystore.Store) {
	t.Helper()
	hist, err := historystore.Open("")
	if err != nil {
		t.Fatalf("historystore.Open: %v", err)
	}
	t.Cleanup(func() { _ = hist.Close() })
	cfg, err := configstore.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("configstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = cfg.Close() })
	reg := engine.NewRegistry()
	for _, e := range engs {
		reg.RegisterTranslator(e)
	}
	svc := NewService(reg, hist, nil, nil)
	svc.SetConfigStore(cfg)
	em := &recEmitter{}
	svc.SetEmitter(em)
	return svc, em, hist
}

func historyRows(t *testing.T, hist *historystore.Store, keyword string) int {
	t.Helper()
	rows, err := hist.QueryByKeyword(context.Background(), keyword, 50, 0)
	if err != nil {
		t.Fatalf("QueryByKeyword: %v", err)
	}
	return len(rows)
}

func multiReq(id string) model.TranslateRequest {
	return model.TranslateRequest{Text: "Hola", From: model.ES, To: model.EN, RequestID: id}
}

// ---- criterion 1: no deadline ------------------------------------------------------------

// The ctx an engine receives has no deadline. Today translateWithEngine wraps a 30 s WithTimeout,
// so ok is true. (Issue #80 removed the identity re-run of #44: the same-language case is now an
// identity result after one call, so exactly one engine call is checked.)
func TestTranslateEngineContextHasNoDeadline(t *testing.T) {
	var mu sync.Mutex
	var hasDeadline []bool
	eng := &fakeEngine{name: "probe", run: func(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		_, ok := ctx.Deadline()
		mu.Lock()
		hasDeadline = append(hasDeadline, ok)
		mu.Unlock()
		// Detected source == requested target: an identity result after this one call (issue #80).
		return &model.TranslateResult{Result: "x", From: model.ZH}, nil
	}}
	reg := engine.NewRegistry()
	reg.RegisterTranslator(eng)
	svc := NewService(reg, nil, nil, nil)
	if _, err := svc.Translate(model.TranslateRequest{Text: "ni hao", From: model.Auto, To: model.ZH, EngineName: "probe"}); err != nil {
		t.Fatalf("Translate: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(hasDeadline) != 1 {
		t.Fatalf("engine called %d times, want 1 (the identity rule does not re-run the engine)", len(hasDeadline))
	}
	for i, ok := range hasDeadline {
		if ok {
			t.Errorf("call %d: ctx has a deadline; the engine ctx must be cancel-only", i+1)
		}
	}
}

// ---- criterion 2: slow engine completes --------------------------------------------------

func TestSlowEngineCompletesWithoutDeadline(t *testing.T) {
	h := newHonoring("slow")
	svc, em, _ := newCancelService(t, h)
	if _, err := svc.TranslateMulti(multiReq("r-slow")); err != nil {
		t.Fatalf("TranslateMulti: %v", err)
	}
	<-h.started
	close(h.release)
	waitUntil(t, "slow engine's success payload", func() bool { return len(em.resultsFor("r-slow", "slow")) == 1 })
	got := em.resultsFor("r-slow", "slow")[0]
	if got.Error != "" || got.Cancelled || got.Result != "done-slow" {
		t.Errorf("payload = %+v, want a plain success carrying the released result", got)
	}
}

// ---- criterion 4: identity and cancel API ------------------------------------------------

func TestTranslateMultiReturnsRequestIDAndStartedEngines(t *testing.T) {
	a, b := newHonoring("a"), newHonoring("b")
	svc, _, _ := newCancelService(t, a, b)
	res, err := svc.TranslateMulti(multiReq("r-id"))
	if err != nil {
		t.Fatalf("TranslateMulti: %v", err)
	}
	if res.RequestID != "r-id" {
		t.Errorf("RequestID = %q, want the caller's id echoed", res.RequestID)
	}
	if len(res.Engines) != 2 || res.Count != 2 {
		t.Errorf("Engines = %v, Count = %d, want the two engines actually started", res.Engines, res.Count)
	}
	svc.CancelTranslate("r-id", "")
}

func TestTranslateMultiGeneratesAnIDWhenNoneIsSent(t *testing.T) {
	a := newHonoring("a")
	svc, _, _ := newCancelService(t, a)
	res, err := svc.TranslateMulti(multiReq(""))
	if err != nil {
		t.Fatalf("TranslateMulti: %v", err)
	}
	if res.RequestID == "" {
		t.Error("RequestID is empty; the backend must generate one when the caller sends none")
	}
	svc.CancelTranslate(res.RequestID, "")
}

func TestCancelTranslateUnknownIsNoOp(t *testing.T) {
	a := newHonoring("a")
	svc, em, _ := newCancelService(t, a)
	if svc.CancelTranslate("nope", "") {
		t.Error("cancelling an unknown request returned true")
	}
	if svc.CancelTranslate("nope", "a") {
		t.Error("cancelling an unknown request's engine returned true")
	}
	if _, err := svc.TranslateMulti(multiReq("r-known")); err != nil {
		t.Fatal(err)
	}
	<-a.started
	if svc.CancelTranslate("r-known", "no-such-engine") {
		t.Error("cancelling an unknown engine of a live request returned true")
	}
	if a.sawCtxEr.Load() != nil {
		t.Error("cancelling an unknown engine touched the live engine")
	}
	svc.CancelTranslate("r-known", "")
	waitUntil(t, "cleanup", func() bool { return len(em.resultsFor("r-known", "a")) == 1 })
}

func TestCancelTranslateIsIdempotent(t *testing.T) {
	a := newHonoring("a")
	svc, em, _ := newCancelService(t, a)
	if _, err := svc.TranslateMulti(multiReq("r-idem")); err != nil {
		t.Fatal(err)
	}
	<-a.started
	if !svc.CancelTranslate("r-idem", "") {
		t.Error("first cancel of a live request returned false")
	}
	svc.CancelTranslate("r-idem", "")
	svc.CancelTranslate("r-idem", "a")
	waitUntil(t, "terminal event", func() bool { return len(em.resultsFor("r-idem", "a")) >= 1 })
	waitUntil(t, "registry empty", func() bool { return svc.activeRequestCount() == 0 })
	if svc.CancelTranslate("r-idem", "") {
		t.Error("cancelling a finished request returned true")
	}
	if n := len(em.resultsFor("r-idem", "a")); n != 1 {
		t.Errorf("got %d terminal payloads after repeated cancels, want exactly 1", n)
	}
}

// ---- criterion 5: cancel is real and per engine ------------------------------------------

func TestCancelOneEngineOthersKeepRunning(t *testing.T) {
	a, b := newHonoring("a"), newHonoring("b")
	svc, em, _ := newCancelService(t, a, b)
	if _, err := svc.TranslateMulti(multiReq("r-one")); err != nil {
		t.Fatal(err)
	}
	<-a.started
	<-b.started
	if !svc.CancelTranslate("r-one", "a") {
		t.Fatal("CancelTranslate(r-one, a) = false")
	}
	waitUntil(t, "a's cancelled payload", func() bool { return len(em.resultsFor("r-one", "a")) == 1 })
	if err, _ := a.sawCtxEr.Load().(error); err == nil {
		t.Error("engine a never saw its ctx cancelled")
	}
	if got := em.resultsFor("r-one", "b"); len(got) != 0 {
		t.Fatalf("b reported %v after a's cancel; it must keep running", got)
	}
	if b.sawCtxEr.Load() != nil {
		t.Error("cancelling a cancelled b's ctx too")
	}
	close(b.release)
	waitUntil(t, "b's success", func() bool { return len(em.resultsFor("r-one", "b")) == 1 })
	if got := em.resultsFor("r-one", "b")[0]; got.Cancelled || got.Error != "" || got.Result != "done-b" {
		t.Errorf("b's payload = %+v, want a plain success", got)
	}
	if got := em.resultsFor("r-one", "a")[0]; !got.Cancelled {
		t.Errorf("a's payload = %+v, want Cancelled", got)
	}
}

func TestCancelWholeRequest(t *testing.T) {
	a, b := newHonoring("a"), newHonoring("b")
	svc, em, _ := newCancelService(t, a, b)
	if _, err := svc.TranslateMulti(multiReq("r-all")); err != nil {
		t.Fatal(err)
	}
	<-a.started
	<-b.started
	if !svc.CancelTranslate("r-all", "") {
		t.Fatal("CancelTranslate(r-all, \"\") = false")
	}
	waitUntil(t, "both cancelled", func() bool {
		return len(em.resultsFor("r-all", "a")) == 1 && len(em.resultsFor("r-all", "b")) == 1
	})
	for _, n := range []string{"a", "b"} {
		if got := em.resultsFor("r-all", n)[0]; !got.Cancelled {
			t.Errorf("%s payload = %+v, want Cancelled", n, got)
		}
	}
}

// An engine that never reads its ctx must not hold the request open after a cancel, and its late
// result must be dropped (callEngine's contract for any such engine; the Apple engine honours its
// ctx, see TestCancelledAppleShapedEngineIsNotAFailure).
func TestCancelDoesNotWaitForAnEngineThatIgnoresItsContext(t *testing.T) {
	g := newIgnoring("apple")
	svc, em, hist := newCancelService(t, g)
	if _, err := svc.TranslateMulti(multiReq("r-ign")); err != nil {
		t.Fatal(err)
	}
	<-g.started
	svc.CancelTranslate("r-ign", "apple")
	deadline := time.After(2 * time.Second)
	for len(em.resultsFor("r-ign", "apple")) == 0 {
		select {
		case <-deadline:
			t.Fatal("no terminal payload within 2 s while the engine is still blocked")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	got := em.resultsFor("r-ign", "apple")[0]
	if !got.Cancelled || got.Error != "" {
		t.Fatalf("payload = %+v, want Cancelled with no error", got)
	}
	close(g.release)
	<-g.returned
	// A negative check ("nothing more happens") needs a short grace window in which an abandoned call
	// that wrongly acts on its late result would show itself. 5 x 1 ms (the waitUntil tick) caught a
	// late history write in 200 of 200 -race runs; 1 ms caught 171 of 200 and no wait caught 0 of 200.
	for i := 0; i < 5; i++ {
		time.Sleep(time.Millisecond)
	}
	if n := len(em.resultsFor("r-ign", "apple")); n != 1 {
		t.Errorf("%d terminal payloads after the engine returned late, want still 1", n)
	}
	if n := historyRows(t, hist, "Hola"); n != 0 {
		t.Errorf("a cancelled engine's late result wrote %d history rows, want 0", n)
	}
}

// ---- criterion 6: a cancel is never a failure --------------------------------------------

func TestCancelEmitsNoFailurePayload(t *testing.T) {
	a := newHonoring("a")
	svc, em, _ := newCancelService(t, a)
	if _, err := svc.TranslateMulti(multiReq("r-nf")); err != nil {
		t.Fatal(err)
	}
	<-a.started
	svc.CancelTranslate("r-nf", "")
	waitUntil(t, "terminal", func() bool { return len(em.resultsFor("r-nf", "a")) >= 1 })
	for _, r := range em.resultsFor("r-nf", "a") {
		if r.Error != "" || r.ErrorKind != "" {
			t.Errorf("cancel payload carries a failure: Error=%q ErrorKind=%q", r.Error, r.ErrorKind)
		}
		if !r.Cancelled {
			t.Errorf("payload not flagged Cancelled: %+v", r)
		}
		if r.RequestID != "r-nf" {
			t.Errorf("payload RequestID = %q, want r-nf", r.RequestID)
		}
	}
}

func TestCancelledEngineIsNotSavedToHistory(t *testing.T) {
	a := newHonoring("a")
	svc, em, hist := newCancelService(t, a)
	if _, err := svc.TranslateMulti(multiReq("r-nh")); err != nil {
		t.Fatal(err)
	}
	<-a.started
	svc.CancelTranslate("r-nh", "")
	waitUntil(t, "terminal", func() bool { return len(em.resultsFor("r-nh", "a")) >= 1 })
	waitUntil(t, "registry empty", func() bool { return svc.activeRequestCount() == 0 })
	if n := historyRows(t, hist, "Hola"); n != 0 {
		t.Errorf("a cancelled engine wrote %d history rows, want 0", n)
	}
}

// ---- criterion 7: exactly one terminal event per started engine --------------------------

func TestEveryStartedEngineReportsExactlyOnce(t *testing.T) {
	ok := &fakeEngine{name: "ok", run: func(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		return okResult("fine"), nil
	}}
	bad := &fakeEngine{name: "bad", run: func(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		return nil, errors.New("boom")
	}}
	boom := &fakeEngine{name: "boom", run: func(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		panic("engine exploded")
	}}
	slow := newHonoring("slow")
	svc, em, _ := newCancelService(t, ok, bad, boom, slow)
	if _, err := svc.TranslateMulti(multiReq("r-once")); err != nil {
		t.Fatal(err)
	}
	<-slow.started
	svc.CancelTranslate("r-once", "slow")
	waitUntil(t, "registry empty", func() bool { return svc.activeRequestCount() == 0 })
	for _, n := range []string{"ok", "bad", "boom", "slow"} {
		if got := len(em.resultsFor("r-once", n)); got != 1 {
			t.Errorf("engine %s: %d terminal payloads, want exactly 1", n, got)
		}
	}
	if got := em.resultsFor("r-once", "bad"); len(got) == 1 && got[0].Error == "" {
		t.Error("failing engine's payload has no Error")
	}
	if got := em.resultsFor("r-once", "boom"); len(got) == 1 && (got[0].Error == "" || got[0].Cancelled) {
		t.Errorf("panicking engine's payload = %+v, want a failure with Error set", got[0])
	}
	if got := em.resultsFor("r-once", "ok"); len(got) == 1 && (got[0].Error != "" || got[0].Cancelled || got[0].Result != "fine") {
		t.Errorf("ok engine's payload = %+v", got[0])
	}
}

// ---- criterion 8: a superseded request emits nothing -------------------------------------

func TestSupersededRequestEmitsNothing(t *testing.T) {
	var calls atomic.Int32
	firstStarted := make(chan struct{})
	firstSawCancel := make(chan struct{})
	eng := &fakeEngine{name: "e", run: func(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		if calls.Add(1) == 1 {
			close(firstStarted)
			<-ctx.Done()
			close(firstSawCancel)
			return nil, fmt.Errorf("%s", ctx.Err().Error())
		}
		return okResult("second"), nil
	}}
	svc, em, hist := newCancelService(t, eng)
	if _, err := svc.TranslateMulti(multiReq("r-old")); err != nil {
		t.Fatal(err)
	}
	<-firstStarted
	if _, err := svc.TranslateMulti(multiReq("r-new")); err != nil {
		t.Fatal(err)
	}
	<-firstSawCancel
	waitUntil(t, "new request's result", func() bool { return len(em.resultsFor("r-new", "e")) == 1 })
	waitUntil(t, "registry empty", func() bool { return svc.activeRequestCount() == 0 })
	if got := em.resultsFor("r-old", "e"); len(got) != 0 {
		t.Errorf("superseded request emitted %+v; it must emit nothing", got)
	}
	if n := historyRows(t, hist, "Hola"); n != 1 {
		t.Errorf("history rows = %d, want 1 (only the replacing request)", n)
	}
}

// ---- criterion 9: the registry cleans up -------------------------------------------------

func TestRequestRegistryIsEmptyAfterCompletion(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ok := &fakeEngine{name: "ok", run: func(context.Context, model.TranslateRequest) (*model.TranslateResult, error) {
			return okResult("y"), nil
		}}
		svc, em, _ := newCancelService(t, ok)
		if _, err := svc.TranslateMulti(multiReq("r1")); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "result", func() bool { return len(em.resultsFor("r1", "ok")) == 1 })
		waitUntil(t, "registry empty", func() bool { return svc.activeRequestCount() == 0 })
	})
	t.Run("failure", func(t *testing.T) {
		bad := &fakeEngine{name: "bad", run: func(context.Context, model.TranslateRequest) (*model.TranslateResult, error) {
			return nil, errors.New("x")
		}}
		svc, em, _ := newCancelService(t, bad)
		if _, err := svc.TranslateMulti(multiReq("r2")); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "result", func() bool { return len(em.resultsFor("r2", "bad")) == 1 })
		waitUntil(t, "registry empty", func() bool { return svc.activeRequestCount() == 0 })
	})
	t.Run("cancel", func(t *testing.T) {
		a := newHonoring("a")
		svc, em, _ := newCancelService(t, a)
		if _, err := svc.TranslateMulti(multiReq("r3")); err != nil {
			t.Fatal(err)
		}
		<-a.started
		if svc.activeRequestCount() != 1 {
			t.Errorf("activeRequestCount = %d while a request is running, want 1", svc.activeRequestCount())
		}
		svc.CancelTranslate("r3", "")
		waitUntil(t, "result", func() bool { return len(em.resultsFor("r3", "a")) == 1 })
		waitUntil(t, "registry empty", func() bool { return svc.activeRequestCount() == 0 })
	})
	t.Run("supersede", func(t *testing.T) {
		a := newHonoring("a")
		svc, _, _ := newCancelService(t, a)
		if _, err := svc.TranslateMulti(multiReq("r4")); err != nil {
			t.Fatal(err)
		}
		<-a.started
		res, err := svc.TranslateMulti(multiReq("r5"))
		if err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "old request gone", func() bool { return svc.activeRequestCount() == 1 })
		svc.CancelTranslate(res.RequestID, "")
		waitUntil(t, "registry empty", func() bool { return svc.activeRequestCount() == 0 })
	})
}

// ---- criterion 10: events ----------------------------------------------------------------

func TestStartedEventPrecedesResults(t *testing.T) {
	h := newHonoring("a")
	svc, em, _ := newCancelService(t, h)
	if _, err := svc.TranslateMulti(multiReq("r-st")); err != nil {
		t.Fatal(err)
	}
	<-h.started
	close(h.release)
	waitUntil(t, "result", func() bool { return len(em.resultsFor("r-st", "a")) == 1 })
	startedAt, resultAt := -1, -1
	for i, e := range em.all() {
		switch v := e.data.(type) {
		case events.TranslateProgressPayload:
			if e.name == events.EventTranslateProgress && v.Engine == "a" && v.Phase == "started" && v.RequestID == "r-st" {
				if startedAt >= 0 {
					t.Error("more than one started event for the same engine")
				}
				startedAt = i
				if v.StartedAtMs <= 0 {
					t.Errorf("StartedAtMs = %d, want a positive epoch-ms", v.StartedAtMs)
				}
			}
		case model.TranslateResult:
			if e.name == events.EventTranslateResult && v.Engine == "a" {
				resultAt = i
			}
		}
	}
	if startedAt < 0 {
		t.Fatal("no started progress event for engine a")
	}
	if resultAt < startedAt {
		t.Errorf("result (index %d) precedes started (index %d)", resultAt, startedAt)
	}
}

func TestProgressChunkHelperEmitsDoneTotal(t *testing.T) {
	h := newHonoring("a")
	svc, em, _ := newCancelService(t, h)
	if _, err := svc.TranslateMulti(multiReq("r-ch")); err != nil {
		t.Fatal(err)
	}
	<-h.started
	svc.reportProgress("r-ch", "a", 2, 5)
	var found bool
	for _, p := range em.progress() {
		if p.Phase == "chunk" && p.RequestID == "r-ch" && p.Engine == "a" && p.Done == 2 && p.Total == 5 {
			found = true
		}
	}
	if !found {
		t.Errorf("no chunk event {done:2,total:5} in %+v", em.progress())
	}
	svc.CancelTranslate("r-ch", "")
}

// ---- criterion 11 (+ A warn 1): screenshot flows -----------------------------------------

func seedScreenshotCache(svc *Service, session string) {
	svc.screenshotCacheMu.Lock()
	svc.screenshotCache[session] = ocrCache{text: "Hola", imageURL: "data:image/png;base64,AAAA"}
	svc.screenshotCacheMu.Unlock()
}

// A cancelled engine of the screenshot flow becomes a placeholder with Cancelled set and no
// error; every push of the run carries the run's request id, announced by the started events, and
// that id is not the id of a concurrent TranslateMulti request.
func TestTranslateAllStreamCancelledPlaceholder(t *testing.T) {
	a := newHonoring("a")
	svc, em, _ := newCancelService(t, a)
	seedScreenshotCache(svc, events.ScreenshotSessionScreenshot)
	done := make(chan error, 1)
	go func() { done <- svc.ScreenshotRetranslate(events.ScreenshotSessionScreenshot, model.ES, model.EN) }()
	<-a.started
	waitUntil(t, "started event", func() bool { return len(em.progress()) > 0 })
	id := em.progress()[0].RequestID
	if id == "" {
		t.Fatal("started event carries no request id")
	}
	if !svc.CancelTranslate(id, "a") {
		t.Fatal("CancelTranslate on the screenshot run's id returned false")
	}
	if err := <-done; err != nil {
		t.Fatalf("ScreenshotRetranslate: %v", err)
	}
	shots := em.screenshots()
	if len(shots) == 0 {
		t.Fatal("no EventScreenshotOCR push")
	}
	last := shots[len(shots)-1]
	if len(last.Translations) != 1 {
		t.Fatalf("translations = %+v, want one placeholder", last.Translations)
	}
	ph := last.Translations[0]
	if ph.Engine != "a" || !ph.Cancelled || ph.Error != "" || ph.ErrorKind != "" || ph.Result != "" {
		t.Errorf("placeholder = %+v, want engine a, Cancelled, empty Error/Result", ph)
	}
	for i, s := range shots {
		if s.RequestID != id {
			t.Errorf("push %d RequestID = %q, want %q on every push of the run", i, s.RequestID, id)
		}
	}
	waitUntil(t, "registry empty", func() bool { return svc.activeRequestCount() == 0 })
}

func TestScreenshotRunIDDiffersFromConcurrentTranslateMultiID(t *testing.T) {
	a := newHonoring("a")
	svc, em, _ := newCancelService(t, a)
	seedScreenshotCache(svc, events.ScreenshotSessionScreenshot)
	done := make(chan error, 1)
	go func() { done <- svc.ScreenshotRetranslate(events.ScreenshotSessionScreenshot, model.ES, model.EN) }()
	<-a.started
	waitUntil(t, "screenshot started event", func() bool { return len(em.progress()) >= 1 })
	shotID := em.progress()[0].RequestID
	res, err := svc.TranslateMulti(multiReq("r-input"))
	if err != nil {
		t.Fatal(err)
	}
	if shotID == "" || shotID == res.RequestID {
		t.Fatalf("screenshot id %q and translate id %q must be distinct and non-empty", shotID, res.RequestID)
	}
	svc.CancelTranslate(shotID, "")
	svc.CancelTranslate(res.RequestID, "")
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
