package settings

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Issue #195: the text-size setting. Default 120 (percent of the pre-#195 size); six allowed
// values on an even 15-point ladder; anything else, in a file or on save, reads as 120 and never
// fails the settings load.

func TestFontSizeStepsAreTheSixEvenSteps(t *testing.T) {
	want := []int{90, 105, 120, 135, 150, 165}
	got := FontSizeSteps()
	if !slices.Equal(got, want) {
		t.Fatalf("FontSizeSteps() = %v, want %v", got, want)
	}
	// The returned slice is a copy: changing it cannot change the ladder.
	clear(got)
	if !slices.Equal(FontSizeSteps(), want) {
		t.Fatal("FontSizeSteps() exposes its backing array")
	}
}

func TestFontSizeDefaultsTo120(t *testing.T) {
	if DefaultFontSize != 120 {
		t.Fatalf("DefaultFontSize = %d, want 120", DefaultFontSize)
	}
	if DefaultSettings().FontSize != 120 {
		t.Fatalf("DefaultSettings().FontSize = %d, want 120", DefaultSettings().FontSize)
	}
	if got := loadWith(t, "").FontSize; got != 120 {
		t.Fatalf("a fresh install reads %d, want 120", got)
	}
}

func TestFontSizeOldSettingsFileReads120(t *testing.T) {
	if got := loadWith(t, `{"language":"en-US","default_to":"en"}`).FontSize; got != 120 {
		t.Fatalf("a settings.json from before #195 reads %d, want 120 (not 100, not 0)", got)
	}
}

func TestFontSizeNormalize(t *testing.T) {
	for _, p := range FontSizeSteps() {
		if got := NormalizeFontSize(p); got != p {
			t.Errorf("NormalizeFontSize(%d) = %d, want it kept", p, got)
		}
	}
	for _, p := range []int{-1, 0, 1, 89, 91, 100, 119, 121, 166, 180, 1000} {
		if got := NormalizeFontSize(p); got != 120 {
			t.Errorf("NormalizeFontSize(%d) = %d, want 120", p, got)
		}
	}
}

func TestFontSizeGarbledValueReads120(t *testing.T) {
	for _, body := range []string{
		`{"font_size":null}`,
		`{"font_size":"big"}`,
		`{"font_size":{"x":1}}`,
		`{"font_size":[150]}`,
		`{"font_size":true}`,
		`{"font_size":100}`,
		`{"font_size":0}`,
		`{"font_size":-135}`,
		`{"font_size":121}`,
		`{"font_size":135.5}`,
		`{"font_size":9999999999999999999}`,
	} {
		if got := loadWith(t, body).FontSize; got != 120 {
			t.Errorf("%s reads %d, want 120", body, got)
		}
	}
}

func TestFontSizeAllowedValuesAreKept(t *testing.T) {
	for _, p := range FontSizeSteps() {
		body := `{"font_size":` + strconv.Itoa(p) + `}`
		if got := loadWith(t, body).FontSize; got != p {
			t.Errorf("%s reads %d", body, got)
		}
	}
	// A whole number written as a float (135.0) is the same setting.
	if got := loadWith(t, `{"font_size":135.0}`).FontSize; got != 135 {
		t.Errorf("135.0 reads %d, want 135", got)
	}
}

func TestFontSizePersistsAcrossReload(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	svc.Get().FontSize = 150
	if err := svc.Save(); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"font_size": 150`) && !strings.Contains(string(raw), `"font_size":150`) {
		t.Fatalf("settings.json does not carry font_size 150:\n%s", raw)
	}
	svc2, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := svc2.Get().FontSize; got != 150 {
		t.Fatalf("reloaded font size = %d, want 150", got)
	}
}

func TestFontSizeDoesNotDisturbOtherSettings(t *testing.T) {
	s := loadWith(t, `{"auto_switch_source":false,"double_copy_translate":true,"font_size":90,"default_to":"fr"}`)
	if s.FontSize != 90 || s.AutoSwitchSource || !s.DoubleCopyTranslate || s.DefaultTo != "fr" {
		t.Fatalf("font=%d auto=%v double=%v to=%q", s.FontSize, s.AutoSwitchSource, s.DoubleCopyTranslate, s.DefaultTo)
	}
}

// The frontend keeps its own copy of the ladder and the default (it must apply a size before any
// Go call returns). This pins the two copies to each other.
func TestFontSizeLadderMatchesTheFrontend(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "frontend", "src", "constants", "fontSize.ts"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	steps := regexp.MustCompile(`FONT_SIZE_STEPS\s*=\s*\[([^\]]*)\]`).FindStringSubmatch(text)
	if len(steps) < 2 {
		t.Fatal("no FONT_SIZE_STEPS in frontend/src/constants/fontSize.ts")
	}
	var fe []int
	for _, f := range strings.Split(steps[1], ",") {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		n, convErr := strconv.Atoi(f)
		if convErr != nil {
			t.Fatalf("FONT_SIZE_STEPS entry %q: %v", f, convErr)
		}
		fe = append(fe, n)
	}
	if goSteps := FontSizeSteps(); !slices.Equal(fe, goSteps) {
		t.Fatalf("frontend steps %v, go steps %v", fe, goSteps)
	}
	def := regexp.MustCompile(`DEFAULT_FONT_SIZE\s*=\s*(\d+)`).FindStringSubmatch(text)
	if len(def) < 2 || def[1] != strconv.Itoa(DefaultFontSize) {
		t.Fatalf("frontend DEFAULT_FONT_SIZE = %v, go = %d", def, DefaultFontSize)
	}
}

// Editing settings.json while the app runs (the file watcher's hot reload) applies the size with
// the same coercion as a load.
func TestFontSizeHotReload(t *testing.T) {
	dir := t.TempDir()
	svc, err := NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	waitFor := func(want int) bool {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			svc.mu.RLock()
			got := svc.cfg.FontSize
			svc.mu.RUnlock()
			if got == want {
				return true
			}
			time.Sleep(50 * time.Millisecond)
		}
		return false
	}
	if err := os.WriteFile(path, []byte(`{"font_size":165}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !waitFor(165) {
		t.Fatal("hot reload did not pick up font_size 165")
	}
	if err := os.WriteFile(path, []byte(`{"font_size":"huge"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if !waitFor(120) {
		t.Fatal("hot reload did not coerce a garbled font_size to 120")
	}
}
