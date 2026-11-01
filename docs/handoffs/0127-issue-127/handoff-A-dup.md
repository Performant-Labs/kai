# Handoff-A-dup: Phase 7 - #127 "Translate again in this context" looks like plain text  (anti-duplication gate)

**Date:** 2026-11-01
**Branch:** issue-0127-issue-127
**Diff base:** 0106912   **Diff head:** 0106912 (working tree)
**Reuse map:** docs/handoffs/0127-issue-127/survey.md (issue-as-survey per ADR 0027; the extend-vs-new recommendation lives in the brief's "Wanted": reuse an existing `u-btn` variant, "likely `u-btn--primary`")
**Verdict:** PASS

## Summary

F extended the existing object exactly as the map/brief named: the `context-send` button in `frontend/src/components/ContextChatPanel.svelte` now carries the existing `.u-btn--primary` CSS variant (`frontend/src/app.css:255`), written statically as `u-btn u-btn--primary …` — the identical idiom used by every other primary action in the codebase (`TranslateWindow.svelte:1966`, `settings/EnginesTab.svelte:876,1113`, `settings/ShortcutsTab.svelte:491,522,548,702`). No new CSS class, no new component, no inline style, no wrapper — zero parallel path. The test additions land in the component's own wiring test file and follow its established pinned-from-source pattern (`readFileSync` + structural regex, no render harness, per the file's #78 header comment).

## Findings

| # | Severity | File:line | Finding | Suggested fix |
|---|---|---|---|---|
| — | — | — | No duplication; extension is clean. | — |

Verified specifically:

- **Extend, not duplicate:** the one-line diff adds `u-btn--primary` to the existing static `class` attribute. The variant already paints `background: var(--app-accent)` (`app.css:255-258`), and the base `.u-btn:disabled { opacity: 0.5 }` (`app.css:250-253`) supplies the dimmed-but-still-a-button state the brief asked for. No new styling seam was created.
- **Pattern consistency:** static variant class matches all nine existing `u-btn--primary` call sites; the sibling cancel button in the same file (`ContextChatPanel.svelte:128`) uses the same `u-btn u-btn--ghost u-no-drag …` shape, so the panel stays internally consistent.
- **Test placement/style:** the new describe block in `contextChatWiring.test.ts` sits beside the file's existing #48 describes, reuses its source-pinning approach (and the already-loaded `panel` constant), and reads `app.css` the same way `settings/shortcutsTabHistory.test.ts` reads a tab source. It also pins the brief's keep-list (`data-testid`, tooltip, `onclick={send}`, disabled expression) rather than inventing new conventions. Test-case adequacy is T's dimension; from the architecture side the style matches its neighbors.
- **Repo gates untouched:** no migrations, no `pg`/`testcontainers` imports, no DB ownership, no cross-workspace paths, no Fastify root hooks. Diff is exactly two files, both in scope.
- **No drive-by edits:** F's handoff claim ("nothing else was touched") matches the diff byte-for-byte.

## Notes for F

None — PASS.

## Patterns referenced

- `frontend/src/app.css:237-274` — the `.u-btn` base + variant system F extended into
- `frontend/src/components/TranslateWindow.svelte:1966` — the static `u-btn u-btn--primary …` idiom
- `frontend/src/components/settings/ShortcutsTab.svelte:491` — same idiom in Settings
- `frontend/src/components/contextChatWiring.test.ts:1-95` — the pinned-from-source test pattern (no render harness, #78) the new describes follow
- `frontend/src/components/settings/shortcutsTabHistory.test.ts` — precedent for tests reading component/CSS sources by file
