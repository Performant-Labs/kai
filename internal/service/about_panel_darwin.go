//go:build darwin

package service

import "cnb.cool/dtapp/kai/pkg/swiftbridge"

// ShowAboutPanelInFront calls the Swift bridge to show the standard About panel in front of the
// app's windows (issue #8): the native orderFrontStandardAboutPanel: selector Wails' default
// menu uses leaves the panel below a pinned always-on-top window. MUST be called on the main
// thread (wrap the call in application.InvokeAsync): it talks to AppKit directly.
func ShowAboutPanelInFront() bool {
	// The function pointer is non-nil only once the bridge has loaded and registered it, so a nil
	// check covers "bridge not loaded" and lets a test stub the call.
	if swiftbridge.KaiShowAboutPanel == nil {
		return false
	}
	return swiftbridge.KaiShowAboutPanel() == 1
}
