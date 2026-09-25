//go:build windows

package execkey

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
	"github.com/aiwaki/makc"
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

// Windows copy key: uses makc (a no-cgo cross-platform input library calling user32.dll via
// purego under the hood), keeping CGO_ENABLED=0 consistent with Windows CI. macOS keeps
// copy_darwin.go's robotgo; this file compiles on Windows only (separated by suffix).
// Clipboard reading reuses Wails' application.App.

// copyClient caches the makc client, avoiding Open/Close on every keypress.
var copyClient *makc.Client

func getCopyClient() (*makc.Client, error) {
	if copyClient != nil {
		return copyClient, nil
	}
	c, err := makc.Open()
	if err != nil {
		return nil, err
	}
	copyClient = c
	return copyClient, nil
}

// parseHotkey parses the user-configured hotkey string into a list of makc.Key on Windows.
// Supports forms like "ctrl+c" / "ctrl+shift+c" / "alt+x"; Windows has no Command key (cmd
// is ignored).
// Returns an error on parse failure (including unknown key names); the caller skips the
// simulation.
func parseHotkey(s string) ([]makc.Key, error) {
	parts := strings.Split(s, "+")
	keys := make([]makc.Key, 0, len(parts))
	for _, p := range parts {
		name := strings.TrimSpace(strings.ToLower(p))
		switch name {
		case "cmd", "command", "win", "super":
			// No Windows equivalent key; skip without error
			continue
		}
		k, err := makc.ParseKey(name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", i18n.T("err.execkey_unknown_key"), name, err)
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf(i18n.T("err.execkey_no_valid_key"), s)
	}
	return keys, nil
}

// attachToForeground attaches the current thread (Kai's calling thread) to the foreground
// window's thread, so the subsequently injected Ctrl+C (via SendInput) is reliably handed to
// the foreground target app.
//
// Background: Windows has a foreground lock timeout mechanism — when a non-foreground
// process delivers input via SendInput, the system may "queue" the input without handing it
// to the foreground window, so the target app sporadically never receives the copy key and
// the clipboard reads empty ("sometimes works, sometimes not"). With AttachThreadInput
// linking the input thread to the foreground thread, SendInput's input enters the foreground
// window's message queue directly, bypassing that throttling.
//
// The caller must invoke the returned restore() after injecting to detach, otherwise input
// routing breaks and system-wide stutter can result. When there is no attachable foreground
// window (e.g. the desktop) or the call fails, a no-op restore is returned.
//
// golang.org/x/sys/windows does not export AttachThreadInput; LazyProc calls user32
// directly here.
func attachToForeground() (restore func()) {
	noOp := func() {}
	fg := windows.GetForegroundWindow()
	if fg == 0 {
		return noOp
	}
	fgThread, _ := windows.GetWindowThreadProcessId(fg, nil)
	selfThread := windows.GetCurrentThreadId()
	if fgThread == 0 || fgThread == selfThread {
		return noOp
	}

	user32 := windows.NewLazySystemDLL("user32.dll")
	attachProc := user32.NewProc("AttachThreadInput")
	// Attach only when not already attached, avoiding duplicate-attach errors.
	r, _, err := attachProc.Call(uintptr(selfThread), uintptr(fgThread), 1)
	if r == 0 {
		// Attach failed (e.g. already claimed); give up without breaking the flow.
		slog.Debug(i18n.T("log.copykey_attach_failed"),
			slog.String(i18n.T("log.field_error"), errNoop(err)))
		return noOp
	}
	return func() {
		attachProc.Call(uintptr(selfThread), uintptr(fgThread), 0)
	}
}

// errNoop safely converts a possibly-nil error into a string for Debug logging.
func errNoop(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// copySelection performs, on behalf of the user, the key configured in ExecKeyConfig.Copy on
// Windows (default Ctrl+C),
//
// With fallback=true: if the custom copy key fails (parse failure / injection failure /
// empty clipboard), it automatically retries once with the system default copy key (Ctrl+C) —
// i.e. "fall back to the system-native copy key when the custom key didn't take effect".
// CopySelection passes true here when fallback is enabled (the method itself
// protects/restores the user's clipboard).
//
// Implementation: makc's Keyboard.Combo injects the combination via user32.SendInput (purego
// underneath, zero CGO).
// makc ships its own Windows SendInput backend — no robotgo (robotgo needs CGO, which
// conflicts with Windows CI).
func (e *ExecKeyController) copySelection(fallback bool) string {
	hotkey := e.settingsSvc.Get().ExecKeys.Copy.Key
	text := e.copyWithHotkey(hotkey)

	// Fallback: the custom key got nothing; retry once with the system default copy key
	// (Ctrl+C).
	if fallback && text == "" {
		e.log.Warn(i18n.T("log.copykey_fallback_default"),
			slog.String(i18n.T("log.field_customkey"), hotkey),
		)
		text = e.copyDefaultKey()
	}
	return text
}

// copyDefaultKey presses the system default copy key Ctrl+C directly with makc's real enum
// keys, without ParseKey string parsing. Used only as the Fallback path: when the custom
// copy key didn't take effect, fall back to the system-native copy key.
func (e *ExecKeyController) copyDefaultKey() string {
	client, err := getCopyClient()
	if err != nil {
		e.log.Error(i18n.T("err.execkey_init_makc"), slog.String(i18n.T("log.field_error"), err.Error()))
		return ""
	}

	parentCtx := context.Background()
	if e.app != nil {
		parentCtx = e.app.Context()
	}

	comboErr := application.InvokeSyncWithError(func() error {
		// Attach Kai's thread to the foreground target thread before injecting, avoiding the
		// sporadic failures caused by the foreground lock.
		defer attachToForeground()()
		ctx, cancel := context.WithTimeout(parentCtx, 2*time.Second)
		defer cancel()
		return client.Keyboard.Combo(ctx, makc.KeyControl, makc.KeyC)
	})
	if comboErr != nil {
		e.log.Error(i18n.T("log.copykey_default_send_failed"),
			slog.String(i18n.T("log.field_error"), comboErr.Error()),
		)
		return ""
	}
	e.log.Debug(i18n.T("log.copykey_exec_makc_default_done"))

	time.Sleep(120 * time.Millisecond)
	text := e.selection.ReadClipboardText()
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

	keys, err := parseHotkey(hotkey)
	if err != nil {
		e.log.Warn(i18n.T("log.copykey_parse_hotkey_failed"),
			slog.String(i18n.T("log.field_key"), hotkey), slog.String(i18n.T("log.field_error"), err.Error()))
		return ""
	}

	client, err := getCopyClient()
	if err != nil {
		e.log.Error(i18n.T("err.execkey_init_makc"), slog.String(i18n.T("log.field_error"), err.Error()))
		return ""
	}

	// ctx uses the app lifecycle context (canceled when the app exits), plus a 2s timeout as
	// protection.
	parentCtx := context.Background()
	if e.app != nil {
		parentCtx = e.app.Context()
	}

	e.log.Debug(i18n.T("log.copykey_invoke_parse"),
		slog.String(i18n.T("log.field_key"), hotkey),
		slog.Any("keys", keys),
	)
	// Run on the main thread
	comboErr := application.InvokeSyncWithError(func() error {
		// Attach Kai's thread to the foreground target thread before injecting, avoiding the
		// sporadic failures caused by the foreground lock.
		defer attachToForeground()()
		ctx, cancel := context.WithTimeout(parentCtx, 2*time.Second)
		defer cancel()
		return client.Keyboard.Combo(ctx, keys...)
	})
	if comboErr != nil {
		e.log.Error(i18n.T("log.copykey_send_combo_failed"),
			slog.String(i18n.T("log.field_key"), hotkey),
			slog.String(i18n.T("log.field_error"), comboErr.Error()),
		)
		return ""
	}

	e.log.Debug(i18n.T("log.copykey_exec_makc_done"), slog.String(i18n.T("log.field_key"), hotkey))

	time.Sleep(120 * time.Millisecond)
	text := e.selection.ReadClipboardText()
	if text == "" {
		e.log.Warn(i18n.T("log.copykey_send_combo_empty"),
			slog.String(i18n.T("log.field_key"), hotkey))
	}
	return text
}
