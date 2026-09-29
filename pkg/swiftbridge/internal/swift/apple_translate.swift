// apple_translate.swift
// System translation @_cdecl entries: kai_translate / kai_translate_cancel / kai_available_languages.
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

// MARK: - Translate jobs (issue #111)
//
// kai_translate has no timer. A translation takes as long as the framework needs (on an M1 Max about
// 12 ms per Latin character and 36 per CJK character, see docs/engine-limits.md, so minutes for a
// long text), and the only ways out of the wait are the Task's own result and kai_translate_cancel,
// which the Go engine calls when the user cancels. Each call therefore gets a TranslateJob,
// registered under the token the Go side chose, so a cancel can find it.
//
// Lock order, everywhere: the registry lock, then a job's lock, never the reverse. The Task's
// completion path (TranslateJob.finish) only ever takes the job lock. Task.cancel() and
// bridgeFileLog are never called with either lock held.

/// One in-flight kai_translate call. Two things can end it: the translation Task reporting through
/// finish, and kai_translate_cancel calling cancel. Whichever writes first wins and the other write
/// is dropped, so the caller reads exactly one result, under the lock, and never races the Task.
/// Every mutable field is guarded by lock, which is what makes @unchecked Sendable true.
final class TranslateJob: @unchecked Sendable {
  /// Signalled once, by the first result. kai_translate waits on it with no timer.
  let sema = DispatchSemaphore(value: 0)

  private let lock = NSLock()
  private var result: String? = nil
  private var cancelled = false
  private var cancelledAt: Date? = nil
  private var task: Task<Void, Never>? = nil

  /// Stores the first result and wakes the caller; a later result is dropped. Returns whether this
  /// one was stored.
  @discardableResult
  func finish(_ json: String) -> Bool {
    lock.lock()
    let first = (result == nil)
    if first { result = json }
    lock.unlock()
    if first { sema.signal() }
    return first
  }

  /// The one result, read under the lock ("{}" only if none exists, which kai_translate never asks).
  func take() -> String {
    lock.lock()
    defer { lock.unlock() }
    return result ?? "{}"
  }

  /// Hands the job its Task, right after the Task was created. A cancel that got in before this
  /// point cancels the Task here, so a cancel can never overtake the Task's creation.
  func attach(_ newTask: Task<Void, Never>) {
    lock.lock()
    let alreadyCancelled = cancelled
    task = newTask
    lock.unlock()
    if alreadyCancelled { newTask.cancel() }
  }

  /// Answers the caller at once with the cancelled result and asks the Task to stop. The Task keeps
  /// running until the framework lets go of it (measured: tens of seconds for a long text, during
  /// which the next translation request queues behind it); its late result is dropped.
  func cancel() {
    lock.lock()
    if !cancelled {
      cancelled = true
      cancelledAt = Date()
    }
    let running = task
    lock.unlock()
    finish(translateCancelledJSON("cancelled by kai_translate_cancel"))
    // Cancellation handlers run on the cancelling thread, so this is outside the lock.
    running?.cancel()
  }

  /// Called when the Task ends, whichever way. After a cancel it records how long the abandoned work
  /// took to unwind, which is how long the framework stays busy for the next request.
  func noteTaskEnded() {
    lock.lock()
    let at = cancelledAt
    lock.unlock()
    guard let at = at else { return }
    bridgeFileLog(
      bridgeLogText("translate.drain", String(format: "%.1f", Date().timeIntervalSince(at))))
  }
}

/// The cancelled result: {"code":"cancelled","detail":"..."}. It carries no user copy; the Go engine
/// turns it into the request's own cancel cause when it asked for the cancel.
func translateCancelledJSON(_ detail: String) -> String {
  bridgeErrorJSON(code: BRIDGE_ERR_CANCELLED, detail: detail)
}

/// The jobs of the kai_translate calls in flight, by token, and the tombstones of cancels that
/// arrived before their call did.
final class TranslateRegistry: @unchecked Sendable {
  /// A cancel for an unknown token is remembered so the call it raced ahead of can be answered.
  /// The list is bounded: the one cancel that is never consumed (its call had already finished)
  /// costs one Int64 until it is evicted.
  static let tombstoneCap = 256

  private let lock = NSLock()
  private var jobs: [Int64: TranslateJob] = [:]
  private var tombstones: [Int64] = []  // oldest first

  /// Starts a call. One critical section decides between the two outcomes, so a cancel cannot fall
  /// between the check and the insert: either the tombstone of a cancel that got here first is
  /// consumed (nil: the caller answers cancelled without translating), or a new job is registered
  /// where cancel can find it. A token of 0 or less registers nothing and can never be cancelled.
  func begin(_ token: Int64) -> TranslateJob? {
    lock.lock()
    defer { lock.unlock() }
    if token > 0, let i = tombstones.firstIndex(of: token) {
      tombstones.remove(at: i)
      return nil
    }
    let job = TranslateJob()
    if token > 0 { jobs[token] = job }
    return job
  }

  /// Ends a call: its job is no longer cancellable.
  func end(_ token: Int64) {
    guard token > 0 else { return }
    lock.lock()
    jobs[token] = nil
    lock.unlock()
  }

  /// Cancels the call running under token. true: a running call was found and cancelled. false: none
  /// was found, and the token is remembered (a tombstone) for a call that has not begun yet.
  func cancel(_ token: Int64) -> Bool {
    guard token > 0 else { return false }
    lock.lock()
    let job = jobs[token]
    if job == nil && !tombstones.contains(token) {
      tombstones.append(token)
      if tombstones.count > Self.tombstoneCap { tombstones.removeFirst() }
    }
    lock.unlock()
    job?.cancel()
    return job != nil
  }
}

let translateRegistry = TranslateRegistry()

// MARK: - Session cache (issue #173 item 8)
//
// Before this, kai_translate created a brand-new TranslationSession and called
// prepareTranslation() on it on every single call — confirmed by measurement (see the
// "translate.timing" log line kai_translate emits below) to be the dominant cost of a
// System-engine translation, well above session.translate() itself for short text.
//
// TranslationSessionCache caches one prepared session per (source, target) pair and reuses
// it across calls, and also warms (creates + prepares) a session ahead of time via warm().
// It is a Swift actor specifically to make reuse safe: Apple's docs do not guarantee
// TranslationSession is safe to use from two concurrent translate() calls, and an actor's
// default isolation serializes every call into this cache — including two concurrent
// session(...) or translate(...) calls for the SAME pair — so two overlapping requests queue
// behind each other instead of racing inside one TranslationSession. Two DIFFERENT pairs also
// queue behind each other under this design (a deliberate, conservative trade against the
// unknown concurrent-use risk); Apple on-device translation is fast enough per-call (see
// docs/engine-limits.md) that serializing same-process Apple-engine calls is not expected to
// be user-visible, and it is far cheaper than a re-entrancy bug.
// SessionEntry owns exactly one (source, target) pair's TranslationSession. It is an actor so
// that two overlapping calls for THIS SAME pair serialize (never call into one
// TranslationSession instance concurrently — Apple does not document that as safe). Its own
// turn only ever runs this one pair's work, so a stuck translate() here never delays a
// different pair — see TranslationSessionCache's doc comment below for why that distinction
// matters (a PR-review finding, reproduced live: a >10 minute translate() during this PR's
// hand-testing).
actor SessionEntry {
  private let source: Locale.Language
  private let target: Locale.Language
  private var session: TranslationSession?

  init(source: Locale.Language, target: Locale.Language) {
    self.source = source
    self.target = target
  }

  private func preparedSession() async throws -> TranslationSession {
    if let existing = session {
      return existing
    }
    let s = TranslationSession(installedSource: source, target: target)
    try await s.prepareTranslation()
    session = s
    return s
  }

  /// Prepares the session if it is not already cached, discarding the result.
  func warm() async {
    _ = try? await preparedSession()
  }

  /// Translates text using the cached (or newly prepared) session. Returns the response plus
  /// a (getSessionMs, translateMs) timing breakdown (issue #173 item 8's real measurement).
  func translate(text: String) async throws -> (
    response: TranslationSession.Response, getSessionMs: Int, translateMs: Int
  ) {
    let sessionStart = Date()
    let s = try await preparedSession()
    let getSessionMs = Int(Date().timeIntervalSince(sessionStart) * 1000)
    let translateStart = Date()
    let response = try await s.translate(text)
    let translateMs = Int(Date().timeIntervalSince(translateStart) * 1000)
    return (response, getSessionMs, translateMs)
  }
}

// TranslationSessionCache maps a (source, target) pair to its own SessionEntry (issue #173
// item 8). Deliberately NOT itself an actor holding all sessions: an earlier revision of this
// cache was one actor serializing every pair's calls behind a single queue, and PR review
// correctly flagged that session.translate() is unbounded and not cancellable (issue #111), so
// one stuck call for pair A would head-of-line-block every later call for pair B too — this
// PR's own hand-testing reproduced exactly that (a translate() still running after 10+
// minutes). Splitting into one SessionEntry actor per pair keeps the safety property that
// matters (two overlapping calls for the SAME pair never touch one TranslationSession
// concurrently) without serializing unrelated pairs against each other.
//
// entries is protected by a plain NSLock, not actor isolation: lookups/inserts here are
// synchronous and hold the lock only for a dictionary access, never across an await — so a
// plain lock is enough and avoids adding yet another actor hop to every call.
final class TranslationSessionCache: @unchecked Sendable {
  static let shared = TranslationSessionCache()

  private let lock = NSLock()
  private var entries: [String: SessionEntry] = [:]

  private func entry(source: Locale.Language, target: Locale.Language) -> SessionEntry {
    let key = "\(source.maximalIdentifier)->\(target.maximalIdentifier)"
    lock.lock()
    defer { lock.unlock() }
    if let existing = entries[key] {
      return existing
    }
    let e = SessionEntry(source: source, target: target)
    entries[key] = e
    return e
  }

  /// Warms the cache for (source, target): see SessionEntry.warm.
  func warm(source: Locale.Language, target: Locale.Language) async {
    await entry(source: source, target: target).warm()
  }

  /// Translates text using the cached (or newly prepared) session for (source, target): see
  /// SessionEntry.translate. Two calls for different pairs run fully concurrently (different
  /// SessionEntry actors); two calls for the same pair serialize on that pair's actor.
  func translate(source: Locale.Language, target: Locale.Language, text: String) async throws
    -> (response: TranslationSession.Response, getSessionMs: Int, translateMs: Int)
  {
    try await entry(source: source, target: target).translate(text: text)
  }
}

// kai_translate: performs one system translation synchronously.
// src/dst are BCP-47 codes (e.g. "en" / "zh-Hans"); src may be an empty string for
// auto-detect.
// token names this call for kai_translate_cancel (issue #111); the Go side allocates one per call,
// counting up from 1. A token of 0 or less can not be cancelled.
// The call waits for the translation with no timer: it returns when the translation is done or
// when kai_translate_cancel cancels it.
// out receives the JSON {"result":"...","from":"..."} (TranslateSuccess); on failure out gets
// {"code":"...","detail":"..."} (BridgeError), plus "from" (the language NaturalLanguage detected)
// when the source was auto and a language was detected before the framework failed (issue #80). A
// cancelled call gets {"code":"cancelled",...} and translates nothing further.
@_cdecl("kai_translate")
public func kai_translate(
  _ src: UnsafePointer<CChar>?,
  _ dst: UnsafePointer<CChar>?,
  _ text: UnsafePointer<CChar>?,
  _ token: Int64,
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

  // A cancel that arrived before this call did is answered without translating anything.
  guard let job = translateRegistry.begin(token) else {
    bridgeFileLog(bridgeLogText("translate.cancel", token, "before_start"))
    return writeCString(
      translateCancelledJSON("cancelled before the translation started"), into: out,
      cap: out_cap)
  }
  defer { translateRegistry.end(token) }

  let task = Task {
    // Whatever way this Task ends, the caller must get an answer: with no timer, a path that forgot
    // to report would leave it waiting forever. finish keeps only the first result, so after a real
    // one this changes nothing.
    defer {
      job.finish(
        bridgeErrorJSON(
          code: BRIDGE_ERR_APPLE_TRANSLATE, detail: "translation task ended without a result"))
      job.noteTaskEnded()
    }

    // Issue #173 item 8: real timing instrumentation, not a repeat of the earlier "it's
    // structural" guess. Four stages are timed and logged as one "translate.timing" line per
    // call: the installed-languages enumeration below (suspected — and confirmed by this very
    // log — to be the dominant per-call cost, since it awaits LanguageAvailability().status
    // once per supported language every single time), source-language detection, getting a
    // ready session (now cache-backed by TranslationSessionCache, so this is ~0 after the
    // first call for a given pair instead of a fresh prepareTranslation() every time), and the
    // translate() call itself.
    let callStart = Date()

    let availability = LanguageAvailability()
    let installedAll = await availability.supportedLanguages
    var installed: [String] = []
    for lang in installedAll {
      if await availability.status(from: lang, to: nil) == .installed {
        installed.append(lang.maximalIdentifier)
      }
    }
    let installedLangsMs = Int(Date().timeIntervalSince(callStart) * 1000)

    // A cancel that came in while the loop above ran stops here, before source detection and before
    // prepareTranslation: the caller already has its answer.
    do { try Task.checkCancellation() } catch {
      job.finish(translateCancelledJSON("cancelled while the installed languages were listed"))
      return
    }

    let detectStart = Date()
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
      job.finish(
        bridgeErrorJSON(code: BRIDGE_ERR_NO_SOURCE_LANG, detail: detail, from: detectedFrom))
      return
    }
    let detectMs = Int(Date().timeIntervalSince(detectStart) * 1000)
    let sourceLang = Locale.Language(identifier: effectiveSource)
    do {
      // Issue #173 item 8: was `TranslationSession(installedSource:target:)` +
      // `prepareTranslation()` inline here, on every call. Now backed by
      // TranslationSessionCache, so only the first call for a given (source, target) pair
      // pays the session-creation + prepareTranslation cost; every later call for the same
      // pair (the overwhelmingly common case — most users translate one language pair) reuses
      // the already-prepared session.
      let (resp, getSessionMs, translateCallMs) = try await TranslationSessionCache.shared
        .translate(source: sourceLang, target: targetLang, text: inputText)
      let from = resp.sourceLanguage.languageCode?.identifier ?? sourceCode
      job.finish(bridgeEncode(TranslateSuccess(result: resp.targetText, from: from)))
      bridgeFileLog(bridgeLogText("translate.done", from, targetCode, resp.targetText.utf8.count))
      let totalMs = Int(Date().timeIntervalSince(callStart) * 1000)
      bridgeFileLog(
        bridgeLogText(
          "translate.timing", installedLangsMs, detectMs, getSessionMs, translateCallMs,
          totalMs))
    } catch {
      // A cancel is not a failure (issue #111): kai_translate_cancel has already answered the
      // caller, so this is the abandoned work unwinding. It is not logged as an error and reports
      // no second result; finish keeps the first one, and only reports cancelled itself if the
      // framework cancelled on its own.
      if error is CancellationError || Task.isCancelled {
        job.finish(translateCancelledJSON("the translation was cancelled"))
        return
      }
      // Apple system-level translation errors: uniformly shaped as
      // {"code":"apple_translate","detail":...},
      // rendered by the Go side's err.apple_translate_engine; detail carries the system's
      // localizedDescription. The framework refuses to translate a language into itself with this
      // same error, so the detected language rides along (from) for the Go side to tell that
      // apart from a real failure.
      let detail = error.localizedDescription
      job.finish(
        bridgeErrorJSON(code: BRIDGE_ERR_APPLE_TRANSLATE, detail: detail, from: detectedFrom))
      bridgeFileLog(bridgeLogText("translate.fail", detail), level: BRIDGE_LOG_ERROR)
    }
  }
  job.attach(task)

  // No timer (issue #111): the wait ends with the Task's result or with kai_translate_cancel, which
  // finishes the job at once and leaves the Task to unwind on its own.
  job.sema.wait()
  return writeCString(job.take(), into: out, cap: out_cap)
}

// kai_translate_cancel: asks the kai_translate call running under token to stop (issue #111). That
// call returns at once with {"code":"cancelled",...}; its Task is cancelled and unwinds in the
// background. Returns 1 when a running call was found, 0 when none was: the token is then
// remembered, so a kai_translate that has not begun yet answers cancelled as soon as it does. A
// token of 0 or less can never be cancelled.
@_cdecl("kai_translate_cancel")
public func kai_translate_cancel(_ token: Int64) -> Int32 {
  let found = translateRegistry.cancel(token)
  bridgeFileLog(bridgeLogText("translate.cancel", token, found ? "running" : "not_running"))
  return found ? 1 : 0
}

// kai_warm_translate: warms (creates + prepareTranslation()s) a TranslationSessionCache entry
// for (src, dst) so the first real kai_translate call for that pair skips the
// prepareTranslation cost (issue #173 item 8). Go calls this once at launch, in its own
// goroutine, for the user's default language pair (main.go), and it is harmless — a cheap
// no-op — if that pair later turns out to already be cached (e.g. a translate happened before
// warming finished) or if src/dst are empty (auto-detect has no fixed pair to warm; the caller
// skips the call in that case, but this still no-ops safely if it didn't).
// Blocks the calling thread until warming finishes or fails; the Go side calls this off the
// main goroutine so it never blocks app startup. Always returns 1 (best-effort: a warm
// failure — e.g. the pair's language pack is not installed — is not fatal, since kai_translate
// still works, just without the pre-warmed session; failures are logged, not surfaced to Go).
@_cdecl("kai_warm_translate")
public func kai_warm_translate(
  _ src: UnsafePointer<CChar>?,
  _ dst: UnsafePointer<CChar>?
) -> Int32 {
  let sourceCode = src.flatMap { String(cString: $0) } ?? ""
  let targetCode = dst.flatMap { String(cString: $0) } ?? ""
  guard !sourceCode.isEmpty, !targetCode.isEmpty else {
    // No fixed pair to warm (source is auto-detect, or target unset); not an error.
    return 1
  }
  bridgeFileLog(bridgeLogText("translate.warm_start", sourceCode, targetCode))
  let start = Date()
  let sema = DispatchSemaphore(value: 0)
  Task {
    await TranslationSessionCache.shared.warm(
      source: Locale.Language(identifier: sourceCode),
      target: Locale.Language(identifier: targetCode))
    sema.signal()
  }
  sema.wait()
  let elapsedMs = Int(Date().timeIntervalSince(start) * 1000)
  bridgeFileLog(bridgeLogText("translate.warm_done", sourceCode, targetCode, elapsedMs))
  return 1
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
