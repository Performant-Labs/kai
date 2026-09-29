package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// issue #178: the CNB (cnb.cool) update source was removed. A settings.json that still says
// updater.source = "cnb" must load without error, run on the remaining source (empty = the
// default), and be rewritten so "cnb" is gone from disk. "github" and empty are kept as they are.
func TestSavedCnbUpdaterSourceMigratesOnLoad(t *testing.T) {
	cases := []struct{ saved, want string }{
		{"cnb", ""},
		{"CNB", ""},
		{"github", "github"},
		{"", ""},
	}
	for _, c := range cases {
		dir := t.TempDir()
		svc, err := NewService(dir)
		if err != nil {
			t.Fatalf("NewService: %v", err)
		}
		svc.Get().Updater.Source = c.saved
		if err := svc.Save(); err != nil {
			t.Fatalf("Save: %v", err)
		}
		svc2, err := NewService(dir)
		if err != nil {
			t.Fatalf("reload with saved source %q must not fail: %v", c.saved, err)
		}
		if got := svc2.Get().Updater.Source; got != c.want {
			t.Errorf("saved source %q: loaded %q, want %q", c.saved, got, c.want)
		}
		raw, err := os.ReadFile(filepath.Join(dir, "settings.json"))
		if err != nil {
			t.Fatalf("read settings.json: %v", err)
		}
		if strings.Contains(strings.ToLower(string(raw)), "cnb") {
			t.Errorf("saved source %q: settings.json still mentions cnb after load:\n%s", c.saved, raw)
		}
	}
}
