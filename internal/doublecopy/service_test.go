package doublecopy

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// ---- fakes ----

type fakeSource struct {
	queue    []Event
	startErr error
	starts   int
	stops    int
	running  bool
}

func (f *fakeSource) Start() error {
	f.starts++
	if f.startErr != nil {
		return f.startErr
	}
	f.running = true
	return nil
}
func (f *fakeSource) Stop() { f.stops++; f.running = false }
func (f *fakeSource) Poll() []Event {
	q := f.queue
	f.queue = nil
	return q
}

type fakePB struct {
	count int64
	types []string
	text  string
	// advanceOnSleep: the app "writes the pasteboard" during the Nth sleep (1-based); 0 = never.
	advanceOnSleep int
	sleeps         int
}

func (p *fakePB) ChangeCount() int64 { return p.count }
func (p *fakePB) Types() []string    { return p.types }
func (p *fakePB) Text() string       { return p.text }
func (p *fakePB) sleep(time.Duration) {
	p.sleeps++
	if p.advanceOnSleep > 0 && p.sleeps == p.advanceOnSleep {
		p.count++
	}
}

type rig struct {
	svc       *Service
	src       *fakeSource
	pb        *fakePB
	delivered []string
	requests  int
	missing   int
	enabled   bool
	logs      *bytes.Buffer
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{src: &fakeSource{}, pb: &fakePB{count: 10, text: "hola, esto es una prueba", advanceOnSleep: 1}, enabled: true, logs: &bytes.Buffer{}}
	r.svc = New(Config{
		Source:              r.src,
		Pasteboard:          r.pb,
		Enabled:             func() bool { return r.enabled },
		Deliver:             func(s string) { r.delivered = append(r.delivered, s) },
		RequestPermission:   func() { r.requests++ },
		OnPermissionMissing: func() { r.missing++ },
		Sleep:               r.pb.sleep,
		Log:                 slog.New(slog.NewTextHandler(r.logs, &slog.HandlerOptions{Level: slog.LevelDebug})),
	})
	return r
}

// pair queues a real Cmd+C pair 200 ms apart with the pasteboard at change count cc.
func (r *rig) pair(bundle string, cc int64) {
	a, b := copyEv(0), copyEv(200)
	a.ChangeCount, b.ChangeCount, a.Bundle, b.Bundle = cc, cc, bundle, bundle
	r.src.queue = append(r.src.queue, a, b)
}

// ---- behaviour ----

func TestDoublePressWithAdvancedPasteboardDeliversTheText(t *testing.T) {
	r := newRig(t)
	r.pair("com.apple.Safari", 10)
	r.svc.Step()
	if len(r.delivered) != 1 || r.delivered[0] != "hola, esto es una prueba" {
		t.Fatalf("delivered %q, want the copied text once", r.delivered)
	}
}

func TestSinglePressDeliversNothing(t *testing.T) {
	r := newRig(t)
	a := copyEv(0)
	a.ChangeCount = 10
	r.src.queue = []Event{a}
	r.svc.Step()
	if len(r.delivered) != 0 {
		t.Fatalf("a single Cmd+C delivered %q", r.delivered)
	}
}

func TestThreePressesDeliverOnce(t *testing.T) {
	r := newRig(t)
	r.pair("com.apple.Safari", 10)
	third := copyEv(400)
	third.ChangeCount, third.Bundle = 10, "com.apple.Safari"
	r.src.queue = append(r.src.queue, third)
	r.svc.Step()
	if len(r.delivered) != 1 {
		t.Fatalf("three quick presses delivered %d times, want 1", len(r.delivered))
	}
}

func TestPairsAcrossPollsStillPair(t *testing.T) {
	r := newRig(t)
	a, b := copyEv(0), copyEv(200)
	a.ChangeCount, b.ChangeCount = 10, 10
	r.src.queue = []Event{a}
	r.svc.Step()
	r.src.queue = []Event{b}
	r.svc.Step()
	if len(r.delivered) != 1 {
		t.Fatalf("a pair split over two polls delivered %d times, want 1", len(r.delivered))
	}
}

func TestUnchangedPasteboardDeliversNothing(t *testing.T) {
	// Cmd+C with nothing selected: the app never writes the pasteboard.
	r := newRig(t)
	r.pb.advanceOnSleep = 0
	r.pair("com.apple.Safari", 10)
	r.svc.Step()
	if len(r.delivered) != 0 {
		t.Fatalf("delivered %q although the pasteboard never changed", r.delivered)
	}
	if r.pb.sleeps == 0 || r.pb.sleeps > 60 {
		t.Fatalf("waited %d ticks for the pasteboard; it must wait a bounded, non-zero time", r.pb.sleeps)
	}
}

func TestPasteboardAlreadyAdvancedBeforeTheSecondPressDoesNotCount(t *testing.T) {
	// The change count recorded at key-down is what the pasteboard must move past: an old change
	// (count 9 when the press saw 10) is not the copy.
	r := newRig(t)
	r.pb.count, r.pb.advanceOnSleep = 10, 0
	r.pair("com.apple.Safari", 10)
	r.svc.Step()
	if len(r.delivered) != 0 {
		t.Fatal("delivered with a pasteboard that did not move past the key-down count")
	}
}

func TestUnknownChangeCountFailsClosed(t *testing.T) {
	r := newRig(t)
	r.pair("com.apple.Safari", 0) // the source could not read the count
	r.svc.Step()
	if len(r.delivered) != 0 {
		t.Fatal("delivered although the change count at key-down is unknown")
	}
}

func TestKaisOwnCopyDeliversNothing(t *testing.T) {
	r := newRig(t)
	a, b := copyEv(0), copyEv(100)
	a.Own, b.Own = true, true
	a.ChangeCount, b.ChangeCount = 10, 10
	r.src.queue = []Event{a, b}
	r.svc.Step()
	if len(r.delivered) != 0 {
		t.Fatal("Kai's own simulated copy triggered a translation")
	}
}

func TestDeniedAppDeliversNothing(t *testing.T) {
	for _, id := range []string{"com.1password.1password", "com.bitwarden.desktop", "com.apple.keychainaccess"} {
		r := newRig(t)
		r.pair(id, 10)
		r.svc.Step()
		if len(r.delivered) != 0 {
			t.Errorf("a copy from %s was delivered", id)
		}
	}
}

func TestConcealedPasteboardDeliversNothing(t *testing.T) {
	for _, typ := range []string{"org.nspasteboard.ConcealedType", "org.nspasteboard.TransientType", "com.agilebits.onepassword"} {
		r := newRig(t)
		r.pb.types = []string{"public.utf8-plain-text", typ}
		r.pair("com.apple.Safari", 10)
		r.svc.Step()
		if len(r.delivered) != 0 {
			t.Errorf("a pasteboard marked %s was delivered", typ)
		}
	}
}

func TestEmptyOrBlankTextDeliversNothing(t *testing.T) {
	for _, txt := range []string{"", "   \n\t "} {
		r := newRig(t)
		r.pb.text = txt
		r.pair("com.apple.Safari", 10)
		r.svc.Step()
		if len(r.delivered) != 0 {
			t.Errorf("delivered %q", r.delivered)
		}
	}
}

func TestSettingOffDeliversNothing(t *testing.T) {
	r := newRig(t)
	r.enabled = false
	r.pair("com.apple.Safari", 10)
	r.svc.Step()
	if len(r.delivered) != 0 {
		t.Fatal("delivered with the setting off")
	}
}

func TestSettingTurnedOffWhileWaitingDeliversNothing(t *testing.T) {
	r := newRig(t)
	r.pb.advanceOnSleep = 1
	orig := r.svc.cfg.Sleep
	r.svc.cfg.Sleep = func(d time.Duration) { orig(d); r.enabled = false }
	r.pair("com.apple.Safari", 10)
	r.svc.Step()
	if len(r.delivered) != 0 {
		t.Fatal("delivered after the setting was switched off during the wait")
	}
}

func TestCopiedTextIsNeverLoggedAtInfo(t *testing.T) {
	r := newRig(t)
	r.pb.text = "SECRET-PAYLOAD-12345"
	r.pair("com.apple.Safari", 10)
	r.svc.Step()
	if len(r.delivered) != 1 {
		t.Fatal("setup: nothing delivered")
	}
	if strings.Contains(r.logs.String(), "SECRET-PAYLOAD-12345") {
		t.Fatalf("the copied text reached the log:\n%s", r.logs.String())
	}
	if !strings.Contains(r.logs.String(), "level=INFO") {
		t.Fatal("setup: no info log line at all, the assertion above proves nothing")
	}
}

// ---- lifecycle and permission ----

func TestApplyOnStartsAndOffStops(t *testing.T) {
	r := newRig(t)
	r.svc.Apply(true)
	if !r.src.running || r.svc.Status() != StatusRunning {
		t.Fatalf("running=%v status=%v after Apply(true)", r.src.running, r.svc.Status())
	}
	r.svc.Apply(true) // idempotent: no second start
	if r.src.starts != 1 {
		t.Fatalf("starts = %d after two Apply(true), want 1", r.src.starts)
	}
	r.svc.Apply(false)
	if r.src.running || r.svc.Status() != StatusOff || r.src.stops != 1 {
		t.Fatalf("running=%v status=%v stops=%d after Apply(false)", r.src.running, r.svc.Status(), r.src.stops)
	}
	r.svc.Apply(false) // idempotent
	if r.src.stops != 1 {
		t.Fatalf("stops = %d after two Apply(false), want 1", r.src.stops)
	}
}

func TestStartupWithFeatureOnAndNoPermissionDoesNotPromptButTellsTheUser(t *testing.T) {
	r := newRig(t)
	r.src.startErr = ErrNoPermission
	r.svc.Apply(true) // the first Apply is app launch, not the user switching the feature on
	if r.requests != 0 {
		t.Fatalf("requested the permission %d times at launch, want 0", r.requests)
	}
	if r.missing != 1 || r.svc.Status() != StatusMissingPermission {
		t.Fatalf("missing=%d status=%v, want one message and status missing", r.missing, r.svc.Status())
	}
}

func TestSwitchingOnWithoutPermissionRequestsItOnceAndTellsTheUser(t *testing.T) {
	r := newRig(t)
	r.svc.Apply(false) // launch with the feature off
	r.src.startErr = ErrNoPermission
	r.svc.Apply(true) // the user flips it on
	if r.requests != 1 || r.missing != 1 {
		t.Fatalf("requests=%d missing=%d, want 1 and 1", r.requests, r.missing)
	}
	// Unrelated settings saves call Apply(true) again: retry quietly, no new prompt, no spam.
	r.svc.Apply(true)
	r.svc.Apply(true)
	if r.requests != 1 || r.missing != 1 {
		t.Fatalf("requests=%d missing=%d after repeated saves, want 1 and 1", r.requests, r.missing)
	}
	// The user grants it: the next save starts the listener.
	r.src.startErr = nil
	r.svc.Apply(true)
	if r.svc.Status() != StatusRunning {
		t.Fatalf("status = %v after the grant, want running", r.svc.Status())
	}
	// Off and on again prompts again (a new user decision).
	r.svc.Apply(false)
	r.src.startErr = ErrNoPermission
	r.svc.Apply(true)
	if r.requests != 2 {
		t.Fatalf("requests = %d after off/on, want 2", r.requests)
	}
}

func TestSwitchingOnWithPermissionDoesNotPrompt(t *testing.T) {
	r := newRig(t)
	r.svc.Apply(false)
	r.svc.Apply(true)
	if r.requests != 0 || r.missing != 0 {
		t.Fatalf("requests=%d missing=%d with the permission granted, want 0 and 0", r.requests, r.missing)
	}
}

func TestUnsupportedPlatformReportsUnsupportedAndNeverPrompts(t *testing.T) {
	r := newRig(t)
	r.src.startErr = ErrUnsupported
	r.svc.Apply(false)
	r.svc.Apply(true)
	if r.svc.Status() != StatusUnsupported || r.requests != 0 || r.missing != 0 {
		t.Fatalf("status=%v requests=%d missing=%d, want unsupported, 0, 0", r.svc.Status(), r.requests, r.missing)
	}
}

func TestOtherStartErrorsAreNotPermissionProblems(t *testing.T) {
	r := newRig(t)
	r.src.startErr = errors.New("boom")
	r.svc.Apply(false)
	r.svc.Apply(true)
	if r.requests != 0 || r.svc.Status() == StatusRunning {
		t.Fatalf("requests=%d status=%v for a generic start error", r.requests, r.svc.Status())
	}
}

func TestStoppingForgetsAPendingFirstPress(t *testing.T) {
	r := newRig(t)
	r.svc.Apply(true)
	a := copyEv(0)
	a.ChangeCount = 10
	r.src.queue = []Event{a}
	r.svc.Step()
	r.svc.Apply(false)
	r.svc.Apply(true)
	b := copyEv(100)
	b.ChangeCount = 10
	r.src.queue = []Event{b}
	r.svc.Step()
	if len(r.delivered) != 0 {
		t.Fatal("a press from before the feature was switched off paired with one after it")
	}
}
