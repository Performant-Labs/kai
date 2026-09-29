// apple_correct.swift
// Text correction on Apple's on-device model (issue #208): kai_correct / kai_correct_availability.
//
// The Go translate service owns the wording of the instructions (translate.CorrectionInstructions,
// the one place); this file only runs them. kai_correct is called from a service goroutine, never
// the main thread, and blocks that goroutine on a semaphore with a bound (a model that never
// answers must not hold it forever); the model call itself runs in a Task, off every actor. The
// text the user gave is never written to the bridge log: only lengths and status words are.
import Foundation
import FoundationModels

/// What the model is asked to produce (guided generation): one field, the corrected text. The
/// framework constrains the model to this shape, so the answer is the text itself, with no
/// preamble, no quotes and no explanation to scrape off.
@Generable
struct CorrectedText {
  @Guide(
    description:
      "The corrected text and nothing else. Same language, same variant, same line breaks. No preamble, no quotation marks, no explanation."
  )
  var text: String
}

/// kai_correct success: {"text":"..."}
struct CorrectionSuccess: Codable {
  let text: String
}

/// kai_correct_availability: {"status":"available"} or the reason the model cannot run.
struct CorrectionAvailability: Codable {
  let status: String
}

/// How long kai_correct waits for the model before giving up. The on-device model answers a
/// paragraph in a few seconds; this is the bound for a stuck one.
let CORRECT_WAIT_SECONDS: Double = 45

/// The single answer of one kai_correct call, written once by whichever of the model's Task and
/// the timeout gets there first.
final class CorrectionBox: @unchecked Sendable {
  private let lock = NSLock()
  private var json: String? = nil

  /// Stores the answer unless one is already there; returns whether this call stored it.
  @discardableResult
  func setIfEmpty(_ value: String) -> Bool {
    lock.lock()
    defer { lock.unlock() }
    if json != nil { return false }
    json = value
    return true
  }

  func take() -> String {
    lock.lock()
    defer { lock.unlock() }
    return json ?? bridgeErrorJSON(code: BRIDGE_ERR_CORRECT_FAILED, detail: "no answer")
  }
}

/// The availability word for `locale` ("" = the model alone): available, or why not. The words are
/// the Go side's engine.CorrectionStatus values.
func correctionStatus(locale: String) -> String {
  let model = SystemLanguageModel.default
  switch model.availability {
  case .available:
    break
  case .unavailable(let reason):
    switch reason {
    case .deviceNotEligible: return "unsupported_hardware"
    case .appleIntelligenceNotEnabled: return "apple_intelligence_off"
    case .modelNotReady: return "model_not_ready"
    @unknown default: return "unavailable"
    }
  }
  if !locale.isEmpty && !model.supportsLocale(Locale(identifier: locale)) {
    return "unsupported_language"
  }
  return "available"
}

// kai_correct_availability: writes {"status":"..."} (CorrectionAvailability). A synchronous query
// of the framework's own state: no model call, no Task, no wait.
@_cdecl("kai_correct_availability")
public func kai_correct_availability(
  _ locale: UnsafePointer<CChar>?,
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let loc = locale.flatMap { String(cString: $0) } ?? ""
  let status = correctionStatus(locale: loc)
  return writeCString(bridgeEncode(CorrectionAvailability(status: status)), into: out, cap: out_cap)
}

// kai_correct: runs `instructions` (the system prompt) over `text` on the on-device model with
// guided generation and greedy sampling (the same text corrects the same way twice), and writes
// {"text":"..."} (CorrectionSuccess), or {"code":"correct_refused|correct_failed|correct_timeout",
// "detail":"..."} (BridgeError). Never touches the main thread.
@_cdecl("kai_correct")
public func kai_correct(
  _ instructions: UnsafePointer<CChar>?,
  _ text: UnsafePointer<CChar>?,
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let system = instructions.flatMap { String(cString: $0) } ?? ""
  let input = text.flatMap { String(cString: $0) } ?? ""
  if input.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
    return writeCString(
      bridgeErrorJSON(code: BRIDGE_ERR_EMPTY_TEXT, detail: ""), into: out, cap: out_cap)
  }
  let start = Date()
  let box = CorrectionBox()
  let sema = DispatchSemaphore(value: 0)
  let task = Task {
    do {
      let session = LanguageModelSession(instructions: system)
      let response = try await session.respond(
        to: input,
        generating: CorrectedText.self,
        options: GenerationOptions(samplingMode: .greedy))
      box.setIfEmpty(bridgeEncode(CorrectionSuccess(text: response.content.text)))
    } catch {
      // Only the kind of failure is reported, never the error's own description: a framework
      // error can quote the prompt, and the text is the user's.
      let described = String(describing: error).lowercased()
      let refused = described.contains("guardrail") || described.contains("refus")
      box.setIfEmpty(
        bridgeErrorJSON(
          code: refused ? BRIDGE_ERR_CORRECT_REFUSED : BRIDGE_ERR_CORRECT_FAILED,
          detail: String(describing: type(of: error))))
    }
    sema.signal()
  }
  if sema.wait(timeout: .now() + CORRECT_WAIT_SECONDS) == .timedOut {
    task.cancel()
    box.setIfEmpty(bridgeErrorJSON(code: BRIDGE_ERR_CORRECT_TIMEOUT, detail: ""))
  }
  let answer = box.take()
  let elapsedMs = Int(Date().timeIntervalSince(start) * 1000)
  bridgeFileLog("correct: \(input.count) chars in, \(answer.count) bytes out, \(elapsedMs) ms", level: BRIDGE_LOG_DEBUG)
  return writeCString(answer, into: out, cap: out_cap)
}
