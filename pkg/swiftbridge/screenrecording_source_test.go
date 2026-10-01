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

// Input Monitoring: the Settings page polls kai_input_monitoring_enabled every 3 seconds (issue
// #14). The query must never create an event tap, because creating a keyboard tap can make macOS
// ask the user for the permission, and a tap created on every poll also leaked a retained object.
// CGPreflightListenEventAccess() answers without a prompt and without creating anything.
func inputMonitoringBody(t *testing.T, src string) string {
	t.Helper()
	i := strings.Index(src, "func kai_input_monitoring_enabled(")
	if i < 0 {
		t.Fatal("kai_input_monitoring_enabled not found in apple_accessibility.swift")
	}
	rest := src[i:]
	if j := strings.Index(rest[1:], "@_cdecl"); j >= 0 {
		rest = rest[:j+1]
	}
	return rest
}

func TestInputMonitoringQueryNeverPrompts(t *testing.T) {
	body := inputMonitoringBody(t, readSwift(t, "apple_accessibility.swift"))
	if !strings.Contains(body, "CGPreflightListenEventAccess()") {
		t.Error("kai_input_monitoring_enabled must use CGPreflightListenEventAccess(), which never prompts")
	}
	for _, banned := range []string{"tapCreate", "CGEvent.tap", "passRetained"} {
		if strings.Contains(body, banned) {
			t.Errorf("kai_input_monitoring_enabled must not use %s: creating an event tap can prompt the user and it is polled every 3 s", banned)
		}
	}
}

// Issue #23: the Grant buttons must reliably put Kai in the list of the matching Privacy pane.

// swiftFuncBody returns the named function up to the next @_cdecl (or the end of the file).
func swiftFuncBody(t *testing.T, name string) string {
	t.Helper()
	src := readSwift(t, "apple_accessibility.swift")
	i := strings.Index(src, "func "+name+"(")
	if i < 0 {
		t.Fatalf("%s not found in apple_accessibility.swift", name)
	}
	rest := src[i:]
	if j := strings.Index(rest[1:], "@_cdecl"); j >= 0 {
		rest = rest[:j+1]
	}
	return rest
}

// Input Monitoring: CGRequestListenEventAccess() is what lists Kai under Input Monitoring. It must
// never be done by creating an event tap (that can itself prompt, and leaked when polled), and it
// must only be the button's job: the 3-second poll uses the Preflight query, which stays read-only.
func TestInputMonitoringRequestRegistersKaiWithoutATap(t *testing.T) {
	body := swiftFuncBody(t, "kai_input_monitoring_request")
	if !strings.Contains(body, "CGRequestListenEventAccess()") {
		t.Error("kai_input_monitoring_request must call CGRequestListenEventAccess(): it is what lists Kai under Input Monitoring")
	}
	for _, banned := range []string{"tapCreate", "CGEvent.tap", "passRetained"} {
		if strings.Contains(body, banned) {
			t.Errorf("kai_input_monitoring_request must not use %s: it must never create an event tap", banned)
		}
	}
}

func TestInputMonitoringRequestOpensThePaneAfterAsking(t *testing.T) {
	body := swiftFuncBody(t, "kai_input_monitoring_request")
	ask := strings.Index(body, "CGRequestListenEventAccess()")
	open := strings.Index(body, "NSWorkspace.shared.open")
	if !strings.Contains(body, "Privacy_ListenEvent") || ask < 0 || open < 0 || ask > open {
		t.Error("the Input Monitoring pane (Privacy_ListenEvent) must open after the request, so Kai is already listed")
	}
}

func TestInputMonitoringPollNeverRequests(t *testing.T) {
	body := swiftFuncBody(t, "kai_input_monitoring_enabled")
	if strings.Contains(body, "CGRequestListenEventAccess") {
		t.Error("the 3-second query must not request the permission; only the Grant button does")
	}
}

// Screen Recording: the request alone is not reliable, so it runs on the main thread with Kai
// frontmost, and only when the preflight still says "not granted" does it make one real (metadata
// only, no pixels, so no flash) ScreenCaptureKit call so macOS lists Kai. The pane opens last.
func TestScreenRecordingRequestRunsOnMainThreadWithKaiActive(t *testing.T) {
	body := requestBody(t, readSwift(t, "apple_accessibility.swift"))
	if !strings.Contains(body, "Thread.isMainThread") || !strings.Contains(body, "DispatchQueue.main") {
		t.Error("the request must run on the main thread")
	}
	if !strings.Contains(body, "NSApp.activate") {
		t.Error("Kai must be the active app when it asks, or macOS may not register the request")
	}
}

func TestScreenRecordingRequestEscalatesToCaptureOnlyWhenNotGranted(t *testing.T) {
	body := requestBody(t, readSwift(t, "apple_accessibility.swift"))
	ask := strings.Index(body, "CGRequestScreenCaptureAccess()")
	pre := strings.Index(body, "CGPreflightScreenCaptureAccess()")
	capture := strings.Index(body, "SCShareableContent")
	// The pane is opened by the openPane closure; it must be CALLED after the capture attempt.
	open := strings.LastIndex(body, "openPane()")
	if !strings.Contains(body, "NSWorkspace.shared.open") {
		t.Fatal("the pane must still be opened")
	}
	if ask < 0 || pre < 0 || capture < 0 || open < 0 {
		t.Fatalf("request, preflight, capture attempt and pane open must all be present (ask=%d pre=%d capture=%d open=%d)", ask, pre, capture, open)
	}
	if ask >= pre || pre >= capture || capture >= open {
		t.Error("order must be: request, preflight, capture attempt (only if not granted), open the pane")
	}
	// The capture attempt sits inside an `if !CGPreflightScreenCaptureAccess()` guard.
	if !strings.Contains(body, "if !CGPreflightScreenCaptureAccess()") {
		t.Error("the capture attempt must be guarded by `if !CGPreflightScreenCaptureAccess()`")
	}
}
