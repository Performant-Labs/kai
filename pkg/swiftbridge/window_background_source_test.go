package swiftbridge

import (
	"os"
	"strings"
	"testing"
)

// Issue #22: Wails sets only the NSWindow's background; the WKWebView inside draws its own white
// whenever a hidden window is shown again. apple_window.swift makes the web view draw no background
// of its own. Like swift_source_test.go this is a TEXT check (CI has no Swift compiler); the flash
// itself can only be judged by a person at a Mac.
func TestWindowBackgroundMakesTheWebViewDrawNoBackground(t *testing.T) {
	src := readSwift(t, "apple_window.swift")
	for _, want := range []string{
		`@_cdecl("kai_window_set_webview_background")`,
		`setValue(false, forKey: "drawsBackground")`,
		"underPageBackgroundColor",
		"nsWindow.backgroundColor",
		"WKWebView",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("apple_window.swift must contain %q", want)
		}
	}
}

func TestWindowBackgroundIsRegisteredInTheLoader(t *testing.T) {
	for _, f := range []string{"load.go", "load_other.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "KaiWindowSetWebviewBackground") {
			t.Errorf("%s must declare KaiWindowSetWebviewBackground", f)
		}
	}
	b, _ := os.ReadFile("load.go")
	if !strings.Contains(string(b), `"kai_window_set_webview_background"`) {
		t.Error("load.go must register kai_window_set_webview_background")
	}
}
