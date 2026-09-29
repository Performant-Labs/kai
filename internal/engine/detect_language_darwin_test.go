//go:build darwin

package engine

import (
	"testing"
	"time"

	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// Issue #200: the real NaturalLanguage detector through the real Swift bridge (the dylib the test
// command's build.sh step produces). Skipped, loudly, where the dylib cannot load.
func TestDetectLanguageRealBridge(t *testing.T) {
	if err := swiftbridge.Init(""); err != nil || !swiftbridge.Available() {
		t.Skipf("Swift bridge not loadable: %v", err)
	}
	if swiftbridge.KaiDetectLanguage == nil {
		t.Fatal("kai_detect_language is not registered")
	}
	for _, tc := range []struct {
		text string
		want string
	}{
		{"Hola, ¿cómo estás? Necesito que me ayudes con este documento hoy.", "es"},
		{"The quick brown fox jumps over the lazy dog again and again.", "en"},
		{"Bonjour tout le monde, ceci est un test de détection de langue.", "fr"},
		{"今天天气很好，我们一起去公园散步吧，然后回家吃饭。", "zh-Hans"},
	} {
		start := time.Now()
		lang, conf, ok := DetectLanguage(tc.text)
		took := time.Since(start)
		if !ok || string(lang) != tc.want || conf < 0.8 {
			t.Errorf("%q: got (%q, %.3f, %v), want %s with confidence >= 0.8", tc.text, lang, conf, ok, tc.want)
		}
		t.Logf("detect %-8s conf=%.3f in %s (through purego)", lang, conf, took)
	}
	// Nonsense must not come back confident.
	if lang, conf, ok := DetectLanguage("12345 67890 !!!! ????? ..... 12345"); ok && conf >= 0.8 {
		t.Errorf("digits and punctuation detected as %q with confidence %.2f", lang, conf)
	}
}
