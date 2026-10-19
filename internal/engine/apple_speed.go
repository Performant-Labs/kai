package engine

import (
	"sync/atomic"
	"unicode"
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
