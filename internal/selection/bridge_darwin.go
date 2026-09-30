//go:build darwin

package selection

import (
	"encoding/json"
	"log/slog"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// TODO(2026-08-11): selectedTextViaBridge is disabled. It was used only by point_darwin.go's
// currentSelectionOSA, which now returns an empty string (see point_darwin.go). Its
// downstream Swift symbol kai_selected_text is likewise commented out. To restore Swift text
// capture later, uncomment this function and restore the cgo declaration and the Swift-side
// @_cdecl.
//
// selectedTextViaBridge reads the foreground app's current selection text via the Swift
// bridge layer (AXUIElement).
// Returns an empty string on failure or no selection; bufSize is the receiving buffer
// capacity.
// func selectedTextViaBridge(bufSize int) string {
// 	if bufSize <= 0 {
// 		bufSize = 4096
// 	}
// 	buf := make([]byte, bufSize)
// 	n := C.kai_selected_text((*C.char)(unsafe.Pointer(&buf[0])), C.int(bufSize))
// 	if n < 0 {
// 		slog.Warn(i18n.T("log.selection_read"))
// 		return ""
// 	}
// 	text := string(buf[:n])
// 	slog.Info(i18n.T("log.selection_read"), slog.Int("length", len([]rune(text))), slog.Bool("hasContent", text != ""))
// 	return text
// }

// accessibilityEnabledViaBridge only queries the accessibility permission state (a liveness
// probe before coordinate positioning); it reads no selection. An answer that cannot be read is
// UNKNOWN and counts as granted (the #211 rule: a bridge problem never blocks a capture that
// works); only a real "false" is missing. A bridge that is not loaded keeps its old answer here
// (false: no positioning without it); AccessibilityGranted decides that case itself, as unknown.
func accessibilityEnabledViaBridge() bool {
	// Degrade safely when the dylib isn’t loaded (no panic).
	if !swiftbridge.Available() {
		slog.Warn(i18n.T("log.swiftbridge_unavailable"))
		return false
	}
	return accessibilityFromQuery(swiftbridge.KaiAccessibilityEnabled)
}

// accessibilityAnswer reads the Swift query through the shared helper. known is false when the
// answer could not be read; granted is only meaningful when known.
func accessibilityAnswer(query func(unsafe.Pointer, int32) int32) (granted, known bool) {
	return swiftbridge.QueryEnabled(query)
}

// accessibilityFromQuery turns the query into the granted/missing decision: unknown counts as
// granted, only a real "false" is missing.
func accessibilityFromQuery(query func(unsafe.Pointer, int32) int32) bool {
	granted, known := accessibilityAnswer(query)
	if !known {
		slog.Warn(i18n.T("log.swiftbridge_unavailable"))
		return true
	}
	slog.Info(i18n.T("log.selection_query"), slog.Bool(i18n.T("log.field_result"), granted))
	return granted
}

// isAccessibilityEnabled checks whether macOS accessibility is granted to the current
// binary (via the Swift bridge).
func isAccessibilityEnabled() bool {
	return accessibilityEnabledViaBridge()
}

// AccessibilityGranted reports whether macOS Accessibility (shown as "Device Control and Data
// Access" on macOS 27) is granted to Kai. A synchronous, prompt-free query (AXIsProcessTrusted); it
// is safe from any goroutine and never opens a dialog. When the bridge itself is not loaded the
// answer is unknown, not "missing": it returns true so a bridge problem never blocks a capture that
// would otherwise have worked (the permission message is for a known-missing grant only).
func AccessibilityGranted() bool {
	return accessibilityGranted(swiftbridge.Available, accessibilityEnabledViaBridge)
}

// accessibilityGranted is AccessibilityGranted with its two bridge calls passed in, so the two
// answers that matter (known missing, and unknown because the bridge is not loaded) can be tested
// without changing this machine's real permission.
func accessibilityGranted(bridgeLoaded, granted func() bool) bool {
	if !bridgeLoaded() {
		return true
	}
	return granted()
}

// selectionPointViaBridge reads the foreground app window anchor via the Swift bridge
// (JSON {x,y}).
func selectionPointViaBridge() (x, y int) {
	if !swiftbridge.Available() {
		return 0, 0
	}
	buf := make([]byte, 128)
	// Call the Swift bridge: unsafe.Pointer is required for the C/Swift interop.
	n := swiftbridge.KaiSelectionPoint(unsafe.Pointer(&buf[0]), int32(len(buf))) //nolint:gosec
	if n <= 0 {
		return 0, 0
	}
	var p swiftbridge.SelectionPoint
	if err := json.Unmarshal(buf[:n], &p); err == nil {
		slog.Info(i18n.T("log.selection_read"), slog.Float64("x", p.X), slog.Float64("y", p.Y))
		return int(p.X), int(p.Y)
	}
	return 0, 0
}

// screenSizeViaBridge reads the primary screen resolution via the Swift bridge (JSON {w,h}).
func screenSizeViaBridge() (w, h float64) {
	if !swiftbridge.Available() {
		return 0, 0
	}
	buf := make([]byte, 128)
	// Call the Swift bridge: unsafe.Pointer is required for the C/Swift interop.
	n := swiftbridge.KaiScreenSize(unsafe.Pointer(&buf[0]), int32(len(buf))) //nolint:gosec
	if n <= 0 {
		return 0, 0
	}
	var s swiftbridge.ScreenSize
	if err := json.Unmarshal(buf[:n], &s); err == nil {
		return s.W, s.H
	}
	return 0, 0
}
