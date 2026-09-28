# Brief — issue #163: Screenshot Translate window reappears on relaunch, bypassing its own startup Hide()

## Issue

https://github.com/Performant-Labs/kai-private/issues/163

Summary: right after relaunching Kai as a genuinely fresh process, the Screenshot Translate
window (`model.WindowScreenshot`) was visible and focused on startup, even though `main.go`
creates it and calls `screenshotWindow.Hide()` immediately at creation (main.go, window
creation block around line 506-530). No `Show()`/hotkey-trigger call from the app's own code
appears in that session's `~/.kai/logs/kai.log` after the fresh launch.

## Investigation performed

1. Read `main.go`'s creation code for all three persistent windows (settings ~L424-456,
   translate ~L461-502, screenshot ~L506-541). All three are created via
   `app.Window.NewWithOptions(application.WebviewWindowOptions{...})` and then immediately
   call `.Hide()` synchronously, before `app.Run()` starts the Cocoa/Wails event loop. This
   pattern is identical across all three windows — none has any restoration opt-out.

2. Read Wails v3 beta.24 source (`$GOMODCACHE/github.com/wailsapp/wails/v3@v3.0.0-beta.24`):
   - `pkg/application/application_darwin_delegate.m`:
     `applicationSupportsSecureRestorableState:` is hardcoded to return `YES`. This is the
     switch that opts the whole app into macOS's Secure State Restoration ("Resume") feature
     at the `NSApplication` level. It is not configurable from Go — no `application.Options`
     or `application.MacOptions` field controls it.
   - Grepped the entire `pkg/application` tree for `Restorable`, `restorable`,
     `NSWindowRestoration`, `restorationClass`, `setIdentifier` — **none exist**. Wails v3
     never sets `NSWindow.restorable` (AppKit default: `YES`) and never assigns a window
     `identifier` or `restorationClass`. `WebviewWindowOptions` / `MacWindow` expose no
     restoration-related field at all.
   - `pkg/application/webview_window.go:1660` (`WebviewWindow.NativeWindow() unsafe.Pointer`)
     returns the raw `NSWindow*` for the current platform (darwin impl:
     `webview_window_darwin.go:1781`, returns `w.nsWindow`). This is the only Go-level escape
     hatch to reach the underlying `NSWindow`.

3. **Root cause**: macOS's Secure State Restoration ("Resume") persists each restorable
   top-level `NSWindow`'s visibility/frame to
   `~/Library/Saved Application State/net.dtapp.kai.savedState/` when the app quits (this is
   independent of an explicit `NSWindowRestoration` identifier/delegate — it is AppKit's
   generic per-window state persistence, active whenever `applicationSupportsSecureRestorableState:`
   returns `YES` and the window's `restorable` property is `YES`, the default). On next
   launch, AppKit replays this saved visibility during its own `NSApplication` launch
   sequence — which runs *before* the app's own `Common.ApplicationStarted`
   (`applicationDidFinishLaunching`) delegate callback fires, and unrelated to the
   already-completed synchronous `screenshotWindow.Hide()` call made at window-creation time
   in `main.go`. That `Hide()` call happens too early to matter: the OS restores visibility
   as part of its own launch machinery afterward, with nothing in the app re-asserting
   `Hide()` once Cocoa's own launch sequence has run.

   This is a latent bug on **all three** persistent windows (settings, translate, screenshot)
   — none opts out of restoration. It surfaces specifically on the screenshot window because
   it is the one most likely to be left visible when the process is killed uncleanly (per the
   issue's own framing); translate/settings are normally hidden (red-X hides, not
   force-quit-while-open) far more often in practice, but they carry the identical latent bug
   and are included in this fix (see Scope below).

4. **Chosen fix — real restoration opt-out, not a timing band-aid.** Checked whether this
   repo has a precedent for calling AppKit directly: `pkg/swiftbridge` deliberately documents
   itself as "zero cgo" (`pkg/swiftbridge/load.go` doc comment: "loads libkai_bridge.dylib at
   runtime via purego's Dlopen ... zero cgo; the bridge library is never statically linked
   into the main binary"). There is no raw-cgo/Objective-C `.m`/`.go` file anywhere in this
   repo (`internal/**/*_darwin.go` all go through the Swift dylib bridge via purego). Adding
   real cgo + `-framework Cocoa` would contradict that explicit architectural decision and
   inflate the build matrix for a one-line AppKit property.

   Instead: `github.com/ebitengine/purego` (already a direct dependency, `go.mod:11`, used by
   `pkg/swiftbridge`) ships a `objc` subpackage
   (`$GOMODCACHE/github.com/ebitengine/purego@v0.11.1/objc`) — a pure-Go, zero-cgo
   Objective-C runtime binding (`objc_msgSend` via purego function registration). Verified
   standalone (`go run` a throwaway `main.go` importing `github.com/ebitengine/purego/objc`)
   that `objc.GetClass`, `objc.RegisterName`, and `objc.ID(...).Send(...)` work correctly on
   this machine, including `respondsToSelector:` as a safety guard before invoking a
   selector. This needs **no new go.mod dependency** (same module/version already required)
   and **no Swift/dylib changes** (so `pkg/swiftbridge/scripts/build.sh` is untouched).

   The fix: send Objective-C `NSWindow.setRestorable:NO` via
   `objc.ID(uintptr(ptr)).Send(objc.RegisterName("setRestorable:"), false)`, guarded by a
   `respondsToSelector:` check (skip silently if the native handle doesn't respond — e.g. a
   nil/non-NSWindow handle in a future platform or a test double) so a bad pointer can never
   crash the app. This is the real, documented AppKit lever for "never let this window
   participate in Resume" (`NSWindow.restorable`, default `YES`) — not a delayed
   re-`Hide()` guess at OS timing.

   **CORRECTED call-site timing (round-1 architecture review, PASS-blocking finding, now
   fixed here):** the first draft of this brief called `win.NativeWindow()` immediately
   after each window's creation-time `.Hide()` in `main.go` (~L456/483/530) — i.e. still
   inside `main()`, before `app.Run()` (main.go:700). That is provably too early and would
   make the fix a silent no-op in production. Traced through Wails v3 beta.24 source:
   - `WindowManager.NewWithOptions` → `addAndRun` → `App.runOrDeferToAppRun`
     (`pkg/application/window_manager.go:54-71`, `pkg/application/application.go:1008-1021`):
     while `a.running == false` (true for all of `main()` until `app.Run()` sets it),
     the window is only queued into `a.pendingRun` — nothing runs yet.
   - `App.Run()` (`pkg/application/application.go:748-756`) sets `a.running = true` and
     only THEN, asynchronously (`go func(){ pending.Run() }()`), runs each pending window.
   - `WebviewWindow.Run()` (`pkg/application/webview_window.go:485-500`) is what actually
     sets `w.impl = newWindowImpl(w)` — i.e. creates the real `NSWindow`. Until this runs,
     `w.impl` is nil.
   - `WebviewWindow.Hide()` (`webview_window.go:527-534`): when `w.impl == nil` (true at
     the original call sites), it only sets `w.options.Hidden = true` and returns —
     consistent with the window still being created hidden once `Run()` does fire.
   - `WebviewWindow.NativeWindow()` (`webview_window.go:1659-1665`): returns `nil` whenever
     `w.impl == nil`.

   So at the original call sites, `win.NativeWindow()` is always `nil`, and
   `disableRestoration`'s nil-handle guard (by design) makes the call a silent no-op —
   `setRestorable:NO` would never actually reach a real `NSWindow` in the shipped app.

   **Corrected call site:** each window's native `NSWindow` (`w.impl`) is created inside
   `WebviewWindow.Run()`, invoked asynchronously after `app.Run()` starts. Wails emits
   `events.Common.WindowRuntimeReady` (`webview_window.go:843`, fired on the
   `wails:runtime:ready` frontend-JS message) once that window's webview/runtime has
   finished initializing — well after `w.impl` (and thus `NativeWindow()`) exists, and
   still milliseconds after launch, long before any user interaction or realistic unclean
   quit. Register the opt-out on each window's `WindowRuntimeReady` event instead of at
   creation time:
   ```go
   win.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
       windowSvc.DisableRestoration(win)
   })
   ```
   placed right after each window's `NewWithOptions(...)` call (before or after the
   existing `.Hide()` — order between the two doesn't matter, since `Hide()` only ever
   toggles `options.Hidden`/`impl.hide` and never touches restorability). The Objective-C
   call itself is idempotent (`setRestorable:NO` repeated has no additional effect), so no
   extra guard is needed against `WindowRuntimeReady` firing more than once, but in
   practice it fires once per window per process lifetime.

## Chosen design (testable seam, matching this repo's existing pattern)

`internal/service/window_toggle_test.go` already establishes the pattern this repo uses for
testing window logic without a real Wails/AppKit runtime: a narrow interface + a fake that
satisfies it (`windowToggler`, `levelWindow`), with a package-level function under test taking
that interface plus an injected side-effect callback. Follow the same shape:

- New file `internal/service/window_restoration.go` (builds on every platform):
  ```go
  // nativeWindowHandle is the slice of application.Window that disableRestoration needs —
  // real Wails windows and test fakes alike only need to hand back their native handle.
  type nativeWindowHandle interface {
      NativeWindow() unsafe.Pointer
  }

  // disableRestoration opts win out of macOS's Secure State Restoration ("Resume"): it calls
  // setNotRestorable with the window's native handle. Safe to call on any platform/test
  // double — a nil window, a nil callback, or a nil native handle (non-darwin builds, where
  // NativeWindow() returns nil) is a no-op.
  func disableRestoration(win nativeWindowHandle, setNotRestorable func(unsafe.Pointer)) {
      if win == nil || setNotRestorable == nil {
          return
      }
      ptr := win.NativeWindow()
      if ptr == nil {
          return
      }
      setNotRestorable(ptr)
  }
  ```
- New file `internal/service/window_restoration_darwin.go` (`//go:build darwin`): provides
  `SetWindowNotRestorable(ptr unsafe.Pointer)`, the real implementation using
  `github.com/ebitengine/purego/objc` as described above (with the
  `respondsToSelector:` guard, and a `recover()` around the `objc.Send` call as defense in
  depth against any unexpected native panic — mirroring `swiftbridge.registerSafe`'s existing
  panic-to-error pattern elsewhere in this codebase).
- New file `internal/service/window_restoration_other.go` (`//go:build !darwin`): a no-op
  `SetWindowNotRestorable(ptr unsafe.Pointer)` stub (this bug and its fix are darwin-only —
  Windows/Linux have no equivalent OS-level window-restoration surprise here).
- `internal/service/window_wrapper.go`: a new small exported method on `WindowWrapper`,
  `DisableRestoration(win application.Window)`, that calls
  `disableRestoration(win, SetWindowNotRestorable)` (package-level `SetWindowNotRestorable`
  from the two build-tagged files above).
- `main.go`: right after each of the three windows' `NewWithOptions(...)` call (settings
  ~L424, translate ~L461, screenshot ~L506 — order relative to the existing `.Hide()` call
  doesn't matter, per the timing correction above), register:
  ```go
  screenshotWindow.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
      windowSvc.DisableRestoration(screenshotWindow)
  })
  ```
  (and the equivalent for `settingsWindow`/`translateWindow`). Three small additions,
  consistent with how `main.go` already delegates window behavior to `WindowWrapper` and
  already registers other `RegisterHook`/`OnWindowEvent`-style callbacks per window (e.g.
  the existing `events.Common.WindowClosing` hooks on all three windows).

## Test-first plan (T authors RED before F writes code)

Unit-testable without a real Wails/AppKit runtime, exactly like `window_toggle_test.go`:

- `TestDisableRestorationCallsSetNotRestorableWithNativeHandle`: a fake window whose
  `NativeWindow()` returns a fixed non-nil `unsafe.Pointer`; assert the injected
  `setNotRestorable` callback is invoked exactly once with that exact pointer.
- `TestDisableRestorationNoopsOnNilWindow`: `disableRestoration(nil, cb)` — assert `cb` is
  never called (no panic).
- `TestDisableRestorationNoopsOnNilNativeHandle`: fake window whose `NativeWindow()` returns
  `nil` (the non-darwin/no-op-stub case) — assert `cb` is never called.
- `TestDisableRestorationNoopsOnNilCallback`: `disableRestoration(fakeWin, nil)` — assert no
  panic.
- A `WindowWrapper.DisableRestoration` seam test (or equivalent) confirming it's wired to
  `service.SetWindowNotRestorable` by default (can inject a package-level var/func for the
  real callback the same way `ToggleTranslateWindow`'s `show func()` is injected, so this
  stays testable without touching main.go's imperative flow).

These are the RED tests (fail before `disableRestoration`/`WindowWrapper.DisableRestoration`
exist), and go GREEN once F implements exactly the design above.

**Wiring verification (main.go call-site timing, the round-1 review's blocking finding):**
`main.go` has no existing test coverage (it is the Wails app-composition root, not a
tested package in this repo), so the exact `OnWindowEvent(events.Common.WindowRuntimeReady, ...)`
registration per window cannot be unit-tested the same way. F must, as part of this story,
grep the final `main.go` diff to confirm: (a) all three registrations are wired to
`events.Common.WindowRuntimeReady`, not any earlier lifecycle point where `NativeWindow()`
would still be nil per the timing trace above, and (b) each closure captures the correct
window variable (a classic Go closure-over-loop-variable risk — these three are separate
statements, not a loop, so this is low-risk here, but F should double check no shared
variable is accidentally captured across the three registrations). S (spec auditor) should
independently re-verify this call-site placement against the corrected timing reasoning in
this brief before sign-off, since it is exactly the class of defect the round-1 review
caught.

`SetWindowNotRestorable`'s
actual AppKit effect (does `setRestorable:NO` really stick) is not mechanically verifiable in
`go test` — that remains a real-machine/manual verification concern (same category as this
repo's existing accessibility/screen-recording permission calls, which are also
`swiftbridge`-backed and not unit-tested for their OS-level effect). The RED/GREEN suite
instead locks the **wiring**: that all three windows get the opt-out call at creation with
their correct native handle, and that the seam degrades safely (never panics) under nil
inputs.

## i18n note

No new log lines are planned (the fix is silent/structural — a `respondsToSelector:` guard
failing silently is the correct behavior, not an error worth logging, matching how
`swiftbridge.Available()` guards elsewhere degrade without a log line at the call site). If F
finds a genuine need for a log line (e.g. logging when `respondsToSelector:` returns false, to
catch a future Wails upgrade that changes the native window class), add the key to **both**
`internal/i18n/locales/split/en-US/logs.json` and
`internal/i18n/locales/split/zh-CN/logs.json` (parity enforced by
`internal/i18n/locales_parity_test.go`), following the existing `log.window_handle_failed` /
`log.swiftbridge_unavailable` naming convention.

## Scope

In scope:
- `internal/service/window_restoration.go` (new, cross-platform, pure Go, unit-testable core)
- `internal/service/window_restoration_darwin.go` (new, darwin-only, `purego/objc` call)
- `internal/service/window_restoration_other.go` (new, non-darwin no-op stub)
- `internal/service/window_wrapper.go` — new `DisableRestoration` method
- `main.go` — three one-line call additions at window creation (settings, translate,
  screenshot)
- New unit tests per the Test-first plan above

Explicitly **not** in scope (do not touch): tray menu, hotkey registration, OCR/translate
flow, the `WindowClosing` hooks, `EventScreenshotRecapture`/`EventScreenshotRetranslate`
handlers, Windows/Linux window behavior (no-op stub only), any refactor of how the three
windows are created beyond adding the one-line opt-out call each.

## Why all three windows, not just screenshot

The issue's own "To investigate" section asks whether the restoration opt-out is "already set
for the other windows ... or missing everywhere." Investigation confirms: missing everywhere
(no window in this codebase, nor anywhere in Wails v3, opts out of restoration). Since
settings and translate share the exact same creation-then-`Hide()` shape as screenshot, they
carry the identical latent bug and would reproduce the identical symptom under the identical
trigger (process killed while one of them happens to be visible). Fixing only the reported
window while knowingly leaving two structurally-identical windows with the same bug would be
an incomplete fix for a root cause explicitly framed as "the OS doesn't know about our
programmatic Hide() at all" — that framing doesn't change per-window. This is called out here
per the task's explicit instruction to state such expansions rather than expand silently.
