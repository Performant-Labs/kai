package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/settings"
)

// Issue #173 item 5: the translate window's size should survive a relaunch. These tests cover
// translateWindowSize's resolution logic in isolation (no native window needed); the actual
// resize -> save -> relaunch -> restore path was additionally hand-tested with a real
// build-install-relaunch cycle (see the PR description).

func newTestSettingsService(t *testing.T) *settings.Service {
	t.Helper()
	dir := t.TempDir()
	svc, err := settings.NewService(dir)
	if err != nil {
		t.Fatalf("settings.NewService: %v", err)
	}
	return svc
}

func TestTranslateWindowSize_FallsBackWhenNeverSaved(t *testing.T) {
	svc := newTestSettingsService(t)
	w, h := translateWindowSize(svc, 960, 640, 780, 520)
	if w != 960 || h != 640 {
		t.Fatalf("got %dx%d, want the 960x640 default", w, h)
	}
}

func TestTranslateWindowSize_UsesSavedValue(t *testing.T) {
	svc := newTestSettingsService(t)
	cfg := svc.Get()
	cfg.TranslateWindowWidth = 1100
	cfg.TranslateWindowHeight = 700
	w, h := translateWindowSize(svc, 960, 640, 780, 520)
	if w != 1100 || h != 700 {
		t.Fatalf("got %dx%d, want the saved 1100x700", w, h)
	}
}

func TestTranslateWindowSize_FallsBackWhenSavedBelowMinimum(t *testing.T) {
	svc := newTestSettingsService(t)
	cfg := svc.Get()
	// Simulates a corrupt/stale saved value smaller than the window's own MinWidth/MinHeight.
	cfg.TranslateWindowWidth = 100
	cfg.TranslateWindowHeight = 100
	w, h := translateWindowSize(svc, 960, 640, 780, 520)
	if w != 960 || h != 640 {
		t.Fatalf("got %dx%d, want the 960x640 default (saved value was below minimum)", w, h)
	}
}

func TestTranslateWindowSize_NilServiceFallsBack(t *testing.T) {
	w, h := translateWindowSize(nil, 960, 640, 780, 520)
	if w != 960 || h != 640 {
		t.Fatalf("got %dx%d, want the 960x640 default", w, h)
	}
}

// TestTranslateWindowSize_PersistsAcrossServiceReload simulates the actual relaunch path at
// the settings layer: save a size, construct a brand-new settings.Service against the same
// directory (as a real relaunch would), and confirm the saved size round-trips through
// settings.json.
func TestTranslateWindowSize_PersistsAcrossServiceReload(t *testing.T) {
	dir := t.TempDir()
	svc, err := settings.NewService(dir)
	if err != nil {
		t.Fatalf("settings.NewService: %v", err)
	}
	cfg := svc.Get()
	cfg.TranslateWindowWidth = 1234
	cfg.TranslateWindowHeight = 789
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// A real relaunch constructs a fresh Service against the same on-disk settings.json.
	svc2, err := settings.NewService(dir)
	if err != nil {
		t.Fatalf("settings.NewService (reload): %v", err)
	}
	w, h := translateWindowSize(svc2, 960, 640, 780, 520)
	if w != 1234 || h != 789 {
		t.Fatalf("got %dx%d after reload, want the saved 1234x789", w, h)
	}

	// Sanity: the value really did land in settings.json (not just in-memory).
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("settings.json is empty")
	}
}

// Issue #195: the default text is 120%, so the resizable windows' widths (default and minimum)
// grow with it; the fixed settings window does not.
func TestWidthForDefaultText(t *testing.T) {
	for in, want := range map[int]int{960: 1152, 780: 936, 900: 1080, 600: 720} {
		if got := widthForDefaultText(in); got != want {
			t.Errorf("widthForDefaultText(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestResizableWindowsUseTheScaledWidths(t *testing.T) {
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	for _, want := range []string{
		"translateWindowSize(settingsService, widthForDefaultText(960), 640, widthForDefaultText(780), 520)",
		"MinWidth:  widthForDefaultText(780),",
		"Width:     widthForDefaultText(900),",
		"MinWidth:  widthForDefaultText(600),",
		"Width:  1280,",
		"MaxWidth:      1280,",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("main.go lacks %q", want)
		}
	}
}
