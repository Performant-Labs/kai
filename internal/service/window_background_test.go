package service

import (
	"testing"
	"unsafe"
)

type fakeBgWindow struct{ ptr unsafe.Pointer }

func (f fakeBgWindow) NativeWindow() unsafe.Pointer { return f.ptr }

func TestApplyWebViewBackgroundPassesHandleAndColour(t *testing.T) {
	x := 7
	ptr := unsafe.Pointer(&x) //nolint:gosec // a pointer to a local, only compared, never dereferenced
	var gotPtr unsafe.Pointer
	var gotR, gotG, gotB int32
	ok := applyWebViewBackground(fakeBgWindow{ptr: ptr}, 0x18, 0x18, 0x1c, func(p unsafe.Pointer, r, g, b int32) bool {
		gotPtr, gotR, gotG, gotB = p, r, g, b
		return true
	})
	if !ok || gotPtr != ptr || gotR != 0x18 || gotG != 0x18 || gotB != 0x1c {
		t.Fatalf("got ok=%v ptr=%v rgb=%d,%d,%d", ok, gotPtr, gotR, gotG, gotB)
	}
}

func TestApplyWebViewBackgroundNoopsWithoutAWindowOrHandleOrCallback(t *testing.T) {
	called := false
	set := func(unsafe.Pointer, int32, int32, int32) bool { called = true; return true }
	if applyWebViewBackground(nil, 1, 2, 3, set) || called {
		t.Error("a nil window must be a no-op")
	}
	if applyWebViewBackground(fakeBgWindow{}, 1, 2, 3, set) || called {
		t.Error("a nil native handle (non-darwin) must be a no-op")
	}
	x := 1
	ptr := unsafe.Pointer(&x) //nolint:gosec // a pointer to a local, only passed along
	if applyWebViewBackground(fakeBgWindow{ptr: ptr}, 1, 2, 3, nil) {
		t.Error("a nil callback must be a no-op")
	}
}
