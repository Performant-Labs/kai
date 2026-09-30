package execkey

import (
	"errors"
	"testing"
	"time"
)

// Issue #173 items 2/3: copyWithHotkey used to read the clipboard exactly once, 120ms after
// injecting the copy key. That was long enough for a native text field (whose Cmd+C/Ctrl+C
// handling writes the pasteboard synchronously) but not for a Chrome tab or an Electron app
// (WhatsApp), whose copy handling is asynchronous — reported as "[CopyKey] Send combo
// succeeded but clipboard empty" for both. pollClipboardText replaces the single fixed-delay
// read with a poll loop, verified here.

func TestPollClipboardText_ReturnsImmediatelyWhenAlreadyPopulated(t *testing.T) {
	calls := 0
	start := time.Now()
	got := pollClipboardText(func() string {
		calls++
		return "hello"
	}, "" /* stale: nothing was there before the copy key was injected */)
	elapsed := time.Since(start)
	if got != "hello" {
		t.Fatalf("got %q, want %q", got, "hello")
	}
	if calls != 1 {
		t.Fatalf("expected exactly one read for the already-populated case, got %d", calls)
	}
	if elapsed >= pollClipboardInterval {
		t.Fatalf("expected no wait when the first read already succeeds, took %v", elapsed)
	}
}

func TestPollClipboardText_PollsUntilPopulated(t *testing.T) {
	// Simulates the Chrome/Electron case: the pasteboard is empty for the first couple of
	// reads (the app's async copy handler hasn't run yet), then populated.
	calls := 0
	got := pollClipboardText(func() string {
		calls++
		if calls < 3 {
			return ""
		}
		return "selection text"
	}, "")
	if got != "selection text" {
		t.Fatalf("got %q, want %q", got, "selection text")
	}
	if calls != 3 {
		t.Fatalf("expected 3 reads before success, got %d", calls)
	}
}

func TestPollClipboardText_GivesUpAtTimeout(t *testing.T) {
	calls := 0
	start := time.Now()
	got := pollClipboardText(func() string {
		calls++
		return ""
	}, "")
	elapsed := time.Since(start)
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
	if calls < 2 {
		t.Fatalf("expected more than one attempt before giving up, got %d", calls)
	}
	// Allow generous slack for CI scheduling jitter; the point is it stops around the
	// configured timeout, not that it hangs indefinitely or returns instantly.
	if elapsed < pollClipboardTimeout {
		t.Fatalf("returned before the timeout elapsed: %v < %v", elapsed, pollClipboardTimeout)
	}
	if elapsed > pollClipboardTimeout+500*time.Millisecond {
		t.Fatalf("took far longer than the timeout: %v", elapsed)
	}
}

// PR review finding: an earlier version of pollClipboardText accepted the first non-empty
// read unconditionally, so if the clipboard already held leftover text from a previous copy
// (stale, non-empty) it would be returned immediately, before the target app's async copy
// handler had a chance to write the actual new selection. Every real caller in this package
// already clears the clipboard before injecting the key (or, for copyDefaultKey/copyWithHotkey,
// snapshots it as `stale`), but pollClipboardText itself must not depend on that discipline —
// this pins the guarantee at the unit under test.
func TestPollClipboardText_IgnoresStaleNonEmptyValue(t *testing.T) {
	const staleLeftover = "yesterday's copied text"
	calls := 0
	got := pollClipboardText(func() string {
		calls++
		if calls < 3 {
			// The pasteboard still holds the pre-existing (stale) value: the target app's
			// async copy handler hasn't overwritten it yet.
			return staleLeftover
		}
		return "the actual new selection"
	}, staleLeftover)
	if got != "the actual new selection" {
		t.Fatalf("got %q, want the new selection (stale value must not be returned)", got)
	}
	if calls != 3 {
		t.Fatalf("expected 3 reads before the stale value changed, got %d", calls)
	}
}

// If the clipboard never changes from the stale value (e.g. the copy genuinely failed), that
// must still be treated as "no new selection" — not silently returned as if it were fresh —
// and give up at the timeout like the empty-clipboard case.
func TestPollClipboardText_GivesUpWhenValueNeverChangesFromStale(t *testing.T) {
	const staleLeftover = "old clipboard content"
	calls := 0
	start := time.Now()
	got := pollClipboardText(func() string {
		calls++
		return staleLeftover
	}, staleLeftover)
	elapsed := time.Since(start)
	if got != "" {
		t.Fatalf("got %q, want empty (the stale value never changed, so no new selection was copied)", got)
	}
	if elapsed < pollClipboardTimeout {
		t.Fatalf("returned before the timeout elapsed: %v < %v", elapsed, pollClipboardTimeout)
	}
	if calls < 2 {
		t.Fatalf("expected more than one attempt before giving up, got %d", calls)
	}
}

// Issue #175 item 1: a Google Sheets cell copy can populate the pasteboard later than the old
// 600ms ceiling. A value that lands at ~900ms must still be returned; this fails if
// pollClipboardTimeout is reverted to 600ms.
func TestPollClipboardText_CapturesValueLandingAfterOldSixHundredMsCeiling(t *testing.T) {
	if pollClipboardTimeout < 1200*time.Millisecond {
		t.Fatalf("pollClipboardTimeout = %v, want >= 1200ms (issue #175 item 1)", pollClipboardTimeout)
	}
	start := time.Now()
	got := pollClipboardText(func() string {
		if time.Since(start) >= 900*time.Millisecond {
			return "sheets cell"
		}
		return ""
	}, "")
	if got != "sheets cell" {
		t.Fatalf("got %q, want the value that landed at ~900ms", got)
	}
}

// ---- Issue #194: a missing Accessibility permission is its own outcome ----
//
// Without the permission macOS drops the simulated Cmd+C silently, the clipboard stays empty and
// the failure looked exactly like "nothing selected". The permission is checked before anything is
// touched. The permission is faked with a func; the code under test is the real guardedCopy and the
// real CopySelection.

func TestGuardedCopy_PermissionMissing_NeverRunsTheCopyAndReportsIt(t *testing.T) {
	ran := 0
	text, err := guardedCopy(func() bool { return false }, func() string { ran++; return "x" })
	if !errors.Is(err, ErrAccessibilityMissing) {
		t.Fatalf("err = %v, want ErrAccessibilityMissing", err)
	}
	if text != "" {
		t.Fatalf("text = %q, want empty", text)
	}
	if ran != 0 {
		t.Fatalf("the copy ran %d times with the permission missing, want 0 (no key, no clipboard touch)", ran)
	}
}

func TestGuardedCopy_PermissionPresent_RunsTheCopyOnceAndReturnsItsText(t *testing.T) {
	ran := 0
	text, err := guardedCopy(func() bool { return true }, func() string { ran++; return "picked" })
	if err != nil || text != "picked" || ran != 1 {
		t.Fatalf("text=%q err=%v ran=%d, want picked/nil/1", text, err, ran)
	}
}

func TestGuardedCopy_PermissionPresentButNothingSelected_IsNotAPermissionError(t *testing.T) {
	text, err := guardedCopy(func() bool { return true }, func() string { return "" })
	if err != nil || text != "" {
		t.Fatalf("text=%q err=%v, want an empty selection with no error", text, err)
	}
}

// CopySelection itself is wired through the check: with the permission missing it returns before
// it reads settings, the clipboard or the keyboard (all nil here, so touching any would panic).
func TestCopySelection_PermissionMissing_ReturnsTheTypedErrorWithoutTouchingAnything(t *testing.T) {
	e := &ExecKeyController{accessibility: func() bool { return false }}
	text, err := e.CopySelection()
	if !errors.Is(err, ErrAccessibilityMissing) || text != "" {
		t.Fatalf("text=%q err=%v, want empty and ErrAccessibilityMissing", text, err)
	}
}
