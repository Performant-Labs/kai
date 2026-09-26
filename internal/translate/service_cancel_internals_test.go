package translate

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #109 Part A (T, Phase 7). White-box pins for behaviors the black-box cancel tests cannot
// reach: the once-only terminal guard, the outcome decision table, the supersede suppression of
// screenshot pushes, a done ctx never reaching the engine, and progress for unknown requests.

func TestReportOnceEmitsOnlyOncePerEngine(t *testing.T) {
	svc, _, _ := newCancelService(t, newHonoring("a").fakeEngine)
	ar := svc.requests.open("s", "req-once")
	runs := svc.requests.start(ar, []string{"a"})
	var n int
	svc.reportOnce(ar, runs[0], func() { n++ })
	svc.reportOnce(ar, runs[0], func() { n++ })
	if n != 1 {
		t.Fatalf("terminal report ran %d times, want exactly 1", n)
	}
	svc.requests.finish(ar, runs[0])
}

func TestOutcomeOfDecidesFromContextCause(t *testing.T) {
	res := okResult("x")
	boom := errors.New("boom")
	mk := func(cause error) context.Context {
		ctx, cancel := context.WithCancelCause(context.Background())
		if cause != nil {
			cancel(cause)
		} else {
			defer cancel(nil)
		}
		return ctx
	}
	cases := []struct {
		name string
		ctx  context.Context
		res  *model.TranslateResult
		err  error
		want outcomeKind
	}{
		{"superseded with a result", mk(errSuperseded), res, nil, outcomeSuperseded},
		{"superseded with an error", mk(errSuperseded), nil, boom, outcomeSuperseded},
		{"user cancel with an error", mk(errUserCancelled), nil, boom, outcomeCancelled},
		{"user cancel but the engine returned a result", mk(errUserCancelled), res, nil, outcomeSuccess},
		{"no cause, error", mk(nil), nil, boom, outcomeFailed},
		{"no cause, result", mk(nil), res, nil, outcomeSuccess},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := outcomeOf(c.ctx, c.res, c.err).kind; got != c.want {
				t.Fatalf("outcomeOf = %v, want %v", got, c.want)
			}
		})
	}
}

func TestPushScreenshotOnSupersededRunEmitsNothing(t *testing.T) {
	svc, em, _ := newCancelService(t)
	old := svc.requests.open("shot", "old")
	svc.pushScreenshot(old, model.ScreenshotResult{})
	if got := len(em.screenshots()); got != 1 {
		t.Fatalf("live run pushed %d, want 1", got)
	}
	_ = svc.requests.open("shot", "new")
	svc.pushScreenshot(old, model.ScreenshotResult{})
	if got := len(em.screenshots()); got != 1 {
		t.Fatalf("superseded run still pushed: %d screenshots, want 1", got)
	}
}

func TestCallEngineWithDoneContextDoesNotStartTheEngine(t *testing.T) {
	var calls atomic.Int32
	eng := &fakeEngine{name: "a", run: func(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		calls.Add(1)
		return okResult("x"), nil
	}}
	svc, _, _ := newCancelService(t, eng)
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(errUserCancelled)
	_, err := svc.callEngine(ctx, eng, "a", model.TranslateRequest{Text: "hi", To: "es"})
	if !errors.Is(err, errUserCancelled) {
		t.Fatalf("err = %v, want the cancel cause", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("engine was called %d times on a done ctx", calls.Load())
	}
}

func TestReportProgressSaysNothingForUnknownRequest(t *testing.T) {
	svc, em, _ := newCancelService(t)
	svc.reportProgress("nope", "a", 1, 3)
	if got := len(em.progress()); got != 0 {
		t.Fatalf("progress for an unknown request emitted %d events", got)
	}
}

// PR-Agent finding on #115: a caller that reuses an id in the same session replaces the old request
// (open supersedes it), and when the old request later finished, removeLocked deleted the SESSION
// mapping although that session now belonged to the new request under the same id, so the next
// request in the session no longer found (and no longer superseded) its predecessor.
func TestFinishingASupersededRequestKeepsTheNewRequestsSession(t *testing.T) {
	svc, _, _ := newCancelService(t, newHonoring("a").fakeEngine)
	first := svc.requests.open("s", "same-id")
	second := svc.requests.open("s", "same-id")
	if first == second {
		t.Fatal("open returned the same request twice")
	}
	svc.requests.release(first) // the superseded request finishes and is removed

	third := svc.requests.open("s", "other-id")
	if !second.isSuperseded() {
		t.Fatal("the second request was not superseded by the third: the finishing first request dropped the session mapping")
	}
	svc.requests.release(second)
	svc.requests.release(third)
	if n := svc.requests.count(); n != 0 {
		t.Fatalf("registry still holds %d requests after all were released", n)
	}
}
