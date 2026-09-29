package hotkey

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"cnb.cool/dtapp/kai/internal/events"
)

// Issue #175 item 5: TriggerInput's copy-key branch used to emit nothing when the simulated
// copy captured no text, leaving the translate window's input showing stale text from a
// previous session with no indication anything failed. copyKeyOutcome is the pure decision
// behind the fix (see manager.go for the full rationale) — these tests pin it directly, since
// TriggerInput itself needs a live application.Window/App to exercise.

func TestCopyKeyOutcome_CapturedText_EmitsInputFill(t *testing.T) {
	event, args := copyKeyOutcome("the captured selection")
	if event != events.EventInputFill {
		t.Fatalf("event = %q, want %q", event, events.EventInputFill)
	}
	if len(args) != 1 || args[0] != "the captured selection" {
		t.Fatalf("args = %#v, want [%q]", args, "the captured selection")
	}
}

func TestCopyKeyOutcome_EmptyCapture_EmitsCopyKeyFailed(t *testing.T) {
	event, args := copyKeyOutcome("")
	if event != events.EventCopyKeyFailed {
		t.Fatalf("event = %q, want %q", event, events.EventCopyKeyFailed)
	}
	if len(args) != 0 {
		t.Fatalf("args = %#v, want none (EventCopyKeyFailed carries no payload)", args)
	}
}

// fakeEmitter records every Emit call so emitCopyKeyOutcome's wiring (not just copyKeyOutcome's
// pure decision) is covered too.
type fakeEmitter struct {
	name string
	args []any
	n    int
}

func (f *fakeEmitter) Emit(name string, data ...any) bool {
	f.name = name
	f.args = data
	f.n++
	return true
}

func TestEmitCopyKeyOutcome_CapturedText(t *testing.T) {
	e := &fakeEmitter{}
	emitCopyKeyOutcome(e, "hello")
	if e.n != 1 {
		t.Fatalf("Emit called %d times, want 1", e.n)
	}
	if e.name != events.EventInputFill {
		t.Fatalf("emitted %q, want %q", e.name, events.EventInputFill)
	}
	if len(e.args) != 1 || e.args[0] != "hello" {
		t.Fatalf("args = %#v, want [%q]", e.args, "hello")
	}
}

func TestEmitCopyKeyOutcome_EmptyCapture(t *testing.T) {
	e := &fakeEmitter{}
	emitCopyKeyOutcome(e, "")
	if e.n != 1 {
		t.Fatalf("Emit called %d times, want 1", e.n)
	}
	if e.name != events.EventCopyKeyFailed {
		t.Fatalf("emitted %q, want %q", e.name, events.EventCopyKeyFailed)
	}
	if len(e.args) != 0 {
		t.Fatalf("args = %#v, want none", e.args)
	}
}

// TestTriggerInputBusy_MutualExclusion pins the concurrency contract TriggerInput's copy-key
// branch relies on (issue #175 item 5): triggerInputBusy.CompareAndSwap(false, true) must let
// exactly one concurrent caller through at a time, and Store(false) must fully release it for
// the next caller — the same shape the real code uses around CopySelection() to stop two
// overlapping hotkey firings from racing on the shared system clipboard (see the doc comments
// on the triggerInputBusy field and its call site in TriggerInput). This exercises the guard
// directly, without needing a live application.Window/App/ExecKeyController — those would need
// a running Wails event loop to invoke safely (robotgo's KeyTap dispatches through
// application.InvokeSyncWithError), which a unit test must not depend on.
func TestTriggerInputBusy_MutualExclusion(t *testing.T) {
	h := &Manager{}

	const attempts = 200
	var acquiredCount int32
	var wg sync.WaitGroup
	wg.Add(attempts)
	for range attempts {
		go func() {
			defer wg.Done()
			if h.triggerInputBusy.CompareAndSwap(false, true) {
				atomic.AddInt32(&acquiredCount, 1)
				// Hold the guard briefly, like CopySelection's real backup/clear/copy/restore
				// sequence would, so overlapping goroutines actually contend for it instead of
				// each finding it already free.
				runtime.Gosched()
				h.triggerInputBusy.Store(false)
			}
		}()
	}
	wg.Wait()

	if acquiredCount == 0 {
		t.Fatal("no goroutine ever acquired the guard — CompareAndSwap contract broken")
	}
	// Every acquisition must have been released: a final CompareAndSwap must still succeed.
	if !h.triggerInputBusy.CompareAndSwap(false, true) {
		t.Fatal("guard left held after all goroutines finished — a Store(false) release was lost")
	}
	h.triggerInputBusy.Store(false)
}
