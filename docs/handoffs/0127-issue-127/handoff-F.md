# Handoff-F: Phase 5 - Implement

**Date:** 2026-11-01
**Branch / worktree:** issue-0127-issue-127 (/Users/andreangelantoni/Projects/kai/.worktrees/0127-issue-127)
**Contract:** handoff-T-red.md (tests + the behaviors they pin)

## What I changed

- `frontend/src/components/ContextChatPanel.svelte` (1 line, the `context-send` button's opening tag):
  added `u-btn--primary` to the **static** `class` attribute — `u-btn u-no-drag px-3 py-1 text-xs` →
  `u-btn u-btn--primary u-no-drag px-3 py-1 text-xs`. This is the entire fix.
  - Satisfies both new tests: `uses the primary variant…` (a: class list contains `u-btn--primary`;
    b: `.u-btn--primary` in `app.css` already paints `background: var(--app-accent)` — verified, no CSS
    change needed) and `keeps the look in both states…` (static attribute, so no `class:` directive that
    could strip the paint when disabled; `u-btn` base kept for `.u-btn:disabled` → `opacity: 0.5`, which
    only dims; `data-testid` / `title={t('translate.contextSend')}` / `disabled={busy || draft.trim() === ''}`
    / `onclick={send}` untouched; button left exactly where it was, inside `{#if open}`).
  - Rationale: matches the brief's "likely `u-btn--primary`" call and the codebase idiom — every other
    primary button (`TranslateWindow.svelte:1966`, all Settings tabs) writes `u-btn u-btn--primary …`
    statically. No CSS, no DOM move, no behaviour change.

Nothing else was touched. `git diff` shows exactly that one line; the test file and handoff docs are T's
staged work.

## Self-check

- `pnpm --dir ./frontend test src/components/contextChatWiring.test.ts` → 16/16 passed (was 14/16).
- `pnpm --dir ./frontend test` (full suite) → **895 passed | 0 failed (82 files)**, matching the RED
  baseline of 893 passed | 2 failed with both new tests now green. The `HistoryList.svelte` a11y warning
  in the output is pre-existing in a file this run never touched.
- Note: the authoritative GREEN is the plugin's T-green run, not this self-check.

## Tests I think are wrong (if any)

None. Both tests read as correct pins of the brief; the RED simulations in handoff-T-red.md matched my
reading, and the suite went green on the intended one-class fix.

## Ready for T(green)

Implemented against the RED contract. The plugin should now run the suite: GREEN means the loop
advances; RED means I rework.
