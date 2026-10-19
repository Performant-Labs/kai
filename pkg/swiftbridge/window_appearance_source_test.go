package swiftbridge

import (
	"os"
	"strings"
	"testing"
)

// Issue #8: Wails sets a window's NSAppearance only at creation, from the theme resolved at
// launch, so a theme change after launch left the title bar light while the content went dark.
// apple_window.swift adds kai_window_set_appearance so the Go side can follow theme changes.
// Like window_background_source_test.go this is a TEXT check (CI has no Swift compiler); the
// title bar itself can only be judged by a person at a Mac.
func TestWindowAppearanceSetsTheNSWindowAppearance(t *testing.T) {
	src := readSwift(t, "apple_window.swift")
	for _, want := range []string{
		`@_cdecl("kai_window_set_appearance")`,
		"NSAppearance",
		"darkAqua",
		"aqua",
		"nsWindow.appearance",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("apple_window.swift must contain %q", want)
		}
	}
}

func TestWindowAppearanceIsRegisteredInTheLoader(t *testing.T) {
	for _, f := range []string{"load.go", "load_other.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "KaiWindowSetAppearance") {
			t.Errorf("%s must declare KaiWindowSetAppearance", f)
		}
	}
	b, _ := os.ReadFile("load.go")
	if !strings.Contains(string(b), `"kai_window_set_appearance"`) {
		t.Error("load.go must register kai_window_set_appearance")
	}
}
