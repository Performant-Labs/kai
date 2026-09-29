// apple_doublecopy.swift
// Double Cmd+C (issue #199): a listen-only event tap that records Cmd+C key-downs, plus the small
// reads the Go side needs to decide what a pair means (pasteboard change count and types, the
// frontmost app).
//
// Thread rules, in one place:
//  - The tap is created with .listenOnly: it can observe a key but never swallow, delay or alter it.
//  - It runs on a thread of its own with its own run loop. Nothing here uses the main thread, the
//    main queue or the main actor, and nothing posts a CGEvent (posting is what must be on the main
//    thread; see internal/execkey/copy_darwin.go and PR #168).
//  - The Go side is never called back from the tap's thread. The tap appends to a locked queue and
//    Go polls it with kai_doublecopy_poll, so no foreign-thread callback ever enters Go.
//  - Only Cmd+C is ever kept. Every other key-down is dropped on the tap's thread before anything
//    is recorded, so this is not a keylogger.
//  - kai_doublecopy_start never prompts: it preflights the permission and returns 1 when it is
//    missing. Only kai_doublecopy_request may show the system prompt.
import AppKit
import ApplicationServices
import CoreGraphics
import Foundation

private let dcKeyCodeC: Int64 = 8
private let dcCommand: UInt64 = 0x100000
private let dcOtherModifiers: UInt64 = 0x20000 | 0x40000 | 0x80000  // shift, control, option
private let dcQueueLimit = 100

/// One recorded Cmd+C key-down. Field names match the Go side's wireEvent.
struct DoubleCopyEvent: Codable {
  let atNs: Int64
  let key: Int64
  let flags: UInt64
  let isRepeat: Bool
  let own: Bool
  let frontKai: Bool
  let cc: Int
  let bundle: String

  enum CodingKeys: String, CodingKey {
    case atNs = "at_ns"
    case key
    case flags
    case isRepeat = "repeat"
    case own
    case frontKai = "front_kai"
    case cc
    case bundle
  }
}

struct DoublePasteboardInfo: Codable {
  let count: Int
  let types: [String]
}

/// Shared state of the listener, all behind `lock`.
final class DoubleCopyState: @unchecked Sendable {
  static let shared = DoubleCopyState()
  let lock = NSLock()
  var queue: [DoubleCopyEvent] = []
  var suppressUntilNs: UInt64 = 0
  var tap: CFMachPort?
  var runLoop: CFRunLoop?
  var running = false
  var stopped: DispatchSemaphore?
}

private func dcNowNs() -> UInt64 {
  return clock_gettime_nsec_np(CLOCK_UPTIME_RAW)
}

/// The body of the tap callback: filters, marks and queues one key-down. Also exported as
/// kai_doublecopy_ingest so it can be exercised without a real tap (which needs the permission).
func doubleCopyIngest(keycode: Int64, flags: UInt64, autorepeat: Bool, srcPid: Int64) {
  // Only a plain Cmd+C is kept; a caps-lock, fn or numpad bit does not change the chord.
  guard keycode == dcKeyCodeC, flags & dcCommand != 0, flags & dcOtherModifiers == 0 else { return }
  let now = dcNowNs()
  let st = DoubleCopyState.shared
  // Kai's own simulated copy: posted by this process (source pid), or inside the suppress window
  // Go opens right before it posts.
  var own = srcPid != 0 && srcPid == Int64(getpid())
  st.lock.lock()
  if now < st.suppressUntilNs { own = true }
  st.lock.unlock()
  let front = NSWorkspace.shared.frontmostApplication
  let event = DoubleCopyEvent(
    atNs: Int64(truncatingIfNeeded: now),
    key: keycode,
    flags: flags,
    isRepeat: autorepeat,
    own: own,
    frontKai: front?.processIdentifier == getpid(),
    cc: NSPasteboard.general.changeCount,
    bundle: front?.bundleIdentifier ?? ""
  )
  st.lock.lock()
  st.queue.append(event)
  if st.queue.count > dcQueueLimit { st.queue.removeFirst(st.queue.count - dcQueueLimit) }
  st.lock.unlock()
}

private func doubleCopyTapCallback(
  _ proxy: CGEventTapProxy, _ type: CGEventType, _ event: CGEvent,
  _ refcon: UnsafeMutableRawPointer?
) -> Unmanaged<CGEvent>? {
  switch type {
  case .tapDisabledByTimeout, .tapDisabledByUserInput:
    let st = DoubleCopyState.shared
    st.lock.lock()
    let tap = st.tap
    st.lock.unlock()
    if let tap = tap { CGEvent.tapEnable(tap: tap, enable: true) }
  case .keyDown:
    doubleCopyIngest(
      keycode: event.getIntegerValueField(.keyboardEventKeycode),
      flags: event.flags.rawValue,
      autorepeat: event.getIntegerValueField(.keyboardEventAutorepeat) != 0,
      srcPid: event.getIntegerValueField(.eventSourceUnixProcessID))
  default:
    break
  }
  // A listen-only tap ignores the return value; the event is passed on untouched either way.
  return Unmanaged.passUnretained(event)
}

private final class TapStartBox: @unchecked Sendable { var code: Int32 = 2 }

// kai_doublecopy_start: starts the listen-only tap on its own thread. Returns 0 when listening (or
// already listening), 1 when Input Monitoring is not granted (nothing is created and no prompt is
// shown), 2 when the tap could not be created for another reason.
@_cdecl("kai_doublecopy_start")
public func kai_doublecopy_start() -> Int32 {
  let st = DoubleCopyState.shared
  st.lock.lock()
  if st.running {
    st.lock.unlock()
    return 0
  }
  st.lock.unlock()
  guard CGPreflightListenEventAccess() else { return 1 }

  let box = TapStartBox()
  let started = DispatchSemaphore(value: 0)
  let stopped = DispatchSemaphore(value: 0)
  let thread = Thread {
    let mask = CGEventMask(1 << CGEventType.keyDown.rawValue)
    guard
      let tap = CGEvent.tapCreate(
        tap: .cgSessionEventTap, place: .tailAppendEventTap, options: .listenOnly,
        eventsOfInterest: mask, callback: doubleCopyTapCallback, userInfo: nil)
    else {
      box.code = 1
      started.signal()
      stopped.signal()
      return
    }
    let source = CFMachPortCreateRunLoopSource(kCFAllocatorDefault, tap, 0)
    let loop = CFRunLoopGetCurrent()
    CFRunLoopAddSource(loop, source, .commonModes)
    CGEvent.tapEnable(tap: tap, enable: true)
    st.lock.lock()
    st.tap = tap
    st.runLoop = loop
    st.running = true
    st.stopped = stopped
    st.lock.unlock()
    box.code = 0
    started.signal()
    CFRunLoopRun()
    CGEvent.tapEnable(tap: tap, enable: false)
    CFMachPortInvalidate(tap)
    stopped.signal()
  }
  thread.name = "kai-doublecopy-tap"
  thread.start()
  started.wait()
  return box.code
}

// kai_doublecopy_stop: stops the tap and waits (bounded) for its thread to finish, so a start right
// after a stop always builds a fresh tap. Safe when not running.
@_cdecl("kai_doublecopy_stop")
public func kai_doublecopy_stop() -> Int32 {
  let st = DoubleCopyState.shared
  st.lock.lock()
  let loop = st.runLoop
  let stopped = st.stopped
  st.running = false
  st.runLoop = nil
  st.tap = nil
  st.stopped = nil
  st.queue.removeAll()
  st.lock.unlock()
  if let loop = loop { CFRunLoopStop(loop) }
  _ = stopped?.wait(timeout: .now() + 1.0)
  return 0
}

// kai_doublecopy_poll: writes and clears the recorded key-downs as a JSON array ([] when none).
@_cdecl("kai_doublecopy_poll")
public func kai_doublecopy_poll(
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let st = DoubleCopyState.shared
  st.lock.lock()
  let events = st.queue
  st.queue.removeAll()
  st.lock.unlock()
  return writeCString(bridgeEncode(events), into: out, cap: out_cap)
}

// kai_doublecopy_suppress: for the next `ms` milliseconds every recorded Cmd+C is marked as Kai's
// own. Go calls it right before Kai posts its simulated copy.
@_cdecl("kai_doublecopy_suppress")
public func kai_doublecopy_suppress(_ ms: Int32) -> Int32 {
  let st = DoubleCopyState.shared
  st.lock.lock()
  st.suppressUntilNs = dcNowNs() + UInt64(max(0, ms)) * 1_000_000
  st.lock.unlock()
  return 0
}

// kai_doublecopy_request: shows the system Input Monitoring prompt (once per install, the system
// decides) and opens Privacy & Security > Input Monitoring. Called only when the user switches the
// feature on.
@_cdecl("kai_doublecopy_request")
public func kai_doublecopy_request() -> Int32 {
  _ = CGRequestListenEventAccess()
  if let url = URL(
    string: "x-apple.systempreferences:com.apple.preference.security?Privacy_ListenEvent")
  {
    NSWorkspace.shared.open(url)
  }
  return 0
}

// kai_doublecopy_pasteboard: {"count":N,"types":[...]}: the pasteboard's change count and the type
// identifiers of all its items (the concealed/transient markers live there).
@_cdecl("kai_doublecopy_pasteboard")
public func kai_doublecopy_pasteboard(
  _ out: UnsafeMutablePointer<CChar>?,
  _ out_cap: Int32
) -> Int32 {
  let pb = NSPasteboard.general
  var seen = Set<String>()
  var types: [String] = []
  for item in pb.pasteboardItems ?? [] {
    for t in item.types where seen.insert(t.rawValue).inserted { types.append(t.rawValue) }
  }
  return writeCString(
    bridgeEncode(DoublePasteboardInfo(count: pb.changeCount, types: types)), into: out, cap: out_cap)
}

// kai_doublecopy_ingest: the tap callback's body as an entry point, so the filtering, marking and
// queue can be exercised without the Input Monitoring grant a real tap needs.
@_cdecl("kai_doublecopy_ingest")
public func kai_doublecopy_ingest(
  _ keycode: Int32, _ flags: UInt64, _ autorepeat: Int32, _ src_pid: Int32
) -> Int32 {
  doubleCopyIngest(
    keycode: Int64(keycode), flags: flags, autorepeat: autorepeat != 0, srcPid: Int64(src_pid))
  return 0
}
