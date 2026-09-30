package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Issue #15: the native window background must match the app's own background (app.css
// --app-bg: #ffffff light, #18181c dark), and be opaque, so no light frame shows before the page draws.
func TestWindowBackground(t *testing.T) {
	if got, want := windowBackground(false), application.NewRGBA(0xff, 0xff, 0xff, 0xff); got != want {
		t.Errorf("light = %+v, want %+v", got, want)
	}
	if got, want := windowBackground(true), application.NewRGBA(0x18, 0x18, 0x1c, 0xff); got != want {
		t.Errorf("dark = %+v, want %+v", got, want)
	}
}
