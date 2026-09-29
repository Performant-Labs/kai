package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// Issue #200: the "auto switch source language" setting is ON by default, and a settings.json
// written before it existed (key missing), or holding a value that is not a boolean, reads as ON.
// Only an explicit false turns it off.

func loadWith(t *testing.T, body string) *Settings {
	t.Helper()
	dir := t.TempDir()
	if body != "" {
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	svc, err := NewService(dir)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc.Get()
}

func TestAutoSwitchSourceDefaultsOn(t *testing.T) {
	if !DefaultSettings().AutoSwitchSource {
		t.Fatal("DefaultSettings().AutoSwitchSource = false, want true")
	}
	if !loadWith(t, "").AutoSwitchSource {
		t.Fatal("a fresh install reads the setting as off, want on")
	}
}

func TestAutoSwitchSourceMissingKeyReadsOn(t *testing.T) {
	// An old settings.json: valid, but written before the key existed.
	if !loadWith(t, `{"language":"en-US","default_to":"en"}`).AutoSwitchSource {
		t.Fatal("a settings.json without the key reads as off, want on")
	}
}

func TestAutoSwitchSourceUnknownValueReadsOn(t *testing.T) {
	for _, body := range []string{
		`{"auto_switch_source":"maybe"}`,
		`{"auto_switch_source":null}`,
		`{"auto_switch_source":{"x":1}}`,
		`{"auto_switch_source":7}`,
	} {
		if !loadWith(t, body).AutoSwitchSource {
			t.Errorf("%s reads as off, want on", body)
		}
	}
}

func TestAutoSwitchSourceExplicitFalseAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"auto_switch_source":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if svc.Get().AutoSwitchSource {
		t.Fatal("explicit false reads as on")
	}
	// The startup re-save must keep it off, and a second load must read the file back as off.
	svc2, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if svc2.Get().AutoSwitchSource {
		t.Fatal("false did not survive the save/load round trip")
	}
	svc2.Get().AutoSwitchSource = true
	if err := svc2.Save(); err != nil {
		t.Fatal(err)
	}
	svc3, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !svc3.Get().AutoSwitchSource {
		t.Fatal("true did not survive the save/load round trip")
	}
}
