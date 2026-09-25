// apple_ocr.swift
// Vision OCR @_cdecl entries: kai_ocr.
// Depends on bridge_errors.swift (error codes / Codable / encoding helpers),
// bridge_common.swift (writeCString),
// bridge_log.swift（bridgeFileLog / bridgeLogText）。
import AppKit
import ApplicationServices
import CoreGraphics
import Foundation
import ImageIO
import NaturalLanguage
import Translation
import UniformTypeIdentifiers
import Vision

// kai_ocr: runs Vision text recognition on the provided image (base64 PNG/JPEG; Chinese by
// default, more languages supported).
// Parameter order/types must match the Go side's cgo declaration:
//   int kai_ocr(const char* img, char* out, int out_cap, int correct, int timeout_sec);
// out receives {"text":"...","regions":[{"text","conf","box"}]} (OCRSuccess); on failure it
// gets {"code":"...","detail":"..."} (BridgeError).
// correct is 0/1 (the Go side's usesLanguageCorrection switch); timeout_sec is the
// recognition timeout in seconds (replacing the old hardcoded 15s);
// retry is the Vision OCR failure-fallback retry count (<=0 uses the default 2), effective
// only against CRImageReaderError-style transient rejections.
@_cdecl("kai_ocr")
public func kai_ocr(
  _ base64: UnsafePointer<CChar>?,
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32,
  _ correct: Int32,
  _ timeout_sec: Int32,
  _ retry: Int32
) -> Int32 {
  let b64 = base64.flatMap { String(cString: $0) } ?? ""
  let correctionOn = correct != 0
  let timeout = max(Int(timeout_sec), 1)
  let retryCount = max(Int(retry), 0)
  // Diagnostics: confirm the cgo calling convention works (out_cap should arrive around
  // 1<<20; out must not be nil).
  bridgeFileLog(
    bridgeLogText(
      "ocr.entry", out == nil ? "nil" : "ok", out_cap, correctionOn ? "on" : "off", timeout),
    level: BRIDGE_LOG_DEBUG)
  bridgeFileLog(
    bridgeLogText("ocr.config", correctionOn ? "on" : "off", timeout), level: BRIDGE_LOG_DEBUG)

  guard let data = Data(base64Encoded: b64) else {
    bridgeFileLog(bridgeLogText("ocr.base64_fail"), level: BRIDGE_LOG_WARN)
    return writeCString(
      bridgeErrorJSON(
        code: BRIDGE_ERR_DECODE_FAILED,
        detail: "base64 decode failed (len=\(b64.count))"), into: out, cap: out_cap)
  }
  guard let src = CGImageSourceCreateWithData(data as CFData, nil),
    let cg = CGImageSourceCreateImageAtIndex(src, 0, nil)
  else {
    bridgeFileLog(bridgeLogText("ocr.image_fail"), level: BRIDGE_LOG_WARN)
    return writeCString(
      bridgeErrorJSON(
        code: BRIDGE_ERR_DECODE_FAILED,
        detail: "CGImageSource decode failed (input is not a valid image)"), into: out, cap: out_cap
    )
  }

  let w = cg.width
  let h = cg.height
  guard w > 0, h > 0 else {
    bridgeFileLog(bridgeLogText("ocr.empty_size"), level: BRIDGE_LOG_WARN)
    return writeCString(
      bridgeErrorJSON(code: BRIDGE_ERR_EMPTY_IMAGE, detail: "image size is 0x0"),
      into: out, cap: out_cap)
  }

  // Pixel normalization (root-cures CRImageReaderError error 1):
  // Feeding a region screenshot straight into VNImageRequestHandler sporadically fails with
  // CRImageReaderError error 1 (an instant rejection, perform in 0.000s).
  // The root cause: Vision outright rejects images with an alpha channel / unknown color
  // space / semi-transparent pixels.
  // Double fallback:
  //  1. Redraw via CGContext into "standard sRGB + 8-bit + noneSkipLast (A forced to 255,
  //     opaque)", removing the degenerate color space and semi-transparency semantics;
  //     note: on this machine's macOS 26.2, creating a bitmap context with
  //     CGImageAlphaInfo.none returns nil (observed
  //     "failed to create bitmap context"), so noneSkipLast is used to guarantee the context
  //     can be created; opaque A=255 works well with Vision.
  //  2. Then re-wrap through a JPEG container and read back (JPEG has no alpha, forcibly
  //     discarding semi-transparent pixels) — the step that truly removes
  //     semi-transparency.
  // Key: normalization is factored into normalizedVariant(_:), and the failure-retry loop
  // **re-normalizes the original cg every time** (rather than reusing one image),
  // because a single normalization may not fully cure error 1 on edge-case images; re-running
  // the JPEG re-wrap path sometimes succeeds.
  // noneSkipLast (RGBA, opaque A) is 4 bytes per pixel → bytesPerRow = w*4 (16-byte
  // aligned).
  func normalizedVariant(_ src: CGImage) -> CGImage {
    let rw = src.width
    let rh = src.height
    let bytesPerRow = (rw * 4 + 15) & ~15
    var work: CGImage = src  // on any step failure, fall back to the previous result rather than failing wholesale
    if let ctx = CGContext(
      data: nil, width: rw, height: rh, bitsPerComponent: 8,
      bytesPerRow: bytesPerRow, space: CGColorSpaceCreateDeviceRGB(),
      bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)
    {
      ctx.draw(src, in: CGRect(x: 0, y: 0, width: CGFloat(rw), height: CGFloat(rh)))
      if let img = ctx.makeImage() {
        work = img
      } else {
        bridgeFileLog(bridgeLogText("ocr.redraw_fail"), level: BRIDGE_LOG_WARN)
      }
    } else {
      bridgeFileLog(bridgeLogText("ocr.bitmap_fail"), level: BRIDGE_LOG_WARN)
    }
    if let jpegData = normalizeToJPEG(work) {
      let hint =
        [kCGImageSourceTypeIdentifierHint: UTType.jpeg.identifier as CFString] as CFDictionary
      if let srcImg = CGImageSourceCreateWithData(jpegData as CFData, hint)
        .flatMap({ CGImageSourceCreateImageAtIndex($0, 0, hint) })
      {
        let reBytesPerRow = (srcImg.width * 4 + 15) & ~15
        let reCtx = CGContext(
          data: nil, width: srcImg.width, height: srcImg.height, bitsPerComponent: 8,
          bytesPerRow: reBytesPerRow, space: CGColorSpaceCreateDeviceRGB(),
          bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue)
        reCtx?.draw(
          srcImg,
          in: CGRect(x: 0, y: 0, width: CGFloat(srcImg.width), height: CGFloat(srcImg.height)))
        if let reImg = reCtx?.makeImage() {
          work = reImg
          bridgeFileLog(
            bridgeLogText(
              "ocr.normalized", work.bitsPerPixel, work.alphaInfo.rawValue, work.width, work.height),
            level: BRIDGE_LOG_DEBUG)
        } else {
          bridgeFileLog(bridgeLogText("ocr.normalize_redraw_fail"), level: BRIDGE_LOG_WARN)
        }
      } else {
        bridgeFileLog(bridgeLogText("ocr.normalize_fail"), level: BRIDGE_LOG_WARN)
      }
    } else {
      bridgeFileLog(bridgeLogText("ocr.normalize_skip"), level: BRIDGE_LOG_WARN)
    }
    return work
  }

  let rawBpp = cg.bitsPerPixel
  let rawAlpha = cg.alphaInfo.rawValue
  let rawCS = cg.colorSpace?.name as String? ?? "unknown"
  bridgeFileLog(
    bridgeLogText(
      "ocr.redraw_info", rawBpp, rawAlpha, rawCS, cg.bitsPerPixel, cg.alphaInfo.rawValue, w, h),
    level: BRIDGE_LOG_DEBUG)

  // Initial normalization (for the first attempt); on retries, re-normalize the original cg
  // per attempt.
  let redrawn: CGImage = normalizedVariant(cg)

  let request = VNRecognizeTextRequest()
  // correctionOn (the Go side's correct 0/1): with language correction on, use the accurate
  // level; when off, still accurate
  // but with usesLanguageCorrection disabled for faster inference (fast recalls worse in
  // Chinese-dense scenes, so accurate is kept).
  request.recognitionLevel = .accurate
  request.usesLanguageCorrection = correctionOn
  // Converge the language candidates: on a cs=unknown degenerate image, many language
  // candidates raise the odds of a Vision internal decoder crash (error 1).
  // The primary candidate set keeps only the most common zh/en/zh-Hant, avoiding ja/ko-style
  // rejections on degenerate images.
  let primaryLangs = ["zh-Hans", "zh-Hant", "en"]
  request.recognitionLanguages = primaryLangs

  var resultJSON = "{\"text\":\"\",\"regions\":[]}"
  let sema = DispatchSemaphore(value: 0)
  let start = Date()

  // performOCR runs one Vision recognition pass; a nil return means success (resultJSON is
  // populated),
  // non-nil means failure detail (including error 1 etc.) for the caller to decide on retry.
  // img is the (already normalized) pixels to recognize this round.
  func performOCR(_ langs: [String], _ img: CGImage) -> String? {
    let req = VNRecognizeTextRequest()
    req.recognitionLevel = .accurate
    req.usesLanguageCorrection = correctionOn
    req.recognitionLanguages = langs
    let h = VNImageRequestHandler(cgImage: img, options: [:])
    do {
      try h.perform([req])
      guard let observations = req.results, !observations.isEmpty else {
        return "Vision perform returned nil/empty results"
      }
      var lines: [String] = []
      var regions: [OCRRegion] = []
      for obs in observations {
        guard let candidate = obs.topCandidates(1).first else { continue }
        let txt = candidate.string
        let conf = candidate.confidence
        let bb = obs.boundingBox  // normalized coordinates, origin bottom-left
        let x1 = Int(bb.origin.x * CGFloat(w))
        let y1 = Int((1 - bb.origin.y - bb.height) * CGFloat(img.height))  // flip y to a top-left origin
        let x2 = Int((bb.origin.x + bb.width) * CGFloat(w))
        let y2 = Int((1 - bb.origin.y) * CGFloat(img.height))
        lines.append(txt)
        regions.append(OCRRegion(text: txt, conf: Double(conf), box: [x1, y1, x2, y2]))
      }
      let full = lines.joined(separator: "\n")
      resultJSON = bridgeEncode(OCRSuccess(text: full, regions: regions))
      bridgeFileLog(
        bridgeLogText("ocr.done", regions.count, full.utf8.count), level: BRIDGE_LOG_DEBUG)
      return nil
    } catch {
      return error.localizedDescription
    }
  }

  DispatchQueue.global(qos: .userInitiated).async {
    // Fallback retries: try different language candidate sets in turn + re-normalize the
    // original pixels each time, working around Vision's crash on this image with multi-language
    // candidates / degenerate pixel formats (CRImageReaderError error 1).
    // Candidate-set priority: primary set (zh/en/zh-Hant) → Chinese only → English only
    // (cycled for reuse once exhausted).
    // Every round re-normalizes from the original cg (normalizedVariant); a single
    // normalization may not cure error 1 on edge-case images,
    // and re-running the JPEG re-wrap path sometimes succeeds — the key fallback for
    // reliably-reproducing error 1 scenarios.
    // retry is the user-configured "extra language-candidate retry count" (excluding the
    // first attempt); but CRImageReaderError error 1
    // is a pixel-format transient rejection and re-normalization is the key fallback that the
    // user should not be able to switch off — hence maxAttempts has a floor of 2
    // (first attempt + at least one re-normalization retry), so reliably-reproducing error 1
    // scenarios still get a chance to self-heal.
    let langCandidates: [[String]] = [primaryLangs, ["zh-Hans"], ["en"]]
    let maxAttempts = max(1 + max(retryCount, 0), 2)  // at least 2: first try + at least one re-normalization
    var detail: String?
    var attempt = 0
    while attempt < maxAttempts {
      attempt += 1
      let langs = langCandidates[(attempt - 1) % langCandidates.count]
      // The 1st attempt uses the initial normalization (redrawn); each later attempt
      // re-normalizes from the original cg, trying a different pixel path.
      let img = attempt == 1 ? redrawn : normalizedVariant(cg)
      detail = performOCR(langs, img)
      if detail == nil { break }  // success; leave the retry loop
      // Non-CRImageReaderError failures (e.g. timeout) are not retried — report directly
      if !detail!.contains("CRImageReaderError") { break }
      if attempt >= maxAttempts { break }  // retry cap reached
      bridgeFileLog(
        bridgeLogText("ocr.retry", attempt, langs.joined(separator: ","), detail!),
        level: BRIDGE_LOG_WARN)
    }
    if let d = detail {
      resultJSON = bridgeErrorJSON(code: BRIDGE_ERR_APPLE_OCR, detail: d)
      bridgeFileLog(bridgeLogText("ocr.fail", d), level: BRIDGE_LOG_ERROR)
    }
    sema.signal()
  }

  let cost = Date().timeIntervalSince(start)
  if sema.wait(timeout: .now() + Double(timeout)) == .timedOut {
    let detail = "Vision perform exceeded \(timeout)s, aborted"
    resultJSON = bridgeErrorJSON(code: BRIDGE_ERR_OCR_TIMEOUT, detail: detail)
    bridgeFileLog(bridgeLogText("ocr.timeout", timeout), level: BRIDGE_LOG_ERROR)
  } else {
    bridgeFileLog(
      bridgeLogText("ocr.perform_cost", String(format: "%.3f", cost)), level: BRIDGE_LOG_DEBUG)
  }
  return writeCString(resultJSON, into: out, cap: out_cap)
}

// normalizeToJPEG encodes a CGImage into JPEG (sRGB, 8-bit, no alpha), returning the
// normalized image data.
// Used to root-cure VNImageRequestHandler's CRImageReaderError error 1: after the JPEG
// container re-wrap,
// pixels are forcibly converted into Vision's most compatible format, avoiding rejections
// caused by unknown color spaces / semi-transparent edges in the source image.
private func normalizeToJPEG(_ image: CGImage) -> Data? {
  let mutData = NSMutableData()
  guard
    let dest = CGImageDestinationCreateWithData(
      mutData as CFMutableData, UTType.jpeg.identifier as CFString, 1, nil)
  else {
    return nil
  }
  let opts =
    [
      kCGImageDestinationLossyCompressionQuality: 0.92
    ] as CFDictionary
  CGImageDestinationAddImage(dest, image, opts)
  guard CGImageDestinationFinalize(dest) else {
    return nil
  }
  return mutData as Data
}
