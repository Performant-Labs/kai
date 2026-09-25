// Package execkey implements the "exec key" capability: simulating the copy key to feed
// selection/clipboard content into translation or input backfill.
// It depends on the selection package to read the clipboard, settings for exec key config,
// and app to push events.
// The platform-specific "simulated key copy" (copySelection) and "hotkey parsing"
// (parseHotkey) live in this package's per-platform files.
package execkey

import (
	"log/slog"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/selection"
	"cnb.cool/dtapp/kai/internal/settings"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// ExecKeyController is the exec key controller: it carries all "program actively simulates a
// keypress" exec key logic (strictly distinct from RegisteredHotkeyConfig-style "listened-for
// registration keys"). Exec keys are not part of mgr.Register interception; the registration
// key callback calls them on demand, using robotgo to press the configured combination (e.g.
// Cmd+C) on the user's behalf and writing the selection into the clipboard.
type ExecKeyController struct {
	settingsSvc *settings.Service
	app         *application.App
	selection   *selection.Service
	log         *slog.Logger
}

// NewExecKeyController constructs the exec key controller.
func NewExecKeyController(
	st *settings.Service,
	app *application.App,
	sel *selection.Service,
) *ExecKeyController {
	cfg := st.Get()
	e := &ExecKeyController{
		settingsSvc: st,
		app:         app,
		selection:   sel,
		log:         slog.Default(),
	}
	e.log.Info(i18n.T("log.execkey_config"),
		slog.String(i18n.T("log.field_copykey"), cfg.ExecKeys.Copy.Key),
		slog.Bool(i18n.T("log.field_enabled"), cfg.ExecKeys.Copy.Enabled),
	)
	return e
}

// SetApp injects app once it is ready (startup orchestration phase). It also propagates app
// to the held selection.Service; otherwise selection.Service.app stays nil forever and
// readClipboardText(nil) always returns an empty string (the copy key reads nothing).
func (e *ExecKeyController) SetApp(app *application.App) {
	e.app = app
	if e.selection != nil {
		e.selection.SetApp(app)
	}
}

// CopySelection grabs the current selection text (used by the "summon main window" hotkey):
// it simulates the copy key to capture the selection, while protecting the user's clipboard
// the whole way — backup → clear → copy → clear-then-restore backup (belt and braces).
// The system clipboard always ends up with the user's original content — no selection
// residue. Copy failure / empty selection returns an empty string.
func (e *ExecKeyController) CopySelection() string {
	cfg := e.settingsSvc.Get()
	// 1. Back up the original clipboard, avoiding any modification.
	backup := e.selection.ReadClipboardText()
	e.log.Debug(i18n.T("log.copykey_backup"), slog.Int(i18n.T("log.field_length"), len(backup)))

	// 2. Clear the clipboard so stale content isn’t mistakenly backfilled when there is no
	// selection.
	_ = e.selection.WriteToClipboard("")

	// 3. Perform the copy (honoring the fallback config): if the custom key got nothing,
	// retry with the system default copy key.
	text := e.copySelection(cfg.ExecKeys.Copy.Fallback)

	// 4. Restore the user's original clipboard: clear first (displacing the copied selection
	// residue), then write back the backup — belt and braces, no residue.
	if err := e.selection.WriteToClipboard(""); err != nil {
		e.log.Warn(i18n.T("log.copykey_clear_clipboard_failed"), slog.String(i18n.T("log.field_error"), err.Error()))
	}
	if err := e.selection.WriteToClipboard(backup); err != nil {
		e.log.Warn(i18n.T("log.copykey_restore_clipboard_failed"), slog.String(i18n.T("log.field_error"), err.Error()))
	}
	return text
}

// ReadClipboard reads the system clipboard's current content directly (no simulated copy /
// backup-restore).
// Used by the hotkey branch in "auto clipboard" mode: the user already copied, so pressing
// the hotkey reads the clipboard and translates — never touching the selection or the
// user's clipboard.
func (e *ExecKeyController) ReadClipboard() string {
	return e.selection.ReadClipboardText()
}
