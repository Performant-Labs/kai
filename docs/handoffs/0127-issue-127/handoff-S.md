# Handoff-S: Phase 9 - #127 context-send button looks like plain text

**Date:** 2026-11-01
**Branch:** issue-0127-issue-127
**Issue:** #127 (Performant-Labs/kai)
**Handoff-T reviewed:** docs/handoffs/0127-issue-127/handoff-T-green.md
**Handoff-A reviewed:** docs/handoffs/0127-issue-127/handoff-A.md, docs/handoffs/0127-issue-127/handoff-A-dup.md
**Handoff-F reviewed:** docs/handoffs/0127-issue-127/handoff-F.md
**Operator-facing report:** N/A — non-visual audit cycle (see "Visual regression" below for why no pixel diff exists for this change)

## A precondition

Confirmed: handoff-A (up-front plan review) returned **PASS** with two non-blocking `warn` findings (variant-choice pinning; mounted-test coupling), and handoff-A-dup (anti-duplication gate) returned **PASS** with zero findings. Both warns were adopted downstream: the variant is pinned (`u-btn--primary`), and the `{#if open}` gate half of warn #1 is now test-pinned.

## T precondition

Confirmed: handoff-T-green reports zero blocking issues — target file 16/16, full suite 895/895 across 82 files, `tsc` clean, `build:dev` clean, plus a pin check (strip the class → 2 fail | 14 pass) proving the new tests fail in isolation for the right reason.

## Browser-tool / Visual-diff-tool precondition

**N/A for this target — determination stated, not buried.** My stage instructions scope the browser and pixel-diff preconditions to "visual / web projects". This repo (`performant-labs/kai`) is a **Wails desktop application**, not a web project: the frontend requires `@wailsio/runtime` and the Go backend, there is no URL to load, no Svelte render harness exists (issue #78, documented in the test file header), there is no Playwright/e2e workspace anywhere in the repo, and there are **no reference design artifacts** (no preview HTML, no design-spec slice, no prior baseline) to diff against — the diff's "reference" side is undefined. Nothing the operator could fix in this session changes that; `CANNOT-AUDIT` is reserved for a fixable environment defect, which this is not.

Instead, this repo's release-owned process gives on-screen verification its own stage: the pipeline runs `ui-walkthrough` before `spec-audit` (AGENTS.md phase order; `.aftersight/opencode.json` stage table), and every prior UI story in decisions.md (#96, #109, #118) resolved visuals as "headless static walkthrough + live cells left for the principal's hand test". Critically, **the spec itself accepted the non-visual route**: issue #127's fourth Wanted bullet is "Add a test that pins the variant class, **or** a visual check" — the test route was chosen and delivered. I did not substitute CSS-rule reading for a render (see Spec compliance: the checks below verify the contract the spec's test-route demands, not "computed-style says it's fine"), and I make no claim of having seen pixels.

## Visual diff results

No pixel diff was produced — no render surface and no reference exist in this repo (see above). Rows per the template, marked accordingly:

| Viewport | Pixels different | Whole-page delta % | Diff PNG | Composite |
|---|---|---|---|---|
| 1280×800 | N/A — no render surface | N/A | — | — |
| 768×1024 | N/A — no render surface | N/A | — | — |
| 360×800 | N/A — desktop app, breakpoints not applicable | N/A | — | — |

**Residual, explicitly not closed by this audit:** the on-screen look (accent-filled send button in the enabled state; dimmed-but-clearly-a-button in the disabled state, i.e. empty draft or request in flight; tooltip per locale) still needs the ui-walkthrough stage and/or the operator's live walk, exactly as T's advisory and the decisions.md T-GREEN entry ("source pins are necessary, not sufficient") already state. O commits with that knowledge.

## Spec compliance

The binding spec is issue #127 (fetched and read; the brief restates it verbatim — no divergence to referee). "Wanted" bullets, each verified against the working tree (`ContextChatPanel.svelte:145-153`, `app.css:237-274`):

| Spec requirement | Compliant | Evidence |
|---|---|---|
| Looks like a button in both states: visible background or border via an existing variant (likely `u-btn--primary`) | YES | `class="u-btn u-btn--primary u-no-drag px-3 py-1 text-xs"` written **statically**; `.u-btn--primary { background: var(--app-accent); color: var(--app-accent-fg) }` (app.css:255-258) paints. No new CSS, no new class, no inline style — the brief's "existing variant" constraint met exactly. |
| Disabled state (empty context text / request in flight) still clearly a button, just dimmed | YES | `disabled={busy \|\| draft.trim() === ''}` unchanged; `.u-btn:disabled { opacity: 0.5; cursor: not-allowed; }` (app.css:250-253) only dims — it never blanks background/border, and no `class:` directive can strip the variant per-state (pinned by test 2). Enabled → filled accent button; disabled → same button at 50% opacity. |
| Keep `data-testid="context-send"`, title/tooltip, click and Cmd+Enter behaviour | YES | Diff shows the class attribute is the **only** change; testid, `title={t('translate.contextSend')}`, `onclick={send}` byte-untouched. Keyboard: `onkeydown` untouched — Enter sends, Cmd+Enter stays the window's Translate shortcut (A's clarifying note; T logged the assumption in decisions.md; pre-existing wiring test still pins it). i18n key exists in both catalogs (en-US.ts:85, zh-CN.ts:81); label text untouched, and the change is locale-independent. |
| Add a test that pins the variant class (or a visual check) so it does not regress | YES | Two new tests in `contextChatWiring.test.ts:105-159`; T's pin check (remove class → 2 failed at `toContain('u-btn--primary')`) proves the regression trips them. RED-phase simulations also ruled out `--ghost` and conditional-`class:` escapes. |
| Out of scope: label text / send behaviour | YES | One-line diff touches only the class attribute; `send()` and the label expression untouched. |

**Spec/preview sanity check (run before deep checks):** the spec itself is sound — standard variant-reuse fix, tokens-based contrast (`--app-accent`/`--app-accent-fg`), standard disabled semantics, no convention violations. The one wording ambiguity ("keep … Cmd+Enter behaviour") was already clarified by A and logged by T; not a source-of-truth defect, so no ADVISORY-HOLD.

## Quality audit

| Area | Result | Notes |
|------|--------|-------|
| API consistency | N/A | No API surface in this change. |
| Error handling | N/A | A class change adds no error paths. |
| UI/UX match to spec | PASS | Matches the spec's described outcome; `--primary` marks the pane's main action per codebase idiom (TranslateWindow:1966, Settings tabs), while the four panel siblings stay `--ghost` — the send action now reads as the primary control, which is the brief's intent. Loading state (busy) pre-existing and untouched. |
| Accessibility | PASS | Real `<button>` with tooltip, now with visible affordance — a net a11y improvement over the reported faded-text look; foreground/background from the design-system token pair used by all nine other primary buttons; keyboard path unchanged. |
| Architecture gate | PASS | A up-front PASS (2 warns, both adopted) and A-dup PASS (0 findings). No quality finding below conflicts with either. A-warn-1 coupling re-verified empirically: the five mounted tests that first-match `className.includes('u-btn--primary')` (mountedSourceSwitch:212, mountedAutoTranslate:192, mountedBackTranslationCopy:105, mountedSourceCorrection:118, shortcutsTabHistory:51) stay safe because the main Translate button (TranslateWindow.svelte:1966) precedes the panel mount (2301) in DOM order and `chatOpen` defaults `false` (line 1481) — and the full suite is green. The `{#if open}` gate half is now pinned (test 2); TranslateWindow DOM order remains a documented keep-constraint. |
| Code organization | PASS | One-line production diff; test additions sit in the component's own wiring test file, reusing its `panel` constant and source-pinning idiom, with an explanatory header comment. No dead code, no TODOs, no commented-out code in the hunks. |
| Security | N/A | No input handling, no auth surface; the diff adds one static class name. |
| Performance | PASS | No impact — one class, painted by a pre-existing rule; no new CSS, deps, or runtime work. |
| Visual regression | N/A | No pixel diff possible in this repo (no render surface, no reference — see precondition section). Source-level contract fully pinned; on-screen look routed to ui-walkthrough / operator live walk. |
| Naming consistency | PASS | `u-btn u-btn--primary …` spelled statically, matching all nine existing primary call sites; testid unchanged. |
| Test quality (test-quality rubric §7) | PASS | **Per test:** both name behaviors, not implementation for its own sake — test 1 = "variant paints" (class present **and** the app.css rule keeps a non-transparent/none background); test 2 = "both states stay painted + identity hooks + chat-open gate". Cheapest sufficient tier: source+CSS pins, matching the file's documented no-render-harness cadence (#78). Fail-in-isolation proven by T's pin check. **Suite proportionality:** 2 pins for a 1-line class change is exactly the brief's ask (893→895 total); the 14 pre-existing tests untouched. **Smells:** none — no assertion-free/tautological, mock-shaped, or snapshot tests; the `--primary` assertion appearing in both tests is mild duplicate signal but serves distinct points (variant choice vs. state-unconditionality) — no delete/merge finding. **One robustness gap (advisory, non-blocking):** test 1 would not catch outright *deletion* of the `.u-btn--primary` rule from app.css — the captured `bg` falls back to `''`, which passes `not.toMatch(/^(transparent|none)$/i)`. Test 2 guards the `:disabled` rule against deletion (`not.toBe('')`) but test 1 lacks the symmetric guard. Low risk (deleting that rule visually breaks nine other primary buttons), noted below. |

## Scope check

Delivered **exactly** the phase scope, neither exceeded nor under-delivered:

- Production change: 1 line, 1 file (`ContextChatPanel.svelte`) — the sole change the brief's "Wanted" implies.
- Test change: 1 file (`contextChatWiring.test.ts`, T's staged Phase-4 work) — the brief's fourth bullet.
- No CSS file changes, no new components, no DOM moves, no behavioural edits, no drive-by fixes. `git status` shows only: the two source files, T's staged `handoff-T-red.md` + `decisions.md` entries, and the run's untracked handoff docs. Matches A-dup's "diff is exactly two files" byte-for-byte claim.

## Verdict

**Verdict:** PASS

All acceptance criteria in issue #127 are met and verified against source: the control uses the existing `u-btn--primary` variant statically (painted background), the disabled state is the same button dimmed via opacity only, every keep-constraint (testid, tooltip, click, Enter-to-send with Cmd+Enter reserved for Translate) is byte-untouched and test-pinned, and the regression pin the spec demanded exists and provably fails when the fix is removed. Scope is exact; A's and T's gates all passed; the architecture warns were adopted. The one dimension this audit cannot execute — on-screen pixels — has no render surface or reference in this Wails desktop repo, is explicitly owned by the pipeline's ui-walkthrough stage and the operator's live walk, and the spec itself accepted the delivered test-pinning route in its stead. Ready for O to commit (see advisory 2 on staging).

## Advisory notes

1. **Test 1 deletion-guard (non-blocking):** if `contextChatWiring.test.ts` is touched again, add `expect(primary, '.u-btn--primary rule must exist').not.toBe('')` for symmetry with test 2's `:disabled` guard, so removing the rule from app.css trips the suite. Not worth a rework cycle now.
2. **Staging state for O:** `ContextChatPanel.svelte` is modified-but-**unstaged** while `contextChatWiring.test.ts` is staged. Stage both (plus the run docs) together so the commit isn't accidentally split.
3. **Live walk (owner: ui-walkthrough / operator):** enabled state should show the accent-filled send button; disabled state (empty draft, or a request in flight) the same button dimmed to ~50%; tooltip "Translate again in this context" / "按此上下文重新翻译" per locale. The issue's Spanish wording has no es catalog in this repo (en-US and zh-CN only) — immaterial here since the change is locale-independent and label text is out of scope.
4. **Keep-constraint reminder for future edits:** do not mount `ContextChatPanel` ahead of the main Translate button in `TranslateWindow.svelte`'s DOM, and keep the send button behind `{#if open}` — five mounted tests locate the main Translate button by first `u-btn--primary` match (the gate half is pinned; the DOM-order half is not, by design — wrong file, brittle from source).
