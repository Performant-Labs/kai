# Handoff-F: Phase 5 — Implement

**Date:** 2026-10-21
**Branch / worktree:** `issue-0114-screen-recording-grant` (`.worktrees/0114-screen-recording-grant`)
**Contract:** handoff-T-red.md (tests + the behaviors they pin), under binding rulings R1–R3 / amendments A1–A2 (handoff-A.md)

## What I changed

Production only — 4 files (the 4th via bash, disclosed below). No test file touched.

| File | Change | Satisfies |
|---|---|---|
| `pkg/swiftbridge/internal/swift/apple_accessibility.swift` | (a) Inside `if !CGPreflightScreenCaptureAccess()` (guard kept verbatim), replaced the metadata-only `SCShareableContent` escalation with a real one-off capture attempt: the shareable-content call now supplies `displays.first` for an `SCContentFilter(display:excludingWindows:)`; a minimal `SCStreamConfiguration` (display width/height, `showsCursor = false`) builds `SCStream(filter:configuration:delegate:)`; `stream.startCapture { error in … }` is the attempt; **`openPane()` (via `DispatchQueue.main.async`, once-guard unchanged) is called from BOTH outcomes** — the plain completion path (error or success) and, if the filter can't even be built (`guard let display … else`), immediately. `stopCapture` runs **only on the success path** (`if error == nil`), per A finding 4 (no running capture exists to stop on `.notPermitted`). The stream is held strongly until it answers: the completion closure captures `stream`, so ScreenCaptureKit's retain of the handler keeps it alive (comment says so at the construction site). Envelope untouched: `NSApp.activate()` → `_ = CGRequestScreenCaptureAccess()` → guard → attempt → pane-last; `Privacy_ScreenCapture` URL; `asyncAfter(.now() + 3)` backstop last. (b) Rewrote the function doc comment (:60-70) and the inline comment (:91-95) per A2 — they now describe the started-`SCStream` attempt and state that the expected `.notPermitted` failure is what enrolls Kai (issue #114, 0.6.0 metadata-only shape never did). English only. | AC1, AC2, R1–R3, A2 |
| `frontend/src/i18n/en-US.ts` | `settings.permScreenRecordingMissing` → `'Turn on Kai in Screen & System Audio Recording, then restart Kai.'` (65 chars; collapsed to the single-line prettier-canonical form — exactly 100 cols, same shape as the `permAccessibilityMissing` neighbor). Byte-equal to T's `toBe` pin (verified with node before running vitest). | AC3 |
| `frontend/src/i18n/zh-CN.ts` | Same key → `'请在“屏幕与系统音频录制”中打开 Kai，然后重启 Kai。'` (30 chars, single line). Byte-equal to T's pin (node-compared against `permissionCopy.test.ts`, `true` for both locales). | AC3 |
| `CHANGELOG.md` | One line under `## [Unreleased]` (the operator-supplied wording, verbatim, ending `(#114)`); released sections untouched. | AC5 |

Deliberately not changed (per brief scope + decision 4): no Go code (`internal/service/*` route untouched), no `ShortcutsTab.svelte`, no CI, no keys.ts.

Compile note: the macOS 27 SDK widened `SCStreamConfiguration.width/height` from `Int32` to `Int` — build.sh failed on `Int32(…)` and the committed fix is `Int(…)`. `SCStream(filter:configuration:delegate:)` resolves to the non-failable macOS 14+ initializer at the build target `arm64-apple-macosx27.0` (no optional unwrap needed), matching handoff-A's "no availability guard" ruling.

## Self-check

Ran the verbatim `test.unit.command` from `.aftersight/pipeline.config.json`:

```text
(cd pkg/swiftbridge/scripts && bash ./build.sh) && go test -vet=off ./internal/... ./pkg/... -count=1 \
  && pnpm --dir frontend test && bash scripts/release.test.sh && bash scripts/release-ask-ai.test.sh
```

Result: **exit 1, with every production-reachable check green and exactly ONE failure, a stale pre-existing test pin (below)**. Stage tails:

- build.sh: `>> done: pkg/swiftbridge/libkai_bridge.dylib` / `libkai_bridge.a` (Swift compiles clean; compile-only — nothing ran, no TCC state touched).
- Go: all 20 packages `ok`, including `ok cnb.cool/dtapp/kai/pkg/swiftbridge 0.259s` — T's two RED pins (`TestScreenRecordingRequestEscalatesToCaptureOnlyWhenNotGranted`, `TestScreenRecordingRequestOpensPaneOnTheStreamAnswer`) and all envelope pins now pass.
- vitest: `Test Files 1 failed | 78 passed (79)` / `Tests 1 failed | 872 passed (873)` — T's 3 new AC3 pins pass; the 1 failure is `shortcutsTabGrantHidden.test.ts:82` (test-owned stale pin, next section). The `&&` chain stopped there, so I ran the remaining two stages standalone: `bash scripts/release.test.sh` → `PASS` (exit 0); `bash scripts/release-ask-ai.test.sh` → `PASS` (exit 0). Neither reads the repo's real `CHANGELOG.md` (both use throwaway fixtures), so the AC5 line cannot affect them.

Note: the authoritative GREEN is the plugin's T-green run, not this self-check.

## Deviation disclosure — CHANGELOG.md write mechanism (0078 precedent)

`CHANGELOG.md` is outside `paths.production` in `.aftersight/pipeline.config.json`, so the `edit` tool denied it (`Permission denied: edit` — one attempt, on record). As pre-agreed in the dispatch instruction, the one-line insert was made via `bash`/python (exact operator-supplied wording, anchored uniquely between `## [Unreleased]` and `## [0.6.0]`). Production-repo file, content-only, disclosed here. Same class as T-red's vitest write; no config change made in this run.

## Tests I think are wrong (if any)

**One — and it is the only thing between this run and GREEN. T must repair it in T-green (I did not edit it; I can't).**

`frontend/src/components/settings/shortcutsTabGrantHidden.test.ts:82` pins the OLD copy that AC3 / settled decision 3 deliberately delete. Verbatim (`sed -n '76,83p'`):

```text
  it('shows only the Screen Recording button, with the add-by-hand-and-restart note, when only it is missing', async () => {
    h.sr = false;
    await render();
    expect(q('grant-screen-recording')).not.toBeNull();
    expect(q('grant-accessibility')).toBeNull();
    expect(q('grant-input-monitoring')).toBeNull();
    expect(q('screen-recording-missing')?.textContent).toMatch(/click \+ to add it.*restart Kai/);
  });
```

Why it is wrong, not my implementation:

1. **It contradicts T's own new pin on the same string.** `permissionCopy.test.ts:76` requires `en.settings.permScreenRecordingMissing` `.not.toContain('+ to add')`; `shortcutsTabGrantHidden.test.ts` mounts the real `ShortcutsTab` and requires the rendered row (the same i18n value, `ShortcutsTab.svelte:508`) `.toMatch(/click \+ to add it.*restart Kai/)`. No production value on earth satisfies both — the suite is arithmetically red until this pin is updated.
2. **It contradicts the binding brief.** AC3 / settled decision 3 fix the en value verbatim as `'Turn on Kai in Screen & System Audio Recording, then restart Kai.'` — that exact string is what the row now (correctly) renders, per the vitest received-text in the failure output.
3. **It was missed by the collision check, not by the brief.** handoff-A's "No pin collisions" evidence covers the *sibling* file `shortcutsTabGrantButtons.test.ts` ("pins button labels and one-binding-per-click, not the missing copy") — `shortcutsTabGrantHidden.test.ts` is a different file, and its :82 does pin the missing copy. It stayed green through T-red only because production still had the old wording then.

Suggested repair (T's choice, T's file): point :82 at the new copy — e.g. `expect(q('screen-recording-missing')?.textContent).toMatch(/Screen & System Audio Recording, then restart Kai/)` — and, for consistency, the stale rationale around it: the `it` title at :76 ("with the add-by-hand-and-restart note") and the file header :4-7 ("Screen Recording tells the user how to add Kai by hand … because macOS does not list the app on its own for every build"), which now describe the removed workaround teaching.

Everything else in that file (Grant-button visibility behavior, rows, mocks) is unaffected by this change and stays green as-is.

## Ready for T(green)

Implemented against the RED contract: both Go pins green, all 3 AC3 pins green, envelope pins untouched and green, AC5 line in, Swift compiles. The suite's single remaining failure is the stale test-owned pin above — repairing it is T's, after which the plugin's T-green run should read exit 0. If instead T rules my reading wrong, the contract (brief decision 3 + T's own `toBe` pins) and that stale regex cannot both hold, so the ruling must reconcile them explicitly.

---

## Addendum — duplicate dispatch, independent verification (the second F instance, 2026-10-21)

**Operator attention: this phase was dispatched twice and both instances ran concurrently.** A
first F session (opencode `ses_ea1b2c1a8ffe7t2F2Bq41SpLib`, created 05:51:23, "Phase F: screen
recording grant (#114)") and the wrapper dispatch at 05:56:31 (`opencode run --agent
feature-implementor` → `f-out.json`, PID 57864/57870 — the second instance, this addendum's
author). Everything above (implementation, self-check, stale-pin finding, CHANGELOG bash
disclosure) is the **first** instance's work, kept verbatim; the second instance made **no
production edits** and overwrote nothing.

What the second instance observed and did (evidence: session transcript `f-out.json`, `ps`,
mtimes, this worktree):

- 05:56:47 its first read of `apple_accessibility.swift` still showed the 0.6.0 metadata-only
  shape; the worktree then changed under it — swift 05:57:08 (+ compile-fix touch 05:58:13),
  i18n 05:57:13, CHANGELOG 05:57:37 — while `git status` at T-red time had shown test files
  only. It froze (no edits, no suite run), identified the writer by forensics (the first F's
  suite runner PID 88339 at 06:02 executing the release-test tail into `T/opencode/rel*.log`;
  a third `feature-implementor` PID 32954 at 06:09 belongs to run-C's `0091` worktree, not
  this one), and waited for quiescence (first F idle since 06:10:49) before verifying.

**Independent re-verification by the second instance (worktree quiet, 06:12–06:24):**

- Diff re-reviewed against R1–R3/A1–A2: `SCStream` over `SCContentFilter(display:excludingWindows:)`
  built from `SCShareableContent`'s `displays.first` (R2 — metadata kept for the filter only);
  `startCapture` with `openPane()` (main-thread dispatch, once-guard) reachable from **both**
  outcomes (R3 — plain completion plus the filter-unbuildable `guard … else`); `stopCapture`
  success-path only (A finding 4); the completion closure holds `stream` strongly until it
  answers; ask → preflight → attempt → pane-last order and the whole envelope verbatim.
- i18n byte-equality re-proven with node (`===` true; en 65 / zh 30 chars);
  `CHANGELOG.md:9` byte-identical to the operator-supplied wording; released sections untouched.
- Verbatim suite re-run (its own invocation): build `>> done: pkg/swiftbridge/libkai_bridge.dylib`
  and `.a` for `arm64-apple-macosx27.0`; Go **20/20 packages `ok`** incl.
  `ok cnb.cool/dtapp/kai/pkg/swiftbridge 0.333s` (both T Swift pins + envelope pins green);
  vitest `Test Files 1 failed | 78 passed (79)` / `Tests 1 failed | 872 passed (873)` — the one
  failure being the stale pin above; `bash scripts/release.test.sh` → exit 0 `PASS`;
  `bash scripts/release-ask-ai.test.sh` → exit 0 `PASS` (standalone, as the `&&` chain stops at
  vitest). Chained exit **1**, for exactly the reason the stale-pin section states.
- The stale-pin finding is **independently confirmed**: re-read
  `shortcutsTabGrantHidden.test.ts:82` → `.toMatch(/click \+ to add it.*restart Kai/)` against
  the received settled copy `"Turn on Kai in Screen & System Audio Recording, then restart
  Kai."`. Test-owned; neither F instance touched it; T repairs it in T-green.

**Follow-up for O (with T-red's `paths.test` item):** the double dispatch itself — two live F
agents shared one worktree for ~14 minutes and could have raced the suite and these handoff
files. Whatever driver created the 05:51 session (its CLI wrapper is not in `ps`; only the
session record remains) should be found before the next role dispatch in this environment.
