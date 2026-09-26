package engine

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Issue #111 (Tester, RED). Contract T assumes, all in package engine, untagged (apple_cancel.go):
//
//	func runCancellable(ctx context.Context, token int64, call func() []byte, cancel func(token int64)) ([]byte, error)
//	func nextAppleToken() int64
//	func appleCancelOutcome(ctx context.Context, code string) (err error, handled bool)
//
// Per handoff-A finding 1, runCancellable is payload-agnostic: its only own error is "ctx already
// done" (call never runs, err is context.Cause(ctx)); otherwise it returns what call returned. The
// rule "a cancelled payload plus a done ctx means context.Cause(ctx)" lives once, in
// appleCancelOutcome. No test here sleeps for a real interval: calls block on channels.

var errCauseX = errors.New("cause-x")

func causedCtx(cause error) context.Context {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	return ctx
}

// counter records cancel(token) invocations.
type counter struct {
	mu     sync.Mutex
	tokens []int64
	fired  chan struct{}
	once   sync.Once
}

func newCounter() *counter { return &counter{fired: make(chan struct{})} }
func (c *counter) cancel(token int64) {
	c.mu.Lock()
	c.tokens = append(c.tokens, token)
	c.mu.Unlock()
	c.once.Do(func() { close(c.fired) })
}
func (c *counter) calls() []int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int64(nil), c.tokens...)
}

// (a) ctx already done: the call is never made, the result is context.Cause(ctx).
func TestRunCancellableCtxAlreadyDone(t *testing.T) {
	var called atomic.Bool
	c := newCounter()
	out, err := runCancellable(causedCtx(errCauseX), 7, func() []byte {
		called.Store(true)
		return []byte("x")
	}, c.cancel)
	if called.Load() {
		t.Error("call was invoked although ctx was already done")
	}
	if !errors.Is(err, errCauseX) {
		t.Errorf("err = %v, want context.Cause(ctx) (%v)", err, errCauseX)
	}
	if out != nil {
		t.Errorf("payload = %q, want nil", out)
	}
	if n := len(c.calls()); n != 0 {
		t.Errorf("cancel called %d times for a call that never started, want 0", n)
	}
}

// (b) ctx ends while the call runs: cancel(token) fires exactly once, with the call's token, and the
// payload the call then returns is passed through.
func TestRunCancellableCancelsOnceWhileCallRuns(t *testing.T) {
	ctx, stop := context.WithCancelCause(context.Background())
	c := newCounter()
	started := make(chan struct{})
	release := make(chan struct{})
	call := func() []byte {
		close(started)
		<-release
		return []byte("opaque-payload")
	}
	cancelFn := func(token int64) {
		c.cancel(token)
		close(release) // a real Swift cancel makes the blocked call return
	}
	type res struct {
		out []byte
		err error
	}
	done := make(chan res, 1)
	go func() {
		out, err := runCancellable(ctx, 42, call, cancelFn)
		done <- res{out, err}
	}()
	<-started
	stop(errCauseX)
	r := <-done
	if got := c.calls(); len(got) != 1 || got[0] != 42 {
		t.Fatalf("cancel calls = %v, want exactly [42]", got)
	}
	if string(r.out) != "opaque-payload" {
		t.Errorf("payload = %q, want the call's own payload passed through", r.out)
	}
	if r.err != nil {
		t.Errorf("err = %v, want nil for a payload that is not a cancel (runCancellable does not judge payloads)", r.err)
	}
}

// (c) call returns normally: cancel is never called, including when ctx ends right after the return.
func TestRunCancellableNormalReturnNeverCancels(t *testing.T) {
	for i := 0; i < 200; i++ {
		ctx, stop := context.WithCancelCause(context.Background())
		c := newCounter()
		out, err := runCancellable(ctx, int64(i+1), func() []byte { return []byte("ok") }, c.cancel)
		if err != nil || string(out) != "ok" {
			t.Fatalf("iteration %d: out=%q err=%v, want ok/nil", i, out, err)
		}
		stop(errCauseX) // ctx ends a moment after the return
		for j := 0; j < 20; j++ {
			runtime.Gosched()
		}
		if n := len(c.calls()); n != 0 {
			t.Fatalf("iteration %d: cancel called %d times after a normal return, want 0", i, n)
		}
	}
}

// (d) the call returns a full result at the moment ctx ends: the result stands.
func TestRunCancellableFullResultStandsWhenCtxEndsAtReturn(t *testing.T) {
	for i := 0; i < 200; i++ {
		ctx, stop := context.WithCancelCause(context.Background())
		c := newCounter()
		out, err := runCancellable(ctx, 1, func() []byte {
			stop(errCauseX)
			return []byte("full-result")
		}, c.cancel)
		if err != nil {
			t.Fatalf("iteration %d: err = %v, want nil: a translation the engine did return stands", i, err)
		}
		if string(out) != "full-result" {
			t.Fatalf("iteration %d: payload = %q, want full-result", i, out)
		}
		if n := len(c.calls()); n > 1 {
			t.Fatalf("iteration %d: cancel called %d times, want at most once", i, n)
		}
	}
}

// (e) no goroutine outlives the call.
func TestRunCancellableLeavesNoGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 200; i++ {
		switch i % 4 {
		case 0: // normal return
			ctx, stop := context.WithCancel(context.Background())
			_, _ = runCancellable(ctx, int64(i+1), func() []byte { return []byte("ok") }, func(int64) {})
			stop()
		case 1: // ctx done up front
			_, _ = runCancellable(causedCtx(errCauseX), int64(i+1), func() []byte { return nil }, func(int64) {})
		case 2: // cancelled mid-call
			ctx, stop := context.WithCancel(context.Background())
			release := make(chan struct{})
			started := make(chan struct{})
			go func() { <-started; stop() }()
			_, _ = runCancellable(ctx, int64(i+1), func() []byte { close(started); <-release; return nil },
				func(int64) { close(release) })
		case 3: // ctx ends as the call returns
			ctx, stop := context.WithCancel(context.Background())
			_, _ = runCancellable(ctx, int64(i+1), func() []byte { stop(); return []byte("r") }, func(int64) {})
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines: %d before, %d after 200 calls: a watcher outlived its call", before, after)
	}
}

// (e), continued: a watcher is stopped and joined when its ctx never ends either. Every ctx in the
// test above ends after runCancellable returns, and that alone would release a watcher that was not
// stopped; context.Background() never ends, so only the return path can end this watcher.
func TestRunCancellableLeavesNoWatcherWithLiveCtx(t *testing.T) {
	before := runtime.NumGoroutine()
	for i := 0; i < 200; i++ {
		_, _ = runCancellable(context.Background(), int64(i+1), func() []byte { return []byte("ok") }, func(int64) {})
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before {
		t.Errorf("goroutines: %d before, %d after 200 normal calls with a ctx that never ends: a watcher outlived its call", before, after)
	}
}

func TestNextAppleTokenIsPositiveIncreasingAndUnique(t *testing.T) {
	a := nextAppleToken()
	b := nextAppleToken()
	if a < 1 {
		t.Errorf("first token = %d, want >= 1 (0 or less can never be cancelled)", a)
	}
	if b != a+1 {
		t.Errorf("tokens %d then %d, want consecutive", a, b)
	}
	const n = 500
	seen := make(chan int64, 8*n)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n; i++ {
				seen <- nextAppleToken()
			}
		}()
	}
	wg.Wait()
	close(seen)
	uniq := map[int64]bool{}
	for v := range seen {
		if v <= b || uniq[v] {
			t.Fatalf("token %d repeated or not above %d", v, b)
		}
		uniq[v] = true
	}
}

// appleCancelOutcome is the single place that decides "the bridge said cancelled": with a done ctx
// the answer is context.Cause(ctx); with a live ctx it is never a cancel and never a context error.
func TestAppleCancelOutcome(t *testing.T) {
	t.Run("cancelled payload, ctx done: the cause, handled", func(t *testing.T) {
		err, handled := appleCancelOutcome(causedCtx(errCauseX), "cancelled")
		if !handled || !errors.Is(err, errCauseX) {
			t.Errorf("got (%v, %v), want (cause-x, true)", err, handled)
		}
	})
	t.Run("plain context.Canceled cause is returned as is", func(t *testing.T) {
		ctx, stop := context.WithCancel(context.Background())
		stop()
		err, handled := appleCancelOutcome(ctx, "cancelled")
		if !handled || !errors.Is(err, context.Canceled) {
			t.Errorf("got (%v, %v), want (context.Canceled, true)", err, handled)
		}
	})
	t.Run("cancelled payload nobody asked for: an ordinary engine error, not a cancel", func(t *testing.T) {
		err, handled := appleCancelOutcome(context.Background(), "cancelled")
		if err != nil && errors.Is(err, context.Canceled) {
			t.Errorf("live ctx produced a cancel-shaped error: %v", err)
		}
		if handled && err == nil {
			t.Error("handled with a nil error would report success for a translation that never happened")
		}
	})
	for _, code := range []string{"", "apple_translate", "no_source_lang", "empty_text", "target_required", "future_code"} {
		code := code
		t.Run("code "+code+" is not a cancel", func(t *testing.T) {
			for _, ctx := range []context.Context{context.Background(), causedCtx(errCauseX)} {
				if err, handled := appleCancelOutcome(ctx, code); handled || err != nil {
					t.Errorf("ctx.Err=%v: got (%v, %v), want (nil, false): a result or another error stands", ctx.Err(), err, handled)
				}
			}
		})
	}
}
