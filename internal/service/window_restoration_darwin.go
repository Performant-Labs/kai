//go:build darwin

package service

import (
	"log/slog"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/i18n"
	"github.com/ebitengine/purego/objc"
)

// SetWindowNotRestorable sends NSWindow.setRestorable:NO to the native window at ptr, opting
// it out of macOS's Secure State Restoration ("Resume", issue #163). Pure Go via
// github.com/ebitengine/purego/objc — no cgo, consistent with this repo's swiftbridge
// zero-cgo policy (see pkg/swiftbridge/load.go).
//
// Guarded by respondsToSelector: (skip silently if the native handle isn't an NSWindow, e.g. a
// future platform or a stray pointer) and by a recover() (mirroring
// swiftbridge.registerSafe's panic-to-error pattern) so a bad pointer can never crash the app.
func SetWindowNotRestorable(ptr unsafe.Pointer) {
	if ptr == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Warn(i18n.T("log.window_restoration_disable_failed"), slog.Any("recover", r))
		}
	}()

	win := objc.ID(uintptr(ptr))
	responds := objc.Send[bool](win, objc.RegisterName("respondsToSelector:"), objc.RegisterName("setRestorable:"))
	if !responds {
		return
	}
	win.Send(objc.RegisterName("setRestorable:"), false)
}
