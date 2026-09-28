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

## A, Phase 3 (re-review #1), 2026-09-28
- **Decided:** BLOCK again. 3 block findings, 3 warns and 3 passes (docs/handoffs/161/handoff-A.md).
  - The substituted-auto mechanism resolves the prior #1: one call, no detector, and every engine accepts auto.
  - But the stale toast, `Corrected` and `fromLang`-assignment text is still there: brief :32, :33, :91, :93, :131, :147, :151, :158, :167 and :175.
  - The data flow omits the case where the detection covers the target, which is the likely bug shape. The seam, what chunks 2..N receive, and the qualification of the corrected `From` are unnamed.
  - Reuse map :135 still has a frontend "differs from displayed FROM" comparison, and (a)4 has no Auto-case rule.
- **Passed:** Scope limits held: no engine, chunker or registry change. `detectedLang.ts` deletion and the ScreenshotRetranslate acceptance are in the brief, though contradicted at :115, :149 and :180.
- **Assumed:** The recommended outcome when the detection covers the target is an identity result with `DetectedFrom` empty, consistent with D's rule that the note is suppressed on identity. O or the principal confirms.
- **Hedged:** Read from source only. No engine was called.
- **Evidence:** `service_chunk.go:103-116`, `service.go:259`, `engine.go:30-35`, `apple_darwin.go:72`, `variants.e2e.test.ts:34-49`.

## A, Phase 3 (re-review #2), 2026-09-28
- **Decided:** BLOCK. 4 block findings, 2 warns and 3 passes (docs/handoffs/161/handoff-A.md). The brief closes the prior B2's structure: the named `sent` seam, the identity/covers-target branch, `pinFallback`, and `Qualify` via `resultFrom(auto, d)`. The prior W1 and W3 are fixed, and the scope limits held. Checked against the code, the brief still fails on four counts:
  - B1: `detectedSource` returns `ok=true` for a native code (Baidu `jp`), and `SameAs("jp","ja")` is false, so every Baidu request of 20 or more code points pinned to `ja` gets a false correction. The fix is to gate the correction on `ParseLanguage`.
  - B2: `identityResult(…, req, …)` reports the wrong pin as `From`, via `resultFrom(req.From, …)`. The fix is to pass `sent`.
  - B3: `DetectedFrom` is "empty always on the non-substituted path", which kills the Auto-mode note that the Visible-cue section and the approved wireframe 2(b) require.
  - B4: stale text remains at brief :32, :33, :210 and :236.
- **Assumed:** Baidu's `br.From` is a native code (`jp`, `kor`) per the existing comments in `detectedSource`/`resultFrom`. The live API was not called. Recommended: no 20-code-point floor on the Auto-case note; O or the principal confirms.
- **Hedged:** Read from source only. No engine was called.
- **Evidence:** `service.go:279-287` and the `identityResult` `From` line; `model.go` `SameAs`/`canonical`; `baidu.go:120-122`; `service_chunk.go:103-128`.
