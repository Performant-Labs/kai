package engine

import (
	"sync/atomic"
	"unicode"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// The System engine's speed mode (issue #10). Apple's Translation.framework gained a strategy
// switch in macOS 26.4: the default path runs the Apple Intelligence models (fluent, slow —
// measured 8.2–8.4 ms per Latin character on an M1 Max), `.lowLatency` runs the traditional
// language-pack models (0.6–0.8 ms per character, about 12x faster, lower quality). Kai exposes
// that switch as the "faster System translation (lower quality)" setting, off by default; this
// file holds the engine's process-wide copy of that setting and the pure math built on it, so it
// all runs and is tested on any OS. The Swift half (session creation with the strategy, the
// missing-language-pack error) lives in pkg/swiftbridge/internal/swift/apple_translate.swift.

// appleFastTranslate is the process-wide copy of settings.FastSystemTranslate: false (the zero
// value) is today's behaviour exactly. Set at wiring time and on every settings change
// (engine.SetAppleFastTranslate), read per call. An atomic, not a plain bool, because the
// settings hot-reload callback and translating goroutines race.
var appleFastTranslate atomic.Bool

// SetAppleFastTranslate syncs the "faster System translation" setting into the engine (issue
// #10). main.go calls it at startup and from the settings OnChange callback; false keeps the
// default high-fidelity path.
func SetAppleFastTranslate(on bool) {
	appleFastTranslate.Store(on)
}

// appleStrategyInt32 is the strategy the Swift bridge's kai_translate_strategy takes: 0 the
// default (no strategy set — today's Apple Intelligence path), 1 low latency. It is an int32
// because that is the C ABI the bridge exports.
func appleStrategyInt32() int32 {
	if appleFastTranslate.Load() {
		return 1
	}
	return 0
}

// The measured per-rune prices of one System-engine translation, in milliseconds. Sources:
// slow (default / high fidelity): docs/engine-limits.md #119, "about 8-9 ms per rune for Latin
// scripts and about 22-31 ms per rune for CJK and Japanese source text" — the upper end is kept
// so the estimate errs on the long side; fast (low latency): issue #10's measurement on the same
// M1 Max, 0.6–0.8 ms per Latin character, upper end kept, and the CJK figure scaled by the same
// ~11x ratio (unmeasured in the fast mode — a projection, and it only feeds a hint that says
// "about").
const (
	appleSlowMsPerLatinRune = 9.0
	appleSlowMsPerCJKRune   = 30.0
	appleFastMsPerLatinRune = 0.8
	appleFastMsPerCJKRune   = 2.7
)

// suggestFastTranslateAfterMs is how long a slow-mode System translation must be estimated to
// take before the progress line offers the faster setting (issue #10: "when a text is long
// enough to take more than a minute or two"). Two minutes.
const suggestFastTranslateAfterMs = 120_000

// appleCJKRunes counts the runes of text that price at the CJK rate: the Han, Hiragana, Katakana
// and Hangul blocks, script by script, so a mixed Latin/CJK text prices each rune at its own
// rate (the chunker's sepAfter makes the same distinction).
func appleCJKRunes(text string) int {
	n := 0
	for _, r := range text {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			n++
		}
	}
	return n
}

// EstimateAppleTranslateMs estimates one System-engine translation of text in milliseconds, at
// the measured per-rune rate of the mode currently set (issue #10). Rune counts, not bytes: a
// CJK character is 3 bytes but one rune. Zero for empty text. It is a hint for humans ("about 3
// minutes left"), never a limit: nothing is cancelled or refused when it is wrong.
func EstimateAppleTranslateMs(text string) int64 {
	if text == "" {
		return 0
	}
	runes := 0
	for range text {
		runes++
	}
	cjk := appleCJKRunes(text)
	latin := runes - cjk
	if appleFastTranslate.Load() {
		return int64(float64(latin)*appleFastMsPerLatinRune + float64(cjk)*appleFastMsPerCJKRune)
	}
	return int64(float64(latin)*appleSlowMsPerLatinRune + float64(cjk)*appleSlowMsPerCJKRune)
}

// SuggestFastSystemTranslate reports whether a translation that is about to run on the engine
// named engineName should offer the "faster System translation" setting (issue #10): the System
// engine, the setting currently off (never suggest switching what is already on, and never
// switch it silently), and an estimate of at least suggestFastTranslateAfterMs at the current
// (slow) rate.
func SuggestFastSystemTranslate(engineName, text string) bool {
	if engineName != "apple" || appleFastTranslate.Load() {
		return false
	}
	return EstimateAppleTranslateMs(text) >= suggestFastTranslateAfterMs
}

// AppleProgressHint is the one call the translate service makes per engine start (issue #10):
// the estimated duration of this engine's work in milliseconds (0 when there is nothing to
// estimate — every engine but the System engine) and whether the progress line should offer the
// faster-setting switch. Both ride the started progress event; the window owns how (and whether)
// to show them.
func AppleProgressHint(engineName, text string) (estimatedMs int64, suggestFast bool) {
	if engineName != "apple" {
		return 0, false
	}
	return EstimateAppleTranslateMs(text), SuggestFastSystemTranslate(engineName, text)
}

// ---- the darwin wiring's decisions, pure so they run (and fail) on any OS ---------------------
//
// apple_darwin.go builds only on macOS, so every decision it needs lives here as a function of
// injected bridge calls (PR review: reading its source with strings.Contains verified nothing).
// The darwin file's job is only to close these over the real swiftbridge symbols.

// The bridge entry points' shapes (pkg/swiftbridge's KaiTranslateStrategy / KaiTranslate).
type (
	appleTranslateEntry = func(src, dst, text string, token int64, strategy int32, out unsafe.Pointer, outCap int32) int32
	appleTranslatePlain = func(src, dst, text string, token int64, out unsafe.Pointer, outCap int32) int32
)

// translateEntryFor picks the kai_translate entry point for a strategy call (issue #10): the
// strategy-taking symbol when the loaded dylib has it; otherwise the old kai_translate, wrapped
// so the strategy parameter is simply dropped (an older dylib serves the default path — the fast
// mode is unavailable until the bridge is rebuilt, which degraded reports for a Debug log).
func translateEntryFor(strategyEntry appleTranslateEntry, plainEntry appleTranslatePlain) (entry appleTranslateEntry, degraded bool) {
	if strategyEntry != nil {
		return strategyEntry, false
	}
	return func(src, dst, text string, token int64, _ int32, out unsafe.Pointer, outCap int32) int32 {
		return plainEntry(src, dst, text, token, out, outCap)
	}, true
}

// warmEntryFor picks the kai_warm_translate entry point (issue #10): the strategy warm with
// strategy 1 when the setting is on AND the dylib has the symbol; otherwise the plain warm of
// the default session (the setting off, or an older dylib).
func warmEntryFor(
	fast bool,
	strategyEntry func(src, dst string, strategy int32) int32,
	plainEntry func(src, dst string) int32,
) func(src, dst string) int32 {
	if fast && strategyEntry != nil {
		return func(src, dst string) int32 { return strategyEntry(src, dst, 1) }
	}
	return plainEntry
}

// translateWithSpeedMode runs one System-engine translation through call at the strategy the
// setting picks, applying the missing-pack rule of issue #10: when the fast mode's traditional
// language pack is not installed (the bridge answers the fixed low_latency_pack_missing code),
// the same text is retried ONCE with strategy 0 — the slow mode — instead of failing.
//
// call receives the strategy and returns that call's parsed payload, or a call error (a dead
// ctx, the buffer, a parse failure), which is returned unchanged and never retried. canRetry
// says whether the request is still alive (apple_darwin.go closes over ctx.Err() == nil): a
// request that ended while the fast attempt answered does not pay the slow retry.
//
// The returned notice is the localized err.apple_low_latency_pack_missing copy, set only when
// the fallback ran AND the slow retry succeeded — it rides the successful result as its Notice.
// A slow retry that answers an error code of its own is returned as that payload with no notice
// and no third call (the caller's ordinary error path renders it); a pack-missing payload at
// strategy 0 (the setting off) is passed through unretried the same way.
func translateWithSpeedMode(
	canRetry func() bool,
	call func(strategy int32) (*swiftbridge.TranslateSuccess, error),
) (res *swiftbridge.TranslateSuccess, notice string, err error) {
	strategy := appleStrategyInt32()
	res, err = call(strategy)
	if err != nil {
		return nil, "", err
	}
	if strategy != 0 && res.Code == swiftbridge.BridgeErrLowLatencyPackMissing && canRetry() {
		res, err = call(0)
		if err != nil {
			return nil, "", err
		}
		if res.Code == "" {
			return res, i18n.T("err.apple_low_latency_pack_missing"), nil
		}
		return res, "", nil
	}
	return res, "", nil
}
