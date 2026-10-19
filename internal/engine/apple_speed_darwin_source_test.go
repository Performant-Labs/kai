package engine

import (
	"os"
	"strings"
	"testing"
)

// Issue #10, the darwin half of the System engine's speed mode. apple_darwin.go builds only on
// macOS, and this file's assertions are TEXT checks of its source, exactly like
// pkg/swiftbridge/swift_source_test.swift's guard on the Swift half: they run everywhere, so a
// wiring mistake cannot land silently. What the real framework does with a low-latency session
// is verified on a Mac (see the PR's manual verification list).

func readAppleDarwin(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("apple_darwin.go")
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestAppleDarwinUsesStrategyEntry: Translate must call kai_translate_strategy with the
// strategy the setting picked (appleStrategyInt32), and an older dylib without the symbol must
// fall back to the old kai_translate instead of failing.
func TestAppleDarwinUsesStrategyEntry(t *testing.T) {
	src := readAppleDarwin(t)
	if !strings.Contains(src, "strategyEntry := swiftbridge.KaiTranslateStrategy") ||
		!strings.Contains(src, "strategyEntry(sl, tl, text, token, strategy") {
		t.Error("apple_darwin.go must call KaiTranslateStrategy with (sl, tl, text, token, strategy)")
	}
	if !strings.Contains(src, "strategy := appleStrategyInt32()") {
		t.Error("the strategy must come from appleStrategyInt32(), the setting's engine-side value")
	}
	if !strings.Contains(src, "strategyEntry == nil") {
		t.Error("a dylib without kai_translate_strategy (an older build) must fall back to KaiTranslate, not fail")
	}
}

// TestAppleDarwinLowLatencyFallback: a low_latency_pack_missing payload must be retried once
// with the slow strategy (strategy 0) and the successful result must carry the
// err.apple_low_latency_pack_missing notice, so the user is told in plain words what happened
// and how to install the pack (issue #10).
func TestAppleDarwinLowLatencyFallback(t *testing.T) {
	src := readAppleDarwin(t)
	if !strings.Contains(src, "swiftbridge.BridgeErrLowLatencyPackMissing") {
		t.Error("apple_darwin.go does not handle the low_latency_pack_missing code")
	}
	if !strings.Contains(src, "s.translateViaBridge(ctx, sl, tl, text, 0, outBuf)") {
		t.Error("the missing-pack path must retry the same text with strategy 0 (the slow mode)")
	}
	if !strings.Contains(src, `i18n.T("err.apple_low_latency_pack_missing")`) {
		t.Error("the fallback's notice copy must be err.apple_low_latency_pack_missing (the plain-words install hint)")
	}
}

// TestAppleDarwinWarmStrategy: WarmTranslate must warm the session the setting will use —
// kai_warm_translate_strategy when the fast mode is on and the dylib has the symbol.
func TestAppleDarwinWarmStrategy(t *testing.T) {
	src := readAppleDarwin(t)
	if !strings.Contains(src, "swiftbridge.KaiWarmTranslateStrategy(sl, tl, appleStrategyInt32())") {
		t.Error("WarmTranslate must warm the strategy the fast-mode setting selected")
	}
}
