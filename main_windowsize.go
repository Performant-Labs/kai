package main

import (
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"cnb.cool/dtapp/kai/internal/settings"
)

// resizePersistDebounce controls how long persistTranslateWindowSize waits after the last
// WindowDidResize event before writing settings.json. WindowDidResize fires on essentially
// every frame of a live-drag resize (macOS/Windows both report continuous size-change
// events), so writing on every event would hammer the config file; debouncing to "quiet for
// this long" means one write per resize gesture, right after the user lets go.
const resizePersistDebounce = 400 * time.Millisecond

// translateWindowSize resolves the width/height to create the translate window with:
// the last size the user resized it to (persisted in settings.json, issue #173 item 5), or
// (defaultW, defaultH) when nothing was saved yet, or when a saved value is invalid — zero
// (never saved) or smaller than the window's own minimums, which would otherwise silently
// create a window below its declared MinWidth/MinHeight.
func translateWindowSize(settingsSvc *settings.Service, defaultW, defaultH, minW, minH int) (int, int) {
	if settingsSvc == nil {
		return defaultW, defaultH
	}
	cfg := settingsSvc.Get()
	if cfg == nil {
		return defaultW, defaultH
	}
	w, h := cfg.TranslateWindowWidth, cfg.TranslateWindowHeight
	if w < minW || h < minH {
		return defaultW, defaultH
	}
	return w, h
}

// persistTranslateWindowSize registers a WindowDidResize listener on win that debounces and
// persists its current size to settingsSvc, so the size survives the next relaunch (issue
// #173 item 5, restored by translateWindowSize above at window creation).
//
// Window size is a native (Go-side) window property (not frontend/Svelte state), so — like
// the DisableRestoration hook right above this call in main.go — this writes directly via
// settingsSvc rather than through the frontend's SaveConfig RPC. The returned stop func
// cancels any pending debounce timer; main() defers it around app.Run() so a pending write at
// shutdown doesn't reference a timer that outlives the process (time.AfterFunc's goroutine is
// otherwise harmless to leak at process exit, but Stop keeps this tidy and testable).
func persistTranslateWindowSize(win application.Window, settingsSvc *settings.Service) (stop func()) {
	var timer *time.Timer
	win.OnWindowEvent(events.Common.WindowDidResize, func(*application.WindowEvent) {
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(resizePersistDebounce, func() {
			w, h := win.Size()
			if w <= 0 || h <= 0 {
				return
			}
			cfg := settingsSvc.Get()
			if cfg == nil {
				return
			}
			if cfg.TranslateWindowWidth == w && cfg.TranslateWindowHeight == h {
				return
			}
			cfg.TranslateWindowWidth = w
			cfg.TranslateWindowHeight = h
			_ = settingsSvc.Save()
		})
	})
	return func() {
		if timer != nil {
			timer.Stop()
		}
	}
}
