package swiftbridge

import (
	"strings"
	"testing"
)

// Screen Recording: macOS lists an app in Privacy & Security > Screen Recording only after the
// app has asked for the permission (CGRequestScreenCaptureAccess) or tried to capture. Kai's
// "Grant access" used to open the pane and nothing else, so Kai never appeared in the list and the
// user had nothing to switch on. Like swift_source_test.go this is a TEXT check of the Swift
// source (CI has no Swift compiler); the prompt itself needs a person at a Mac.

// requestBody returns kai_screenrecording_request(...) up to the next @_cdecl.
func requestBody(t *testing.T, src string) string {
	t.Helper()
	i := strings.Index(src, "func kai_screenrecording_request(")
	if i < 0 {
		t.Fatal("kai_screenrecording_request not found in apple_accessibility.swift")
	}
	rest := src[i:]
	if j := strings.Index(rest[1:], "@_cdecl"); j >= 0 {
		rest = rest[:j+1]
	}
	return rest
}

func TestScreenRecordingRequestAsksMacOSForThePermission(t *testing.T) {
	body := requestBody(t, readSwift(t, "apple_accessibility.swift"))
	if !strings.Contains(body, "CGRequestScreenCaptureAccess()") {
		t.Error("kai_screenrecording_request must call CGRequestScreenCaptureAccess(): without it macOS never lists Kai under Screen Recording")
	}
}

func TestScreenRecordingRequestStillOpensThePane(t *testing.T) {
	body := requestBody(t, readSwift(t, "apple_accessibility.swift"))
	if !strings.Contains(body, "Privacy_ScreenCapture") || !strings.Contains(body, "NSWorkspace.shared.open") {
		t.Error("kai_screenrecording_request must still open the Screen Recording pane so the user can switch Kai on")
	}
}

func TestScreenRecordingRequestAsksBeforeOpeningThePane(t *testing.T) {
	body := requestBody(t, readSwift(t, "apple_accessibility.swift"))
	ask := strings.Index(body, "CGRequestScreenCaptureAccess()")
	open := strings.Index(body, "NSWorkspace.shared.open")
	if ask < 0 || open < 0 || ask > open {
		t.Error("the permission must be requested before the pane opens, so Kai is already in the list when the pane appears")
	}
}
