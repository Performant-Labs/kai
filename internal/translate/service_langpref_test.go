package translate

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/langpref"
	"cnb.cool/dtapp/kai/internal/model"
)

// Contract pinned by T-red for #53: translate.Service takes a *langpref.Store via
// SetLangPrefs (setter injection, like SetConfigStore); nil/unset means no qualification.

func translateAuto(t *testing.T, svc *Service, from model.Language) model.Language {
	t.Helper()
	res, err := svc.Translate(model.TranslateRequest{Text: "Hello", From: from, To: model.EN, EngineName: "google"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	return res.From
}

// Acceptance: after one explicit es-MX selection, an auto-detected Spanish send is qualified.
func TestAutoDetectedSpanishQualifiedByPreference(t *testing.T) {
	store := langpref.New()
	store.Learn(model.ESMX)
	svc := newLoopbackService(t, `"es"`)
	svc.SetLangPrefs(store)
	if got := translateAuto(t, svc, model.Auto); got != model.ESMX {
		t.Errorf("From = %q, want es-MX", got)
	}
}

// Detection is normalized through the alias table before qualification (es-419 -> es -> es-MX).
func TestDetectedAliasNormalizedThenQualified(t *testing.T) {
	store := langpref.New()
	store.Learn(model.ESMX)
	svc := newLoopbackService(t, `"es-419"`)
	svc.SetLangPrefs(store)
	if got := translateAuto(t, svc, model.Auto); got != model.ESMX {
		t.Errorf("From = %q, want es-MX", got)
	}
}

// No preference: the base language is reported as detected, unqualified.
func TestDetectedSpanishStaysBaseWithoutPreference(t *testing.T) {
	svc := newLoopbackService(t, `"es"`)
	svc.SetLangPrefs(langpref.New())
	if got := translateAuto(t, svc, model.Auto); got != model.ES {
		t.Errorf("From = %q, want es", got)
	}
}

// A preference for one family never touches another language.
func TestPreferenceDoesNotQualifyUnrelatedDetection(t *testing.T) {
	store := langpref.New()
	store.Learn(model.ESMX)
	svc := newLoopbackService(t, `"fr"`)
	svc.SetLangPrefs(store)
	if got := translateAuto(t, svc, model.Auto); got != model.FR {
		t.Errorf("From = %q, want fr", got)
	}
}

// Explicit overrides preference: an explicit source is used as-is even with a stored pref.
func TestExplicitSourceOverridesPreference(t *testing.T) {
	store := langpref.New()
	store.Learn(model.PTBR)
	svc := newLoopbackService(t, `"pt"`)
	svc.SetLangPrefs(store)
	if got := translateAuto(t, svc, model.PTPT); got != model.PTPT {
		t.Errorf("From = %q, want pt-PT (explicit wins)", got)
	}
}
