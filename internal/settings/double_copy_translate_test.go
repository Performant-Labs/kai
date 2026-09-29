package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// Issue #199: the "translate on double Cmd+C" setting is OFF by default (it needs the Input
// Monitoring permission and changes global behaviour). Unlike auto_switch_source, only an explicit
// true turns it on: a missing key, or a value that is not a boolean, reads as off.

func TestDoubleCopyTranslateDefaultsOff(t *testing.T) {
	if DefaultSettings().DoubleCopyTranslate {
		t.Fatal("DefaultSettings().DoubleCopyTranslate = true, want false")
	}
	if loadWith(t, "").DoubleCopyTranslate {
		t.Fatal("a fresh install reads the setting as on, want off")
	}
}

func TestDoubleCopyTranslateMissingKeyReadsOff(t *testing.T) {
	if loadWith(t, `{"language":"en-US","default_to":"en"}`).DoubleCopyTranslate {
		t.Fatal("a settings.json without the key reads as on, want off")
	}
}

func TestDoubleCopyTranslateGarbledValueReadsOff(t *testing.T) {
	for _, body := range []string{
		`{"double_copy_translate":"maybe"}`,
		`{"double_copy_translate":null}`,
		`{"double_copy_translate":{"x":1}}`,
		`{"double_copy_translate":7}`,
		`{"double_copy_translate":[true]}`,
	} {
		if loadWith(t, body).DoubleCopyTranslate {
			t.Errorf("%s reads as on, want off", body)
		}
	}
}

func TestDoubleCopyTranslateExplicitTrueAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"double_copy_translate":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !svc.Get().DoubleCopyTranslate {
		t.Fatal("explicit true reads as off")
	}
	// The startup re-save keeps it on, a second load reads it back.
	svc2, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !svc2.Get().DoubleCopyTranslate {
		t.Fatal("true did not survive the save/load round trip")
	}
	svc2.Get().DoubleCopyTranslate = false
	if err := svc2.Save(); err != nil {
		t.Fatal(err)
	}
	svc3, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if svc3.Get().DoubleCopyTranslate {
		t.Fatal("false did not survive the save/load round trip")
	}
}

func TestDoubleCopyAndAutoSwitchAreIndependent(t *testing.T) {
	s := loadWith(t, `{"auto_switch_source":false,"double_copy_translate":true}`)
	if s.AutoSwitchSource || !s.DoubleCopyTranslate {
		t.Fatalf("auto_switch_source=%v double_copy_translate=%v", s.AutoSwitchSource, s.DoubleCopyTranslate)
	}
}
