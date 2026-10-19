package engine

import (
	"errors"
	"strings"
	"testing"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// Issue #10: the System engine's speed mode. The strategy value the Swift bridge takes (0 the
// default high-fidelity path, 1 Apple's low-latency strategy), the time estimate the progress
// line and the "offer the faster setting" hint read, and the rule that decides when to offer it.
// All pure functions of process state and arguments, testable on any OS.

// resetAppleSpeed restores the process-wide fast-mode switch around a test that flips it.
func resetAppleSpeed(t *testing.T) {
	t.Helper()
	SetAppleFastTranslate(false)
	t.Cleanup(func() { SetAppleFastTranslate(false) })
}

func TestAppleStrategyFollowsSetting(t *testing.T) {
	resetAppleSpeed(t)
	if got := appleStrategyInt32(); got != 0 {
		t.Fatalf("strategy = %d with the setting off, want 0 (today's default path)", got)
	}
	SetAppleFastTranslate(true)
	if got := appleStrategyInt32(); got != 1 {
		t.Fatalf("strategy = %d with the setting on, want 1 (low latency)", got)
	}
	SetAppleFastTranslate(false)
	if got := appleStrategyInt32(); got != 0 {
		t.Fatalf("strategy = %d after switching the setting off again, want 0", got)
	}
}

func TestEstimateAppleTranslateMs(t *testing.T) {
	resetAppleSpeed(t)
	latin := strings.Repeat("a", 1000)
	cjk := strings.Repeat("世", 1000)
	mixed := strings.Repeat("a", 500) + strings.Repeat("世", 500)
	cases := []struct {
		name string
		fast bool
		text string
		want int64
	}{
		{"empty slow", false, "", 0},
		{"empty fast", true, "", 0},
		{"latin slow", false, latin, 9000},
		{"latin fast", true, latin, 800},
		{"cjk slow", false, cjk, 30000},
		{"cjk fast", true, cjk, 2700},
		{"mixed slow", false, mixed, 19500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			SetAppleFastTranslate(c.fast)
			if got := EstimateAppleTranslateMs(c.text); got != c.want {
				t.Fatalf("EstimateAppleTranslateMs(%q) = %d ms, want %d ms", c.name, got, c.want)
			}
		})
	}
}

func TestEstimateAppleTranslateMsCountsRunesNotBytes(t *testing.T) {
	resetAppleSpeed(t)
	// 世 is 3 bytes: 100 runes of it must price as 100 CJK runes (3000 ms), not 300 (9000 ms).
	if got := EstimateAppleTranslateMs(strings.Repeat("世", 100)); got != 3000 {
		t.Fatalf("estimate for 100 CJK runes = %d ms, want 3000 ms", got)
	}
}

func TestSuggestFastSystemTranslate(t *testing.T) {
	resetAppleSpeed(t)
	// 20,000 Latin runes at the slow rate is 180 s: over the "a minute or two" line.
	long := strings.Repeat("a", 20000)
	// 2,000 Latin runes at the slow rate is 18 s: well under it.
	short := strings.Repeat("a", 2000)
	if SuggestFastSystemTranslate("google", long) {
		t.Fatal("a non-Apple engine must never be told to offer the System speed setting")
	}
	SetAppleFastTranslate(true)
	if SuggestFastSystemTranslate("apple", long) {
		t.Fatal("the setting already on means there is nothing to suggest")
	}
	SetAppleFastTranslate(false)
	if SuggestFastSystemTranslate("apple", short) {
		t.Fatal("a text that translates in seconds does not warrant the suggestion")
	}
	if !SuggestFastSystemTranslate("apple", long) {
		t.Fatal("a slow-mode text over the threshold must be suggested the setting")
	}
}

func TestSuggestFastSystemTranslateThreshold(t *testing.T) {
	resetAppleSpeed(t)
	// Around the threshold ("more than a minute or two" reads as at-or-over two minutes):
	// 13,334 Latin runes price at 120,006 ms (over — suggested); one rune fewer at 119,997 ms
	// (under — not suggested).
	over := strings.Repeat("a", 13334)
	under := strings.Repeat("a", 13333)
	if got := EstimateAppleTranslateMs(over); got < suggestFastTranslateAfterMs {
		t.Fatalf("over-fixture estimates %d ms, want at least the %d ms threshold", got, suggestFastTranslateAfterMs)
	}
	if !SuggestFastSystemTranslate("apple", over) {
		t.Fatal("a text at or just over the threshold must be suggested the setting")
	}
	if got := EstimateAppleTranslateMs(under); got >= suggestFastTranslateAfterMs {
		t.Fatalf("under-fixture estimates %d ms, want under the %d ms threshold", got, suggestFastTranslateAfterMs)
	}
	if SuggestFastSystemTranslate("apple", under) {
		t.Fatal("a text just under the threshold must not be suggested the setting")
	}
}

func TestAppleProgressHint(t *testing.T) {
	resetAppleSpeed(t)
	long := strings.Repeat("a", 20000)
	if ms, suggest := AppleProgressHint("google", long); ms != 0 || suggest {
		t.Fatalf("AppleProgressHint(google) = (%d, %v), want (0, false): only the System engine is estimated", ms, suggest)
	}
	ms, suggest := AppleProgressHint("apple", long)
	if ms != 180000 || !suggest {
		t.Fatalf("AppleProgressHint(apple, 20k latin) = (%d, %v), want (180000, true)", ms, suggest)
	}
	SetAppleFastTranslate(true)
	if _, suggest := AppleProgressHint("apple", long); suggest {
		t.Fatal("no suggestion when the fast setting is already on")
	}
}

func TestAppleCJKRunes(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"abc", 0},
		{"世界", 2},
		{"a世b界c", 2},
		{"こんにちは", 5},      // Hiragana counts as CJK-priced
		{"한국어", 3},        // Hangul counts as CJK-priced
		{"\U00020000", 1}, // CJK Extension B (astral plane)
		{"a  b\nc", 0},    // whitespace and punctuation price as Latin
	}
	for _, c := range cases {
		if got := appleCJKRunes(c.text); got != c.want {
			t.Errorf("appleCJKRunes(%q) = %d, want %d", c.text, got, c.want)
		}
	}
}

// ---- the darwin wiring's decision logic, as pure functions (issue #10, PR review) ----------
//
// apple_darwin.go builds only on macOS; these tests pin the DECISIONS it used to carry inline —
// the fallback retry, the entry-point choice, the warm choice — by injecting the bridge calls,
// so they run (and fail) on any OS. apple_darwin.go keeps only thin closures over swiftbridge.

// speedCallRecorder records the strategies one injected bridge call saw, and answers per
// strategy from scripts.
type speedCallRecorder struct {
	calls   []int32
	scripts map[int32]*swiftbridge.TranslateSuccess
	errs    map[int32]error
}

func (r *speedCallRecorder) call(strategy int32) (*swiftbridge.TranslateSuccess, error) {
	r.calls = append(r.calls, strategy)
	if err, ok := r.errs[strategy]; ok {
		return nil, err
	}
	res, ok := r.scripts[strategy]
	if !ok {
		return nil, errors.New("unexpected strategy call")
	}
	return res, nil
}

func (r *speedCallRecorder) strategies() []int32 { return r.calls }

func TestTranslateWithSpeedModeFastOk(t *testing.T) {
	resetAppleSpeed(t)
	SetAppleFastTranslate(true)
	fast := &swiftbridge.TranslateSuccess{Result: "hola"}
	r := &speedCallRecorder{scripts: map[int32]*swiftbridge.TranslateSuccess{1: fast}}
	res, notice, err := translateWithSpeedMode(alwaysTrue, r.call)
	if err != nil || notice != "" || res != fast {
		t.Fatalf("res=%v notice=%q err=%v, want the fast payload, no notice, no error", res, notice, err)
	}
	if got := r.strategies(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("strategies = %v, want exactly [1]", got)
	}
}

func TestTranslateWithSpeedModeFallbackTable(t *testing.T) {
	packMissing := func() *swiftbridge.TranslateSuccess {
		return &swiftbridge.TranslateSuccess{BridgeError: swiftbridge.BridgeError{Code: swiftbridge.BridgeErrLowLatencyPackMissing}}
	}
	okSlow := &swiftbridge.TranslateSuccess{Result: "hola lenta"}
	otherErr := &swiftbridge.TranslateSuccess{BridgeError: swiftbridge.BridgeError{Code: swiftbridge.BridgeErrAppleTranslate}}
	cases := []struct {
		name       string
		fast       bool
		scripts    map[int32]*swiftbridge.TranslateSuccess
		errs       map[int32]error
		wantCalls  []int32
		wantRes    *swiftbridge.TranslateSuccess // nil: compare by code below
		wantCode   string
		wantNotice bool
		wantErr    error
	}{
		{
			name:       "fast pack-missing falls back to slow ok with the notice",
			fast:       true,
			scripts:    map[int32]*swiftbridge.TranslateSuccess{1: packMissing(), 0: okSlow},
			wantCalls:  []int32{1, 0},
			wantRes:    okSlow,
			wantNotice: true,
		},
		{
			name:      "fast pack-missing and slow pack-missing ends as the code (the caller's error path), exactly two calls",
			fast:      true,
			scripts:   map[int32]*swiftbridge.TranslateSuccess{1: packMissing(), 0: packMissing()},
			wantCalls: []int32{1, 0},
			wantCode:  swiftbridge.BridgeErrLowLatencyPackMissing,
		},
		{
			name:      "pack-missing at strategy 0 is not retried",
			fast:      false,
			scripts:   map[int32]*swiftbridge.TranslateSuccess{0: packMissing()},
			wantCalls: []int32{0},
			wantCode:  swiftbridge.BridgeErrLowLatencyPackMissing,
		},
		{
			name:      "any other code passes through with one call",
			fast:      true,
			scripts:   map[int32]*swiftbridge.TranslateSuccess{1: otherErr},
			wantCalls: []int32{1},
			wantCode:  swiftbridge.BridgeErrAppleTranslate,
		},
		{
			name:      "a call error is returned unchanged with one call",
			fast:      true,
			errs:      map[int32]error{1: errFastCallBoom},
			wantCalls: []int32{1},
			wantErr:   errFastCallBoom,
		},
		{
			name:      "an error from the slow retry ends the request (never a third call)",
			fast:      true,
			scripts:   map[int32]*swiftbridge.TranslateSuccess{1: packMissing()},
			errs:      map[int32]error{0: errSlowRetryBoom},
			wantCalls: []int32{1, 0},
			wantErr:   errSlowRetryBoom,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resetAppleSpeed(t)
			SetAppleFastTranslate(c.fast)
			r := &speedCallRecorder{scripts: c.scripts, errs: c.errs}
			res, notice, err := translateWithSpeedMode(alwaysTrue, r.call)
			if got := r.strategies(); !slicesEqual(got, c.wantCalls) {
				t.Fatalf("strategies = %v, want %v", got, c.wantCalls)
			}
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if c.wantErr != nil {
				return
			}
			if c.wantRes != nil && res != c.wantRes {
				t.Fatalf("res = %+v, want the slow payload %+v", res, c.wantRes)
			}
			if c.wantCode != "" && (res == nil || res.Code != c.wantCode) {
				t.Fatalf("res = %+v, want code %q", res, c.wantCode)
			}
			if c.wantNotice && notice != i18n.T("err.apple_low_latency_pack_missing") {
				t.Fatalf("notice = %q, want the err.apple_low_latency_pack_missing copy", notice)
			}
			if !c.wantNotice && notice != "" {
				t.Fatalf("notice = %q, want empty (no fallback success)", notice)
			}
		})
	}
}

var (
	errFastCallBoom  = errors.New("fast call failed")
	errSlowRetryBoom = errors.New("slow retry failed")
)

func alwaysTrue() bool { return true }

// A request that is no longer alive (cancelled / superseded) must not pay the slow retry: the
// code path apple_darwin.go encodes as ctx.Err() == nil.
func TestTranslateWithSpeedModeDeadRequestDoesNotRetry(t *testing.T) {
	resetAppleSpeed(t)
	SetAppleFastTranslate(true)
	packMissing := &swiftbridge.TranslateSuccess{BridgeError: swiftbridge.BridgeError{Code: swiftbridge.BridgeErrLowLatencyPackMissing}}
	r := &speedCallRecorder{scripts: map[int32]*swiftbridge.TranslateSuccess{1: packMissing}}
	res, notice, err := translateWithSpeedMode(func() bool { return false }, r.call)
	if err != nil || notice != "" || res != packMissing {
		t.Fatalf("res=%v notice=%q err=%v, want the pack-missing payload passed through", res, notice, err)
	}
	if got := r.strategies(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("strategies = %v, want exactly [1] (no retry on a dead request)", got)
	}
}

// The entry-point choice: the strategy symbol when the dylib has it, the old kai_translate
// otherwise (an older build: the fast mode silently unavailable).
func TestTranslateEntryFor(t *testing.T) {
	var plainGot struct {
		called bool
		args   []string
	}
	plain := func(src, dst, text string, token int64, out unsafe.Pointer, outCap int32) int32 {
		plainGot.called = true
		plainGot.args = []string{src, dst, text}
		return 7
	}
	var stratGot struct {
		strategy int32
		called   bool
	}
	strat := func(src, dst, text string, token int64, strategy int32, out unsafe.Pointer, outCap int32) int32 {
		stratGot.called = true
		stratGot.strategy = strategy
		return 9
	}
	// Both symbols: the strategy entry is used as-is, strategy passed through.
	entry, degraded := translateEntryFor(strat, plain)
	if entry("en", "fr", "x", 3, 1, nil, 16) != 9 || !stratGot.called || stratGot.strategy != 1 {
		t.Fatal("with the strategy symbol present it must be called with the strategy")
	}
	if degraded {
		t.Fatal("degraded must be false when the strategy symbol is present")
	}
	// Older dylib (nil strategy symbol): the plain entry answers, the strategy is dropped.
	entry, degraded = translateEntryFor(nil, plain)
	if entry("en", "fr", "x", 3, 1, nil, 16) != 7 || !plainGot.called {
		t.Fatal("with the strategy symbol missing the old entry must answer")
	}
	if plainGot.args[0] != "en" || plainGot.args[2] != "x" {
		t.Fatalf("plain entry saw %v, want the (src, dst, text) it always took", plainGot.args)
	}
	if !degraded {
		t.Fatal("degraded must be true when the strategy symbol is missing")
	}
}

// WarmTranslate's entry choice: the strategy warm only when the setting is on AND the symbol
// exists; otherwise the plain warm of the default session.
func TestWarmEntryFor(t *testing.T) {
	var plainCalled, stratCalled bool
	var stratStrategy int32
	plain := func(src, dst string) int32 { plainCalled = true; return 1 }
	strat := func(src, dst string, strategy int32) int32 { stratCalled = true; stratStrategy = strategy; return 1 }
	warmEntryFor(true, strat, plain)("en", "fr")
	if !stratCalled || stratStrategy != 1 || plainCalled {
		t.Fatal("fast on + symbol present: the strategy warm must run with strategy 1")
	}
	plainCalled, stratCalled = false, false
	warmEntryFor(true, nil, plain)("en", "fr")
	if !plainCalled || stratCalled {
		t.Fatal("fast on but older dylib: the plain warm must run")
	}
	plainCalled, stratCalled = false, false
	warmEntryFor(false, strat, plain)("en", "fr")
	if !plainCalled || stratCalled {
		t.Fatal("fast off: the plain warm must run even though the symbol exists")
	}
}

func slicesEqual(a, b []int32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
