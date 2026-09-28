package service

import "unsafe"

// nativeWindowHandle is the slice of application.Window that disableRestoration needs — real
// Wails windows and test fakes alike only need to hand back their native handle.
type nativeWindowHandle interface {
	NativeWindow() unsafe.Pointer
}

// disableRestoration opts win out of macOS's Secure State Restoration ("Resume", issue #163):
// it calls setNotRestorable with the window's native handle. Safe to call on any
// platform/test double — a nil window, a nil callback, or a nil native handle (non-darwin
// builds, where NativeWindow() returns nil) is a no-op.
func disableRestoration(win nativeWindowHandle, setNotRestorable func(unsafe.Pointer)) {
	if win == nil || setNotRestorable == nil {
		return
	}
	ptr := win.NativeWindow()
	if ptr == nil {
		return
	}
	setNotRestorable(ptr)
}
