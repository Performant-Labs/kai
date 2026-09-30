//go:build !darwin

// Package swiftbridge is Kai's Swift bridge layer (a pure-Go dynamic loader + two-sided
// contract types).
//
// This file is the compile stub for non-macOS platforms (windows/linux): purego.Dlopen and
// the RTLD_* constants are Unix-only and cannot link on Windows, so the real loading logic
// is isolated in load.go (//go:build darwin).
// This file keeps only the package-level function pointer declarations matching load.go and
// an empty Init, guaranteeing:
//  1. the package compiles on non-macOS (make check-cross's GOOS=windows no longer reports
//     undefined);
//  2. swiftbridge.Init(""), which main.go calls unconditionally, is a no-op on non-macOS
//     (returns nil);
//  3. the package API signatures stay cross-platform consistent, so callers never reference
//     undefined symbols on darwin.
//
// Non-macOS platforms never load the Swift bridge anyway (all kai_* callers live in
// *_darwin.go); the function pointers here stay nil and are never invoked.
package swiftbridge

import "unsafe"

var (
	// handle is always 0 on non-macOS (see Available); all KaiXxx function pointers stay
	// nil.
	handle uintptr

	// kai_* function pointers (always nil on non-macOS, kept for signature parity only).
	KaiOCR                    func(base64 string, out unsafe.Pointer, outCap int32, correct int32, timeout int32, retry int32) int32
	KaiAccessibilityEnabled   func(out unsafe.Pointer, outCap int32) int32
	KaiAccessibilityRequest   func() int32
	KaiScreenRecordingEnabled func(out unsafe.Pointer, outCap int32) int32
	KaiScreenRecordingRequest func() int32
	KaiSelectionPoint         func(out unsafe.Pointer, outCap int32) int32
	KaiScreenSize             func(out unsafe.Pointer, outCap int32) int32
	KaiInputMonitoringEnabled func(out unsafe.Pointer, outCap int32) int32
	KaiAvailableLanguages     func(out unsafe.Pointer, outCap int32) int32
	KaiSetLogConfig           func(dir string, level string, retentionDays int32, compress bool)
	KaiSetLocale              func(locale string)
	KaiTranslate              func(src string, dst string, text string, token int64, out unsafe.Pointer, outCap int32) int32
	KaiTranslateCancel        func(token int64) int32
	KaiWarmTranslate          func(src string, dst string) int32
	KaiDetectLanguage         func(text string, out unsafe.Pointer, outCap int32) int32
	KaiCorrect                func(instructions string, text string, out unsafe.Pointer, outCap int32) int32
	KaiCorrectAvailability    func(locale string, out unsafe.Pointer, outCap int32) int32
	KaiDoubleCopyStart        func() int32
	KaiDoubleCopyStop         func() int32
	KaiDoubleCopyPoll         func(out unsafe.Pointer, outCap int32) int32
	KaiDoubleCopySuppress     func(ms int32) int32
	KaiDoubleCopyRequest      func() int32
	KaiDoubleCopyPasteboard   func(out unsafe.Pointer, outCap int32) int32
	KaiDoubleCopyIngest       func(keycode int32, flags uint64, autorepeat int32, srcPid int32) int32
	KaiLaunchObserve          func() int32
	KaiLaunchKind             func() int32
)

// Init is the non-macOS empty implementation: no dylib is loaded, nil is returned directly.
// The Swift bridge is macOS-only; on non-macOS every feature falls back to its
// per-platform implementation (or is disabled).
func Init(dylibPath string) error {
	return nil
}

// Available always returns false on non-macOS: no Swift bridge; callers should degrade
// safely.
func Available() bool {
	return handle != 0
}
