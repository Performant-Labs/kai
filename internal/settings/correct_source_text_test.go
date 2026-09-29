package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// Issue #208: the "correct grammar and wording" setting is OFF by default. A settings.json written
// before it existed (key missing), or holding a value that is not a boolean, reads as OFF: only an
// explicit true turns it on.

func TestCorrectSourceTextDefaultsOff(t *testing.T) {
	if DefaultSettings().CorrectSourceText {
		t.Fatal("DefaultSettings().CorrectSourceText = true, want false")
	}
	if loadWith(t, "").CorrectSourceText {
		t.Fatal("a fresh install reads the setting as on, want off")
	}
}

func TestCorrectSourceTextMissingKeyReadsOff(t *testing.T) {
	if loadWith(t, `{"language":"en-US","default_to":"en"}`).CorrectSourceText {
		t.Fatal("a settings.json without the key reads as on, want off")
	}
}

func TestCorrectSourceTextGarbledValueReadsOff(t *testing.T) {
	for _, body := range []string{
		`{"correct_source_text":"maybe"}`,
		`{"correct_source_text":null}`,
		`{"correct_source_text":{"x":1}}`,
		`{"correct_source_text":7}`,
		`{"correct_source_text":[true]}`,
	} {
		if loadWith(t, body).CorrectSourceText {
			t.Errorf("%s reads as on, want off", body)
		}
	}
}

func TestCorrectSourceTextExplicitTrueAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"correct_source_text":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !svc.Get().CorrectSourceText {
		t.Fatal("explicit true reads as off")
	}
	// The startup re-save must keep it on, and a second load must read the file back as on.
	svc2, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !svc2.Get().CorrectSourceText {
		t.Fatal("true did not survive the save/load round trip")
	}
	svc2.Get().CorrectSourceText = false
	if err := svc2.Save(); err != nil {
		t.Fatal(err)
	}
	svc3, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if svc3.Get().CorrectSourceText {
		t.Fatal("false did not survive the save/load round trip")
	}
}

func TestCorrectSourceTextTrueStringReadsOn(t *testing.T) {
	// The same weak decoding auto_switch_source has: a "true" string is a boolean spelled out.
	if !loadWith(t, `{"correct_source_text":"true"}`).CorrectSourceText {
		t.Fatal(`"true" reads as off`)
	}
}
