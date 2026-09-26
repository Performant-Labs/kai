// bridge_log.swift
// The Swift bridge layer's logging subsystem: level constants, the log-config cdecl,
// Chinese/English log copy, day rotation and compression.
// All bridge logs are appended via bridgeFileLog to dataDir/logs/kai-bridge.log with no
// prefix;
// when no log directory is set (kai_set_log_config not called), nothing is written. Level,
// retention days and the compression
// switch all come from the Go side's LogConfig via kai_set_log_config, matching the main app
// log (kai.log) policy.
import AppKit
import ApplicationServices
import CoreGraphics
import Foundation
import NaturalLanguage
import Translation
import Vision

// Log-level numeric mapping (higher = more severe; persisted only when
// level >= bridgeLogLevel).
let BRIDGE_LOG_DEBUG: Int = 0
let BRIDGE_LOG_INFO: Int = 1
let BRIDGE_LOG_WARN: Int = 2
let BRIDGE_LOG_ERROR: Int = 3

// The log directory is set by Go at startup via kai_set_log_config (pointing at
// dataDir/logs);
// when unset it is nil and bridgeFileLog returns immediately. Level/retention/compression
// also sync from LogConfig.
var bridgeLogDir: URL? = nil
var bridgeLogLevel: Int = BRIDGE_LOG_INFO
var bridgeRetentionDays: Int = 30
var bridgeCompress: Bool = true

// Current log language: set by Go via kai_set_locale; "zh" writes Chinese logs, "en" writes
// English logs; default zh.
var bridgeLogLocale: String = "zh"

// kai_set_log_config is called by Go at startup and on config hot-updates, uniformly setting
// the bridge layer's log directory and policy.
// dir: log directory (dataDir/logs); level: debug/info/warn/error (invalid falls back to
// info);
// retention_days: days to keep (<=0 = day rotation only, no cleanup); compress: whether
// expired archives are compressed to .gz.
@_cdecl("kai_set_log_config")
public func kai_set_log_config(
  _ dir: UnsafePointer<CChar>?,
  _ level: UnsafePointer<CChar>?,
  _ retention_days: Int32,
  _ compress: Bool
) {
  if let dir = dir {
    let path = String(cString: dir)
    let url = URL(fileURLWithPath: path, isDirectory: true)
    try? FileManager.default.createDirectory(at: url, withIntermediateDirectories: true)
    bridgeLogDir = url
  }
  if let level = level {
    switch String(cString: level).lowercased() {
    case "debug": bridgeLogLevel = BRIDGE_LOG_DEBUG
    case "warn", "warning": bridgeLogLevel = BRIDGE_LOG_WARN
    case "error": bridgeLogLevel = BRIDGE_LOG_ERROR
    default: bridgeLogLevel = BRIDGE_LOG_INFO
    }
  }
  bridgeRetentionDays = Int(retention_days)
  bridgeCompress = compress
  bridgeFileLog(
    bridgeLogText(
      "log.config_applied", bridgeLogDir?.path ?? "", bridgeLogLevel, bridgeRetentionDays,
      String(bridgeCompress)), level: BRIDGE_LOG_INFO)
}

// kai_set_locale is called by Go at startup and on language switches, setting the bridge
// layer's log output language.
// locale: a code like "zh-CN" / "en-US"; anything starting with "en" is English, everything
// else is treated as Chinese.
@_cdecl("kai_set_locale")
public func kai_set_locale(_ locale: UnsafePointer<CChar>?) {
  guard let locale = locale else { return }
  let l = String(cString: locale).lowercased()
  bridgeLogLocale = l.hasPrefix("en") ? "en" : "zh"
}

// bridgeLogText returns the Chinese/English log copy per the current bridgeLogLocale.
// key is a stable English identifier (decoupled from the Go i18n tables); Swift maintains
// only this one lightweight table.
// The %@ / %d placeholders in the copy correspond to String(format:).
func bridgeLogText(_ key: String, _ args: CVarArg...) -> String {
  let zh: [String: String] = [
    "log.config_applied": "日志配置已应用 dir=%@ level=%d retention_days=%d compress=%@",
    "translate.call": "系统翻译调用 src=%@ dst=%@ 文本长度=%d",
    "translate.empty": "系统翻译失败: 文本为空",
    "translate.no_target": "系统翻译失败: 缺少目标语言",
    "translate.detect_src": "系统翻译自动检测源语言=%@ dst=%@",
    "translate.detect_fail": "系统翻译自动检测失败且无已安装语言回退 dst=%@",
    "translate.fail": "系统翻译失败: %@",
    "translate.done": "系统翻译完成 from=%@ dst=%@ 译文长度=%d",
    "translate.cancel": "系统翻译取消 token=%lld 状态=%@",
    "translate.drain": "系统翻译取消后，被弃用的任务已结束 取消后耗时=%@s",
    "lang.query_done": "系统已安装语言列表查询完成 总数=%d 已安装=%d",
    "a11y.query": "辅助功能授权查询 结果=%@",
    "a11y.request": "辅助功能授权请求 弹出系统授权框并尝试打开设置面板",
    "a11y.request_done": "辅助功能授权请求 已打开系统设置面板",
    "screen.query": "屏幕录制授权查询 结果=%@",
    "screen.request": "屏幕录制授权请求 打开系统设置面板",
    "screen.request_done": "屏幕录制授权请求 已打开系统设置面板",
    "selection.point_done": "选区坐标读取完成 x=%@ y=%@",
    "ocr.base64_fail": "系统 OCR 失败: base64 解码失败",
    "ocr.image_fail": "系统 OCR 失败: 图片解码失败",
    "ocr.empty_size": "系统 OCR 失败: 图片尺寸为空",
    "ocr.bitmap_fail": "系统 OCR 失败: 位图上下文创建失败",
    "ocr.redraw_fail": "系统 OCR 失败: 位图重绘失败",
    "ocr.redraw_info": "系统 OCR 重绘前 raw bpp=%d alpha=%d cs=%@ | 重绘后 bpp=%d alpha=%d w=%d h=%d",
    "ocr.fail": "系统 OCR 失败: %@",
    "ocr.done": "系统 OCR 完成 行数=%d 首行长度=%d",
    "ocr.entry": "系统 OCR 入口 out=%@ cap=%d correct=%@ timeout=%d",
    "ocr.config": "系统 OCR 配置 correction=%@ level=%d",
    "ocr.perform_cost": "系统 OCR perform 实际耗时=%@s",
    "ocr.timeout": "系统 OCR 超时: perform 超过 %d s 未完成",
    "selection.fail_app": "选区读取 无法获取前台应用 axErr=%d",
    "selection.empty": "选区读取结果为空 axErr=%d",
    "selection.done": "选区读取完成 长度=%d",
    "input.tap_fail": "输入监控检测 创建 EventTap 失败（未授权或被策略拒绝）",
    "input.tap_enabled": "输入监控检测 tapIsEnabled=%@",
    "input.settings": "输入监控 打开系统设置面板",
  ]
  let en: [String: String] = [
    "log.config_applied": "log config applied dir=%@ level=%d retention_days=%d compress=%@",
    "translate.call": "system translate call src=%@ dst=%@ text_len=%d",
    "translate.empty": "translate failed: empty text",
    "translate.no_target": "translate failed: missing target language",
    "translate.detect_src": "system translate auto-detected source=%@ dst=%@",
    "translate.detect_fail":
      "system translate auto-detect failed and no installed language fallback dst=%@",
    "translate.fail": "translate failed: %@",
    "translate.done": "system translate done from=%@ dst=%@ result_len=%d",
    "translate.cancel": "system translate cancel token=%lld state=%@",
    "translate.drain": "system translate: cancelled task finished unwinding %@s after the cancel",
    "lang.query_done": "installed language list query done total=%d installed=%d",
    "a11y.query": "accessibility authorization query result=%@",
    "a11y.request":
      "accessibility authorization request: popup system dialog and try opening settings panel",
    "a11y.request_done": "accessibility authorization request: settings panel opened",
    "screen.query": "screen recording authorization query result=%@",
    "screen.request": "screen recording authorization request: open settings panel",
    "screen.request_done": "screen recording authorization request: settings panel opened",
    "selection.point_done": "selection point read done x=%@ y=%@",
    "ocr.base64_fail": "system OCR failed: base64 decode failed",
    "ocr.image_fail": "system OCR failed: image decode failed",
    "ocr.empty_size": "system OCR failed: image size empty",
    "ocr.bitmap_fail": "system OCR failed: bitmap context creation failed",
    "ocr.redraw_fail": "system OCR failed: bitmap redraw failed",
    "ocr.redraw_info":
      "system OCR before redraw raw bpp=%d alpha=%d cs=%@ | after redraw bpp=%d alpha=%d w=%d h=%d",
    "ocr.fail": "system OCR failed: %@",
    "ocr.done": "system OCR done lines=%d first_len=%d",
    "ocr.entry": "system OCR entry out=%@ cap=%d correct=%@ timeout=%d",
    "ocr.config": "system OCR config correction=%@ level=%d",
    "ocr.perform_cost": "system OCR perform actual cost=%@s",
    "ocr.timeout": "system OCR timeout: perform exceeded %d s",
    "selection.fail_app": "selection read failed: cannot get front app axErr=%d",
    "selection.empty": "selection read result empty axErr=%d",
    "selection.done": "selection read done length=%d",
    "input.tap_fail":
      "input monitoring check: create EventTap failed (not authorized or blocked by policy)",
    "input.tap_enabled": "input monitoring tapIsEnabled=%@",
    "input.settings": "input monitoring: open system settings panel",
  ]
  guard let tmpl = (bridgeLogLocale == "en" ? en : zh)[key] else {
    // When a translation is missing, pass the key through to make gaps visible
    return key
  }
  return String(format: tmpl, arguments: args)
}

// bridgeDayString returns the date as 2006-01-02, for day-rotated archive file names.
private func bridgeDayString(_ d: Date) -> String {
  let f = DateFormatter()
  f.timeZone = TimeZone.current
  f.dateFormat = "yyyy-MM-dd"
  return f.string(from: d)
}

// bridgeGzip compresses a file with the system gzip, deleting the original on success; on
// failure it returns silently (keeping the original).
private func bridgeGzip(_ path: String) {
  let proc = Process()
  proc.executableURL = URL(fileURLWithPath: "/usr/bin/gzip")
  proc.arguments = [path]
  do {
    try proc.run()
    proc.waitUntilExit()
  } catch {
    return
  }
}

// Appends one line to kai-bridge.log (ISO8601 timestamp, local timezone, no prefix).
// level drives level filtering: default info; logs below the current bridgeLogLevel are
// dropped.
// Day-rotates before writing: when kai-bridge.log's mtime is not today, it is archived as
// kai-bridge-YYYY-MM-DD.log
// (compressed to .gz when bridgeCompress is true); archive names keep the kai- prefix and are
// covered by the Go side's unified cleanup policy.
func bridgeFileLog(_ message: String, level: Int = BRIDGE_LOG_INFO) {
  guard level >= bridgeLogLevel else { return }
  guard let dir = bridgeLogDir else { return }
  let url = dir.appendingPathComponent("kai-bridge.log")

  // Day rotation: archive the current file when its mtime is not today (compress first, then
  // keep — the Go cleanup deletes expired archives).
  if let attrs = try? FileManager.default.attributesOfItem(atPath: url.path),
    let mtime = attrs[.modificationDate] as? Date,
    !Calendar.current.isDateInToday(mtime)
  {
    let archiveName = "kai-bridge-\(bridgeDayString(mtime)).log"
    let archivePath = dir.appendingPathComponent(archiveName).path
    let uniqueArchive = uniqueBridgePath(archivePath)
    if (try? FileManager.default.moveItem(atPath: url.path, toPath: uniqueArchive)) != nil {
      if bridgeCompress {
        bridgeGzip(uniqueArchive)
      }
    }
  }

  let fmt = ISO8601DateFormatter()
  fmt.timeZone = TimeZone.current
  let ts = fmt.string(from: Date())
  let line = "\(ts) \(message)\n"
  if let data = line.data(using: .utf8) {
    let fd = open(url.path, O_WRONLY | O_CREAT | O_APPEND, 0o644)
    if fd >= 0 {
      _ = data.withUnsafeBytes { write(fd, $0.baseAddress, $0.count) }
      close(fd)
    }
  }
}

// uniqueBridgePath inserts .N into the middle of path when it exists, avoiding overwrite.
private func uniqueBridgePath(_ path: String) -> String {
  if !FileManager.default.fileExists(atPath: path) { return path }
  let url = URL(fileURLWithPath: path)
  let ext = url.pathExtension
  let base = url.deletingPathExtension().path
  var i = 1
  while true {
    let cand = "\(base).\(i).\(ext)"
    if !FileManager.default.fileExists(atPath: cand) { return cand }
    i += 1
  }
}
