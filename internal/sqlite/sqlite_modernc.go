//go:build !sqlite_mattn

package sqlite

import (
	"database/sql"
	"fmt"

	"modernc.org/sqlite"
)

func init() {
	// Registers under the driver name "sqlite3" used by ent's dialect.SQLite, and is reused
	// by every sqlite caller (httplog, tests, etc.).
	sql.Register("sqlite3", &sqlite.Driver{})
}

// BuildDSN builds the connection string for modernc sqlite (pure Go, no CGO).
// Uses the _pragma syntax to declare foreign-key constraints, WAL journal mode and the
// busy-timeout.
func BuildDSN(path string) string {
	return fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout=5000", path)
}
