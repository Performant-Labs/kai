//go:build darwin

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// appleTranslator calls macOS's built-in system translation (Translation.framework).
// It loads the Swift bridge dynamic library at runtime via purego (pkg/swiftbridge),
// needs no API key and no accessibility permission, and is the out-of-the-box local offline
// translation backend. After changing Swift code, just rebuild internal/swift/build.sh
// (produces the .dylib and copies it into pkg/swiftbridge); the runtime Dlopen then picks up
// the latest code with no need to relink the main binary.
type appleTranslator struct{}

// NewApple creates the system translation engine.
func NewApple() Translator {
	return &appleTranslator{}
}

func (s *appleTranslator) Name() string { return "apple" }

// SetLogConfig syncs the log config (directory + LogConfig level/retention days/compression)
// to the Swift bridge layer, so its kai-bridge.log follows the same policy as the main app
// log (kai.log): level filtering, day rotation, retention days, compression.
// An empty dir skips; level is debug/info/warn/error (invalid values fall back to info on the
// Swift side); retentionDays <=0 means day-rotation only, no cleanup; compress decides
// whether expired archives are compressed to .gz.
func SetLogConfig(dir, level string, retentionDays int, compress bool) {
	if dir == "" {
		return
	}
	// Skip safely when the dylib isn't loaded (non-macOS / missing / wrong path), avoiding a
	// nil function-pointer panic.
	if !swiftbridge.Available() {
		return
	}
	// Safe int -> int32 conversion: clamp to the max on int32 overflow (gosec G115).
	days := min(retentionDays, math.MaxInt32)
	swiftbridge.KaiSetLogConfig(dir, level, int32(days), compress) //nolint:gosec // overflow is explicitly clamped above
}

// SetBridgeLocale syncs the current UI language to the Swift bridge layer so its
// kai-bridge.log debug logs switch between Chinese/English with the system language.
// locale looks like "zh-CN" / "en-US"; anything starting with "en" is treated as English.
// An empty string skips (keeping the Swift-side default of zh).
func SetBridgeLocale(locale string) {
	if locale == "" {
		return
	}
	if !swiftbridge.Available() {
		return
	}
	swiftbridge.KaiSetLocale(locale)
}

// SupportsAutoSource: system translation (Translation.framework) supports auto-detecting the
// source language. With from=auto, Go passes an empty string to Swift, which uses
// NaturalLanguage to detect the language and constrain it to the installed list.
func (s *appleTranslator) SupportsAutoSource() bool { return true }

// Translate performs the translation via Translation.framework.
// When src is "auto" (or empty), Swift auto-detects the source language with NaturalLanguage,
// constrained to the locally installed list; the target language must be explicit.
func (s *appleTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return nil, fmt.Errorf(i18n.T("err.empty_text"))
	}
	sl := normalizeLang(string(req.From))
	tl, err := normalizeTarget(string(req.To))
	if err != nil {
		return nil, err
	}
	// sl == "" means auto-detect the source language, delegated to Swift; non-empty requires
	// an explicit source.
	if tl == "auto" || tl == "" {
		return nil, fmt.Errorf(i18n.T("err.apple_need_target"))
	}

	slog.Debug(i18n.T("log.apple_translate_invoke"), "from", sl, "to", tl, "text_len", len(text))

	outBuf := make([]byte, 1<<16) // 64KB output buffer, enough for a long translation + JSON wrapping
	if !swiftbridge.Available() {
		return nil, fmt.Errorf(i18n.T("err.swiftbridge_unavailable"))
	}
	n := swiftbridge.KaiTranslate(sl, tl, text, unsafe.Pointer(&outBuf[0]), int32(len(outBuf))) //nolint:gosec // required for the Swift interop; buffer is allocated on the Go side
	if n < 0 {
		slog.Error(i18n.T("err.apple_translate_buffer"), "from", sl, "to", tl, "text_len", len(text))
		return nil, fmt.Errorf(i18n.T("err.apple_translate_buffer"))
	}

	// Trim the trailing \0 Swift may have written (C-string convention), avoiding a \x00
	// error during JSON parsing.
	payload := bytes.TrimRight(outBuf[:n], "\x00")
	var tr swiftbridge.TranslateSuccess
	if err := json.Unmarshal(payload, &tr); err != nil {
		slog.Error(i18n.T("err.apple_translate_parse"), "from", sl, "to", tl, "raw", string(payload), "error", err)
		return nil, fmt.Errorf("%s: %w", i18n.T("err.apple_translate_parse"), err)
	}
	if tr.Code != "" {
		// Swift custom error: log it here (this is the only place that knows the request's
		// languages), then render the user-visible copy by error code, with detail as the
		// technical context (appleBridgeError, untagged so its mapping is testable on any OS).
		// The bridge also reports the source language it detected before the framework failed
		// (tr.From, only for an auto source); appleBridgeError attaches it to the error so the
		// translate service can recognize a text that already is in the target language (issue #80).
		// Known codes map to err.apple_<code>; unknown codes fall back to the generic engine
		// error copy, never exposing the raw key string to the user.
		switch tr.Code {
		case swiftbridge.BridgeErrEmptyText, swiftbridge.BridgeErrTargetRequired:
			// Caller-side input problems: not logged as engine errors (as before).
		case swiftbridge.BridgeErrNoSourceLang:
			slog.Error(i18n.T("err.apple_no_source_lang"), "from", sl, "to", tl, "detail", tr.Detail)
		case swiftbridge.BridgeErrAppleTranslate:
			slog.Error(i18n.T("err.apple_translate_engine"), "from", sl, "to", tl, "detail", tr.Detail)
		default:
			slog.Error(i18n.T("err.apple_translate_engine"), "from", sl, "to", tl, "code", tr.Code, "detail", tr.Detail)
		}
		return nil, appleBridgeError(tr.Code, tr.Detail, tr.From)
	}
	if tr.Result == "" {
		slog.Error(i18n.T("err.apple_translate_empty"), "from", sl, "to", tl)
		return nil, fmt.Errorf(i18n.T("err.apple_translate_empty"))
	}
	slog.Debug(i18n.T("log.apple_translate_done"), "from", sl, "to", tl, "detected_from", tr.From, "result_len", len(tr.Result))
	return &model.TranslateResult{
		Engine: "apple",
		From:   model.Language(coalesceLang(tr.From, sl)),
		To:     model.Language(tl),
		Text:   text,
		Result: tr.Result,
	}, nil
}

// AvailableLanguages returns the language codes (BCP-47) of the system's installed language
// packs. For use by the frontend language picker etc.; returns an error on failure.
func AvailableLanguages() ([]string, error) {
	slog.Debug(i18n.T("log.apple_query_langs"))
	if !swiftbridge.Available() {
		return nil, fmt.Errorf(i18n.T("err.swiftbridge_unavailable"))
	}
	outBuf := make([]byte, 1<<16)
	n := swiftbridge.KaiAvailableLanguages(unsafe.Pointer(&outBuf[0]), int32(len(outBuf))) //nolint:gosec // required for the Swift interop; buffer is allocated on the Go side
	if n < 0 {
		slog.Error(i18n.T("err.apple_lang_buffer"))
		return nil, fmt.Errorf(i18n.T("err.apple_lang_buffer"))
	}
	// Trim the trailing \0 Swift may have written (C-string convention), avoiding a \x00
	// error during JSON parsing.
	payload := bytes.TrimRight(outBuf[:n], "\x00")
	// Swift returns {"langs":[...]} where langs are the locally installed (downloaded,
	// offline-translatable) language identifiers.
	var resp swiftbridge.AvailableLanguages
	if err := json.Unmarshal(payload, &resp); err != nil {
		slog.Error(i18n.T("err.apple_lang_parse"), "raw", string(payload), "error", err)
		return nil, fmt.Errorf("%s: %w", i18n.T("err.apple_lang_parse"), err)
	}
	slog.Info(i18n.T("log.apple_query_langs_done"), "count", len(resp.Langs), "langs", resp.Langs)
	return resp.Langs, nil
}

// coalesceLang falls back to "auto" (meaning auto-detect) when Swift didn't return a source
// language.
func coalesceLang(got, fallback string) string {
	if got == "" {
		if fallback == "" {
			return "auto"
		}
		return fallback
	}
	return got
}

// normalizeLang maps a SOURCE language to the BCP-47 code Translation.framework accepts (a
// lookup into the language capability registry; dialects alias to their base). "auto" / ""
// map to an empty string, routing Swift to its NaturalLanguage auto-detect branch.
func normalizeLang(code string) string {
	switch code {
	case "zh-TW", "zh_Hant", "zh-Hant":
		return "zh-Hant"
	}
	if isAuto(code) {
		return ""
	}
	return sourceCode("apple", code, identity)
}

// normalizeTarget maps a TARGET language to its Translation.framework code. Exact match only: a
// dialect the registry does not list for apple is refused, never sent as its base language.
// An empty result means no target was given (the caller reports err.apple_need_target).
func normalizeTarget(code string) (string, error) {
	switch code {
	case "zh-TW", "zh_Hant", "zh-Hant":
		return "zh-Hant", nil
	}
	if isAuto(code) {
		return "", nil
	}
	return targetCode("apple", code, identity)
}
