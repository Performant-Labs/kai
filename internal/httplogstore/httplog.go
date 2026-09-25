// Package httplogstore provides persistent storage for HTTP request logs.
// Responsibilities: full lifecycle management of the separate SQLite log database
// (httplog.db) — creation/migration, Transport wrapping (the http_log RoundTripper),
// asynchronous inserts, periodic cleanup of expired logs, and shutdown.
//
// Constraint: httplog.db is append-only — only INSERTs use the persistent connection;
// DELETEs run on a temporary connection in Cleanup, never blocking writes.
//
// Design: package-level self-containment — Init/Close/WrapTransport never need an external
// *sql.DB; callers only care about start/stop and never touch database details.
package httplogstore

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/sqlite"
	"cnb.cool/dtapp/kai/internal/useragent"
	"go.dtapp.net/library/contrib/http_log"
)

// ---------------------------------------------------------------------------
// Package-level state (self-contained, not exposed to callers)
// ---------------------------------------------------------------------------

var (
	conn        *sql.DB
	connDSN     string
	mu          sync.RWMutex
	once        sync.Once
	cleanupDone chan struct{}
)

// ---------------------------------------------------------------------------
// Unicode decoding
// ---------------------------------------------------------------------------

// decodeUnicodeEscapes detects \uXXXX escape sequences in the bytes and decodes them into
// normal text when present.
// Supports ordinary BMP characters and surrogate pairs (e.g. \uD83D\uDE00 → 😀).
// Without escape sequences it returns the input unchanged, avoiding needless replace work.
func decodeUnicodeEscapes(b []byte) []byte {
	if !bytes.Contains(b, []byte(`\u`)) {
		return b
	}
	result := make([]byte, 0, len(b))
	remaining := b
	for len(remaining) > 0 {
		idx := bytes.Index(remaining, []byte(`\u`))
		if idx < 0 {
			result = append(result, remaining...)
			break
		}
		result = append(result, remaining[:idx]...)
		if idx+6 > len(remaining) {
			result = append(result, remaining[idx:]...)
			break
		}
		code, err := strconv.ParseUint(string(remaining[idx+2:idx+6]), 16, 16)
		if err != nil {
			result = append(result, remaining[idx:idx+6]...)
			remaining = remaining[idx+6:]
			continue
		}
		// High surrogate (0xD800-0xDBFF): check for a surrogate pair
		if code >= 0xD800 && code <= 0xDBFF && idx+12 <= len(remaining) &&
			bytes.Equal(remaining[idx+6:idx+8], []byte(`\u`)) {
			code2, err2 := strconv.ParseUint(string(remaining[idx+8:idx+12]), 16, 16)
			if err2 == nil && code2 >= 0xDC00 && code2 <= 0xDFFF {
				r := utf16.DecodeRune(rune(code), rune(code2))
				result = append(result, []byte(string(r))...)
				remaining = remaining[idx+12:]
				continue
			}
		}
		result = append(result, []byte(string(rune(code)))...)
		remaining = remaining[idx+6:]
	}
	return result
}

// ---------------------------------------------------------------------------
// Embedded resources
// ---------------------------------------------------------------------------

//go:embed schema.sql
var schemaSQL string

//go:embed migration.sql
var migrationSQL string

// ---------------------------------------------------------------------------
// Init / Close (package-level self-containment)
// ---------------------------------------------------------------------------

// Init initializes the HTTP request-log store. Only when httpLogEnabled is true does it
// open the DB, create tables/migrate, and take over http.DefaultTransport globally.
// Safe to call repeatedly (sync.Once); a no-op when httpLogEnabled=false.
func Init(dataDir string, httpLogEnabled bool) error {
	var initErr error
	once.Do(func() {
		if !httpLogEnabled {
			return
		}
		connDSN = filepath.Join(dataDir, "httplog.db")
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			initErr = fmt.Errorf(i18n.T("err.httplog_create_dir"), err, err)
			return
		}
		c, err := sql.Open("sqlite3", sqlite.BuildDSN(connDSN))
		if err != nil {
			initErr = fmt.Errorf(i18n.T("err.httplog_open_db"), err, err)
			return
		}
		if _, err := c.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL"); err != nil {
			c.Close()
			initErr = fmt.Errorf(i18n.T("err.httplog_pragma"), err, err)
			return
		}
		// Run the table-creation DDL
		if _, err := c.ExecContext(context.Background(), schemaSQL); err != nil {
			c.Close()
			initErr = fmt.Errorf(i18n.T("err.httplog_create_schema"), err, err)
			return
		}
		// Run the migration script
		for stmt := range strings.SplitSeq(migrationSQL, ";") {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}
			if _, err := c.Exec(stmt); err != nil {
				if !strings.Contains(err.Error(), "duplicate column") {
					c.Close()
					initErr = fmt.Errorf(i18n.T("err.httplog_migrate"), err, err)
					return
				}
			}
		}
		mu.Lock()
		conn = c
		mu.Unlock()
		// Take over http.DefaultTransport globally (so bare http.Get calls from third-party
		// libraries get logged too)
		http.DefaultTransport = WrapTransport(http.DefaultTransport)
		http.DefaultClient = &http.Client{Transport: http.DefaultTransport}
	})
	return initErr
}

// Close closes the log database connection and stops the periodic cleanup goroutine.
func Close() error {
	// Stop the cleanup goroutine first
	mu.Lock()
	if cleanupDone != nil {
		close(cleanupDone)
		cleanupDone = nil
	}
	c := conn
	conn = nil
	mu.Unlock()
	if c != nil {
		return c.Close()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Transport wrapping
// ---------------------------------------------------------------------------

// WrapTransport wraps the base RoundTripper into one with HTTP request logging, injecting
// the global User-Agent at the outermost layer.
// A nil base falls back to http.DefaultTransport; when httplog is disabled, the
// useragent-wrapped base is returned directly.
func WrapTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	mu.RLock()
	c := conn
	mu.RUnlock()
	if c == nil {
		return useragent.Wrap(base)
	}
	return useragent.Wrap(http_log.NewLoggingRoundTripper(base, &entLogSaver{}, nil))
}

// WrapClient wraps the client's Transport with WrapTransport, returning a new
// *http.Client.
func WrapClient(client *http.Client) *http.Client {
	if client == nil {
		client = &http.Client{}
	}
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	return &http.Client{
		Transport:     WrapTransport(base),
		CheckRedirect: client.CheckRedirect,
		Jar:           client.Jar,
		Timeout:       client.Timeout,
	}
}

// ---------------------------------------------------------------------------
// Log handler (implements http_log.LogHandler)
// ---------------------------------------------------------------------------

// entLogSaver implements the http_log.LogHandler interface, writing HTTP request logs into
// httplog.db.
// Stateless — reads/writes through the package-level conn; no db field.
type entLogSaver struct{}

// HandleLog writes HTTP request log data into the http_log table.
func (s *entLogSaver) HandleLog(ctx context.Context, data *http_log.LogData) error {
	if data == nil {
		return nil
	}
	mu.RLock()
	c := conn
	mu.RUnlock()
	if c == nil {
		return nil
	}

	params := InsertHttpLogParams{
		Hostname:          nullableStr(data.Hostname),
		Method:            nullableStr(data.Method),
		Url:               nullableStr(data.URL),
		StatusCode:        nullableInt64(int64(data.StatusCode)),
		ElapseTime:        nullableInt64(data.ElapseTime),
		ProcessElapseTime: nullableInt64(data.ProcessElapseTime),
		IsError:           data.IsError,
		CreatedAt:         time.Now(),
		GoVersion:         nullableStr(data.GoVersion),
		PluginVersion:     nullableStr(data.PluginVersion),
	}

	// Request/response headers serialized as JSON text
	if data.RequestHeaders != nil {
		if b, err := json.Marshal(data.RequestHeaders); err == nil {
			s := string(b)
			params.RequestHeaders = &s
		}
	}
	if data.ResponseHeaders != nil {
		if b, err := json.Marshal(data.ResponseHeaders); err == nil {
			s := string(b)
			params.ResponseHeaders = &s
		}
	}
	// Request/response bodies written only when non-empty
	if len(data.RequestBody) > 0 {
		params.RequestBody = decodeUnicodeEscapes(data.RequestBody)
	}
	if len(data.ResponseBody) > 0 {
		params.ResponseBody = decodeUnicodeEscapes(data.ResponseBody)
	}

	if err := New(c).InsertHttpLog(ctx, params); err != nil {
		return fmt.Errorf(i18n.T("err.httplog_insert"), err, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// nullableStr converts a Go string into a *string (empty string treated as NULL).
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// nullableInt64 converts an int64 into a *int64 (zero treated as NULL).
func nullableInt64(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

// ---------------------------------------------------------------------------
// Cleanup
// ---------------------------------------------------------------------------

// Cleanup deletes HTTP request logs older than retentionDays days (based on created_at).
// retentionDays <= 0 means no cleanup; return immediately.
// The DELETE runs on a temporary independent connection, never touching the persistent
// append-only connection.
func Cleanup(retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	mu.RLock()
	dsn := connDSN
	mu.RUnlock()
	if dsn == "" {
		return 0, nil
	}
	cleanupDB, err := sql.Open("sqlite3", sqlite.BuildDSN(dsn))
	if err != nil {
		return 0, fmt.Errorf(i18n.T("err.httplog_cleanup_open"), err, err)
	}
	defer cleanupDB.Close()
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	n, err := New(cleanupDB).DeleteOldHttpLog(context.Background(), cutoff)
	if err != nil {
		return 0, fmt.Errorf(i18n.T("err.httplog_cleanup"), err, err)
	}
	return int(n), nil
}

// StartCleanup starts the periodic cleanup goroutine, purging expired logs every hour.
// The goroutine stops automatically on Close().
func StartCleanup(retentionDays int, logger *slog.Logger) {
	if retentionDays <= 0 {
		return
	}
	mu.RLock()
	dsn := connDSN
	mu.RUnlock()
	if dsn == "" {
		return
	}
	mu.Lock()
	if cleanupDone != nil {
		mu.Unlock()
		return
	}
	cleanupDone = make(chan struct{})
	mu.Unlock()
	ticker := time.NewTicker(1 * time.Hour)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				n, err := Cleanup(retentionDays)
				if err != nil {
					logger.Error(i18n.T("log.httplog_cleanup_failed"), slog.Any("error", err))
				} else if n > 0 {
					logger.Info(i18n.T("log.httplog_cleaned"), slog.Int("count", n))
				}
			case <-cleanupDone:
				return
			}
		}
	}()
}
