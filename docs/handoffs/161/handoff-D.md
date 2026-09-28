[DESIGN] # Handoff-D: Phase 2 - Design (#161 source-language correction + Auto landmark)

**Date:** 2026-09-28
**Branch:** issue-161-source-lang-correct
**Mode:** (a) generated low-fi. No earlier wireframe covers the language bar.
**Wireframe:** docs/handoffs/161/wireframe.html (HTML). The zoom control starts at 150%; use its buttons (top right) or the - / + / 0 keys. `#z=1` in the URL starts it at 100%. A checkbox at the top switches each state from 4 frames to all 8 combinations (960/780 px x light/dark x en-US/zh-CN). The default 4 frames per state already cover every width, theme and locale.

## Screens & states covered
Everything is in the Translate window. Frames are whole windows at the real widths: 960 px default and 780 px minimum (`main.go` MinWidth). The language list and its order are the real `ALL_TRANSLATE_LANGS`, and the names are the real `lang.*` strings. The open dropdown is drawn as the native macOS menu, with ✓ on the selected entry. The closed select is drawn at the width of its widest option, which is how a native `<select>` sizes. The page measures each toolbar when it loads.

- **A. The Auto entry, source = Auto** (dropdown open):
  - A1: no detection yet. The entry reads bare "Auto" / "自动", and ⇄ is disabled.
  - A2: detected as another language. The entry reads "Auto — detected: Spanish (Mexico)" / "自动 — 已识别：西班牙语（墨西哥）". The plain "Spanish (Mexico)" entry below it is untouched.
  - A3: detected as the target (identity, #80). The entry reads "Auto — detected: English". The result pane shows the existing `translate.identity` note and the source text. There is no toast.
  - A4: the detection is unusable or low-confidence (the engine echoed `auto`, or reported a native code that `Covers` can't match). The entry falls back to bare "Auto". It makes no claim, shows no toast, and ⇄ stays disabled.
- **B.** A table with the Auto entry's text in every state, in both locales. The pinned "Spanish (Mexico)" entry is never suffixed.
- **C. Pinned source.**
  - C1: pinned and correct. There is no toast and no change; it looks the same as today.
  - C2: option A. The dropdown is reassigned to English and the toast reads "Source corrected to English" / "源语言已更正为英语".
  - C3: option B. The dropdown keeps Spanish (Mexico) and only the toast changes: "Translated from English, not Spanish (Mexico)".
  - C4: a mismatched paste under 20 code points ("ok thanks"). There is no correction and no toast, and the echo stays (unchanged, as the brief decides).
- **D.** The dropdown opened right after a correction, option A next to option B. In both, the Auto entry reads bare "Auto" / "自动", the fixed way back to Auto.
- **E.** The correction toast next to the Copied toast, in both themes and both locales. The strip in E2 shows that only one toast is visible at a time: a Copy click 0.8 s into the correction toast replaces its text and restarts the 1.6 s timer.
- **F.** A table with one line of behaviour per control. **G.** A copy table. **H.** Open questions.

Measured in headless Chrome: in every frame the toolbar fits with no overlap of the gear, and the open menu stays inside the window. The widest from-select is 274 px ("Auto — detected: Portuguese (Portugal)"-class labels). At 780 px it still leaves 93 px or more before the gear. I looked at the rendered screenshots of sections A and C.

## Existing components/patterns reused
- `.u-toast` and `showToast`, with the 1.6 s timer and the single `toast` state, unchanged. This follows #145's cancel toast.
- `detectedSourceLabel` / `fromOptionLabel` as the one relabeling chokepoint. The `<option>` template doesn't change.
- `lang.auto` ("Auto" / "自动"), `translate.identity` and `common.copied`, all existing.
- The `swap()` / `loadDefaults()` "assign `fromLang` directly, never teach" convention, used for option A.
- The window mock, tokens and zoom control come from docs/handoffs/145/wireframe.html.

New keys: `translate.autoDetected` ("Auto — detected: {lang}" / "自动 — 已识别：{lang}") and `translate.sourceCorrected` ("Source corrected to {lang}" / "源语言已更正为{lang}", or the option B wording if B is chosen).

## Open questions for approval
1. **Brief open decision 1:** does a correction reassign the visible dropdown (option A, C2 and the left of D) or leave the pin showing, with only the toast (option B, C3 and the right of D)? Recommendation: **A**. The dropdown and the request that ran never disagree, and the toast explains the jump.
2. **The AC6 tension under option A.** `fromLang` becomes English, so the *next* Translate in this window sends English. AC6 says the next translation "still starts from the user's original pin." Recommendation: AC6 is met because `default_from` stays es-MX (next window open or relaunch) and nothing is taught. Within the window, the dropdown is the truth. Reverting silently would bring back a mismatch between the dropdown and the request. The principal should confirm this reading.
3. **Wording.** The drawing uses "Auto — detected: X" / "自动 — 已识别：X". This keeps the existing `lang.auto` "自动" instead of the brief's "自动检测", so the zh-CN landmark matches the bare entry. The toast is "Source corrected to X" / "源语言已更正为X".
4. **Engines that disagree.** The toast and the reassignment follow the active engine only. Under A, switching to an engine whose result wasn't corrected leaves the dropdown on the corrected value; it does not revert. Is that OK?
5. **Toast duration.** 1.6 s is short for a sentence, especially option B's longer wording. Recommendation: keep the shared 1.6 s with option A's short wording, and revisit only if B is chosen.
6. **Short paste (C4).** A mismatched paste under 20 code points still echoes silently. Please confirm this stays out of scope (the brief decided it).

## Hedged
- All rendering was in headless Chrome, not WKWebView. The native macOS menu is a stand-in drawing, and on macOS the real menu opens over the select, not below it. Widths come from canvas text measurement with the system font stack.
- The toast sits bottom-centre and covers the source pane's Translate button in the 960 px frames. That is the existing `.u-toast` position (`pointer-events: none`), not a new overlap.

## Approval
[To be filled by O. D does not self-approve.]
