package service

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/langpref"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #53: LangPrefWrapper.Learn is the binding the language selects call. If it stopped
// reaching the store the translate service qualifies with, picking es-MX would silently teach
// nothing, so these pin that a Learn through the wrapper is visible to the shared store.

func TestLangPrefWrapperLearnTeachesTheSharedStore(t *testing.T) {
	store := langpref.New()
	w := NewLangPrefWrapper(store)

	if got := store.Qualify(model.ES); got != model.ES {
		t.Fatalf("precondition: Qualify(es) before any selection = %q, want es", got)
	}
	w.Learn(model.ESMX)
	if got := store.Qualify(model.ES); got != model.ESMX {
		t.Errorf("after Learn(es-MX) through the wrapper, Qualify(es) = %q, want es-MX", got)
	}
}

func TestLangPrefWrapperLastExplicitSelectionWins(t *testing.T) {
	store := langpref.New()
	w := NewLangPrefWrapper(store)

	w.Learn(model.PTBR)
	w.Learn(model.PTPT)
	if got := store.Qualify(model.PT); got != model.PTPT {
		t.Errorf("Learn(pt-BR) then Learn(pt-PT): Qualify(pt) = %q, want pt-PT", got)
	}
}

func TestLangPrefWrapperIgnoresBasesAndLanguagesWithoutDialects(t *testing.T) {
	store := langpref.New()
	w := NewLangPrefWrapper(store)

	w.Learn(model.ESMX)
	w.Learn(model.ES) // a bare base is not a preference and must not erase the learned variant
	w.Learn(model.EN)
	if got := store.Qualify(model.ES); got != model.ESMX {
		t.Errorf("Qualify(es) = %q, want es-MX still (a base selection teaches nothing)", got)
	}
	if got := store.Qualify(model.EN); got != model.EN {
		t.Errorf("Qualify(en) = %q, want en (no dialects)", got)
	}
}
