package i18n

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #11: the hotkey paths used to write the user's selected or copied text into kai.log
// through the log.field_content string. The key is gone from the locale sources and no
// production code may reference it again: a log call on a path that touches user text must
// carry its length, language or engine — never the content. This test scans the repo's own
// Go sources for the key, so it guards every such path at once (and runs wherever the suite
// runs, no Wails needed). Found a failure? Log length/language/engine instead of content.
func TestNoProductionCodeLogsTheContentField(t *testing.T) {
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", ".worktrees", "frontend", "docs":
				// Not Go sources of this app (other worktrees hold old branches).
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), "log.field_content") {
			t.Errorf("%s references log.field_content: the log must carry length/language/engine, never the text (issue #11)", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The locale sources (split and merged) must not define log.field_content either, so the
// removed field cannot quietly come back with a "valid" translation. If this fails, delete
// the key from locales/split/<locale>/logs.json and re-run scripts/merge-go-i18n.sh.
func TestLocaleSourcesDoNotDefineTheContentField(t *testing.T) {
	for _, p := range []string{
		filepath.Join("locales", "split", "en-US", "logs.json"),
		filepath.Join("locales", "split", "zh-CN", "logs.json"),
		filepath.Join("locales", "en-US.json"),
		filepath.Join("locales", "zh-CN.json"),
	} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if strings.Contains(string(b), "log.field_content") {
			t.Errorf("%s still defines log.field_content (issue #11)", p)
		}
	}
}
