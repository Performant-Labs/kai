package swiftbridge

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"unsafe"
)

// Issue #10: the System engine's speed mode, Swift half. Like swift_source_test.go this is a
// TEXT check of the Swift sources, not a behaviour test: this machine's CI has no Swift compiler
// and no Translation framework, so the source text is the one guard that runs everywhere. The
// strategy's behaviour against the real framework is verified on a Mac (see the PR's manual
// verification list).

// Compile-time contract: the strategy-taking entry points exist with these exact signatures on
// every OS (load.go on macOS, load_other.go elsewhere).
var (
	_ func(src, dst, text string, token int64, strategy int32, out unsafe.Pointer, outCap int32) int32 = KaiTranslateStrategy
	_ func(src, dst string, strategy int32) int32                                                      = KaiWarmTranslateStrategy
)

// cdeclBody returns the text of the @_cdecl named symbol up to the next @_cdecl.
func cdeclBody(t *testing.T, src, symbol string) string {
	t.Helper()
	i := strings.Index(src, `@_cdecl("`+symbol+`")`)
	if i < 0 {
		t.Fatalf("@_cdecl(\"%s\") not found", symbol)
	}
	rest := src[i:]
	if j := strings.Index(rest[1:], "@_cdecl"); j >= 0 {
		rest = rest[:j+1]
	}
	return rest
}

// TestLowLatencyPackMissingLiteralsMatch: the missing-language-pack code exists on both sides
// with the same literal (issue #10). The bridge returns it instead of translating when the fast
// mode's traditional language pack is not installed; the Go engine falls back to the slow mode
// and shows the copy this key names.
func TestLowLatencyPackMissingLiteralsMatch(t *testing.T) {
	m := regexp.MustCompile(`BRIDGE_ERR_LOW_LATENCY_PACK_MISSING\s*:\s*String\s*=\s*"([^"]*)"`).
		FindStringSubmatch(readSwift(t, "bridge_errors.swift"))
	if m == nil {
		t.Fatal("bridge_errors.swift declares no BRIDGE_ERR_LOW_LATENCY_PACK_MISSING")
	}
	if m[1] != BridgeErrLowLatencyPackMissing {
		t.Errorf("Swift BRIDGE_ERR_LOW_LATENCY_PACK_MISSING = %q, Go BridgeErrLowLatencyPackMissing = %q: literals must match", m[1], BridgeErrLowLatencyPackMissing)
	}
	if BridgeErrLowLatencyPackMissing != "low_latency_pack_missing" {
		t.Errorf("BridgeErrLowLatencyPackMissing = %q, want \"low_latency_pack_missing\"", BridgeErrLowLatencyPackMissing)
	}
}

// TestTranslateStrategyEntryPoint: kai_translate_strategy is the strategy-taking form of
// kai_translate (issue #10). kai_translate itself must keep its old C signature and delegate
// with strategy 0, so a dylib built from these sources answers both symbols and an older Go
// binary still works.
func TestTranslateStrategyEntryPoint(t *testing.T) {
	src := readSwift(t, "apple_translate.swift")
	body := cdeclBody(t, src, "kai_translate_strategy")
	if !strings.Contains(body, "_ strategy: Int32") {
		t.Error("kai_translate_strategy must take a strategy: Int32 parameter (0 default, 1 low latency)")
	}
	old := cdeclBody(t, src, "kai_translate")
	if strings.Contains(old, "TranslationSession") {
		t.Error("kai_translate must be a thin wrapper (no session logic): the body lives in kai_translate_strategy's path so the two entry points cannot drift")
	}
	if !strings.Contains(old, "kaiTranslateImpl") || !strings.Contains(old, "strategy: 0") {
		t.Error("kai_translate must delegate to the shared implementation with strategy: 0")
	}
}

// TestSessionCacheKeyIncludesStrategy: one cached session per (pair, strategy) (issue #10): the
// fast and the slow mode must not share a TranslationSession, and switching the setting must not
// reuse a session built for the other strategy.
func TestSessionCacheKeyIncludesStrategy(t *testing.T) {
	src := readSwift(t, "apple_translate.swift")
	if !strings.Contains(src, "strategyKey") || !strings.Contains(src, `#\(lowLatency ? 1 : 0)`) {
		t.Error("TranslationSessionCache's key must append the strategy (a \"#0\"/\"#1\" suffix via strategyKey), so one session is cached per (pair, strategy)")
	}
	if !strings.Contains(src, "lowLatency: Bool") {
		t.Error("SessionEntry must hold the strategy as a plain lowLatency: Bool (the 26.4-only TranslationSession.Strategy type is touched only inside #available guards, so older SDKs still compile the file)")
	}
}

// TestLowLatencyPackCheck: before translating with the fast strategy the bridge must ask
// LanguageAvailability(preferredStrategy: .lowLatency) and answer the fixed
// low_latency_pack_missing code when the pair is not installed (issue #10) — a code, never
// copy, so the Go side can fall back to the slow mode for this call. The strategy switch itself
// is macOS 26.4+ (docs/research/apple-translation-speed-grok-2026-09-30.md): an older OS keeps
// the default session instead of failing.
func TestLowLatencyPackCheck(t *testing.T) {
	src := readSwift(t, "apple_translate.swift")
	body := cdeclBody(t, src, "kai_translate_strategy")
	if !strings.Contains(body, "LanguageAvailability(preferredStrategy: .lowLatency)") {
		t.Error("the fast path must check LanguageAvailability(preferredStrategy: .lowLatency) before translating")
	}
	if !strings.Contains(body, "BRIDGE_ERR_LOW_LATENCY_PACK_MISSING") {
		t.Error("a not-installed pair must be reported as the fixed low_latency_pack_missing code")
	}
	if !strings.Contains(body, "#available(macOS 26.4, *)") {
		t.Error("the strategy switch is macOS 26.4+; an older OS must fall back to the default session, not fail")
	}
}

// TestWarmStrategyEntryPoint: kai_warm_translate_strategy warms the session the setting will
// actually use (issue #10); kai_warm_translate keeps its old signature and delegates with
// strategy 0.
func TestWarmStrategyEntryPoint(t *testing.T) {
	src := readSwift(t, "apple_translate.swift")
	body := cdeclBody(t, src, "kai_warm_translate_strategy")
	if !strings.Contains(body, "_ strategy: Int32") {
		t.Error("kai_warm_translate_strategy must take a strategy: Int32 parameter")
	}
	old := cdeclBody(t, src, "kai_warm_translate")
	if !strings.Contains(old, "kaiWarmTranslateImpl") || !strings.Contains(old, "strategy: 0") {
		t.Error("kai_warm_translate must delegate to the shared implementation with strategy: 0")
	}
}

// TestSpeedEntryPointsRegistered: both new symbols are registered by the loader and stubbed on
// the other OSes, exactly like kai_detect_language (issue #200).
func TestSpeedEntryPointsRegistered(t *testing.T) {
	load, err := os.ReadFile("load.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"kai_translate_strategy", "kai_warm_translate_strategy"} {
		if !strings.Contains(string(load), `register(&KaiTranslateStrategy, "`+name+`")`) &&
			!strings.Contains(string(load), `"`+name+`"`) {
			t.Errorf("load.go does not register %s", name)
		}
	}
	other, err := os.ReadFile("load_other.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"KaiTranslateStrategy", "KaiWarmTranslateStrategy"} {
		if !strings.Contains(string(other), name) {
			t.Errorf("load_other.go has no %s stub", name)
		}
	}
}
