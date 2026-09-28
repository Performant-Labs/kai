package service

import (
	"reflect"
	"testing"
	"unsafe"
)

// Issue #163: right after a fresh relaunch, the screenshot translate window (and, latently,
// settings/translate too) could reappear visible even though main.go calls Hide() on it at
// creation time. Root cause: macOS's Secure State Restoration ("Resume") replays each
// restorable NSWindow's saved visibility during AppKit's own launch sequence, independent of
// and later than our own Hide() call. The fix opts each window out of restoration by sending
// NSWindow.setRestorable:NO via the window's native handle. disableRestoration is the
// cross-platform wiring seam (package-level, no AppKit/purego dependency) that the darwin-only
// SetWindowNotRestorable implementation plugs into; these tests lock the wiring only, not the
// actual AppKit effect (not mechanically verifiable in go test, per the brief).

// fakeNativeWindow implements nativeWindowHandle with a fixed, caller-supplied pointer so
// disableRestoration can be exercised without a real Wails/AppKit window.
type fakeNativeWindow struct {
	ptr unsafe.Pointer
}

func (f *fakeNativeWindow) NativeWindow() unsafe.Pointer { return f.ptr }

func TestDisableRestorationCallsSetNotRestorableWithNativeHandle(t *testing.T) {
	var handle byte
	win := &fakeNativeWindow{ptr: unsafe.Pointer(&handle)}

	calls := 0
	var got unsafe.Pointer
	cb := func(p unsafe.Pointer) {
		calls++
		got = p
	}

	disableRestoration(win, cb)

	if calls != 1 {
		t.Errorf("setNotRestorable called %d times, want exactly 1", calls)
	}
	if got != unsafe.Pointer(&handle) {
		t.Errorf("setNotRestorable called with %v, want %v", got, unsafe.Pointer(&handle))
	}
}

func TestDisableRestorationNoopsOnNilWindow(t *testing.T) {
	calls := 0
	cb := func(unsafe.Pointer) { calls++ }

	disableRestoration(nil, cb) // must not panic

	if calls != 0 {
		t.Errorf("setNotRestorable called %d times for a nil window, want 0", calls)
	}
}

func TestDisableRestorationNoopsOnNilNativeHandle(t *testing.T) {
	win := &fakeNativeWindow{ptr: nil} // e.g. the non-darwin no-op stub case
	calls := 0
	cb := func(unsafe.Pointer) { calls++ }

	disableRestoration(win, cb)

	if calls != 0 {
		t.Errorf("setNotRestorable called %d times for a nil native handle, want 0", calls)
	}
}

func TestDisableRestorationNoopsOnNilCallback(t *testing.T) {
	var handle byte
	win := &fakeNativeWindow{ptr: unsafe.Pointer(&handle)}

	disableRestoration(win, nil) // must not panic
}

// TestWindowWrapperDisableRestorationDefaultsToSetWindowNotRestorable confirms
// WindowWrapper.DisableRestoration is wired to the real SetWindowNotRestorable symbol by
// default.
//
// Seam-shape deviation from the brief's literal prose: application.Window (the brief's
// required DisableRestoration parameter type) has several unexported methods
// (handleDragAndDropMessage, shouldUnconditionallyClose, cut/copy/paste/undo/redo/delete/
// selectAll), so it cannot be implemented by a fake type outside package application — there is
// no way to hand DisableRestoration a fake window and observe a call through it, the way
// windowToggler/levelWindow fakes work elsewhere in this file's sibling
// window_toggle_test.go. So this test locks the wiring at the package-level hook instead: F
// must implement DisableRestoration via a swappable `var setWindowNotRestorable =
// SetWindowNotRestorable` (mirroring the injectable-callback pattern used throughout this
// file) and call `disableRestoration(win, setWindowNotRestorable)`. This test asserts that var
// points at SetWindowNotRestorable by default, by comparing the underlying function pointers
// (the standard way to compare func values in Go, which are otherwise only comparable to nil).
func TestWindowWrapperDisableRestorationDefaultsToSetWindowNotRestorable(t *testing.T) {
	got := reflect.ValueOf(setWindowNotRestorable).Pointer()
	want := reflect.ValueOf(SetWindowNotRestorable).Pointer()

	if got != want {
		t.Errorf("setWindowNotRestorable is not wired to SetWindowNotRestorable by default")
	}
}

// TestWindowWrapperDisableRestorationNoopsOnNilWindow confirms the exported method degrades
// safely (no panic) on a nil window, same as the package-level disableRestoration it wraps.
func TestWindowWrapperDisableRestorationNoopsOnNilWindow(t *testing.T) {
	w := &WindowWrapper{}
	w.DisableRestoration(nil) // must not panic
}
