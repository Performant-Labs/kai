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
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/doublecopy"
	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/execkey"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/settings"
)

// accessibilityNoticeEvery is the least time between two "Accessibility permission missing"
// messages (issue #194). The message is a 12-second toast, so a user who presses the hotkey again
// within a few minutes of reading it is not shown it again; a press after that shows it again,
// because the capture is still failing and a window that stays silent is the original bug.
const accessibilityNoticeEvery = 3 * time.Minute

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

	// accessNoticeMu guards accessNoticeAt, the time the missing-permission message was last sent
	// (zero: not yet this launch). now is the clock (nil: time.Now), a field so tests control it.
	accessNoticeMu sync.Mutex
	accessNoticeAt time.Time
	now            func() time.Time

	// triggerInputBusy guards TriggerInput's copy-key branch against reentrancy (issue #175
	// item 5). A real ~/.kai/logs/kai.log capture showed two "[Hotkey] Hotkey triggered" lines
	// well under a second apart for what the user experienced as one press (macOS occasionally
	// redelivers a global-hotkey keydown, and a fast double-tap does the same) — with nothing
	// serializing them, the two calls' CopySelection() invocations raced on the one shared
	// system clipboard: one call's step 2 clear, step 3 copy, or step 4 restore could interleave
	// with another's, so a hotkey press could legitimately-looking capture text that was never
	// actually the current selection. That is a stronger, more general explanation for "returns
	// stale content, not what was highlighted" than a slow/failed copy alone (which is already
	// covered by EventCopyKeyFailed above) — it also explains a *non-empty* wrong capture, which
	// an empty-capture check can never catch. An overlapping call now backs off immediately
	// instead of racing.
	triggerInputBusy atomic.Bool

	// doubleCopy is the "translate on double Cmd+C" listener (issue #199). Register keeps it in
	// step with the setting, so switching it on or off takes effect without a restart.
	doubleCopy doubleCopyController
}

// doubleCopyController is the slice of *doublecopy.Service the manager drives (fakeable).
type doubleCopyController interface {
	Apply(on bool)
	Status() doublecopy.Status
}

// SetApp injects app once it is ready (startup orchestration phase).
func (h *Manager) SetApp(app *application.App) {
	h.app = app
}

// Unregister unregisters each currently registered global hotkey one by one (working around
// UnregisterAll's internal early-return pitfall).
func (h *Manager) Unregister() {
	if h.doubleCopy != nil {
		h.doubleCopy.Apply(false) // shutting down: stop the event tap
	}
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
	m := &Manager{
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
	m.doubleCopy = m.newDoubleCopy()
	return m
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
		var emitter eventEmitter
		if h.app != nil {
			emitter = h.app.Event
		}
		showAndFill(w, emitter, text)
	case cfg.ExecKeys.Copy.Key != "" && cfg.ExecKeys.Copy.Enabled:
		// Issue #175 item 5: CopySelection backs up/clears/copies/restores the one shared
		// system clipboard, which is only safe as an atomic sequence. A second overlapping
		// TriggerInput (a redelivered global-hotkey event or a fast double-tap; see
		// triggerInputBusy's doc comment) would race its own backup/clear/copy/restore against
		// this one's, and the loser can walk away with whatever the winner's step happened to
		// leave on the clipboard — non-empty, plausible-looking, and wrong. CompareAndSwap
		// claims the guard atomically; an overlapping call backs off immediately instead of
		// racing.
		var emitter eventEmitter
		if h.app != nil {
			emitter = h.app.Event
		}
		h.triggerCopyKey(w, emitter, h.execKeyCtrl.CopySelection)
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

// showAndFill is the one "bring the window up and hand it this text" step: the window is shown and
// focused, and non-empty text is delivered to the input box through EventInputFill, the event whose
// arrival path (source-language switch and translate, issue #200) every filled text goes through.
// The auto-clipboard branch of TriggerInput and the double Cmd+C trigger (issue #199) both end
// here. emitter may be nil.
func showAndFill(w windowShower, emitter eventEmitter, text string) {
	w.Show()
	w.Focus()
	if text != "" && emitter != nil {
		emitter.Emit(events.EventInputFill, text)
	}
}

// windowShower is the slice of application.Window that triggerCopyKey drives (fakeable).
type windowShower interface {
	Show() application.Window
	Focus()
}

// triggerCopyKey is TriggerInput's copy-key branch, with its dependencies injected so the
// reentrancy guard and the outcome emission can be tested by calling it (issue #175 item 5).
// Order matters: copy first, then Show/Focus (see TriggerInput). emitter may be nil.
func (h *Manager) triggerCopyKey(w windowShower, emitter eventEmitter, copySelection func() (string, error)) {
	// An overlapping call would race its own backup/clear/copy/restore of the shared system
	// clipboard against the in-flight one (see triggerInputBusy); back off instead.
	if !h.triggerInputBusy.CompareAndSwap(false, true) {
		h.log.Warn(i18n.T("log.hotkey_trigger_busy"))
		return
	}
	defer h.triggerInputBusy.Store(false)

	sel, err := copySelection()
	if errors.Is(err, execkey.ErrAccessibilityMissing) {
		// Issue #194: the key was never posted. Say so; do not report an empty selection.
		h.log.Warn("copy-key capture skipped: macOS Accessibility permission is not granted to Kai")
		w.Show()
		w.Focus()
		if emitter != nil && h.accessibilityNoticeDue() {
			emitter.Emit(events.EventAccessibilityMissing)
		}
		return
	}
	h.log.Info(i18n.T("log.hotkey_read_clipboard"), slog.String(i18n.T("log.field_source"), i18n.T("log.source_copy_key")), slog.Int(i18n.T("log.field_length"), len(sel)), slog.String(i18n.T("log.field_content"), sel))
	w.Show()
	w.Focus()
	if emitter != nil {
		emitCopyKeyOutcome(emitter, sel)
	}
}

// accessibilityNoticeDue reports whether the missing-permission message should be shown now, and
// records that it was: true for the first call of a launch, then at most once per
// accessibilityNoticeEvery.
func (h *Manager) accessibilityNoticeDue() bool {
	now := time.Now
	if h.now != nil {
		now = h.now
	}
	t := now()
	h.accessNoticeMu.Lock()
	defer h.accessNoticeMu.Unlock()
	if !h.accessNoticeAt.IsZero() && t.Sub(h.accessNoticeAt) < accessibilityNoticeEvery {
		return false
	}
	h.accessNoticeAt = t
	return true
}

// eventEmitter is the slice of *application.App that emitCopyKeyOutcome drives, so the
// decision can be exercised with a fake and no running Wails app (same style as
// window_wrapper.go's windowToggler/levelWindow).
type eventEmitter interface {
	Emit(name string, data ...any) bool
}

// copyKeyOutcome decides which event TriggerInput's copy-key branch should emit for the result
// of ExecKeyController.CopySelection, given sel: EventInputFill with sel as payload when
// something was actually captured, or EventCopyKeyFailed (no payload) when the simulated copy
// captured nothing.
//
// Issue #175 item 5: before this existed, a failed capture (sel == "") emitted nothing at all.
// TriggerInput's w.Show()/w.Focus() always run regardless, so the window still comes to the
// front — but with no EventInputFill, the input box keeps showing whatever text a previous
// session left in it (issue #81's retained-session design: it is cleared only by Clear, a new
// EventInputFill, or a new translate — never just by the window closing and reopening). A user
// who doesn't notice the text didn't change has no way to tell that apart from the old text
// genuinely being the new selection — "translates the wrong, stale content with no indication
// anything is wrong" is exactly the bug report. Pure and side-effect-free so the mapping itself
// (not the Emit call) is what gets tested.
func copyKeyOutcome(sel string) (event string, args []any) {
	if sel != "" {
		return events.EventInputFill, []any{sel}
	}
	return events.EventCopyKeyFailed, nil
}

// emitCopyKeyOutcome emits whatever copyKeyOutcome decides for sel on emitter.
func emitCopyKeyOutcome(emitter eventEmitter, sel string) {
	event, args := copyKeyOutcome(sel)
	emitter.Emit(event, args...)
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
	// The double Cmd+C listener follows its setting on every register, which runs at startup and on
	// every settings save (issue #199). It does not depend on the global-shortcut manager below.
	if h.settingsSvc != nil {
		h.syncDoubleCopy(h.settingsSvc.Get())
	}
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

// newDoubleCopy builds the double Cmd+C listener (issue #199) around this manager's window and
// events. Its source is the macOS event tap in the Swift bridge; elsewhere it reports unsupported.
func (h *Manager) newDoubleCopy() *doublecopy.Service {
	src, pb := doublecopy.NewPlatformSource(func() string {
		if h.execKeyCtrl == nil {
			return ""
		}
		return h.execKeyCtrl.ReadClipboard()
	})
	return doublecopy.New(doublecopy.Config{
		Source:     src,
		Pasteboard: pb,
		Enabled: func() bool {
			cfg := h.settingsSvc.Get()
			return cfg != nil && cfg.DoubleCopyTranslate
		},
		Deliver:             h.DeliverDoubleCopy,
		RequestPermission:   doublecopy.RequestPermission,
		OnPermissionMissing: h.notifyDoubleCopyPermissionMissing,
		Log:                 h.log,
	})
}

// DeliverDoubleCopy is where a double Cmd+C ends: it shows the translate window and fills its input
// with the copied text, exactly like the auto-clipboard branch of TriggerInput, so the text takes
// the same arrival path. It never logs the text.
func (h *Manager) DeliverDoubleCopy(text string) {
	if h.mainWindow == nil {
		return
	}
	w := h.mainWindow()
	if w == nil {
		return
	}
	var emitter eventEmitter
	if h.app != nil {
		emitter = h.app.Event
	}
	h.deliverDoubleCopy(w, emitter, text)
}

func (h *Manager) deliverDoubleCopy(w windowShower, emitter eventEmitter, text string) {
	h.log.Info(i18n.T("log.hotkey_read_clipboard"), slog.String(i18n.T("log.field_source"), "double Cmd+C"), slog.Int(i18n.T("log.field_length"), len(text)))
	showAndFill(w, emitter, text)
}

// notifyDoubleCopyPermissionMissing tells the translate window that Input Monitoring is missing.
func (h *Manager) notifyDoubleCopyPermissionMissing() {
	h.log.Warn("double Cmd+C is on but macOS Input Monitoring is not granted to Kai")
	if h.app != nil {
		h.app.Event.Emit(events.EventDoubleCopyPermissionMissing)
	}
}

// syncDoubleCopy makes the listener follow the setting. A nil config means off.
func (h *Manager) syncDoubleCopy(cfg *settings.Settings) {
	if h.doubleCopy == nil {
		return
	}
	h.doubleCopy.Apply(cfg != nil && cfg.DoubleCopyTranslate)
}

// DoubleCopyStatus is the listener's state as a string for the Settings page: "off", "running",
// "missing_permission", "unsupported" or "error".
func (h *Manager) DoubleCopyStatus() string {
	if h.doubleCopy == nil {
		return string(doublecopy.StatusOff)
	}
	return string(h.doubleCopy.Status())
}
