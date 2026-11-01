# Handoff-T-green: Phase 6 - #127 context-send button looks like plain text

**Date:** 2026-11-01
**Branch:** issue-0127-issue-127
**Issue:** #127
**Handoff-F reviewed:** docs/handoffs/0127-issue-127/handoff-F.md
**Handoff-T-red:** docs/handoffs/0127-issue-127/handoff-T-red.md

## GREEN confirmation

F's diff is exactly the intended one-line fix and nothing else: `git diff` shows a single
production change — `ContextChatPanel.svelte:146`, `class="u-btn u-no-drag px-3 py-1 text-xs"` →
`class="u-btn u-btn--primary u-no-drag px-3 py-1 text-xs"` (static attribute; `data-testid`,
`title`, `disabled` expr, `onclick` untouched). No Go files, no CSS files changed.

```
$ pnpm --dir ./frontend test src/components/contextChatWiring.test.ts
 Test Files  1 passed (1)
      Tests  16 passed (16)          # was 14 passed | 2 failed at RED

$ pnpm --dir ./frontend test          # full suite
 Test Files  82 passed (82)
      Tests  895 passed (895)         # RED baseline: 893 passed | 2 failed
```

**Pin check (behavior, not implementation):** temporarily removed `u-btn--primary` from the
component (backup copy, restored byte-exact afterwards — `cmp` + `git diff` verified):

```
 Tests  2 failed | 14 passed (16)     # both fail at expect(classes()).toContain('u-btn--primary')
```

That is the exact RED signature: the suite passes only while the visible-button behavior exists.
RED-phase simulations already pinned the variant choice (`u-btn--ghost` → RED) and
unconditionality (`class:u-btn--primary={…}` directive → RED).

## Tier 1 results

| Check | Command | Expected | Actual | PASS/FAIL |
|---|---|---|---|---|
| Frontend tests (target) | `pnpm --dir ./frontend test src/components/contextChatWiring.test.ts` | 16/16 | 16/16 | PASS |
| Frontend tests (full) | `pnpm --dir ./frontend test` | 895/895, 82 files | 895/895, 82 files | PASS |
| Type check (CI's frontend lint, `make lint-frontend`) | `pnpm --dir ./frontend run tsc` | exit 0 | exit 0, no errors | PASS |
| Frontend build (CI's other frontend check) | `pnpm --dir ./frontend run build:dev` | builds | ✓ built in 474ms | PASS |
| Format check, changed files | `pnpm --dir ./frontend exec prettier --check` on both changed files + app.css | all pass | all pass (after fixing my test file's line-wrapping — T's file, not F's) | PASS |
| Go compile sanity | `go build ./...` | builds | GO-BUILD-OK (pre-existing linker warnings only; first attempt failed only on missing `frontend/dist` embed target — clean worktree, resolved after `build:dev`; artifact removed again) | PASS |
| Go tests | — | unchanged | 0 Go files in the diff (`git diff --stat HEAD`: 2 frontend files + 2 docs), so the Go suite is byte-identical to HEAD's; left to CI | N/A |
| Server starts / API smoke | — | n/a | Wails desktop app, no server/ and no e2e/ workspace in this repo — the Postgres/testcontainers and Playwright harness notes do not apply | N/A |

The `HistoryList.svelte` a11y warning in the vitest output is pre-existing in a file this run
never touched (matches F's note).

## Tier 2 results

- **Test coverage per criterion** — PASS. All four "Wanted" bullets in the brief are backed (see
  status table below).
- **Test quality (§7)** — PASS. Both tests name a behavior, fail in isolation for the right reason
  (pin check above), sit at the cheapest sufficient tier (source+CSS pin; no Svelte render harness
  exists, #78 — the repo's established pattern for this component), and do not duplicate: test 1 =
  variant + CSS paint; test 2 = both-states/unconditionality + identity hooks + `{#if open}` gate;
  keyboard behaviour stays with the pre-existing Enter/Cmd+Enter test in the same file. Suite is
  proportionate: 2 unit pins for a 1-line class change, exactly what the brief asked for. No
  redundant tests to prune; the 14 pre-existing tests in the file are untouched.
- **Type safety** — PASS. `tsc --noEmit` clean over the frontend; no `any` casts or type
  suppressions in the test file (regex + optional chaining only); changed files are `.svelte` +
  `.test.ts`, both covered.
- **Error handling** — N/A. A CSS-class change adds no error paths; nothing to test.
- **Data integrity** — N/A. No database operations in the change.
- **API contract** — N/A. No request/response shapes touched.
- **Security** — N/A/PASS. No input handling, no auth surface; the diff adds one static class name.
- **Migration safety** — N/A. No schema (frontend-only repo area; no migrations in this repo).
- **Playwright** — N/A (no `e2e/` workspace in this repo). **Coverage note for U:** these are
  source pins, not rendered pixels — jsdom does not lay out, and no render harness exists (#78),
  so the *on-screen* look (painted accent background; dimmed-but-still-a-button disabled state in
  the real webview, in both default and Spanish UI locales) is precisely what U must walk live.

F's cross-checked claims, verified directly: `.u-btn--primary { background: var(--app-accent); … }`
(app.css:255) paints; `.u-btn:disabled { opacity: 0.5; cursor: not-allowed; }` (app.css:250) only
dims; the static `u-btn u-btn--primary …` spelling matches every other primary button in the
codebase (ShortcutsTab ×4, EnginesTab ×2, TranslateWindow:1966). No discrepancy with handoff-F.

## Acceptance criteria status

| Brief criterion | Status | Backed by |
|---|---|---|
| Looks like a button in both states: visible background/border via an existing variant (likely `u-btn--primary`) | PASS | `uses the primary variant, painted by app.css with a background…` (static class contains `u-btn--primary` **and** the CSS rule keeps a non-`transparent`/`none` background) + `keeps the look in both states…` (no `class:` directive that could strip the paint in any state) |
| Disabled state (empty draft / request in flight) still clearly a button, just dimmed | PASS | `keeps the look in both states…`: base `u-btn` kept so `.u-btn:disabled` applies; that rule dims via `opacity` and never blanks `background`/`border`; `disabled={busy \|\| draft.trim() === ''}` unchanged |
| Keep `data-testid="context-send"`, title/tooltip, click and Cmd+Enter behaviour | PASS | test 2 pins testid, `title={t('translate.contextSend')}`, `onclick={send}`; Enter-not-Shift/Cmd+Enter pinned by the pre-existing test "sends on Enter (not Shift+Enter, not Cmd+Enter which translates)"; `git diff` shows those attributes byte-untouched |
| Add a test that pins the variant class (or a visual check) so it does not regress | PASS | the two new tests + the Phase-6 pin check (removing the class → 2 failed) prove the regression trips them |
| Out of scope: label text / send behaviour | PASS | diff touches only the class attribute |

## Blocking issues

None.

## Advisory notes

- The pin check temporarily edited `ContextChatPanel.svelte` and restored it byte-exact
  (`cmp`-verified; final `git diff` = F's one line only). The build-generated `frontend/dist`
  (Go embed target) was created by `build:dev` and removed again — worktree left clean of artifacts.
- My Phase-4 test file needed a Prettier line-wrap fix (long lines). Fixed and re-staged by path;
  re-ran the file (16/16) after. CI itself runs `tsc` + `build:dev`, not `format:check`, so this
  was cosmetic compliance, not a gate.
- This is a UI-surface change and the tests are source pins, not rendered output — **U should
  verify on screen**: enabled state shows the accent-filled button, disabled state (empty context
  draft / request in flight) is dimmed but clearly a button, and the tooltip still reads
  "Translate again in this context" / "Traducir de nuevo en este contexto" per UI locale.
- A warn #1's other half (do not mount `ContextChatPanel` ahead of the main Translate button in
  `TranslateWindow.svelte`'s DOM) remains a keep-constraint, not a test — F's diff did not move it.
