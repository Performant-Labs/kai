package historystore

import (
	"path/filepath"
	"strings"
	"testing"
)

// Regression for the launch abort (#64): the loader splits migration.sql on ";" and runs every
// piece, so a semicolon inside a "--" comment turns the rest of the comment into an invalid
// statement ("near \"this\": syntax error") and Open fails, which aborts the app at startup.

func TestOpenInMemoryAppliesEmbeddedMigration(t *testing.T) {
	s, err := Open("")
	if err != nil {
		t.Fatalf("Open(\"\") must succeed with the embedded migration: %v", err)
	}
	defer s.Close()

	// The migration's only statement adds history.engine_id; prove it actually ran.
	rows, err := s.db.Query("PRAGMA table_info(history)")
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan: %v", err)
		}
		if name == "engine_id" {
			found = true
		}
	}
	if !found {
		t.Fatal("history.engine_id is missing: the embedded migration did not run")
	}
}

func TestOpenAlreadyMigratedFileStoreIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	for i := 1; i <= 2; i++ {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("Open #%d of the same file must succeed (a repeated ALTER's duplicate-column error is ignored): %v", i, err)
		}
		if err := s.Close(); err != nil {
			t.Fatalf("Close #%d: %v", i, err)
		}
	}
}

func TestMigrationCommentsContainNoSemicolon(t *testing.T) {
	for n, line := range strings.Split(migrationSQL, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") && strings.Contains(line, ";") {
			t.Errorf("migration.sql line %d is a comment containing a semicolon; the loader splits on it: %q", n+1, line)
		}
	}
}
