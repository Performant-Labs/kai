// Package execkey implements the "exec key" capability: simulating the copy key to feed
// selection/clipboard content into translation or input backfill.
// It depends on the selection package to read the clipboard, settings for exec key config,
// and app to push events.
// The platform-specific "simulated key copy" (copySelection) and "hotkey parsing"
// (parseHotkey) live in this package's per-platform files.
package execkey

import (
	"log/slog"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/selection"
	"cnb.cool/dtapp/kai/internal/settings"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// pollClipboardDelay/pollClipboardTimeout/pollClipboardInterval govern how long copyWithHotkey
// (both platforms) waits for the target app to actually populate the pasteboard after the
// simulated copy key is injected.
//
// Issue #173 items 2/3: a single fixed 120ms sleep-then-read-once was reliable for native
// AppKit/Win32 text fields (whose Cmd+C/Ctrl+C handling writes the pasteboard synchronously,
// inline with the key event), but returned an empty clipboard for a Chrome tab (e.g. a Google
// Sheets cell) and for WhatsApp's Electron window — both log
// "[CopyKey] Send combo succeeded but clipboard empty". Neither is native: Chrome's copy
// handling runs a JS `copy` event listener inside the page's renderer process, and Electron
// (also Chromium) routes the synthetic key through its own input pipeline before any
// clipboard write happens — both add IPC/event-loop hops a native NSResponder never needs, so
// a single fixed 120ms read can race ahead of the write. pollClipboardTimeout is a generous
// upper bound (chosen well above every observed Chrome/Electron latency while staying under
// the time a user would notice as "nothing happened"); pollClipboardInterval keeps the common
// native-field case (which already has the text within the first tick) just as fast as
// before.
//
// Issue #175 item 1: 600ms was still not enough for a Google Sheets grid-cell copy
// specifically, as opposed to the plain Chrome text field #174's original 600ms was verified
// against. A cell/range copy in Sheets writes several clipboard representations at once (plain
// text, HTML, and its own internal spreadsheet format) through the same async JS `copy` handler
// pollClipboardTimeout already accounts for above — just with more work to do before the
// pasteboard is fully populated, so the existing ceiling could still lose the race on a slower
// machine or a larger selection. Raised to 1200ms: still well under the ~2s a user would read as
// "the app is unresponsive", and pollClipboardInterval already returns as soon as the text
// lands, so this only adds latency to the genuinely-slow case, never the common one. Real
// synthetic-Cmd+C testing during this issue's investigation (a standalone robotgo.KeyTap probe
// against a Chrome textarea, independent of Kai's own clipboard clear/restore dance) showed
// Chrome-based copies are inherently less reliable than native fields regardless of the delay
// between the synthetic keydown and keyup (robotgo.KeySleep 10/25/40/60ms all showed the same
// failure pattern) — so unlike KeySleep, which was tested and found to make no difference and
// was deliberately NOT changed, a longer poll window is the one lever here that's both provably
// safe (bounded added latency, no behavior change on success) and plausibly helpful for exactly
// the multi-representation-write case Sheets exercises. Paired with EventCopyKeyFailed (see
// hotkey/manager.go) as a second line of defense: if 1200ms still isn't enough, the user now
// sees a clear failure instead of stale silence.
const (
	pollClipboardInterval = 40 * time.Millisecond
	pollClipboardTimeout  = 1200 * time.Millisecond
)

// pollClipboardText polls read (selection.Service.ReadClipboardText) every pollClipboardInterval
// until it returns text that is both non-empty AND different from stale, or pollClipboardTimeout
// elapses, whichever comes first. It always performs at least one read. This replaces a single
// fixed-delay read so that apps whose copy handling is asynchronous (a Chrome tab, an Electron
// app) get enough time to populate the pasteboard, without slowing down the common case where a
// native text field already has it on the first read.
//
// PR review finding (issue #173): an earlier version accepted the first non-empty read
// unconditionally. Every caller here already clears the clipboard before injecting the copy key
// (CopySelection's step 2 in this same package), so in practice the first read during normal
// operation is "" and this made no difference — but that safety depended entirely on every
// caller remembering to clear first, which pollClipboardText itself had no way to enforce. The
// stale parameter makes the guarantee self-contained: the caller passes what the clipboard held
// right before the copy key was injected (in the normal path, "" — the same value CopySelection
// already cleared it to), and a read that still equals stale is treated as "not yet updated" and
// keeps polling, even if it is non-empty (e.g. a caller that skipped the clear, or a race where
// the clear itself hadn't visibly landed yet). This is what stops a genuinely stale clipboard
// value — leftover text from a previous unrelated copy — from being returned as if it were the
// new selection.
func pollClipboardText(read func() string, stale string) string {
	deadline := time.Now().Add(pollClipboardTimeout)
	for {
		if text := read(); text != "" && text != stale {
			return text
		}
		if time.Now().After(deadline) {
			return ""
		}
		time.Sleep(pollClipboardInterval)
	}
}

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
