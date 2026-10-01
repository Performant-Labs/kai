// apple_window.swift
// Native window chrome (issue #22). Wails sets only the NSWindow's own background colour; the
// WKWebView inside keeps its default white background and draws it again every time a hidden
// window is shown (WebKit drops its tiles while the window is hidden), so a dark window flashed
// white on every open. This makes the web view draw no background of its own, so the window's
// colour shows through until the page repaints, and sets the colour behind the page too.
//
// MUST run on the main thread (the Go caller wraps it in application.InvokeAsync): AppKit.
import AppKit
import Foundation
import WebKit

/// Finds the first WKWebView under view, depth first.
private func kaiFindWebView(_ view: NSView?) -> WKWebView? {
  guard let view = view else { return nil }
  if let web = view as? WKWebView { return web }
  for sub in view.subviews {
    if let found = kaiFindWebView(sub) { return found }
  }
  return nil
}

/// kai_window_set_webview_background: window is the NSWindow pointer (Wails' NativeWindow()), r g b
/// are 0-255. Returns 1 when a web view was found and updated, 0 when the window or web view is
/// missing (nothing is changed then).
@_cdecl("kai_window_set_webview_background")
public func kai_window_set_webview_background(_ window: UInt, _ r: Int32, _ g: Int32, _ b: Int32) -> Int32 {
  guard window != 0, let raw = UnsafeRawPointer(bitPattern: window) else { return 0 }
  let win = Unmanaged<AnyObject>.fromOpaque(raw).takeUnretainedValue()
  guard let nsWindow = win as? NSWindow, let web = kaiFindWebView(nsWindow.contentView) else { return 0 }
  let colour = NSColor(
    calibratedRed: CGFloat(r) / 255.0, green: CGFloat(g) / 255.0, blue: CGFloat(b) / 255.0, alpha: 1.0)
  // No public API draws a WKWebView without its background; this key is the long-standing private
  // property Wails itself uses for a transparent web view.
  web.setValue(false, forKey: "drawsBackground")
  if #available(macOS 12.0, *) {
    web.underPageBackgroundColor = colour
  }
  nsWindow.backgroundColor = colour
  return 1
}
