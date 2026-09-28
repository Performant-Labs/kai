# Handoff-F: Phase 5 - Implement (#161 source-lang-correct)

**Date:** 2026-09-28
**Branch / worktree:** issue-161-source-lang-correct (`.worktrees/0161-source-lang-correct`), head 6b68fc7 plus this change. Staged by explicit path, not committed. No test file touched.
**Contract:** handoff-T-red.md (tests and the behaviors they pin), and the brief's pseudocode in docs/handoffs/161-brief.md, "Where a pinned request's detection comes from".
**Verdict:** done. The authoritative command exits 0 on the final tree (8:08 AM MDT): all 14 Go packages `ok`, and the frontend runs 29 files / 396 tests green. T's RED had 13 Go tests and 13 frontend tests failing.
**Toolchain:** Go 1.27.1, Node v26.7.0 (`NODE_OPTIONS=--no-experimental-webstorage`), pnpm 11.3.0, wails3 v3.0.0-beta.25 (bindings only). Model: Opus 5.5 (`claude-opus-5-5`), in-session, no outside model.

## What I changed

### Backend (Go)
- **`internal/model/model.go:276-282`:** adds `DetectedFrom Language \`json:"detected_from,omitempty"\`` to `TranslateResult`, with an `Identity`-style doc comment (set by the service only).
  - It goes after `Cancelled`, not beside `Identity`, so the aligned field block above stays untouched and the diff is purely additive.
  - `callEngine` builds results field by field, so no engine can set the field.
  - Both fan-outs copy the whole result (`*out.res`), so it reaches every payload with no extra wiring.
- **`internal/translate/service.go`, `translateWithEngine` (:271):** the brief's pseudocode, line for line.
  - `sent := req`. The gate `substitute := !isAutoSource(req.From) && utf8.RuneCountInString(strings.TrimSpace(req.Text)) >= minPinCheckRunes` (:277) measures trimmed text in code points, with `>=`. When it is true, `sent.From = model.Auto` and `pinFallback = req.From`. `req` is never written.
  - The engine call is `callEngineChunked(ctx, reg, engineName, sent, progress, pinFallback)`.
  - `detected, recognized := detectedSource(res, err)`. `recognized` also requires `model.ParseLanguage(string(detected))`.
  - The identity check is re-gated on `sent.From`, requires `recognized`, and passes `sent` to `identityResult`, so `From` is `resultFrom(auto, detected)` and never the wrong pin. It has one extra conjunct: see Deviation 1.
  - **Not substituted:** `res.From = s.resultFrom(req.From, res.From)` as before, plus `res.DetectedFrom = res.From` when the source is auto and the detection is recognized. There is no length floor on this path.
  - **Substituted:** when `recognized && !detected.SameAs(req.From)`, it sets `res.From = s.resultFrom(model.Auto, detected)` and `res.DetectedFrom = res.From`, the same qualified value as on the other path. Otherwise `res.From = req.From` and `DetectedFrom` stays empty.
  - `const minPinCheckRunes = 20` (:329), with the brief's reason: it is the whole threshold, because no engine reports a confidence.
  - Doc comment: branch 2 now names the checked pin, plus one paragraph on the #161 check and one on the recognition gate and `DetectedFrom`.
- **`internal/translate/service_chunk.go`, `callEngineChunked` (:71):**
  - It takes a new last parameter, `pinFallback model.Language`, consulted in the brief's two branches of the chunk-1 `from` decision (:133-141):
    - A recognized detection that is `SameAs` the pin keeps the pin (es-MX, not es).
    - An unrecognized detection falls back to the pin.
  - With `pinFallback == ""` (every request that is not substituted), the old code path runs unchanged.
  - One extra conjunct on the chunk-1 identity exit (:118): see Deviation 1.
  - Untouched: `resultFrom`, `detectedSource`, `identityResult`, the chunker, the request registry, every engine and `pkg/swiftbridge`.

### Frontend
- **`frontend/src/utils/detectedLang.ts`:** deleted (`git rm`). T had already deleted its test and the `variants.e2e.test.ts` block.
- **`TranslateWindow.svelte`:**
  - The `detectedLang` import is gone.
  - `fromOptionLabel` (:538) is now `value === TRANSLATE_LANG.Auto ? t('translate.sourceAuto') : langName(value)`.
  - The `detectedFrom` line (:534) is byte-identical and still feeds `swapPair`. Only its comment changed, since it no longer describes #11's relabel.
  - The new note (:1303-1314) is first in the result branch's note stack, before the phonetic line and the textarea:
    `{#if activeResult.detected_from && !activeResult.identity}` → `<p class="u-muted px-4 text-[11px] first:pt-4" data-testid="detected-from-note">{t('translate.translatedFromDetected', { lang: langName(activeResult.detected_from) })}</p>`
  - The note makes no comparison and never reads `fromLang`, toasts, teaches or persists. The pane comment now lists four notes.
- **i18n:**
  - `translate.detected` is removed from `en-US.ts`, `zh-CN.ts` and `keys.ts`.
  - Added `translate.sourceAuto` ("Detected" / "自动检测") and `translate.translatedFromDetected` ("Translated from auto-detected {lang}" / "译自自动识别的{lang}"), the approved wireframe copy.
  - `lang.auto` is untouched; `GeneralTab.svelte` still reads it.
- **The three stale precedent comments, repointed:**
  - `translateSession.ts:21` → swapLangs.ts / targetCapability.ts.
  - `langLearn.ts:5` → swapLangs.ts, the same injected-binding seam.
  - `swapLangs.ts:19` → targetCapability.ts.
- **`frontend/bindings/.../model/models.ts`:** adds `"detected_from"?: Language` with the Go doc comment. I regenerated it with `wails3 generate bindings -d <scratch> -clean=true -ts -i` and copied this file only. The two comment-only drifts in `service/windowwrapper.ts` and `settings/models.ts` belong to other stories and are left out.

## Deviation 1 (behavioral; O and the principal should see this): a pin the detection confirms never becomes an identity result

**The problem with the pseudocode verbatim.** It breaks the brief's own AC (a)2 for pinned dialect pairs, and the case is reachable from the UI.
- Today, source dialects reach every engine as their base code (`internal/engine/language_capability.go:16-18`). So a pinned **pt-BR → pt-PT** request is sent `pt → pt-PT`, and the engine's European Portuguese translation is shown.
  - That is the #80 rule: a pinned pair is decided by `SameAs`.
  - `Covers`' own doc (`model.go:132-135`) says "a dialect pair is left to the engine, as it is for a pinned source".
  - `TestPinnedSourceIgnoresReportedDetection` pins this, but only for short text.
- Under the verbatim pseudocode, the same request at 20+ code points goes out as auto. The engine reports a bare `pt`, `pt.Covers(pt-PT)` is true, and the identity branch discards the engine's translation. The user sees "Source and target language are the same — showing the source text".
- The pin was right (`pt.SameAs(pt-BR)`), so this breaks AC (a)2: "if it matches, nothing changed from the user's point of view — the pin was right, the translation is what it always would have been, no note".
- Portuguese (Brazil) and Portuguese (Portugal) are both selectable on both sides, so users can hit this.

**What I did.** I added one conjunct at each of the two identity decision points, so a checked pin that the detection confirms stays under the pinned rule:
- `service.go:296-297`: `... && detected.Covers(req.To) && !(substitute && detected.SameAs(req.From))`.
- `service_chunk.go:118`: `... && d.Covers(req.To) && !(pinFallback != "" && d.SameAs(pinFallback))`.

Both are needed. With only the first, the chunked early exit hands back the source text, and `translateWithEngine` would then report it as a translation. Everything else is the pseudocode. This does not affect the reported bug (pin es-MX, English text, target en), because `en` is not the pin's language, and every T test passes either way.

**Evidence, on the real path.** A throwaway `main` package (now deleted) ran the real Google engine against a loopback gtx server, which answered `[EUROPEAN-PT]` with the given detection, through the exported `translate.NewService(...).Translate`. For the verbatim column, I swapped the two production files to the pseudocode and then restored them (sha256 verified).

| Case | With the guard (this change) | Pseudocode verbatim |
|---|---|---|
| A. pt-BR → pt-PT, Portuguese, 3 sentences (1 call) | sent `auto>pt-PT` once, identity false, From pt-BR, the engine's result | identity **true**, From pt, result is **the source text** |
| B. The same with 60 paragraphs (chunked) | `auto>pt-PT` once then `pt>pt-PT` 7 times, identity false, From pt-BR, the joined translations | **1 call**, identity **true**, the source text |
| C. es-MX → en, English text (the reported bug) | identity true, From en | the same |
| D. pt-BR → pt-PT, English text | corrected: From en, detected_from en, the engine's translation | the same |
| E. pt-BR → pt-PT, 14 code points (under the floor) | sent `pt>pt-PT`, the translation, From pt-BR | the same |

Case E shows the inconsistency the guard removes: verbatim, the same pair flips between a translation and "same language" depending only on the text's length.

**To revert,** if the principal prefers the literal pseudocode: delete the two conjuncts. No test changes either way. **No test pins this behavior:** T's prototype was the verbatim pseudocode and was green, and so is this code. So T should add a test (item 2 under "Tests I think are wrong").

## Deviation 2 (formatting only): `<!-- prettier-ignore -->` on the note's `<p>`
- The note's call line is 103 columns, and the repo's `printWidth` is 100.
- Prettier wraps it as `t('translate.translatedFromDetected', {⏎ lang: langName(activeResult.detected_from),⏎ })`, with a trailing comma (`trailingComma: "all"`). The regex at `detectedSourceNote.test.ts:174-176` (`\)\s*\}\s*\)`) rejects that trailing comma.
- Left unformatted, the file fails `prettier --check`, and the next `pnpm format` (or `make format`, which push.yml runs before `make check`) breaks T's test.
- With the directive, the file is prettier-clean and stable (prettier's output is byte-identical to the source) and the regex passes.
- The directive can go once T's regex allows the comma (item 1 under "Tests I think are wrong").

## Reuse / extend-vs-new
- **Extended**, as the Reuse map names them:
  - `translateWithEngine` (the one seam).
  - `callEngineChunked`'s chunk-1 decision (one parameter).
  - `TranslateResult` (one field of the same kind as `Identity`).
  - `fromOptionLabel` (now trivial).
  - The pane's muted note stack: the existing `u-muted px-4 first:pt-4` pattern, at the `text-[11px]` size "Cancelled" uses.
  - The i18n catalogs.
- **Reused unchanged:**
  - `detectedSource` and `identityResult`.
  - `resultFrom`: a corrected pin calls it as `resultFrom(model.Auto, detected)`, which takes its existing auto branch to `langPrefs.Qualify`.
  - `SameAs`, `Covers` and `ParseLanguage`.
  - `detectedFrom` and `swapPair`.
- **New:** only the `minPinCheckRunes` constant.
- **Deleted:** `detectedLang.ts`.
- **Not built:** a toast, a relabel, any assignment to `fromLang`, a second engine call, or a confidence score.

## Self-check (Tier 1)
- **Authoritative command**, exact, with `NODE_OPTIONS=--no-experimental-webstorage`, final tree, 8:08 AM MDT: **exit 0**.
  - Go, all 14 packages `ok`: configstore, engine, engine/enginelimits, historystore, httplogstore, i18n, langpref, model, network, service, settings, translate, pkg/swiftbridge, pkg/wails-updater-providers.
  - vitest: 29 files, **396/396**.
- **Race detector:** the 15 new Go tests, plus `TestChunkedTranslateMultiSplitsAndReassemblesInOrder` and `TestChunkUnrecognizedDetectionIsNotPinned`, pass 3 times under `-race`.
- **Go static checks:**
  - `gofmt -l internal/ pkg/` is empty.
  - `go vet ./internal/translate ./internal/model` reports 4 diagnostics, all the existing `non-constant format string in call to fmt.Errorf` class (service.go :716, :800, :822, :928), which `.golangci.yml` excludes. None is on a changed line.
  - golangci-lint is not installed locally; the fleet CI runs it.
- **Frontend type-check:**
  - `pnpm tsc` exits 0.
  - `svelte-check` does not start in this environment: it wants TypeScript 6 installed alongside 7, plus `--tsgo`. This change did not cause that.
  - So I type-checked the new `.svelte` expressions through a temporary `.ts` copy, since deleted. tsc exits 0, and both `@ts-expect-error` negative controls hold. Against the old binding, tsc rejects `activeResult.detected_from` (TS2339), so the check is live.
- **prettier:** `prettier --check` is clean on every changed production file. The whole-tree `format:check` flags 13 files, all of them `*.test.ts`.
- **`pnpm build:dev`:** 196 modules.
  - The one warning is the existing a11y one on the pane divider (:1152), which this change does not touch.
  - `text-[11px]`, `first:pt-4` and `u-muted` are in the CSS bundle, and `detected-from-note` is in the translate chunk.
  - `frontend/dist` was removed afterwards; it did not exist before.
- **Cross-compile:**
  - Windows: the build and the test binary (`-vet=off`) of translate and model succeed.
  - darwin/arm64: `./internal/... ./pkg/...` builds.
  - Linux with `CGO_ENABLED=0` fails inside Wails' own GTK package (`undefined: pointer`), which needs cgo. That is environmental and happens on master too.
- **Real render, headless.** A jsdom mount of the real `TranslateWindow`, with only the Wails runtime and the bindings mocked, ran 10 checks. The harness is in the scratchpad at `161-mount/`, not in the repo.
  - **A1, before any result:** the From select reads `Detected, English, Spanish (Mexico), Portuguese (Brazil), Portuguese (Portugal), Chinese`, its value is auto, and there is no note.
  - **A2, auto result with `detected_from: es-MX`:**
    - The note reads "Translated from auto-detected Spanish (Mexico)", has exactly the expected classes, and sits before the textarea.
    - The select is still on auto and reads "Detected". No option contains "detected".
    - SaveConfig and Learn are never called.
  - **A3, Copy:** the clipboard gets exactly the result text.
  - **A4, zh-CN:** the note reads "译自自动识别的西班牙语（墨西哥）" and the Auto entry reads "自动检测".
  - **A5, identity result, even with `detected_from` set:** only the identity note shows.
  - **A6, no detection:** no note.
  - **P1-P4, pinned es-MX:**
    - The select shows "Spanish (Mexico)" throughout.
    - A corrected result (`detected_from: en`) shows "Translated from auto-detected English", the SaveConfig count doesn't change, and Learn is never called.
    - The next Translate is sent with `from: es-MX`.
    - A pin that stood shows no note.
  - **Negative controls:** a note that ignores `identity` fails A5. An Auto entry that isn't relabeled fails A1, A2, A4, A6, P1 and P2. The component was restored byte-for-byte (sha256).
- **Real backend path:** the probe under Deviation 1.
- **RED:** F did not re-run the full RED. A baseline of `internal/translate` alone showed 13 failures, matching T's handoff.

## Tests I think are wrong (for T)
1. **`detectedSourceNote.test.ts:174-176` (the note's call regex) rejects prettier's own formatting of the markup it requires.**
   - The line is over `printWidth`, and prettier's wrapped form ends with `langName(activeResult.detected_from),⏎ })`.
   - Suggested fix: allow an optional trailing comma, `...detected_from\s*\)\s*,?\s*\}\s*\)`.
   - After that, the `<!-- prettier-ignore -->` from Deviation 2 can be dropped and `pnpm format` applied.
2. **Gap: nothing tests Deviation 1's pinned dialect pair.** Suggested tests, in the style of `service_source_correct_test.go`:
   - **Single call:** pin pt-BR, target pt-PT, 20+ code points, gtx detects `pt`. Expect one call, sent `auto`; identity false; `From` pt-BR; `detected_from` absent; `Result` is the engine's translation.
   - **Chunked twin, with the scripted engine:** chunk 1 reports pt. Expect chunks 2..N sent as pt-BR, no identity result, and more than one call.
   - Each test catches the removal of its matching conjunct; the probe above confirmed both.
3. **Not wrong, just a note:** `TestPinnedSourceIgnoresReportedDetection` (`service_identity_test.go:412`) uses a 10-code-point text, so it only guards the path without substitution. Item 2's single-call case is its sibling above the floor.

## Known edges (no change made)
- **Identity check vs. the chunk-1 exit.** The brief puts the `recognized` gate on `translateWithEngine`'s identity check but not on `callEngineChunked`'s chunk-1 early exit, whose block it keeps "otherwise untouched".
  - The two can disagree only for an unrecognized detection that `Covers` the target: `es-419` against target `es`, or an unrecognized target.
  - The target selects offer neither `es` nor any unrecognized code, so the windows can't reach it.
- **`ScreenshotRetranslate`.** Its payloads now carry `detected_from` (they copy the whole struct), but `TranslateCard` shows no note. This is the gap the brief accepts; the PR body should mention it.
- **Short pastes.** A mismatched paste under 20 code points is still sent with the pin. That is the brief's decision (open question 6 in handoff-D.md).

## Architecture
`archChanged: false`.
- The only interface changes are the ones A reviewed in the plan:
  - The additive `TranslateResult.DetectedFrom` field, and its generated binding.
  - `callEngineChunked`'s unexported `pinFallback` parameter.
- No module boundary, dependency direction or public function signature changed beyond those.
- Deviation 1 is two conjuncts inside the reviewed seam. It is a behavior question for S and the principal, not a structural change.

## Ready for T(green)
Implemented against the RED contract. The authoritative command exits 0 locally; the plugin's own run is the verdict.

## Files changed (staged by explicit path)
- **Production:**
  - `internal/model/model.go`, `internal/translate/service.go`, `internal/translate/service_chunk.go`
  - `frontend/src/components/TranslateWindow.svelte`
  - `frontend/src/i18n/{en-US,zh-CN,keys}.ts`
  - `frontend/src/utils/{langLearn,swapLangs,translateSession}.ts`, and `frontend/src/utils/detectedLang.ts` (deleted)
  - `frontend/bindings/cnb.cool/dtapp/kai/internal/model/models.ts`
- **Docs:** this file, and one entry in `docs/handoffs/161/decisions.md`.
- No test file is touched.
