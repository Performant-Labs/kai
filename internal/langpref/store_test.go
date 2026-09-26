package langpref

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// Contract pinned by T-red for #53 (new package, depends only on model):
//   New() *Store; (*Store).Learn(model.Language); Qualify(model.Language) model.Language;
//   Reset(). Safe for concurrent use. Preferences are keyed base -> variant.

func TestQualifyReturnsLearnedVariantForBase(t *testing.T) {
	s := New()
	s.Learn(model.ESMX)
	if got := s.Qualify(model.ES); got != model.ESMX {
		t.Errorf("Qualify(es) = %q, want es-MX", got)
	}
}

func TestQualifyNormalizesAliasBeforeLookup(t *testing.T) {
	s := New()
	s.Learn(model.ESMX)
	if got := s.Qualify(model.Language("es-419")); got != model.ESMX {
		t.Errorf("Qualify(es-419) = %q, want es-MX", got)
	}
}

func TestQualifyWithoutPreferenceReturnsBase(t *testing.T) {
	s := New()
	if got := s.Qualify(model.ES); got != model.ES {
		t.Errorf("Qualify(es) = %q, want es", got)
	}
	// Alias with no preference collapses to its base, never to a guessed variant.
	if got := s.Qualify(model.Language("es-419")); got != model.ES {
		t.Errorf("Qualify(es-419) = %q, want es", got)
	}
}

func TestQualifyLeavesOtherFamiliesAlone(t *testing.T) {
	s := New()
	s.Learn(model.ESMX)
	for _, l := range []model.Language{model.PT, model.FR, model.Auto} {
		if got := s.Qualify(l); got != l {
			t.Errorf("Qualify(%q) = %q, want unchanged", l, got)
		}
	}
}

// Portuguese: prefs apply only after an explicit pt-BR / pt-PT choice; bare pt never teaches.
func TestLearnIgnoresBasesAndNonVariants(t *testing.T) {
	s := New()
	for _, l := range []model.Language{model.PT, model.ES, model.Auto, model.FR, model.Language("")} {
		s.Learn(l)
	}
	if got := s.Qualify(model.PT); got != model.PT {
		t.Errorf("Qualify(pt) = %q, want pt (no pref learned from bases)", got)
	}
	if got := s.Qualify(model.ES); got != model.ES {
		t.Errorf("Qualify(es) = %q, want es", got)
	}
	s.Learn(model.PTPT)
	if got := s.Qualify(model.PT); got != model.PTPT {
		t.Errorf("Qualify(pt) after pt-PT = %q, want pt-PT", got)
	}
}

// Last explicit selection wins within a family (pt-BR then pt-PT).
func TestLastExplicitSelectionWins(t *testing.T) {
	s := New()
	s.Learn(model.PTBR)
	s.Learn(model.PTPT)
	if got := s.Qualify(model.PT); got != model.PTPT {
		t.Errorf("Qualify(pt) = %q, want pt-PT", got)
	}
}

func TestLearnNormalizesCase(t *testing.T) {
	s := New()
	s.Learn(model.Language("pt-br"))
	if got := s.Qualify(model.PT); got != model.PTBR {
		t.Errorf("Qualify(pt) = %q, want pt-BR", got)
	}
}

// Session scoping: Reset empties prefs; separate stores are independent.
func TestResetAndIsolation(t *testing.T) {
	a, b := New(), New()
	a.Learn(model.ESMX)
	if got := b.Qualify(model.ES); got != model.ES {
		t.Errorf("fresh store leaked pref: %q", got)
	}
	a.Reset()
	if got := a.Qualify(model.ES); got != model.ES {
		t.Errorf("Qualify(es) after Reset = %q, want es", got)
	}
}
