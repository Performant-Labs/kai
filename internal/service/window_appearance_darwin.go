//go:build darwin

package service

import (
	"unsafe"

	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// SetWindowAppearance calls the Swift bridge to set the window's NSAppearance, so the title bar
// follows the theme (issue #8). MUST be called on the main thread (wrap the call in
// application.InvokeAsync, like DisableRestoration): it talks to AppKit directly.
func SetWindowAppearance(ptr unsafe.Pointer, dark bool) bool {
	// The function pointer is non-nil only once the bridge has loaded and registered it, so a nil
	// check covers "bridge not loaded" and lets a test stub the call.
	if ptr == nil || swiftbridge.KaiWindowSetAppearance == nil {
		return false
	}
	var d int32
	if dark {
		d = 1
	}
	return swiftbridge.KaiWindowSetAppearance(uintptr(ptr), d) == 1
}
