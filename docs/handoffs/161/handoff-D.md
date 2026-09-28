[DESIGN] # Handoff-D: Phase 2 - Design, revision 2 (#161 source-language note; the dropdown never changes)

**Date:** 2026-09-28
**Branch:** issue-161-source-lang-correct
**Mode:** (a) generated low-fi. This is a re-design after the principal's override; it replaces revision 1 entirely.
**Wireframe:** docs/handoffs/161/wireframe.html (HTML). The zoom control starts at 150%; use its buttons (top right) or the - / + / 0 keys. `#z=1` in the URL starts it at 100%. A checkbox shows all 8 combinations (960/780 px, light/dark, en-US/zh-CN) per state. By default each state shows 4 frames, which together cover every width, theme and locale.

## What changed from revision 1
The principal overrode it: "I don't want the dropdown to change from Auto". So these are gone:
- the relabeled Auto row ("Auto — detected: X", `translate.autoDetected`)
- option A (reassign `fromLang` to the corrected language) and option B
- the correction toast (`showToast` / `translate.sourceCorrected`), and section E's toast-sequence frames

What replaces them is one persistent, muted line at the top of the result pane's existing note stack: **"Translated from auto-detected {lang}"**. The source select is now provably inert: every option reads `langName(value)`. The frames mark the Auto row, the pinned "Spanish (Mexico)" row and the closed source select with a green dashed "must never change" outline. This walks back #11's shipped Auto-row relabeling, and the PR body should say so.

## Screens & states covered
Everything is in the Translate window. Frames are whole windows at the real widths, 960 px (default) and 780 px (`main.go` MinWidth). The language list and names are the real `ALL_TRANSLATE_LANGS` and `lang.*` values. The result-pane notes copy the real markup (`TranslateWindow.svelte` ≈ :1309-1322: `u-muted px-4`, with `first:pt-4` on the first note, stacked above the textarea).

- **1. The dropdown is open, and it never changes.**
  - 1a: Auto, nothing translated. The Auto entry reads "Auto" / "自动" and has the ✓. ⇄ is disabled.
  - 1b: Auto, and the engine detected Spanish (Mexico). The menu is identical to 1a: bare "Auto" with the ✓, and the "Spanish (Mexico)" row stays plain.
  - 1c: The source is pinned to Spanish (Mexico) and English text was corrected for this request. The ✓ stays on Spanish (Mexico), the Auto row stays bare, and nothing moves.
  - 1d: A table listing every state (6 rows, both locales). The Auto and pinned labels are constant; only the note column varies.
- **2. The new note's three states.**
  - 2a, not shown: the pin was correct. This looks exactly like today.
  - 2a′, not shown: nothing has been translated.
  - 2b, shown under an Auto result: "Translated from auto-detected Spanish (Mexico)" / "译自自动识别的西班牙语（墨西哥）". The dropdown reads "Auto".
  - 2c, shown under a pinned-but-wrong result: "Translated from auto-detected English". The dropdown above still reads "Spanish (Mexico)", unmoved, and the result is a real English→Chinese translation, not the old echo.
- **3. No collision with the existing notes.**
  - 3a: the note stacked with the phonetic (pinyin) line.
  - 3b: note, phonetic and "Cancelled" stacked (a chunked translation that was cancelled).
  - 3c, recommended: identity (detected = target) shows only the identity note, and the new note is suppressed.
  - 3c′, alternative: both lines show, which contradict each other.
  - 3d: the longest name, "Portuguese (Portugal)", at both widths and in both locales.
- **4.** One line per control, stating what it does and when it is disabled or hidden. **5.** A copy table. **6.** Open questions.

**Measured** (headless Chrome, every frame, recorded in the grey line under each frame): the toolbar fits with at least 135 px to the gear at 780 px, and the open menu stays inside the window. The source select is now a constant width (183 px en-US, 169 px zh-CN, set by "Portuguese (Portugal)"), because the Auto label no longer grows. The notes stack with no overlap and stay inside the pane. The new note fits on 1 line in every frame, including "Portuguese (Portugal)" at 780 px in zh-CN. 0 frames were flagged. I looked at the rendered screenshots of sections 1, 2 and 3 (3a and 3b closely). The first render showed the orange marker outline bleeding over the phonetic line, which suggested a collision the layout doesn't have. I changed the marker to an inset left bar and re-checked.

## Existing components/patterns reused
- The pane's muted note stack (`u-muted px-4 first:pt-4`), as used by phonetic, cancelled (`text-[11px]`) and identity (`text-xs`). The new note is drawn at `text-[11px]`, as the brief suggests.
- `lang.auto`, `translate.identity` and `translate.cancelled`, all existing and unchanged.
- The window mock, tokens and zoom control come from revision 1 and #145's wireframe.
- New key: `translate.translatedFromDetected`, "Translated from auto-detected {lang}" / "译自自动识别的{lang}". `translate.detected` is no longer used by the select; F removes it if nothing else uses it.
- **Not used:** `showToast` / `.u-toast`, `translate.autoDetected`, `translate.sourceCorrected`. Nothing assigns `fromLang` either.

Rendering rule drawn for T and F: the note shows when `activeResult.detected_from` is non-empty, differs from the displayed FROM value, a result is showing, and (recommended) `identity` is false. It follows the active engine, so switching engines can show or hide it. It has no timer and is not clickable, and Copy never includes it.

## Open questions for approval
1. **zh-CN wording.** "译自自动识别的{lang}" (drawn) mirrors the principal's sentence. "源语言自动识别为{lang}" reads more naturally but drops "translated". Recommendation: as drawn.
2. **Size.** `text-[11px]` (drawn; the brief's suggestion, matching "Cancelled") or `text-xs` (matching phonetic and identity)? Recommendation: 11 px. It sets the line apart from the phonetic reading directly below it.
3. **Identity case.** Suppress the new note when `identity` is set (3c), or show both lines (3c′)? Recommendation: suppress. "Translated from …" over text that wasn't translated contradicts the identity line.
4. **⇄ after a pinned mismatch** swaps the pin (Chinese ↔ Spanish (Mexico)), not the language actually detected (English). The brief puts swap logic out of scope. Recommendation: leave swap unchanged. The note already shows the mismatch.
5. **Position.** First in the note stack (drawn) or last, next to the text? Recommendation: first. It is about where the text came from, and the existing notes keep their relative order. The lines stack with no gap between them, as the existing notes already do.
6. **Short pastes.** A mismatched paste under 20 code points still echoes silently, with no note. Please confirm it stays out of scope (the brief decided it).

## Hedged
- All rendering was in headless Chrome, not WKWebView. The native macOS menu is a stand-in drawing; on macOS the real menu opens over the select, not below it. Widths come from canvas text measurement with the system font stack.
- The phonetic line in 3a and 3b is illustrative pinyin. Which engines actually return `phonetic` for which targets wasn't checked; the frame only tests stacking.

## Approval
[To be filled by O. D does not self-approve.]
