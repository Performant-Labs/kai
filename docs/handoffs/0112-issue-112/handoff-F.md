# Handoff-F: Phase 5 — Implement

**Date:** 2026-10-21
**Branch / worktree:** issue-0112-issue-112 (`.worktrees/0112-issue-112`)
**Contract:** handoff-T-red.md (tests + the behaviors they pin) + brief.md AC1–AC10 + handoff-A Findings 1–5 / R2-1

## What I changed

Per file — the behavior implemented, the test it satisfies, the rationale.

### Go

**`internal/service/window_wrapper.go`** — the close policy rule (#112 AC2/AC3):
- `type hideWindow interface { Hide() application.Window }` + package-level
  `HideTranslateWindow(win hideWindow, emit func(name string, data ...any))`: nil → no-op;
  exactly one `emit(kevents.EventWindowClosing, model.WindowTranslate)`; exactly one `Hide()`.
  Satisfies `TestHideTranslateWindowBroadcastsWindowClosingForTheTranslateWindow` /
  `…HidesWithoutDestroying` / `…WithoutAWindowIsANoOp`. Placed right after `LowerForSettings`
  (the package-level-rule cadence), documented in the file's own style. Added the `kevents`
  import.

**`main_windowsize.go`** — the level creation param (#113 AC5):
- `translateWindowStartsPinned(settingsSvc *settings.Service) bool`: nil service → false; nil
  config → false; else `cfg.TranslateWindowOnTop`. Satisfies the three
  `TestTranslateWindowStartsPinned_*` tests. Added the file-level description comment widening
  the file's role to "creation params (size, level)" per A Finding 5.

**`main.go`** — the wiring (#112 AC1/AC2/AC3, #113 AC5), pinned by the four source-assertion
tests:
- translate options: `CloseButtonState: application.ButtonHidden` (was `ButtonEnabled`);
  Settings/History/Screenshot blocks untouched. → `TestTranslateWindowCloseButtonIsHidden`.
- translate options: `AlwaysOnTop: translateWindowStartsPinned(settingsService)`.
  → `TestTranslateWindowCreationFeedsStartsPinnedIntoAlwaysOnTop`.
- the WindowClosing hook = `event.Cancel()` + `service.HideTranslateWindow(translateWindow,
  emitForCloseRule)` — the inline Emit+Hide is gone (absorbed by the rule); the #81/#53
  comments that stayed true are kept, the stale red-X ones rewritten honestly (there is no red
  X anymore). → `TestTranslateWindowClosingHookComposesCancelThenTheCloseRule`.
- `_ = …RegisterKeyBinding("Cmd+W", func(window application.Window) {
  service.HideTranslateWindow(window, emitForCloseRule) })` right after the hook.
  → `TestTranslateWindowCmdWBindingCallsTheCloseRule`.

Two wiring shapes deviate from the handoff's prose — both forced by the compiler and pinned
tests, neither visible to any assertion difference (details under Deviations): a tiny
`emitForCloseRule` adapter local instead of passing `app.Event.Emit` directly, and a
comma-ok narrowing assertion to `*application.WebviewWindow` for `RegisterKeyBinding`.

### Frontend

**`frontend/src/utils/pinDefault.ts`** — added `effectivePin(storedV2, pinTouched,
thisMountChoice, liveSetting)` = R2-1 verbatim (`pinTouched ? thisMountChoice :
initialPin(storedV2, liveSetting)`); `initialPin` untouched. → `effectivePin.test.ts` (4).

**`frontend/src/stores/translatePin.ts`** (NEW) — `TRANSLATE_PIN_KEY =
'kai:translate:pinned:v2'`, `TranslatePinStore = Writable<boolean> & { apply(v) }`,
`createTranslatePinStore()`: reads v2 only (JSON boolean, absent/corrupt → false, never
rewritten), `subscribe` writes nothing, `apply` updates subscribers only, `set`/`update`
persist under v2 only. → `translatePin.test.ts` (9).

**`frontend/src/utils/events.ts`** — added `EventTranslateWindowOnTopChanged =
'kai:translate-window-on-top:changed'` beside its `EventHistoryShortcutChanged` frontend-only
precedent. → `events.test.ts` (1).

**`frontend/src/components/TranslateWindow.svelte`** — the store swap + the ONE decision:
- `storedPinRaw` ← `TRANSLATE_PIN_KEY`; `pinnedStore = createTranslatePinStore()`;
  `togglePin` keeps `.set` (now the only write path).
- `resolvePinLevel()`: re-fetch `GetConfig`, then
  `pinnedStore.apply(effectivePin(storedPinRaw, pinTouched, get(pinnedStore),
  cfg?.translate_window_on_top === true))` — never a write.
- mount init: `await reapplyPin()` (= resolvePinLevel + apply `$pinnedStore` to the window) —
  the `if (!pinTouched)` guard is gone because `effectivePin` encodes it (a click during the
  config load resolves to that click).
- new `EventTranslateWindowOnTopChanged` listener → `reapplyPin()` (payload not trusted; config
  re-fetched); off-function registered in the onMount cleanup.
- the Settings/History-close re-pin: `resolvePinLevel().then(() =>
  Window.SetAlwaysOnTop($pinnedStore)…)` — resolves through the decision, NOT the raw store,
  and (nuance below) applies via the same literal line it always used.
- dead imports dropped (`pinKey`, `initialPin`); `persisted` stays (other stores).

**`frontend/src/components/settings/GeneralTab.svelte`** — `toggleTranslateWindowOnTop`
emits `emitEvent(EventTranslateWindowOnTopChanged)` after a successful `SaveConfig` (inside
the try, so a failed save broadcasts nothing). → `generalTabOnTop.test.ts` AC7 test.

**`CHANGELOG.md`** — two `[Unreleased]` lines: the not-closable window (#112); the on-top
setting governing creation + open windows, incl. the one-time pin-default reset (#113), in the
existing line style.

## Self-check

- `go test -vet=off ./internal/service/... -count=1` → `ok  cnb.cool/dtapp/kai/internal/service  1.785s`
- `go test -vet=off . -count=1` → `ok  cnb.cool/dtapp/kai  0.528s`
- `pnpm --dir frontend test` → `Test Files 1 failed | 81 passed (82)`, `Tests 1 failed | 888
  passed (889)` — the single failure is `src/components/historyUseRestore.test.ts` "the
  history window closing puts this window's pin back", a stale T-owned test (see below). Every
  contract suite is green: translateWindowPinDefault 10/10, translatePin 9/9, effectivePin
  4/4, events 1/1, generalTabOnTop 6/6, translateWindowGear 7/7 (pre-existing source-pin
  suite), and the pre-existing `TestSettingsWindowStartsHidden` stays green.
- `gofmt -l` clean on all touched Go files; prettier clean on all touched frontend files
  (TranslateWindow.svelte was prettier-formatted after my edit; HEAD was clean).
- The authoritative GREEN is the plugin's T-green run of the full `test.unit.command`, not
  this self-check. I did not run the full suite (per instruction); the three commands above
  cover every package/file this diff touches.

## Deviations from T's contract (2, both compile-forced, assertion-invisible)

1. **`emitForCloseRule` adapter instead of passing `app.Event.Emit` as the value.** The
   handoff's BINDING section says main.go passes `app.Event.Emit` directly — but wails'
   `Event.Emit` is `func(name string, data ...any) bool` and the rule's `emit` param (pinned by
   T's own `fakeEmitter.emit`, which returns nothing) is `func(name string, data ...any)`. Go
   has no implicit conversion between those; the direct pass cannot compile. main.go defines
   `emitForCloseRule := func(name string, data ...any) { app.Event.Emit(name, data...) }` once
   and passes it at both call sites. The wiring greps (`service.HideTranslateWindow(` in the
   hook and in the binding) pass identically; no forbidden literal is introduced.
2. **`RegisterKeyBinding` needs the concrete `*WebviewWindow`.** It is not on the
   `application.Window` interface that the package-level `translateWindow` variable holds, and
   the pre-existing `TestSettingsWindowStartsHidden` (T-owned) pins the literal
   `translateWindow = app.Window.NewWithOptions` — so capturing a second variable at creation
   broke that test's anchor. The binding site narrows instead:
   `if webviewWin, ok := translateWindow.(*application.WebviewWindow); ok { … }` (the
   assertion always succeeds — `NewWithOptions` returns a `*WebviewWindow`).

One deliberate wiring nuance (not a deviation): the Settings/History-close re-pin inlines
`resolvePinLevel().then(() => Window.SetAlwaysOnTop($pinnedStore)…)` rather than calling a
helper end-to-end, because the pre-existing `translateWindowGear.test.ts` pins the literal
`SetAlwaysOnTop($pinnedStore)` inside the `EventWindowClosing` handler source. The value
applied is the just-resolved decision (apply runs before the `.then`), so the pinned line and
the R2-1 rule both hold.

## Tests I think are wrong (1 — T repair needed)

**`frontend/src/components/historyUseRestore.test.ts:317-331`** — "the history window closing
puts this window's pin back" seeds the **retired old key**
(`localStorage.setItem('kai:translate:pinned', JSON.stringify(true))`) and expects the re-pin
to call `setAlwaysOnTop(true)`. Under the contract this run itself pins — T's re-pointed
`translateWindowPinDefault.test.ts` ("an old-key pin is pollution, not a choice"),
`translatePin.test.ts` (v2-only reads), and R2-1 (the re-pin resolves through
`effectivePin(storedV2, …)`, and with v2 absent + the setting off that is `false`) — the old
key must be ignored at every application point, so this assertion cannot hold. I did NOT edit
it (I can't); no implementation can satisfy both it and the contract tests, and special-casing
the retired key at the re-pin to save it would be exactly the "weaken the implementation to
cheat a test" my role forbids. The repair is T's one-liner, the same re-pointing T already
applied three times in `translateWindowPinDefault.test.ts`: seed
`'kai:translate:pinned:v2'` instead of the old key. Its sibling assertions in that test
(another window's closing does not touch the pin) are unaffected.

**Also for O (config gap, second occurrence):** `main_windowsize.go` and `CHANGELOG.md` are
inside the brief's scope guard but outside this session's generated edit allowlist
(`paths.production` has `main.go`, not root `main_*.go` siblings; no `CHANGELOG.md` entry). I
edited exactly those two files via `bash` (explicitly allowed), T's own disclosed precedent
for the root-test-file gap, with git-diff verification shown above. Same owner follow-up as
T's advisory: `paths.production`/edit allowlist deserve the missing rows.

## Disclosed for S (nuances a test cannot see)

- **Cmd+W maps to Ctrl+W on Windows** (Wails' `CmdOrCtrlKey` parsing) — the binding is
  registered with the literal `"Cmd+W"` per the contract; on Windows that is Ctrl+W, which is
  harmless (hide-only) but is a wording nuance for the PR description.
- **Manual-verification items stay manual** (W-1/W-3): the traffic-light/title-bar close
  button actually not rendering per platform, the binding actually firing on a real app, the
  level actually dropping behind other apps. The wiring tests are source assertions; real-app
  behavior needs the signed app (PR description's manual list).
- **The re-pin re-fetches `GetConfig` on every Settings/History close** (live setting, R2-1's
  "evaluated live at every application point"). That is one extra config read per close event
  — negligible (in-memory settings read via the wrapper), but it is a new call where the old
  code applied a cached store value.
- **`get(pinnedStore)` as `thisMountChoice`:** after any 🧷 click the store value IS the
  explicit choice (the only writer), so the resolution is exact; before any click the argument
  is ignored by `effectivePin`. If someone later adds another `.set` caller, that invariant
  breaks silently — `togglePin` must remain the only `.set` call site (the store's tests pin
  the write paths, not the Svelte call sites).
- **`emitForCloseRule` is registered before `translateReadyOnce` concerns apply** — it captures
  `app` (non-nil in this scope; the window was created from it), so the old `if app != nil`
  guard around the inline Emit is subsumed and removed.
- **Frontend `svelte-check` cannot run in this environment at all** (TypeScript 7 tooling
  mismatch inside svelte-check's own startup — pre-existing, unrelated to this diff, and not
  part of `test.unit.command`); vitest compiled every touched component, which is the type
  signal this run has.

## Ready for T(green)

Implemented against the RED contract. Expected T-green state: everything green **except**
`historyUseRestore.test.ts:317` (stale old-key seed — one-line T repair, see above); after
that repair the suite should be 889/889. The plugin should now run the suite: GREEN means the
loop advances; RED means I rework.
