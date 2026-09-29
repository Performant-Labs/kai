package service

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/settings"
)

// Issue #195: SaveConfig persists the text size, coerces a value outside the six allowed sizes to
// the 120 default (never an error), and leaves the size alone when only other fields change.
func TestSaveConfigPersistsFontSize(t *testing.T) {
	st, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := NewConfigWrapper(st, nil, nil)
	cfg := *st.Get()
	for _, p := range settings.FontSizeSteps() {
		cfg.FontSize = p
		if err := w.SaveConfig(&cfg); err != nil {
			t.Fatal(err)
		}
		if got := st.Get().FontSize; got != p {
			t.Fatalf("SaveConfig(%d) stored %d", p, got)
		}
	}
}

func TestSaveConfigCoercesFontSize(t *testing.T) {
	st, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := NewConfigWrapper(st, nil, nil)
	cfg := *st.Get()
	for _, bad := range []int{0, -5, 100, 121, 500} {
		cfg.FontSize = 150
		if err := w.SaveConfig(&cfg); err != nil {
			t.Fatal(err)
		}
		cfg.FontSize = bad
		if err := w.SaveConfig(&cfg); err != nil {
			t.Fatalf("SaveConfig(%d) failed: %v", bad, err)
		}
		if got := st.Get().FontSize; got != 120 {
			t.Fatalf("SaveConfig(%d) stored %d, want 120", bad, got)
		}
	}
}

func TestSaveConfigKeepsFontSizeWhenOtherFieldsChange(t *testing.T) {
	st, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := NewConfigWrapper(st, nil, nil)
	cfg := *st.Get()
	cfg.FontSize = 135
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	cfg.DefaultTo = "fr"
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	if got := st.Get().FontSize; got != 135 {
		t.Fatalf("font size = %d after an unrelated save, want 135", got)
	}
}
