package service

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Issue #69: a tray click toggles the translate window. The show branch must go through
// WindowWrapper.ShowTranslateWindow (the window is created hidden, so a bare Show() only builds
// the webview and displays nothing), and the hide branch is a bare Hide(). ToggleTranslateWindow
// is the package-level seam main.go's tray handler delegates to; it deliberately takes no
// clipboard/selection collaborator, so a tray click cannot capture anything.

type fakeToggleWindow struct {
	visible bool
	hides   int
}

func (f *fakeToggleWindow) IsVisible() bool { return f.visible }
func (f *fakeToggleWindow) Hide() application.Window {
	f.hides++
	f.visible = false
	return nil
}

func TestToggleTranslateWindowShowsThroughShowPathWhenHidden(t *testing.T) {
	win := &fakeToggleWindow{visible: false}
	shows := 0

	ToggleTranslateWindow(win, func() { shows++ })

	if shows != 1 {
		t.Errorf("hidden window: show path called %d times, want exactly 1", shows)
	}
	if win.hides != 0 {
		t.Errorf("hidden window: Hide called %d times, want 0", win.hides)
	}
}

func TestToggleTranslateWindowHidesWithoutShowingWhenVisible(t *testing.T) {
	win := &fakeToggleWindow{visible: true}
	shows := 0

	ToggleTranslateWindow(win, func() { shows++ })

	if win.hides != 1 {
		t.Errorf("visible window: Hide called %d times, want exactly 1", win.hides)
	}
	if shows != 0 {
		t.Errorf("visible window: show path called %d times, want 0", shows)
	}
}

func TestToggleTranslateWindowTwiceReturnsToShown(t *testing.T) {
	win := &fakeToggleWindow{visible: true}
	shows := 0
	show := func() { shows++; win.visible = true }

	ToggleTranslateWindow(win, show) // visible -> hide
	ToggleTranslateWindow(win, show) // hidden -> show

	if win.hides != 1 || shows != 1 {
		t.Errorf("hide then show: hides=%d shows=%d, want 1 and 1", win.hides, shows)
	}
}

// Issue #69: opening Settings must not leave it hidden behind a pinned translate window.
type fakeLevelWindow struct {
	visible bool
	calls   []bool
}

func (f *fakeLevelWindow) IsVisible() bool { return f.visible }
func (f *fakeLevelWindow) SetAlwaysOnTop(b bool) application.Window {
	f.calls = append(f.calls, b)
	return nil
}

func TestLowerForSettingsDropsAVisibleWindowToNormalLevel(t *testing.T) {
	w := &fakeLevelWindow{visible: true}
	LowerForSettings(w)
	if len(w.calls) != 1 || w.calls[0] != false {
		t.Fatalf("SetAlwaysOnTop calls = %v, want exactly one false", w.calls)
	}
}

func TestLowerForSettingsLeavesAHiddenWindowAlone(t *testing.T) {
	w := &fakeLevelWindow{visible: false}
	LowerForSettings(w)
	if len(w.calls) != 0 {
		t.Fatalf("a hidden window must not be touched, got calls %v", w.calls)
	}
}
