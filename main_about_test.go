package main

import (
	"os"
	"strings"
	"testing"
)

// Issue #8: the menu bar's About item must show the About panel in front of kai's windows.
// Wails' default application menu wires the item to the native orderFrontStandardAboutPanel:
// selector, which orders the panel front only within the normal window level — a pinned
// (always-on-top) translate window stays above it, and no Go code runs at all. The fix reroutes
// the item through the Swift bridge instead. The wiring needs a running app and a menu bar, so
// like TestMainAppliesTheWebViewBackgroundToEveryWindowAndThemeChange this is a guard on the
// text of main.go and main_about.go; the stacking itself can only be judged at a Mac.
func TestMainReroutesTheAboutItemThroughTheBridge(t *testing.T) {
	src := ""
	for _, f := range []string{"main.go", "main_about.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		src += "\n" + string(b)
	}
	flat := strings.Join(strings.Fields(src), " ")
	for _, want := range []string{
		// the default menu is rebuilt with the About item rerouted, and installed before Run
		"installAboutMenu(app, func() {",
		"service.ShowAboutPanelInFront()",
		// the selector is dropped (NoRole has no native selector, so the click reaches OnClick)
		"about.SetRole(application.NoRole)",
		// AppKit is called on the main thread, like every other native window call
		"application.InvokeAsync(func() { if !service.ShowAboutPanelInFront() { app.Menu.ShowAbout() } })",
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("the About wiring lacks %q", want)
		}
	}
}
