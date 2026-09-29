package doublecopy

import (
	"testing"
	"time"
)

// Issue #199: the pure double-Cmd+C detector. It sees one event per Cmd+C key-down (stamped by the
// source, so the rule does not depend on how late Kai polls) and answers one question: is this
// press the second of a quick pair that has not fired yet?

func copyEv(atMs int) Event {
	return Event{At: time.Duration(atMs) * time.Millisecond, KeyCode: KeyCodeC, Flags: FlagCommand, ChangeCount: 1}
}

func feedAll(d *Detector, evs ...Event) (fired []int) {
	for i, e := range evs {
		if d.Feed(e) {
			fired = append(fired, i)
		}
	}
	return fired
}

func assertFired(t *testing.T, got []int, want ...int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("fired at %v, want %v", got, want)
	}
	for i, g := range got {
		if g != want[i] {
			t.Fatalf("fired at %v, want %v", got, want)
		}
	}
}

func TestWindowIsFiveHundredMilliseconds(t *testing.T) {
	if Window != 500*time.Millisecond {
		t.Fatalf("Window = %v, want 500ms", Window)
	}
}

func TestSinglePressNeverFires(t *testing.T) {
	assertFired(t, feedAll(&Detector{}, copyEv(0)))
}

func TestTwoPressesInsideTheWindowFireOnTheSecond(t *testing.T) {
	assertFired(t, feedAll(&Detector{}, copyEv(0), copyEv(300)), 1)
}

func TestWindowBoundary(t *testing.T) {
	assertFired(t, feedAll(&Detector{}, copyEv(0), copyEv(500)), 1) // exactly the window: still a pair
	assertFired(t, feedAll(&Detector{}, copyEv(0), copyEv(501)))    // one ms late: two singles
}

func TestTwoPressesFarApartDoNotFireButALaterPairDoes(t *testing.T) {
	assertFired(t, feedAll(&Detector{}, copyEv(0), copyEv(2000), copyEv(2200)), 2)
}

func TestThirdPressAfterAFireDoesNotFireAgain(t *testing.T) {
	// Three quick presses: the pair fires once, the third is part of the same burst.
	assertFired(t, feedAll(&Detector{}, copyEv(0), copyEv(200), copyEv(400)), 1)
	// Mashing: four and five presses are still one trigger.
	assertFired(t, feedAll(&Detector{}, copyEv(0), copyEv(200), copyEv(400), copyEv(600), copyEv(800)), 1)
}

func TestNewPairFiresOnceTheWindowHasPassedAfterAFire(t *testing.T) {
	// 0,200 fire; a third at 400 extends the burst; the gap to 1200 is > window, so 1200,1400 is
	// a fresh pair.
	assertFired(t, feedAll(&Detector{}, copyEv(0), copyEv(200), copyEv(400), copyEv(1200), copyEv(1400)), 1, 4)
}

func TestHeldKeyRepeatsAreIgnored(t *testing.T) {
	rep := func(ms int) Event { e := copyEv(ms); e.Autorepeat = true; return e }
	// The user holds Cmd+C: one real key-down, then a stream of auto-repeats.
	assertFired(t, feedAll(&Detector{}, copyEv(0), rep(50), rep(100), rep(150), rep(200)))
	// A repeat is not a first press either: repeat then a real press does not pair.
	assertFired(t, feedAll(&Detector{}, rep(0), copyEv(100)))
}

func TestKaisOwnSimulatedCopyIsNotCounted(t *testing.T) {
	own := func(ms int) Event { e := copyEv(ms); e.Own = true; return e }
	assertFired(t, feedAll(&Detector{}, own(0), own(100)))                    // Kai's copy, twice
	assertFired(t, feedAll(&Detector{}, own(0), copyEv(100)))                 // Kai's copy is not a first press
	assertFired(t, feedAll(&Detector{}, copyEv(0), own(100)))                 // ... nor a second one
	assertFired(t, feedAll(&Detector{}, copyEv(0), own(100), copyEv(200)), 2) // and does not break a real pair
}

func TestCopiesInsideKaiAreIgnored(t *testing.T) {
	inKai := func(ms int) Event { e := copyEv(ms); e.FrontIsKai = true; return e }
	assertFired(t, feedAll(&Detector{}, inKai(0), inKai(100)))
}

func TestOnlyPlainCmdCCounts(t *testing.T) {
	notC := copyEv(0)
	notC.KeyCode = 9 // V
	noCmd := copyEv(0)
	noCmd.Flags = 0
	withShift := copyEv(0)
	withShift.Flags = FlagCommand | FlagShift
	withOpt := copyEv(0)
	withOpt.Flags = FlagCommand | FlagOption
	withCtrl := copyEv(0)
	withCtrl.Flags = FlagCommand | FlagControl
	for name, e := range map[string]Event{"V": notC, "no command": noCmd, "cmd+shift": withShift, "cmd+option": withOpt, "cmd+ctrl": withCtrl} {
		second := e
		second.At = 100 * time.Millisecond
		if fired := feedAll(&Detector{}, e, second); len(fired) != 0 {
			t.Errorf("%s pair fired", name)
		}
	}
	// Caps lock and the fn/numpad bits do not change what the chord is.
	e := copyEv(0)
	e.Flags |= 0x10000 | 0x800000 | 0x200000
	second := e
	second.At = 100 * time.Millisecond
	assertFired(t, feedAll(&Detector{}, e, second), 1)
}

func TestClockGoingBackwardsStartsOver(t *testing.T) {
	assertFired(t, feedAll(&Detector{}, copyEv(1000), copyEv(200)))
	assertFired(t, feedAll(&Detector{}, copyEv(1000), copyEv(200), copyEv(300)), 2)
}

func TestResetForgetsAPendingFirstPress(t *testing.T) {
	d := &Detector{}
	d.Feed(copyEv(0))
	d.Reset()
	if d.Feed(copyEv(100)) {
		t.Fatal("a press before Reset paired with a press after it")
	}
}
