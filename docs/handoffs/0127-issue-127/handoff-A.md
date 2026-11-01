# Handoff-A: Phase 3 - #127 context-send button looks like plain text  (up-front plan review)

**Date:** 2026-11-01
**Branch:** issue-0127-issue-127
**Brief reviewed:** docs/handoffs/0127-issue-127/brief.md   **Reuse map:** docs/handoffs/0127-issue-127/survey.md (issue-only, deterministic driver per ADR 0027; the named objects live in the brief)   **Wireframe:** N/A
**Verdict:** PASS

## Summary
PASS. The plan extends the exact existing objects — the `context-send` `<button>` in `ContextChatPanel.svelte:145-153` and an existing CSS variant from `app.css` — with no new component, no new CSS, and no parallel path. The brief's cause analysis is accurate against the source: bare `.u-btn` carries no background or border (`app.css:237-244`), and a repo-wide grep shows 43 of 44 button class strings carry a variant (`--primary`/`--danger`/`--ghost`/`--link-danger`); `ContextChatPanel.svelte:146` is the sole bare one. The plan fixes the outlier toward the dominant pattern, which is the opposite of drift. Two `warn` findings for T/F awareness; neither blocks.

## Findings

| # | Severity | Plan element | Drift dimension | Finding | Suggested fix |
|---|---|---|---|---|---|
| 1 | warn | `u-btn--primary` on `context-send` | pattern consistency | Four mounted tests find the main Translate button by first-match on `className.includes('u-btn--primary')` (`mountedSourceSwitch.test.ts:210-213`, `mountedAutoTranslate.test.ts:192`, `mountedBackTranslationCopy.test.ts:105`, `mountedSourceCorrection.test.ts:118`), and they mount the real `TranslateWindow`, which renders this panel. Safe today only because the main Translate button (`TranslateWindow.svelte:1966`) precedes the panel (line 2301) in document order and the send button sits behind `{#if open}` with the chat closed by default — an order-sensitive coupling the plan silently creates. | No change required now. T: if a mounted test opens the context chat, assert on `data-testid="context-send"` (already present) rather than by variant class; keep DOM order and the `{#if open}` gate intact. |
| 2 | warn | "using an existing variant (likely `u-btn--primary`)" | pattern consistency | Two defensible variants: `--primary` matches how the codebase marks a pane's main action (Translate button `TranslateWindow.svelte:1966`, save/submit buttons in settings), and with a filled background the existing `.u-btn:disabled { opacity: 0.5 }` (`app.css:250`) already yields "clearly a button, just dimmed". `--ghost` would match the four sibling buttons in this same panel (`ContextChatPanel.svelte:61,79,98,128`). Both are inside the existing variant system, so neither is drift — but the choice should be pinned, not left open. | Keep the brief's `--primary` call; the new test must pin the chosen variant so it cannot regress to bare `u-btn`. |

## Notes for O
Informational (PASS, no amendment required): the brief's "keep … the current click and Cmd+Enter behaviour" is correct as a keep-constraint, but in the panel Cmd+Enter deliberately does NOT send — Enter sends and the keydown filter excludes `metaKey` (`ContextChatPanel.svelte:51`, pinned by `contextChatWiring.test.ts:91` as "not Cmd+Enter which translates"). One clarifying word in the brief ("keep Enter-to-send; Cmd+Enter remains the window's Translate shortcut") would prevent T from pinning a Cmd+Enter send as new behaviour. Also, the new variant-pinning test belongs in `contextChatWiring.test.ts`'s `describe('the chat panel (issue #48)')` block using the existing `expect(panel).toMatch(...)` structure-pinning pattern — that file's header documents there is no Svelte render harness (#78), so source pinning is the established cadence for this component.

## Patterns referenced
- `frontend/src/app.css:236-274` — the `.u-btn` base + variant system; bare `.u-btn` has no background/border, `.u-btn:disabled` dims via opacity.
- `frontend/src/components/ContextChatPanel.svelte:58-153` — the extend target; all four sibling buttons use `u-btn u-btn--ghost u-no-drag …`, confirming a missing variant is the anomaly.
- `frontend/src/components/TranslateWindow.svelte:1966, 2301-2314` — `u-btn--primary` marks the main action; panel mounts after the Translate button, gated by `chatOpen`.
- `frontend/src/components/contextChatWiring.test.ts:76-96` — the structure-pinning test pattern for this component (no render harness, #78), where the new test should land.
- Repo-wide `class="u-btn…"` grep (44 hits) — 43/44 carry a variant; `ContextChatPanel.svelte:146` is the only bare one.
