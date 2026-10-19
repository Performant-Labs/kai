package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// Issue #69: the "keep the translate window on top of other apps" setting is OFF by default
// (the issue's title asks the window to stop staying on top of every app). Like double_copy_translate,
// only an explicit true turns it on: a missing key, or a value that is not a boolean, reads as off.
// The frontend reads it once when the translate window mounts — it is the DEFAULT the window's pin
// state starts from, not a live toggle.

func TestTranslateWindowOnTopDefaultsOff(t *testing.T) {
	if DefaultSettings().TranslateWindowOnTop {
		t.Fatal("DefaultSettings().TranslateWindowOnTop = true, want false")
	}
	if loadWith(t, "").TranslateWindowOnTop {
		t.Fatal("a fresh install reads the setting as on, want off")
	}
}

func TestTranslateWindowOnTopMissingKeyReadsOff(t *testing.T) {
	// An old settings.json written before the key existed.
	if loadWith(t, `{"language":"en-US","default_to":"en"}`).TranslateWindowOnTop {
		t.Fatal("a settings.json without the key reads as on, want off")
	}
}

func TestTranslateWindowOnTopGarbledValueReadsOff(t *testing.T) {
	for _, body := range []string{
		`{"translate_window_on_top":"maybe"}`,
		`{"translate_window_on_top":null}`,
		`{"translate_window_on_top":{"x":1}}`,
		`{"translate_window_on_top":7}`,
		`{"translate_window_on_top":[true]}`,
	} {
		if loadWith(t, body).TranslateWindowOnTop {
			t.Errorf("%s reads as on, want off", body)
		}
	}
}

// The case string branch of readTranslateWindowOnTop: a boolean spelled out as a string is
// weak-decoded, exactly like auto_switch_source / correct_source_text (strconv.ParseBool —
// so "1"/"0" count too). This test is what fails if that branch is deleted.
func TestTranslateWindowOnTopBooleanStringIsWeakDecoded(t *testing.T) {
	for _, tc := range []struct {
		body string
		want bool
	}{
		{`{"translate_window_on_top":"true"}`, true},
		{`{"translate_window_on_top":"false"}`, false},
		{`{"translate_window_on_top":"1"}`, true},
		{`{"translate_window_on_top":"0"}`, false},
	} {
		if got := loadWith(t, tc.body).TranslateWindowOnTop; got != tc.want {
			t.Errorf("%s reads as %v, want %v", tc.body, got, tc.want)
		}
	}
}

func TestTranslateWindowOnTopExplicitTrueAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"translate_window_on_top":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !svc.Get().TranslateWindowOnTop {
		t.Fatal("explicit true reads as off")
	}
	// The startup re-save keeps it on, a second load reads it back.
	svc2, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !svc2.Get().TranslateWindowOnTop {
		t.Fatal("true did not survive the save/load round trip")
	}
	svc2.Get().TranslateWindowOnTop = false
	if err := svc2.Save(); err != nil {
		t.Fatal(err)
	}
	svc3, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if svc3.Get().TranslateWindowOnTop {
		t.Fatal("false did not survive the save/load round trip")
	}
}
