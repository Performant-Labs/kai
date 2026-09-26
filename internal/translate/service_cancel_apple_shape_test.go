package translate

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #111 criterion 7 (Tester). An engine shaped like the Apple engine after this story: it
// blocks until its ctx is done and then returns context.Cause(ctx) with the error chain intact
// (unlike `honoring`, which flattens it). It is a sibling of honoring/ignoring and reuses
// newCancelService (handoff-A finding 5). It uses only symbols that exist in Part A, so it is a
// guard that must already pass at RED and keep passing: the service never treats such a cancel as
// a failure.
func TestCancelledAppleShapedEngineIsNotAFailure(t *testing.T) {
	started := make(chan struct{})
	var returned atomic.Bool
	apple := &fakeEngine{name: "apple", run: func(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		close(started)
		<-ctx.Done()
		returned.Store(true)
		return nil, context.Cause(ctx)
	}}
	before := runtime.NumGoroutine()
	svc, em, hist := newCancelService(t, apple)
	if _, err := svc.TranslateMulti(multiReq("r-apple")); err != nil {
		t.Fatal(err)
	}
	<-started
	if !svc.CancelTranslate("r-apple", "") {
		t.Fatal("CancelTranslate of a live request returned false")
	}
	waitUntil(t, "terminal payload", func() bool { return len(em.resultsFor("r-apple", "apple")) >= 1 })
	waitUntil(t, "registry empty", func() bool { return svc.activeRequestCount() == 0 })
	waitUntil(t, "engine goroutine ended", func() bool { return returned.Load() })

	got := em.resultsFor("r-apple", "apple")
	if len(got) != 1 {
		t.Fatalf("%d terminal payloads, want exactly 1", len(got))
	}
	r := got[0]
	if !r.Cancelled {
		t.Errorf("payload not flagged Cancelled: %+v", r)
	}
	if r.Error != "" || r.ErrorKind != "" {
		t.Errorf("a cancel was reported as a failure: Error=%q ErrorKind=%q", r.Error, r.ErrorKind)
	}
	if n := historyRows(t, hist, "Hola"); n != 0 {
		t.Errorf("a cancelled Apple-shaped engine wrote %d history rows, want 0", n)
	}
	// no engine goroutine is left behind (the registry, test service and emitter add none of their own once idle)
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before+4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if after := runtime.NumGoroutine(); after > before+4 {
		t.Errorf("goroutines: %d before, %d after: the engine goroutine was not released", before, after)
	}
}
