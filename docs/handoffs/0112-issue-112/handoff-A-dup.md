# Handoff-A-dup: Phase 7 — anti-duplication gate, issues #112 + #113 (run 0112-issue-112)

**Date:** 2026-10-21
**Branch:** issue-0112-issue-112 (worktree `.worktrees/0112-issue-112`)
**Diff base:** ab5ea852af5bff27bb4fb09c9718204ab7f4d0bd   **Diff head:** 5d8b14907be5cfc600c224d818b126ba3efd1915
**Reuse map:** `docs/handoffs/0112-issue-112/survey.md` §"Reuse & Analogous-Feature map" (rows re-checked against `brief.md` AC1–AC9 and handoff-A round-1/round-2 rulings)
**Verdict:** **PASS**

## Summary

F extended every seam the reuse map named — zero parallel paths. The close policy is a
package-level rule in `window_wrapper.go` beside `ToggleTranslateWindow`/`LowerForSettings`
(the file's own cadence, fake-window test fixture reused, not re-created); the level creation
param is a `translateWindowSize` sibling in the same file with the same nil-guard shape; the
frontend additions compose the existing utilities (`effectivePin` **calls** `initialPin`, the
broadcast rides the existing `emitEvent`/`onEvent` plumbing in the `EventEnginesChanged`
shape) rather than re-implementing them. The one new object, `stores/translatePin.ts`, is the
explicit-write-only sibling the reuse map and the brief's scope guard sanctioned in writing —
a deliberate, reviewed decision, not drift. Blast radius is exactly as guarded:
`persisted.ts` untouched (`pinKey()` byte-identical), Screenshot/Settings/History windows
untouched. One nit (stale doc comment, already T-green advisory 3). No block findings.

## Reuse-map conformity (row by row)

| Map row | Named seam | What the diff does | Verdict |
|---|---|---|---|
| Testable close rule | `ToggleTranslateWindow` / `LowerForSettings` / `showHistory` package-level funcs over small window interfaces | `HideTranslateWindow(win hideWindow, emit)` at `window_wrapper.go:183`, placed directly after `LowerForSettings` (:149), documented in the file's own idiom ("A package-level func rather than a WindowWrapper method so Wails generates no frontend binding for it", :181-182); nil-guard (:184-186) mirrors `LowerForSettings`. **No parallel rule path**: `grep EventWindowClosing internal/service/*_test.go` shows no other emitter fake existed to duplicate; `window_close_test.go` **reuses** `fakeToggleWindow` from `window_toggle_test.go:15` and adds only a 3-line `fakeEmitter`. | ✅ extended |
| No close affordance | `CloseButtonState` per window in `main.go` | Translate → `application.ButtonHidden` (`main.go:672`); Settings `:546`, History `:607`, Screenshot `:750` all still `application.ButtonEnabled` (verified by grep — translate-only, exactly AC1). | ✅ extended |
| Cmd+W deterministic | Wails `KeyBindings` / `RegisterKeyBinding` ("new but tiny" — justified: no other `RegisterKeyBinding` site exists in the repo) | One binding, `main.go:723-725`, whose callback calls the AC2 rule directly (`service.HideTranslateWindow(window, emitForCloseRule)`) — better than the map's bare-Hide sketch, per A Finding 2 which the amended brief folded in. No parallel binding infrastructure. | ✅ as ruled |
| Creation-time level from the setting | `translateWindowSize(settingsService, …)` shape in `main_windowsize.go` | `translateWindowStartsPinned(settingsSvc *settings.Service) bool` (`main_windowsize.go:53-68`), same file, same nil-guard shape (`nil svc → false`, `nil cfg → false`, else the field — mirroring `translateWindowSize`'s `:40-48`), fed at `main.go:655` `AlwaysOnTop: translateWindowStartsPinned(settingsService)`. No `main_windowlevel.go` sibling (A Finding 5 honored — file comment widened to "creation params … (size, level)"). Tests extend `main_windowsize_test.go` including the established collapsed-whitespace source-assertion idiom of `TestResizableWindowsUseTheScaledWidths:112-131`. | ✅ extended |
| Pin store that cannot be polluted | keep `persisted`; pin store becomes explicit-write only + versioned key | `stores/translatePin.ts` (NEW): `TRANSLATE_PIN_KEY = 'kai:translate:pinned:v2'` (:9), reads v2 only, `apply()` never writes, `set`/`update` the only persist path. **Sanctioned new object in writing** — reuse map row ("keep `persisted` for other stores; the pin store becomes explicit-write only") + brief scope guard (`brief.md:102` "or a pin-specific sibling") + A Finding 3/4 rulings. `persisted.ts` diff-empty; `pinKey()` unchanged (`persisted.ts:26-28`, byte-identical to master). Per the verdict rules a brief-justified new object is a PASS, not drift. | ✅ as sanctioned |
| Settings → translate broadcast | `EventAutoClipboardChanged`/`EventEnginesChanged` emit-after-save shape | `EventTranslateWindowOnTopChanged = 'kai:translate-window-on-top:changed'` (`events.ts:103`) in the established `kai:<thing>:changed` cadence, unique (grep of all 21 constants — no collision); `GeneralTab.svelte` emits via the **existing** `emitEvent` runtime util after `SaveConfig` (the `EnginesTab.svelte:259` precedent, not a new emit path); the listener uses the existing `onEvent` + off-in-cleanup shape and re-fetches `GetConfig` (payload not trusted). | ✅ extended |

## Drift check vs the amended plan

- **effectivePin at all three application points — yes.** The one decision
  (`pinDefault.ts:36-38`, R2-1 verbatim: `pinTouched ? thisMountChoice : initialPin(storedV2,
  liveSetting)` — composes `initialPin`, does not re-implement it) is routed through at:
  mount init `TranslateWindow.svelte:958` (`await reapplyPin()`); the broadcast `:947`
  (`onEvent(EventTranslateWindowOnTopChanged, …)` → `reapplyPin()`); the re-pin `:898`
  (`void resolvePinLevel().then(…)`). `resolvePinLevel` (:51-63) is the single resolution
  path; grep shows exactly one `.apply(` (:54) and one pin `.set(` (:36, `togglePin`) in the
  component.
- **The re-pin does not apply the raw store.** `:898` resolves first
  (`resolvePinLevel()` → `effectivePin` → `apply`), then applies `$pinnedStore`, whose value
  at that moment is the fresh decision. The literal `SetAlwaysOnTop($pinnedStore)` line is
  retained because the pre-existing `translateWindowGear.test.ts` pins it (F's disclosed
  nuance; T-green ruled it "not a deviation"). I concur: the composition satisfies "must not
  apply the raw store" — the store is re-resolved immediately before the apply, and the
  mounted AC7(a)/AC7(b) tests pin that the re-pin follows the decision, not the mount-time
  value.
- **v2 key translate-only; `pinKey()` untouched.** `translatePin.ts` never reads/writes
  `kai:translate:pinned` (grep: the old key appears only in comments and tests); the
  screenshot window's `persisted(pinKey('screenshot'))` is untouched (ScreenshotWindow.svelte
  has no diff).
- **Cmd+W calls the rule; the hook composes Cancel + rule.** `main.go:704-714`:
  `event.Cancel()` then `service.HideTranslateWindow(translateWindow, emitForCloseRule)` —
  the inline `Emit` is gone (the wiring test pins its absence); the binding `:723-725` calls
  the same rule. `emitForCloseRule` (:703) is F's disclosed compile-forced adapter
  (`app.Event.Emit` returns `bool`; the rule's tested shape returns nothing), defined once,
  passed at both sites — faithful forwarding, no behavioral difference.
- **Close button hidden for translate only** — see the row above; `main.go` grep is
  conclusive.

## Duplication sweep

- **Interfaces:** `hideWindow` (`window_wrapper.go:159-161`, `Hide() application.Window`)
  overlaps `windowToggler` (:106-109) on `Hide`, but the file's established cadence is one
  minimal per-rule slice — `levelWindow` (:134-137) similarly overlaps `windowToggler` on
  `IsVisible`. Reusing `windowToggler` would saddle the rule with an `IsVisible` constraint
  it does not use. Consistent with the pattern; not duplication.
- **Settings read helpers:** `translateWindowStartsPinned` consumes the service's public
  surface (`settingsSvc.Get().TranslateWindowOnTop`); it does not re-parse settings.json or
  shadow `readTranslateWindowOnTop` (`internal/settings/service.go:329` remains the single
  tolerant read).
- **Frontend store/event/decision utilities:** no re-implementations found — `effectivePin`
  composes `initialPin`; `createTranslatePinStore` shares only the try/catch localStorage
  idiom with `persisted` (deliberately different write semantics, per the sanctioned row);
  the event rides the existing `emitEvent` (`runtime/index.ts:11`) / `onEvent` plumbing.
  `SetAlwaysOnTop` callers in the frontend: ScreenshotWindow (pre-existing, untouched,
  different window class) + the three translate application points — no parallel level path.
- **Test fixtures/files:** `window_close_test.go` reuses `fakeToggleWindow`; the per-rule
  test-file name matches `window_toggle_test.go`/`window_history_test.go` cadence;
  `effectivePin.test.ts` matches the utils function-level cadence (`historyShortcut.test.ts`
  et al.), and its header documents the RED-mechanics reason for not folding into
  `pinDefault.test.ts`.

## Blast radius

- `frontend/src/stores/persisted.ts` — **no diff** (grep-verified `pinKey` byte-identical).
- `frontend/src/components/ScreenshotWindow.svelte` — **no diff**; its pin store untouched.
- Settings/History/Screenshot window blocks in `main.go` — untouched (`CloseButtonState:
  ButtonEnabled` at :546/:607/:750; their own hooks at :548/:612/:768 unchanged; the diff
  hunks touch only the translate block).
- Touched files = exactly the brief's scope guard list (translatePin.ts via the "pin-specific
  sibling" clause; CHANGELOG.md per AC9); nothing outside it.

## Findings

| # | Severity | File:line | Finding | Suggested fix |
|---|---|---|---|---|
| 1 | nit | `internal/service/window_wrapper.go:177-178` | Stale doc comment: "emit has wails' app.Event.Emit signature …, so main.go passes `app.Event.Emit` itself as the value" — false since the `emitForCloseRule` adapter (`main.go:703`); the plain-func shape is kept, the direct pass is not. Already recorded as T-green advisory 3; zero behavior impact. | One-line comment fix (F or S, with the next touch of the file): "main.go adapts `app.Event.Emit` (dropping its bool return) into this shape — see `emitForCloseRule`." |

No block findings. No architectural drift introduced during rework. The B-1 remediation
(commit f5f75e0) added a test only — no production movement, nothing to re-flag.

## Notes for F

None — PASS. (Finding 1 is optional hygiene, foldable into any later touch of the file.)

## Patterns referenced

1. `internal/service/window_wrapper.go:104-154` — `windowToggler`/`ToggleTranslateWindow` and
   `levelWindow`/`LowerForSettings`: the package-level rule + minimal-interface cadence the
   close rule extends.
2. `main_windowsize.go:40-48` + `main_windowsize_test.go:112-131` — the settings-derived
   creation-param shape (nil guards, collapsed-whitespace source assertions) the level param
   and its wiring tests extend.
3. `frontend/src/components/settings/EnginesTab.svelte:259` + `frontend/src/runtime/index.ts:11`
   + `frontend/src/utils/events.ts` — the emit-after-save settings→translate broadcast shape
   the new event extends.
4. `frontend/src/utils/pinDefault.ts` — the pure-decision home; `effectivePin` composes
   `initialPin` (R2-1).
5. `frontend/src/stores/persisted.ts` — the auto-write store deliberately left untouched for
   all other consumers (A Finding 3's blast-radius guard).
