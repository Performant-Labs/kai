package engine

import (
	"strings"
	"testing"
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
