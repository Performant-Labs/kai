//go:build darwin

package service

import (
	"testing"
	"unsafe"

	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// SetWindowAppearance is the only Go path into the Swift call (issue #8): it must pass the
// window handle and the theme through (dark as 1, light as 0), read the 1 from Swift as
// success, and treat a nil handle or a bridge that is not loaded as a no-op. These fail if the
// call is removed or changed.
func TestSetWindowAppearanceCallsTheBridge(t *testing.T) {
	prev := swiftbridge.KaiWindowSetAppearance
	t.Cleanup(func() { swiftbridge.KaiWindowSetAppearance = prev })

	x := 1
	ptr := unsafe.Pointer(&x) //nolint:gosec // a pointer to a local, only converted and compared
	var gotWin uintptr
	var gotDark int32
	swiftbridge.KaiWindowSetAppearance = func(window uintptr, dark int32) int32 {
		gotWin, gotDark = window, dark
		return 1
	}
	if !SetWindowAppearance(ptr, true) {
		t.Fatal("a 1 from Swift means the appearance was updated and must read as success")
	}
	if gotWin != uintptr(ptr) || gotDark != 1 {
		t.Errorf("dark: the bridge got window=%v dark=%d, want window=%v dark=1", gotWin, gotDark, uintptr(ptr))
	}
	if !SetWindowAppearance(ptr, false) {
		t.Fatal("a 1 from Swift means the appearance was updated and must read as success")
	}
	if gotDark != 0 {
		t.Errorf("light: the bridge got dark=%d, want 0", gotDark)
	}

	swiftbridge.KaiWindowSetAppearance = func(uintptr, int32) int32 { return 0 }
	if SetWindowAppearance(ptr, true) {
		t.Error("a 0 from Swift (no window found) must read as failure")
	}
}

func TestSetWindowAppearanceIsANoopWithoutAHandleOrBridge(t *testing.T) {
	prev := swiftbridge.KaiWindowSetAppearance
	t.Cleanup(func() { swiftbridge.KaiWindowSetAppearance = prev })

	called := false
	swiftbridge.KaiWindowSetAppearance = func(uintptr, int32) int32 { called = true; return 1 }
	if SetWindowAppearance(nil, true) || called {
		t.Error("a nil native handle must not reach the bridge")
	}
	x := 1
	ptr := unsafe.Pointer(&x) //nolint:gosec // a pointer to a local, only passed along
	swiftbridge.KaiWindowSetAppearance = nil
	if SetWindowAppearance(ptr, true) {
		t.Error("a bridge that is not loaded (nil function) must be a no-op")
	}
}
