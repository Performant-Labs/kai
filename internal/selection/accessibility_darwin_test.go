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

// Issue #194: the decision inside AccessibilityGranted, with the bridge calls passed in. A version
// that always answered true (the bridge query removed) fails the first case; one that treated an
// unloaded bridge as "missing" fails the second, which would block captures that work today.
func TestAccessibilityGrantedDecision(t *testing.T) {
	yes := func() bool { return true }
	no := func() bool { return false }
	for _, c := range []struct {
		name          string
		loaded, asked func() bool
		want          bool
	}{
		{"bridge loaded, permission missing is missing", yes, no, false},
		{"bridge loaded, permission present is granted", yes, yes, true},
		{"bridge not loaded is unknown, not missing", no, no, true},
	} {
		if got := accessibilityGranted(c.loaded, c.asked); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
	called := false
	if !accessibilityGranted(no, func() bool { called = true; return false }) || called {
		t.Error("an unloaded bridge must not be asked about the permission")
	}
}

// The real query must agree with the decision function fed the real bridge, and must not prompt.
func TestAccessibilityGranted_RealBridgeMatchesTheDecision(t *testing.T) {
	if err := swiftbridge.Init(""); err != nil || !swiftbridge.Available() {
		t.Skipf("Swift bridge not loadable here: %v", err)
	}
	real := swiftbridge.KaiAccessibilityEnabled() != 0
	if got := AccessibilityGranted(); got != real {
		t.Fatalf("AccessibilityGranted() = %v, but the bridge says %v", got, real)
	}
}
