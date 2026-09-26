package httplogstore

import (
	"database/sql"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/sqlite"
)

// Regression for the launch abort (#64). Init itself is not exercised here (it takes over
// http.DefaultTransport globally behind a sync.Once), so the embedded migration is checked
// directly: no comment may contain the semicolon the loader splits on, and every piece the
// loader would run must be accepted by SQLite.

func TestMigrationCommentsContainNoSemicolon(t *testing.T) {
	for n, line := range strings.Split(migrationSQL, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") && strings.Contains(line, ";") {
			t.Errorf("migration.sql line %d is a comment containing a semicolon; the loader splits on it: %q", n+1, line)
		}
	}
}

func TestEmbeddedMigrationIsAcceptedBySQLite(t *testing.T) {
	c, err := sql.Open("sqlite3", sqlite.BuildDSN(":memory:"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer c.Close()
	c.SetMaxOpenConns(1) // one in-memory database for schema and migration

	if _, err := c.Exec(schemaSQL); err != nil {
		t.Fatalf("schema.sql: %v", err)
	}
	// Mirrors Init's loop: split on ";", skip empty pieces, ignore only "duplicate column".
	for _, stmt := range strings.Split(migrationSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := c.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			t.Fatalf("SQLite rejected a migration piece %q: %v", stmt, err)
		}
	}
}
