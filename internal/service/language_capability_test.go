package service

import (
	"slices"
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/model"
)

// issue #52 (Tester, RED): capability is backend-owned and delivered to the frontend via
// AllEngineItem.TargetLanguages (json target_languages) — handoff-A finding 1; the dropdown
// list (GetLanguages) is the SELECTABLE set — finding 4.

func TestGetAllEnginesExposesTargetLanguagesFromRegistry(t *testing.T) {
	_, _, w := setupPrimaryEnv(t, []*engine.EngineConfig{
		{Engine: "openai", Enabled: true, APIKey: "k", Endpoint: "http://127.0.0.1:1"},
		{Engine: "baidu", Enabled: true, APIKey: "k", Secret: "s"},
	})
	byName := map[string]AllEngineItem{}
	for _, it := range w.GetAllEngines() {
		byName[it.Value] = it
	}
	llm, ok := byName["openai"]
	if !ok {
		t.Fatal("openai missing from GetAllEngines")
	}
	if !slices.Contains(llm.TargetLanguages, model.PTBR) {
		t.Errorf("openai TargetLanguages should include pt-BR, got %v", llm.TargetLanguages)
	}
	bd, ok := byName["baidu"]
	if !ok {
		t.Fatal("baidu missing from GetAllEngines")
	}
	if slices.Contains(bd.TargetLanguages, model.PTBR) || len(bd.TargetLanguages) == 0 {
		t.Errorf("baidu TargetLanguages should be non-empty and exclude pt-BR, got %v", bd.TargetLanguages)
	}
	if !slices.Equal(llm.TargetLanguages, engine.SupportedTargets("openai")) {
		t.Errorf("TargetLanguages must come from the engine registry, got %v", llm.TargetLanguages)
	}
}

func TestGetLanguagesReturnsSelectableSetOnly(t *testing.T) {
	var cw ConfigWrapper
	var got []string
	for _, it := range cw.GetLanguages("en-US") {
		got = append(got, it.Value)
	}
	var want []string
	for _, l := range model.SelectableLanguages() {
		want = append(want, string(l))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("GetLanguages values = %v, want SelectableLanguages %v", got, want)
	}
	if slices.Contains(got, "es") || slices.Contains(got, "pt") {
		t.Errorf("bare es/pt must not be selectable: %v", got)
	}
}
