package swiftbridge

import (
	"os"
	"strings"
	"testing"
)

// Issue #199: the double-Cmd+C entry points. The event tap must be listen-only and run on its own
// thread's run loop, must never touch the main thread, and must never keep any key but Cmd+C.
func TestDoubleCopyEntryPoints(t *testing.T) {
	src := readSwift(t, "apple_doublecopy.swift")
	for _, sym := range []string{"kai_doublecopy_start", "kai_doublecopy_stop", "kai_doublecopy_poll",
		"kai_doublecopy_suppress", "kai_doublecopy_request", "kai_doublecopy_pasteboard", "kai_doublecopy_ingest"} {
		if !strings.Contains(src, `@_cdecl("`+sym+`")`) {
			t.Errorf("apple_doublecopy.swift does not declare @_cdecl(%q)", sym)
		}
	}
	if !strings.Contains(src, ".listenOnly") {
		t.Error("the event tap is not created with .listenOnly: it must never be able to swallow or alter a key")
	}
	if strings.Contains(src, ".defaultTap") {
		t.Error("apple_doublecopy.swift creates a .defaultTap (active) event tap")
	}
	for _, banned := range []string{"DispatchQueue.main", "MainActor", "NSApp."} {
		if strings.Contains(src, banned) {
			t.Errorf("apple_doublecopy.swift uses %q: the tap and its queue must not depend on the main thread", banned)
		}
	}
	if !strings.Contains(src, "CGPreflightListenEventAccess") {
		t.Error("kai_doublecopy_start must preflight the permission so that starting never pops the system prompt")
	}
	load, err := os.ReadFile("load.go")
	if err != nil {
		t.Fatal(err)
	}
	other, err := os.ReadFile("load_other.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"KaiDoubleCopyStart", "KaiDoubleCopyStop", "KaiDoubleCopyPoll",
		"KaiDoubleCopySuppress", "KaiDoubleCopyRequest", "KaiDoubleCopyPasteboard", "KaiDoubleCopyIngest"} {
		if !strings.Contains(string(load), name) {
			t.Errorf("load.go has no %s", name)
		}
		if !strings.Contains(string(other), name) {
			t.Errorf("load_other.go has no %s stub", name)
		}
	}
}
