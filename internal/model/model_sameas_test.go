package model

import "testing"

// Issue #80: Language.SameAs is the strict identity rule for "same source and target language".
// It is deliberately NOT Normalize() equality (Normalize folds pt-BR and pt-PT to pt).
func TestLanguageSameAs(t *testing.T) {
	cases := []struct {
		a, b Language
		want bool
	}{
		{"es", "es-MX", true},
		{"es-MX", "es", true},
		{"es-MX", "es-MX", true},
		{"zh", "zh-CN", true},
		{"zh-CN", "zh", true},
		{"en", "en", true},
		{"ES", "es-mx", true},
		{"pt", "pt-BR", true},
		{"es_MX", "es-MX", true},
		{"xx-YY", "xx-yy", true}, // identical unrecognized codes
		{"pt-BR", "pt-PT", false},
		{"pt-PT", "pt-BR", false},
		{"es-MX", "es-ES", false},
		{"es-MX", "es-419", false},
		{"es", "es-ES", false},
		{"en", "fr", false},
		{"", "", false},
		{"", "en", false},
		{"en", "", false},
		{Auto, Auto, false},
		{Auto, "en", false},
		{"en", Auto, false},
		{"AUTO", "auto", false},
	}
	for _, c := range cases {
		if got := c.a.SameAs(c.b); got != c.want {
			t.Errorf("%q.SameAs(%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// Pins why SameAs exists: Normalize equality would call these dialect pairs identical.
func TestNormalizeAloneWouldConflateDialects(t *testing.T) {
	if PTBR.Normalize() != PTPT.Normalize() {
		t.Fatal("premise changed: Normalize no longer folds pt-BR and pt-PT together")
	}
	if PTBR.SameAs(PTPT) {
		t.Error("pt-BR.SameAs(pt-PT) = true; SameAs must not be Normalize equality")
	}
}

// Issue #80: Language.Covers compares a detected language (a) against the target (b). A bare
// detection covers any dialect of it; a dialect detection covers only that dialect.
func TestLanguageCovers(t *testing.T) {
	cases := []struct {
		det, target Language
		want        bool
	}{
		{"es", "es-MX", true},
		{"es", "es-419", true},
		{"es", "es-ES", true},
		{"pt", "pt-BR", true},
		{"zh", "zh-CN", true},
		{"en", "en", true},
		{"pt-BR", "pt-PT", false},
		{"es-MX", "es-ES", false},
		{"es-MX", "es-419", false},
		{"es", "zh", false},
		{"en", "", false},
		{Auto, "en", false},
	}
	for _, c := range cases {
		if got := c.det.Covers(c.target); got != c.want {
			t.Errorf("%q.Covers(%q) = %v, want %v", c.det, c.target, got, c.want)
		}
	}
}
