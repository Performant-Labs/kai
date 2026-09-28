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

## A, Phase 3 (re-review #3), 2026-09-28
- **Decided:** BLOCK, narrowly: 2 block findings, 4 warns and 3 passes (docs/handoffs/161/handoff-A.md). The pseudocode now resolves every prior finding against the real code: the `ParseLanguage` gate (B1), `sent` passed to the identity branch (B2), the Auto-path `DetectedFrom` (B3), the stale text removed (B4), the pin's dialect preserved for chunks 2..N (W1), and open decision 3 settled (W2). The surrounding contract text was not updated to match:
  - B1: :19, :189 and :263 still say `detectedSource` returns `ok=false` for a native code, and Reuse row :235 omits the gate.
  - B2: (a)4 :53 and Reuse row :239 still say `DetectedFrom` is "empty always on the non-substituted path". :208 still applies the 20-code-point floor to the note. The substituted path stores bare `detected` while the Auto path stores the qualified `From`.
- **Passed:** Scope limits held: no engine, chunker, registry, `resultFrom` or `detectedSource` change beyond the named `pinFallback` extension point and the one model field. `c.from` carries chunk 1's raw detection (`service_chunk.go:183-186`), so the chunked post-call decision is consistent.
- **Assumed:** The Auto path's qualified value is the one both paths should use, per the prior review's B3 fix and wireframe 2(b)'s "Spanish (Mexico)".
- **Hedged:** Read from source only. No engine was called.
- **Evidence:** `service.go:279-287`, `service_chunk.go:103-128` and `:183-186`, `model.go:113-185`, `handoff-D.md:50`.

## A, Phase 3 (re-review #4), 2026-09-28
- **Decided:** PASS (docs/handoffs/161/handoff-A.md at head 4258c9f), with 0 blocks, 4 warns and 4 passes.
  - B1 residual is fixed in the prose. :19, :200, :246 and :274 now put native-code exclusion on the `ParseLanguage` gate, and `detectedSource` is `ok=false` only for the auto echo.
  - B2 residual is fixed. (a)4, Reuse row :250, pseudocode :140-161 and open decision 3 state one `DetectedFrom` contract. The value is always the qualified `From`. Auto: always set, with no floor. Substituted pin: set only on `!SameAs`. Identity, match and unusable: empty. :219 no longer floor-gates the note.
  - The W1, W2 and W4 fixes are in. `handoff-D.md:50` now has no frontend comparison.
- **Warns handed to T:**
  - Reuse row :245 still shows the untrimmed length. The pseudocode :78 governs.
  - Missing test: an Auto request under the floor must return `DetectedFrom == Qualify("es")`.
  - Missing assertion: `DetectedFrom == From` on the real-mismatch test.
  - The frontend render condition should be non-empty and `!identity`, with no `fromLang` comparison.
- **Passed:** Scope held. The diff since base is docs only. The only extension points are `pinFallback` and the one model field. `resultFrom` and `detectedSource` are unmodified.
- **Hedged:** Read from source only. No engine was called.
- **Evidence:** `service.go:279-287`, `:389-393`; `service_chunk.go:103-128`, `:183-186`; `model.go:113-185`; `handoff-D.md:50`.

## T, Phase 4 (RED), 2026-09-28
- **Decided:** RED valid (docs/handoffs/161/handoff-T-red.md).
  - New backend suite: `internal/translate/service_source_correct_test.go`, 15 tests. It uses a loopback gtx server that records `sl` for single-call cases, and the #84 scripted engine for chunked cases.
  - New frontend suite: `frontend/src/components/detectedSourceNote.test.ts`, 19 tests, source-contract plus an evaluated `fromOptionLabel`.
  - Deleted per the brief: `detectedLang.test.ts` and the `variants.e2e.test.ts` detected-label block.
  - Two existing tests adjusted, because the brief's mandated behaviour changes what they encoded:
    - `TestChunkedTranslateMultiSplitsAndReassemblesInOrder`: chunk 1 of a pinned request is now `auto`.
    - `identityResult.test.ts` `identityBlock`: it now skips the note's `!identity` guard.
  - All four of A's warns are covered: the trimmed boundary, the Auto-under-floor `detected_from == Qualify("es")`, `detected_from == From` with a variant preference, and the frontend condition as non-empty plus `!identity` with no comparison.
- **Assumed:**
  - `DetectedFrom` is read through its JSON key `detected_from` (omitempty), per the Reuse map. The Go file therefore compiles pre-F, and RED fails on assertions.
  - The note's test id is `detected-from-note`, and it sits first in the note stack (wireframe revision 3, as drawn and approved).
  - `fromOptionLabel` stays a named `function` with a `(value: string): string` signature, because the test evaluates it.
- **Hedged:** Satisfiability was proven with a throwaway prototype of the brief's pseudocode, reverted before handoff (`git status` shows test files only). With it, the Go and frontend suites were fully green, the new Go tests passed 3× with `-race`, and 11 targeted mutations were each killed. No real engine was called: google runs against loopback, and chunked cases use the scripted engine. The Apple bridge was not exercised.
- **Evidence:** the authoritative command exits 1 on `internal/translate` only. `pnpm --dir frontend test` gives 13 failed / 383 passed, all 13 in `detectedSourceNote.test.ts`.
