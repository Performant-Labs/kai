package swiftbridge

import (
	"os"
	"strings"
	"testing"
)

// Issue #8: the menu bar's About item must open the About panel in front of kai's windows. The
// native orderFrontStandardAboutPanel: selector (Wails' default app menu) orders the panel front
// only within the normal window level, so a pinned (always-on-top, floating-level) translate
// window stays above it. apple_about.swift shows the panel, lifts it to the floating level and
// makes it key. TEXT check like swift_source_test.go: CI has no Swift compiler; the actual
// stacking can only be judged by a person at a Mac.
func TestShowAboutPanelRaisesTheStandardPanel(t *testing.T) {
	src := readSwift(t, "apple_about.swift")
	for _, want := range []string{
		`@_cdecl("kai_show_about_panel")`,
		"NSApp.activate",
		"orderFrontStandardAboutPanel",
		".floating",
		"makeKeyAndOrderFront",
		"AboutPanel",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("apple_about.swift must contain %q", want)
		}
	}
}

func TestShowAboutPanelIsRegisteredInTheLoader(t *testing.T) {
	for _, f := range []string{"load.go", "load_other.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "KaiShowAboutPanel") {
			t.Errorf("%s must declare KaiShowAboutPanel", f)
		}
	}
	b, _ := os.ReadFile("load.go")
	if !strings.Contains(string(b), `"kai_show_about_panel"`) {
		t.Error("load.go must register kai_show_about_panel")
	}
}
