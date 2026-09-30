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
		buf := unsafe.Slice((*byte)(out), int(outCap)) //nolint:gosec // test fake writing into the caller buffer
		n := copy(buf[:outCap-1], s)
		buf[n] = 0
		return int32(n) //nolint:gosec // n < outCap
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
		"input monitoring": {s.isInputMonitoringEnabled, swiftbridge.KaiInputMonitoringEnabled},
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

// Issue #14: Input Monitoring gets a real check (the Settings page polls it). Like the other two it
// must never read as granted when the answer cannot be read: a bridge that is not loaded, a -1 (no
// buffer) or a nil query all give "not granted".
func TestInputMonitoringFromQuery(t *testing.T) {
	if !inputMonitoringFromQuery(true, fakeAnswer("true", 0)) {
		t.Error("real true must be granted")
	}
	if inputMonitoringFromQuery(true, fakeAnswer("false", 0)) {
		t.Error("real false must be not granted")
	}
	if inputMonitoringFromQuery(true, fakeAnswer("", -1)) {
		t.Error("-1 (no buffer) must not read as granted")
	}
	if inputMonitoringFromQuery(true, nil) {
		t.Error("a nil query must not read as granted")
	}
	if inputMonitoringFromQuery(false, fakeAnswer("true", 0)) {
		t.Error("a bridge that is not loaded must not read as granted, whatever the query says")
	}
}

// The exported binding the frontend calls is a thin wrapper over the same check.
func TestCheckInputMonitoringBinding(t *testing.T) {
	s := &AppService{log: slog.Default()}
	if err := swiftbridge.Init(""); err != nil || !swiftbridge.Available() {
		if s.CheckInputMonitoring() {
			t.Error("with the bridge not loaded the binding must say not granted")
		}
		return
	}
	if got, want := s.CheckInputMonitoring(), s.isInputMonitoringEnabled(); got != want {
		t.Errorf("binding says %v, the service check says %v", got, want)
	}
}
