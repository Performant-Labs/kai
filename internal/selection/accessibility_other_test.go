//go:build !darwin

package selection

import "testing"

// Issue #194: only macOS has an Accessibility permission for simulated keys; on Windows and Linux
// the copy key must never be blocked by it.
func TestAccessibilityGranted_IsAlwaysTrueOffMacOS(t *testing.T) {
	if !AccessibilityGranted() {
		t.Fatal("AccessibilityGranted() = false on a platform with no such permission")
	}
}
