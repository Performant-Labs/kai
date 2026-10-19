package hotkey

// Issue #11: nothing the user selects or copies may reach kai.log — logs get pasted into
// issues, so a leaked capture is a leaked password. Every arrival path in this package writes
// its one "text arrived" line through h.log; these tests push a canary string through each
// path (the copy-key branch, the auto-clipboard branch and the double Cmd+C trigger) and fail
// if any of it shows up in the captured output at ANY level, while the length — the debugging
// signal that stays — still does. Found a failure? Drop the content field from the log call;
// log the length (and source) only.

import (
	"bytes"
	"log/slog"
	"strconv"
	"strings"
	"testing"
)

// privacyCanary is so unique it cannot appear in the log by chance; its bytes (Chinese
// included) also make len() ≠ rune count, pinning that the logged length is the byte length.
const privacyCanary = "KaiCanary swordfish-密码-Azul-9fJ2mQv"

// captureLogAtDebugLevel points the default logger at a buffer with the level at Debug, so a
// leak at any level fails the test (issue #11 says: at any level).
func captureLogAtDebugLevel(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// assertCanaryAbsent fails when the captured output carries the canary or lacks its length.
func assertCanaryAbsent(t *testing.T, buf *bytes.Buffer, path string) {
	t.Helper()
	out := buf.String()
	if strings.Contains(out, privacyCanary) {
		t.Fatalf("%s wrote the captured text into the log (issue #11):\n%s", path, out)
	}
	// The tests run under the default en-US locale, where the field is "length" (same
	// locale-rooted assertion style as the ReportSourceSwitchSkipped tests).
	if want := "length=" + strconv.Itoa(len(privacyCanary)); !strings.Contains(out, want) {
		t.Fatalf("%s must still log the length %q (issue #11), got:\n%s", path, want, out)
	}
}

func TestTriggerCopyKey_NeverLogsTheCapturedSelection(t *testing.T) {
	buf := captureLogAtDebugLevel(t)
	h := &Manager{log: slog.Default()}
	h.triggerCopyKey(&fakeWindow{}, &fakeEmitter{}, func() (string, error) { return privacyCanary, nil })
	assertCanaryAbsent(t, buf, "copy-key branch")
}

func TestDeliverAutoClipboard_NeverLogsTheClipboardText(t *testing.T) {
	buf := captureLogAtDebugLevel(t)
	h := &Manager{log: slog.Default()}
	h.deliverAutoClipboard(&fakeWin{}, &fakeEmitter{}, privacyCanary)
	assertCanaryAbsent(t, buf, "auto-clipboard branch")
}

func TestDeliverDoubleCopy_NeverLogsTheCopiedText(t *testing.T) {
	buf := captureLogAtDebugLevel(t)
	h := &Manager{log: slog.Default()}
	h.deliverDoubleCopy(&fakeWin{}, &fakeEmitter{}, privacyCanary)
	assertCanaryAbsent(t, buf, "double Cmd+C path")
}
