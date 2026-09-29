package service

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/settings"
)

// Issue #208: SaveConfig copies the correction switch from the incoming struct, both ways round:
// on persists on, off persists off. (Without the copy the toolbar checkbox would flip and forget.)
func TestSaveConfigPersistsCorrectSourceText(t *testing.T) {
	st, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := NewConfigWrapper(st, nil, nil)
	cfg := *st.Get()
	if cfg.CorrectSourceText {
		t.Fatal("a fresh install has the correction on")
	}
	cfg.CorrectSourceText = true
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	if !st.Get().CorrectSourceText {
		t.Fatal("SaveConfig dropped CorrectSourceText=true")
	}
	cfg.CorrectSourceText = false
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	if st.Get().CorrectSourceText {
		t.Fatal("SaveConfig dropped CorrectSourceText=false")
	}
}
