package main

import "github.com/wailsapp/wails/v3/pkg/application"

// windowBackground is the native window background colour, chosen before the window draws so no
// light frame shows while the page loads (issue #15). The values are the app's own --app-bg tokens
// (frontend/src/app.css, and the same values in the HTML pages' inline head style); alpha is opaque.
func windowBackground(dark bool) application.RGBA {
	if dark {
		return application.NewRGBA(0x18, 0x18, 0x1c, 0xff)
	}
	return application.NewRGBA(0xff, 0xff, 0xff, 0xff)
}
