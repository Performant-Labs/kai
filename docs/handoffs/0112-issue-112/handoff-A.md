# Handoff-A: Phase 3 — Up-front plan review, issues #112 + #113 (run 0112-issue-112)

**Date:** 2026-10-21
**Branch:** issue-0112-issue-112
**Brief reviewed:** `docs/handoffs/0112-issue-112/brief.md` · **Reuse map:** `survey.md` §"Reuse & Analogous-Feature map" · **Wireframe:** N/A (uiSurface false, D/U declared)
**Verdict:** **BLOCK** — one plan-level defect (AC7 × AC8 contradiction on the mainline runtime path). Everything else verified clean against source; three binding rulings below.

## Summary

Both root causes match the source exactly — I re-verified every survey anchor with my own excerpts and all line numbers are exact (`main.go:664/:689`, `persisted.ts:14`, `window_wrapper.go:123/:148`, `TranslateWindow.svelte:858/:911`, `GeneralTab.svelte:128`, `main_windowsize.go:33`, settings `service.go:57-65/:322-332`, Wails beta.24 close chain + `RegisterKeyBinding:1768`). The reuse map is extend-everywhere and correct; the one "new but tiny" row (Cmd+W binding) is justified — kai registers no keybindings today. One BLOCK: AC7 and AC8, taken together, contradict on the **only path a runtime setting change can occur on** (Settings window open → toggle → close Settings → re-pin fires with the store's mount-time value, reverting AC7's application). Fixing it by writing the setting into the store would re-create the #113 pollution defect, so the plan needs an explicit **effective-pin rule** plus two added test cases before T writes anything.

## Findings

### 1. BLOCK — AC7 × AC8 contradict on the standard path of a runtime setting change

The setting's only toggle lives in Settings > General, so every runtime change has the Settings window open, and closing Settings always fires the re-pin:

`frontend/src/components/TranslateWindow.svelte:857-858` — the re-pin applies the raw store:
```svelte
      if (name === WindowSettings || name === WindowHistory) {
        Window.SetAlwaysOnTop($pinnedStore).catch((e) =>
```

`frontend/src/components/TranslateWindow.svelte:909-911` — the store's value is the mount-time resolution (or an explicit choice):
```svelte
        if (!pinTouched) {
          pinnedStore.set(initialPin(storedPinRaw, cfg?.translate_window_on_top === true));
```

`frontend/src/components/settings/GeneralTab.svelte:128-132` — the toggle (saves; broadcast is the new AC7 part):
```ts
  async function toggleTranslateWindowOnTop(e: Event) {
    ...
      await SaveConfig({ ...cfg, translate_window_on_top: enabled });
```

Contradiction walkthrough: window open, no 🧷 click, setting off at mount → store `false` → user opens Settings, toggles ON → AC7 broadcast applies on-top ✓ → user closes Settings → `:858` re-pin applies `$pinnedStore = false` → off-top again. #113's "also check whether an already-open window picks up a changed preference" fails on its mainline path. And the obvious implementation fix — updating the store from the broadcast — is forbidden by AC6 (writes only on explicit toggle) and would re-create the pollution root cause (a stored `true` masking a later setting=off is exactly survey "Root cause").

**Required amendment (AC7 + AC8):**
- State one rule: **applied level = explicit 🧷 choice this mount (`pinTouched`) ? that choice : the current setting** — evaluated live at *every* application point: mount init, the AC7 setting-changed broadcast, and the Settings/History-close re-pin. The re-pin resolves through this decision; it must not apply the raw store.
- Extend AC7's Vitest matrix with the interaction: (a) no explicit choice + setting toggled while open + Settings closed → level follows the new setting (the re-pin does not revert it); (b) explicit 🧷 choice + setting toggled + Settings closed → the explicit choice wins (#69's locked rule).
- AC8's test asserts the re-pin resolves through the same decision with the **versioned** key present.
- Suggested home for the pure decision: extend `frontend/src/utils/pinDefault.ts` (already in scope, already the pure-decision + Vitest seam) — Svelte wiring stays thin, as AC7 already says.

### 2. WARN — binding ruling: the Cmd+W callback must invoke the same close rule as the hook, not a bare `Hide()`

Today a handled Cmd+W routes through the native close chain, so the frontend close cleanup fires:

Wails beta.24 `pkg/application/webview_window_darwin.m:478-494`:
```objc
- (BOOL)windowShouldClose:(NSWindow *)sender {
    ...
    processWindowEvent(delegate.windowId, EventWindowShouldClose);
    return false;
```
…and kai's hook (`main.go:689-703`) then broadcasts `EventWindowClosing(WindowTranslate)`, which `TranslateWindow.svelte:866-877` uses for `autoRun.cancel()` / `commitAutoNow()` (issues #57/#81/#109). A binding callback of bare `window.Hide()` (the `HideOnEscape` shape, `webview_window.go:377-380`) would silently drop the broadcast on the new path — a behavior regression on the very shortcut #112 names.

**Ruling (refines AC2/AC3):** the named `window_wrapper.go` rule is **broadcast + Hide**; the hook composes `event.Cancel()` + rule; the Cmd+W binding calls the rule directly. T locks the rule once (broadcast+hide counts) and the hook composition; both entry points delegate. Tray-toggle `Hide` stays broadcast-free (existing, deliberate).

### 3. WARN — binding ruling: the versioned pin key must be translate-specific; `pinKey()` itself must not change

`frontend/src/stores/persisted.ts:26-28`:
```ts
export function pinKey(windowName: string): string {
  return `kai:${windowName}:pinned`;
}
```
`frontend/src/components/ScreenshotWindow.svelte:61`:
```ts
  const pinnedStore = persisted<boolean>(pinKey('screenshot'), false);
```
Changing `pinKey()`'s format would also drop the screenshot window's stored pin — an out-of-scope behavior change the brief's own guard forbids ("the screenshot window's pin semantics … out of scope"). **Ruling:** the version bump is a translate-pin-only key (e.g. `kai:translate:pinned:v2`, constant in the pin store or `pinDefault.ts`); `pinKey()` and its screenshot caller stay untouched.

### 4. WARN — the mount-init `pinnedStore.set(...)` must not persist under the new store (test gap)

The mount init (`TranslateWindow.svelte:911`) writes the store with the *resolved* value. Under AC6's "written only on an explicit 🧷 toggle", that call must not reach localStorage — otherwise the pollution defect re-enters on first mount of a fresh profile (auto-written `false` later masking a setting=on; survey's "fresh installs' `false`"). **Ruling:** AC6's Vitest suite adds the mount-path case — a mount with nothing stored leaves localStorage empty afterward; only an explicit 🧷 writes. Falls out naturally once Finding 1's rule lands (the store becomes purely the explicit-choice record).

### 5. NIT — AC5's home

`main_windowsize.go` is named for size; `translateWindowStartsPinned` is a level param. Keeping it there is fine (the file's established role is "settings-derived creation param with its own test file"; `main_windowsize.go:33` + `main_windowsize_test.go` is the cited shape, and the scope guard lists it). Do **not** create a `main_windowlevel.go` sibling without amending the scope guard. A one-line comment widening the file's description to "creation params (size, level)" is enough. Non-binding.

### 6. NIT — survey anchor precision

`updater_window.go:144` is `pkg/wails-updater-providers/updater_window.go:144` (`AlwaysOnTop: true`). Cosmetic; disclosure only, no action.

## Verified clean (no findings)

- **#112 anchors, all exact:** hook `main.go:689-703` (Cancel → Emit → Hide); `CloseButtonState: ButtonEnabled` at `main.go:664` (and Settings `:546`, History `:607`, Screenshot `:727` — only translate's changes); tray toggle `main.go:1003-1005`; `GetByName` → nil (`window_wrapper.go:34-40`); Wails destroy listener (`webview_window.go:366-371`) with hooks-first, cancel-skips-listeners dispatch (`HandleWindowEvent`, `webview_window.go:991-1008`); `Mac.WindowShouldClose → Common.WindowClosing` (`pkg/events/defaults.go:43`). Bonus: hidden windows don't emit close events at all (`windowIsHidden` early-return in `windowShouldClose:`) — Hide is destruction-proof by construction.
- **Key-binding seam is real:** `RegisterKeyBinding` exists in beta.24 (`webview_window.go:1768`); `"Cmd+W"` parses (`"cmd": CmdOrCtrlKey`, `keys.go:57`; maps to Ctrl+W on Windows — harmless). Firing stays a manual item per W-1. kai registers no keybindings today, so "new but tiny" is accurate.
- **#113 anchors, all exact:** no `AlwaysOnTop` in the translate window's options (`main.go:646-665`); auto-write (`persisted.ts:14-26`); `initialPin` stored-wins (`pinDefault.ts:9-17`); setting default-off + tolerant read (`internal/settings/service.go:57-65`, `:322-332`); GeneralTab saves without broadcasting (`:128-139`; no `emitEvent` anywhere in GeneralTab).
- **AC7's direction has a direct in-repo precedent:** `EventEnginesChanged` — settings emits (`EnginesTab.svelte:259`), translate listens. The reuse map's "EventAutoClipboardChanged shape" citation is sound even though that event's direction is translate→settings.
- **Reuse map:** extend-not-new on every row; no parallel paths proposed. AC2 extends the `ToggleTranslateWindow`/`LowerForSettings` package-level-rule cadence (fake-window pattern exists in `window_toggle_test.go`); AC5 extends the `translateWindowSize` shape.
- **Scope and issue fidelity:** touched-file list matches the anchors; #112's open question settled as hidden with the issue's own "hide is not a close" sanction; Cmd+Q untouched; CHANGELOG `[Unreleased]` exists for AC9; screenshot-window and updater disclosures are accurate.

## Notes for O

Amend AC7/AC8 per Finding 1 (the effective-pin rule + the two added Vitest interaction cases + AC8 asserting the re-pin resolves through the decision), and fold the one-line forms of Findings 2-4 into AC3/AC6 (binding callback = same rule; translate-specific versioned key, `pinKey()` untouched; mount-path no-write test). Finding 5: keep AC5 in `main_windowsize.go`. Re-review will read only the delta.

## Patterns referenced

1. `internal/service/window_wrapper.go` — `ToggleTranslateWindow:123`, `LowerForSettings:148` (package-level rule cadence AC2 extends)
2. `main_windowsize.go:33` + `main_windowsize_test.go` (settings-derived creation param + test shape AC5 extends)
3. `frontend/src/utils/events.ts:50-59` + `EnginesTab.svelte:259` emit + TranslateWindow `onEvent` (settings→translate broadcast shape AC7 extends)
4. `frontend/src/utils/pinDefault.ts` (pure-decision + Vitest home for the effective-pin rule)
5. Wails v3.0.0-beta.24 — `webview_window.go:366-371`, `:991-1008`, `:1768`; `webview_window_darwin.m:478-494`; `pkg/events/defaults.go:43`

## Round 2 — re-review of the amended brief (2026-10-21)

**Inputs:** amended `brief.md` (AC2/AC3/AC5/AC6/AC7/AC8), decisions.md "Phase A round 1" entry, round-1 findings above. Delta-only: per-finding resolution + new-contradiction sweep. Source unchanged (no code exists yet); round-1 verbatim excerpts above remain the evidence baseline.

**Verdict: PASS** — all five actionable findings resolved; one new `warn` (R2-1) pins the interpretation of the effective-pin rule so T cannot implement it in the one way that would collide with AC6.

### Finding resolution

| Finding | Round-1 severity | Where amended | Resolved | Evidence |
|---|---|---|---|---|
| 1 — AC7×AC8 contradiction | block | AC7 `brief.md:68-77`; AC8 `:78-80` | ✅ PASS | The effective-pin rule is stated verbatim (`:69-73`: applied level = `pinTouched` ? that choice : the current setting, evaluated live at mount init, the setting-changed broadcast, **and** the Settings/History-close re-pin; "it must not apply the raw store"); both interaction cases (a)/(b) are in the Vitest matrix (`:74-77`); AC8's test exercises the re-pin through the decision **with the versioned key present** (`:78-80`, W-2 folded); pure decision extends `pinDefault.ts` (`:73-74`). The contradiction's mechanism (re-pin applying the raw store) is explicitly dead. |
| 2 — Cmd+W = same rule, not bare Hide | warn | AC2 `:36-42`; AC3 `:43-47` | ✅ PASS | Rule = broadcast `kevents.EventWindowClosing(model.WindowTranslate)` + Hide, in `window_wrapper.go`, no new file; hook composes `event.Cancel()` + rule; binding calls the rule directly; tray `Hide` stays broadcast-free; tested claim = the rule + hook composition, binding firing manual (W-1). Matches the ruling 1:1. |
| 3 — translate-only versioned key | warn | AC6 `:60-61` | ✅ PASS | `kai:translate:pinned:v2`, `pinKey()` untouched so the screenshot window's stored pin is unaffected. Constant's homes (pin store / `pinDefault.ts`) are both inside the scope guard (`:96-97`). |
| 4 — mount-init no-write | warn | AC6 `:62-64` | ✅ PASS | "the mount-init resolution must not reach localStorage — A Finding 4's test: a mount with nothing stored leaves localStorage empty"; writes only on explicit 🧷 set. |
| 5 — AC5's home | nit | AC5 `:53-58` | ✅ PASS | Kept in `main_windowsize.go`, description comment widened to "creation params (size, level)"; no sibling file; scope guard unchanged. |
| 6 — survey anchor | nit | `survey.md:73` | ✅ fixed | Now reads `pkg/wails-updater-providers/updater_window.go:144`. |

### New-contradiction sweep

One `warn`; everything else composes cleanly:

- **AC2 × AC4:** absorbing Emit+Hide into the rule keeps the hook's today-sequence Cancel → Emit → Hide (round-1 excerpts, `main.go:689-703` + `HandleWindowEvent` dispatch), and hide-not-destroy keeps every bring-back path green.
- **AC6 × AC7:** broadcast/re-pin *application* never writes the store — the pollution defect stays dead; only explicit 🧷 writes.
- **Scope:** every amended element (`pinDefault.ts`+test, `events.ts`, `persisted.ts` or pin sibling, `GeneralTab.svelte`, `main_windowsize.go`+test, `window_wrapper.go`, `main.go`) is already inside the scope guard; **no new file anywhere** (AC2 says so explicitly).

### R2-1 — WARN: the effective-pin rule must **compose** `initialPin`, not replace it

AC7's formula reads "explicit 🧷 choice **this mount** (`pinTouched`) ? that choice : the current setting". Implemented literally as the complete decision, it collides with AC6's own clause "`initialPin` semantics stay (a genuine stored choice still wins over the setting)" (`brief.md:64-65`) — and with the documented contract of the very function AC7 extends (`pinDefault.ts:4-6`):

```ts
//   nothing stored -> the setting is the default (the setting's own default is off);
//   a stored choice -> the stored choice, always (an explicit choice made any time wins);
```

("always", "any time" — this is #69's locked rule, the same rule AC7's case (b) cites.) Literal reading, stored-choice user (v2 holds a genuine `true` from a previous session, setting off): mount applies `true` (AC6/`initialPin`), then the first Settings open+close fires the re-pin → `pinTouched` false → literal rule applies the setting → `false`. The stored explicit choice is reverted by merely closing Settings — Finding 1's defect shape, mirror image, and case (b) forbids it.

**Binding interpretation for T/F (the only completion consistent with AC6 + AC7 + case (b)):** the pure decision is

```ts
effectivePin(storedV2, pinTouched, thisMountChoice, liveSetting) =
  pinTouched ? thisMountChoice : initialPin(storedV2, liveSetting)
```

one function at **every** application point. Mount init then coincides with `initialPin` exactly (AC6's clause holds by construction); the unspecified matrix cell — *stored* v2 choice, no this-mount touch, setting toggled (or Settings merely closed) → **the stored choice keeps winning; the re-pin must not revert it**. Case (a) is unchanged (nothing stored, no touch → live setting everywhere). Suggest O fold this one line into AC7 before t-red; not blocking — the brief's own clauses admit exactly this reading and no other.

**Overall Round 2 verdict: PASS** (0 block). Run proceeds to t-red with R2-1's interpretation recorded.
