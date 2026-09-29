package hotkey

import (
	"log/slog"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

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

type fakeWindow struct{ shows, focuses int }

func (f *fakeWindow) Show() application.Window { f.shows++; return nil }
func (f *fakeWindow) Focus()                   { f.focuses++ }

func newTestManager() *Manager { return &Manager{log: slog.Default()} }

// These call triggerCopyKey (TriggerInput's copy-key branch) so that removing the busy-guard or
// the outcome emission from it fails a test (issue #175 item 5).

func TestTriggerCopyKey_CapturedText_ShowsWindowAndEmitsFill(t *testing.T) {
	h, w, e := newTestManager(), &fakeWindow{}, &fakeEmitter{}
	h.triggerCopyKey(w, e, func() string { return "sel" })
	if w.shows != 1 || w.focuses != 1 {
		t.Fatalf("shows=%d focuses=%d, want 1/1", w.shows, w.focuses)
	}
	if e.n != 1 || e.name != events.EventInputFill || len(e.args) != 1 || e.args[0] != "sel" {
		t.Fatalf("emit = %d %q %#v", e.n, e.name, e.args)
	}
}

func TestTriggerCopyKey_EmptyCapture_EmitsCopyKeyFailed(t *testing.T) {
	h, w, e := newTestManager(), &fakeWindow{}, &fakeEmitter{}
	h.triggerCopyKey(w, e, func() string { return "" })
	if w.shows != 1 || e.n != 1 || e.name != events.EventCopyKeyFailed {
		t.Fatalf("shows=%d emit=%d %q", w.shows, e.n, e.name)
	}
}

func TestTriggerCopyKey_OverlappingCallIsSkipped(t *testing.T) {
	h, e := newTestManager(), &fakeEmitter{}
	w1, w2 := &fakeWindow{}, &fakeWindow{}
	inCopy, release := make(chan struct{}), make(chan struct{})
	done := make(chan struct{})
	go func() {
		h.triggerCopyKey(w1, e, func() string { close(inCopy); <-release; return "first" })
		close(done)
	}()
	<-inCopy
	called := false
	h.triggerCopyKey(w2, e, func() string { called = true; return "second" })
	if called || w2.shows != 0 {
		t.Fatal("overlapping call must not run CopySelection or touch the window")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("first call never finished")
	}
	if e.n != 1 || e.args[0] != "first" {
		t.Fatalf("emit count=%d args=%#v, want only the first call's fill", e.n, e.args)
	}
	// Guard released: a later call runs normally.
	w3 := &fakeWindow{}
	h.triggerCopyKey(w3, e, func() string { return "third" })
	if w3.shows != 1 {
		t.Fatal("guard was not released after the first call finished")
	}
}
