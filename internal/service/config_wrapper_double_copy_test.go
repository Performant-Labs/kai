package service

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/settings"
)

// Issue #199: SaveConfig copies the double-copy switch from the incoming struct, and both ways
// round: on persists on, off persists off (with and without a hotkey manager to re-register).
func TestSaveConfigPersistsDoubleCopyTranslate(t *testing.T) {
	st, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := NewConfigWrapper(st, nil, nil)
	cfg := *st.Get()
	cfg.DoubleCopyTranslate = true
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	if !st.Get().DoubleCopyTranslate {
		t.Fatal("SaveConfig dropped DoubleCopyTranslate=true")
	}
	cfg.DoubleCopyTranslate = false
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	if st.Get().DoubleCopyTranslate {
		t.Fatal("SaveConfig dropped DoubleCopyTranslate=false")
	}
}

func TestGetDoubleCopyStatusWithoutManagerIsOff(t *testing.T) {
	st, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got := NewConfigWrapper(st, nil, nil).GetDoubleCopyStatus(); got != "off" {
		t.Fatalf("status = %q, want off", got)
	}
}
