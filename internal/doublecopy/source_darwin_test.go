//go:build darwin

package doublecopy

import (
	"os"
	"testing"
	"time"

	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// Issue #199: the macOS event source through the REAL Swift bridge (the dylib the test command's
// build.sh step produces). A real event tap needs the Input Monitoring grant, which a headless test
// run does not have, so the tap itself is not started here. What is exercised is everything the tap
// callback does with a key-down (kai_doublecopy_ingest is that callback's body), plus the queue,
// the own-copy suppression and the pasteboard reads.

func realSource(t *testing.T) Source {
	t.Helper()
	if err := swiftbridge.Init(""); err != nil || !swiftbridge.Available() {
		t.Skipf("Swift bridge not loadable: %v", err)
	}
	if swiftbridge.KaiDoubleCopyIngest == nil || swiftbridge.KaiDoubleCopyPoll == nil {
		t.Fatal("the kai_doublecopy_* functions are not registered")
	}
	src, _ := NewPlatformSource(func() string { return "" })
	src.Poll() // drain anything a previous test left
	return src
}

func ingest(keycode int32, flags uint64, autorepeat int32, pid int32) {
	swiftbridge.KaiDoubleCopyIngest(keycode, flags, autorepeat, pid)
}

func TestBridgeRecordsOnlyPlainCmdC(t *testing.T) {
	src := realSource(t)
	ingest(9, FlagCommand, 0, 0)                         // Cmd+V
	ingest(int32(KeyCodeC), 0, 0, 0)                     // C alone
	ingest(int32(KeyCodeC), FlagCommand|FlagShift, 0, 0) // Cmd+Shift+C
	ingest(int32(KeyCodeC), FlagCommand|FlagOption, 0, 0)
	if evs := src.Poll(); len(evs) != 0 {
		t.Fatalf("the bridge recorded %d events for keys that are not Cmd+C: %+v (other keystrokes must never be kept)", len(evs), evs)
	}
	ingest(int32(KeyCodeC), FlagCommand|0x10000 /*caps lock*/, 0, 0)
	evs := src.Poll()
	if len(evs) != 1 || !IsCopyChord(evs[0]) || evs[0].Own || evs[0].Autorepeat {
		t.Fatalf("events = %+v, want one plain real Cmd+C", evs)
	}
	if evs[0].ChangeCount <= 0 {
		t.Fatalf("ChangeCount = %d, want the pasteboard change count at key-down", evs[0].ChangeCount)
	}
	if len(src.Poll()) != 0 {
		t.Fatal("Poll did not drain the queue")
	}
}

func TestBridgeMarksAutorepeatAndOrdersEvents(t *testing.T) {
	src := realSource(t)
	ingest(int32(KeyCodeC), FlagCommand, 0, 0)
	time.Sleep(5 * time.Millisecond)
	ingest(int32(KeyCodeC), FlagCommand, 1, 0)
	evs := src.Poll()
	if len(evs) != 2 || evs[0].Autorepeat || !evs[1].Autorepeat {
		t.Fatalf("events = %+v, want a key-down then an auto-repeat", evs)
	}
	if !(evs[1].At > evs[0].At) {
		t.Fatalf("timestamps not increasing: %v then %v", evs[0].At, evs[1].At)
	}
}

func TestBridgeMarksEventsPostedByKaiItself(t *testing.T) {
	src := realSource(t)
	ingest(int32(KeyCodeC), FlagCommand, 0, int32(os.Getpid()))
	evs := src.Poll()
	if len(evs) != 1 || !evs[0].Own {
		t.Fatalf("events = %+v, want one Own event (source pid is this process)", evs)
	}
}

func TestBridgeSuppressWindowMarksOwnAndExpires(t *testing.T) {
	src := realSource(t)
	MarkOwnCopy()
	ingest(int32(KeyCodeC), FlagCommand, 0, 0) // pid 0: like a real key; only the suppress window marks it
	evs := src.Poll()
	if len(evs) != 1 || !evs[0].Own {
		t.Fatalf("inside the suppress window: %+v, want Own", evs)
	}
	time.Sleep(ownCopyGrace + 100*time.Millisecond)
	ingest(int32(KeyCodeC), FlagCommand, 0, 0)
	evs = src.Poll()
	if len(evs) != 1 || evs[0].Own {
		t.Fatalf("after the suppress window: %+v, want a real event", evs)
	}
}

func TestBridgeStartWithoutPermissionNeverPromptsOrHangs(t *testing.T) {
	src := realSource(t)
	done := make(chan error, 1)
	go func() { done <- src.Start() }()
	select {
	case err := <-done:
		if err == nil {
			src.Stop() // this machine has granted the test binary Input Monitoring: a real tap, listen-only
		} else if err != ErrNoPermission {
			t.Fatalf("Start returned %v, want nil or ErrNoPermission", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start hung")
	}
	src.Stop() // safe when not running
}

func TestBridgePasteboardReads(t *testing.T) {
	realSource(t)
	_, pb := NewPlatformSource(func() string { return "text" })
	a, b := pb.ChangeCount(), pb.ChangeCount()
	if a <= 0 || a != b {
		t.Fatalf("change counts %d, %d: want a positive, stable count", a, b)
	}
	_ = pb.Types()
	if pb.Text() != "text" {
		t.Fatal("Text does not use the injected reader")
	}
}
