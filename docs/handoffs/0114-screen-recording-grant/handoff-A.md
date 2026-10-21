# Handoff-A: Phase 3 — architecture review (up-front plan review), issue #114

**Date:** 2026-10-21
**Branch:** `issue-0114-screen-recording-grant` (worktree `.worktrees/0114-screen-recording-grant`)
**Brief reviewed:** `docs/handoffs/0114-screen-recording-grant/brief.md` · **Reuse map:** `docs/handoffs/0114-screen-recording-grant/survey.md` §Reuse & Analogous-Feature map · **Wireframe:** N/A (`uiSurface: false`, Phase D skipped and declared — brief.md:17-18)
**Verdict:** **PASS** (0 block · 2 warn · 2 nit)

## Summary

The plan extends exactly the right object at exactly the right layer: the escalation block inside
`kai_screenrecording_request` (the single owner of the Screen Recording TCC choreography), with the
Swift C ABI, the Go function-pointer contract, the Wails binding, and the i18n key all unchanged.
Extend-vs-new is honored in every row of the Reuse map — no new module, file, route, or API — and
the test plan extends the repo's established source-text/vitest pin idioms rather than inventing
new ones. No parallel path exists or is proposed. Two sharp edges are settled by ruling below so
they cannot become rework: the pane must open on the **failure** outcome of the stream start (the
expected, enrollment-producing path — R3), and the rationale comments that today justify the
metadata-only design are explicitly in scope to rewrite (A2).

## Evidence baseline (verified verbatim against the worktree)

- The object under extension — `pkg/swiftbridge/internal/swift/apple_accessibility.swift:87-96`:

  ```swift
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
  ```

  Envelope above it: `NSApp.activate()` + `CGRequestScreenCaptureAccess()` (:74-75), `openPane`
  once-guard + `Privacy_ScreenCapture` URL (:77-86), main-thread dispatch (:98-102). All as the
  survey claims.
- The pin to re-pin — `pkg/swiftbridge/screenrecording_source_test.go:143-145`:

  ```go
  ask := strings.Index(body, "CGRequestScreenCaptureAccess()")
  pre := strings.Index(body, "CGPreflightScreenCaptureAccess()")
  capture := strings.Index(body, "SCShareableContent")
  ```

  plus the guard pin at :158 (`"if !CGPreflightScreenCaptureAccess()"`). `SCShareableContent`
  appears in exactly one test in the repo (grep over `pkg/`, `internal/`), so the scope row for
  this file is complete.
- Go path unchanged and correct — `internal/service/accessibility_darwin.go:58-64`:

  ```go
  func (s *AppService) openScreenRecordingSettings() {
      s.log.Info(i18n.T("log.screenrecording_request"))
      if !swiftbridge.Available() {
          return
      }
      swiftbridge.KaiScreenRecordingRequest()
  }
  ```

  Wails binding `OpenScreenRecordingSettings` at `internal/service/app.go:117-121` (survey/journal
  cite 114-119 — corrected anchor; same function). Frontend consumer
  `frontend/src/components/settings/ShortcutsTab.svelte:133-141` (`openScreenRecording`) and the
  Grant button `:520-527`; the missing-copy render `:506-509`.
- Contract stability — `pkg/swiftbridge/load.go:56` `KaiScreenRecordingRequest func() int32`,
  registered by symbol name at `load.go:222` (`register(&KaiScreenRecordingRequest,
  "kai_screenrecording_request")`). The plan changes only the Swift function **body**; the ABI
  (`@_cdecl ... -> Int32`, no args) is untouched, so no Go-side change is needed — decision 4 is
  correct.
- Copy anchors — `frontend/src/i18n/en-US.ts:368-369` `'Turn on Kai in Screen & System Audio
  Recording (click + to add it), then restart Kai.'` and `frontend/src/i18n/zh-CN.ts:351-352`
  `'请在“屏幕与系统音频录制”中打开 Kai（没有时点 + 添加），然后重启 Kai。'`. Key
  `settings.permScreenRecordingMissing` typed at `frontend/src/i18n/keys.ts:346` — **not** in
  scope, correctly: the key name is unchanged. AC3 replacements measure 65 chars (en) and 30 chars
  (zh), both ≤110, both name the pane and the restart, both drop the "+ add" teaching.
- Test safety (AC4) — `internal/service/accessibility_darwin_test.go:49-57` `unwireBridgeRequests`
  already clears `swiftbridge.KaiScreenRecordingRequest` among its four pointers; the plan adds no
  Go test, so nothing new can wire a real request pointer. Holds.
- Availability — `pkg/swiftbridge/scripts/build.sh` pins `SDK_MIN` to ≥ 26.0, and the file already
  imports ScreenCaptureKit (`apple_accessibility.swift:11`) and calls `SCShareableContent`
  unguarded (:90). `SCStream` / `SCContentFilter` / `SCStreamConfiguration` /
  `startCapture(completionHandler:)` are macOS 12.0+ APIs — **no availability guard is needed or
  wanted**; adding one would itself be drift against the neighboring call.
- Changelog — `CHANGELOG.md:8` `## [Unreleased]` exists (empty); header convention says "Add a line
  under `[Unreleased]` when a change merges". The 0.6.0 Known Issues entry describing this bug
  stays untouched (released sections are immutable per the same header).
- No pin collisions — `frontend/src/components/settings/shortcutsTabGrantButtons.test.ts` pins
  button labels and one-binding-per-click, not the missing copy; `swift_source_test.go:147`'s
  banned-strings list (`"NSApp"`, `"DispatchQueue.main"`, …) applies only to
  `kai_detect_language`'s body, so the new Swift code cannot trip it.

## Findings

| # | Severity | Plan element | Drift dimension | Finding | Suggested fix |
|---|---|---|---|---|---|
| 1 | warn | AC1 escalation ("a started `SCStream`, stopped immediately") | error handling (cross-cutting) | The **expected** outcome of the start is a `.notPermitted` **error** — that failure is what enrolls Kai. The natural-but-wrong translation of the current shape puts `openPane()` only in `stopCapture`'s completion, leaving the enrollment path riding the 3 s deadline alone. | Adopt R3: `openPane()` from **both** outcomes of `startCapture` (error and success), dispatched to main; T pins that `openPane()` is reachable from the start-capture completion handler (A1). |
| 2 | warn | Scope row 1 ("replace the metadata-only escalation") | documentation consistency | The doc comment `apple_accessibility.swift:62-67` and the inline comment `:88-89` justify the *metadata-only* design ("no pixels are captured, so no flash and no second prompt"); `screenrecording_source_test.go:128-130` repeats it. Left stale they directly contradict the new pins and mislead the next reader of a rationale-carrying file. | Adopt A2: the comment rewrite is explicit, non-optional scope for F (Swift comments) and T (test-file comment). |
| 3 | nit | AC3 en copy | naming | The new en copy omits the "Privacy & Security >" prefix the neighboring a11y row keeps (`en-US.ts:366`). Pre-existing per-key convention — the *current* SR copy also omits it (`en-US.ts:369`) — so the settled wording (decision 3, verbatim) stands; recorded, no change. | None. |
| 4 | nit | Settled decision 1 | resource lifecycle | On the expected `.notPermitted` path there is no running capture to stop; `stopCapture` belongs on the success path only. The stream must be held strongly by the closure until it answers (an `SCStream` released early can tear down before TCC evaluates). | Implementation note for F; T's presence pin for `stopCapture` (A1) is satisfied by the success path. |

No parallel path is created; the extension is clean. Findings 1-2 are settling, not blocking: both
are refinements of a correctly-shaped plan, absorbed as rulings/amendments below.

## Rulings (binding for F and T)

- **R1 — primitive confirmed: a started `SCStream` via ScreenCaptureKit.** Not `CGDisplayStream`
  (deprecated on the macOS line where the pane is "Screen & System Audio Recording", and the file
  already imports ScreenCaptureKit at `:11`). This settles the "Assumed" hedge in
  `decisions.md:51-53`.
- **R2 — in-place replacement inside the existing guard.** The stream attempt replaces the
  `SCShareableContent` escalation exactly where it sits (`apple_accessibility.swift:87-96`), inside
  `if !CGPreflightScreenCaptureAccess()`. `SCShareableContent` may remain **only** to build the
  `SCContentFilter` (settled decision 1). Binding order: ask → preflight → attempt → `openPane()`
  last; the guard pin (`screenrecording_source_test.go:158`) and the four envelope pins
  (`:28-49`, `:131-139`) keep holding untouched.
- **R3 — the pane opens on the attempt's answer, and the `.notPermitted` error *is* the answer.**
  `openPane()` must be reachable from both outcomes of `startCapture`, via `DispatchQueue.main`,
  with the existing once-guard and 3 s deadline unchanged (they remain the backstop, not the
  primary path). This is AC1's "on the attempt's answer" made explicit so no one reads error ≠
  answer.

## Amendments to the brief

- **A1 (test plan sketch — exact re-pin set).** For
  `TestScreenRecordingRequestEscalatesToCaptureOnlyWhenNotGranted`
  (`screenrecording_source_test.go:141-161`): the `capture` index target at `:145` changes from
  `"SCShareableContent"` to the stream-start marker (e.g. `SCStream(`/`startCapture`); add
  presence pins for `SCStream`, `startCapture` and `stopCapture`; keep the order pin
  (ask < pre < attempt < openPane-last) and the guard pin. **Do not ban the string
  `SCShareableContent`** — it legitimately remains for filter building (R2), so the pin must
  require the stream start, not assert the metadata call's absence. A metadata-only enumeration
  (no `SCStream`/`startCapture`) must fail the pin.
- **A2 (scope — comments explicit).** F rewrites the function doc comment
  (`apple_accessibility.swift:60-69`) and the inline comment (`:88-89`) to describe the one-off
  started-stream attempt; T updates the mirroring test comment (`:128-130`). English only
  (settled decision 5).

## Notes for O

- No re-review needed: verdict is PASS; A1/A2 are binding refinements F and T read from this
  handoff, and R1-R3 settle the journal's open assumption. No change to the brief's Scope table,
  AC set, or settled decisions is required beyond A1/A2.
- Anchor correction for later phases: the Wails binding is `internal/service/app.go:117-121`
  (survey and journal say 114-119 — overlapping but imprecise). Use the corrected anchor.
- The 0.6.0 Known Issues line describing this bug stays as-is (released sections immutable);
  AC5's `[Unreleased]` line is the only changelog touch. Confirmed feasible: the section exists
  and is empty (`CHANGELOG.md:8`).

## Patterns referenced

1. `pkg/swiftbridge/internal/swift/apple_accessibility.swift` — the sibling request functions
   (`kai_accessibility_request` :27-46, `kai_input_monitoring_request` :168-184) establish the
   file's "registering call, then pane" pattern and its rationale-carrying comment convention.
2. `pkg/swiftbridge/screenrecording_source_test.go` + `swift_source_test.go` (`readSwift` :23-30,
   `swiftFuncBody` :83-95) — the source-text pin idiom (presence pins, `strings.Index` order pins,
   guard pins) T extends.
3. `frontend/src/components/permissionCopy.test.ts` (:29-32 length pin for this exact key,
   :49-54 `toContain` "where" pins) — the copy-pin idiom T extends.
4. `internal/service/accessibility_darwin_test.go` (:42-57 `unwireBridgeRequests`, #101/#102) —
   the no-prompt test-safety pattern AC4 leans on, reused as-is.
5. `CHANGELOG.md` header (release convention: one line under `[Unreleased]` per merged change) —
   AC5's format baseline.
