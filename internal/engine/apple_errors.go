package engine

import (
	"errors"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// appleBridgeError is the engine error for a failing Swift bridge translate call (issue #96): the
// bridge's error code becomes the localized user copy, with the bridge's technical detail after it
// as "copy (detail)". It is untagged and pure, so it is testable on any OS; apple_darwin.go calls
// it after logging the failure.
//
// The one structured signal is no_source_lang, auto-detect found no installed source pack, which is
// a language-pair problem the bridge already identifies: it wraps ErrUnsupportedPair, so the
// classifier reports "pair" without reading text. apple_translate carries the OS's own
// localizedDescription as its detail, so it stays text-only: the classifier's substring fallback
// catches "Unable to Translate" on an English macOS (a Swift-side fix is a follow-up, brief D6d).
// Any other code falls back to the generic engine copy, never exposing the raw code to the user.
func appleBridgeError(code, detail string) error {
	var msg string
	var cause error
	switch code {
	case swiftbridge.BridgeErrEmptyText:
		msg = i18n.T("err.apple_empty_text")
	case swiftbridge.BridgeErrTargetRequired:
		msg = i18n.T("err.apple_target_required")
	case swiftbridge.BridgeErrNoSourceLang:
		msg = i18n.T("err.apple_no_source_lang")
		cause = ErrUnsupportedPair
	default: // BridgeErrAppleTranslate and unknown codes
		msg = i18n.T("err.apple_translate_engine")
	}
	if detail != "" {
		msg = msg + " (" + detail + ")"
	}
	if cause != nil {
		return withText(msg, cause)
	}
	return errors.New(msg)
}
