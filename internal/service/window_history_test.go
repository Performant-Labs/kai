package service

import "testing"

// PR #67 review (8189ccb): WindowWrapper.ShowHistory lowers a pinned translate window before
// summoning the history window (the #69 rule applied to the history screen), and that call
// had no test. showHistory is ShowHistory's seam — the same shape as ToggleTranslateWindow:
// the app's window lookups stay in the wrapper, the rule is exercised here with the
// fakeLevelWindow window_toggle_test.go already defines. Every assertion fails if the
// LowerForSettings call (or the show) is removed from the seam.

func TestShowHistoryLowersAVisibleTranslateWindowThenShows(t *testing.T) {
	tw := &fakeLevelWindow{visible: true}
	shows := 0

	showHistory(tw, func() { shows++ })

	if len(tw.calls) != 1 || tw.calls[0] != false {
		t.Fatalf("a visible translate window must be lowered exactly once (SetAlwaysOnTop(false)), got %v", tw.calls)
	}
	if shows != 1 {
		t.Fatalf("the history window must be shown exactly once, got %d calls", shows)
	}
}

func TestShowHistoryLeavesAHiddenTranslateWindowAloneButStillShows(t *testing.T) {
	tw := &fakeLevelWindow{visible: false}
	shows := 0

	showHistory(tw, func() { shows++ })

	if len(tw.calls) != 0 {
		t.Fatalf("a hidden translate window must not be touched, got %v", tw.calls)
	}
	if shows != 1 {
		t.Fatalf("the history window must be shown exactly once, got %d calls", shows)
	}
}

func TestShowHistoryShowsEvenWithoutATranslateWindow(t *testing.T) {
	// The app lookup found no translate window (a nil app, or the window is not registered):
	// nothing can be lowered, but the history window must still be summoned.
	shows := 0

	showHistory(nil, func() { shows++ })

	if shows != 1 {
		t.Fatalf("show called %d times, want 1", shows)
	}
}
