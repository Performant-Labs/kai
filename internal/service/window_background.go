package service

import "unsafe"

// applyWebViewBackground makes the web view of win draw no background of its own and sets the colour
// behind the page (issue #22): it passes the window's native handle and the colour to set. Safe on
// any platform or test double: a nil window, a nil callback or a nil native handle (non-darwin
// builds, where NativeWindow() returns nil) is a no-op. It returns whether the callback reported
// success, for logging only.
func applyWebViewBackground(win nativeWindowHandle, r, g, b uint8, set func(ptr unsafe.Pointer, r, g, b int32) bool) bool {
	if win == nil || set == nil {
		return false
	}
	ptr := win.NativeWindow()
	if ptr == nil {
		return false
	}
	return set(ptr, int32(r), int32(g), int32(b))
}
