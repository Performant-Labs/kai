## D, Phase 2 (revision 2), 2026-09-28
- **Decided:** The FROM dropdown is inert. Every option reads `langName(value)`; nothing assigns `fromLang`, and there is no toast. The detected source is shown by one persistent muted note at the top of the result pane's note stack ("Translated from auto-detected {lang}", `text-[11px]`, new key `translate.translatedFromDetected`).
- **Assumed:** The note is suppressed when `identity` is set, and zh-CN reads "译自自动识别的{lang}". Both are open questions 1 and 3 in handoff-D.md, awaiting the principal.
- **Hedged:** Rendered in headless Chrome, not WKWebView. The native menu is a stand-in drawing.
- **Evidence:** docs/handoffs/161/wireframe.html. The per-frame measure lines show 0 flagged frames: the toolbar fits, the menu stays inside the window, the notes don't overlap, and the new note is 1 line at 780 px in both locales.

## D, Phase 2 (revision 3), 2026-09-28
- **Decided:** The source dropdown's Auto entry (value `auto`) always reads "Detected" (new key `translate.sourceAuto`), in every state. The shared `lang.auto` stays "Auto" for `GeneralTab.svelte:79`. Everything else is carried from revision 2 unchanged: the dropdown is inert, and the persistent muted note is the only cue.
- **Assumed:** zh-CN "自动检测" for "Detected" (open question 0 in handoff-D.md, awaiting the principal).
- **Hedged:** Rendered in headless Chrome, not WKWebView. The native menu is a stand-in drawing.
- **Evidence:** docs/handoffs/161/wireframe.html. 48 measured frames, 0 flagged. A headless screenshot of section 1 shows "Detected" / "自动检测" ✓ in all four 1a frames.

## A, Phase 3 (up-front plan review), 2026-09-28
- **Decided:** BLOCK. 3 block findings, 4 warns and 3 passes (docs/handoffs/161/handoff-A.md).
  - The plan reads the pinned-path detection from `detectedSource`, but engines echo the pin on a pinned request. Apple's bridge detects only for an auto source (`apple_translate.swift:253`, :273-279). DeepL and Tencent use `echoLanguage(req.From)`, and the LLM engines return `req.From`. So the correction cannot fire for the engine the bug was reported on.
  - A request within budget has one engine call and nothing to decide before it. Fixing it after the call means a second engine call, which `service.go:242` rules out.
  - The brief still specifies the superseded `Corrected` boolean, toast and `fromLang` reassignment.
- **Passed:** `translate.sourceAuto` leaves `lang.auto` untouched (`GeneralTab.svelte:79` is its only other use). The note sits outside the result textarea, as a sibling in the existing note stack (`TranslateWindow.svelte:1309-1322`).
- **Assumed:** Google gtx may report a real detection on pinned requests. Not verified against the live API.
- **Hedged:** Engine behaviour was read from source only. No engine was called.
- **Evidence:** handoff-A.md findings #1-#10, with file:line citations.
