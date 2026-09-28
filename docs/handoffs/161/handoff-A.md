# Handoff-A: Phase 3 - #161 source-lang-correct  (up-front plan review, RE-REVIEW #4)

**Date:** 2026-09-28
**Branch:** issue-161-source-lang-correct (base 096643a, head 4258c9f)
**Brief reviewed:** docs/handoffs/161-brief.md at 4258c9f (299 lines; line numbers below are the brief's own)   **Prior review:** this file at 1b48e41 (BLOCK, B1-B2 residual, W1-W4)   **Wireframe:** docs/handoffs/161/wireframe.html (revision 3, approved)
**Verdict:** PASS

## Summary

PASS. Both prior blocks were in the prose around an already-correct pseudocode, and both are now fixed in the prose itself, not just the code block. I checked each one by grepping the whole brief, as the prior Notes for O asked:

- `ok=false`, `native code`, `already excludes`, `already discards`: every hit now puts native-code exclusion on the `ParseLanguage` gate, never on `detectedSource` (:19, :200, :246, :262, :274).
- `DetectedFrom`, `detected_from`, `floor`: (a)4, the pseudocode, the Reuse map row and open decision 3 now state one contract. No hit says "empty ... non-substituted" or applies the floor to the note.

Commit 4258c9f touches only `161-brief.md` and `handoff-D.md`. No code changed on the branch.

Four small warns remain. All are test anchors or a Reuse-row echo. None contradicts the contract, and T can absorb them.

## Re-review checklist (against handoff-A @ 1b48e41)

| Prior | Item | Status now | Evidence |
|---|---|---|---|
| B1 | False claim that `detectedSource` returns `ok=false` for a native code | **Fixed** | :19 now says `ok=false` only for the auto echo, and that a native code (`jp`) comes back `ok=true` and is caught by the separate `ParseLanguage` gate. :200 says the same. Reuse row :246 names "`detectedSource` + a new `model.ParseLanguage` recognition gate at the call site". Test :274 splits into (i) auto echo, `ok=false`, and (ii) native `jp`, `ok=true` but `recognized=false`. Checked against the code: `service.go:279-287` returns `(res.From, true)` for any non-auto `From`. |
| B2 | One `DetectedFrom` contract, one value shape | **Fixed** | (a)4 :53, Reuse row :250, pseudocode :140-145 and :149-161, and open decision 3 :298 all state the same rule. On a genuine Auto result, recognized and non-identity, the field is always set with no floor. On a substituted pin it is set only when `!SameAs`. On both paths it holds the qualified `From` (`resultFrom(model.Auto, detected)`). It is empty on identity, a matched pin, and an unusable or unrecognized detection. :155 now assigns `res.DetectedFrom = res.From` after the qualifying `resultFrom` at :150, which mirrors :141-143. :219 no longer floor-gates the note. On the Auto path, `resultFrom(auto, x)` is `Qualify(x)` (`service.go:389-393`), so both paths store the same shape. |
| W1 | `pinFallback` Reuse row names both branches | **Fixed** | :247 names the dialect-preservation branch (recognized match) and the unrecognized-fallback branch, consistent with :175-185. |
| W2 | Test anchors | **Fixed** | :273 asserts `From == Qualify("en")` on the identity bug shape. :276 adds the dialect-preservation case (pin `es-MX`, chunk 1 detects `es`, chunks 2..N get `es-MX`). |
| W3 | Gate measures trimmed text | **Pseudocode fixed; Reuse row not** | :78 now has `codePointLen(strings.TrimSpace(req.Text)) >= 20`, which matches :198. Reuse row :245 still says `codePointLen(req.Text) >= 20`. See W1 below. |
| W4 | No frontend FROM comparison | **Fixed in handoff-D; brief test bullet partial** | `handoff-D.md:50` now reads "non-empty and `identity` is false. No frontend comparison against the displayed FROM value". The brief's frontend test bullet :285 checks non-empty/empty, but it doesn't name `!identity` and doesn't forbid a `fromLang` comparison. See W3 below. |

## Findings

| # | Severity | Plan element | Drift dimension | Finding | Suggested fix |
|---|---|---|---|---|---|
| W1 | warn | Reuse row :245 | contract shape | The row still states the gate as `codePointLen(req.Text) >= 20`, untrimmed. The pseudocode :78 and the rule :198 use the trimmed length. The row defers to the pseudocode ("see ... for the exact algorithm"), so F has a correct source. But this is the row F checks first. | F implements :78, the trimmed version. T adds one boundary case: a pinned request of about 15 runes padded with whitespace past 20 must NOT be substituted, so the fixture receives the pin. |
| W2 | warn | Backend tests :271-278 | contract shape | Two test anchors the prior B2 fix asked for are missing. (1) No backend test covers the genuine-Auto half of the contract: an Auto request under 20 code points whose fixture detects `es` must return `DetectedFrom == Qualify("es")`. Without it, nothing locks open decision 3. (2) The real-mismatch test :271 says "`DetectedFrom` set to it", but it doesn't assert `DetectedFrom == From`, which is the value-shape guarantee (qualified, not bare). The contract text itself is now unambiguous, and T owns coverage, so this does not block. | T adds both: the Auto-under-floor `DetectedFrom == Qualify("es")` test, and `DetectedFrom == From` on :271, with the langPrefs store holding a variant preference such as `es` → `es-MX` so the two shapes would actually differ. |
| W3 | warn | Frontend test :285, prose :219 and :221 | layering | :285 tests non-empty and empty, but it doesn't name the `!identity` condition from D's render rule. The backend already leaves the field empty on identity, so the condition is belt-and-braces. The Visible-cue prose (:219 "differs from what the FROM dropdown displays", :221) describes the user-visible effect in comparison terms. It could still be read as a frontend comparison. (a)4 and `handoff-D.md:50` both govern and both forbid one. | T writes the render condition as `detected_from` non-empty and `!identity`. Add a source-contract assertion that the note's condition does not reference `fromLang`. |
| W4 | warn | (a)2 :51 | contract shape | "`DetectedFrom` is set to what was actually detected" loosely implies the bare detection. (a)4 :53, two lines below, pins the qualified shape explicitly and governs. | None required. (a)4 is the contract. |
| P1 | pass | Mechanism (pseudocode :72-186) | contract shape | Unchanged from the prior pass, and still correct against the code. There is one engine call. `req` is never mutated. Identity is gated on `sent.From` and receives `sent`, so `From` is qualified. The `ParseLanguage` gate runs before any correction. `c.from` carries chunk 1's raw detection back to the post-call decision (`service_chunk.go:183-186`). The three-way chunk-1 `from` decision leaves a genuine Auto request bit-for-bit unchanged, because `pinFallback == ""`. | None. |
| P2 | pass | Prose-to-code consistency | contract shape | Every prose statement of the `detectedSource`/gate split and of the `DetectedFrom` contract now matches the pseudocode (:19, :53, :200, :219, :246, :247, :250, :274, :298). The only remaining echo drift is W1 (:245). | None. |
| P3 | pass | Scope limits | scope | Held. `git diff 096643a HEAD --stat` shows docs only. The plan changes nothing in `internal/engine`, `configstore`, `chunk.go`/`Split`/`chunkTarget`/reassembly, or `requests.go`. The extension points are the `pinFallback` parameter and its chunk-1 `from` decision (`service_chunk.go:103-128`), plus one `DetectedFrom` field sibling to `Identity`. `resultFrom` and `detectedSource` are unmodified. | None. |
| P4 | pass | Frontend | dependency / duplication | Unchanged. `detectedFrom` still feeds `swapPair`. The note reads the separate `detected_from`. `detectedLang.ts` is deleted. `translate.sourceAuto` leaves `lang.auto` for `GeneralTab.svelte:79`. | None. |

## Notes for O

- **This PASS closes the Phase-3 loop, and the brief is ready for T.** A repeat pass on this unchanged brief returns this same PASS. No further brief edit is required to proceed.
- **Hand W1-W3 to T as explicit RED-suite items. Don't reopen the brief for them.**
  - The trimmed-whitespace boundary case (W1).
  - The Auto-under-floor `DetectedFrom == Qualify("es")` test (W2).
  - `DetectedFrom == From` on the real-mismatch test, with a variant preference set (W2).
  - The frontend render condition as non-empty and `!identity` with no `fromLang` reference (W3).
- **F implements the pseudocode at :72-186, not the Reuse-row paraphrases.** Where they differ, the pseudocode governs. The only live difference is :245's untrimmed length (W1).
- The optional one-line edit to :245 (`codePointLen(strings.TrimSpace(req.Text)) >= 20`) is cosmetic. If O folds it into T's commit, there's no need to re-run A.

## Patterns referenced

- `internal/translate/service.go:279-287` (`detectedSource`: `ok=true` for any non-auto `From`), `:389-393` (`resultFrom`: auto branch → `s.langPrefs.Qualify`)
- `internal/translate/service_chunk.go:66`, `:103-128`, `:183-186`
- `internal/model/model.go:113` (`SameAs`), `:136` (`Covers`), `:174-185` (`ParseLanguage`)
- `docs/handoffs/161/handoff-D.md:50` (corrected render rule)
