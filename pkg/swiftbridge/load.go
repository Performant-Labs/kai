//go:build darwin

// Package swiftbridge is Kai's Swift bridge layer (a pure-Go dynamic loader + two-sided
// contract types).
//
// It loads libkai_bridge.dylib at runtime via github.com/ebitengine/purego's Dlopen and
// registers all kai_* function pointers (zero cgo; the bridge library is never statically
// linked into the main binary). After changing Swift code, just rebuild
// internal/swift/build.sh (produces the .dylib and copies it into this directory); the
// runtime Dlopen then loads the latest — no "rebuilt but app still runs old code" doubts.
//
// The types (OCRSuccess / TranslateSuccess / BridgeErr* / SelectionPoint / ScreenSize etc.)
// correspond one-to-one with the Swift side's Codable / BRIDGE_ERR_* literals; see
// bridge_errors.go and internal/swift/.
//
// Type mapping follows purego conventions:
//   - C char* (input-only strings) -> Go string (purego auto-CString's and frees after the
//     call)
//   - C char* (output buffer)      -> Go unsafe.Pointer (caller passes
//     unsafe.Pointer(&buf[0]))
//   - C int / Int32                -> Go int32 (C int is 32-bit under macOS LP64)
//   - Swift Int64                  -> Go int64 (the kai_translate call token)
//   - C Bool                       -> Go bool (1-byte _Bool)
//
// This file compiles on macOS only (purego.Dlopen/RTLD_* are Unix-only). Non-macOS platforms
// get same-signature empty implementations from load_other.go (Init returns nil directly,
// function pointers stay nil; all callers are behind darwin builds and never trigger on
// non-darwin; the Init that main.go calls unconditionally is a no-op on non-darwin).
package swiftbridge

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/buildinfo"
	"cnb.cool/dtapp/kai/internal/i18n"
	"github.com/ebitengine/purego"
)

var (
	loadOnce sync.Once
	loadErr  error
	handle   uintptr

	// kai_* function pointers (registered by purego, zero cgo).
	KaiOCR                    func(base64 string, out unsafe.Pointer, outCap int32, correct int32, timeout int32, retry int32) int32
	KaiAccessibilityEnabled   func() int32
	KaiAccessibilityRequest   func() int32
	KaiScreenRecordingEnabled func() int32
	KaiScreenRecordingRequest func() int32
	KaiSelectionPoint         func(out unsafe.Pointer, outCap int32) int32
	KaiScreenSize             func(out unsafe.Pointer, outCap int32) int32
	KaiInputMonitoringEnabled func() int32
	KaiAvailableLanguages     func(out unsafe.Pointer, outCap int32) int32
	KaiSetLogConfig           func(dir string, level string, retentionDays int32, compress bool)
	KaiSetLocale              func(locale string)
	KaiTranslate              func(src string, dst string, text string, token int64, out unsafe.Pointer, outCap int32) int32
	KaiTranslateCancel        func(token int64) int32
	// KaiWarmTranslate warms (creates + prepareTranslation()s) the Apple engine's cached
	// TranslationSession for (src, dst) — issue #173 item 8. Blocks the calling goroutine
	// until warming finishes or fails; main.go calls it in its own goroutine at launch so
	// startup is never blocked on it.
	KaiWarmTranslate func(src string, dst string) int32
	// KaiDetectLanguage detects the language of text locally with NaturalLanguage's
	// NLLanguageRecognizer and writes {"lang":"es","confidence":0.99} (DetectedLanguage) into out
	// (issue #200). A synchronous, pure computation: safe from any goroutine, no main-thread rule.
	KaiDetectLanguage func(text string, out unsafe.Pointer, outCap int32) int32
	// KaiCorrect runs Apple's on-device Foundation Models over text with the given instructions and
	// writes {"text":"..."} (CorrectionSuccess) into out (issue #208). Synchronous, bounded by a
	// timeout of its own, and it never touches the main thread: call it from a goroutine.
	// KaiCorrectAvailability writes {"status":"available"} or the reason the model cannot run
	// (locale empty: the model alone; else also whether it supports that language).
	KaiCorrect             func(instructions string, text string, out unsafe.Pointer, outCap int32) int32
	KaiCorrectAvailability func(locale string, out unsafe.Pointer, outCap int32) int32
	// Double Cmd+C (issue #199). Start creates a listen-only event tap on a thread of its own and
	// returns 0 (listening), 1 (Input Monitoring missing; nothing created, no prompt) or 2 (failed).
	// Poll writes and clears the recorded Cmd+C key-downs as JSON. None of these needs the main
	// thread, and none calls back into Go.
	KaiDoubleCopyStart      func() int32
	KaiDoubleCopyStop       func() int32
	KaiDoubleCopyPoll       func(out unsafe.Pointer, outCap int32) int32
	KaiDoubleCopySuppress   func(ms int32) int32
	KaiDoubleCopyRequest    func() int32
	KaiDoubleCopyPasteboard func(out unsafe.Pointer, outCap int32) int32
	// KaiDoubleCopyIngest is the tap callback's body, exposed so tests can drive it without a tap.
	KaiDoubleCopyIngest func(keycode int32, flags uint64, autorepeat int32, srcPid int32) int32
)

// The dylib defaults to the same directory as this .go source file (build.sh copies the
// latest libkai_bridge.dylib here).
const dylibName = "libkai_bridge.dylib"

// Init loads the dylib and registers all kai_* functions. Safe to call repeatedly (only the
// first call actually runs).
// dylibPath is optional: an empty value uses the default (libkai_bridge.dylib in this
// package's source directory, dev mode);
// if the default path fails to load, it automatically falls back to the packaged app bundle
// standard location Contents/Frameworks/libkai_bridge.dylib (copied in by the build script).
// An error is returned only when every candidate fails, and callers (via Available()) should
// still degrade safely — not fatal.
func Init(dylibPath string) error {
	loadOnce.Do(func() {
		candidates := make([]string, 0, 6)
		if dylibPath != "" {
			candidates = append(candidates, dylibPath)
		}
		if buildinfo.IsDev() {
			// Dev mode: prefer the local pkg/swiftbridge/libkai_bridge.dylib (build.sh's
			// output) — after rebuilding Swift it takes effect without rebuilding the Go
			// binary. Only two reliable paths are kept:
			//  - the directory of this .go source file (go build embeds the real source path
			//    into the binary, hitting pkg/swiftbridge/ directly)
			//  - pkg/swiftbridge/ under the project root, walked up from the executable's
			//    directory (project root or bin/)
			if _, thisFile, _, ok := runtime.Caller(0); ok {
				candidates = append(candidates, filepath.Join(filepath.Dir(thisFile), dylibName))
			}
			if exe, err := os.Executable(); err == nil {
				exeDir := filepath.Dir(exe)
				candidates = append(candidates,
					filepath.Join(exeDir, "pkg", "swiftbridge", dylibName),
					filepath.Join(exeDir, "..", "pkg", "swiftbridge", dylibName),
				)
			}
		}
		// Non-dev (packaged): load from the go:embed-embedded bytes written to a temp file,
		// no external file needed;
		// in dev, the embed also backs up a missing local file, so loading always succeeds and
		// never panics.
		if embedded, err := writeEmbeddedDylib(); err == nil {
			candidates = append(candidates, embedded)
		}

		var lastErr error
		for _, p := range candidates {
			if p == "" {
				continue
			}
			h, err := purego.Dlopen(p, purego.RTLD_NOW|purego.RTLD_GLOBAL)
			if err != nil {
				lastErr = err
				continue
			}
			slog.Info(i18n.T("log.swiftbridge_loaded"), "path", p)
			handle = h
			loadErr = nil
			registerAll(handle)
			return
		}
		if lastErr != nil {
			loadErr = fmt.Errorf(i18n.T("err.swiftbridge_purego_dlopen", "path", strings.Join(candidates, ", "), "detail", lastErr.Error()))
			slog.Warn(i18n.T("log.swiftbridge_unavailable"), "candidates", strings.Join(candidates, ", "))
		}
	})
	return loadErr
}

// writeEmbeddedDylib writes the go:embed-embedded dylib bytes to a temp file and returns its
// path.
// purego.Dlopen only supports path loading, so the embedded bytes must be written out first.
// File permissions are 0o755 so dlopen can execute it.
func writeEmbeddedDylib() (string, error) {
	if len(dylibEmbed) == 0 {
		return "", fmt.Errorf("embedded dylib is empty")
	}
	dir, err := os.MkdirTemp("", "kai-bridge-")
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, dylibName)
	if err := os.WriteFile(p, dylibEmbed, 0o755); err != nil {
		return "", err
	}
	return p, nil
}

// registerAll registers the kai_* functions one by one; a missing symbol is not fatal
// (logged then skipped), keeping the remaining functions usable.
func registerAll(h uintptr) {
	register := func(fptr any, name string) {
		if err := registerSafe(fptr, h, name); err != nil {
			loadErr = fmt.Errorf(i18n.T("err.swiftbridge_purego_register", "name", name, "detail", err.Error()))
		}
	}
	register(&KaiOCR, "kai_ocr")
	register(&KaiAccessibilityEnabled, "kai_accessibility_enabled")
	register(&KaiAccessibilityRequest, "kai_accessibility_request")
	register(&KaiScreenRecordingEnabled, "kai_screenrecording_enabled")
	register(&KaiScreenRecordingRequest, "kai_screenrecording_request")
	register(&KaiSelectionPoint, "kai_selection_point")
	register(&KaiScreenSize, "kai_screen_size")
	register(&KaiInputMonitoringEnabled, "kai_input_monitoring_enabled")
	register(&KaiAvailableLanguages, "kai_available_languages")
	register(&KaiSetLogConfig, "kai_set_log_config")
	register(&KaiSetLocale, "kai_set_locale")
	register(&KaiTranslate, "kai_translate")
	register(&KaiTranslateCancel, "kai_translate_cancel")
	register(&KaiWarmTranslate, "kai_warm_translate")
	register(&KaiDetectLanguage, "kai_detect_language")
	register(&KaiCorrect, "kai_correct")
	register(&KaiCorrectAvailability, "kai_correct_availability")
	register(&KaiDoubleCopyStart, "kai_doublecopy_start")
	register(&KaiDoubleCopyStop, "kai_doublecopy_stop")
	register(&KaiDoubleCopyPoll, "kai_doublecopy_poll")
	register(&KaiDoubleCopySuppress, "kai_doublecopy_suppress")
	register(&KaiDoubleCopyRequest, "kai_doublecopy_request")
	register(&KaiDoubleCopyPasteboard, "kai_doublecopy_pasteboard")
	register(&KaiDoubleCopyIngest, "kai_doublecopy_ingest")
}

// Available reports whether the Swift bridge loaded successfully (dylib Dlopen'ed and the
// handle valid).
// Callers must check before invoking any KaiXxx function pointer; on load failure (non-macOS,
// dylib missing/wrong path/corrupt) it returns false and call sites must degrade safely —
// never call a nil function pointer (it panics).
func Available() bool {
	return handle != 0
}

// registerSafe wraps RegisterLibFunc, converting panic/error into a returned error (a
// missing symbol doesn't panic directly).
func registerSafe(fptr any, h uintptr, name string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf(i18n.T("err.swiftbridge_purego_panic", "detail", fmt.Sprintf("%v", r)))
		}
	}()
	purego.RegisterLibFunc(fptr, h, name)
	return nil
}
