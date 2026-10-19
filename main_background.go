package main

import (
	"cnb.cool/dtapp/kai/internal/service"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// windowBackground is the native window background colour, chosen before the window draws so no
// light frame shows while the page loads (issue #15). The values are the app's own --app-bg tokens
// (frontend/src/app.css, and the same values in the HTML pages' inline head style); alpha is opaque.
func windowBackground(dark bool) application.RGBA {
	if dark {
		return application.NewRGBA(0x18, 0x18, 0x1c, 0xff)
	}
	return application.NewRGBA(0xff, 0xff, 0xff, 0xff)
}

// applyWindowBackground sets one window's colours for the theme (issue #22): the web view draws no
// background of its own, so the window's colour shows through, and the colour behind the page and
// the window itself match the theme. Wails sets only the NSWindow colour at creation, and a hidden
// window shown again used to flash the web view's own white. recolourWindow also sets the window's
// own colour (needed when the theme changes after creation). The window's appearance is applied on
// every call (issue #8): the title bar follows the theme, and Wails sets the appearance only at
// creation from the launch-time theme, so even the first call can correct a wrong creation-time
// value. MUST run on the main thread (call it inside application.InvokeAsync, like
// DisableRestoration).
func applyWindowBackground(windowSvc *service.WindowWrapper, win application.Window, dark, recolourWindow bool) {
	if win == nil {
		return
	}
	c := windowBackground(dark)
	windowSvc.ApplyWebViewBackground(win, c.Red, c.Green, c.Blue)
	windowSvc.ApplyWindowAppearance(win, dark)
	if recolourWindow {
		win.SetBackgroundColour(c)
	}
}
