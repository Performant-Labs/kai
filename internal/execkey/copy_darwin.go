//go:build darwin

package execkey

import (
	"log/slog"
	"strings"

	"cnb.cool/dtapp/kai/internal/doublecopy"
	"cnb.cool/dtapp/kai/internal/i18n"
	"github.com/go-vgo/robotgo"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// parseHotkey parses the user-configured hotkey string into the (key, modifiers) that
// robotgo.KeyTap needs, on macOS.
// macOS-specific aliases: Cmd/Command → "cmd", Option → "alt".
func parseHotkey(s string) (key string, modifiers []string) {
	for part := range strings.SplitSeq(s, "+") {
		p := strings.TrimSpace(part)
		switch strings.ToLower(p) {
		case "cmd", "command":
			modifiers = append(modifiers, "cmd")
		case "ctrl", "control":
			modifiers = append(modifiers, "ctrl")
		case "shift":
			modifiers = append(modifiers, "shift")
		case "alt", "option":
			modifiers = append(modifiers, "alt")
		default:
			key = strings.ToLower(p)
		}
	}
	return key, modifiers
}

// copySelection performs, on behalf of the user, the key configured in ExecKeyConfig.Copy on
// macOS, writing the target app's selection into the clipboard, and returns the clipboard
// text.
//
// With fallback=true: if the custom copy key fails (parse failure / injection failure /
// empty clipboard), it automatically retries once with the system default copy key (Cmd+C) —
// i.e. "fall back to the system-native copy key when the custom key didn't take effect".
// CopySelection passes true here when fallback is enabled (the method itself
// protects/restores the user's clipboard).
//
// Key constraint: robotgo.KeyTap goes through CGEvent and must run on the main thread,
// otherwise SIGTRAP. Wails3's GlobalShortcut callback runs on its own goroutine (not the
// main thread), so application.InvokeSyncWithError dispatches the KeyTap back to the main
// thread (same call structure as Windows' makc Combo, easing unified debugging).
// sleep + readClipboard still run on the current goroutine.
func (e *ExecKeyController) copySelection(fallback bool) string {
	hotkey := e.settingsSvc.Get().ExecKeys.Copy.Key
	text := e.copyWithHotkey(hotkey)

	// Fallback: the custom key got nothing; retry once with the system default copy key
	// (Cmd+C).
	if fallback && text == "" {
		e.log.Warn(i18n.T("log.copykey_fallback_default"),
			slog.String(i18n.T("log.field_customkey"), hotkey),
		)
		text = e.copyDefaultKey()
	}
	return text
}

// copyDefaultKey presses the system default copy key Cmd+C directly via robotgo's real API,
// without string parsing (robotgo's KeyTap takes key/mods string values anyway).
// Used only as the Fallback path: when the custom copy key didn't take effect, fall back to
// the system-native copy key.
func (e *ExecKeyController) copyDefaultKey() string {
	// Snapshot the clipboard before injecting the key, so pollClipboardText can tell a genuinely
	// new value from a stale one still sitting there when the target app's copy handler hasn't
	// run yet (see pollClipboardText's doc comment).
	before := e.selection.ReadClipboardText()
	doublecopy.MarkOwnCopy() // Kai's own Cmd+C is not the user's double press (issue #199)
	comboErr := application.InvokeSyncWithError(func() error {
		return robotgo.KeyTap("c", "cmd")
	})
	if comboErr != nil {
		e.log.Warn(i18n.T("log.copykey_default_send_failed"),
			slog.Any("error", comboErr),
		)
		return ""
	}
	e.log.Debug(i18n.T("log.copykey_exec_default_done"))

	text := pollClipboardText(e.selection.ReadClipboardText, before)
	if text == "" {
		e.log.Warn(i18n.T("log.copykey_default_empty"))
	}
	return text
}

// copyWithHotkey performs one "simulated copy + read clipboard" round with the given hotkey
// string.
// Any failure along the way (parse failure / injection failure / empty clipboard) returns an
// empty string; the caller decides whether to fall back.
func (e *ExecKeyController) copyWithHotkey(hotkey string) string {
	if strings.TrimSpace(hotkey) == "" {
		return ""
	}

	key, modifiers := parseHotkey(hotkey)
	if key == "" {
		e.log.Warn(i18n.T("log.copykey_parse_hotkey_failed"),
			slog.String(i18n.T("log.field_key"), hotkey),
		)
		return ""
	}

	mods := make([]any, len(modifiers))
	for i, m := range modifiers {
		mods[i] = m
	}

	e.log.Debug(i18n.T("log.copykey_invoke_parse"),
		slog.String(i18n.T("log.field_key"), hotkey),
		slog.String("key", key),
		slog.Any("modifiers", modifiers),
		slog.Any("mods", mods),
	)
	// Snapshot the clipboard before injecting the key — see pollClipboardText's doc comment.
	before := e.selection.ReadClipboardText()
	// Run on the main thread
	doublecopy.MarkOwnCopy() // Kai's own Cmd+C is not the user's double press (issue #199)
	comboErr := application.InvokeSyncWithError(func() error {
		return robotgo.KeyTap(key, mods...)
	})
	if comboErr != nil {
		e.log.Warn(i18n.T("log.copykey_send_combo_failed"),
			slog.String(i18n.T("log.field_key"), hotkey),
			slog.Any("error", comboErr),
		)
		return ""
	}
	e.log.Debug(i18n.T("log.copykey_exec_hotkey_done"),
		slog.String(i18n.T("log.field_key"), hotkey),
	)

	text := pollClipboardText(e.selection.ReadClipboardText, before)
	if text == "" {
		e.log.Warn(i18n.T("log.copykey_send_combo_empty"),
			slog.String(i18n.T("log.field_key"), hotkey),
		)
	}
	return text
}
