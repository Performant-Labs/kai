//go:build darwin

package selection

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// currentSelectionPoint gets the foreground app window anchor via the Swift bridge (AX)
// for floating-window positioning.
func currentSelectionPoint() *application.Point {
	if !isAccessibilityEnabled() {
		return nil
	}
	x, y := selectionPointViaBridge()
	if x == 0 && y == 0 {
		return nil
	}
	return &application.Point{X: x, Y: y}
}

// primaryScreenSize returns the primary screen resolution (via the Swift bridge).
func primaryScreenSize() (float64, float64) {
	return screenSizeViaBridge()
}

// TODO(2026-08-11): currentSelectionOSA has Swift text capture disabled. The original
// implementation (selectedTextViaBridge → Swift kai_selected_text) was commented out after
// users reported machine issues. It now always returns an empty string, so currentSelection()
// on darwin naturally falls back to the clipboard (aligned with the input-translate
// "copy key first" logic).
// To restore: uncomment the selectedTextViaBridge call below and make sure bridge_darwin.go
// and the Swift side are restored.
//
// currentSelectionOSA reads the current app's selection text via the Swift bridge layer
// (AXUIElement) on macOS.
// No longer depends on AppleScript / System Events.
//
//	func currentSelectionOSA() string {
//		return selectedTextViaBridge(nil, 0)
//	}
func currentSelectionOSA() string {
	return ""
}
