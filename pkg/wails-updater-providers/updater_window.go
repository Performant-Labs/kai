package wails_updater_providers

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

// The update window's name/size/resize event names go through package globals (defined in
// mirror_provider.go):
// globalWindowName / globalWindowW / globalWindowH / globalResizeEvent,
// read/written externally via package functions like SetWindowName/GetWindowName for runtime
// overrides.

// handleMu guards updaterHandles; the language-switch HTML refresh needs serialized updates.
// Framework-internal user action event names (fired when the user clicks window buttons). In
// BYO mode the framework does not show/hide our window on these
// events, so user-initiated closes are done by this package listening for these events and
// hiding itself (see registerCloseHandler).
const (
	evtUserCancel = "wails:updater:user:cancel"
	evtUserSkip   = "wails:updater:user:skip"
	evtUserRemind = "wails:updater:user:remind"
)

var (
	handleMu                sync.Mutex
	updaterHandles          = map[string]*updaterWindow{}
	resizeHandlerRegistered bool // the resize global listener registers once, avoiding leaks on repeated window rebuilds
	closeHandlerRegistered  bool // the user-action close listener registers once
	themeHandlerRegistered  bool // the system theme change listener registers once
)

// updaterWindow adapts the library's own *application.WebviewWindow into
// updater.WindowHandle. Note: this *deliberately does not implement* updater.WindowSizer
// (i.e. it exposes no SetSize method), otherwise the framework's transition() would overwrite
// our JS-driven adaptive height with its built-in sizes. Height adaptation instead happens
// inside this package: on eventResize we call
// win.SetSize.
//
// The framework registers this handle pointer at Init and it never changes after that;
// language/theme switches refresh content via SetHTML, keeping the same window instance.
type updaterWindow struct {
	win *application.WebviewWindow
	app *application.App
}

func (h *updaterWindow) EmitEvent(name string, data ...any) bool {
	return h.win.EmitEvent(name, data...)
}

// Show is called by the framework when transitioning to the showing state. Idempotent.
func (h *updaterWindow) Show() {
	h.win.Show()
}

// Close is called by the framework in two scenarios:
//  1. the user clicks "Cancel/Skip/Remind me later" → framework u.closeWindow → handle.Close
//  2. CheckAndInstall reopens a session and cleans up the old one → u.session.close →
//     handle.Close
//
// Note scenario 2: every CheckAndInstall run closes the leftover previous session,
// while this BYO window's visibility is controlled entirely by the menu's ShowUpdaterWindow —
// unrelated to the framework session lifecycle. If we hid here, the window just shown by a
// second "check for updates" would be hidden instantly (a flash).
// So this is a no-op — window visibility is fully controlled by this package:
//   - showing: ShowUpdaterWindow force-rebuilds a visible window
//   - hiding: the user clicks X (WindowClosing → Hide) or a button (registerCloseHandler
//     listens for user:cancel/skip/remind and hides). Scenario 2's old session close no
//     longer hides by mistake.
func (h *updaterWindow) Close() {}

// createUpdaterWindow only creates a *updaterWindow object (a stable handle), **not the
// underlying WebviewWindow**. The window's on-demand creation is deferred to the first
// ShowUpdaterWindow — so the startup phase (app.Updater.Init getting the handle via
// OpenUpdaterWindow) never builds the update window,
// avoiding needless webview initialization and traffic-light placeholder. Once created, the
// handle object stays stable; afterwards
// only its inner win is rebuilt (see recreateNativeWindow), so the WindowHandle pointer the
// framework held at Init remains valid across window destroy/rebuild — otherwise the new
// window from "check for updates after closing" and the framework's
// old handle would be different objects, user:cancel would hit the destroyed old window, and
// the close button would appear dead.
func createUpdaterWindow(app *application.App) *updaterWindow {
	return &updaterWindow{app: app}
}

// recreateNativeWindow rebuilds only h's underlying WebviewWindow (the h object itself stays
// stable).
// At creation, Wails correctly injects the native bridge window._wails.invoke and the inline
// event shim,
// so JS Events.Emit (close/install/skip buttons, resize adaptivity) all work.
func recreateNativeWindow(app *application.App, h *updaterWindow, show bool) {
	// BackgroundColour: only the pre-load flash color of the webview — dark (30,30,30) /
	// light (255,255,255),
	// following the app theme GetTheme() (dark/light; auto was already resolved by main.go's
	// resolveUpdaterTheme and pushed in via SetTheme).
	// Title-bar appearance (macOS Appearance / Windows CustomTheme) is never customized;
	// it matches the main window (settings) and uses the system default title bar.
	dark := GetTheme() == ThemeDark
	bg := application.NewRGB(255, 255, 255)
	appearance := application.NSAppearanceNameAqua
	if dark {
		bg = application.NewRGB(30, 30, 30)
		appearance = application.NSAppearanceNameDarkAqua
	}
	winTheme := application.Light
	if dark {
		winTheme = application.Dark
	}
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:                 globalWindowName,
		Title:                T("window_title_check"),
		Width:                globalWindowW,
		Height:               globalWindowH,
		HTML:                 renderWindowHTML(app),
		DisableResize:        false,
		Hidden:               true, // always created hidden; showing is the caller’s double Show() job (avoids 80010108)
		AllowSimpleEventEmit: true, // key: lets JS Events.Emit drive Go listeners directly
		BackgroundColour:     bg,
		// Title-bar theme follows the in-app theme (dark/light): uses Wails3's native theme
		// APIs (macOS Mac.Appearance / Windows WindowsWindow.Theme),
		// not CustomTheme color overrides. Switching dark/light in-app then updates the title
		// bar too.
		Mac: application.MacWindow{
			Appearance: appearance,
		},
		Windows: application.WindowsWindow{
			// Set false, consistent with settings/translate/screenshot. On Windows,
			// true adds WS_EX_TOOLWINDOW (tool window), whose native title bar
			// height and close button styling differ from normal windows; false keeps the
			// title bar aligned with the main window.
			HiddenOnTaskbar: false,
			Theme:           winTheme,
		},
		// Title bar minimize/maximize/close buttons: exactly like the main window (settings)
		MinimiseButtonState: application.ButtonHidden,
		MaximiseButtonState: application.ButtonHidden,
		CloseButtonState:    application.ButtonEnabled,
		// Always stays above the app’s other windows (without stealing focus from other
		// foreground apps).
		AlwaysOnTop: true,
	})

	win.OnWindowEvent(events.Common.WindowClosing, func(e *application.WindowEvent) {
		e.Cancel()
		win.Hide()
	})

	h.win = win
}

// getLiveUpdaterWindow returns a live update window handle: nil when the window was never
// created or was closed/destroyed by the user (Wails' internal WindowClosing listener
// destroys it and removes it from the registry).
// Unlike ensureUpdaterWindow, this function never rebuilds — it only probes liveness (when
// refreshing language/theme, a destroyed window needs no refresh; leave it to
// ShowUpdaterWindow to rebuild, avoiding native crashes from calling
// SetHTML/SetTitle/SetSize on a destroyed window).
//
// Key: app.Window.Get(name) alone is not enough — after a window is destroyed its Go object
// lingers in the registry for a while, and wails' SetTitle/SetSize only check
// w.impl != nil (impl is never nil'ed after destruction); sending setTitle: to a native view
// already in teardown throws NSInvalidArgumentException
// and SIGABRTs immediately. So the window's own IsVisible() is used as the backstop
// (internally it returns false for isDestroyed()),
// and the window counts as usable only when currently truly visible.
//
// Must be called under handleMu.
func getLiveUpdaterWindow(app *application.App) *updaterWindow {
	h, ok := updaterHandles[globalWindowName]
	if !ok || h == nil || h.win == nil {
		return nil
	}
	// The registry keys by id; after a window is destroyed app.Window.Get(name) returns
	// false.
	if _, live := app.Window.Get(globalWindowName); !live {
		return nil
	}
	// A window counts as "safely refreshable" only when currently visible; hidden windows
	// (including never-shown Hidden ones)
	// or destroyed ones are skipped — the next ShowUpdaterWindow rebuilds a window with the
	// latest copy.
	if !h.win.IsVisible() {
		return nil
	}
	return h
}

// ensureUpdaterWindow returns a usable update window handle. The handle object
// (*updaterWindow) stays stable once created and is stored in updaterHandles; h.win is rebuilt
// only when the underlying WebviewWindow is missing or destroyed.
// This keeps the WindowHandle the framework held at Init (pointing at the same h object)
// valid across window destroy/rebuild — the root-cause fix for "the close button is dead
// after reopening".
// The resize global listener registers once (guarded by resizeHandlerRegistered), avoiding
// leaks on repeated rebuilds.
//
// Must be called under handleMu.
func ensureUpdaterWindow(app *application.App) *updaterWindow {
	h := updaterHandles[globalWindowName]
	if h == nil {
		h = createUpdaterWindow(app)
		updaterHandles[globalWindowName] = h
		if !resizeHandlerRegistered {
			registerResizeHandler(app)
			resizeHandlerRegistered = true
		}
		if !closeHandlerRegistered {
			registerCloseHandler(app)
			closeHandlerRegistered = true
		}
		if !themeHandlerRegistered {
			registerThemeHandler(app)
			themeHandlerRegistered = true
		}
		return h
	}

	// The handle object is stable; rebuild h.win (hidden) only when the underlying native
	// window is missing or destroyed.
	alive := h.win != nil
	if alive {
		if _, live := app.Window.Get(globalWindowName); !live {
			alive = false
		}
	}
	if !alive {
		recreateNativeWindow(app, h, false)
	}
	return h
}

// OpenUpdaterWindow creates a stable update window handle on the given app, wraps it into an
// updater.Window option and returns it for app.Updater.Init. **Note: this only creates the
// stable handle object; it does not create the underlying WebviewWindow** — the underlying
// window is created on demand at the first ShowUpdaterWindow, so the startup phase never
// builds the update window (avoiding useless webview initialization, traffic-light
// placeholder, etc.). The window is truly built only when the framework's Show()/check-updates
// menu triggers a show.
//
// The initial language is read straight from the package global GetLocale (injected into the
// HTML template); colors follow the app's GetTheme().
// Callers just set language and theme via SetLocale/SetTheme before Open.
func OpenUpdaterWindow(app *application.App) updater.WindowOption {
	handleMu.Lock()
	defer handleMu.Unlock()
	return updater.BYOWindow(ensureUpdaterWindow(app))
}

// ShowUpdaterWindow shows the update window (called on check-for-updates menu clicks).
//
// Key fixes (root cause of the "flash then vanish"):
//  1. Every show force-destroys the old WebviewWindow and rebuilds it as visible, so the new
//     window **belongs to no old session**; when CheckAndInstall later cleans the old session
//     it calls handle.Close
//     (implemented as a no-op in this package), so it can no longer hide the currently shown
//     window.
//  2. Wails v3's first Show() of a Hidden window only triggers webview creation without
//     actually showing; two consecutive Show() calls are needed (the same known workaround as
//     the project's showScreenshotWindow),
//     plus a Focus() to make sure the window really comes to the front.
func ShowUpdaterWindow(app *application.App) {
	handleMu.Lock()
	defer handleMu.Unlock()
	h := ensureUpdaterWindow(app)
	// On-demand creation: the window was never built before (Init only created the stable
	// handle, not the underlying WebviewWindow),
	// so just create a visible window directly — no need to build hidden then destroy. If one
	// exists, destroy the old window and rebuild it visible.
	if h.win == nil {
		recreateNativeWindow(app, h, true)
	} else {
		h.win.Close() // WebviewWindow.Close: the framework unconditionally destroys and removes from the registry
		recreateNativeWindow(app, h, true)
	}
	h.win.Show()
	h.win.Show() // double Show: works around Wails’ bug where the first Show of a Hidden window doesn’t show
	h.win.Focus()
}

// SetUpdaterLocaleTheme applies a new language and colors to the update window.
// Must be called after OpenUpdaterWindow (language/theme switch scenario).
//
// Key: a visible window cannot be refreshed with SetHTML — after a SetHTML reload, Wails
// does not re-inject the native bridge window._wails.invoke, so all JS Events.Emit (close/
// install/skip buttons, resize adaptivity) break, showing up as "close button dead, window
// doesn't adapt". So when visible we instead
// destroy and rebuild the underlying WebviewWindow (recreateNativeWindow; the creation path
// gets the bridge and inline event shim injected correctly by Wails); the h object stays
// stable, so the framework's WindowHandle remains valid.
// Invisible/destroyed windows skip the refresh — the next ShowUpdaterWindow rebuilds a
// window with the latest copy.
func SetUpdaterLocaleTheme(app *application.App) {
	handleMu.Lock()
	defer handleMu.Unlock()

	h := getLiveUpdaterWindow(app)
	if h == nil {
		return
	}
	// Rebuild the underlying window to re-render with the latest copy; show it again after
	// rebuilding (preserving the user’s current visible state).
	recreateNativeWindow(app, h, true)
	h.win.Show()
	h.win.Show() // double Show: consistent with ShowUpdaterWindow
	h.win.Focus()
}

// registerResizeHandler listens for adaptive-height requests the frontend JS sends via
// Events.Emit(globalResizeEvent, [w, h]) and turns them into window SetSize calls.
//
// Note: the live window must be resolved dynamically every time — the initial win cannot be
// captured — because the resize listener registers only once
// (guarded by resizeHandlerRegistered) while the window may be closed, destroyed and
// rebuilt; with a captured old win, resize events after a rebuild would hit the destroyed
// window, leaving the size stale or crashing.
func registerResizeHandler(app *application.App) {
	// Events.Emit initiated through the inline event shim drops the payload (only the event
	// name arrives),
	// so height data can't be sent back; the window height is fixed via
	// globalWindowW/globalWindowH.
	// When this event arrives we just apply a guaranteed size to the live window, making sure
	// the size is right initially/after rebuilds.
	app.Event.On(globalResizeEvent, func(e *application.CustomEvent) {
		h := getLiveUpdaterWindow(app)
		if h == nil {
			return
		}
		h.win.SetSize(globalWindowW, globalWindowH)
	})
}

// registerCloseHandler listens for the framework events of user-initiated window closes
// (cancel/skip/remind later) and actually hides the window in the callback.
//
// Why we can't rely on the framework's handle.Close() to hide:
//   - In BYO mode handle.Close() is implemented as a no-op by this package (see
//     updaterWindow.Close), to keep CheckAndInstall's old-session cleanup from hiding the
//     currently shown window by mistake.
//   - So user-initiated closes must be an explicit Hide by this package. Clicking X is backed
//     by WindowClosing → Hide;
//     clicking a button (user:cancel/skip/remind) goes through the framework's u.closeWindow
//     → handle.Close (no-op),
//     and the Hide here completes the close.
func registerCloseHandler(app *application.App) {
	hide := func(*application.CustomEvent) {
		h := getLiveUpdaterWindow(app)
		if h == nil {
			return
		}
		h.win.Hide()
	}
	app.Event.On(evtUserCancel, hide)
	app.Event.On(evtUserSkip, hide)
	app.Event.On(evtUserRemind, hide)
}

// registerThemeHandler listens for system appearance changes (Wails' official
// events.Common.ThemeChanged,
// implemented natively per platform; main.go already dispatches using
// application.Env.IsDarkMode() as the trusted source).
// On trigger it rebuilds the window; the background color is decided dark/light inside
// recreateNativeWindow from the app's GetTheme().
// When the system toggles dark/light and the update window is currently visible, the
// underlying window is destroyed and rebuilt so BackgroundColour
// follows the new appearance (Wails' BackgroundColour is fixed at creation and cannot be
// hot-updated — only rebuilt).
// Hidden or destroyed windows skip — the next ShowUpdaterWindow rebuilds with the latest
// system appearance.
func registerThemeHandler(app *application.App) {
	app.Event.OnApplicationEvent(events.Common.ThemeChanged, func(*application.ApplicationEvent) {
		h := getLiveUpdaterWindow(app)
		if h == nil {
			return
		}
		recreateNativeWindow(app, h, true)
		h.win.Show()
		h.win.Show() // double Show: consistent with ShowUpdaterWindow
		h.win.Focus()
	})
}
