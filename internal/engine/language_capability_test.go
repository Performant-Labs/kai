package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// issue #52 (Tester, RED): per-engine language capability registry (handoff-A findings 2, 3, 7).
//
// Contract T assumes (F must provide, in internal/engine):
//   type LanguageCapability struct { Supported bool; Code string }
//   func LookupLanguage(engineName string, l model.Language) (LanguageCapability, bool)
//        // bool=false means NO explicit decision recorded for that language.
//   func SupportedTargets(engineName string) []model.Language
//        // selectable, non-auto targets the engine supports, in SelectableLanguages() order.

func translatorNames() []string {
	var out []string
	for _, m := range KnownEngines() {
		if m.Kind == KindTranslator {
			out = append(out, m.Name)
		}
	}
	return out
}

// Exhaustiveness: CI fails when a new language lacks a capability decision for any engine.
func TestRegistryHasDecisionForEveryTranslatorAndLanguage(t *testing.T) {
	names := translatorNames()
	if len(names) == 0 {
		t.Fatal("no translator engines found")
	}
	for _, name := range names {
		for _, l := range model.AllLanguages() {
			if l == model.Auto {
				continue
			}
			c, ok := LookupLanguage(name, l)
			if !ok {
				t.Errorf("engine %q has no capability decision for language %q", name, l)
				continue
			}
			if c.Supported && c.Code == "" {
				t.Errorf("engine %q supports %q but has an empty request code", name, l)
			}
		}
	}
}

func TestLLMEnginesSupportAllVariantTargets(t *testing.T) {
	for _, name := range []string{"openai", "anthropic", "gemini"} {
		got := SupportedTargets(name)
		for _, v := range []model.Language{model.ESMX, model.PTBR, model.PTPT} {
			if !slices.Contains(got, v) {
				t.Errorf("%s should support target %q, got %v", name, v, got)
			}
		}
	}
}

func TestEnginesWithoutVariantsDisableThemAsTargets(t *testing.T) {
	for _, name := range []string{"baidu", "tencent", "youdao"} {
		got := SupportedTargets(name)
		for _, v := range []model.Language{model.ESMX, model.PTBR, model.PTPT} {
			if slices.Contains(got, v) {
				t.Errorf("%s must not offer variant target %q (no target degradation), got %v", name, v, got)
			}
		}
		if len(got) == 0 {
			t.Errorf("%s should still support its base targets", name)
		}
	}
}

// Verified values (principal ruling 2026-09-25): apple honors all three dialects (macOS 27
// evidence); google serves es-MX as its default Spanish and honors pt-BR/pt-PT; deepl has no
// es-MX (refused) but PT-BR/PT-PT.
func TestVerifiedDialectCapabilities(t *testing.T) {
	want := map[string]map[model.Language]bool{
		"apple":  {model.ESMX: true, model.PTBR: true, model.PTPT: true},
		"google": {model.ESMX: true, model.PTBR: true, model.PTPT: true},
		"deepl":  {model.ESMX: false, model.PTBR: true, model.PTPT: true},
	}
	for name, byLang := range want {
		got := SupportedTargets(name)
		for l, sup := range byLang {
			if slices.Contains(got, l) != sup {
				t.Errorf("%s target %q supported=%v, want %v (targets %v)", name, l, !sup, sup, got)
			}
		}
	}
	if c, _ := LookupLanguage("google", model.ESMX); c.Code != "es" {
		t.Errorf("google es-MX request code = %q, want es", c.Code)
	}
	if c, _ := LookupLanguage("google", model.PTPT); c.Code != "pt-PT" {
		t.Errorf("google pt-PT request code = %q, want pt-PT", c.Code)
	}
}

// A recognized target an engine has no entry for is refused before any request is built.
func TestUnsupportedTargetRefused(t *testing.T) {
	i18n.SetLocale("en-US")
	t.Cleanup(func() { i18n.SetLocale("zh-CN") })
	cases := []struct {
		name string
		fn   func(string) (string, error)
		to   string
	}{
		{"baidu", baiduTarget, "es-MX"}, {"baidu", baiduTarget, "pt-BR"}, {"baidu", baiduTarget, "pt-PT"},
		{"tencent", tencentTarget, "es-MX"}, {"tencent", tencentTarget, "pt-BR"},
		{"youdao", youdaoTarget, "pt-PT"}, {"youdao", youdaoTarget, "es-MX"},
		{"deepl", deeplTarget, "es-MX"},
	}
	for _, c := range cases {
		code, err := c.fn(c.to)
		if err == nil {
			t.Errorf("%s target %q: want refusal, got code %q", c.name, c.to, code)
			continue
		}
		if !strings.Contains(err.Error(), c.name) || strings.Contains(err.Error(), "err.engine_unsupported_target") {
			t.Errorf("%s target %q: message %q should be rendered and name the engine", c.name, c.to, err)
		}
	}
	if code, err := deeplTarget("pt-PT"); err != nil || code != "PT-PT" {
		t.Errorf("deeplTarget(pt-PT) = %q, %v; want PT-PT", code, err)
	}
	if code, err := baiduTarget("es"); err != nil || code != "spa" {
		t.Errorf("baiduTarget(es) = %q, %v; want spa", code, err)
	}
}

func TestGoogleDialectTargetSentExactly(t *testing.T) {
	for to, want := range map[model.Language]string{model.PTPT: "pt-PT", model.PTBR: "pt-BR", model.ESMX: "es"} {
		var q url.Values
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q = r.URL.Query()
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[[["x","Hello","","","0"]],null,"en"]`))
		}))
		tr := NewGoogle(srv.URL, http.DefaultClient)
		_, err := tr.Translate(context.Background(), model.TranslateRequest{Text: "Hello", From: model.EN, To: to})
		srv.Close()
		if err != nil {
			t.Fatalf("Translate to %q: %v", to, err)
		}
		if got := q.Get("tl"); got != want {
			t.Errorf("tl for %q = %q, want %q", to, got, want)
		}
	}
}

func TestSupportedTargetsAreSelectableAndNeverAuto(t *testing.T) {
	sel := model.SelectableLanguages()
	for _, name := range translatorNames() {
		for _, l := range SupportedTargets(name) {
			if l == model.Auto || l == model.ES || l == model.PT {
				t.Errorf("%s: SupportedTargets contains non-selectable/auto %q", name, l)
			}
			if !slices.Contains(sel, l) {
				t.Errorf("%s: %q not in SelectableLanguages()", name, l)
			}
		}
	}
	if got := SupportedTargets("no-such-engine"); len(got) != 0 {
		t.Errorf("unknown engine should have no targets, got %v", got)
	}
}

// LLM prompt phrasing (finding 7): variants must be named, not collapsed to "auto"/raw code.
func TestLLMLanguageNamesCoverVariants(t *testing.T) {
	// Names follow the UI locale; pin en-US (package default is zh-CN) and restore.
	i18n.SetLocale("en-US")
	t.Cleanup(func() { i18n.SetLocale("zh-CN") })
	cases := map[string]string{"es-MX": "Mexico", "pt-BR": "Brazil", "pt-PT": "Portugal"}
	for code, frag := range cases {
		if got := dstName(code); !strings.Contains(got, frag) {
			t.Errorf("dstName(%q) = %q, want it to mention %q", code, got, frag)
		}
		if got := srcName(code); got == srcName("auto") || !strings.Contains(got, frag) {
			t.Errorf("srcName(%q) = %q, want a named variant mentioning %q (not the auto label)", code, got, frag)
		}
	}
	if got := srcName("es"); got == srcName("auto") {
		t.Errorf("srcName(es) collapsed to the auto label: %q", got)
	}
}

// Source side accepts variants on engines lacking them: alias to base, no case-mangled codes.
func TestBaiduSourceVariantAliasesToBase(t *testing.T) {
	if got := baiduLang("es-MX"); got != "spa" {
		t.Errorf("baiduLang(es-MX) = %q, want spa (base alias)", got)
	}
	if got := baiduLang("pt-BR"); got != "pt" {
		t.Errorf("baiduLang(pt-BR) = %q, want pt (base alias, not %q)", got, "pt-br")
	}
}

// google: real loopback (#38 pattern). A variant SOURCE is sent as its base code.
func TestGoogleVariantSourceSentAsBase(t *testing.T) {
	var q url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[[["Hello","Hola","","","0"]],null,"es"]`))
	}))
	defer srv.Close()

	tr := NewGoogle(srv.URL, http.DefaultClient)
	if _, err := tr.Translate(context.Background(), model.TranslateRequest{Text: "Hola", From: model.ESMX, To: model.EN}); err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if got := q.Get("sl"); got != "es" {
		t.Errorf("sl = %q, want es (variant source aliased to base)", got)
	}
}
