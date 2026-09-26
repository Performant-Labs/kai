# handoff-S: #81 idle pane and retained session (Phase 9, spec audit)

- **Date:** 2026-09-26
- **Branch:** issue-81-implementation (worktree `.worktrees/0081-idle-and-retain`), diff base master fe629dc, head 1a50f04 (A-dup)
- **Brief:** docs/handoffs/81-brief.md. Rigor: in-session. UI surface: no (visual preconditions skipped).
- **Inputs read:** brief, handoff-A.md, handoff-T-red.md, handoff-F.md, handoff-T-green.md, handoff-A-dup.md, 81/decisions.md, full production and test diff.
- **Verdict:** PASS

## A precondition
handoff-A.md: PASS (0 block, 6 warn). handoff-A-dup.md: PASS (0 block, 3 warn). Met.

## T precondition
handoff-T-green.md: "Blocking issues: none". 177/177 frontend, Go ok, `tsc --noEmit` exit 0. Met.

## Brief sanity check
The brief is coherent. Two small internal tensions were settled upstream, and S accepts both. (1) AC1's "and no edit" in the failed rule contradicts the template, which cannot render a card without a result. A finding 1 correctly drops it. (2) AC3's `Record<string, unknown>` would have pushed an unchecked cast to the seam. A finding 4 uses the structural `PaneResult` instead. Neither is a defect in the source of truth that needs an ADVISORY-HOLD.

## Spec compliance

| AC | Status | Evidence |
|---|---|---|
| 1 Idle pane | Met | `paneState` added to `resultPane.ts` (extended, no new file). Order: no-engine > loading (entry presence) > result (non-empty) > failed (requested) > idle. The chain reads `pane`. `idle` has no branch, so the pane renders blank. `failed` still renders `translate.failed`. `requested = true` in `doTranslate`, `false` in `clearInput`. The "no edit" clause was dropped per A1 (see above). |
| 2 Dots | Met | `statusDot` and `statusDots` both take a required `requested` argument (no default). `DotState` gains `'idle'`. Idle dots have no fill, and title/aria are the engine name only, so no new copy. |
| 3 Session module | Met | `translateSession.ts` is pure with a type-only import. `emptySession` returns a fresh object each call. `restoreSession` handles non-object/null/array/wrong type as empty, keeps only entries that are objects with a string `engine`, and forces `requested` false when no entry survives. `results` is typed `PaneResult` (a stricter deviation, accepted). `Object.fromEntries` also neutralises a `__proto__` key. |
| 4 Persistence wiring | Met | `persisted<TranslateSession>('kai:translate:session', emptySession())`. Values are seeded once through `restoreSession(get(...))`. A single `$effect` writes back `input/results/requestedTo/requested`. `loading` and `edited` are never stored. |
| 5 Close retains | Met | The translate branch of the closing handler is a no-op. The WindowSettings pin branch and the `name !== WindowTranslate` guard are kept. `loading` is also left alone on close (A6; brief says "may"). The session is reset only by Clear (through the effect), EventInputFill (-> doTranslate) and a new translate. |
| 6 Languages | Met | `loadDefaults` and `persistLangs` are unchanged. Tests pin that both restore/save from `default_from`/`default_to`, that `loadDefaults` runs on mount, and that the close handler never assigns the languages. |
| 7 Tests | Met | These are the four files named in AC7. The jsdom store test sits at `stores/translateSessionStore.test.ts`, the brief's "stores/-style" location. RED was confirmed for the right reasons (missing export/module, and assertion failures on the source contract). F observed the mutation check on disk: restoring clear-on-close fails, and idle rendering failed fails. |
| 8 Suite and tsc | Met | T-green reports the authoritative suite green with `NODE_OPTIONS=--no-experimental-webstorage` and `tsc` exit 0. |

## Quality audit

| Dimension | Result | Notes |
|---|---|---|
| API consistency | Pass | `paneState` takes one object argument. `statusDot`/`statusDots` gain a required parameter. The only non-test caller is TranslateWindow. Naming follows the module. |
| Error handling | Pass | `restoreSession` never throws and falls back whole rather than half-applying. `persisted` guards corrupt JSON and failed writes. |
| UI/UX match | Pass | The idle pane is blank and idle dots are unfilled with no new copy. How the idle dots look has not been checked on screen (headless). That is left to the principal's hand test of the built app, as the brief assigns. |
| Accessibility | Pass | `aria-label` mirrors `title`. An idle dot reads as the engine name alone, which is accurate: there is no state to announce. |
| Architecture gate | Pass | Not overruled. A (0 block) and A-dup (0 block) are honoured. There is one write path, and utils import no stores or bindings. |
| Code organization | Pass | The template's inline chain conditions are gone, and `paneState` is now the single home of that logic. The small tail-rule overlap between `statusDot` and `paneState` is deliberate (A-dup warn 1). |
| Security | Pass | Source text and results now also sit in webview localStorage. That is the same data class the history DB already holds. Clear removes them, and restore validates untrusted input. |
| Performance | Pass (advisory) | The whole session is stored synchronously on every keystroke, with no debounce. This matches the divider store's pattern. It only matters for very large pastes. |
| Naming consistency | Pass | The key `kai:translate:session` follows the `kai:<window>:<name>` convention. `requested` and `requestedTo` are named consistently. |
| Test quality | Pass | Each test names one behaviour. The paneState cases cover every state, including an error payload, a result for another engine, and idle with a stale empty result. The restoreSession cases are table-driven and non-redundant. The source-contract tests are the accepted substitute for the missing render harness (#78). T's repair of the mis-scoped localStorage test was correct. Minor: the store test "wrong shape" partly duplicates a restoreSession unit case. It still adds the path through `persisted`, so keep it. There are no assertion-free, tautological or snapshot tests. |

## Scope check
Nothing was under-delivered. Scope was exceeded only by comment-only edits in `main.go`, `internal/events/events.go` and `frontend/src/utils/events.ts`. Each one corrected a now-false statement that the translate window clears on close. They change no behaviour, and S accepts them. The PR body should mention them so that the "backend out of scope" line is not read as violated (A-dup warn 3b). S verified the new claim in those comments, that the screenshot window clears its image and results on close, at `ScreenshotWindow.svelte:262-266`. ScreenshotWindow itself is untouched, and there are no i18n, CSS or bindings changes.

## Verdict
**PASS.** No required changes.

## Advisory notes (non-blocking; for O and the PR body)
1. The `main.go` comment says "only its Clear button empties them". A new translate or EventInputFill also replaces the session, so this is slightly narrower than the truth. It is cosmetic and can be fixed in a follow-up.
2. Pre-existing guard, F Known issue 4: an `EventInputFill` that arrives before the engine list has loaded sets the new text but skips `doTranslate`, so the retained previous result sits beside the new text. The window is narrow, and the brief does not require changing that guard. Worth a follow-up issue if the principal sees it.
3. Restart with a request in flight (F Known issue 3): an engine that had not answered restores as failed. This is correct given that `loading` is never persisted. Re-translating fixes it.
4. The `PaneResult`/`TranslateResult` type hole is pre-existing and not gated because `svelte-check` is off. The seam cast `as unknown as` adds one more instance to that class.
5. Follow-up candidate from A-dup warn 2: derive one dot label for both `title` and `aria-label`.
6. Hand test for the principal: the look of the idle dots, and real WKWebView localStorage surviving a quit and relaunch.
