package model

import (
	"reflect"
	"testing"
)

// issue #52 (Tester, RED): language model foundation — dialect variants, recognized vs
// selectable sets, and base aliasing. Contract settled in docs/handoffs/52/handoff-A.md
// (findings 1, 4): AllLanguages() = RECOGNIZED set; SelectableLanguages() = dropdown set;
// Language.Base() = alias to the base language (source-side / detection only).

func TestVariantConstantsHaveBCP47Values(t *testing.T) {
	cases := map[string]Language{"es-MX": ESMX, "pt": PT, "pt-BR": PTBR, "pt-PT": PTPT}
	for want, got := range cases {
		if string(got) != want {
			t.Errorf("constant value = %q, want %q", got, want)
		}
	}
}

// Issue #165 (principal, 2026-09-28): the selectable dropdowns lead with English, Spanish
// (Mexico), Portuguese (Portugal); families stay adjacent (ES/ESMX, PT/PTPT/PTBR).
func TestAllLanguagesIsRecognizedSetFamiliesAdjacent(t *testing.T) {
	want := []Language{Auto, EN, ES, ESMX, PT, PTPT, PTBR, JA, KO, FR, DE, RU, ZH}
	if got := AllLanguages(); !reflect.DeepEqual(got, want) {
		t.Fatalf("AllLanguages() = %v, want %v (recognized set, families adjacent)", got, want)
	}
}

func TestSelectableLanguagesExcludesBareEsPtKeepsOrder(t *testing.T) {
	want := []Language{Auto, EN, ESMX, PTPT, PTBR, JA, KO, FR, DE, RU, ZH}
	if got := SelectableLanguages(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SelectableLanguages() = %v, want %v", got, want)
	}
}

func TestSelectableIsSubsetOfRecognized(t *testing.T) {
	rec := map[Language]bool{}
	for _, l := range AllLanguages() {
		rec[l] = true
	}
	for _, l := range SelectableLanguages() {
		if !rec[l] {
			t.Errorf("selectable %q is not recognized", l)
		}
	}
}

func TestSelectableLanguagesReturnsCopy(t *testing.T) {
	a := SelectableLanguages()
	a[0] = "mutated"
	if SelectableLanguages()[0] == "mutated" {
		t.Fatal("SelectableLanguages() must return a defensive copy")
	}
}

func TestBaseAliasesVariantsToBaseLanguage(t *testing.T) {
	cases := map[Language]Language{
		ESMX: ES, "es-419": ES, ES: ES,
		PTBR: PT, PTPT: PT, PT: PT,
		EN: EN, ZH: ZH, Auto: Auto,
	}
	for in, want := range cases {
		if got := in.Base(); got != want {
			t.Errorf("Base(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSelectableOrCoercesBareBaseOnly(t *testing.T) {
	cases := map[Language]Language{
		ES: ESMX, PT: PTBR, ESMX: ESMX, PTPT: PTPT, Auto: Auto, RU: RU, "xx": "xx", "": "",
	}
	for in, want := range cases {
		if got := in.SelectableOr(ZH); got != want {
			t.Errorf("%q.SelectableOr(zh) = %q, want %q", in, got, want)
		}
	}
}

// #53: canonical-code parsing moves from engine.resolveLanguage to model so translate and the
// preference store share one normalization (no second case-folding table).
func TestParseLanguage(t *testing.T) {
	cases := []struct {
		in   string
		want Language
		ok   bool
	}{
		{"pt-br", PTBR, true},
		{"ES-mx", ESMX, true},
		{"zh_CN", ZH, true},
		{"es", ES, true},
		{"auto", "", false},
		{"xx", "", false},
	}
	for _, c := range cases {
		got, ok := ParseLanguage(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("ParseLanguage(%q) = (%q,%v), want (%q,%v)", c.in, got, ok, c.want, c.ok)
		}
	}
}
