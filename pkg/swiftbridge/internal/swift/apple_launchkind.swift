// apple_launchkind.swift
// Tells a launch at login apart from a launch by the user (Dock, Finder, open) — issue #17.
//
// macOS tells an app how it was launched through the 'oapp' Apple event it is sent while it
// finishes launching: when the launch is a login item, the event's keyAEPropData ('prdt')
// parameter is keyAELaunchedAsLogInItem ('lgit'). That event is only "current"
// (NSAppleEventManager.currentAppleEvent) while AppKit is delivering it, i.e. synchronously inside
// the will/did-finish-launching notifications, so a NotificationCenter observer with no queue
// (runs on the posting thread, synchronously) is the one place it can be read. A Go handler of
// Wails' ApplicationDidFinishLaunching is dispatched asynchronously and would see nil.
//
// kai_launch_observe must be called before NSApplication runs (main.go, before app.Run()).
import AppKit
import Foundation

private let launchLock = NSLock()
private var launchObserving = false
private var launchEventSeen = false
private var launchAsLoginItem = false

// 'prdt' and 'lgit' as four-char codes (AppleEvents.h keyAEPropData, keyAELaunchedAsLogInItem).
private let kaiKeyAEPropData: AEKeyword = 0x70726474
private let kaiLaunchedAsLogInItem: OSType = 0x6C676974

private func recordLaunchEvent() {
  guard let event = NSAppleEventManager.shared().currentAppleEvent else { return }
  let isLogin =
    event.paramDescriptor(forKeyword: kaiKeyAEPropData)?.enumCodeValue == kaiLaunchedAsLogInItem
  launchLock.lock()
  launchEventSeen = true
  if isLogin { launchAsLoginItem = true }
  launchLock.unlock()
  bridgeFileLog(bridgeLogText("launch.event", String(isLogin)))
}

// kai_launch_observe: registers the launch observers (idempotent). Returns 0.
@_cdecl("kai_launch_observe")
public func kai_launch_observe() -> Int32 {
  launchLock.lock()
  defer { launchLock.unlock() }
  if launchObserving { return 0 }
  launchObserving = true
  let center = NotificationCenter.default
  _ = center.addObserver(
    forName: NSApplication.willFinishLaunchingNotification, object: nil, queue: nil
  ) { _ in recordLaunchEvent() }
  _ = center.addObserver(
    forName: NSApplication.didFinishLaunchingNotification, object: nil, queue: nil
  ) { _ in recordLaunchEvent() }
  return 0
}

// kai_launch_kind: 0 = unknown (no launch event was seen, or the observer was never registered),
// 1 = launched by the user, 2 = launched as a login item.
@_cdecl("kai_launch_kind")
public func kai_launch_kind() -> Int32 {
  launchLock.lock()
  defer { launchLock.unlock() }
  if !launchEventSeen { return 0 }
  return launchAsLoginItem ? 2 : 1
}
