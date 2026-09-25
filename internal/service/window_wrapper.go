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

// ShowSettings opens settings
func (w *WindowWrapper) ShowSettings() {
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
