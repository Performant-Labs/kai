# Handoff-S: Phase 9 - #161 source-lang-correct (spec audit)

**Date:** 2026-09-28, 8:21 AM MDT
**Branch:** issue-161-source-lang-correct @ b659b74. The production diff is 096643a..d977ade. Everything after d977ade touches docs only.
**Issue:** #161
**Handoff-T reviewed:** docs/handoffs/161/handoff-T-red.md, docs/handoffs/161/handoff-T-green.md
**Handoff-A reviewed:** docs/handoffs/161/handoff-A.md (plan, PASS), docs/handoffs/161/handoff-A-dup.md (anti-duplication, PASS)
**Handoff-F reviewed:** docs/handoffs/161/handoff-F.md
**Also read:** docs/handoffs/161-brief.md, handoff-D.md, handoff-U.md, decisions.md
**Operator-facing report:** none. This is a headless audit, per the driver prompt and issue #28 (a Wails app can't be driven by synthetic input). No browser, preview or screenshot was opened. The live checks that need a person are listed under "Needs hand confirmation".

## A precondition
Met. handoff-A.md returned PASS (plan re-review at 4258c9f), and handoff-A-dup.md returned PASS (0 blocks, 3 warns) on 096643a..d977ade.

## T precondition
Met. handoff-T-green.md returned PASS with zero blocking issues. U re-ran the authoritative command at 348bb4f and it exited 0: 14 Go packages ok, vitest 396/396. No production file changed after d977ade (`git diff --stat d977ade..HEAD` lists docs only), so that run covers the audited code. S did not re-run Tier 1.

## Visual-diff-tool precondition
Waived by the driver prompt ("Headless audit (issue #28)"). There is no reference render to pixel-diff against. The wireframe is low-fi HTML, and the live surface is WKWebView. U's jsdom mount of the real `TranslateWindow` (13/13) is the headless stand-in. Pixel and theme checks move to the hand-confirmation list.

## Spec compliance

### (a) Pinned-source correction: "Where a pinned request's detection comes from"
I read this against the brief's pseudocode, line by line (`internal/translate/service.go:271-321`, `service_chunk.go:71-142`).

| Brief requirement | Built | OK |
|---|---|---|
| `sent := req`; `req` never mutated | `sent := req`; only `sent.From` is written | YES |
| Gate: not auto, and trimmed text is at least 20 code points | `!isAutoSource(req.From) && utf8.RuneCountInString(strings.TrimSpace(req.Text)) >= minPinCheckRunes`, with `const minPinCheckRunes = 20` | YES |
| `pinFallback = req.From` only when substituting | yes; otherwise the zero value | YES |
| One engine call: `callEngineChunked(..., sent, progress, pinFallback)` | yes; no re-run or second call anywhere | YES |
| `recognized` = `detectedSource` ok AND `model.ParseLanguage` succeeds | yes (:289-292) | YES |
| Identity gated on `sent.From`, `ctx.Err()==nil`, `recognized`, `detected.Covers(req.To)`; built from `sent` | yes (:296-299), `identityResult(engineName, sent, detected)`. Adds one conjunct (Deviation 1, below) | YES + deviation |
| Not substituted: `res.From = resultFrom(req.From, res.From)`; `DetectedFrom = res.From` when auto and recognized, with no floor | yes, byte-for-byte as the pseudocode | YES |
| Substituted mismatch: `res.From = resultFrom(model.Auto, detected)`; `DetectedFrom = res.From` | yes | YES |
| Substituted match, unusable or unrecognized: `res.From = req.From`, `DetectedFrom` empty | yes | YES |
| `callEngineChunked` gets `pinFallback`, with both branches (a SameAs match keeps the pin's dialect; an unrecognized detection falls back to the pin) | yes (:133-141); with `pinFallback == ""` the old path runs unchanged | YES |
| `resultFrom`, `detectedSource`, `identityResult` unmodified | yes; the diff doesn't touch them | YES |

**This is the exact mechanism, not an approximation.** Every pseudocode line is present. The one addition is Deviation 1.

**Deviation 1 (behavioral; accepted by S, and must be surfaced to the principal in the PR body).** F added `!(substitute && detected.SameAs(req.From))` to the identity check in `translateWithEngine`. F added `!(pinFallback != "" && d.SameAs(pinFallback))` to `callEngineChunked`'s chunk-1 identity exit.
- **Why:** under the literal pseudocode, a pinned pt-BR → pt-PT request of 20+ code points becomes "same language — showing the source text". The engine reports a bare `pt`, which `Covers(pt-PT)`, so the engine's translation is thrown away.
- **Conflicts it resolves:** the literal pseudocode contradicts the brief's own AC (a)2 ("if it matches, nothing changed… the translation is what it always would have been"). It also contradicts the #80 rule that a pinned dialect pair is the engine's to translate (`Covers` doc, `model.go`), and it makes the result flip at the 20-code-point floor.
- **Reachable:** both Portuguese dialects are selectable on both sides.
- **The reported bug is unaffected:** in pin es-MX, English text, target en, `en` is not SameAs `es-MX`.
- **Why S accepts it:**
  - It is the minimal change that makes the pseudocode satisfy the higher-order acceptance criterion.
  - It keeps today's behavior for a correct pin.
  - T pinned it with two tests. T removed each conjunct on its own, and each removal failed a test.
  - A-dup reviewed it (W1: a revert must drop both conjuncts and both tests).
- **This is not an ADVISORY-HOLD.** The brief is internally inconsistent here, but its governing criterion, AC (a)2, is unambiguous, and F followed it. The literal pseudocode would ship a user-visible regression.

**Identity/covers-target case (the reported bug's shape).** Pin es-MX, English text, target en. The call is sent as auto. `detected=en` Covers `en`, and the deviation conjunct is false because `en` is not SameAs `es-MX`. So the result is `identityResult(sent, en)`, and `From` = `resultFrom(auto, en)` (qualified, never the wrong pin), with `DetectedFrom` empty. The chunked twin exits at chunk 1 through the same `identityResult`. **Works.** It is covered by `TestPinnedDetectionCoveringTargetIsIdentity` (including a case where a stored preference makes the qualification visible, es → es-MX), `TestChunkedPinnedDetectionCoveringTargetIsIdentity` and `TestTranslateMultiSurfacesCorrectionAndIdentity/identity`.

**DetectedFrom is decided by the backend and has the same shape on both paths.**
- Corrected pin: `res.DetectedFrom = res.From` right after `res.From = s.resultFrom(model.Auto, detected)`.
- Genuine auto: `res.DetectedFrom = res.From` right after `res.From = s.resultFrom(req.From, res.From)` with `req.From` auto. That is the same auto branch of `resultFrom`, and so the same `langPrefs.Qualify`.
- It is never set on identity, on a pin that stood, or when nothing usable was detected.
- The frontend condition is `activeResult.detected_from && !activeResult.identity`. It does no comparison and has no floor.
- `TestDetectedFromSameShapeOnBothPaths` runs the same detection through both paths with an es → es-MX preference and asserts es-MX on both.
- **YES.**

**AC (a)3 (decided once, from chunk 1):** YES. See `TestChunkedPinnedMismatchDecidedFromChunkOne`, `...DialectMatchKeepsPinForLaterChunks`, `...UnusableDetectionFallsBackToPin`, and `TestChunkedGenuineAutoWithoutDetectionKeepsAuto`, which guards the unchanged auto path.

**AC (a)5 (under the floor, unchanged):** YES. The wire shows the pin being sent. The trimmed and code-point (CJK) boundaries are tested at 19 and 20.

**AC (a)6 (never taught back or persisted):** YES. The note's code path and every line that reads `detected_from` name none of `learnLangVariant`, `persistLangs`, `onLangPicked`, `showToast` or `default_from`, and nothing assigns `fromLang`. U's mount (P2/P3) confirms that SaveConfig and Learn are never called and the next request still carries `from: es-MX`.

**AC (a)7 (callers):** YES.
- `Translate` (:212) and `runEngine` (:649) are the only callers of `translateWithEngine`. `runEngine` serves both `TranslateMulti` and `translateAllStream`.
- `ScreenshotRetranslate` (:923) builds `req` with the caller's `from` and reaches `translateAllStream` → `runEngine` → `translateWithEngine`, so it **does get the fix**.
- `TranslateCard.svelte` is not touched, so there is no note on that surface. This gap is documented in handoff-F "Known edges", handoff-T-green, handoff-U and decisions.md. **The PR body still has to state it.** That is O's step, and S can't check it yet.

### (b) Auto landmark / dropdown never changes
- `fromOptionLabel` is now `value === TRANSLATE_LANG.Auto ? t('translate.sourceAuto') : langName(value)`. It reads no result state. YES.
- `detectedFrom` (`$derived(String(activeResult?.from ?? ''))`) is byte-identical and still feeds `swapPair` / `swapLanguages`. Only its comment changed. YES.
- `swapPair` / `swap()` are untouched. YES.
- **`detectedLang.ts` is deleted.** The file is gone, and so is `detectedLang.test.ts`. A grep of `frontend/src` for `detectedLang|detectedSourceLabel` finds nothing outside the sweep test, which builds those names at run time. The three precedent comments (`translateSession.ts`, `langLearn.ts`, `swapLangs.ts`) were repointed to `swapLangs.ts` / `targetCapability.ts`. YES.
- **The stale block in `variants.e2e.test.ts` is deleted:** the `describe('detected label with the real dictionaries')` block and its import are removed, not updated. YES.
- **`translate.detected` is removed from `en-US.ts`, `zh-CN.ts` and the `Dict` type in `keys.ts`.** `grep -E '^\s+detected:'` over `frontend/src/i18n` is empty. YES.
- `translate.sourceAuto` ("Detected" / "自动检测") and `translate.translatedFromDetected` ("Translated from auto-detected {lang}" / "译自自动识别的{lang}") match wireframe revision 3. `lang.auto` is untouched, and `GeneralTab.svelte` still uses it. YES.
- **No toast plumbing.** `showToast` / `u-toast` are not referenced by any added line. `translate.sourceCorrected` and `translate.autoDetected` were never added. YES.
- The binding `models.ts` adds only `detected_from?: Language`. The two unrelated binding drifts in the primary checkout were correctly left out. YES.

### Scope limits
`git diff --stat 096643a..d977ade` is empty for all of these:
- `internal/engine`, `internal/configstore`, `internal/langpref`, `pkg/`
- `internal/translate/chunk.go`, `requests.go`
- `TranslateCard.svelte`, `GeneralTab.svelte`

No confidence score, no second call and no new request type were added. **Held.**

### Key-shaped literals
I scanned every added line in 096643a..HEAD for `sk-…`, `AKIA…`, `AIza…`, `ghp_…`, 32+ hex runs, PEM headers and `api_key = "…"` shapes. **None found.** The loopback fixtures use `httptest` URLs only.

## Quality audit
| Area | Result | Notes |
|------|--------|-------|
| API consistency | PASS | One additive `omitempty` field, a sibling of `Identity` in kind. The binding is regenerated for this delta only. |
| Error handling | PASS | The failure path still returns `nil, err` before any `DetectedFrom` logic. The cancel check (`ctx.Err()`) still comes before identity. |
| UI/UX match to spec | PASS | Matches wireframe rev 3 and handoff-D Q0-Q6 (U, 13/13 mount checks). |
| Accessibility | PASS | The note is a plain `<p>` text node. The select's labels are real text. No new interactive control. |
| Architecture gate | PASS | A plan PASS and A-dup PASS. None of S's findings conflict with A. |
| Code organization | PASS (1 advisory) | A stale comment; see Advisory 1. |
| Security | PASS | No new input surface. `detected_from` is rendered through `langName` as text, never as HTML. No secrets. |
| Performance | PASS | Still one engine call per request, proved on the wire. The added cost is one rune count over the trimmed text. |
| Visual regression | N/A (headless) | See "Needs hand confirmation". |
| Naming consistency | PASS | `DetectedFrom` / `detected_from` / `detectedFrom` are kept distinct, as the brief's WARN 5 fix requires, and the comments name the distinction. |
| Test quality | PASS (2 advisories) | See below. |

### Test-quality audit
- **Proportionality.** The backend has 18 tests (plus subtests) in `service_source_correct_test.go`, and each one maps to a named brief bullet: real mismatch, pin match including the dialect, covers-target, both detection-failure modes, the floor with the trimmed and CJK boundaries, auto with no floor, both entry points, the chunked decisions, and Deviation 1's two tests. On the frontend, one file (`detectedSourceNote.test.ts`) follows the repo's source-contract convention (#28: no render harness). That is proportionate for a story that changes backend behavior plus the frontend contract. There is no coverage padding.
- **Behavior on the wire.** The single-call cases run the real google engine against a loopback gtx server that records `sl` for each request. So "sent as auto", "exactly one call" and "the pin is sent below the floor" are observed facts, not mock expectations. `DetectedFrom` is read through the JSON contract the frontend consumes.
- **Dialect preservation.** `TestChunkedPinnedDialectMatchKeepsPinForLaterChunks`: pin es-MX, chunk 1 reports a bare `es` (recognized, SameAs). It asserts chunks 2..3 are sent `es-MX`, not `es`, and that the result reports the pin with no `detected_from`. This fails for the right reason if the new branch is dropped, because chunks would then go out as `es`. It is distinct from the unrecognized-fallback test. **Good.**
- **Both detection-failure modes.** `TestPinnedNoUsableDetectionFallsBackToPin` has (i) `"auto"` echoed (`detectedSource` returns ok=false), (ii) `"jp"`, which passes `detectedSource` with ok=true and is rejected only by the `ParseLanguage` gate, and (iii) a null slot. The chunked twin covers `jp`, `""` and `auto`. Case (ii) is the one that would falsely "correct" a Japanese pin if the recognition gate were removed. T's RED mutation log confirms that removing the gate is caught. **Good.**
- **Identity case.** It asserts `From == resultFrom(auto, detected)` with a setup guard that the qualification actually differs from the bare detection (es → es-MX), so it can't pass by accident. **Good.**
- **Deviation 1.** It is pinned at both points, and the mutation removing each conjunct was killed (T-green). **Good.**
- **Modified existing tests.** `TestChunkedTranslateMultiSplitsAndReassemblesInOrder` now expects chunk 1 to be sent as auto and chunks 2..N as the pin. That is the intended contract change, and the assertion is not weakened. `identityResult.test.ts`'s regex was narrowed so it keeps finding the positively-read identity block. Its assertions are unchanged. **Legitimate.**
- **Advisory T1 (not blocking).** `TestResultFromBranchesUnchanged` directly tests `resultFrom`, a function this story does not modify. The qualification property it stands for is already proved end to end by the pt → pt-BR mismatch case and by `TestDetectedFromSameShapeOnBothPaths`. It is a low-value duplicate signal and a candidate to delete or merge. The brief asked for it explicitly, so it stays.
- **Advisory T2 (not blocking).** `TestChunkedPinnedMismatchDecidedFromChunkOne` asserts "chunk 1 runs alone" after a fixed `time.Sleep(30ms)`. A slow machine can make it pass falsely, but it can't make it fail falsely. The later assertion that chunks 2..4 were sent `en` is the load-bearing check, and it is deterministic.
- **Frontend.** The source-contract assertions (class list, call regex) are shape-coupled by necessity (#28). They are balanced by `fromOptionLabel` being *evaluated* with stubs, where a reference to result state throws, and by U's jsdom mount.
- **No smells found:** no tautological, snapshot-everything or mock-shaped tests.

## Scope check
F delivered exactly the brief's scope, plus Deviation 1. That is two conjuncts, tested, and justified against AC (a)2. Nothing is under-delivered. The out-of-scope `TranslateCard` note was correctly not built.

## Needs hand confirmation (the principal's live test; headless can't reach these)
1. **Real Apple, wrong pin:** pin es-MX, target zh, paste 20+ code points of French. Expect a real translation (not an echo), the note "Translated from auto-detected French", and the dropdown still on "Spanish (Mexico)". This is the first run of substitute-as-auto on the real Swift bridge; the tests used only loopback and scripted engines.
2. **Reported bug, identity case:** pin es-MX, target en, paste 20+ code points of English. Expect only the identity note (no "Translated from…"), with the dropdown unmoved.
3. **Deviation 1, live:** pin Portuguese (Brazil), target Portuguese (Portugal), paste 20+ code points of Brazilian Portuguese. Expect a real pt-PT translation, no identity note and no detected note.
4. **WKWebView rendering of the note:** muted, 11 px, on one line at 960 and 780 px, in light and dark, and in en-US and zh-CN. Check its spacing both as the first line and above a phonetic line.
5. **Native macOS select:** the Auto entry reads "Detected" / "自动检测", both open and closed. Settings > General's interface-language option still reads "Auto" / "自动".
6. **Under the floor:** a mismatched paste shorter than 20 code points still echoes silently. This is the decided scope; confirm it is acceptable.
7. **Screenshot re-translate with a wrong pin:** the translation is corrected, and `TranslateCard` shows no note. This is the accepted gap.

## Verdict

PASS — every acceptance criterion is met, the implementation complies with the spec, and the quality is acceptable. It is ready for O to commit and open the PR.

Conditions on O's PR body. These are items to carry into the PR text, not code rework:
- State Deviation 1 plainly, as a departure from the literal pseudocode. Include its reason (AC (a)2 and #80's pinned dialect-pair rule) and how to revert it (drop both conjuncts and both tests).
- State the `ScreenshotRetranslate` / `TranslateCard` missing-note gap as a known follow-up (AC (a)7).
- State that #11's Auto-row relabeling is deliberately walked back, per the brief's "Visible-cue mechanism".
- Include the "Needs hand confirmation" list above.

## Advisory notes
1. **A stale comment in production code (A-dup W2, and U also flagged it).** The `<!-- prettier-ignore -->` above the note at `TranslateWindow.svelte` ~:1304-1310 is justified by a comment claiming that the #161 test "reads it without the trailing comma prettier adds". Since T-green's `,?` regex fix, that claim is false.
   - It is cosmetic and doesn't block this verdict.
   - The clean fix is a direct-rigor edit at PR time: delete the directive and the last sentence of the comment, run prettier on the file, then re-run `detectedSourceNote.test.ts`.
   - If O doesn't fold it in, it stays a harmless but inaccurate comment.
2. **Chunk-1 exit has no `ParseLanguage` gate (A-dup W3).** An unrecognized detection that `Covers` the target would take the chunk-1 exit, then fail `translateWithEngine`'s recognized gate and come back as a non-identity "translation" of the source text. This is unreachable from the windows (no target select offers bare `es` or an unrecognized code), and it predates #161 for genuine auto requests. Informational only.
3. Test advisories T1 and T2 above.
