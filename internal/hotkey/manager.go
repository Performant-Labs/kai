// Package hotkey handles global hotkey registration and broadcasting only, decoupled from
// AppService (the RPC facade).
// It holds only the minimal dependencies needed to register hotkeys (app / settings / exec
// key controller / a few callbacks), with no translation, history, or engine concerns mixed
// in.
//
// The copy key (ExecKeyConfig.Copy) is an exec key — not registered here; execKeyCtrl calls
// it on demand from the registration-key callback.
package hotkey

import (
	"log/slog"

	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/execkey"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/settings"
)

// Manager is the global hotkey manager.
type Manager struct {
	app         *application.App
	settingsSvc *settings.Service
	log         *slog.Logger

	// execKeyCtrl is the exec key controller: the registration-key callback calls its
	// CopySelection on demand to simulate a copy.
	execKeyCtrl *execkey.ExecKeyController

	// TODO(2026-08-11): the selSvc field is disabled. The system text-capture branch (Swift
	// bridge kai_selected_text) is commented out inside Register (users reported that path
	// causing machine issues), so selSvc is no longer held here.
	// To restore the system text-capture path later, uncomment this field and restore the
	// selSvc assignment in NewManager.
	// selSvc *selection.Service

	// The callbacks below are injected by upper layers, bridging window/translation
	// capabilities.
	mainWindow          func() application.Window
	screenshotOCR       func() error
	screenshotTranslate func() error              // Screenshot translate main flow: region screenshot→OCR→translate→deliver to screenshot window
	screenshotWindow    func() application.Window // Screenshot translate window
	emitHotkeysChanged  func([]string)            // Broadcast the currently active list to the frontend
}

// SetApp injects app once it is ready (startup orchestration phase).
func (h *Manager) SetApp(app *application.App) {
	h.app = app
}

// Unregister unregisters each currently registered global hotkey one by one (working around
// UnregisterAll's internal early-return pitfall).
func (h *Manager) Unregister() {
	if h.app == nil {
		return
	}
	mgr := h.app.GlobalShortcut
	if mgr == nil {
		return
	}
	for _, accel := range mgr.GetAll() {
		mgr.Unregister(accel)
	}
}

// NewManager constructs the hotkey manager.
func NewManager(
	app *application.App,
	st *settings.Service,
	execKeyCtrl *execkey.ExecKeyController,
	mainWindow func() application.Window,
	screenshotOCR func() error,
	screenshotTranslate func() error,
	screenshotWindow func() application.Window,
	emitHotkeysChanged func([]string),
) *Manager {
	return &Manager{
		app:         app,
		settingsSvc: st,
		log:         slog.Default(),
		execKeyCtrl: execKeyCtrl,
		// TODO(2026-08-11): the selSvc field is disabled; the parameter is kept for now to
		// preserve main.go's call signature.
		// When restoring the system text-capture path, change back to `selSvc: selSvc,`.
		mainWindow:          mainWindow,
		screenshotOCR:       screenshotOCR,
		screenshotTranslate: screenshotTranslate,
		screenshotWindow:    screenshotWindow,
		emitHotkeysChanged:  emitHotkeysChanged,
	}
}

// TriggerInput is equivalent to pressing the "input translate" hotkey: summons the main
// window, then per config either simulates the copy key or (in the future) takes the system
// text-capture branch, delivering the selection to the input box via EventInputFill.
// Reused by tray menu clicks so menu clicks behave exactly like the real hotkey.
func (h *Manager) TriggerInput() {
	w := h.mainWindow()
	if w == nil {
		return
	}
	cfg := h.settingsSvc.Get()
	switch {
	case cfg.AutoClipboard:
		// Auto-clipboard mode: the user already copied; pressing the hotkey (the "input
		// translate" key) reads the clipboard directly and translates.
		// No simulated copy, no touching the selection, no clobbering the user's clipboard —
		// avoiding double triggering with the original copy key.
		text := h.execKeyCtrl.ReadClipboard()
		h.log.Info(i18n.T("log.hotkey_read_clipboard"), slog.String(i18n.T("log.field_source"), i18n.T("log.source_auto_clipboard")), slog.Int(i18n.T("log.field_length"), len(text)), slog.String(i18n.T("log.field_content"), text))
		w.Show()
		w.Focus()
		if text != "" {
			h.app.Event.Emit(events.EventInputFill, text)
		}
	case cfg.ExecKeys.Copy.Key != "" && cfg.ExecKeys.Copy.Enabled:
		// Copy-key branch: the order is strictly "simulate Cmd+C copy first → then
		// Show/Focus".
		// If Focus ran first, focus would move to Kai, the simulated Cmd+C would land on the
		// Kai window (nothing selected), the clipboard would keep its old value, and the wrong
		// content would be picked up.
		sel := h.execKeyCtrl.CopySelection()
		h.log.Info(i18n.T("log.hotkey_read_clipboard"), slog.String(i18n.T("log.field_source"), i18n.T("log.source_copy_key")), slog.Int(i18n.T("log.field_length"), len(sel)), slog.String(i18n.T("log.field_content"), sel))
		w.Show()
		w.Focus()
		if sel != "" {
			h.app.Event.Emit(events.EventInputFill, sel)
		}
		// TODO(2026-08-11): the "system text capture (macOS Swift bridge kai_selected_text)"
		// branch is temporarily disabled.
		// Reason: users reported odd machine issues after enabling this path (suspected to
		// relate to the 150ms-delayed AX query/focus switch inside the global hotkey
		// callback). Root cause must be found before deciding whether to restore.
		// To restore: uncomment the whole block below (the selSvc field and the
		// SelectedTextViaSystem implementation stay unchanged).
		//
		// case h.selSvc != nil:
		// 	// No copy key configured: system text capture (macOS Swift bridge
		// 	kai_selected_text).
		// 	// Key constraint: at capture time the foreground app must still be the "target
		// 	app", otherwise AX can't read its selection.
		// 	// So Kai must not be Focused first (that would make Kai the foreground app).
		// 	// Order: "delayed capture (focus still on the target app) → then Show/Focus Kai".
		// 	// Why delay: at the instant the global hotkey callback fires, the system is still
		// 	// processing that key event; a synchronous AX query then is rejected
		// 	// (kAXErrorCannotComplete -25212). Only after events settle and focus stabilizes
		// 	// (target app still foreground) can the selection be read.
		// 	time.AfterFunc(150*time.Millisecond, func() {
		// 		sel := h.selSvc.SelectedTextViaSystem()
		// 		h.log.Info("system capture read selection", slog.String("source", "Swift bridge capture"), slog.Int("length", len(sel)))
		// 		w.Show()
		// 		w.Focus()
		// 		if sel != "" {
		// 			h.app.Event.Emit(events.EventInputFill, sel)
		// 		}
		// 		})
	}
}

// TriggerScreenshot is equivalent to pressing the "screenshot translate" hotkey: hide the
// screenshot window→region screenshot→OCR→translate→summon the screenshot window to show the
// result. Reused by tray menu clicks so menu clicks behave exactly like the real hotkey.
// The window summon is triggered by ScreenshotTranslate as soon as the screenshot image is in
// hand (window shows right after capture, without waiting for OCR/translation); this function
// only hides the old window + starts the screenshot flow.
func (h *Manager) TriggerScreenshot() {
	// Hide our own window first so it doesn't cover the user's selection (screencapture's
	// interactive selection needs a clean screen).
	if w := h.screenshotWindow(); w != nil {
		w.Hide()
	}
	if err := h.screenshotTranslate(); err != nil {
		h.log.Error(i18n.T("log.hotkey_screenshot_failed"), slog.Any(i18n.T("log.field_error"), err))
		return
	}
}

// Register registers the global hotkeys (registration keys only: Input/Screenshot).
// Each is Unregistered first, so hotkey config changes take effect live (no restart).
func (h *Manager) Register() {
	if h.app == nil {
		return
	}
	mgr := h.app.GlobalShortcut
	if mgr == nil {
		return
	}
	// Unregister each currently registered item (bypassing UnregisterAll's internal
	// hadShortcuts/started early return, making sure any registered hotkey can truly be
	// revoked at runtime, avoiding "still fires after being cleared").
	for _, accel := range mgr.GetAll() {
		mgr.Unregister(accel)
	}
	cfg := h.settingsSvc.Get()
	if cfg == nil {
		return
	}
	hk := cfg.Hotkeys
	h.log.Info(i18n.T("log.hotkey_start_register"),
		slog.String(i18n.T("log.field_source"), hk.Input.Key),
		slog.Bool(i18n.T("log.field_enabled"), hk.Input.Enabled),
		slog.String(i18n.T("log.field_source"), hk.Screenshot.Key),
		slog.Bool(i18n.T("log.field_enabled"), hk.Screenshot.Enabled),
	)

	// Summon the main window (input focused)
	if hk.Input.Key != "" && hk.Input.Enabled {
		if err := mgr.Register(hk.Input.Key, func() {
			h.log.Info(i18n.T("log.hotkey_trigger"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_main_window")), slog.String(i18n.T("log.field_key"), hk.Input.Key))
			h.TriggerInput()
		}); err != nil {
			h.log.Error(i18n.T("log.hotkey_register_failed"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_main_window")), slog.String(i18n.T("log.field_key"), hk.Input.Key), slog.Any(i18n.T("log.field_error"), err))
		} else {
			h.log.Info(i18n.T("log.hotkey_registered"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_main_window")), slog.String(i18n.T("log.field_key"), hk.Input.Key))
		}
	} else {
		h.log.Info(i18n.T("log.hotkey_skip"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_main_window")), slog.String(i18n.T("log.field_reason"), i18n.T("log.field_value_not_set")))

	}

	// Screenshot translate: region screenshot→system OCR→translate→deliver to the screenshot
	// window and summon it
	if hk.Screenshot.Key != "" && hk.Screenshot.Enabled {
		if err := mgr.Register(hk.Screenshot.Key, func() {
			h.log.Info(i18n.T("log.hotkey_trigger"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_screenshot")), slog.String(i18n.T("log.field_key"), hk.Screenshot.Key))
			h.TriggerScreenshot()
		}); err != nil {
			h.log.Error(i18n.T("log.hotkey_register_failed"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_screenshot")), slog.String(i18n.T("log.field_key"), hk.Screenshot.Key), slog.Any(i18n.T("log.field_error"), err))
		} else {
			h.log.Info(i18n.T("log.hotkey_registered"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_screenshot")), slog.String(i18n.T("log.field_key"), hk.Screenshot.Key))
		}
	} else {
		h.log.Info(i18n.T("log.hotkey_skip"), slog.String(i18n.T("log.field_type"), i18n.T("log.field_value_screenshot")), slog.String(i18n.T("log.field_reason"), i18n.T("log.field_value_not_set")))
	}

	// Collect the currently truly-active registration list (for debugging "still fires after
	// being cleared")
	active := mgr.GetAll()
	h.log.Info(i18n.T("log.hotkey_reregister_done"), slog.Any(i18n.T("log.field_active"), active))

	// Push the currently truly-active hotkey list to the frontend for live display.
	h.emitHotkeysChanged(active)
}
