# Survey — issue #114: Screen Recording "Grant access" does not get Kai listed

Run: `0114-screen-recording-grant` · worktree `.worktrees/0114-screen-recording-grant` · branch
`issue-0114-screen-recording-grant`. Survey done in the primary checkout at master `ab5ea85`
(2026-10-21); the worktree branches from the same commit.

## The story

After installing 0.6.0, clicking **Grant access** for Screen Recording in Settings > Shortcuts did
not make Kai appear (or switch on) in System Settings > Privacy & Security > **Screen & System
Audio Recording**. The owner had to remove Kai from the list, add `/Applications/Kai.app` by hand
with +, and restart Kai. Wanted: Grant access reliably registers Kai with macOS (trigger the
system screen-capture access request **and a real capture attempt** from the signed app before the
Privacy pane opens), and the row says plainly that Kai must be **restarted** after switching it on.

## What already exists (and why 0.6.0 still failed)

The #23-era fix is on master and shipped in 0.6.0 (`d7e859e`, 2026-09-30 ≤ v0.6.0, 2026-10-20):

- `pkg/swiftbridge/internal/swift/apple_accessibility.swift:70-104` — `kai_screenrecording_request`
  already: runs on the main thread with `NSApp.activate()`, calls `CGRequestScreenCaptureAccess()`,
  and — only when `CGPreflightScreenCaptureAccess()` still says not-granted — makes **one
  metadata-only** `SCShareableContent.getExcludingDesktopWindows(...)` call, opening the pane last
  (on callback or a 3 s deadline).
- `internal/service/accessibility_darwin.go:56-64` — `openScreenRecordingSettings()` logs and calls
  `swiftbridge.KaiScreenRecordingRequest()` (guarded by `swiftbridge.Available()`); Wails binding
  `OpenScreenRecordingSettings` at `internal/service/app.go:114-119`; the frontend button calls it
  (`frontend/src/components/settings/ShortcutsTab.svelte:133-139`, Grant button `:524-526`).

Defect analysis (why the owner still hit it):

1. `CGRequestScreenCaptureAccess()` is a silent no-op when TCC already holds a stale record for the
   bundle id (asked/denied by an earlier Kai version) — it does not re-prompt and does not list.
2. The escalation is **metadata-only**: on the macOS line where the pane is "Screen & System Audio
   Recording", enumerating `SCShareableContent` does not enroll the app in TCC. What reliably
   enrolls is **starting a real capture** (a started `SCStream`; the start fails with
   `.notPermitted` but macOS then lists the app — the standard, pixel-free-enrollment route).
3. So the pane opened with Kai unlisted, and the only working path was the owner's manual
   remove/+/restart.

The Settings row copy currently teaches that manual workaround:
`frontend/src/i18n/en-US.ts:368-369` `permScreenRecordingMissing` = "Turn on Kai in Screen & System
Audio Recording **(click + to add it)**, then restart Kai." and `frontend/src/i18n/zh-CN.ts:351-352`
= "请在“屏幕与系统音频录制”中打开 Kai**（没有时点 + 添加）**，然后重启 Kai。"

## Test topology (what T can and cannot pin)

- **Swift**: CI has no Swift compiler — the repo pins Swift behavior with **source-text tests**
  (`pkg/swiftbridge/screenrecording_source_test.go`, `swift_source_test.go`'s `readSwift` helper).
  Existing pins: ask-before-pane, pane URL `Privacy_ScreenCapture`, main-thread + `NSApp.activate`,
  escalation order ask → preflight → `SCShareableContent` → `openPane()` last, guarded by
  `if !CGPreflightScreenCaptureAccess()`. The local suite DOES compile the dylib first
  (`test.unit.command` runs `pkg/swiftbridge/scripts/build.sh`), but no test may *execute* a request.
- **Go service**: tests stub the bridge pointers. `internal/service/accessibility_darwin_test.go`
  owns `unwireBridgeRequests(t)` (issue #101/#102): the four request pointers are cleared when a
  real-bridge test ends, so no `go test` run opens System Settings or pops a permission prompt.
  Read-only query pointers stay wired.
- **Frontend**: vitest. `frontend/src/components/permissionCopy.test.ts` pins permission copy
  length (≤110 chars, both locales) and "says where to switch on";
  `shortcutsTabGrantButtons.test.ts` pins the Grant button labels.
- **Suite** (`.aftersight/pipeline.config.json`): build.sh → `go test -vet=off ./internal/... ./pkg/...`
  → `pnpm --dir frontend test` → the two release harnesses. Frontend needs
  `NODE_OPTIONS=--no-experimental-webstorage` on this Mac (CI parity; 0078 history).

## Reuse & Analogous-Feature map

| Need | Closest existing thing | Extend vs new |
|---|---|---|
| Reliable TCC enrollment | `kai_screenrecording_request` escalation block (`apple_accessibility.swift:87-96`) | **Extend**: replace the metadata-only attempt with a real one-off started-`SCStream` capture attempt (start → stop immediately; no frames retained). Request → preflight → attempt → pane-last order and the main-thread/activate envelope stay. |
| Pane URL / envelope | same function (`Privacy_ScreenCapture`, `openPane()` closure, 3 s deadline) | Reuse unchanged |
| Go service call path | `accessibility_darwin.go:56-64` + `app.go:114-119` | **Unchanged** (already routes the button to the request; no new API) |
| Swift source pins | `screenrecording_source_test.go` | **Extend** (re-pin the escalation to a started stream; keep order/envelope pins) |
| No-prompt test safety | `unwireBridgeRequests` (#101/#102) | Reuse as-is; no new real-bridge test needed |
| Row copy | `settings.permScreenRecordingMissing` (en/zh) | **Edit** the two strings (drop the "+ add" teaching, say the restart plainly); `ShortcutsTab.svelte` itself unchanged |
| Copy pins | `permissionCopy.test.ts` | **Extend** |
| Changelog | `CHANGELOG.md` `[Unreleased]` | One user-visible line (#114) |

**Recommendation: extend everywhere, add no new module/file except nothing — no new routes,
entities, or Go API.** The analogous Input Monitoring request in the same Swift file
(`kai_input_monitoring_request`) is the style reference; it is NOT in scope.

## Constraints (binding)

- **Never touch the machine's TCC/Privacy state**: no `tccutil`, no system-settings changes, in
  tests or verification (operator rule; the issue's clean-state check is the owner's to run with
  the real signed app — unit tests over a stub cannot show it, as the issue itself states).
- Tests must follow the unwired-bridge pattern: no test opens a real macOS prompt or the Privacy pane.
- Text change only: no new layout, no new UI surface (D and U are **N/A, declared**).
- i18n strings in **both** en-US and zh-CN; new code (identifiers/comments) in English only.
- Do not touch files outside the story's scope (other runs work in parallel on other issues).
