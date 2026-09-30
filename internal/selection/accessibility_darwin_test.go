//go:build darwin

package selection

import (
	"testing"

	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// Issue #194: the real bridge query behind AccessibilityGranted. It must answer, without a prompt
// and without hanging, from a plain test binary once the real dylib is loaded. Which answer it
// gives depends on the machine's grant for this binary, so the value is logged, not asserted.
func TestAccessibilityGranted_RealBridgeAnswersWithoutPrompting(t *testing.T) {
	if err := swiftbridge.Init(""); err != nil || !swiftbridge.Available() {
		t.Skipf("Swift bridge not loadable here: %v", err)
	}
	got := AccessibilityGranted()
	t.Logf("real kai_accessibility_enabled for this test binary: %v", got)
}
