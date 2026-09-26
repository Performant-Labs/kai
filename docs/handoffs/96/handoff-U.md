[U] #96 UI walkthrough handoff (2026-09-26)

## T precondition
Met: handoff-T-green.md verdict PASS. Re-ran `pnpm --dir frontend test` (NODE_OPTIONS=--no-experimental-webstorage): 23 files, 215 tests passed.

## Wireframe conformance (headless, markup and copy read against wireframe.html, approved 2026-09-26)
- Failed pane (TranslateWindow.svelte): centered flex column kept (Q4); danger headline; muted detail with `max-w-[260px] break-words` (Q5, no truncation UI); Settings button (`u-btn--ghost`, label `titlebar.settings`, en "Settings" / zh "设置") only when `failure.action === 'settings'`, i.e. not_configured and auth; onclick `ShowSettings()`. Matches A1/A2/A3/B1/B2. B3 (no payload) yields generic headline, empty detail, no button.
- Dot tooltip and aria-label: `engineName - headline` when the failed engine has `error`, else `engineName - Failed` (C1/C2); headline only, never the raw detail (Q3).
- TranslateCard: failed card stacks language line over headline, right-aligned in the header slot; muted detail `<p data-testid="failure-detail">` below; no Settings button (Q2); no `error` keeps `screenshot.translateFailed` and no detail row (D3).
- Copy: all 10 rows of wireframe section E match en-US.ts and zh-CN.ts exactly (failedAuth and failedNetwork changed as marked; failedPair unchanged for Apple; failedUnsupported for other engines).
- Enabled/disabled: Settings button always enabled; no other new controls.

## Run environment
Headless only. No browser, no screenshots, no live Wails app (issue #28: synthetic input infeasible). Source, i18n tables and vitest contracts inspected.

## State matrix
Walked headlessly (markup/copy/tests): A1, A2, A3, B1, B2 (markup wrap classes), B3, C1, C2, D1, D2, D3 in en-US and zh-CN.
Not walked live, for the principal's hand test on the built app:
1. Wrapping of long headlines in the card header slot (D1/D2) and long zh headlines.
2. Vertical centering of the failed block with a button (Q4) and 260 px detail wrap (B2 with a long/HTML-derived detail).
3. Dot tooltip placement and text on a real hover (C1/C2).
4. Settings button actually opens Settings and the translate window stays open.
5. A real not_configured engine (key unset) and a real rejected key end to end, incl. Apple pair copy.
6. Language switch to zh-CN mid-session re-renders headlines.

## Findings
- None blocking. Note (from T/F, not a UI defect): merge with #95 needs the `flatResultPane` assertion updated; layout on the #95 flat pane is unseen.

## Evidence
git diff --cached of TranslateCard.svelte, TranslateWindow.svelte, resultPane.ts, en-US.ts, zh-CN.ts, keys.ts; wireframe section E table; vitest run above.

## Verdict
PASS (headless conformance). Live layout cells above remain for the principal's hand test.
