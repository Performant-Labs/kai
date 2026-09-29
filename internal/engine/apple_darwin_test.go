//go:build darwin

package engine

import "testing"

// Issue #173 item 8, added in response to a PR review finding: WarmTranslate's normalizing/
// skip logic (which source/target pairs are actually worth warming) had no test. Pulled out
// as warmTranslatePair, a pure function with no swiftbridge dependency, so it is testable
// without a loaded dylib.

func TestWarmTranslatePair_SkipsAutoSource(t *testing.T) {
	if _, _, ok := warmTranslatePair("auto", "en"); ok {
		t.Fatal("auto source should not be warmable")
	}
	if _, _, ok := warmTranslatePair("", "en"); ok {
		t.Fatal("empty source should not be warmable")
	}
}

func TestWarmTranslatePair_SkipsEmptyOrAutoTarget(t *testing.T) {
	if _, _, ok := warmTranslatePair("en", ""); ok {
		t.Fatal("empty target should not be warmable")
	}
	if _, _, ok := warmTranslatePair("en", "auto"); ok {
		t.Fatal("auto target should not be warmable")
	}
}

// Every model.Language constant the app defines is currently present in the apple engine's
// capability registry (internal/engine/language_capability.go), so there is no real-world
// "recognized language, unsupported by apple" target today — normalizeTarget's error path
// exists for engines whose registry is a strict subset (see its own doc comment). An
// unrecognized string (not a model.Language at all) falls through targetCode's identity
// fallback instead of erroring, which is intentional existing behavior shared by every
// engine, not a warmTranslatePair bug — so it is NOT treated as "unsupported" here: it is
// passed through, and the Swift side harmlessly no-ops on a code it cannot parse.
func TestWarmTranslatePair_PassesThroughAnUnrecognizedTargetCode(t *testing.T) {
	sl, tl, ok := warmTranslatePair("en", "not-a-real-language-code")
	if !ok {
		t.Fatal("an unrecognized (not erroring) target code should still be warmable")
	}
	if sl == "" || tl == "" {
		t.Fatalf("expected non-empty normalized codes, got sl=%q tl=%q", sl, tl)
	}
}

func TestWarmTranslatePair_NormalizesAValidPair(t *testing.T) {
	sl, tl, ok := warmTranslatePair("es-MX", "en")
	if !ok {
		t.Fatal("es-MX -> en should be warmable")
	}
	if sl == "" || sl == "auto" {
		t.Fatalf("unexpected normalized source: %q", sl)
	}
	if tl == "" || tl == "auto" {
		t.Fatalf("unexpected normalized target: %q", tl)
	}
}

func TestWarmTranslatePair_NeverReturnsLiteralAuto(t *testing.T) {
	// Regression guard for the doc comment's claim: "auto" must never be passed through as
	// the literal string to the Swift bridge.
	if sl, _, ok := warmTranslatePair("auto", "en"); ok || sl == "auto" {
		t.Fatalf("must not pass through the literal \"auto\" source, got sl=%q ok=%v", sl, ok)
	}
	if _, tl, ok := warmTranslatePair("en", "auto"); ok || tl == "auto" {
		t.Fatalf("must not pass through the literal \"auto\" target, got tl=%q ok=%v", tl, ok)
	}
}

// WarmTranslate itself must never panic, with or without a loaded dylib — main.go calls it
// unconditionally in a goroutine at launch, on every platform/build.
func TestWarmTranslate_NeverPanics(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("WarmTranslate panicked: %v", r)
		}
	}()
	WarmTranslate("auto", "en")
	WarmTranslate("es-MX", "en")
	WarmTranslate("", "")
}
