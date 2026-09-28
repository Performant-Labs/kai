# Handoff-T-green: Phase 7 - #161 source-lang-correct (verify GREEN + Tier 2)

**Date:** 2026-09-28, 8:14 AM MDT
**Branch / head:** issue-161-source-lang-correct @ 417ca4e (F's implementation), plus T's test-only changes below (staged, not committed)
**Verdict:** PASS. The suite is GREEN and no Tier 2 issue requires a change to F's production code.

## GREEN
Authoritative command, exact, with `NODE_OPTIONS=--no-experimental-webstorage`:
`sh -c "(cd pkg/swiftbridge/scripts && bash ./build.sh) && go test -vet=off ./internal/... ./pkg/... -count=1 && pnpm --dir frontend test"`
- At F's head, before any T change: exit 0.
- After T's repairs and additions (final tree): **exit 0**.
  - All 14 Go packages are `ok`.
  - vitest: 29 files, 396/396.
- `-race -count=3` over every #161 and chunk test in `internal/translate` (`Pinned|DetectedFrom|ResultFromBranches|TranslateMultiSurfaces|Chunk`): ok.
- The RED failures from handoff-T-red.md (13 Go, 13 frontend) are all green now. No test was weakened to get there.

## Test changes in this phase (F's "Tests that look wrong")
1. **`frontend/src/components/detectedSourceNote.test.ts`: the note's call regex now allows prettier's trailing comma** (`...detected_from\s*\)\s*,?\s*\}\s*\)`). F was right: the old regex rejected prettier's own wrapped form of the required markup.
   - Checked with node: it matches the one-line form and prettier's wrapped form (`lang: langName(activeResult.detected_from),⏎})`), and it still rejects a malformed `,,`.
   - F's `<!-- prettier-ignore -->` on the note is still valid and harmless. It is no longer needed, so the next `pnpm format` pass may drop it. It does not block anything.
2. **Gap closed: F's Deviation 1 is now pinned.** Two new tests in `internal/translate/service_source_correct_test.go`:
   - `TestPinnedDialectPairConfirmedByDetectionIsNotIdentity`
     - Setup: pin pt-BR, target pt-PT, Portuguese text over the floor, and gtx reports a bare `pt`.
     - Expected: exactly one call, sent auto; not identity; `Result` is the engine's translation; `From` is pt-BR; `detected_from` is absent.
   - `TestChunkedPinnedDialectPairConfirmedByDetectionIsNotIdentity`
     - Setup: the chunked twin. Chunk 1 detects `pt`.
     - Expected: all 3 chunks translated; chunk 1 sent auto and chunks 2..3 sent pt-BR; not identity; the reassembled result; `From` is pt-BR; `detected_from` is absent.
   - **Mutation check.** Each conjunct was removed on its own, and each removal was killed:
     - Dropping `service.go`'s `!(substitute && detected.SameAs(req.From))` fails both tests.
     - Dropping `service_chunk.go`'s `!(pinFallback != "" && d.SameAs(pinFallback))` fails the chunked test (`calls = map[1:auto]`).
     - Production was restored from git (`git checkout HEAD --`), and `git status` shows only test files modified.
3. No other test was changed.

## Tier 2 review of F's production diff
- **Pseudocode conformance.** `service.go` and `service_chunk.go` match the brief:
  - `sent := req`, and `req` is never written.
  - The gate is trimmed code points `>= 20`.
  - `pinFallback` has both branches: a dialect match keeps the pin, and an unusable detection falls back to the pin.
  - The `ParseLanguage` recognition gate is there.
  - Identity is built from `sent`, so `From` is `resultFrom(Auto, detected)`.
  - `DetectedFrom` holds the qualified `From` on both paths. The auto path has no floor, and the pin path sets it only on `!SameAs`.
  - `resultFrom`, `detectedSource`, `identityResult`, the engines and swiftbridge are untouched.
- **Deviation 1: accepted on the merits, and flagged for O, S and the principal.**
  - It departs from the literal pseudocode. The literal version turns a pinned pt-BR → pt-PT request of 20+ code points into "same language, showing the source text", because the bare `pt` detection Covers pt-PT.
  - That breaks the brief's own AC (a)2, where a pin the text confirms changes nothing. It also breaks the #80 rule that a dialect pair under a pin is the engine's to translate (`Covers` doc, `model.go`). And it makes the result flip at the length floor.
  - I verified `SameAs(pt-BR, pt-PT)` is false, so the request does reach the engine, and the case is reachable from the selects.
  - The reported bug (es-MX pin, English text, target en) is unaffected: `en` is not SameAs the pin.
  - It is now pinned by the two tests above. To revert, drop the two conjuncts and these two tests.
- **Deviation 2 (prettier-ignore):** formatting only. See item 1.
- **Efficiency and security:** still exactly one engine call per request, as the wire-level `oneAutoCall` assertions show. The extra work is a code-point count over the trimmed text. There is no new input surface, and `detected_from` is rendered through `langName` as text, never as HTML.
- **Frontend:** the deletion sweep is clean. No file under `frontend/src` names `detectedLang`, `detectedSourceLabel` or `'translate.detected'` outside the sweep test itself. `prettier --check` passes on the repaired test.
- **Go hygiene:** `gofmt -l internal/translate` is empty. `go vet` reports the same 4 existing `non-constant format string` diagnostics, none on a changed line.
- **Known edges F listed, none blocking:**
  - The chunk-1 exit has no `ParseLanguage` gate. The windows cannot reach the difference, because no target select offers bare `es` or an unrecognized code.
  - `ScreenshotRetranslate` shows no note. The brief accepts this gap; the PR body should say so.
  - Mismatched pastes under the floor are not corrected. That is a brief decision.

## Not verified
- No live provider was called. Single-call cases run the real google engine against loopback, and chunked cases use the scripted engine. The Apple bridge was not exercised.
- No visual check of the WKWebView layout, per the headless rule. F's jsdom mount covers the render contract.

## Files (staged by explicit path)
- `internal/translate/service_source_correct_test.go` (2 tests added)
- `frontend/src/components/detectedSourceNote.test.ts` (regex allows `,?`)
- `docs/handoffs/161/handoff-T-green.md`, `docs/handoffs/161/decisions.md`
