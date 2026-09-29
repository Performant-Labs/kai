package service

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/events"
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
	for _, bad := range []int{0, -5, 105, 121, 165, 500} {
		cfg.FontSize = 160
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
	cfg.FontSize = 140
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	cfg.DefaultTo = "fr"
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	if got := st.Get().FontSize; got != 140 {
		t.Fatalf("font size = %d after an unrelated save, want 140", got)
	}
}

type emitted struct {
	name string
	data any
}

// wrapperWithRecorder returns a wrapper whose broadcasts are recorded, over a real settings
// service, so SaveConfig runs its real path.
func wrapperWithRecorder(t *testing.T) (*ConfigWrapper, *settings.Service, *[]emitted) {
	t.Helper()
	st, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := NewConfigWrapper(st, nil, nil)
	var got []emitted
	w.emit = func(name string, data any) { got = append(got, emitted{name, data}) }
	return w, st, &got
}

func fontSizeEvents(all []emitted) []emitted {
	var out []emitted
	for _, e := range all {
		if e.name == events.EventFontSizeChanged {
			out = append(out, e)
		}
	}
	return out
}

func TestSaveConfigBroadcastsAChangedFontSizeOnce(t *testing.T) {
	w, st, got := wrapperWithRecorder(t)
	cfg := *st.Get()
	cfg.FontSize = 160
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	ev := fontSizeEvents(*got)
	if len(ev) != 1 {
		t.Fatalf("got %d font size events, want exactly 1: %v", len(ev), *got)
	}
	if ev[0].data != 160 {
		t.Fatalf("payload = %#v, want the int 160", ev[0].data)
	}
}

func TestSaveConfigDoesNotBroadcastAnUnchangedFontSize(t *testing.T) {
	w, st, got := wrapperWithRecorder(t)
	cfg := *st.Get()
	cfg.FontSize = 140
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	*got = nil
	cfg.DefaultTo = "fr" // an unrelated change, same size
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	if ev := fontSizeEvents(*got); len(ev) != 0 {
		t.Fatalf("unchanged size broadcast: %v", ev)
	}
}

func TestSaveConfigBroadcastsTheCoercedFontSize(t *testing.T) {
	w, st, got := wrapperWithRecorder(t)
	cfg := *st.Get()
	cfg.FontSize = 160
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	*got = nil
	cfg.FontSize = 137
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	ev := fontSizeEvents(*got)
	if len(ev) != 1 || ev[0].data != 120 {
		t.Fatalf("events = %v, want one carrying 120", ev)
	}
	// Garbage that coerces to the size already in force changes nothing and says nothing.
	*got = nil
	cfg.FontSize = -4
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	if ev := fontSizeEvents(*got); len(ev) != 0 {
		t.Fatalf("coerced-to-current size broadcast: %v", ev)
	}
}

func TestSaveConfigWithoutAppOrEmitterStillSaves(t *testing.T) {
	st, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w := NewConfigWrapper(st, nil, nil) // no app, no emitter
	cfg := *st.Get()
	cfg.FontSize = 180
	if err := w.SaveConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	if st.Get().FontSize != 180 {
		t.Fatalf("font size = %d, want 180", st.Get().FontSize)
	}
}
