//go:build !darwin

package selection

// AccessibilityGranted: only macOS has an Accessibility permission for simulated keys, so
// everywhere else there is nothing to be missing and the answer is always true.
func AccessibilityGranted() bool {
	return true
}
