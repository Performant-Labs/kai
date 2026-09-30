package hotkey

import (
	"log/slog"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/doublecopy"
	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/execkey"
	"cnb.cool/dtapp/kai/internal/settings"
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
	h.triggerCopyKey(w, e, func() (string, error) { return "sel", nil })
	if w.shows != 1 || w.focuses != 1 {
		t.Fatalf("shows=%d focuses=%d, want 1/1", w.shows, w.focuses)
	}
	if e.n != 1 || e.name != events.EventInputFill || len(e.args) != 1 || e.args[0] != "sel" {
		t.Fatalf("emit = %d %q %#v", e.n, e.name, e.args)
	}
}

func TestTriggerCopyKey_EmptyCapture_EmitsCopyKeyFailed(t *testing.T) {
	h, w, e := newTestManager(), &fakeWindow{}, &fakeEmitter{}
	h.triggerCopyKey(w, e, func() (string, error) { return "", nil })
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
		h.triggerCopyKey(w1, e, func() (string, error) { close(inCopy); <-release; return "first", nil })
		close(done)
	}()
	<-inCopy
	called := false
	h.triggerCopyKey(w2, e, func() (string, error) { called = true; return "second", nil })
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
	h.triggerCopyKey(w3, e, func() (string, error) { return "third", nil })
	if w3.shows != 1 {
		t.Fatal("guard was not released after the first call finished")
	}
}

// ---- Issue #194: a missing Accessibility permission is reported as such, once in a while ----

func missingPermission() (string, error) { return "", execkey.ErrAccessibilityMissing }

func TestTriggerCopyKey_PermissionMissing_EmitsTheAccessibilityMessageNotCopyKeyFailed(t *testing.T) {
	h, w, e := newTestManager(), &fakeWindow{}, &fakeEmitter{}
	h.triggerCopyKey(w, e, missingPermission)
	if w.shows != 1 || w.focuses != 1 {
		t.Fatalf("shows=%d focuses=%d, want the window still shown 1/1", w.shows, w.focuses)
	}
	if e.n != 1 || e.name != events.EventAccessibilityMissing || len(e.args) != 0 {
		t.Fatalf("emit = %d %q %#v, want one %q", e.n, e.name, e.args, events.EventAccessibilityMissing)
	}
}

func TestTriggerCopyKey_PermissionPresentAndNothingSelected_DoesNotClaimAPermissionProblem(t *testing.T) {
	h, w, e := newTestManager(), &fakeWindow{}, &fakeEmitter{}
	h.triggerCopyKey(w, e, func() (string, error) { return "", nil })
	if e.name != events.EventCopyKeyFailed || e.n != 1 {
		t.Fatalf("emit = %d %q, want the ordinary copy-key failure", e.n, e.name)
	}
}

func TestTriggerCopyKey_PermissionMissing_IsNotRepeatedOnEveryPress(t *testing.T) {
	h, e := newTestManager(), &fakeEmitter{}
	clock := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	h.now = func() time.Time { return clock }

	press := func() { h.triggerCopyKey(&fakeWindow{}, e, missingPermission) }
	press()
	clock = clock.Add(10 * time.Second)
	press()
	clock = clock.Add(accessibilityNoticeEvery - 11*time.Second)
	press()
	if e.n != 1 {
		t.Fatalf("emitted %d times for three presses inside the throttle, want 1", e.n)
	}
	clock = clock.Add(2 * time.Second) // now more than the interval after the first message
	press()
	if e.n != 2 || e.name != events.EventAccessibilityMissing {
		t.Fatalf("emitted %d (%q) after the interval, want a second message", e.n, e.name)
	}
}

func TestTriggerCopyKey_PermissionMissing_WindowStillShownWhileThrottled(t *testing.T) {
	h, e := newTestManager(), &fakeEmitter{}
	h.triggerCopyKey(&fakeWindow{}, e, missingPermission)
	w := &fakeWindow{}
	h.triggerCopyKey(w, e, missingPermission)
	if w.shows != 1 || e.n != 1 {
		t.Fatalf("shows=%d emits=%d, want the window shown and no second message", w.shows, e.n)
	}
}

// ---- Issue #199: double Cmd+C feeds text in through the same arrival path as auto-clipboard ----

type fakeWin struct{ shown, focused int }

func (f *fakeWin) Show() application.Window { f.shown++; return nil }
func (f *fakeWin) Focus()                   { f.focused++ }

func TestShowAndFill_ShowsThenEmitsInputFill(t *testing.T) {
	w, e := &fakeWin{}, &fakeEmitter{}
	showAndFill(w, e, "hola mundo")
	if w.shown != 1 || w.focused != 1 {
		t.Fatalf("shown=%d focused=%d, want 1 and 1", w.shown, w.focused)
	}
	if e.n != 1 || e.name != events.EventInputFill || len(e.args) != 1 || e.args[0] != "hola mundo" {
		t.Fatalf("emitted %q %#v (%d times), want one EventInputFill with the text", e.name, e.args, e.n)
	}
}

func TestShowAndFill_EmptyTextShowsButEmitsNothing(t *testing.T) {
	w, e := &fakeWin{}, &fakeEmitter{}
	showAndFill(w, e, "")
	if w.shown != 1 || e.n != 0 {
		t.Fatalf("shown=%d emits=%d, want the window shown and nothing emitted", w.shown, e.n)
	}
}

func TestDeliverDoubleCopy_UsesTheSharedFillPathOnTheTranslateWindow(t *testing.T) {
	w, e := &fakeWin{}, &fakeEmitter{}
	h := &Manager{log: slog.Default(), mainWindow: func() application.Window { return nil }}
	h.deliverDoubleCopy(w, e, "bonjour")
	if w.shown != 1 || e.name != events.EventInputFill || e.args[0] != "bonjour" {
		t.Fatalf("shown=%d emitted=%q %#v", w.shown, e.name, e.args)
	}
}

type fakeDC struct {
	applied []bool
	status  doublecopy.Status
}

func (f *fakeDC) Apply(on bool)             { f.applied = append(f.applied, on) }
func (f *fakeDC) Status() doublecopy.Status { return f.status }

func TestSyncDoubleCopy_FollowsTheSetting(t *testing.T) {
	dc := &fakeDC{}
	h := &Manager{log: slog.Default(), doubleCopy: dc}
	h.syncDoubleCopy(&settings.Settings{DoubleCopyTranslate: true})
	h.syncDoubleCopy(&settings.Settings{DoubleCopyTranslate: false})
	if len(dc.applied) != 2 || !dc.applied[0] || dc.applied[1] {
		t.Fatalf("Apply calls = %v, want [true false]", dc.applied)
	}
}

func TestSyncDoubleCopy_NilControllerAndNilConfigAreSafe(t *testing.T) {
	(&Manager{log: slog.Default()}).syncDoubleCopy(&settings.Settings{DoubleCopyTranslate: true})
	dc := &fakeDC{}
	(&Manager{log: slog.Default(), doubleCopy: dc}).syncDoubleCopy(nil)
	if len(dc.applied) != 1 || dc.applied[0] {
		t.Fatalf("a nil config must switch the feature off, got %v", dc.applied)
	}
}

func TestDoubleCopyStatusString(t *testing.T) {
	h := &Manager{log: slog.Default(), doubleCopy: &fakeDC{status: doublecopy.StatusMissingPermission}}
	if got := h.DoubleCopyStatus(); got != "missing_permission" {
		t.Fatalf("status = %q", got)
	}
	if got := (&Manager{log: slog.Default()}).DoubleCopyStatus(); got != "off" {
		t.Fatalf("status with no controller = %q, want off", got)
	}
}

func TestRegister_SyncsTheDoubleCopyListenerEvenBeforeTheAppExists(t *testing.T) {
	st, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st.Get().DoubleCopyTranslate = true
	dc := &fakeDC{}
	h := &Manager{log: slog.Default(), settingsSvc: st, doubleCopy: dc}
	h.Register() // app is nil: the hotkey part returns early, the listener sync must not
	if len(dc.applied) != 1 || !dc.applied[0] {
		t.Fatalf("Apply calls = %v, want [true]", dc.applied)
	}
	st.Get().DoubleCopyTranslate = false
	h.Register()
	if len(dc.applied) != 2 || dc.applied[1] {
		t.Fatalf("Apply calls = %v, want [true false]", dc.applied)
	}
}

func TestUnregister_StopsTheDoubleCopyListener(t *testing.T) {
	dc := &fakeDC{}
	(&Manager{log: slog.Default(), doubleCopy: dc}).Unregister()
	if len(dc.applied) != 1 || dc.applied[0] {
		t.Fatalf("Apply calls = %v, want [false]", dc.applied)
	}
}
