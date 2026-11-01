# Handoff-T-red: Phase 4 - #127 context-send button looks like plain text

**Date:** 2026-11-01
**Branch:** issue-0127-issue-127
**Brief / wireframe reviewed:** `docs/handoffs/0127-issue-127/brief.md`; plan review `docs/handoffs/0127-issue-127/handoff-A.md` (PASS, 2 warns) + `survey.md`. Wireframe: N/A (CSS-class change on an existing control).

## A precondition
Confirmed: A returned **PASS** on the plan (Phase 3), with two non-blocking warns that shaped these tests:
- **warn #2** — "the new test must pin the chosen variant": the tests pin `u-btn--primary` specifically (the brief's "likely `u-btn--primary`" call; it is how the codebase marks a pane's main action, and a filled background + existing `.u-btn:disabled { opacity: 0.5 }` already yields "clearly a button, just dimmed").
- **warn #1** — mounted tests find the main Translate button by *first* `className.includes('u-btn--primary')` match; safety depends on DOM order and the `{#if open}` gate. One assertion pins the gate for `context-send`; see "For F" below for the DOM-order half, which is in `TranslateWindow.svelte` (out of this test's file scope).

Tests land in `frontend/src/components/contextChatWiring.test.ts` (new `describe` block) rather than a new file, per A's note that this component's established cadence is source-pinning in that file (no Svelte render harness, #78; same pattern as `toastCss.test.ts` for the CSS).

## Tests authored
Both are **unit-tier source/CSS pins** (cheapest sufficient tier): jsdom does not lay anything out and there is no render harness for this panel (#78), so the visible-button contract is pinned as `component source ⇄ app.css rule`, the repo's established pattern for exactly this kind of change.

File: `frontend/src/components/contextChatWiring.test.ts`, describe `the send button looks like a button, not text (issue #127)`:

1. **`uses the primary variant, painted by app.css with a background, so it is not bare text`**
   Pins brief criterion "looks like a button… using an existing variant" + A warn #2. Extracts the `<button …>` opening tag that carries `data-testid="context-send"` (so the testid itself is load-bearing), and asserts (a) its static class list contains `u-btn--primary`, and (b) the `.u-btn--primary` rule in `app.css` still has a non-`transparent`/`none` background — the pin cannot be defeated by adding the class and stripping the CSS.

2. **`keeps the look in both states and keeps its identity: unconditional variant, :disabled only dims, testid, tooltip, click, and it stays behind the chat-open gate`**
   Pins brief criteria "the disabled state… is still clearly a button, just dimmed" and "Keep `data-testid`, the title/tooltip, and the current click… behaviour":
   - `u-btn--primary` in the **static** `class="…"` attribute and **no** `class:u-btn--*` Svelte directive — a directive conditioned on enabled-ness would strip the paint exactly when disabled (the reported bug);
   - base `u-btn` stays (`.u-btn:disabled` hangs off it);
   - `disabled={busy || draft.trim() === ''}` unchanged (disabled in both trigger states: request in flight, empty draft);
   - `.u-btn:disabled` rule dims via `opacity` and never blanks `background`/`border`;
   - keeps `data-testid="context-send"`, `title={t('translate.contextSend')}`, `onclick={send}`;
   - the button sits **after** `{#if open}` in the panel (A warn #1: with the chat closed by default it is absent on mount, which keeps the first-match `u-btn--primary` mounted tests safe).

Not duplicated here: the Enter / Shift+Enter / Cmd+Enter keyboard behaviour is already pinned in this same file ("sends on Enter (not Shift+Enter, not Cmd+Enter which translates)"), and A confirmed the brief's "Cmd+Enter behaviour" means *keeping* Enter-to-send with Cmd+Enter reserved for the window's Translate shortcut — no new keyboard pin needed. Label text and send behaviour are out of scope per the brief.

## RED confirmation
Command (from repo root; this repo's frontend suite is `pnpm --dir ./frontend test`, see Makefile `test-frontend` — the Postgres/testcontainers harness notes do not apply, this worktree has no `server/`):

```
$ pnpm --dir ./frontend test src/components/contextChatWiring.test.ts
 ❯ src/components/contextChatWiring.test.ts (16 tests | 2 failed) 12ms
     × uses the primary variant, painted by app.css with a background, so it is not bare text 4ms
     × keeps the look in both states and keeps its identity: unconditional variant, :disabled only dims, testid, tooltip, click, and it stays behind the chat-open gate 0ms

 FAIL … > uses the primary variant, painted by app.css with a background, so it is not bare text
AssertionError: got "u-btn u-no-drag px-3 py-1 text-xs": expected [ 'u-btn', 'u-no-drag', 'px-3', …(2) ] to include 'u-btn--primary'

 FAIL … > keeps the look in both states and keeps its identity: …
AssertionError: expected [ 'u-btn', 'u-no-drag', 'px-3', …(2) ] to include 'u-btn--primary'

      Tests  2 failed | 14 passed (16)
```

Both fail on the feature assertion (the button's class list carries no `u-btn--primary`) — not on imports, setup, or extraction. Full-suite baseline: `pnpm --dir ./frontend test` → **893 passed | 2 failed (895)**; the only failures are the two new tests, the 14 pre-existing tests in the file still pass.

RED validity was verified by throwaway edits to the component (reverted immediately; the real fix is F's job):

| Simulation | Result |
|---|---|
| static `u-btn u-btn--primary u-no-drag …` | 16/16 pass — the suite is green-able by the intended one-class fix |
| static `u-btn--ghost` instead | both new tests fail — variant choice is pinned (A warn #2) |
| `class:u-btn--primary={draft.trim() !== ''}` (bare static) | fails — the paint must not be conditional on enabled-ness |

`git status` after: only `frontend/src/components/contextChatWiring.test.ts` modified; component source untouched.

## Ready for F
**Confirmed RED is valid; F may implement against these tests.** The implementation A passed is: add `u-btn--primary` to the `context-send` button's static class attribute in `ContextChatPanel.svelte` (~line 146); no CSS change is required. For F, from A's warn #1 (informational, not covered by a test — it lives in `TranslateWindow.svelte`): do not move the `<ContextChatPanel>` mount ahead of the main Translate button, and keep the send button behind `{#if open}`.
