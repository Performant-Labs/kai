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
	// gofmt aligns the values of a struct literal's keys, so a longer neighbouring key (issue #15 added
	// BackgroundColour) changes the spacing after a colon; compare with whitespace runs collapsed.
	collapse := func(s string) string { return strings.Join(strings.Fields(s), " ") }
	text := collapse(string(src))
	for _, want := range []string{
		"translateWindowSize(settingsService, widthForDefaultText(960), 640, widthForDefaultText(780), 520)",
		"MinWidth:  widthForDefaultText(780),",
		"Width:     widthForDefaultText(900),",
		"MinWidth:  widthForDefaultText(600),",
		"Width:  1280,",
		"MaxWidth:      1280,",
	} {
		if !strings.Contains(text, collapse(want)) {
			t.Errorf("main.go lacks %q", want)
		}
	}
}

// Issues #112 + #113: the creation params the issues name, and the main.go wiring that uses
// them. translateWindowStartsPinned is the translateWindowSize shape one level over: a pure
// decision over settings that main.go feeds into the translate window's AlwaysOnTop at
// creation (#113 — the window must START at the level the setting says, not whatever the
// frontend applies after mount). The wiring itself (hidden close button, the closing hook's
// composition, the Cmd+W binding, the AlwaysOnTop feed) lives in main.go against a live Wails
// window, so it is pinned as source assertions on main.go — the pattern
// TestResizableWindowsUseTheScaledWidths uses directly above. What CANNOT be tested here is
// the real-app behaviour (the traffic-light button actually not rendering, the key binding
// actually firing, the level actually dropping behind other apps): those stay manual
// verification items on a signed build (brief W-1/W-3).

func TestTranslateWindowStartsPinned_OffByDefaultStartsAtTheNormalLevel(t *testing.T) {
	svc := newTestSettingsService(t)
	if translateWindowStartsPinned(svc) {
		t.Fatal("translateWindowStartsPinned(fresh settings) = true, want false: the #69 setting is off by default, so the window must start at the normal level (#113)")
	}
}

func TestTranslateWindowStartsPinned_OnStartsAlwaysOnTop(t *testing.T) {
	svc := newTestSettingsService(t)
	svc.Get().TranslateWindowOnTop = true
	if !translateWindowStartsPinned(svc) {
		t.Fatal("translateWindowStartsPinned(setting on) = false, want true")
	}
}

func TestTranslateWindowStartsPinned_NilServiceStartsAtTheNormalLevel(t *testing.T) {
	if translateWindowStartsPinned(nil) {
		t.Fatal("translateWindowStartsPinned(nil) = true, want false (an unreadable preference must never pin the window)")
	}
}

// mainGoText returns main.go with whitespace runs collapsed (gofmt's key alignment makes raw
// contains-checks brittle — same trick as TestResizableWindowsUseTheScaledWidths).
func mainGoText(t *testing.T) string {
	t.Helper()
	src, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(strings.Fields(string(src)), " ")
}

// windowOptionsBlock returns main.go's collapsed source from windowName's `Name: model.X,`
// line through the end of that NewWithOptions literal, so per-window assertions (the close
// button state, the AlwaysOnTop feed) cannot match a different window's options.
func windowOptionsBlock(t *testing.T, windowName string) string {
	t.Helper()
	text := mainGoText(t)
	start := strings.Index(text, "Name: model."+windowName+",")
	if start < 0 {
		t.Fatalf("main.go has no window named model.%s", windowName)
	}
	end := strings.Index(text[start:], "})")
	if end < 0 {
		t.Fatalf("main.go: no end of options literal after model.%s", windowName)
	}
	return text[start : start+end]
}

func TestTranslateWindowCloseButtonIsHidden(t *testing.T) {
	// #112 AC1: the translate window offers no close affordance at all — hidden, not merely
	// disabled ("cannot close", and the issue's own ruling that hide is not a close).
	block := windowOptionsBlock(t, "WindowTranslate")
	if !strings.Contains(block, "CloseButtonState: application.ButtonHidden") {
		t.Errorf("the translate window's CloseButtonState must be application.ButtonHidden (issue #112); options block: %s", block)
	}
	if strings.Contains(block, "CloseButtonState: application.ButtonEnabled") {
		t.Errorf("the translate window must not keep an enabled close button (issue #112); options block: %s", block)
	}
	// The auxiliary windows are unchanged: their close affordance stays enabled (their
	// hide-on-close hooks already exist and are correct).
	for _, name := range []string{"WindowSettings", "WindowHistory", "WindowScreenshot"} {
		if b := windowOptionsBlock(t, name); !strings.Contains(b, "CloseButtonState: application.ButtonEnabled") {
			t.Errorf("model.%s must keep CloseButtonState: application.ButtonEnabled (only the translate window's close affordance is removed); options block: %s", name, b)
		}
	}
}

func TestTranslateWindowClosingHookComposesCancelThenTheCloseRule(t *testing.T) {
	// #112 AC2: main.go's WindowClosing hook composes event.Cancel() (Wails' own WindowClosing
	// listener destroys the window unless cancelled) + the tested rule service.HideTranslateWindow
	// (broadcast + Hide). The rule absorbs the broadcast the hook inlines today.
	text := mainGoText(t)
	start := strings.Index(text, "translateWindow.RegisterHook(events.Common.WindowClosing")
	if start < 0 {
		t.Fatal("main.go: the translate window's WindowClosing hook is gone")
	}
	// The hook body ends at translateWindow.Center(), registered right after it today.
	end := strings.Index(text[start:], "translateWindow.Center()")
	if end < 0 {
		t.Fatal("main.go: cannot find the end of the translate window's closing hook")
	}
	hook := text[start : start+end]
	if !strings.Contains(hook, "event.Cancel()") {
		t.Errorf("the hook must Cancel the close (otherwise Wails' own WindowClosing listener destroys the window); hook: %s", hook)
	}
	if !strings.Contains(hook, "service.HideTranslateWindow(") {
		t.Errorf("the hook must delegate to service.HideTranslateWindow (the tested broadcast+Hide rule) instead of inlining the close behaviour; hook: %s", hook)
	}
	if strings.Contains(hook, "app.Event.Emit(kevents.EventWindowClosing") {
		t.Errorf("the hook must not also broadcast inline: service.HideTranslateWindow owns the broadcast, and a leftover inline Emit would double-fire the frontend close cleanup (#57/#81/#109); hook: %s", hook)
	}
}

func TestTranslateWindowCmdWBindingCallsTheCloseRule(t *testing.T) {
	// #112 AC3: Cmd+W deterministically hides the window by calling the SAME rule the hook
	// uses — service.HideTranslateWindow directly (broadcast + Hide), never a bare Hide():
	// today's Cmd+W rides the native close chain, which is exactly what broadcasts the close
	// to the frontend; a bare-Hide binding would drop that cleanup on the new path (A Finding 2).
	// The binding's registration and firing on a real app is a manual-verification item (W-1);
	// what this pins is the wiring: the binding exists and its callback is the tested rule.
	text := mainGoText(t)
	start := strings.Index(text, `RegisterKeyBinding("Cmd+W"`)
	if start < 0 {
		t.Fatal(`main.go: the translate window must register the Cmd+W key binding (issue #112)`)
	}
	// The callback is an inline literal a few lines long; 500 collapsed characters is a
	// generous window for it.
	end := start + 500
	if end > len(text) {
		end = len(text)
	}
	binding := text[start:end]
	if !strings.Contains(binding, "service.HideTranslateWindow(") {
		t.Errorf(`the Cmd+W callback must call service.HideTranslateWindow directly (broadcast + Hide, not a bare Hide()); binding site: %s`, binding)
	}
}

func TestTranslateWindowCreationFeedsStartsPinnedIntoAlwaysOnTop(t *testing.T) {
	// #113 AC5: the setting governs the level at creation — main.go passes
	// translateWindowStartsPinned(settingsService) into the window options' AlwaysOnTop.
	block := windowOptionsBlock(t, "WindowTranslate")
	if !strings.Contains(block, "AlwaysOnTop: translateWindowStartsPinned(settingsService)") {
		t.Errorf("the translate window must be created with AlwaysOnTop: translateWindowStartsPinned(settingsService) (issue #113); options block: %s", block)
	}
}
