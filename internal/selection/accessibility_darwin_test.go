//go:build darwin

package selection

import (
	"testing"
	"unsafe"

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
	real, ok := swiftbridge.QueryEnabled(swiftbridge.KaiAccessibilityEnabled)
	if !ok {
		t.Fatal("the real accessibility query was not readable through the helper")
	}
	if got := AccessibilityGranted(); got != real {
		t.Fatalf("AccessibilityGranted() = %v, but the bridge says %v", got, real)
	}
}

// fakeAnswer stands in for the Swift query: it writes s into the buffer and returns its length, or
// returns raw when s is empty and raw is negative (the no-buffer answer).
func fakeAnswer(s string, raw int32) func(unsafe.Pointer, int32) int32 {
	return func(out unsafe.Pointer, outCap int32) int32 {
		if s == "" {
			return raw
		}
		buf := unsafe.Slice((*byte)(out), int(outCap)) //nolint:gosec // test fake writing into the caller buffer
		n := copy(buf[:outCap-1], s)
		buf[n] = 0
		return int32(n) //nolint:gosec // n < outCap
	}
}

// The regression that hid this bug: the query was called with no buffer, got -1, and "!= 0" made
// every call "granted". A -1 answer must be the UNKNOWN path (known=false), and a real "false" must
// be a real missing grant.
func TestAccessibilityAnswer(t *testing.T) {
	for _, c := range []struct {
		name           string
		fn             func(unsafe.Pointer, int32) int32
		granted, known bool
	}{
		{"real true", fakeAnswer("true", 0), true, true},
		{"real false is missing", fakeAnswer("false", 0), false, true},
		{"-1 (no buffer) is unknown, not granted by accident", fakeAnswer("", -1), false, false},
		{"garbage is unknown", fakeAnswer("maybe", 0), false, false},
		{"nil function is unknown", nil, false, false},
	} {
		granted, known := accessibilityAnswer(c.fn)
		if granted != c.granted || known != c.known {
			t.Errorf("%s: got (granted=%v, known=%v), want (%v, %v)", c.name, granted, known, c.granted, c.known)
		}
	}
}

// Unknown counts as granted (the #211 rule: a bridge problem never blocks a capture that works);
// only a real "false" is missing.
func TestAccessibilityFromAnswer_UnknownCountsAsGranted(t *testing.T) {
	if !accessibilityFromQuery(fakeAnswer("", -1)) {
		t.Error("an unreadable answer must count as granted (unknown), not missing")
	}
	if accessibilityFromQuery(fakeAnswer("false", 0)) {
		t.Error("a real false must be missing")
	}
	if !accessibilityFromQuery(fakeAnswer("true", 0)) {
		t.Error("a real true must be granted")
	}
}
