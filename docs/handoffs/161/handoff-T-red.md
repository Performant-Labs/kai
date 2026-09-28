# Handoff-T-red: Phase 4 - #161 source-lang-correct (pinned-source auto-correction + "Detected" landmark)

**Date:** 2026-09-28
**Branch:** issue-161-source-lang-correct (base 096643a, head 6e6fdf2)
**Brief / wireframe reviewed:** docs/handoffs/161-brief.md (the pseudocode in "Where a pinned request's detection comes from" governs); docs/handoffs/161/wireframe.html revision 3 and handoff-D.md (approved); handoff-A.md (re-review #4)

## A precondition
Confirmed: A returned **PASS** on the plan (re-review #4, handoff-A.md). A handed four warns to T, and all four are in this suite:
- W1: trimmed-length boundary (`TestPinnedShortTextIsNotSubstituted/whitespace-padded short`).
- W2: an Auto request under the floor gets `detected_from == Qualify("es")` (`TestAutoResultCarriesDetectedFromWithoutFloor`), and a real mismatch gets `detected_from == From` with a variant preference that makes qualified and bare differ (`TestPinnedMismatchIsCorrectedFromOneCall`).
- W3: the render condition is non-empty `detected_from` and `!identity`, with no `fromLang` or comparison (`detectedSourceNote.test.ts`).

## Tests authored

### Backend: `internal/translate/service_source_correct_test.go` (new)
Tier: service-level integration. Single-call cases run the real google engine against a loopback gtx server that records each request's `sl`/`tl`, so "dispatched as auto" and "exactly one call" are checked on the wire. Chunked cases use `service_chunk_test.go`'s scripted engine, as #84's tests do. `DetectedFrom` is read through the JSON contract the frontend consumes (`detected_from`, omitempty). That keeps the file compiling before the field exists, so RED fails on assertions, not on the build.

| Test | Pins |
|---|---|
| `TestPinnedMismatchIsCorrectedFromOneCall` (2 cases) | (a)1-2, (a)4. A pin over the floor is sent as `sl=auto`, with exactly one call. `From == detected_from == resultFrom(Auto, detected)`, the qualified value: pt-BR, not the bare pt, when a pt→pt-BR preference exists. The engine's translation is used as-is. |
| `TestDetectedFromSameShapeOnBothPaths` | (a)4. For the same detection, the substituted-pin path and the genuine-Auto path store the identical qualified `detected_from` and `From` (es-MX). |
| `TestPinnedDetectionMatchingPinIsNotACorrection` (exact, dialect) | (a)2. Pin `fr`/detect `fr`, and pin `es-MX`/detect bare `es` (SameAs): `From` = pin, `detected_from` absent, sent as auto. |
| `TestPinnedDetectionCoveringTargetIsIdentity` (2 cases) | The reported bug: pin es-MX, English text, target en. `Identity`, `Result` = source text, `From == resultFrom(Auto, detected)` and never the wrong pin, `detected_from` absent, one call. Case 2 (fr pin, target es, es-MX preference) makes the qualification visible: `From` = es-MX. |
| `TestPinnedNoUsableDetectionFallsBackToPin` (3 subtests) | Scope and the two gates. (i) `auto` echoed back: `detectedSource` ok=false. (ii) Native `jp` on a `ja` pin: ok=true but fails the `ParseLanguage` gate. (iii) A null slot. All three fall back to the pin, with `detected_from` absent. |
| `TestPinnedShortTextIsNotSubstituted` (3 subtests) | (a)5 guard. Below the floor the wire gets the pin, not auto. The cases are short text, whitespace-padded short text (trimmed gate, A-W1), and 19 CJK code points = 57 bytes (code points, not bytes). |
| `TestPinnedTextAtFloorIsSubstituted` | The boundary is `>= 20`: exactly 20 trimmed code points is substituted and corrected. |
| `TestAutoResultCarriesDetectedFromWithoutFloor` (+3 subtests) | Open decision 3 and A-W2. A 4-rune Auto request gets `detected_from` = es-MX (= From). It is absent for no detection, an unrecognized `jp`, and identity. |
| `TestResultFromBranchesUnchanged` | `resultFrom` is untouched: the pinned branch is verbatim, and the auto branch is `Qualify`. Guard, passes today. |
| `TestTranslateMultiSurfacesCorrectionAndIdentity` | (a)7. The TranslateMulti streamed payload carries the same correction and identity as Translate. |
| `TestChunkedPinnedMismatchDecidedFromChunkOne` | (a)3. Chunk 1 is sent alone as auto. Its detection (en) is sent for every later chunk even though they would detect fr. The result reports en/en. |
| `TestChunkedPinnedDialectMatchKeepsPinForLaterChunks` | The pinFallback dialect branch (fourth-review W2). Pin es-MX, chunk 1 detects bare es: chunks 2..N get **es-MX**, not es. |
| `TestChunkedPinnedUnusableDetectionFallsBackToPin` (jp, "", auto) | The pinFallback fallback branch. Chunks 2..N get the ORIGINAL pin, not auto and not jp. |
| `TestChunkedPinnedDetectionCoveringTargetIsIdentity` | A chunked identity: one call, identity over the whole text, From en. |
| `TestChunkedGenuineAutoWithoutDetectionKeepsAuto` | Regression guard, passes today. A genuine Auto request with no chunk-1 detection keeps re-detecting (chunks stay auto). Its unrecognized-code half is the existing `TestChunkUnrecognizedDetectionIsNotPinned`, which is not duplicated. |

**Modified existing test:** `service_chunk_test.go` `TestChunkedTranslateMultiSplitsAndReassemblesInOrder` asserted every chunk of a pinned `es` request is sent `From=es`. That is exactly the contract #161 changes. Now chunk 1 is `auto` and chunks 2..N are `es` (pinFallback, since the scripted engine reports no detection). It is the only existing Go test the change breaks. Evidence below.

### Frontend: `frontend/src/components/detectedSourceNote.test.ts` (new)
Tier: pure and source-contract (no Svelte render harness, #28), the same convention as `identityResult.test.ts` and `swapWindow.test.ts`.
- **`fromOptionLabel`**: the real function is extracted from the component and **evaluated** with stubs. Auto gives `t('translate.sourceAuto')` and any other value gives `langName(value)`. Any reference to result state (detectedFrom, activeResult, fromLang) raises a ReferenceError, which proves the label is the same in every state. Separately, the function reads no `detectedFrom`, `detected_from`, `fromLang`, `translate.detected` or `lang.auto`. The `<option>` still renders through `fromOptionLabel(l.value)`. GeneralTab keeps `t('lang.auto')`.
- **Deletion sweep**: `utils/detectedLang.ts` and its test don't exist, and no file under `frontend/src` names `detectedLang` or `detectedSourceLabel`. This covers the import and the three stale precedent comments in `translateSession.ts`, `langLearn.ts` and `swapLangs.ts`. The names are assembled at run time. The component uses none of `translate.detected`, `translate.sourceCorrected` or `translate.autoDetected`.
- **`detectedFrom`/`swapPair` regression**: `const detectedFrom = $derived(String(activeResult?.from ?? ''))` is unchanged, and swapPair still passes `detectedFrom,` and never `detected_from`.
- **Note**: an `{#if …}` block whose condition reads `activeResult.detected_from` and `!activeResult.identity`. The condition has no `fromLang`, `toLang`, `detectedFrom`, `TRANSLATE_LANG`, `.length`, `.text`, `===` or `!==`: no comparison and no floor. The body has `data-testid="detected-from-note"` and `t('translate.translatedFromDetected', { lang: langName(activeResult.detected_from) })`, with classes `u-muted px-4 text-[11px] first:pt-4`. It sits in the `pane === 'result'` branch, first in the note stack (before phonetic, per the approved wireframe) and before the `<textarea>`, and it appears exactly once.
- **No-toast / no-teach**: neither the note block nor any line reading `detected_from` names learnLangVariant, LearnLangVariant, learnFromSelection, persistLangs, onLangPicked, showToast or default_from, and none assigns fromLang or toLang. `onLangPicked`, `learnLangVariant`, `persistLangs`, `swap` and `copy` never read `detected_from`, and `activeDisplay` (the copied text) excludes it.
- **i18n**: `translate.sourceAuto` is "Detected" / "自动检测". `translate.translatedFromDetected` is "Translated from auto-detected {lang}" / "译自自动识别的{lang}". `translate.detected` is gone from both catalogs and from `keys.ts`. `lang.auto` is unchanged ("Auto" / "自动"). `keys.test.ts` parity keeps both catalogs in step.

**Deleted, per brief:** `frontend/src/utils/detectedLang.test.ts` (F deletes `detectedLang.ts`), and `variants.e2e.test.ts`'s `describe('detected label with the real dictionaries')` block and its import, not updated.
**Modified existing test:** `identityResult.test.ts`'s `identityBlock` regex matched the first `{#if …identity…}`, which is now the note's `!activeResult.identity` guard. I narrowed it to a positively-read identity condition (`[^}!]*`). Its assertions are unchanged.

## RED confirmation
Command (authoritative, with `NODE_OPTIONS=--no-experimental-webstorage`):
`sh -c "(cd pkg/swiftbridge/scripts && bash ./build.sh) && go test -vet=off ./internal/... ./pkg/... -count=1 && pnpm --dir frontend test"` exits **1**. Every Go package is `ok` except `internal/translate`. The Go stage short-circuits the `&&`, so I also ran `pnpm --dir frontend test` on its own: **13 failed | 383 passed (396)**, and all 13 failures are in `detectedSourceNote.test.ts`.

Go failures, every one an assertion on the feature and none a build or setup error:
- `engine was sent sl="es", want auto (a pinned request over the floor is dispatched as auto)`: every substituted case.
- `From = "es-MX", want "en" (the qualified detection, not the pin "es-MX")`; `detected_from = "", want "pt-BR" (qualified, same value as From)`.
- `Identity = false, want an identity result (detection "en" covers target "en")` (the reported bug).
- `genuine-auto detected_from = "", want es-MX (qualified)`; `detected_from = "", want es-MX (= From; no 20-code-point floor on auto)`.
- Chunked: `2 calls while chunk 1 was running, want chunk 1 alone`; `chunk 1 From = "es-MX", want auto (substituted)`; `chunk 2 From = "es-MX", want en (chunk 1's detection, decided once)`; `engine called 3 times, want exactly 1 (chunk 1 only)`.
- `TestChunkedTranslateMultiSplitsAndReassemblesInOrder`: chunk 1 `From="es"`, want `auto`.

Frontend failures (13): `ReferenceError: detectedSourceLabel is not defined` (fromOptionLabel is still the #11 relabeler); `detectedFrom: expected 'function fromOptionLabel…' not to contain 'detectedFrom'`; the module still exists; sweep hits (6: the import, the three comments, the module and its own header); `'translate.detected'` still used; `no {#if …detected_from…} block` (×4 note tests); `expected undefined to be 'Detected'`; `…'Translated from auto-detected {lang}'`; `'detected' in enT` still true.

Guards that pass today by design, and why they are not vacuous:
- The Go below-floor cases, `TestResultFromBranchesUnchanged` and `TestChunkedGenuineAutoWithoutDetectionKeepsAuto`.
- The frontend option markup, the GeneralTab `lang.auto`, `detectedFrom` and swapPair, `activeDisplay`, and the `lang.auto` values.
- Each guard's mutation is killed (below). The note tests that could have passed vacuously on a missing block now assert that the block exists first.

**Satisfiability and mutation check.** I applied the brief's pseudocode as a throwaway prototype to `service.go`, `service_chunk.go` and `model.go`, plus the frontend change, then **reverted all of it** (`git checkout`, and `git status` shows only test files). With the prototype, the full Go suite and the full frontend suite (396/396) were green. The new Go tests passed 3× under `-race`. Each of these mutations of the prototype was killed by at least one test:

| Mutation | Killed by |
|---|---|
| untrimmed length | `TestPinnedShortTextIsNotSubstituted` |
| byte length | `TestPinnedShortTextIsNotSubstituted` |
| `>` instead of `>=` | `TestPinnedTextAtFloorIsSubstituted` |
| no `ParseLanguage` gate | NoUsable, AutoResult, ChunkedUnusable |
| `identityResult(req…)` | CoveringTarget, TranslateMulti, ChunkedIdentity |
| bare `detected` on `DetectedFrom` | Mismatch, SameShape |
| no Auto-path `DetectedFrom` | SameShape, AutoResult |
| string `!=` instead of `SameAs` | MatchingPin, ChunkedDialect |
| no dialect branch | `TestChunkedPinnedDialectMatchKeepsPinForLaterChunks` |
| no unrecognized fallback | ChunkedUnusable and the updated #84 test |
| fallback also for genuine auto | `TestChunkUnrecognizedDetectionIsNotPinned`, `TestChunkedGenuineAutoWithoutDetectionKeepsAuto` |

## Notes for F
- Implement the brief's pseudocode (:72-186). The trimmed gate `codePointLen(strings.TrimSpace(req.Text)) >= 20` governs, not Reuse row :245.
- Add `DetectedFrom model.Language \`json:"detected_from,omitempty"\`` and regenerate or extend the TS bindings (`frontend/bindings/.../model/models.ts`), so `activeResult.detected_from` type-checks. The vitest suite doesn't type-check, but `svelte-check` does.
- Frontend: the note markup the tests expect is `{#if activeResult.detected_from && !activeResult.identity}<p class="u-muted px-4 text-[11px] first:pt-4" data-testid="detected-from-note">{t('translate.translatedFromDetected', { lang: langName(activeResult.detected_from) })}</p>{/if}`, placed first in the result-branch note stack. `fromOptionLabel` must stay a `function fromOptionLabel(value: string): string` whose body references only `TRANSLATE_LANG`, `t` and `langName`, because the test evaluates it.
- Delete `utils/detectedLang.ts`. Repoint the three comments without naming it (the sweep is a plain substring match). Remove `translate.detected` from en-US, zh-CN and keys.ts.

## Ready for F
RED is valid. F may implement against these tests.
