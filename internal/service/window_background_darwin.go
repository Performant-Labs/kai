//go:build darwin

package service

import (
	"unsafe"

	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// SetWindowWebViewBackground calls the Swift bridge to make the window's web view draw no background
// of its own (issue #22). MUST be called on the main thread (wrap the call in application.InvokeAsync,
// like DisableRestoration): it talks to AppKit directly.
func SetWindowWebViewBackground(ptr unsafe.Pointer, r, g, b int32) bool {
	// The function pointer is non-nil only once the bridge has loaded and registered it, so a nil check
	// covers "bridge not loaded" and lets a test stub the call.
	if ptr == nil || swiftbridge.KaiWindowSetWebviewBackground == nil {
		return false
	}
	return swiftbridge.KaiWindowSetWebviewBackground(uintptr(ptr), r, g, b) == 1
}
