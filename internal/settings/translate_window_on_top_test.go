package settings

import (
	"os"
	"path/filepath"
	"testing"
	"time"
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

// Editing settings.json while the app runs (the file watcher's hot reload) picks the setting
// up through the same weak decoding as a load. This is what fails if the hot-reload
// assignment `s.cfg.TranslateWindowOnTop = readTranslateWindowOnTop(s.v)` is removed.
func TestTranslateWindowOnTopHotReload(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	waitFor := func(want bool) bool {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			svc.mu.RLock()
			got := svc.cfg.TranslateWindowOnTop
			svc.mu.RUnlock()
			if got == want {
				return true
			}
			time.Sleep(50 * time.Millisecond)
		}
		return false
	}
	if err := os.WriteFile(path, []byte(`{"translate_window_on_top":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !waitFor(true) {
		t.Fatal("hot reload did not pick up translate_window_on_top true")
	}
	// Back to off, then on again as a boolean string: the reload path must weak-decode it
	// exactly like a load does (readTranslateWindowOnTop, not mapstructure).
	if err := os.WriteFile(path, []byte(`{"translate_window_on_top":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !waitFor(false) {
		t.Fatal("hot reload did not pick up translate_window_on_top false")
	}
	if err := os.WriteFile(path, []byte(`{"translate_window_on_top":"true"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !waitFor(true) {
		t.Fatal(`hot reload did not weak-decode "true" as on`)
	}
}
