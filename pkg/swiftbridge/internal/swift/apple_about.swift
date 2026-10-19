// apple_about.swift
// The About dialog (issue #8). Wails' default application menu shows macOS's standard About
// panel with the orderFrontStandardAboutPanel: selector, which orders the panel front only
// within the normal window level — kai's pinned translate window (always-on-top, floating
// level, Wails' SetAlwaysOnTop) stayed above the panel, so the About "dialog" opened behind the
// app's own windows. kai_show_about_panel activates the app, shows the standard panel, then
// lifts it to the floating level and makes it key so it opens in front of every kai window.
//
// MUST run on the main thread (the Go caller wraps it in application.InvokeAsync): AppKit.
import AppKit

/// kai_show_about_panel: shows the standard About panel in front of the app's windows (issue #8).
/// Returns 1 when the panel was found and raised, 0 when AppKit has no about panel (nothing is
/// changed then).
@_cdecl("kai_show_about_panel")
public func kai_show_about_panel() -> Int32 {
  // Clicking kai's own menu bar already makes kai active; this is belt-and-braces for any other
  // entry point (activate() never steals focus from a user who is mid-click elsewhere on 14+).
  NSApp.activate()
  // orderFrontStandardAboutPanel creates the panel lazily, so it only exists AFTER this call.
  NSApp.orderFrontStandardAboutPanel(nil)
  // There is no public class or identifier for the about panel; the long-standing AppKit
  // workaround matches the private NSAboutPanel* window by class name.
  guard let panel = NSApp.windows.first(where: {
    String(describing: type(of: $0)).contains("AboutPanel")
  }) else { return 0 }
  // The pinned translate window sits at the floating level (Wails' SetAlwaysOnTop); the same
  // level plus makeKeyAndOrderFront puts the panel above it.
  panel.level = .floating
  panel.makeKeyAndOrderFront(nil)
  return 1
}
