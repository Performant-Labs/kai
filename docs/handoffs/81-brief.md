# Brief: #81 idle-pane-and-retain-session

Repo: Performant-Labs/kai-private (never upstream dtapps/kai). Issue: #81. Rigor: in-session. UI surface: no for the pipeline (no layout change, no new control; the idle pane is empty; live UI walkthrough infeasible per #28, the principal hand-tests the built app). Kind: bug fix + feature, one file family (TranslateWindow.svelte + two new pure utils).

## Problem
1. An idle translate window (nothing requested yet, or after Clear) renders "Translation failed" in the result pane: the last `{:else}` branch means "no result and not loading", which is also the idle state.
2. Closing the translate window (the red X) clears the source text and the results, so reopening shows an empty window. The principal wants the last source text, last result text and the language dropdowns retained across close/reopen and app restart.

## Evidence (verbatim, master fe629dc)
```
frontend/src/components/TranslateWindow.svelte:293-306  (inside onMount)
    const offClosing = onEvent(EventWindowClosing, (name: string) => {
      // Issue #69: opening Settings drops this window out of always-on-top ... 
      if (name === WindowSettings) {
        Window.SetAlwaysOnTop($pinnedStore).catch((e) => console.error(t('log.restorePinFailed'), e));
        return;
      }
      // Global broadcast: only handle this window's (translate) closing, so closing another
      // window doesn't mistakenly clear the translation.
      if (name !== WindowTranslate) return;
      results = {};
      input = '';
      loading = false;
      edited = new Map();
    });
```
```
frontend/src/components/TranslateWindow.svelte:826-892  the result-pane chain
  {#if activeEngines.length === 0} ...noActiveEngine
  {:else if loading && !results[activeEngine]} ...loading placeholder
  {:else if (result present) ...}   (the result card, `activeResult?.result` / edited)
  {:else}
    <!-- The active engine failed (absent from results with loading already cleared) or returned an empty result: failed state (design §5). ... -->
    <div class="flex h-full flex-col items-center justify-center gap-2 text-center">
      <span class="text-sm" style="color: var(--app-danger)">{t('translate.failed')}</span>
    </div>
  {/if}
```
```
frontend/src/components/TranslateWindow.svelte:114-131 state: let input = $state(''); ... let fromLang, toLang ($state, TRANSLATE_LANG.Auto / EN); let results = $state<Record<string, TranslateResult>>({}); let requestedTo = $state<string>(''); let loading = $state(false);
frontend/src/components/TranslateWindow.svelte:~502-508  function clearInput() { input = ''; results = {}; edited = new Map(); editingResult = false; }
frontend/src/components/TranslateWindow.svelte:doTranslate()  guards `!input.trim() || activeEngines.length === 0`; sets loading = true; requestedTo = toLang; results = {}; edited = new Map(); then TranslateMulti(...)
frontend/src/components/TranslateWindow.svelte:onMount  EventInputFill handler: input = text; doTranslate();   loadDefaults(): reads GetConfig().default_from / default_to into fromLang / toLang; persistLangs() saves them (called from the selects' onchange and from swap()).
frontend/src/stores/persisted.ts   export function persisted<T>(key, initial): Writable<T>  // localStorage-backed writable; corrupt values fall back to initial; write failures ignored. Used by pinnedStore and lastUsedStore in this component; tested in stores/persisted.test.ts (jsdom, real localStorage).
frontend/src/utils/resultPane.ts   pure helpers already used by the pane: statusDot/statusDots/anyPending/resetEdits (dot 'failed' = !loading and no result).
```
The window's red X emits `EventWindowClosing`; a tray hide is a bare `Hide()` and never clears (window_toggle_test.go, #69).

## Acceptance criteria
1. Idle pane. New pure helper `paneState(...)` in `frontend/src/utils/resultPane.ts` (extend, do not add a file) returning one of `'no-engine' | 'loading' | 'result' | 'idle' | 'failed'`. `'failed'` only when a request was made (`requested` true), not loading, and the active engine has no non-empty result and no edit; `'idle'` when nothing was requested. The template's chain uses it; `idle` renders an empty pane (no text, no danger colour); `failed` still renders `translate.failed`. `requested` becomes true in doTranslate() and false in clearInput().
2. The status dot for an idle window must not show failed either: `statusDot(...)` gets a `requested` input (default behaviour preserved when omitted is NOT allowed: pass it explicitly) so an idle window shows no failed dots.
3. Retention. New module `frontend/src/utils/translateSession.ts` (pure, no bindings import): type `TranslateSession = { input: string; results: Record<string, unknown>; requestedTo: string; requested: boolean }`, `emptySession()`, and `restoreSession(raw: unknown): TranslateSession` that validates a value read back from storage: anything malformed (not an object, wrong field types, null) yields `emptySession()`; `requested` is forced false when `results` is empty (a request that never produced anything reads as idle after a restore, not failed); `results` keeps only entries that are objects with a string `engine`.
4. TranslateWindow uses `persisted<TranslateSession>('kai:translate:session', emptySession())` (reuse the existing store; do not write raw localStorage). It seeds `input`, `results`, `requestedTo`, `requested` from `restoreSession($store)` at init and writes them back when they change (a `$effect`), so the source text, per-engine result texts and the #44 flipped notice survive close/reopen and an app restart. `loading` is never persisted (always false after a restore). Manual result edits (`edited`) are not persisted (design §3: edits are discarded).
5. The translate window's `EventWindowClosing` handler no longer clears `input` / `results` / `edited` (keep the WindowSettings pin branch and the `name !== WindowTranslate` guard; it may still set `loading = false`). Clearing the retained values happens only through the existing Clear button (`clearInput` also resets the stored session), through a new `EventInputFill` (which replaces the text and translates), or through a new translate.
6. Languages. The dropdowns already persist through `persistLangs()` (selects' onchange, swap()). Pin, with a test, that `loadDefaults()` restores `fromLang`/`toLang` from `default_from`/`default_to` on mount (it does today) and that no code path resets them on close. No change to the persistence mechanism.
7. Tests (Tester authors, RED first): `resultPane.test.ts` extended for `paneState` and the `requested` dot rule (idle, loading, result, failed-after-request, failed vs idle when an edit exists, no-engine); new `translateSession.test.ts` (empty/garbage/wrong-type inputs -> empty; a valid saved session round-trips through JSON; requested forced false with empty results; a results entry without a string engine is dropped); a `stores/`-style jsdom test that `persisted('kai:translate:session', ...)` reads a saved session back through `restoreSession` and falls back on corrupt JSON; a source-contract test `frontend/src/components/sessionRetention.test.ts` in the style of `swapWindow.test.ts` (closing handler no longer assigns `input`/`results`; clearInput resets the session; the chain uses `paneState`; `EventInputFill` still calls doTranslate). All fail on current master for the right reason. Mutation-check: restoring the clear-on-close block, or making idle render `translate.failed`, must fail a test.
8. Full authoritative suite green with `NODE_OPTIONS=--no-experimental-webstorage`; `tsc --noEmit` clean.

## Files
Production: `frontend/src/components/TranslateWindow.svelte`, `frontend/src/utils/resultPane.ts` (extend), `frontend/src/utils/translateSession.ts` (new). Tests: `frontend/src/utils/resultPane.test.ts` (extend), `frontend/src/utils/translateSession.test.ts` (new), `frontend/src/components/sessionRetention.test.ts` (new).
Reuse map (extend, do not duplicate): `persisted()` store; `resultPane.ts` helpers (`statusDot`, `anyPending`); pure-helper pattern of `swapLangs.ts` / `flippedTarget.ts`; existing `loadDefaults`/`persistLangs` for languages.

## Decisions already made (MO)
- Storage is the frontend `persisted` localStorage store (same as pin and last-used engine): local only, per app data dir, no backend change and no settings-file change.
- What is retained: source text, per-engine results, requestedTo, requested flag, and (already) languages. Not retained: manual edits, loading, engine selection is already retained by lastUsedStore.
- Clear-on-close is reversed on purpose (principal, 2026-09-26). Other windows' closing broadcasts still never touch this window.
- ScreenshotWindow is untouched.
- Idle pane is blank (no new copy, no i18n keys).

## Out of scope
ScreenshotWindow; backend; new copy/i18n; retaining manual edits; Apple identity error (#80); component-render harness (#78).

## Test plan
RED first as in criterion 7. `translateSession.ts` and the extended `paneState` do not exist yet, so a failing import/undefined-export is an accepted RED for the new-module and new-export tests provided the assertions are exactly the listed behaviours; the source-contract test fails on assertions today (close handler still clears; failed branch unconditional). GREEN after F.

## Risks
Stale stored results after an engine is disabled (results keyed by an engine no longer enabled): the pane only reads `results[activeEngine]`, so an orphan entry is inert. A restored session whose `requestedTo` no longer matches the languages: harmless (the flipped notice compares result.to to requestedTo, not to the select). Storage quota/large results: writes are try/catch guarded by `persisted`. Two windows sharing localStorage: only the translate window writes this key.
