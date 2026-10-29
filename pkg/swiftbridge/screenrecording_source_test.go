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
// frontmost, and only when the preflight still says "not granted" does it start one real
// ScreenCaptureKit capture attempt — a started SCStream, stopped as soon as it answers; the
// expected .notPermitted failure is what makes macOS list Kai (issue #114: the metadata-only
// enumeration shipped in 0.6.0 never enrolled Kai). The pane opens last, on the attempt's
// answer (the 3 s deadline is only the backstop).
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
	// Issue #114 (handoff-A A1/R1): the escalation must be a REAL capture attempt — an SCStream
	// that is started and, once it answers, stopped. Metadata-only SCShareableContent enumeration
	// (the 0.6.0 shape) does not make macOS list Kai. SCShareableContent itself is deliberately
	// NOT banned here: it legitimately stays to build the stream's SCContentFilter (handoff-A R2),
	// so the pin requires the stream start instead of asserting the metadata call's absence.
	if !strings.Contains(body, "SCStream") {
		t.Error("the escalation must construct an SCStream: a metadata-only SCShareableContent enumeration does not enroll Kai with macOS")
	}
	if !strings.Contains(body, "startCapture") {
		t.Error("the escalation must start the stream (startCapture): constructing an SCStream without starting it captures nothing")
	}
	if !strings.Contains(body, "stopCapture") {
		t.Error("the stream must be stopped once it answers (stopCapture): no frames are kept")
	}
	if !strings.Contains(body, "NSWorkspace.shared.open") {
		t.Fatal("the pane must still be opened")
	}
	ask := strings.Index(body, "CGRequestScreenCaptureAccess()")
	pre := strings.Index(body, "CGPreflightScreenCaptureAccess()")
	attempt := strings.Index(body, "startCapture")
	// The pane is opened by the openPane closure; it must be CALLED after the capture attempt.
	open := strings.LastIndex(body, "openPane()")
	if ask < 0 || pre < 0 || attempt < 0 || open < 0 {
		t.Fatalf("request, preflight, stream attempt and pane open must all be present (ask=%d pre=%d attempt=%d open=%d)", ask, pre, attempt, open)
	}
	if ask >= pre || pre >= attempt || attempt >= open {
		t.Error("order must be: request, preflight, capture attempt (only if not granted), open the pane")
	}
	// The capture attempt sits inside an `if !CGPreflightScreenCaptureAccess()` guard.
	if !strings.Contains(body, "if !CGPreflightScreenCaptureAccess()") {
		t.Error("the capture attempt must be guarded by `if !CGPreflightScreenCaptureAccess()`")
	}
}

// Issue #114 (handoff-A R3): the pane must open on the attempt's answer — the expected
// .notPermitted error from startCapture IS the answer (that failure is what enrolls Kai), so on
// the not-granted path the pane must not ride the 3 s deadline backstop alone. Source-text
// approximation (CI has no Swift compiler): an openPane() call must sit in the source between
// startCapture and the deadline backstop, i.e. inside the attempt's completion region.
func TestScreenRecordingRequestOpensPaneOnTheStreamAnswer(t *testing.T) {
	body := requestBody(t, readSwift(t, "apple_accessibility.swift"))
	attempt := strings.Index(body, "startCapture")
	if attempt < 0 {
		t.Fatal("no startCapture in kai_screenrecording_request: the real capture attempt is missing")
	}
	deadline := strings.Index(body, "asyncAfter")
	if deadline < 0 {
		t.Fatal("the 3 s deadline backstop (DispatchQueue.main.asyncAfter) must stay: a slow answer must never leave the user with no pane")
	}
	pane := strings.Index(body[attempt:], "openPane()")
	if pane < 0 || attempt+pane >= deadline {
		t.Error("an openPane() call must be reachable from the startCapture completion (between startCapture and the 3 s deadline backstop): the pane opens on the attempt's answer, not only on the deadline")
	}
}
