# handoff-F: #81 idle pane and retained session (Phase 6, implement)

- **Date:** 2026-09-26
- **Branch:** issue-81-implementation (worktree `.worktrees/0081-idle-and-retain`, base master fe629dc, HEAD 2060a8c at start)
- **Inputs read:** docs/handoffs/81-brief.md, handoff-A.md (PASS, 6 warn), handoff-T-red.md, the four T-authored files, decisions.md, AGENTS.md / CLAUDE.md
- **Result:** implemented against T's RED. Frontend suite 176 passed, 1 failed; the one failure is a T test that cannot pass without a refactor of unrelated pre-existing code (see "Tests that look wrong"). Everything else, including Go, is green. `tsc --noEmit` exit 0, dev `vite build` ok.
- **Return:** `done: true`, `archChanged: true` (new leaf module, widened exported signatures in `resultPane.ts`, a new persisted-storage contract, and a lifecycle change on window close).

## What was done (production files)

| File | Change |
|---|---|
| `frontend/src/utils/resultPane.ts` | Extended. `DotState` gains `'idle'`; `statusDot` / `statusDots` take a required 4th `requested` argument (`done` > `pending` > `failed` if requested, else `idle`); new `paneState({ hasEngines, engine, results, loading, requested })` returning `'no-engine' \| 'loading' \| 'result' \| 'idle' \| 'failed'`; exported types `PaneState`, `PaneStateArgs`. Module header updated. |
| `frontend/src/utils/translateSession.ts` | New, bindings-free leaf. `TranslateSession`, `emptySession()` (fresh object per call), `restoreSession(raw: unknown)` (validates, never throws). Only import is `import type { PaneResult }` from `./resultPane.ts`. |
| `frontend/src/components/TranslateWindow.svelte` | `sessionStore = persisted<TranslateSession>('kai:translate:session', emptySession())`; `input` / `results` / `requestedTo` / `requested` seeded from `restoreSession(get(sessionStore))`; one `$effect` is the only writer; `requested = true` in `doTranslate()`; `clearInput()` resets `requested` and `requestedTo` as well; the result-pane chain reads one `pane = $derived(paneState(...))`; the translate branch of the `EventWindowClosing` handler no longer clears anything (guard and Settings pin branch kept); dots take `requested`, idle dots have no fill and an engine-name-only label. |
| `frontend/src/utils/events.ts` | Comment only: the `EventWindowClosing` doc said the translate window clears results. |
| `internal/events/events.go` | Comment only, same statement on the Go side. |
| `main.go` | Comment only, the translate window's close hook said the frontend clears the input and results. |

No i18n key, CSS, backend logic, bindings or test file changed.

## Design decisions

A's six warn defaults were applied, because decisions.md holds no O ruling on them.

1. **Result-only `'result'`, `edited` is not an input** (A1). `paneState` returns `'result'` iff `results[engine]?.result` is non-empty. The template branch is `pane === 'result' && activeResult`: the second half is redundant at runtime and only narrows the type for the card (which dereferences `activeResult.phonetic`).
2. **Two different tests on `results`, kept as they were** (A2). Order: no-engine > loading (`loading && !results[engine]`, entry presence) > result (non-empty `.result`) > failed (`requested`) > idle. An error payload that arrives while loading ends the wait and falls through to failed.
3. **`requested` also goes through `statusDots`, `'idle'` is a `DotState`, no new copy** (A3). An idle dot has no fill class; its `title` and `aria-label` are the engine name alone. The active engine's ring is unchanged, so an idle window shows a ring on the active dot and nothing on the others. Both attribute expressions were edited in place (the duplicated ternary was not consolidated).
4. **`results` typed `Record<string, PaneResult>` through a type-only import** (A4). A suggested "a single `as Record<string, TranslateResult>`" at the seed. That does not compile (see "Deviations"), so the seed is `as unknown as Record<string, TranslateResult>` once, with a comment. The write direction needs no cast (`$state.snapshot(results)` is assignable to `TranslateSession['results']`).
5. **One write path** (A5). The `$effect` is the only writer and `clearInput()` resets the four fields, so Clear empties the store through the same path. `requestedTo = ''` is reset too (T pinned only `requested`, `input`, `results`, `edited`), so the stored value after Clear is exactly `emptySession()`. The store is read once at init; `$sessionStore` is not used anywhere.
6. **Closing does nothing for the translate window** (A6). No `loading = false` either: results still in flight when the window is hidden keep landing and end loading, and the existing 15 s fallback covers a request that never answers. Verified in a scratch mount (below): with `loading = false` on close, a quick reopen shows the red failed pane until the result arrives.

Smaller choices:
- `restoreSession` builds `results` with `Object.fromEntries(...filter...)` (own properties), so a stored key such as `__proto__` stays plain data instead of hitting the prototype setter.
- `restoreSession` does not clone the surviving entries. The component only ever replaces `results` wholesale, never mutates an entry.
- The idle state has no `{:else}` branch in the chain. A comment above the chain says so.
- `paneState` takes the `hasEngines` boolean, so the pure module never sees `activeEngines`.

## Reuse / extend-vs-new (the brief's Reuse map)

| Map entry | What F did |
|---|---|
| `persisted()` store | Reused as is for `kai:translate:session`; no raw localStorage in the new code. |
| `resultPane.ts` helpers (`statusDot`, `anyPending`) | `statusDot` / `statusDots` extended in place, `paneState` added beside them; `anyPending` untouched (it is already gated on `loading`). |
| Pure-helper pattern of `swapLangs.ts` / `flippedTarget.ts` | `translateSession.ts` follows it (header, no bindings import, plain values). |
| `loadDefaults` / `persistLangs` for languages | Untouched. |

New object: only `translateSession.ts`, which the brief's AC3 mandates. There is no second write path to the session key and no second derivation of "no result and not loading": the chain's old inline conditions are gone and `paneState` is their single home.

## Architecture notes for A

- Dependency direction unchanged: `TranslateWindow.svelte` -> `utils/translateSession.ts` -> (type only) `utils/resultPane.ts`. The utils layer imports neither stores nor bindings.
- `resultPane.ts` public surface changed: `statusDot` / `statusDots` gained a required parameter (only `TranslateWindow.svelte` and tests call them, checked by grep), `DotState` widened, `paneState` / `PaneState` / `PaneStateArgs` added.
- New persisted contract: localStorage key `kai:translate:session`, JSON `TranslateSession`, no version field. Anything unrecognised falls back to the empty session, and the first `$effect` run rewrites the normalised value.
- Lifecycle contract changed: the translate window's `EventWindowClosing` branch is now a no-op. Go still broadcasts the event (`main.go:487`); the frontend still uses it for the Settings pin restore (#69) and ScreenshotWindow filters by its own name.
- Data at rest: the source text and each engine's result (including the `text` and `dict` fields of the payload) now also sit in the webview's localStorage. The history DB already stores every translation, so this is the same data class; Clear removes it.

## Deviations from spec / wireframe

1. **Comment-only edits outside the brief's Files list** (`main.go`, `internal/events/events.go`, `frontend/src/utils/events.ts`). The brief lists the backend as out of scope; these do not change behavior, but each stated the old clear-on-close behavior as fact. Verbatim, on master fe629dc:
   ```
   main.go:483-485      // Red X = hide the window (don't quit): first broadcast kai:window:closing so the
                        // frontend clears the translate input/results, then Hide. The window is not destroyed
                        // (Svelte components stay mounted), so the next invocation starts from a clean state.
   internal/events/events.go:10-14   ... each window clears its own state as needed (e.g. the translate
                        // window clears input and results). ...
   frontend/src/utils/events.ts:6-8  // cleans up its own state as needed (e.g. the translate window clears results), then closes.
   ```
   Left as they were, they would tell the next reader that clearing on close is the intended behavior and invite putting it back. Each edit is one isolated hunk, so S can revert any of them without touching the feature. `gofmt -l` clean, `go vet ./internal/events/` ok, `internal/events` has no test files. `docs/design/issue-0009-active-engine-pane.md:127` also mentions the old handler; it is a historical design record and was left alone.
2. **`TranslateSession.results` is `Record<string, PaneResult>`, not the brief's literal `Record<string, unknown>`.** That is A's finding 4 default. Note for the seam: `restored.results as Record<string, TranslateResult>` is rejected by the compiler (`TS2352 ... neither type sufficiently overlaps`, `PaneResult` lacks `from`, `to`, `text`, `dict`, `from_ocr`), so the seam needs `as unknown as`. With the brief's `unknown` a single plain `as` would have compiled. If S prefers the brief's type, changing the alias in `translateSession.ts` and dropping the `unknown` hop is a two-line edit; T's tests type-check either way.
3. `'idle'` is a `DotState` and `statusDots` takes `requested`: A's finding 3 and T's pinned signatures, not in the brief's text.

Nothing else differs. There is no wireframe (UI surface: no).

## Tier 1 self-check

Environment: `NODE_OPTIONS=--no-experimental-webstorage`, Node v26.7.0, from `frontend/` unless noted.

RED reproduced before any edit (four T files):
```
 Test Files  4 failed (4)
      Tests  22 failed | 32 passed (54)
```

The four T files after implementation:
```
 ✓ src/utils/translateSession.test.ts (18 tests)
 ✓ src/utils/resultPane.test.ts (39 tests)
 ✓ src/stores/translateSessionStore.test.ts (4 tests)
 ❯ src/components/sessionRetention.test.ts (15 tests | 1 failed)
   × session retention wiring > does not touch localStorage directly
```
Full frontend suite: `Test Files  1 failed | 19 passed (20)`, `Tests  1 failed | 176 passed (177)`. The failure is the one above and only that.

Authoritative command (`sh -c "(cd pkg/swiftbridge/scripts && bash ./build.sh) && go test -vet=off ./internal/... ./pkg/... -count=1 && pnpm --dir frontend test"`): Swift bridge builds; Go `ok` for configstore, engine, historystore, httplogstore, i18n, langpref, model, service, settings, translate, pkg/wails-updater-providers (11 packages, no FAIL); frontend as above; exit 1 solely because of the flagged test. `git status` after the run shows no `pnpm-lock.yaml` churn (`pmOnFail: ignore` in pnpm-workspace.yaml).

Other gates:
- `tsc --noEmit`: exit 0.
- Dev `vite build --minify false --mode development` (scratch outDir): exit 0, 198 modules (197 on master plus `translateSession.ts`). The Svelte warnings are the same 5 a11y warnings master emits (only their line numbers moved).
- `prettier --check`: `resultPane.ts`, `translateSession.ts`, `events.ts` clean. `TranslateWindow.svelte` has exactly the one pre-existing violation master has (`Window.SetAlwaysOnTop($pinnedStore).catch(...)`, line 352 here, 299 on master) and nothing new.
- `gofmt -l main.go internal/events/events.go`: empty. `go vet ./internal/events/`: ok. `golangci-lint` is not installed here; the Go edits are comments.

### Supplementary evidence (not a substitute for T's own checks)

The committed tests read the component source, so they cannot see reactive behavior. As in #13 and #43, F ran throwaway checks in the scratchpad (nothing staged, nothing committed):

- **Real component mounted in jsdom** (`@sveltejs/vite-plugin-svelte`, `resolve.conditions: ['browser']`, `server.deps.inline: [/svelte/]`), real `persisted` store, real jsdom localStorage, real i18n; the **Wails runtime and the bindings are mocked**. 19 checks, all pass on this branch, 14 of 19 fail on master (the 5 that pass are guards). Covered: a fresh window is blank with no failed text and no failed dots; idle dots are titled with the engine name only; a request whose active engine gets no result shows failed; Clear returns to idle and the stored value equals `{input:'',results:{},requestedTo:'',requested:false}`; close keeps text and result and leaves storage untouched; restart restores text, result, dots and the flipped-target notice and never restores loading; Clear then restart is idle; a stored request with no result restores as idle; corrupt JSON, a wrong-shaped value and an entry without `engine` all restore as an idle window; another window's closing and Settings' closing behave as before; closing mid-flight keeps the loading placeholder and the late result still lands; `EventInputFill` replaces the retained text and translates it; only `input`, `requested`, `requestedTo`, `results` are stored; `loadDefaults` still sets the selects and close does not reset them.
- **7 in-memory mutants** of the component, each killed by at least one of those checks: clear-on-close restored, idle renders failed, dots ignore `requested`, Clear keeps `requested`, no write-back, request never marked, close clears `loading`.
- **6 on-disk mutants against the committed tests** (backup, mutate, run, restore; the restore was verified byte-identical, the component's sha256 was `7ea7c53a217372f88b47833ddcca12d754ffdff5766d35200554dde1ae57d471` before the first mutant and after the last; a comment edit came later), each killed by the intended committed test: clear-on-close -> "no longer clears input / results / edited on close"; idle renders failed -> "translate.failed is rendered only in the failed branch"; dots ignore requested -> "passes requested to the status dots"; Clear keeps requested -> "clearInput resets requested, ..."; request never marked -> "doTranslate sets requested = true"; persist `loading` -> "never persists loading or edited". (The brief's AC7 mutation check, clear-on-close and idle-shows-failed, is therefore observed, not only reasoned.)
- **Scratch type check of the extracted `<script>` block** (`tsc` on master's script vs this branch's): 10 errors on master, 11 here. All are artifacts of extraction (`$store` syntax, use-before-declare of `edited`) or the pre-existing index-signature mismatch; the one extra is the same mismatch at the new `paneState` call (see Known issues 1).

## Tests that look wrong (for T)

1. **`frontend/src/components/sessionRetention.test.ts` > "does not touch localStorage directly"**, first assertion `expect(src).not.toMatch(/localStorage\s*\./)`. It scans the whole component and matches the pre-existing #39 pin migration, which is unrelated to the session and identical on master (commit 10f9f9e). It failed on master for this reason, not for the missing store, and no correct production change can turn it green. Verbatim, tree `TranslateWindow.svelte:23-27` (22-26 on master):
   ```
   if (typeof localStorage !== 'undefined' && !localStorage.getItem(PIN_MODAL_MIGRATION_KEY)) {
     if (localStorage.getItem(pinKey('translate')) === JSON.stringify(false)) {
       pinnedStore.set(true);
     }
     localStorage.setItem(PIN_MODAL_MIGRATION_KEY, '1');
   }
   ```
   F did not move that block (a drive-by refactor of #39 code) and did not edit the test. T's handoff pinned "no `localStorage.` calls" for the component; the intent (AC4: no raw localStorage for the session) is met. Suggested replacement, verified in a scratch: it passes on this branch, fails on master, and fails when the session key is also written or read through raw `localStorage` (two mutants):
   ```ts
   it('does not touch localStorage directly for the session', () => {
     expect(src).toContain('persisted<TranslateSession>');
     // the key appears once, as the persisted store's key
     expect(src.match(/kai:translate:session/g)?.length).toBe(1);
     // the only raw localStorage calls are the pre-existing #39 pin migration
     for (const m of src.matchAll(/localStorage\s*\.\s*\w+\(([^)]*)\)/g)) {
       expect(m[1]).not.toMatch(/session/i);
     }
   });
   ```
2. Informational, not a defect: `resultPane.test.ts`, `translateSessionStore.test.ts` and `sessionRetention.test.ts` are not Prettier-clean (`format:check` is not part of CI or `make check`; only `tsc` and `build:dev` are).
3. Gap, not wrong: the source-contract suite pins the failed branch and the `paneState` import, but nothing stops another chain branch from bypassing `pane` (for example `{#if activeEngines.length === 0}` back in place). The scratch mount above covers that behavior; a committed check needs the component-render harness (#78).

## Known issues

1. **Type-hole class, pre-existing, one more instance.** `Record<string, TranslateResult>` (the bindings interface) does not satisfy `Record<string, PaneResult>` (an index signature `[key: string]: unknown`): `TS2345 ... Index signature for type 'string' is missing in type 'TranslateResult'`. The component already passes `results` to `statusDots`, `anyPending` and `resetEdits` that way, and now to `paneState`. Nothing gates it: CI and `make check` run `tsc` (which does not read `.svelte`) and `build:dev`, and `svelte-check` is commented out in `lint-frontend`. Root fix, if wanted (not done, shared code): `[key: string]: any` on `PaneResult`, or an explicit cast at the call sites.
2. Idle dots are unfilled (A's default): on an idle window the active engine shows only its accent ring and the others show nothing. Not seen on screen (headless mandate), so the principal's hand test should look at it. One-class change if a hollow outline is preferred.
3. Restore after a quit with a request in flight: `loading` is not restored, so an engine that had not answered yet reads as failed after the restart. Re-translating fixes it.
4. `EventInputFill` arriving before the engine list has loaded (for example a hotkey that launches the app): `doTranslate()` returns early (pre-existing guard `!input.trim() || activeEngines.length === 0`), so the new text sits beside the retained previous result once engines load, where master showed the failed pane (results were always empty at that point). Narrow window; not changed.
5. The session is written on every change of the text (each keystroke, no debounce), the same synchronous pattern as the divider drag. Only matters for very large pasted texts.

Not exercised (headless mandate, plus the mocks above): real WKWebView localStorage surviving a quit and relaunch (the brief's decision already rests on it, as do the pin and last-used engine), the Go `RegisterHook` -> `Emit` -> frontend hop on the red X, and the look of the idle dots. The brief assigns those to the principal's hand test of the built app.

## Files changed (every PRODUCTION file)

- `frontend/src/components/TranslateWindow.svelte`
- `frontend/src/utils/resultPane.ts`
- `frontend/src/utils/translateSession.ts` (new)
- `frontend/src/utils/events.ts` (comment only)
- `internal/events/events.go` (comment only)
- `main.go` (comment only)

Staged by explicit path; no test file staged, nothing committed.
