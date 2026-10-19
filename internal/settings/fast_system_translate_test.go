package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// Issue #10: the "faster System translation (lower quality)" setting is OFF by default, so
// nothing changes for anyone who does not choose it. Like double_copy_translate, only an
// explicit true turns it on: a missing key, or a value that is not a boolean, reads as off.

func TestFastSystemTranslateDefaultsOff(t *testing.T) {
	if DefaultSettings().FastSystemTranslate {
		t.Fatal("DefaultSettings().FastSystemTranslate = true, want false")
	}
	if loadWith(t, "").FastSystemTranslate {
		t.Fatal("a fresh install reads the setting as on, want off")
	}
}

func TestFastSystemTranslateMissingKeyReadsOff(t *testing.T) {
	if loadWith(t, `{"language":"en-US","default_to":"en"}`).FastSystemTranslate {
		t.Fatal("a settings.json without the key reads as on, want off")
	}
}

func TestFastSystemTranslateGarbledValueReadsOff(t *testing.T) {
	for _, body := range []string{
		`{"fast_system_translate":"maybe"}`,
		`{"fast_system_translate":null}`,
		`{"fast_system_translate":{"x":1}}`,
		`{"fast_system_translate":7}`,
		`{"fast_system_translate":[true]}`,
	} {
		if loadWith(t, body).FastSystemTranslate {
			t.Errorf("%s reads as on, want off", body)
		}
	}
}

func TestFastSystemTranslateExplicitTrueAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"fast_system_translate":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !svc.Get().FastSystemTranslate {
		t.Fatal("explicit true reads as off")
	}
	// The startup re-save keeps it on, a second load reads it back.
	svc2, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !svc2.Get().FastSystemTranslate {
		t.Fatal("true did not survive the save/load round trip")
	}
	svc2.Get().FastSystemTranslate = false
	if err := svc2.Save(); err != nil {
		t.Fatal(err)
	}
	svc3, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if svc3.Get().FastSystemTranslate {
		t.Fatal("false did not survive the save/load round trip")
	}
}
