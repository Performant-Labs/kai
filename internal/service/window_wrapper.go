package service

import (
	"log/slog"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// WindowWrapper is a thin adapter: responsible for summoning each window (main window /
// settings window).
// Window handles are looked up on demand via the app reference, avoiding stale window
// instances.
// Only exposes the RPCs the frontend needs — none of the wails lifecycle trio.
type WindowWrapper struct {
	app *application.App
}

// NewWindowWrapper constructs the window Wrapper. app may be injected via SetApp once ready.
func NewWindowWrapper(app *application.App) *WindowWrapper {
	return &WindowWrapper{app: app}
}

// SetApp injects the app once it is ready.
func (w *WindowWrapper) SetApp(app *application.App) {
	w.app = app
}

func (w *WindowWrapper) translateWindow() application.Window {
	if w.app == nil {
		return nil
	}
	win, ok := w.app.Window.GetByName(model.WindowTranslate)
	if !ok {
		slog.Error(i18n.T("log.window_handle_failed"), slog.String("window", model.WindowTranslate))
		return nil
	}
	return win
}

func (w *WindowWrapper) settingsWindow() application.Window {
	if w.app == nil {
		return nil
	}
	win, ok := w.app.Window.GetByName(model.WindowSettings)
	if !ok {
		slog.Error(i18n.T("log.window_handle_failed"), slog.String("window", model.WindowSettings))
		return nil
	}
	return win
}

// screenshotWindow fetches the screenshot translate window handle by name (same style as
// translateWindow/settingsWindow).
func (w *WindowWrapper) screenshotWindow() application.Window {
	if w.app == nil {
		return nil
	}
	win, ok := w.app.Window.GetByName(model.WindowScreenshot)
	if !ok {
		slog.Error(i18n.T("log.window_handle_failed"), slog.String("window", model.WindowScreenshot))
		return nil
	}
	return win
}

// showAndFocus summons and ensures the window is really visible.
// Note: Wails v3 lazily creates the webview implementation (impl) for Hidden windows.
// On the first Show() call, if impl is still nil, the underlying layer only triggers Run()
// to create the webview and does NOT actually show — the first hotkey invocation shows
// nothing and a second press is needed.
// Hence two consecutive Show() calls: the first triggers Run() to build the impl, the second
// actually shows now that the impl is ready.
func showAndFocus(win application.Window) {
	if win == nil {
		return
	}
	win.Show()
	win.Show()
	win.Focus()
}

// ShowTranslateWindow summons the translate window
func (w *WindowWrapper) ShowTranslateWindow() {
	showAndFocus(w.translateWindow())
}

// windowToggler is the slice of application.Window that ToggleTranslateWindow drives, so the
// toggle can be exercised with a fake window and no running Wails app.
type windowToggler interface {
	IsVisible() bool
	Hide() application.Window
}

// ToggleTranslateWindow is the tray-click toggle for the translate window (issue #69): hide it
// when it is visible, otherwise summon it through show.
//
// show MUST be WindowWrapper.ShowTranslateWindow (i.e. showAndFocus), never a bare Show().Focus().
// The translate window is created hidden, so Wails builds its webview lazily and a lone Show()
// only builds it without displaying anything — the first tray click after launch would do nothing.
//
// The hide branch is a bare Hide(). It does not emit EventWindowClosing (only the red X does), so
// text typed into the window survives a tray hide. Nothing here reads the clipboard or the
// selection: a tray click captures nothing, that stays with the input hotkey (TriggerInput).
//
// It is a package-level func rather than a WindowWrapper method so Wails generates no frontend
// binding for it.
func ToggleTranslateWindow(win windowToggler, show func()) {
	if win.IsVisible() {
		win.Hide()
		return
	}
	show()
}

// levelWindow is the slice of application.Window that LowerForSettings drives, so it can be
// exercised with a fake window and no running Wails app.
type levelWindow interface {
	IsVisible() bool
	SetAlwaysOnTop(b bool) application.Window
}

// LowerForSettings drops a visible, possibly pinned (always-on-top) window to the normal level so a
// Settings window opened next is not hidden behind it (issue #69: the pinned translate window
// covered a freshly opened Settings). A hidden window is left alone: it is not in the way, and its
// own pin is re-applied when it is next shown.
//
// The pin preference lives in the frontend (Wails has no getter for the always-on-top state), so
// restoring it is the frontend's job: the Settings close hook broadcasts EventWindowClosing with
// the Settings window name and the translate window re-applies its pin.
//
// A package-level func rather than a WindowWrapper method so Wails generates no frontend binding.
func LowerForSettings(win levelWindow) {
	if win == nil || !win.IsVisible() {
		return
	}
	win.SetAlwaysOnTop(false)
}

// ShowSettings opens settings, in front of a pinned translate window.
func (w *WindowWrapper) ShowSettings() {
	if tw := w.translateWindow(); tw != nil {
		LowerForSettings(tw)
	}
	showAndFocus(w.settingsWindow())
}

// ShowScreenshotWindow summons the screenshot translate window (symmetric with
// ShowTranslateWindow).
// Note: main.go's EventWindowShow('screenshot') still goes through the standalone
// showScreenshotWindow() package-level function; it can later be migrated here uniformly so
// all window summons choke-point through WindowWrapper.
func (w *WindowWrapper) ShowScreenshotWindow() {
	showAndFocus(w.screenshotWindow())
}
