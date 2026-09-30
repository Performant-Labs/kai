//go:build darwin

package service

import (
	"log/slog"
	"testing"
	"unsafe"

	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

func fakeAnswer(s string, raw int32) func(unsafe.Pointer, int32) int32 {
	return func(out unsafe.Pointer, outCap int32) int32 {
		if s == "" {
			return raw
		}
		buf := unsafe.Slice((*byte)(out), int(outCap))
		n := copy(buf[:outCap-1], s)
		buf[n] = 0
		return int32(n)
	}
}

// Settings > Shortcuts rows: a real answer is shown as is; an unreadable one (the old no-buffer -1)
// must NOT be shown as "Granted" (that was the bug: every call read as granted).
func TestPermissionFromQuery(t *testing.T) {
	if !permissionFromQuery(fakeAnswer("true", 0)) {
		t.Error("real true must be granted")
	}
	if permissionFromQuery(fakeAnswer("false", 0)) {
		t.Error("real false must be not granted")
	}
	if permissionFromQuery(fakeAnswer("", -1)) {
		t.Error("-1 (no buffer) must not read as granted")
	}
	if permissionFromQuery(nil) {
		t.Error("a nil query must not read as granted")
	}
}

// Both service methods go through the helper against the real dylib and must agree with it.
func TestServicePermissionRows_RealBridge(t *testing.T) {
	if err := swiftbridge.Init(""); err != nil || !swiftbridge.Available() {
		t.Skipf("Swift bridge not loadable here: %v", err)
	}
	s := &AppService{log: slog.Default()}
	for name, c := range map[string]struct {
		got func() bool
		fn  func(unsafe.Pointer, int32) int32
	}{
		"accessibility":    {s.isAccessibilityEnabled, swiftbridge.KaiAccessibilityEnabled},
		"screen recording": {s.isScreenRecordingEnabled, swiftbridge.KaiScreenRecordingEnabled},
	} {
		want, ok := swiftbridge.QueryEnabled(c.fn)
		if !ok {
			t.Fatalf("%s: real query unreadable", name)
		}
		t.Logf("%s: real answer %v", name, want)
		if got := c.got(); got != want {
			t.Errorf("%s: service row says %v, the real query says %v", name, got, want)
		}
	}
}
