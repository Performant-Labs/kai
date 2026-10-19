package service

import "unsafe"

// applyWindowAppearance points win's native chrome (the title bar) at the theme (issue #8): it
// passes the window's native handle and the resolved theme to set. Wails sets a window's
// appearance only at creation, from the theme resolved at launch, so a theme change after launch
// left a light title bar over dark content. Safe on any platform or test double: a nil window, a
// nil callback or a nil native handle (non-darwin builds, where NativeWindow() returns nil) is a
// no-op. It returns whether the callback reported success, for logging only.
func applyWindowAppearance(win nativeWindowHandle, dark bool, set func(ptr unsafe.Pointer, dark bool) bool) bool {
	if win == nil || set == nil {
		return false
	}
	ptr := win.NativeWindow()
	if ptr == nil {
		return false
	}
	return set(ptr, dark)
}
