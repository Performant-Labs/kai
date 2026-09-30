package main

import (
	"os"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Issue #15: the native window background must match the app's own background (app.css
// --app-bg: #ffffff light, #18181c dark), and be opaque, so no light frame shows before the page draws.
func TestWindowBackground(t *testing.T) {
	if got, want := windowBackground(false), application.NewRGBA(0xff, 0xff, 0xff, 0xff); got != want {
		t.Errorf("light = %+v, want %+v", got, want)
	}
	if got, want := windowBackground(true), application.NewRGBA(0x18, 0x18, 0x1c, 0xff); got != want {
		t.Errorf("dark = %+v, want %+v", got, want)
	}
}

// Issue #22: the web view background must be applied to all three windows when each becomes ready
// (WindowRuntimeReady, on the main thread through InvokeAsync), and every theme change must recolour
// them, otherwise a hidden window shown again flashes the web view's own white. main.go is wiring that
// needs a running app, so this is a guard on its text.
func TestMainAppliesTheWebViewBackgroundToEveryWindowAndOnThemeChange(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, w := range []string{"settingsWindow", "translateWindow", "screenshotWindow"} {
		if !strings.Contains(src, "applyWindowBackground(windowSvc, "+w+", currentDark(), false)") {
			t.Errorf("main.go must call applyWindowBackground for %s when it becomes ready", w)
		}
	}
	if !strings.Contains(src, "app.Event.On(kevents.EventThemeChanged") || !strings.Contains(src, "events.Common.ThemeChanged, func(*application.ApplicationEvent) { recolourWindows() }") {
		t.Error("main.go must recolour the windows on the theme setting event and on the system appearance event")
	}
	if !strings.Contains(src, "applyWindowBackground(windowSvc, w, dark, true)") {
		t.Error("a theme change must also recolour the window itself")
	}
}

// Issue #22: the launch show (#17) must wait for the translate window's web view to be prepared, or the
// window is drawn with the web view's own white background at launch (the ready event can fire after
// the show). The ready handler closes translateReady after applying the background, and the startup
// handler waits for it, with a timeout so a missing event can never leave the user without a window.
func TestStartupShowWaitsForTheTranslateWindowToBePrepared(t *testing.T) {
	b, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	src := strings.Join(strings.Fields(string(b)), " ")
	for _, want := range []string{
		"translateReady := make(chan struct{})",
		"translateReadyOnce.Do(func() { close(translateReady) })",
		"case <-translateReady:",
		"case <-time.After(2 * time.Second):",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("main.go lacks %q", want)
		}
	}
	ready := strings.Index(src, "applyWindowBackground(windowSvc, translateWindow, currentDark(), false) translateReadyOnce.Do")
	if ready < 0 {
		t.Error("the translate ready handler must close translateReady right after applying the background")
	}
}
