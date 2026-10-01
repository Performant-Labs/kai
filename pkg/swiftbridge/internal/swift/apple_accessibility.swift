// apple_accessibility.swift
// Accessibility / screen-recording permission queries and guidance, selection anchor
// reading, primary screen size reading.
// Depends on bridge_common.swift (writeCString) and bridge_log.swift (bridgeFileLog /
// bridgeLogText).
import AppKit
import ApplicationServices
import CoreGraphics
import Foundation
import NaturalLanguage
import ScreenCaptureKit
import Translation
import Vision

// kai_accessibility_enabled: queries whether accessibility is granted.
// out receives "true"/"false".
@_cdecl("kai_accessibility_enabled")
public func kai_accessibility_enabled(
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let enabled = AXIsProcessTrusted()
  bridgeFileLog(bridgeLogText("a11y.query", String(enabled)))
  return writeCString(enabled ? "true" : "false", into: out, cap: out_cap)
}

// kai_accessibility_request: pops the system permission dialog and tries to open System
// Settings > Privacy & Security > Accessibility.
// A 0 return means the request was initiated ("requested" only — not the same as granted).
@_cdecl("kai_accessibility_request")
public func kai_accessibility_request() -> Int32 {
  bridgeFileLog(bridgeLogText("a11y.request"))
  // Triggers the system permission dialog (async request; pops on first use).
  let opts = [kAXTrustedCheckOptionPrompt.takeUnretainedValue() as String: true] as CFDictionary
  _ = AXIsProcessTrustedWithOptions(opts)
  // Additionally opens the settings pane so the user can tick the box right away.
  if #available(macOS 13.0, *) {
    if let url = URL(
      string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility")
    {
      NSWorkspace.shared.open(url)
    }
  }
  bridgeFileLog(bridgeLogText("a11y.request_done"))
  return 0
}

// kai_screenrecording_enabled: queries whether screen recording is granted.
// out receives "true"/"false".
@_cdecl("kai_screenrecording_enabled")
public func kai_screenrecording_enabled(
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let enabled = CGPreflightScreenCaptureAccess()
  bridgeFileLog(bridgeLogText("screen.query", String(enabled)))
  return writeCString(enabled ? "true" : "false", into: out, cap: out_cap)
}

// kai_screenrecording_request: makes macOS list Kai under Privacy & Security > Screen Recording,
// then opens that pane.
// CGRequestScreenCaptureAccess() alone is not reliable: it does nothing once the app has been asked
// before, or when the app is not frontmost / not on the main thread. So this runs on the main thread
// with Kai active, and if the preflight still says "not granted" after the request it makes one
// real ScreenCaptureKit call (SCShareableContent: window and display metadata only, no pixels are
// captured, so no flash and no second prompt beyond the system's own). macOS lists an app once it
// has asked or tried to capture. The pane opens last, so Kai is already in the list.
// A 0 return means the request was started ("requested", not "granted"); the pane opens
// asynchronously on the main thread.
@_cdecl("kai_screenrecording_request")
public func kai_screenrecording_request() -> Int32 {
  bridgeFileLog(bridgeLogText("screen.request"))
  let work = {
    NSApp.activate()
    _ = CGRequestScreenCaptureAccess()
    var opened = false
    let openPane = {
      if opened { return }
      opened = true
      if let url = URL(
        string: "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture")
      {
        NSWorkspace.shared.open(url)
      }
      bridgeFileLog(bridgeLogText("screen.request_done"))
    }
    if !CGPreflightScreenCaptureAccess() {
      // One minimal capture attempt so macOS registers Kai; the pane opens when it answers (or
      // after 3 s at the latest, so a slow answer never leaves the user with no pane).
      SCShareableContent.getExcludingDesktopWindows(false, onScreenWindowsOnly: true) { _, _ in
        DispatchQueue.main.async { openPane() }
      }
      DispatchQueue.main.asyncAfter(deadline: .now() + 3) { openPane() }
    } else {
      openPane()
    }
  }
  if Thread.isMainThread {
    work()
  } else {
    DispatchQueue.main.async(execute: work)
  }
  return 0
}

// kai_selection_point: reads the current mouse position, returning {"x":0,"y":0} (screen
// coordinates, origin bottom-left).
@_cdecl("kai_selection_point")
public func kai_selection_point(
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let loc = NSEvent.mouseLocation  // Cocoa coordinates, origin top-left
  let h = NSScreen.screens.first?.frame.height ?? 0
  let point = SelectionPoint(x: Double(loc.x), y: Double(h - loc.y))  // converted to a bottom-left origin
  let json = bridgeEncode(point)
  bridgeFileLog(
    bridgeLogText(
      "selection.point_done", String(format: "%.1f", point.x), String(format: "%.1f", point.y)),
    level: BRIDGE_LOG_DEBUG)
  return writeCString(json, into: out, cap: out_cap)
}

// kai_screen_size: returns the primary screen size {"w":0,"h":0} (logical points).
@_cdecl("kai_screen_size")
public func kai_screen_size(
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  guard let main = NSScreen.screens.first else {
    let json = bridgeEncode(ScreenSize(w: 0, h: 0))
    return writeCString(json, into: out, cap: out_cap)
  }
  let size = ScreenSize(w: Double(main.frame.width), h: Double(main.frame.height))
  let json = bridgeEncode(size)
  return writeCString(json, into: out, cap: out_cap)
}

// Note: selection text reading (AXUIElement's AXSelectedText) used to live in
// kai_selected_text;
// it failed on most apps (empty returns / deadlocks), and the Go side's engine.CaptureRegion
// + OCR actually handles it,
// so that entry is disabled; the comment is kept to prevent misuse:
//   // @_cdecl("kai_selected_text")
//   // public func kai_selected_text(...) -> Int32 { ... AXUIElementCopyAttributeValue(AXValue(...)) ... }
//
// Input Monitoring permission detection: judged unauthorized when EventTap creation fails.
// Used to live in kai_input_monitoring, but event click forwarding is handled by the Go
// side's hotkey — Swift is no longer responsible;
// only the query capability is kept; see the commented example in the detectSourceLanguage
// module.

// kai_input_monitoring_enabled: detects whether Input Monitoring is granted, with
// CGPreflightListenEventAccess(), which never shows a permission prompt and creates nothing. (It used
// to create and drop a real event tap to see whether creation failed; that can ask the user for the
// permission, and the Settings page now polls this every 3 seconds.)
// out receives "true"/"false".
@_cdecl("kai_input_monitoring_enabled")
public func kai_input_monitoring_enabled(
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let enabled = CGPreflightListenEventAccess()
  bridgeFileLog(bridgeLogText("input.tap_enabled", String(enabled)), level: BRIDGE_LOG_DEBUG)
  return writeCString(enabled ? "true" : "false", into: out, cap: out_cap)
}

// kai_input_monitoring_request: makes macOS list Kai under Privacy & Security > Input Monitoring,
// then opens that pane. The registering call is the Core Graphics listen-event request below; it
// never creates an event tap. Only the Grant button calls this: the 3-second Settings poll uses the read-only
// kai_input_monitoring_enabled above.
// A 0 return means the request was made and the pane opened ("requested", not "granted").
@_cdecl("kai_input_monitoring_request")
public func kai_input_monitoring_request() -> Int32 {
  bridgeFileLog(bridgeLogText("input.request"))
  _ = CGRequestListenEventAccess()
  if let url = URL(
    string: "x-apple.systempreferences:com.apple.preference.security?Privacy_ListenEvent")
  {
    NSWorkspace.shared.open(url)
  }
  bridgeFileLog(bridgeLogText("input.request_done"))
  return 0
}
