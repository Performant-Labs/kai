package settings

import "testing"

// issue #52: a persisted bare es/pt (recognized, no longer selectable) is coerced to the first
// selectable variant of its family when the settings load; unknown codes are untouched.
// Real data dir + real settings.json: save the legacy value, reload with a fresh Service.
func TestPersistedBareLanguagesCoercedOnLoad(t *testing.T) {
	cases := []struct{ from, to, wantFrom, wantTo string }{
		{"es", "pt", "es-MX", "pt-BR"},
		{"auto", "es", "auto", "es-MX"},
		{"pt-PT", "fr", "pt-PT", "fr"},
		{"it", "it", "it", "it"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		svc, err := NewService(dir)
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}
		cfg := svc.Get()
		cfg.DefaultFrom, cfg.DefaultTo = c.from, c.to
		if err := svc.Save(); err != nil {
			t.Fatalf("Save: %v", err)
		}
		svc2, err := NewService(dir)
		if err != nil {
			t.Fatalf("second NewService: %v", err)
		}
		got := svc2.Get()
		if got.DefaultFrom != c.wantFrom || got.DefaultTo != c.wantTo {
			t.Errorf("load from=%q to=%q: got from=%q to=%q, want from=%q to=%q",
				c.from, c.to, got.DefaultFrom, got.DefaultTo, c.wantFrom, c.wantTo)
		}
	}
}

// issue #44: a fresh install's default target is English, on disk as well as in memory (the
// startup re-save writes the default, so it must be en, not the old zh trap).
func TestFreshInstallDefaultTargetIsEnglish(t *testing.T) {
	if got := DefaultSettings().DefaultTo; got != "en" {
		t.Errorf("DefaultSettings().DefaultTo = %q, want en", got)
	}
	dir := t.TempDir()
	if _, err := NewService(dir); err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc2, err := NewService(dir)
	if err != nil {
		t.Fatalf("reload NewService: %v", err)
	}
	if got := svc2.Get().DefaultTo; got != "en" {
		t.Errorf("fresh config, reloaded from disk: DefaultTo = %q, want en", got)
	}
}

// issue #44: an explicit persisted zh target is honored on load (the runtime guard, not a
// migration, protects against X->X).
func TestPersistedChineseTargetIsNotMigrated(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.Get().DefaultTo = "zh"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	svc2, err := NewService(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := svc2.Get().DefaultTo; got != "zh" {
		t.Errorf("persisted zh became %q, want zh untouched", got)
	}
}
