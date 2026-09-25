//go:build sqlite_mattn

package sqlite

import (
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// mattn/go-sqlite3 registers the driver as "sqlite3" via its own init() on import, so this
// file needs no manual registration; it only provides a BuildDSN named like modernc's so the
// driver can be switched.

// BuildDSN builds the connection string for mattn/go-sqlite3 (CGO).
// mattn uses the _foreign_keys / _journal_mode / _busy_timeout query-parameter syntax, which
// is not interchangeable with modernc's _pragma= syntax.
func BuildDSN(path string) string {
	return fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", path)
}
