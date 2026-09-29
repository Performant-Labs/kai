// This file is the authoritative two-sided contract for the Kai Swift bridge's (cgo) error
// codes and JSON structures.
//
// The Go error-code constants are defined here exactly once (exported) for the
// internal/engine package to reference;
// the Swift BRIDGE_ERR_* constants (internal/swift/bridge_errors.swift) must be identical
// literals.
// When adding/changing any error code, three places must be synced — all of them:
//   1. this file's BridgeErr* constants (the Go definition)
//   2. kai_bridge.swift's BRIDGE_ERR_* constants (Swift; literals must match)
//   3. the err.apple_<code> keys in internal/i18n/locales/split/
//      — Apple system-level errors (apple_translate / apple_ocr) reuse the generic engine
//      copy; no new keys.
//      — cancelled has no key and no copy, on purpose: a cancel is reported through the engine's
//      context (context.Cause), never through text. The Go engine decides from its ctx whether
//      anybody asked for it (appleCancelOutcome); a cancelled payload nobody asked for is an
//      ordinary engine error through appleBridgeError's default branch.

package swiftbridge

// Error-code constants: literals must match the Swift side's BRIDGE_ERR_* exactly.
const (
	BridgeErrEmptyText          = "empty_text"           // Swift: BRIDGE_ERR_EMPTY_TEXT  -> err.apple_empty_text
	BridgeErrTargetRequired     = "target_required"      // Swift: BRIDGE_ERR_TARGET_REQUIRED -> err.apple_target_required
	BridgeErrNullImage          = "null_image"           // Swift: BRIDGE_ERR_NULL_IMAGE  -> err.apple_null_image
	BridgeErrDecodeFailed       = "decode_failed"        // Swift: BRIDGE_ERR_DECODE_FAILED -> err.apple_decode_failed
	BridgeErrEmptyImage         = "empty_image"          // Swift: BRIDGE_ERR_EMPTY_IMAGE -> err.apple_empty_image
	BridgeErrBitmapCtxFailed    = "bitmap_ctx_failed"    // Swift: BRIDGE_ERR_BITMAP_CTX_FAILED -> err.apple_bitmap_ctx_failed
	BridgeErrBitmapRedrawFailed = "bitmap_redraw_failed" // Swift: BRIDGE_ERR_BITMAP_REDRAW_FAILED -> err.apple_bitmap_redraw_failed
	BridgeErrOcrTimeout         = "ocr_timeout"          // Swift: BRIDGE_ERR_OCR_TIMEOUT -> err.apple_ocr_timeout
	BridgeErrNoSourceLang       = "no_source_lang"       // Swift: BRIDGE_ERR_NO_SOURCE_LANG -> err.apple_no_source_lang
	BridgeErrAppleTranslate     = "apple_translate"      // Swift: BRIDGE_ERR_APPLE_TRANSLATE (system-level; reuses err.apple_translate_engine)
	BridgeErrAppleOcr           = "apple_ocr"            // Swift: BRIDGE_ERR_APPLE_OCR (system-level; reuses err.vision_ocr_engine)
	BridgeErrCancelled          = "cancelled"            // Swift: BRIDGE_ERR_CANCELLED (kai_translate cancelled by kai_translate_cancel; no copy: reported through the ctx)
)

// Swift-side constants (mirror; the real definitions live in
// internal/swift/bridge_errors.swift):
//
//	let BRIDGE_ERR_EMPTY_TEXT          = "empty_text"
//	let BRIDGE_ERR_TARGET_REQUIRED     = "target_required"
//	let BRIDGE_ERR_NULL_IMAGE          = "null_image"
//	let BRIDGE_ERR_DECODE_FAILED       = "decode_failed"
//	let BRIDGE_ERR_EMPTY_IMAGE         = "empty_image"
//	let BRIDGE_ERR_BITMAP_CTX_FAILED   = "bitmap_ctx_failed"
//	let BRIDGE_ERR_BITMAP_REDRAW_FAILED= "bitmap_redraw_failed"
//	let BRIDGE_ERR_OCR_TIMEOUT         = "ocr_timeout"
//	let BRIDGE_ERR_NO_SOURCE_LANG      = "no_source_lang"
//	let BRIDGE_ERR_APPLE_TRANSLATE     = "apple_translate"
//	let BRIDGE_ERR_APPLE_OCR           = "apple_ocr"
//	let BRIDGE_ERR_CANCELLED           = "cancelled"

// ---------------------------------------------------------------------------
// Go struct mirrors of the returned JSON (one-to-one with the Swift Codable struct fields)
// ---------------------------------------------------------------------------
// Each Go struct's json tags must exactly match the corresponding Swift Codable struct's
// field names;
// success structs embed BridgeError so the same JSON yields either the success payload or
// code/detail on failure.
// Every detail field is "non-translatable technical context" (sizes / raw system errors /
// language identifiers);
// user-visible copy is rendered by the Go side's err.apple_* i18n, and the two are joined as
// "user copy (technical detail)".

// BridgeError mirrors Swift BridgeError: every failing function returns
// {"code":...,"detail":...}.
// Success JSON lacks both fields, parsing to empty strings (safe).
// A failing kai_translate call may also return "from" (issue #80): the bare language code the
// bridge detected in the text before the framework failed, only for an auto source. It parses into
// TranslateSuccess.From below, the same field the success payload uses, so the struct needs no
// change.
type BridgeError struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// TranslateSuccess mirrors Swift TranslateSuccess: {"result":...,"from":...}.
// Embeds BridgeError so the same JSON yields the success payload or code/detail on failure.
type TranslateSuccess struct {
	Result string `json:"result"`
	From   string `json:"from"`
	BridgeError
}

// AvailableLanguages mirrors Swift AvailableLanguages: {"langs":[...]}.
type AvailableLanguages struct {
	Langs []string `json:"langs"`
}

// DetectedLanguage mirrors Swift DetectedLanguage (kai_detect_language, issue #200):
// {"lang":"es","confidence":0.99}. Lang is empty when nothing was recognized.
type DetectedLanguage struct {
	Lang       string  `json:"lang"`
	Confidence float64 `json:"confidence"`
}

// SelectionPoint mirrors Swift SelectionPoint: {"x":0,"y":0}.
type SelectionPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// ScreenSize mirrors Swift ScreenSize: {"w":0,"h":0}.
type ScreenSize struct {
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// OCRRegion mirrors Swift OCRRegion: {"text":...,"conf":0,"box":[x1,y1,x2,y2]}.
type OCRRegion struct {
	Text string  `json:"text"`
	Conf float64 `json:"conf"`
	Box  []int   `json:"box"`
}

// OCRSuccess mirrors Swift OCRSuccess: {"text":...,"regions":[...]}.
// Embeds BridgeError so the same JSON yields the recognition result or code/detail on
// failure.
type OCRSuccess struct {
	Text    string      `json:"text"`
	Regions []OCRRegion `json:"regions"`
	BridgeError
}
