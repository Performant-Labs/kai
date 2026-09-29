package execkey

import (
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
