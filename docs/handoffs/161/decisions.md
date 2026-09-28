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

## F, Phase 5 (implement), 2026-09-28
- **Decided:** Implemented the brief's pseudocode (docs/handoffs/161/handoff-F.md).
  - Backend: `sent := req`; the trimmed code-point gate `>= minPinCheckRunes` (20); `pinFallback` threaded into `callEngineChunked` with its two branches (dialect kept, pin as the fallback); the `ParseLanguage` recognition gate; identity re-gated on `sent.From` and built from `sent`; `DetectedFrom` holds the qualified `From` on both paths (Auto: always, no floor; pin: only on `!SameAs`).
  - Frontend: `detectedLang.ts` deleted; `fromOptionLabel` is trivial; the note is `{#if activeResult.detected_from && !activeResult.identity}`, first in the stack; `translate.sourceAuto` and `translate.translatedFromDetected` added and `translate.detected` removed; the three comments repointed; the binding regenerated (the #161 delta only).
- **Decided (deviation 1, for O/principal):** one conjunct at each identity point (`service.go:296-297`, `service_chunk.go:118`), so a checked pin the detection confirms (`SameAs`) never becomes identity.
  - Verbatim, a pinned pt-BR → pt-PT request of 20+ code points became "same language — showing the source text", discarding the engine's pt-PT translation. That breaks AC (a)2 and #80's pinned dialect-pair rule, and it is reachable from the selects.
  - The real-path probe (Google over loopback) shows the flip at the floor. No test pins it either way; T should add one. Revert = drop the two conjuncts.
- **Decided (deviation 2):** `<!-- prettier-ignore -->` on the note's `<p>`. `detectedSourceNote.test.ts:174-176` rejects the trailing comma prettier adds when it wraps the 103-column line. T should allow `,?`; then the directive goes.
- **Assumed:** `archChanged: false`. The field and the parameter are the ones A reviewed; the deviation is behavior, not structure.
- **Hedged:**
  - `svelte-check` doesn't start here (TS 7 without TS 6), so the `.svelte` expressions were type-checked through a temporary `.ts` with negative controls.
  - golangci-lint wasn't run locally.
  - Linux cross-compile needs cgo (Wails GTK).
  - No live provider was called; the WKWebView layout was not seen (headless jsdom only).
- **Evidence:**
  - The authoritative command exits 0 (8:08 AM MDT): 14 Go packages ok, vitest 396/396.
  - The new Go tests pass 3× under `-race`.
  - A jsdom mount of the real `TranslateWindow` passed 10 checks, and 2 component mutations were caught.
  - `tsc` exits 0; gofmt is clean; `go vet` has only the 4 existing diagnostics; prettier is clean on the production files.

## T, Phase 7 (GREEN + Tier 2), 2026-09-28
- **Decided:** GREEN, PASS (docs/handoffs/161/handoff-T-green.md). The authoritative command exits 0 at F's head and on the final tree: 14 Go packages ok, vitest 396/396. The #161 and chunk tests pass `-race -count=3`.
- **Decided:** repaired `detectedSourceNote.test.ts`'s note regex to accept prettier's trailing comma (`,?`), per F's flag. F's `prettier-ignore` is now optional and harmless.
- **Decided:** accepted F's Deviation 1 (a pin its detection confirms, e.g. pt-BR → pt-PT with a detected bare pt, is never identity), because the literal pseudocode violates AC (a)2 and #80's pinned dialect-pair rule. Pinned it with `TestPinnedDialectPairConfirmedByDetectionIsNotIdentity` and its chunked twin. Removing either conjunct on its own is killed. Flagged for O, S and the principal as a behavior departure from the literal pseudocode.
- **Assumed:** the principal keeps Deviation 1. To revert, drop the two conjuncts and the two tests.
- **Hedged:** no live provider was called and no Apple bridge was exercised; the checks were loopback gtx and the scripted engine. No visual check (headless).
- **Evidence:** handoff-T-green.md. Mutation runs showed `calls = map[1:auto]` with the chunk conjunct removed, and both tests failing with the service conjunct removed. Production was restored via `git checkout HEAD --`, and `git status` shows test files only.

## A, Phase 7 (anti-duplication gate), 2026-09-28
- **Decided:** PASS (docs/handoffs/161/handoff-A-dup.md, diff 096643a..d977ade), with 0 blocks, 3 warns and 4 passes.
  - F extended the objects the Reuse map named: `translateWithEngine`, the single seam both `Translate` and the fan-out run use; `callEngineChunked`, through the one `pinFallback` parameter; and one `DetectedFrom` field sibling to `Identity`.
  - `detectedSource`, `resultFrom`, `identityResult`, `ParseLanguage`, `SameAs` and `Covers` are reused unchanged.
  - `detectedLang.ts` is deleted, not left parallel.
  - Scope held: no change in engine, configstore, pkg, `chunk.go` or `requests.go`.
- **Warns:**
  - W1: Deviation 1's conjunct lives in two places, mirroring #84's twin identity points. A revert must drop both, plus both tests.
  - W2: the `prettier-ignore` comment in `TranslateWindow.svelte` is stale since T-green's `,?` regex.
  - W3: the ParseLanguage asymmetry between the two identity checks is unreachable via `Covers`, so it is informational only.
- **Assumed:** the phase prompt's "fifth plan review" text was carried over from Phase 3. The brief and handoff-D are unchanged since the Phase-3 PASS at 4258c9f (empty diff), so that PASS stands, and this run did the Phase-7 gate its phase line names.
- **Hedged:** read from source and the diff only. No tests or engines were run at this gate. T-green's run is the runtime evidence.
- **Evidence:** `service.go:212`, `:649`, `:271-321`; `service_chunk.go:71`, `:118`, `:131-141`; `model.go` `Covers`/`canonical`.

## U, Phase 8 (UI walkthrough), 2026-09-28
- **Decided:** PASS (docs/handoffs/161/handoff-U.md). The UI conforms to wireframe revision 3:
  - The Auto entry always reads "Detected" / "自动检测", and pinned entries are never suffixed.
  - The note shows when `detected_from` is set and `identity` is not. It is 11 px, first in the note stack and outside the textarea.
  - The select never moves, and neither SaveConfig nor Learn is called.
  - Swap after a pinned correction exchanges the pin.
- **Decided:** used a jsdom mount of the real `TranslateWindow` as the headless stand-in for a browser, because this is a Wails app (issue #28) and the rule is headless only. I re-ran F's 10 mount checks at HEAD and added 3 U checks: the stack order with phonetic and cancelled, the note following the active engine, and swap after a correction. 13/13 pass, and the throwaway files were removed, so the tree is clean.
- **Assumed:** the principal's live hand test covers what jsdom can't: the real Apple substitute-as-auto call on a wrong pin, the identity case (pin wrong and text in the target language), and the note's WKWebView rendering at 960 and 780 px, light and dark, in both locales.
- **Hedged:** no live engine, no Apple bridge and no WKWebView. The backend was covered only by T's loopback tests.
- **Evidence:** the authoritative command exits 0 at 348bb4f (8:18 AM MDT): Go ok, vitest 396/396. The mount log `[A1]…[U3]` is in handoff-U.md.

## S, Phase 9 (spec audit), 2026-09-28
- **Decided:** PASS (docs/handoffs/161/handoff-S.md), audited headless on the production diff 096643a..d977ade. Everything after d977ade is docs only.
  - F built the brief's exact mechanism from "Where a pinned request's detection comes from":
    - `sent := req` with `req` never mutated, and the trimmed ≥20-code-point gate.
    - `pinFallback`, with both of its branches.
    - The `ParseLanguage` recognition gate.
    - Identity gated on `sent.From` and built from `sent`.
    - `DetectedFrom` = the qualified `From` on both paths.
  - The reported bug's identity/covers-target case resolves to `identityResult(sent, en)` with `DetectedFrom` empty.
  - `detectedFrom` and `swapPair` are untouched.
  - `detectedLang.ts`, its test and the stale `variants.e2e.test.ts` block are deleted.
  - `translate.detected` is gone from en-US, zh-CN and `keys.ts`.
  - There is no toast plumbing, and the scope limits held (engine, configstore, langpref, pkg, `chunk.go`, `requests.go`, `TranslateCard`, `GeneralTab` untouched).
  - No key-shaped literals were found.
- **Decided:** accepted F's Deviation 1 (a confirmed pin never becomes identity) rather than issue ADVISORY-HOLD.
  - The brief's literal pseudocode contradicts its own AC (a)2 and #80's pinned dialect-pair rule. The deviation is the minimal fix toward the governing criterion.
  - It is tested, with both conjuncts mutation-killed.
- **Assumed:** O's PR body will carry four items: Deviation 1, the `ScreenshotRetranslate`/`TranslateCard` note gap (AC (a)7), the #11 relabel walk-back, and S's hand-confirmation list.
- **Hedged:**
  - S did not re-run Tier 1, and relies on the run U made at 348bb4f.
  - No live engine or Apple bridge was called, and nothing was rendered in WKWebView. Seven cells are listed for the principal's hand test.
- **Advisories (non-blocking):**
  - The `prettier-ignore` comment above the note is stale since T-green's regex fix (A-dup W2).
  - `TestResultFromBranchesUnchanged` tests an unmodified function, a duplicate signal.
  - The sleep-based negative check in `TestChunkedPinnedMismatchDecidedFromChunkOne`.
