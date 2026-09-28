# Handoff-A-dup: Phase 7 - #161 source-lang-correct  (anti-duplication gate)

**Date:** 2026-09-28
**Branch:** issue-161-source-lang-correct
**Diff base:** 096643a   **Diff head:** d977ade
**Reuse map:** docs/handoffs/161-brief.md, the Reuse map table (:243-252) and the pseudocode (:72-186)
**Verdict:** PASS

## Summary

PASS. F extended the objects the Reuse map named, and built no parallel path.

- The correction lives in `translateWithEngine`, the single seam that both `Translate` (`service.go:212`) and the fan-out run (`service.go:649`) go through.
- Chunk 1's decision is extended in place in `callEngineChunked`, through the one named `pinFallback` parameter.
- The result carries one new field, `DetectedFrom`, sibling to `Identity`.
- `detectedSource`, `resultFrom`, `identityResult`, `model.ParseLanguage`, `SameAs` and `Covers` are reused unchanged. No second detector, no second engine call, and no new comparison helper.
- On the frontend, the note joins the existing muted note stack. The old parallel labeler, `detectedLang.ts` and its test, is deleted, not left beside the new path.

Scope held: `git diff 096643a HEAD` touches nothing in `internal/engine`, `internal/configstore`, `pkg`, `chunk.go` or `requests.go`.

This gate is not the "fifth plan review" the phase prompt describes. That prompt text was carried over from the Phase-3 loop. The Phase-3 loop already closed PASS at re-review #4 (handoff-A.md, head 4258c9f), and `git diff 4258c9f HEAD -- docs/handoffs/161-brief.md docs/handoffs/161/handoff-D.md` is empty. So the prose-to-code fixes that review confirmed still stand verbatim: `ok=false` and the native-code split at :19, :200, :246 and :274; the single `DetectedFrom` contract; the `TrimSpace` gate; both `pinFallback` branches; and `handoff-D.md:50`. The code matches that contract: `service.go:277`, `:287-290`, `:305-318`.

## Findings

| # | Severity | File:line | Finding | Suggested fix |
|---|---|---|---|---|
| W1 | warn | `internal/translate/service.go:296-297`, `internal/translate/service_chunk.go:118` | F's Deviation 1 ("a checked pin its detection confirms is never identity") is written as two copies of the same conjunct, one at each existing identity point. This follows #84's established twin structure, where chunk 1 decides early identity and `translateWithEngine` decides it again. So it is not a parallel path. But the rule now lives in two places with two spellings: `substitute && detected.SameAs(req.From)` and `pinFallback != "" && d.SameAs(pinFallback)`. T-green pins both, and removing either one alone is killed. | None required for this gate. If the principal reverts Deviation 1, both conjuncts and both tests (`TestPinnedDialectPairConfirmedByDetectionIsNotIdentity` and its chunked twin) must go together. O should state this in the PR body. |
| W2 | warn | `frontend/src/components/TranslateWindow.svelte:1305-1309` | The comment justifying `<!-- prettier-ignore -->` says the source-contract test reads the line "without the trailing comma prettier adds". T-green changed that regex to accept `,?`, so the justification is now stale. The directive is harmless but unexplained. | Drop the directive and its last two comment sentences, then let prettier wrap the line. Cosmetic, and it doesn't need an A re-run. |
| W3 | warn | `internal/translate/service.go:295-296` vs `service_chunk.go:118` | The Auto-path identity check now also requires `recognized` (the `ParseLanguage` gate), but chunk 1's early identity check does not. The two decisions differ only in theory. `Covers` on an unrecognized native code (`jp`) against a recognized target (`ja`) is false (`model.go` `canonical`/`Covers`), and targets come only from recognized selects, so no reachable input behaves differently. | None. Informational only. |
| P1 | pass | `service.go:271-321` | The single seam is extended in place. `sent := req` keeps `req` unmutated. The trimmed code-point gate uses a named constant, `minPinCheckRunes`, placed next to the function in the file's existing style. `resultFrom(model.Auto, detected)` is reused for the corrected `From`, so both paths store the same qualified value in `DetectedFrom`. | None. |
| P2 | pass | `service_chunk.go:71`, `:131-141` | `pinFallback` is threaded as one trailing parameter. The chunk-1 `from` decision extends the existing `ParseLanguage` branch with the two named cases: dialect kept, or the pin as fallback. When `pinFallback == ""` the path is byte-for-byte the prior behavior. | None. |
| P3 | pass | `internal/model/model.go:276-282`, bindings `models.ts:349-357` | One field, doc-commented in the `Identity`/`Cancelled` style, `omitempty`. The binding carries the #161 delta only. | None. |
| P4 | pass | `TranslateWindow.svelte:532-540`, `:1303-1314`; i18n | `detectedFrom` still feeds the swap, so it is not duplicated by the note, which reads the separate `detected_from`. There is no frontend comparison against `fromLang`. The i18n keys follow the existing `translate.*` naming: `detected` is removed, and `sourceAuto` and `translatedFromDetected` are added in `keys.ts`, `en-US.ts` and `zh-CN.ts`. The three comments that cited `detectedLang.ts` now point at live siblings. | None. |

No duplication; extension is clean.

## Notes for F

None. This is not a BLOCK.

## Notes for O

- **This PASS clears the gate for U and S.** The verdict rests only on the diff 096643a..d977ade. A repeat run on the same head returns this same PASS.
- **Deviation 1 is a behavior call, not an architecture one.** It is the pinned pt-BR → pt-PT case: a checked pin whose detection confirms it is never identity. Its placement is architecturally sound (W1). Whether to keep it belongs to S and the principal. Put the revert recipe from W1 in the PR body.
- W2 is the only edit worth making before merge. It is a comment and directive cleanup in one `.svelte` block, and it does not need A to re-run.
- If the workflow harness keeps sending the Phase-3 "fifth review" prompt text to later A phases, fix the prompt template. This run's prompt asked for a plan re-review that had already closed.

## Patterns referenced

- `internal/translate/service.go:212`, `:271-321`, `:337-345` (`detectedSource`), `:354-363` (`identityResult`), `:447-452` (`resultFrom`)
- `internal/translate/service_chunk.go:52-71`, `:112-141`
- `internal/model/model.go` `SameAs`, `Covers`, `canonical`, `ParseLanguage`
- `docs/handoffs/161/handoff-F.md` §Reuse / extend-vs-new (:90-)
- `docs/handoffs/161/handoff-A.md` (Phase-3 PASS, re-review #4)
