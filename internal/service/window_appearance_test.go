package service

import (
	"testing"
	"unsafe"
)

// Issue #8: the title bar follows the theme. applyWindowAppearance is the cross-platform wiring
// seam (like applyWebViewBackground, issue #22): it hands the window's native handle and the
// resolved theme to the platform setter. These tests lock the wiring; the NSAppearance effect
// itself is darwin-only and not verifiable in go test (see window_appearance_darwin_test.go).
func TestApplyWindowAppearancePassesHandleAndTheme(t *testing.T) {
	x := 7
	ptr := unsafe.Pointer(&x) //nolint:gosec // a pointer to a local, only converted and compared
	var gotPtr unsafe.Pointer
	var gotDark bool
	ok := applyWindowAppearance(fakeBgWindow{ptr: ptr}, true, func(p unsafe.Pointer, dark bool) bool {
		gotPtr, gotDark = p, dark
		return true
	})
	if !ok || gotPtr != ptr || !gotDark {
		t.Fatalf("dark: got ok=%v ptr=%v dark=%v, want ok=true ptr=%v dark=true", ok, gotPtr, gotDark, ptr)
	}
	ok = applyWindowAppearance(fakeBgWindow{ptr: ptr}, false, func(_ unsafe.Pointer, dark bool) bool {
		gotDark = dark
		return true
	})
	if !ok || gotDark {
		t.Fatalf("light: got ok=%v dark=%v, want ok=true dark=false", ok, gotDark)
	}
}

func TestApplyWindowAppearanceNoopsWithoutAWindowOrHandleOrCallback(t *testing.T) {
	called := false
	set := func(unsafe.Pointer, bool) bool { called = true; return true }
	if applyWindowAppearance(nil, true, set) || called {
		t.Error("a nil window must be a no-op")
	}
	if applyWindowAppearance(fakeBgWindow{}, true, set) || called {
		t.Error("a nil native handle (non-darwin) must be a no-op")
	}
	x := 1
	ptr := unsafe.Pointer(&x) //nolint:gosec // a pointer to a local, only passed along
	if applyWindowAppearance(fakeBgWindow{ptr: ptr}, true, nil) {
		t.Error("a nil callback must be a no-op")
	}
}
