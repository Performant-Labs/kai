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

// warmTranslatePair decides whether (src, dst) is warmable and, if so, normalizes it to the
// BCP-47 codes the Swift bridge accepts. Pulled out of WarmTranslate as a pure function (no
// swiftbridge dependency) so this decision is unit-testable without a loaded dylib — see
// apple_darwin_test.go. src="auto" (or empty: no fixed source) is not warmable and is
// reported via ok=false, never passed through as the literal string "auto"; likewise an
// empty or unsupported dst.
func warmTranslatePair(src, dst string) (sl, tl string, ok bool) {
	// normalizeLang returns "" for "auto"/empty (no fixed source to warm); normalizeTarget
	// likewise returns "" (no error) for "auto"/empty, and an error for an unsupported target.
	sl = normalizeLang(src)
	if sl == "" || dst == "" {
		return "", "", false
	}
	tl, err := normalizeTarget(dst)
	if err != nil || tl == "" {
		return "", "", false
	}
	return sl, tl, true
}

// WarmTranslate warms the Apple engine's cached TranslationSession for (src, dst) — issue
// #173 item 8. Intended to be called once at launch, in its own goroutine (main.go), for the
// user's current default_from/default_to language pair, so the first real translate() call
// for that pair skips the prepareTranslation() cost.
// Best-effort: swallows a missing dylib (Available()==false) and any Swift-side failure
// (e.g. the pair's language pack isn't installed) without returning an error — a failed warm
// never blocks or breaks translation, it just means the first call pays the usual cost.
func WarmTranslate(src, dst string) {
	if !swiftbridge.Available() || swiftbridge.KaiWarmTranslate == nil {
		return
	}
	sl, tl, ok := warmTranslatePair(src, dst)
	if !ok {
		return
	}
	swiftbridge.KaiWarmTranslate(sl, tl)
}

// SupportsAutoSource: system translation (Translation.framework) supports auto-detecting the
// source language. With from=auto, Go passes an empty string to Swift, which uses
// NaturalLanguage to detect the language and constrain it to the installed list.
func (s *appleTranslator) SupportsAutoSource() bool { return true }

// Translate performs the translation via Translation.framework.
// When src is "auto" (or empty), Swift auto-detects the source language with NaturalLanguage,
// constrained to the locally installed list; the target language must be explicit.
//
// The call has no time limit of its own (issue #111): it returns when Apple is done, or when ctx
// ends. A ctx that ends first cancels the Swift call (runCancellable) and Translate returns
// context.Cause(ctx), never error copy: the translate service decides cancelled or superseded from
// that cause. The framework itself keeps working on the abandoned text for a while and queues the
// next request behind it, which nothing here can shorten.
//
// Issue #173 item 1, investigated and NOT fixed here: a screenshot showed a blank line
// inserted after every line of a 9-line System-engine result. Neither this file, the Swift
// bridge (apple_translate.swift), the Go translate service, nor the frontend split or
// rejoin lines/paragraphs for a call this size — the #84 chunker in service_chunk.go only
// runs for over-budget (long) text, which a 9-line input is not, and its sepAfter/join logic
// is not on this call path at all. req.Text is passed to Swift verbatim (see kai_translate
// above) and session.translate(inputText)'s targetText is returned verbatim, with no
// post-processing on either side. The extra blank lines therefore come from
// TranslationSession.translate itself reflowing multi-paragraph input — an on-device
// Translation.framework behavior, not a Kai bug — and there is no Kai-side hook to suppress
// it (nothing else engine-side reflows or joins the framework's own targetText). Left
// undone deliberately; re-open if Apple ever exposes a per-line/no-reflow translation mode.
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
	// KaiTranslateCancel comes from the same bridge build as the token-taking KaiTranslate (issue
	// #111). A library without it is a stale build whose kai_translate has the old argument list, and
	// calling that with the new one would hand it the token where it expects the output buffer, so
	// such a library counts as unavailable.
	if !swiftbridge.Available() || swiftbridge.KaiTranslateCancel == nil {
		return nil, fmt.Errorf(i18n.T("err.swiftbridge_unavailable"))
	}
	// The call has no timer of its own: it returns when Apple is done, or as soon as ctx ends
	// (issue #111). runCancellable then asks the bridge to cancel the call named by this token; the
	// bridge answers at once and the framework unwinds the abandoned work in the background.
	token := nextAppleToken()
	var n int32
	payload, err := runCancellable(ctx, token, func() []byte {
		n = swiftbridge.KaiTranslate(sl, tl, text, token, unsafe.Pointer(&outBuf[0]), int32(len(outBuf))) //nolint:gosec // required for the Swift interop; buffer is allocated on the Go side
		if n < 0 {
			return nil
		}
		return outBuf[:n]
	}, func(token int64) { swiftbridge.KaiTranslateCancel(token) })
	if err != nil {
		// ctx had already ended: the bridge was never called, and the error is the ctx's cause,
		// never error copy (the translate service decides cancelled or superseded from it).
		return nil, err
	}
	if n < 0 {
		slog.Error(i18n.T("err.apple_translate_buffer"), "from", sl, "to", tl, "text_len", len(text))
		return nil, fmt.Errorf(i18n.T("err.apple_translate_buffer"))
	}

	// Trim the trailing \0 Swift may have written (C-string convention), avoiding a \x00
	// error during JSON parsing.
	payload = bytes.TrimRight(payload, "\x00")
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
		case swiftbridge.BridgeErrCancelled:
			// A cancel is not an engine failure, so it is not an error line (the translate service
			// logs one the user asked for). One nobody asked for still becomes an engine error
			// below, and the service logs that as a failure.
			slog.Debug(i18n.T("log.translate_engine_cancelled"), "engine", "apple", "from", sl, "to", tl, "detail", tr.Detail)
		default:
			slog.Error(i18n.T("err.apple_translate_engine"), "from", sl, "to", tl, "code", tr.Code, "detail", tr.Detail)
		}
		// A cancelled payload that ctx asked for ends as the ctx's cause (appleCancelOutcome, the
		// one place that decides), never as error copy.
		if cancelErr, handled := appleCancelOutcome(ctx, tr.Code); handled {
			return nil, cancelErr
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

// DetectLanguage detects the language of text locally with NaturalLanguage (issue #200) and
// returns it as the bare language code (es, zh) with its confidence in [0, 1]. ok is false
// when the text is blank, the Swift bridge is not loaded, or nothing was recognized. The Swift
// entry point is a pure synchronous computation (no AppKit, no main-thread rule), so this may be
// called from any goroutine.
func DetectLanguage(text string) (model.Language, float64, bool) {
	if strings.TrimSpace(text) == "" || !swiftbridge.Available() || swiftbridge.KaiDetectLanguage == nil {
		return "", 0, false
	}
	outBuf := make([]byte, 1<<10)
	n := swiftbridge.KaiDetectLanguage(text, unsafe.Pointer(&outBuf[0]), int32(len(outBuf))) //nolint:gosec // required for the Swift interop; buffer is allocated on the Go side
	if n < 0 {
		return "", 0, false
	}
	return parseDetection(outBuf[:n])
}
