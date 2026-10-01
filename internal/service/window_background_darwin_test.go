//go:build darwin

package service

import (
	"testing"
	"unsafe"

	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// SetWindowWebViewBackground is the only Go path into the Swift call (issue #22): it must pass the
// window handle and the colour through, read the 1 from Swift as success, and treat a nil handle or a
// bridge that is not loaded as a no-op. These fail if the call is removed or changed.
func TestSetWindowWebViewBackgroundCallsTheBridge(t *testing.T) {
	prev := swiftbridge.KaiWindowSetWebviewBackground
	t.Cleanup(func() { swiftbridge.KaiWindowSetWebviewBackground = prev })

	x := 1
	ptr := unsafe.Pointer(&x) //nolint:gosec // a pointer to a local, only converted and compared
	var gotWin uintptr
	var gotR, gotG, gotB int32
	swiftbridge.KaiWindowSetWebviewBackground = func(window uintptr, r, g, b int32) int32 {
		gotWin, gotR, gotG, gotB = window, r, g, b
		return 1
	}
	if !SetWindowWebViewBackground(ptr, 0x18, 0x18, 0x1c) {
		t.Fatal("a 1 from Swift means the web view was updated and must read as success")
	}
	if gotWin != uintptr(ptr) || gotR != 0x18 || gotG != 0x18 || gotB != 0x1c {
		t.Errorf("the bridge got window=%v rgb=%d,%d,%d", gotWin, gotR, gotG, gotB)
	}

	swiftbridge.KaiWindowSetWebviewBackground = func(uintptr, int32, int32, int32) int32 { return 0 }
	if SetWindowWebViewBackground(ptr, 1, 2, 3) {
		t.Error("a 0 from Swift (no web view found) must read as failure")
	}
}

func TestSetWindowWebViewBackgroundIsANoopWithoutAHandleOrBridge(t *testing.T) {
	prev := swiftbridge.KaiWindowSetWebviewBackground
	t.Cleanup(func() { swiftbridge.KaiWindowSetWebviewBackground = prev })

	called := false
	swiftbridge.KaiWindowSetWebviewBackground = func(uintptr, int32, int32, int32) int32 { called = true; return 1 }
	if SetWindowWebViewBackground(nil, 1, 2, 3) || called {
		t.Error("a nil native handle must not reach the bridge")
	}
	x := 1
	ptr := unsafe.Pointer(&x) //nolint:gosec // a pointer to a local, only passed along
	swiftbridge.KaiWindowSetWebviewBackground = nil
	if SetWindowWebViewBackground(ptr, 1, 2, 3) {
		t.Error("a bridge that is not loaded (nil function) must be a no-op")
	}
}
