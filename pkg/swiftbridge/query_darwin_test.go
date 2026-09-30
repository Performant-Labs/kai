//go:build darwin

package swiftbridge

import (
	"testing"
	"unsafe"
)

// The real dylib, through the helper. Which boolean comes back depends on this machine's grants for
// the test binary, so the value is logged; what is asserted is that the answer is READABLE (ok) and
// that the old signature (no buffer, raw -1 treated as "true") is gone.
func TestQueryEnabled_RealBridge(t *testing.T) {
	if err := Init(""); err != nil || !Available() {
		t.Skipf("Swift bridge not loadable here: %v", err)
	}
	for name, fn := range map[string]func(p unsafe.Pointer, n int32) int32{
		"accessibility":    KaiAccessibilityEnabled,
		"screen recording": KaiScreenRecordingEnabled,
		"input monitoring": KaiInputMonitoringEnabled,
	} {
		enabled, ok := QueryEnabled(fn)
		if !ok {
			t.Errorf("%s: the real query was not readable through the helper", name)
			continue
		}
		t.Logf("real %s query through QueryEnabled: enabled=%v ok=%v", name, enabled, ok)
		// No buffer must never look like an answer: the Swift side returns -1, and the helper never
		// converts that into a boolean.
		if n := fn(nil, 0); n != -1 {
			t.Errorf("%s: a nil buffer returned %d, expected -1 from the Swift side", name, n)
		}
	}
}
