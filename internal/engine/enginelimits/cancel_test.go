//go:build enginelimits && darwin

package enginelimits

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// Apple cancel checks (issue #111, criterion 8). Opt-in like the probe: build tag enginelimits
// (darwin), KAI_ENGINE_PROBE=1, external linking, and the main-thread TestMain in probe_test.go. They
// run against the real Translation framework, with the installed English and Chinese (Simplified)
// languages and the freshly built bridge, so they cannot run in CI; the Swift half has no other
// behaviour test (pkg/swiftbridge/swift_source_test.go only reads its text). Run them with:
//
//	KAI_ENGINE_PROBE=1 CGO_ENABLED=1 go test -tags enginelimits -ldflags=-linkmode=external -run 'TestAppleCancel|TestAppleNotCutOff' -timeout 30m -count=1 -v ./internal/engine/enginelimits/
//
// Every latency here is logged as a "CANCEL" line. Only the ones named "asserted" decide a verdict:
// how long the framework takes is the machine's, not the code's.

const (
	cancelSize       = 3200             // Latin runes: failed at exactly 20.0 s under the old wait
	oldWait          = 20 * time.Second // the bridge's removed wait, for the log note only
	cancelAfter      = 3 * time.Second  // how far into the request the user cancels
	promptly         = time.Second      // a cancel must free the caller within this
	afterCancelGuard = 3 * time.Minute  // ceiling for the request issued right after a cancel
)

// gatedProber skips the test unless the probe gate is set, then runs the probe's pre-check, so a
// missing prerequisite stops the run with its cause instead of looking like a cancel failure.
func gatedProber(t *testing.T) *prober {
	t.Helper()
	if os.Getenv(gateEnv) != "1" {
		t.Skip("set " + gateEnv + "=1 to run the Apple cancel checks (the command is in cancel_test.go)")
	}
	p := &prober{t: t, tr: engine.NewApple()}
	p.precheck()
	return p
}

// direct calls the bridge the way the engine does, but with a token the test chooses, so it can
// cancel before, during and after a call. It returns the decoded payload and how long the call took.
func direct(s script, text string, token int64) (swiftbridge.TranslateSuccess, time.Duration, error) {
	buf := make([]byte, 1<<16)
	start := time.Now()
	n := swiftbridge.KaiTranslate(s.fromCode, s.toCode, text, token, unsafe.Pointer(&buf[0]), int32(len(buf)))
	d := time.Since(start)
	var tr swiftbridge.TranslateSuccess
	if n < 0 {
		return tr, d, fmt.Errorf("the bridge returned %d (output buffer)", n)
	}
	if err := json.Unmarshal(bytes.TrimRight(buf[:n], "\x00"), &tr); err != nil {
		return tr, d, fmt.Errorf("payload %q does not parse: %w", buf[:n], err)
	}
	return tr, d, nil
}

// (a) A request the old 20 s wait cut off now completes, through the engine the app uses.
func TestAppleNotCutOff(t *testing.T) {
	p := gatedProber(t)
	text, paragraphs := latin.buildInput(cancelSize)
	res, d, err := p.call(latin, text, pointCeiling)
	t.Logf("CANCEL a not_cut_off runes=%d latency_ms=%d err=%v", cancelSize, d.Milliseconds(), err)
	if err != nil {
		t.Fatalf("asserted: %d Latin runes must translate with no wait to cut them off, got: %v", cancelSize, err)
	}
	if m := firstMissingMarker(res.Result, paragraphs); m != 0 {
		t.Errorf("asserted: paragraph %d of %d is missing from the output (a silent truncation)", m, paragraphs)
	}
	if d < oldWait {
		t.Logf("CANCEL a note=the request finished in %s, inside the old %s wait: on this host it does not show the wait is gone", d.Round(time.Millisecond), oldWait)
	}
}

// (b) A ctx cancelled mid-request frees the engine's caller promptly, with the ctx's own error, and
// the bridge is told (the call returns the cancelled payload instead of running on to the end).
// (d) The request issued right after it is not lost: it queues behind the abandoned unit of work in
// the framework, which the bridge cannot shorten, so its latency is logged and not asserted.
func TestAppleCancelReachesSwift(t *testing.T) {
	p := gatedProber(t)
	text, _ := latin.buildInput(cancelSize)
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	var cancelledAt atomic.Int64
	timer := time.AfterFunc(cancelAfter, func() {
		cancelledAt.Store(time.Now().UnixNano())
		stop()
	})
	defer timer.Stop()

	start := time.Now()
	res, err := p.tr.Translate(ctx, model.TranslateRequest{Text: text, From: latin.from, To: latin.to})
	returned := time.Now()
	at := cancelledAt.Load()
	if at == 0 {
		t.Fatalf("Translate ended after %s, before the cancel was issued at %s (result=%v err=%v)", returned.Sub(start).Round(time.Millisecond), cancelAfter, res != nil, err)
	}
	lag := returned.Sub(time.Unix(0, at))
	t.Logf("CANCEL b cancel_after_ms=%d returned_after_ms=%d lag_ms=%d err=%v", cancelAfter.Milliseconds(), returned.Sub(start).Milliseconds(), lag.Milliseconds(), err)
	if err == nil {
		t.Fatalf("asserted: Translate returned a result after the cancel; want an error (the cancel never reached the bridge)")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("asserted: err = %v, want errors.Is(err, context.Canceled): a cancel is the ctx's cause, never error copy", err)
	}
	if lag > promptly {
		t.Errorf("asserted: Translate returned %s after the cancel, want within %s", lag.Round(time.Millisecond), promptly)
	}

	ctxD, stopD := context.WithTimeout(context.Background(), afterCancelGuard)
	defer stopD()
	startD := time.Now()
	resD, errD := p.tr.Translate(ctxD, model.TranslateRequest{Text: latin.warm, From: latin.from, To: latin.to})
	dD := time.Since(startD)
	t.Logf("CANCEL d next_request_after_cancel latency_ms=%d err=%v (queues behind the abandoned work; not asserted)", dD.Milliseconds(), errD)
	if errD != nil || resD == nil || resD.Result == "" {
		t.Fatalf("asserted: a one-sentence request right after a cancel must still succeed within %s, got: %v", afterCancelGuard, errD)
	}
}

// (c) A cancel that reaches the bridge before its call does is remembered: the call then returns the
// cancelled payload at once and translates nothing. The remembered cancel is consumed by that call,
// so the same token afterwards runs a normal translation.
func TestAppleCancelBeforeStart(t *testing.T) {
	gatedProber(t)
	const token = int64(9_000_001) // far above the engine's counter, which this process has barely used
	if got := swiftbridge.KaiTranslateCancel(token); got != 0 {
		t.Errorf("asserted: cancel of a token with no running call returned %d, want 0 (it leaves a tombstone)", got)
	}
	tr, d, err := direct(latin, latin.warm, token)
	t.Logf("CANCEL c before_start latency_ms=%d code=%q err=%v", d.Milliseconds(), tr.Code, err)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Code != swiftbridge.BridgeErrCancelled {
		t.Errorf("asserted: payload code = %q, want %q (result %q)", tr.Code, swiftbridge.BridgeErrCancelled, tr.Result)
	}
	if tr.Result != "" {
		t.Errorf("asserted: a call cancelled before it started translated %q", tr.Result)
	}
	if d >= promptly {
		t.Errorf("asserted: the cancelled call took %s, want under %s (nothing was translated)", d.Round(time.Millisecond), promptly)
	}

	tr2, d2, err := direct(latin, latin.warm, token)
	t.Logf("CANCEL c same_id_again latency_ms=%d code=%q result_len=%d err=%v", d2.Milliseconds(), tr2.Code, len(tr2.Result), err)
	if err != nil {
		t.Fatal(err)
	}
	if tr2.Code != "" || tr2.Result == "" {
		t.Errorf("asserted: the tombstone was not consumed: the same token again gave code=%q result=%q, want a translation", tr2.Code, tr2.Result)
	}
}

// The documented return values of kai_translate_cancel: 1 for a running call, 0 for one that is not
// (or already finished, or a token that can never be cancelled), and that a token of 0 registers
// nothing.
func TestAppleCancelReturnValues(t *testing.T) {
	gatedProber(t)
	const token = int64(9_000_002)
	text, _ := latin.buildInput(400) // a few seconds: long enough to cancel in the middle

	type answer struct {
		tr  swiftbridge.TranslateSuccess
		d   time.Duration
		err error
	}
	done := make(chan answer, 1)
	go func() {
		tr, d, err := direct(latin, text, token)
		done <- answer{tr, d, err}
	}()
	time.Sleep(500 * time.Millisecond) // the call is inside the bridge and registered by now
	if got := swiftbridge.KaiTranslateCancel(token); got != 1 {
		t.Errorf("asserted: cancel of a running call returned %d, want 1", got)
	}
	a := <-done
	t.Logf("CANCEL r running_call code=%q latency_ms=%d err=%v", a.tr.Code, a.d.Milliseconds(), a.err)
	if a.err != nil {
		t.Fatal(a.err)
	}
	if a.tr.Code != swiftbridge.BridgeErrCancelled {
		t.Errorf("asserted: the cancelled call returned code %q, want %q", a.tr.Code, swiftbridge.BridgeErrCancelled)
	}
	if a.d > 500*time.Millisecond+promptly {
		t.Errorf("asserted: the call took %s, want it freed within %s of the cancel", a.d.Round(time.Millisecond), promptly)
	}
	if got := swiftbridge.KaiTranslateCancel(token); got != 0 {
		t.Errorf("asserted: cancel of a call that already ended returned %d, want 0", got)
	}

	// A token of 0 or less registers nothing: it cannot be cancelled, and its call is not disturbed.
	zero := make(chan answer, 1)
	go func() {
		tr, d, err := direct(latin, latin.warm, 0)
		zero <- answer{tr, d, err}
	}()
	time.Sleep(300 * time.Millisecond)
	for _, bad := range []int64{0, -5} {
		if got := swiftbridge.KaiTranslateCancel(bad); got != 0 {
			t.Errorf("asserted: cancel of token %d returned %d, want 0", bad, got)
		}
	}
	z := <-zero
	t.Logf("CANCEL r zero_id code=%q latency_ms=%d result_len=%d err=%v", z.tr.Code, z.d.Milliseconds(), len(z.tr.Result), z.err)
	if z.err != nil || z.tr.Code != "" || z.tr.Result == "" {
		t.Errorf("asserted: a call with token 0 must run to a translation whatever cancels are sent, got code=%q result=%q err=%v", z.tr.Code, z.tr.Result, z.err)
	}
}
