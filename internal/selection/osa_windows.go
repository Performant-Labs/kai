//go:build windows

package selection

// isAccessibilityEnabled: UI Automation on Windows needs no extra permission toggle,
// always true.
func isAccessibilityEnabled() bool {
	return true
}
