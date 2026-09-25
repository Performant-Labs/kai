# Decision journal

## 2026-09-25 — #52 A (Phase 3, up-front plan review): PASS

- Verdict PASS, 0 block / 8 warn. Handoff: docs/handoffs/52/handoff-A.md.
- Settled capability shape (brief delegated it to A): `AllEngineItem.TargetLanguages []model.Language`, populated in `EngineWrapper.GetAllEngines` from an `internal/engine` registry keyed by engine name (same pattern as `Supported: engine.EngineSupported(...)`); no separate wrapper method.
- Model contract: `AllLanguages()` = recognized (adds `PT`), new `SelectableLanguages()` drives `GetLanguages`; `Language.Base()` alias helper in `internal/model`.
- Registry must absorb the existing per-engine `xxxLang` code maps rather than sit beside them; deepl needs an explicit capability decision (omitted from brief).

## #52 T-red (2026-09-25)
- Decided: tests pin A's settled contract (SelectableLanguages, Base, engine registry, AllEngineItem.TargetLanguages); exhaustiveness iterates KnownEngines translators x AllLanguages.
- Assumed: registry API shape `LookupLanguage`/`LanguageCapability`/`SupportedTargets` (A gave only intent). Added `@bindings` alias to frontend/vitest.config.ts so lang.ts is testable.
- Hedged: google/deepl/apple capability values left unasserted (fixture verification pending; exhaustiveness forces a recorded decision). Persisted-`es` coercion untested (no defined seam).
- Evidence: docs/handoffs/52/handoff-T-red.md.

## #52 F (2026-09-25)
- Decided: one capability registry (`internal/engine/language_capability.go`, table keyed by engine name, `Supported` + request `Code`); every `xxxLang`/`normalizeLang` is a thin source-side lookup (variants alias to base), and each engine has a target counterpart that refuses a recognized language it has no exact entry for (`err.engine_unsupported_target`, generic `engine` failure kind, no network call) — the alias is never used to degrade a target.
- Decided (evidence over the brief's expectations): apple supports es-MX/pt-BR/pt-PT (verified live on macOS 27.0 26A428 via Translation.framework: distinct dialect output). google: pt-BR and pt-PT supported, es-MX refused (live gtx probe: es, es-MX, es-419, es-ES byte-identical, region ignored; pt-PT distinct). deepl: es-MX refused (ES-419 is a broader dialect, not substituted), PT-BR/PT-PT supported (docs only).
- Decided: UI gating is a union over enabled, platform-supported translators (`isTargetDisabled`, both windows, fail-open when nothing to gate against); persisted bare `es`/`pt` in `default_from`/`default_to` are coerced at settings load/hot-reload to the first selectable variant of the family (es-MX, pt-BR), unknown codes untouched.
- Decided: bindings regenerated in full (only model/models.ts and service/models.ts have code changes; 11 files are comment-only drift from the #47/#55 comment translation).
- Assumed: union (not intersection / active-engine) is the intended reading of "engines lacking a target variant disable it"; `es` to `es-MX` is an acceptable coercion for a persisted Spanish target; apple's static entry reflects macOS 27 only (engine floor is macOS 26).
- Hedged: T's `TestEnginesWithoutVariantsDisableThemAsTargets` (apple in the no-variants list) and `TestLLMLanguageNamesCoverVariants` (English fragments under the zh-CN default locale) are believed wrong and left red for T; details and suggested fixes in the handoff. Product consequence needing a ruling: with bare es/pt unselectable, engines without dialect support (baidu, tencent, youdao; deepl and google for Spanish) have no Spanish/Portuguese target, so a google-only setup has no Spanish target until an LLM engine is enabled; one-line lever in the google table if the principal prefers to serve es-MX as gtx's generic Spanish.
- Evidence: docs/handoffs/52/handoff-F.md (verbatim probe outputs, Tier 1 run, observations incl. `internal/translate/service.go:173-175` discarding the engine-detected `From`, relevant to #53).

## #52 A-dup (Phase 7, anti-duplication gate, 2026-09-25): PASS
- Verdict PASS, 0 block / 4 warn, diff 4c2c647..89dd044. Handoff: docs/handoffs/52/handoff-A-dup.md.
- Confirmed: registry absorbed the six per-engine switch maps (no second code table); capability exposed only via `AllEngineItem.TargetLanguages`; `isEnabledTranslate` reused, not restated.
- Follow-up candidates: `engine.resolveLanguage` belongs in `model` (as `ParseLanguage`) once #53 needs it outside engine; `lang.test.ts` checks a literal list, not the generated enum, and ScreenshotWindow still uses the static lists (A finding 5 only partly applied).

## #52 F rework 1 (Phase 6 re-entry after S REWORK 4eba3dd, 2026-09-25)
- Decided: no production change this round. S's REWORK list (docs/handoffs/52/handoff-S.md) is all T-owned or principal-owned, and S states "No F-side production change is required by this audit." F does not edit tests, add tests or write `handoff-T-green.md` (role boundary); both wrong tests (`TestEnginesWithoutVariantsDisableThemAsTargets`, `TestLLMLanguageNamesCoverVariants`) were re-diagnosed independently and left for T with exact corrections.
- Decided: did not take the F-side alternative for S item 3 (ScreenshotWindow onto `GetLanguages`): A-dup rated it a follow-up warn, S offered the test-side alternative, the brief sanctions the synced static list, and a Wails window change is only type-check/build verifiable here while closing nothing S requires. The test-side parity check would pass on the code as it stands (static list == generated enum minus `""` `$zero`, `es`, `pt`).
- Decided: returned `done: true` (F's part is complete; a test-only REWORK is the designed F no-op followed by T-green). The flag is not used to override the pipeline's routing.
- Decided: flagged a follow-up for the playbook repo (spawn-task chip, nothing changed there): the Workflow driver never runs the Tester role after F (`coding-pipeline.workflow.mjs:3440-3444` auto-PASSes Phase 7 under in-session; the tester role is used once, `:3527`, for T-red; S REWORK routes to F, `coding-pipeline-logic.mjs:650`), so T-owned rework cannot execute and S will REWORK again until an operator runs T.
- Assumed: S's next pass will hit the same T precondition unless T runs first (S's own T-precondition text). The principal's ruling on the google-only Spanish consequence (round 1; handoff-S.md "Operator decision needed") is still pending and, if it flips google/deepl `es-MX`, T's assertions flip with it.
- Hedged: the operator may prefer one F session to apply the two test edits directly; that would be a deliberate role exception and is not taken here. Evidence for apple/deepl/google entries is unchanged from round 1 (apple: macOS 27.0 only; deepl: docs only).
- Evidence: docs/handoffs/52/handoff-F-rework.md (suite output with the two FAIL blocks verbatim, real-path scratch runs incl. refusal with zero requests, google `tl` codes, source aliasing on 5 engines, settings load + fsnotify hot reload coercion, `GetAllEngines` JSON, `isTargetDisabled`, enum parity); bindings idempotent (`wails3 generate bindings -clean=true -ts -i`, no diff); `go test -vet=off ./internal/... ./pkg/... -count=1` fails only the two T-owned tests; frontend 7 files / 61 tests pass.

## #52 A-dup cycle 2 (Phase 7 re-entry after F rework 1, 2026-09-25): PASS
- Verdict PASS, 0 block / 2 warn, diff 4eba3dd..334a73e. Handoff: docs/handoffs/52/handoff-A-dup.md ("Cycle 2").
- Confirmed: the rework cycle touched docs only (`handoff-F-rework.md`, `decisions.md`); no source/test/bindings change, so the cycle-1 PASS on 4c2c647..89dd044 carries over unchanged.
- Carried: the `lang.test.ts` enum-parity warn is still open (T-owned). A process warn from F: the driver never runs T-green under in-session, so S will likely REWORK again unless T runs first.

## #52 principal rulings (2026-09-25, in response to S's ADVISORY-HOLD)
- Decided (principal): apple's variant support follows the macOS 27 evidence. The registry entry (es-MX/pt-BR/pt-PT supported) stands, and the brief's line "baidu/tencent/youdao/apple: expected unsupported" is superseded for apple. T's `TestEnginesWithoutVariantsDisableThemAsTargets` must drop apple from the no-variants list and assert the verified values.
- Decided (principal): google serves es-MX as the default Spanish: registry entry `model.ESMX: yes("es")` (request code `es`), so a google-only setup keeps a Spanish target and a persisted `default_to = es` no longer maps to a disabled option. The dialect is not honored by gtx; that is accepted.
- Assumed: baidu/tencent/youdao stay refused for es-MX (no ruling changes them), so "no target degradation" still holds for them.
- Evidence: docs/handoffs/52/handoff-S.md (Proposed resolution), internal/engine/language_capability.go (google table comment).

## #52 A-dup cycle 3 (Phase 7 re-entry after principal rulings + T-green, 2026-09-25): PASS
- Verdict PASS, 0 block / 2 warn, diff c571d3e..434635b. Handoff: docs/handoffs/52/handoff-A-dup.md ("Cycle 3").
- Confirmed: the only production change is google `model.ESMX: yes("es")` in the existing registry, which is the single source of truth, so no parallel table or branch was added. The T-green tests drive the existing seams.
- Closed: cycle-1 warn 2 (the enum-parity test now exists in `lang.test.ts`). Carried: cycle-1 warns 1, 3 and 4 (`resolveLanguage` placement belongs to #53).
