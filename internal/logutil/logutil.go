// Package logutil provides app runtime-log initialization, day rotation, compression and
// expiry cleanup.
// Level, retention days and the compression switch are driven by settings.LogConfig and can
// be hot-updated via settings.json.
package logutil

import (
	"compress/gzip"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
)

// ParseLevel parses a level string into slog.Level; invalid values fall back to info.
func ParseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Rotator manages day-rotated log files and exposes a slog.Handler whose level can be
// adjusted dynamically.
type Rotator struct {
	mu          sync.Mutex
	dir         string
	level       slog.Level
	handler     *slog.TextHandler
	currentPath string
	file        *os.File
	retention   int
	compress    bool
}

// NewRotator creates a Rotator, rotates once immediately (archiving kai.log when it is not
// today’s), and opens today’s log file.
func NewRotator(dir string, level slog.Level, retention int, compress bool) (*Rotator, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("err.logutil_create_dir"), err)
	}
	r := &Rotator{
		dir:       dir,
		level:     level,
		retention: retention,
		compress:  compress,
	}
	if err := r.rotateIfNeeded(); err != nil {
		return nil, err
	}
	return r, nil
}

// Handler returns the slog.Handler; callers build a slog.Logger from it and SetDefault.
func (r *Rotator) Handler() slog.Handler { return r.handler }

// Close closes the underlying log file handle (used for the panic handler’s final flush);
// idempotent.
func (r *Rotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.file != nil {
		err := r.file.Close()
		r.file = nil
		return err
	}
	return nil
}

// SetLevel adjusts the log level dynamically (hot update).
func (r *Rotator) SetLevel(level slog.Level) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.level = level
	r.rebuildHandler()
}

// UpdateRetention adjusts retention days and the compression switch dynamically (hot
// update).
func (r *Rotator) UpdateRetention(retention int, compress bool) {
	r.mu.Lock()
	r.retention = retention
	r.compress = compress
	r.mu.Unlock()
	// Trigger one cleanup immediately so the change takes effect right away.
	r.cleanup()
}

// dayFile returns the archived log file name for the given date (without extension).
func (r *Rotator) dayFile(t time.Time) string {
	return fmt.Sprintf("kai-%s.log", t.Format("2006-01-02"))
}

// rotateIfNeeded renames a kai.log that is not from today to kai-YYYY-MM-DD.log (compressing
// as needed), then creates a fresh kai.log for today and rebuilds the handler.
func (r *Rotator) rotateIfNeeded() error {
	today := time.Now().Format("2006-01-02")
	current := filepath.Join(r.dir, "kai.log")

	// If kai.log exists and is not from today (judged by mtime date), archive the old file.
	if info, err := os.Stat(current); err == nil {
		modDay := info.ModTime().Format("2006-01-02")
		if modDay != today {
			archiveName := r.dayFile(info.ModTime())
			archivePath := filepath.Join(r.dir, archiveName)
			// Avoid overwriting on multiple same-day launches: append a sequence suffix.
			archivePath = uniquePath(archivePath)
			if err := os.Rename(current, archivePath); err != nil {
				return fmt.Errorf("%s: %w", i18n.T("err.logutil_archive_old"), err)
			}
			if r.compress {
				if err := gzipFile(archivePath, archivePath+".gz"); err == nil {
					os.Remove(archivePath)
				}
			}
		}
	}

	f, err := os.OpenFile(current, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("err.logutil_open_file"), err)
	}
	if r.file != nil {
		r.file.Close()
	}
	r.file = f
	r.currentPath = current
	r.rebuildHandler()
	return nil
}

// rebuildHandler rebuilds the handler with the current level and writer.
func (r *Rotator) rebuildHandler() {
	w := io.MultiWriter(r.file, os.Stderr)
	r.handler = slog.NewTextHandler(w, &slog.HandlerOptions{Level: r.level})
}

// cleanup deletes archived log files older than retention days (kai-YYYY-MM-DD.log or
// .gz).
func (r *Rotator) cleanup() {
	r.mu.Lock()
	retention := r.retention
	compress := r.compress
	r.mu.Unlock()
	if retention <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -retention)
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return
	}
	type archived struct {
		path string
		day  time.Time
	}
	var olds []archived
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "kai-") || (!strings.HasSuffix(name, ".log") && !strings.HasSuffix(name, ".log.gz")) {
			continue
		}
		// Parse the date prefix kai-2006-01-02
		base := strings.TrimSuffix(strings.TrimSuffix(name, ".gz"), ".log")
		day, err := time.Parse("2006-01-02", strings.TrimPrefix(base, "kai-"))
		if err != nil {
			continue
		}
		if day.Before(cutoff) {
			olds = append(olds, archived{path: filepath.Join(r.dir, name), day: day})
		}
	}
	sort.Slice(olds, func(i, j int) bool { return olds[i].day.Before(olds[j].day) })
	for _, o := range olds {
		os.Remove(o.path)
	}
	_ = compress
}

// uniquePath inserts .N into the middle of path when it already exists, avoiding overwrite
// (kai-2026-08-12.1.log).
func uniquePath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s.%d%s", base, i, ext)
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}
}

// gzipFile compresses src into dst, overwriting dst if it exists.
func gzipFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	gw := gzip.NewWriter(out)
	if _, err := io.Copy(gw, in); err != nil {
		return err
	}
	return gw.Close()
}

// FrontendWriter is a standalone slog.Handler container writing logs/frontend.log.
// Frontend console logs and JS errors land here, separate from the main app log (kai.log).
type FrontendWriter struct {
	mu    sync.Mutex
	dir   string
	file  *os.File
	rot   *Rotator
	level slog.Level
}

// NewFrontendWriter creates the frontend log writer (day rotation + expiry cleanup,
// following the main log policy).
func NewFrontendWriter(dir string, level slog.Level, retention int, compress bool) (*FrontendWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("err.logutil_create_dir"), err)
	}
	fw := &FrontendWriter{dir: dir, level: level, rot: &Rotator{dir: dir, level: level, retention: retention, compress: compress}}
	if err := fw.rotateIfNeeded(); err != nil {
		return nil, err
	}
	return fw, nil
}

// rotateIfNeeded day-rotates the frontend log (frontend.log -> frontend-YYYY-MM-DD.log).
func (fw *FrontendWriter) rotateIfNeeded() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	today := time.Now().Format("2006-01-02")
	current := filepath.Join(fw.dir, "frontend.log")
	if info, err := os.Stat(current); err == nil {
		if info.ModTime().Format("2006-01-02") != today {
			archivePath := uniquePath(filepath.Join(fw.dir, fmt.Sprintf("frontend-%s.log", info.ModTime().Format("2006-01-02"))))
			if err := os.Rename(current, archivePath); err == nil && fw.rot.compress {
				if err := gzipFile(archivePath, archivePath+".gz"); err == nil {
					os.Remove(archivePath)
				}
			}
		}
	}
	f, err := os.OpenFile(current, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("%s: %w", i18n.T("err.logutil_open_frontend"), err)
	}
	if fw.file != nil {
		fw.file.Close()
	}
	fw.file = f
	return nil
}

// Write implements io.Writer: reusing logutil’s rotation/cleanup logic (sharing
// compression/cleanup with Rotator).
func (fw *FrontendWriter) Write(p []byte) (int, error) {
	fw.mu.Lock()
	defer fw.mu.Unlock()
	if fw.file == nil {
		return len(p), nil
	}
	return fw.file.Write(p)
}

// Handler returns the handler for slog use.
func (fw *FrontendWriter) Handler() slog.Handler {
	return slog.NewTextHandler(fw, &slog.HandlerOptions{Level: fw.level})
}

// SetLevel adjusts the frontend log level dynamically.
func (fw *FrontendWriter) SetLevel(level slog.Level) {
	fw.mu.Lock()
	fw.level = level
	fw.mu.Unlock()
}

// FrontendLogService is the Go binding service the frontend calls: it receives frontend
// logs/errors and writes them to frontend.log.
type FrontendLogService struct {
	logger *slog.Logger
	fw     *FrontendWriter
}

// NewFrontendLogService creates the frontend log service.
func NewFrontendLogService(fw *FrontendWriter) *FrontendLogService {
	return &FrontendLogService{
		logger: slog.New(fw.Handler()),
		fw:     fw,
	}
}

// FrontendLog is called by the frontend: level is debug/info/warn/error, msg is the log
// text.
func (s *FrontendLogService) FrontendLog(level, msg string) {
	switch ParseLevel(level) {
	case slog.LevelDebug:
		s.logger.Debug(msg)
	case slog.LevelWarn:
		s.logger.Warn(msg)
	case slog.LevelError:
		s.logger.Error(msg)
	default:
		s.logger.Info(msg)
	}
}

// SetLevel syncs the frontend log level (called by applyLogConfig).
func (s *FrontendLogService) SetLevel(level slog.Level) {
	s.fw.SetLevel(level)
	s.logger = slog.New(s.fw.Handler())
}
