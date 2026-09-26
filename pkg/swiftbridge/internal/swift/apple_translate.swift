// apple_translate.swift
// System translation @_cdecl entries: kai_translate / kai_available_languages.
// Depends on bridge_errors.swift (error codes / Codable / encoding helpers) and
// bridge_common.swift (detectSourceLanguage),
// bridge_log.swift（bridgeFileLog / bridgeLogText）。
import AppKit
import ApplicationServices
import CoreGraphics
import Foundation
import NaturalLanguage
import Translation
import Vision

// kai_translate: performs one system translation synchronously.
// src/dst are BCP-47 codes (e.g. "en" / "zh-Hans"); src may be an empty string for
// auto-detect.
// out receives the JSON {"result":"...","from":"..."} (TranslateSuccess); on failure out gets
// {"code":"...","detail":"..."} (BridgeError), plus "from" (the language NaturalLanguage detected)
// when the source was auto and a language was detected before the framework failed (issue #80).
@_cdecl("kai_translate")
public func kai_translate(
  _ src: UnsafePointer<CChar>?,
  _ dst: UnsafePointer<CChar>?,
  _ text: UnsafePointer<CChar>?,
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let sourceCode = src.flatMap { String(cString: $0) } ?? ""
  let targetCode = dst.flatMap { String(cString: $0) } ?? ""
  let inputText = text.flatMap { String(cString: $0) } ?? ""

  bridgeFileLog(bridgeLogText("translate.call", sourceCode, targetCode, inputText.utf8.count))

  guard !inputText.isEmpty else {
    bridgeFileLog(bridgeLogText("translate.empty"), level: BRIDGE_LOG_WARN)
    // detail: the input text length (pre-trim), helping the Go side debug empty inputs.
    return writeCString(
      bridgeErrorJSON(
        code: BRIDGE_ERR_EMPTY_TEXT,
        detail: "input text is empty (len=0)"), into: out, cap: out_cap)
  }
  guard !targetCode.isEmpty else {
    bridgeFileLog(bridgeLogText("translate.no_target"), level: BRIDGE_LOG_WARN)
    return writeCString(
      bridgeErrorJSON(
        code: BRIDGE_ERR_TARGET_REQUIRED,
        detail: "target language code is empty"), into: out, cap: out_cap)
  }

  let targetLang = Locale.Language(identifier: targetCode)

  let sema = DispatchSemaphore(value: 0)
  var resultJSON = "{}"

  Task {
    let availability = LanguageAvailability()
    let installedAll = await availability.supportedLanguages
    var installed: [String] = []
    for lang in installedAll {
      if await availability.status(from: lang, to: nil) == .installed {
        installed.append(lang.maximalIdentifier)
      }
    }

    var effectiveSource = sourceCode
    var detectedLang: String? = nil
    if sourceCode.isEmpty {
      if let detected = detectSourceLanguage(
        inputText, installed: installed, detectedLang: &detectedLang)
      {
        effectiveSource = detected
        bridgeFileLog(bridgeLogText("translate.detect_src", effectiveSource, targetCode))
      } else {
        bridgeFileLog(bridgeLogText("translate.detect_fail", targetCode), level: BRIDGE_LOG_ERROR)
      }
    }
    // The language NaturalLanguage detected, as the bare language code the success path also
    // reports as `from` (zh-Hans becomes zh), for the two failure paths below (issue #80). It is
    // the raw detection, not the installed-language fallback in effectiveSource, which is a guess:
    // Go treats a detection equal to the target as "the text is already in the target language".
    // detectedLang is only ever set for an auto source, so a pinned source reports none.
    let detectedFrom: String? = detectedLang.flatMap {
      Locale.Language(identifier: $0).languageCode?.identifier
    }
    guard !effectiveSource.isEmpty else {
      let installedDesc =
        installed.isEmpty
        ? "no installed language pack" : "installed: \(installed.joined(separator: ", "))"
      let detectedDesc = detectedLang.map { "detected source: \($0)" } ?? ""
      let detail =
        "no installed source language for auto-detect\(detectedDesc). \(installedDesc). download the language pack in system settings > general > language & region > translate."
      bridgeFileLog(bridgeLogText("translate.fail", detail), level: BRIDGE_LOG_ERROR)
      // Returns a structured error code; the Go side's err.apple_no_source_lang renders the
      // user-visible copy;
      // detail is appended purely as technical context (including Apple language identifiers —
      // not translatable copy).
      resultJSON = bridgeErrorJSON(
        code: BRIDGE_ERR_NO_SOURCE_LANG, detail: detail, from: detectedFrom)
      sema.signal()
      return
    }
    let sourceLang = Locale.Language(identifier: effectiveSource)
    let session = TranslationSession(installedSource: sourceLang, target: targetLang)
    do {
      try await session.prepareTranslation()
      let resp = try await session.translate(inputText)
      let from = resp.sourceLanguage.languageCode?.identifier ?? sourceCode
      resultJSON = bridgeEncode(TranslateSuccess(result: resp.targetText, from: from))
      bridgeFileLog(bridgeLogText("translate.done", from, targetCode, resp.targetText.utf8.count))
    } catch {
      // Apple system-level translation errors: uniformly shaped as
      // {"code":"apple_translate","detail":...},
      // rendered by the Go side's err.apple_translate_engine; detail carries the system's
      // localizedDescription. The framework refuses to translate a language into itself with this
      // same error, so the detected language rides along (from) for the Go side to tell that
      // apart from a real failure.
      let detail = error.localizedDescription
      resultJSON = bridgeErrorJSON(
        code: BRIDGE_ERR_APPLE_TRANSLATE, detail: detail, from: detectedFrom)
      bridgeFileLog(bridgeLogText("translate.fail", detail), level: BRIDGE_LOG_ERROR)
    }
    sema.signal()
  }

  _ = sema.wait(timeout: .now() + 20)
  return writeCString(resultJSON, into: out, cap: out_cap)
}

// kai_available_languages: queries locally downloaded (installed, offline-translatable)
// languages via LanguageAvailability, returning {"langs":[...]}.
@_cdecl("kai_available_languages")
public func kai_available_languages(
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let sema = DispatchSemaphore(value: 0)
  var resultJSON = "{\"langs\":[]}"

  Task {
    let availability = LanguageAvailability()
    let all = await availability.supportedLanguages
    var installed: [String] = []
    for lang in all {
      let status = await availability.status(from: lang, to: nil)
      if status == .installed {
        installed.append(lang.maximalIdentifier)
      }
    }
    resultJSON = bridgeEncode(AvailableLanguages(langs: installed))
    bridgeFileLog(
      bridgeLogText("lang.query_done", all.count, installed.count), level: BRIDGE_LOG_DEBUG)
    sema.signal()
  }

  _ = sema.wait(timeout: .now() + 30)
  return writeCString(resultJSON, into: out, cap: out_cap)
}
