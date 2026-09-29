// bridge_errors.swift
// The Swift bridge layer's two-sided contract: error-code constants + Codable models of the
// outward JSON.
// One-to-one with the Go side's pkg/swiftbridge/bridge_errors.go (literals / field names must
// match).
//
// When adding/changing any error code, three places must be synced — all of them:
//   1. the Go side's BridgeErr* constants in bridge_errors.go (the real definitions)
//   2. this file's BRIDGE_ERR_* constants (Swift literals must match)
//   3. the err.apple_<code> keys in internal/i18n/locales/split/
//      — Apple system-level errors (apple_translate / apple_ocr) reuse the generic engine
//      copy; no new keys.
//      — cancelled has no copy at all: a cancel is reported through the Go engine's context, not
//      through text (see bridge_errors.go).
import Foundation

// The Swift bridge layer's custom error codes (literals matching the Go side's BridgeErr*
// constants).
let BRIDGE_ERR_EMPTY_TEXT: String = "empty_text"
let BRIDGE_ERR_TARGET_REQUIRED: String = "target_required"
let BRIDGE_ERR_NULL_IMAGE: String = "null_image"
let BRIDGE_ERR_DECODE_FAILED: String = "decode_failed"
let BRIDGE_ERR_EMPTY_IMAGE: String = "empty_image"
let BRIDGE_ERR_BITMAP_CTX_FAILED: String = "bitmap_ctx_failed"
let BRIDGE_ERR_BITMAP_REDRAW_FAILED: String = "bitmap_redraw_failed"
let BRIDGE_ERR_OCR_TIMEOUT: String = "ocr_timeout"
let BRIDGE_ERR_NO_SOURCE_LANG: String = "no_source_lang"
// Apple system-level translation/recognition errors (e.g. localized errors thrown by
// Translation.framework / Vision).
// detail carries the system's localizedDescription — not translatable copy, technical context
// only.
let BRIDGE_ERR_APPLE_TRANSLATE: String = "apple_translate"
let BRIDGE_ERR_APPLE_OCR: String = "apple_ocr"
// kai_translate ended because kai_translate_cancel asked it to (issue #111), not because the
// translation failed. The Go engine decides from its own context whether anybody asked; the code
// carries no user-visible copy.
let BRIDGE_ERR_CANCELLED: String = "cancelled"
// kai_correct (issue #208): the on-device model declined, failed, or did not answer in time. None
// carries user-visible copy: the text is translated as it came.
let BRIDGE_ERR_CORRECT_REFUSED: String = "correct_refused"
let BRIDGE_ERR_CORRECT_FAILED: String = "correct_failed"
let BRIDGE_ERR_CORRECT_TIMEOUT: String = "correct_timeout"

// MARK: - Codable models of the bridge's returned JSON
// All outward (cgo) JSON is uniformly encoded with Codable structs + JSONEncoder;
// field names must exactly match the Go side's corresponding struct json tags (see
// bridge_errors.go).
// Note: the cgo boundary only passes C types, so Swift organizes data in structs internally,
// then encodes to a String written back into the out buffer.

/// Error return: {"code":"...","detail":"..."} (detail is non-translatable technical
/// context). A failing translate call can also carry "from" (issue #80): the bare language code
/// NaturalLanguage detected in the text before the framework failed, the same shape the success
/// payload reports. It is left out of the JSON (a nil Optional is not encoded) when the source was
/// pinned or nothing was detected, so every other error keeps its two-field shape.
struct BridgeError: Codable {
  let code: String
  let detail: String
  var from: String? = nil
}

/// Translation success: {"result":"...","from":"..."}
struct TranslateSuccess: Codable {
  let result: String
  let from: String
}

/// Available languages: {"langs":["...","..."]}
struct AvailableLanguages: Codable {
  let langs: [String]
}

/// Selection anchor: {"x":0,"y":0}
struct SelectionPoint: Codable {
  let x: Double
  let y: Double
}

/// Primary screen size: {"w":0,"h":0}
struct ScreenSize: Codable {
  let w: Double
  let h: Double
}

/// One OCR region: {"text":"...","conf":0,"box":[x1,y1,x2,y2]}
struct OCRRegion: Codable {
  let text: String
  let conf: Double
  let box: [Int]
}

/// OCR success: {"text":"...","regions":[...]}
struct OCRSuccess: Codable {
  let text: String
  let regions: [OCRRegion]
}

/// Encodes any Codable into a compact JSON string; on failure falls back to the empty
/// object "{}" (the Go side handles it as the default error copy).
func bridgeEncode<T: Codable>(_ value: T) -> String {
  guard let data = try? JSONEncoder().encode(value),
    let str = String(data: data, encoding: .utf8)
  else {
    return "{}"
  }
  return str
}

/// Convenience builder for error JSON strings. from is only passed by kai_translate (see
/// BridgeError).
func bridgeErrorJSON(code: String, detail: String, from: String? = nil) -> String {
  bridgeEncode(BridgeError(code: code, detail: detail, from: from))
}
