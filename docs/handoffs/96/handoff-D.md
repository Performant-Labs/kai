[DESIGN] # #96 D handoff (Design, 2026-09-26)

## Mode
(a) generate a low-fi wireframe. No prior design asset exists for these failed states.

## Wireframe path
docs/handoffs/96/wireframe.html (HTML, zoom control starting at 150%, buttons and - / + / 0 keys).

## Screens and states covered
Drawn on #95's flat pane (no card chrome). Copy is draft (brief D9); full en/zh table is section E of the wireframe.

Translate window, failed pane (`pane === 'failed'`):
- A1 not_configured: headline "No API key set for {engine}", muted detail, Settings button (enabled, calls `ShowSettings()`).
- A2 rate_limit: headline "{engine} is rate-limiting requests; try again shortly", muted detail, no button.
- A3 engine: headline "Translation failed" (existing), muted detail, no button.
- B1 auth: as A1 with Settings button. B2 long/HTML-derived detail (wraps, HTML body replaced by a marker). B3 failed with no payload (15 s fallback): generic headline, no detail, no button (unchanged).
- Other kinds (quota, unavailable, network, pair, too_long) reuse the A2 layout; only the headline differs.
Failed-dot tooltip (title and aria-label): C1 `engineName - headline` when a payload exists; C2 `engineName - Failed` otherwise.
Screenshot TranslateCard: D1/D2 headline replaces the red badge in the header slot, muted detail row below inside the card; D3 no `error` keeps `screenshot.translateFailed`, no detail row. Cards get no Settings button.
Empty/one/many: the window's success and idle states are not altered by this change and are not redrawn.

## Existing components and patterns reused
Palette from `--app-*` properties; `--app-danger` headline; `--app-muted` detail; existing header layout of `TranslateCard`; #69 Settings opener (`ShowSettings`, label `titlebar.settings`); existing `translate.failed`, `translate.engineFailed`, `failedPair`, `screenshot.translateFailed`. Boxed text only, no hand-authored SVG paths.

## Open questions for approval
- Q1: en/zh copy in section E (D9).
- Q2: card placement (headline in header slot, detail below) and no Settings button on cards. Alternative: headline on its own row under the header.
- Q3: dot tooltip shows headline only, never the raw detail.
- Q4: the failed-pane block stays vertically centered (as today) even with a button; alternative is top-aligned like the success text.
- Q5: detail max width about 260 px with wrapping; no truncation UI (SanitizeDetail truncates upstream).

## Verification
Checked headlessly only: well-formed markup via Python `html.parser`. Never rendered in a browser, so layout (wrapping of long headlines in the card header, tooltip placement) is unconfirmed by eye.

## Approval
Approved by operator 2026-09-26 ("wireframe approved"). Open questions Q1-Q5 accepted as drawn: en/zh copy in section E as written; headline in the card header slot with detail below and no Settings button on cards; dot tooltip headline only; failed-pane block vertically centered; detail wrapped at about 260 px with no truncation UI. Visual layout (wrapping, tooltip placement) is confirmed in the principal's hand test after build.
