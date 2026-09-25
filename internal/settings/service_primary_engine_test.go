package settings

import (
	"testing"
)

// issue #8 (Tester role, RED): persistence round-trip of the default_engine config field.
//
// Real data directory (t.TempDir) + real settings.json file, no mocks:
// after saving, a fresh Service instance re-reads from disk and default_engine must
// round-trip unchanged.
//
// RED note: the Settings struct has no DefaultEngine field (json "default_engine") yet, and
// writeConfig/setDefaults don't serialize it either, so this file fails at compile time
// (cfg.DefaultEngine undefined). This is a "missing behavior" RED — not an environment or
// typo problem.

func TestDefaultEngineConfigRoundtrip(t *testing.T) {
	dir := t.TempDir()

	svc, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	cfg := svc.Get()
	cfg.DefaultEngine = "google"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Fresh instance re-reads from disk: default_engine must round-trip from settings.json.
	svc2, err := NewService(dir)
	if err != nil {
		t.Fatalf("second NewService: %v", err)
	}
	if got := svc2.Get().DefaultEngine; got != "google" {
		t.Fatalf("default_engine disk roundtrip failed: want %q, got %q (settings service did not persist default_engine)", "google", got)
	}
}
