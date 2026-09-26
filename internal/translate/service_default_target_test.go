package translate

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/settings"
)

// Issue #44 (PR-Agent finding): the screenshot flow has no language bar, so defaultTarget() is the
// one place its requested target comes from. It used to be three hard-coded model.ZH literals.
// These pin the decision without needing a screen capture.

func TestDefaultTargetWithoutSettingsIsThePolicyDefaultNotChinese(t *testing.T) {
	s := &Service{}
	got := s.defaultTarget()
	if got == model.ZH {
		t.Fatalf("defaultTarget() = zh: the old hard-coded Chinese target is back")
	}
	if got != settings.DefaultTarget {
		t.Fatalf("defaultTarget() = %q, want the policy default %q", got, settings.DefaultTarget)
	}
}

func TestDefaultTargetOfAFreshInstallIsEnglish(t *testing.T) {
	st, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatalf("settings.NewService: %v", err)
	}
	s := &Service{settings: st}
	if got := s.defaultTarget(); got != model.EN {
		t.Fatalf("defaultTarget() on a fresh install = %q, want en", got)
	}
}

func TestDefaultTargetHonorsTheSavedTarget(t *testing.T) {
	dir := t.TempDir()
	st, err := settings.NewService(dir)
	if err != nil {
		t.Fatalf("settings.NewService: %v", err)
	}
	st.Get().DefaultTo = "fr"
	if err := st.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	st2, err := settings.NewService(dir)
	if err != nil {
		t.Fatalf("reopen settings: %v", err)
	}
	s := &Service{settings: st2}
	if got := s.defaultTarget(); got != model.Language("fr") {
		t.Fatalf("defaultTarget() = %q, want the saved fr", got)
	}
}
