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

## #53 A (Phase 3, up-front plan review, 2026-09-25): PASS
- Verdict PASS, 0 block / 6 warn. Handoff: docs/handoffs/53/handoff-A.md (includes an extend-vs-new baseline in place of the absent survey Reuse map).
- Decided: a new preference domain package (depends only on `model`) plus a thin `internal/service` wrapper is justified, since no per-session language state exists anywhere. The detected-`From` fix and qualification go in the existing `translate.Service.translateWithEngine` choke point.
- Direction for F: promote `engine.resolveLanguage` to `model` (the carried #52 A-dup warn); reverse-map engine-native detected codes in the engine registry layer; call learn only from select `onchange`, never from `persistLangs`/`swap()`/hydration.
- Open for O before T-red: define the "window session" reset trigger (windows hide, never close; suggested `Reset()` from the translate-window `WindowClosing` hook in main.go), and rule on "LLM prompts phrase the qualified variant" for an auto source (qualification is post-call; do not add a bound request field). History rows will record the qualified `From`; accept or reject in the brief.

## #53 T-red (Phase 4, 2026-09-25)
- Decided: pinned API = `model.ParseLanguage`, `internal/langpref` (`New/Learn/Qualify/Reset`), `translate.Service.SetLangPrefs`. Detected-From path has an assertion-level RED; new-symbol tests are a declared compile-RED (T cannot write production stubs).
- Assumed: Learn no-ops for bases/non-variants; explicit request From is never qualified or replaced; only auto-source results are carried/qualified.
- Hedged: A warns 3 (Reset trigger) and 4 (LLM prompt phrasing) were unruled in the brief, so no tests assert them. History-row `From` change (A warn 6) not asserted either.
- Evidence: docs/handoffs/53/handoff-T-red.md.

## #53 F (Phase 6, 2026-09-25)
- Decided: implemented T's pinned contract as written; `engine.resolveLanguage` is now `model.ParseLanguage` (4 engine call sites). One addition: `model.Language.Normalize()` (`ParseLanguage(Base())`), so no canonicalization logic lives outside `model` (A finding 1); `Store.Qualify` is nil-receiver-safe, so a `translate.Service` with no store still carries a normalized detection.
- Decided (unruled in the brief, on A's safe defaults): one process-wide store, `Reset()` from the translate window's `WindowClosing` hook (`main.go:485: langPrefs.Reset()`), the screenshot window's close does not reset (A warn 3). LLM prompts (A warn 4): ruling (a), no pre-call hint, #52's `srcName`/`dstName` cover explicit selections and #13's swap will carry the qualified value. History rows record the carried/qualified `From` (A warn 6). All three are open for O to overrule.
- Decided: `Learn` is called only from the language selects' `onchange` in both windows; `swap()` and settings hydration never teach (A warn 5). One-line guard in `detectedSourceLabel` (auto = nothing detected), beyond the acceptance lines and revertable. Engine-native detected codes are out of scope in writing (A warn 2): `internal/engine/baidu.go:118: from := model.Language(br.From)` and `internal/engine/youdao.go:125: from := model.Language(yr.L)` stay unmapped; two follow-up chips raised (registry reverse lookup; an unrelated shadowed `t()` in `ScreenshotWindow.svelte`).
- Assumed: `Learn(es-419)` is ignored (an alias, not recognized or selectable); unrecognized detections pass through as reported (T's pin); youdao's `l` is a direction pair (A's reading, not verified live, needs a key).
- Hedged: the F scope cap (about 6 files, one surface) was crossed (12 hand-written files, 3 layers) and not split: non-interactive run, A passed the whole plan, edits are small; flagged for A/S. Screenshot-only sessions keep prefs until process exit or a translate window close, and after a reset a persisted `es-MX` default can show while the store is empty (hydration must not teach). The GUI hops (`onchange` -> `Learn`, `WindowClosing` -> `Reset`) were not exercised (headless mandate).
- Evidence: docs/handoffs/53/handoff-F.md. Live Google gtx probe (2026-09-25 06:34 PM MDT): detected slot `es` for Mexican and Peninsular Spanish alike, `pt` for both Portuguese variants, `zh-CN`, `it`. Real endpoint through service + store + wrapper: `es` -> `es-MX` after `Learn(es-MX)`, `pt` -> `pt-PT` after `Learn(pt-PT)`, explicit `fr` untouched, `Reset` -> base. Wails server mode: `POST /wails/runtime` methodID 1546460697 -> HTTP 200 and the store updated. `test.unit.command` exit 0 (8 Go packages ok, frontend 8 files / 67 tests); `-race` ok.

## #53 A-dup (Phase 7, anti-duplication gate, 2026-09-25): PASS
- Verdict PASS, 0 block / 3 warn, diff 436ab39..a74554a. Handoff: docs/handoffs/53/handoff-A-dup.md.
- Confirmed: F extended every object the Phase-3 baseline named. The detected-From fix and qualification are in `translateWithEngine`. `resolveLanguage` moved to `model.ParseLanguage` and was deleted from `engine`, and no second case-folding loop exists. `detectedSourceLabel` gained one guard and was not forked. The new `internal/langpref` and `LangPrefWrapper` were justified at Phase 3.
- Warns (follow-up candidates): `translate.isAutoSource` duplicates `engine.isAuto` and should be promoted to `model`. `sourceCode` hand-composes what `Language.Normalize()` names. The per-window `learnLangVariant` follows the existing per-window convention, but a third caller should extract a shared helper.

## #53 F rework 1 (Phase 6 re-entry after S REWORK d6dd201, 2026-09-25)

- Decided: no change to #53's own code. S's REWORK list (docs/handoffs/53/handoff-S.md, S cycle 1) is two missing tests, T-owned, and S states "tests only; no production change required". F wrote no tests (role boundary, as in #52 rework 1). Both are flagged under "Tests that look wrong (for T)" in docs/handoffs/53/handoff-F-rework.md (and by a pointer at the top of handoff-F.md), with runnable recipes and verified expected values. Unlike the #52 rework, the driver now runs T-green after F (playbook #783), so the flag has a consumer.
- Decided (outside the brief; the brief's own scope addition folds a defect found while verifying into the story): fixed a startup-fatal regression that S item 2 runs into. `historystore.Open` fails on this branch and on master since #55 (4c2c647). The loader splits the migration file on the semicolon character (`internal/historystore/store.go:57: for stmt := range strings.SplitSeq(migrationSQL, ";") {`, same at `internal/httplogstore/httplog.go:141`), and #55's English comment (`internal/historystore/migration.sql:4: -- Table creation itself lives in schema.sql; this file only holds ALTERs / extra indexes.`) leaves a bare `this file only holds ...` piece that SQLite rejects (`near "this": syntax error`). `main.go:243: log.Fatalf(i18n.T("log.open_history_db_failed"), err)`, so the app aborts on launch. The fix is comment-only in the two `migration.sql` files (semicolons taken out of comments, an in-file warning added). It touches two files and depends on nothing in #53, so it can be cherry-picked as a master hotfix and dropped from this branch; recommended: ship it to master first (a task chip, "Hotfix master: app aborts at launch (migration.sql)", was raised for that). It is required for S item 2 because `translate.Service.history` is the concrete `*historystore.Store` (`internal/translate/service.go:35: history     *historystore.Store`), whose only constructor is `Open`.
- Decided: A-dup warns 1 and 2 (`isAutoSource` twin of `engine.isAuto`, `sourceCode` hand-composing `Normalize`) stay follow-ups. Promoting `isAuto` adds an untested exported method and a shared-engine edit in the round whose finding is untested behavior.
- Assumed: T-green authors the two S-required tests once they are flagged (missing coverage is T's own Tier 2 duty, and the driver's T-green task text names repairing what F flags). If it does not, S REWORKs a second time and a third cycle escalates.
- Hedged: whether to hotfix master separately is O's or the principal's call. The class sweep found no other semicolon split of embedded SQL (two sites, both fixed) and no other malformed embedded data file (both locale JSONs parse). The GUI hops (`onchange` to `Learn`, `WindowClosing` to `Reset`) are still unexercised (headless mandate).
- Evidence: docs/handoffs/53/handoff-F-rework.md. Real-store probe: `historystore.Open` and `httplogstore.Init` FAIL with `near "this": syntax error (1)` on the unmodified tree, ok after the fix (and ok reopening an already-migrated file); `configstore.Open` ok both times. History through the real service, real google engine over loopback and real history/config stores: detected `es` stores `from_lang="es"`, after `Learn(es-MX)` a new row `from_lang="es-MX"`, the repeat send is deduped, no detection stores `auto`, and after `Reset()` it is back to `es`. `test.unit.command` exit 0 before and after the change (8 Go packages ok, frontend 8 files / 67 tests).

## #53 A-dup cycle 2 (Phase 7, anti-duplication gate after S REWORK, 2026-09-25): PASS
- Verdict PASS, 0 block / 5 warn, rework delta a74554a..3ebabf7 (story base 436ab39). Handoff: docs/handoffs/53/handoff-A-dup.md (cycle 1 version is in 8d24cdb).
- Confirmed: the rework added no production object. The migration.sql fix is comment-only and edits the two existing files in place, with no new loader. T's additions are tests only.
- Warns: (new) `newHistoryService` re-types `newLoopbackService`'s loopback-registry fixture; the migration regression test is duplicated per store, mirroring the pre-existing twin `SplitSeq(migrationSQL, ";")` loaders (shared runner is a follow-up). (carried) `isAutoSource` twin, `sourceCode` vs `Normalize`, per-window `learnLangVariant`.
- Hedged: the relayed user reply "1. false 2. fold it in" is ambiguous for item 2. If it rules A-dup warn 1 in, it is not yet folded in at 3ebabf7 (`service.go:238`, `language_capability.go:177`); O to confirm and route to F.

## #53 principal rulings (2026-09-25, in response to S's open questions)
- Decided (principal): learned variant preferences are kept until the app quits, not wiped when the translate window is hidden. `langPrefs.Reset()` is removed from the translate window's `WindowClosing` hook in `main.go`; the session is the process, so a restart starts empty. `Reset()` stays as API (tested) and is not called in production today.
- Decided (principal): LLM prompt phrasing stays as built (option a): the variant is used for an explicit selection, not for an auto-detected source. Accepted because the source dialect changes the output little; the target dialect already works.
- Hedged: no automated test pins the hook change (`main.go` is not unit-testable); `Reset` itself and the store's session semantics stay covered in `internal/langpref/store_test.go`.

## #43 A (Phase 3, up-front plan review, 2026-09-25): PASS
- Verdict PASS, 0 block / 6 warn. Handoff: docs/handoffs/43/handoff-A.md (includes an extend-vs-new baseline in place of the absent survey Reuse map).
- Decided: #43 is verify-and-close-gaps on the seams #52/#53 built, with no new production object. Every acceptance line maps to an existing owner: `SelectableLanguages`/`GetLanguages`, `languageRegistry`/`SupportedTargets`/`isTargetDisabled`, the i18n `lang.*` tables, `langpref` plus `translateWithEngine`, and `detectedSourceLabel`.
- Direction for T/F: map each acceptance line to an existing or new test at those seams, and make production changes only where a real RED shows one is needed. Any native detected-code reverse lookup goes in the engine capability registry. ScreenshotWindow's static list should follow TranslateWindow's `loadLanguages` pattern if it is touched. Regenerate bindings only if a bound signature changes.
- Open for O before T-red: (1) whether "detected es/pt labels correctly" covers baidu/youdao (evidence: `internal/engine/baidu.go:118: from := model.Language(br.From)`, where baidu reports `spa`); default is google plus the LLM engines only. (2) Whether the LLM round-trip is live or over loopback, and which engine; default is loopback, with any live gap disclosed.

## #43 T-red (2026-09-25)
- **Decided:** authored 5 Go integration tests, 1 Go unit test, 1 frontend test file at existing seams; no production change.
- **Assumed:** A "Notes for O" defaults (detection scoped to google + LLM; LLM round trip via loopback).
- **Hedged:** no test is RED (feature already delivered by #52/#53, A finding 1); validity proven by a mutation (google pt-PT code) that fails `TestGoogleRoundTripPerVariantTarget`.
- **Evidence:** `go test ./internal/translate ./internal/engine` ok; vitest src/constants 9 passed; see docs/handoffs/43/handoff-T-red.md.

## #43 F (Phase 6, 2026-09-25)
- **Decided:** no production change. T-red produced no RED (A finding 1), and F writes production code only against a real RED; every acceptance line already resolves to the seam #52/#53 built. F verified instead: regenerated bindings with the Makefile command (`wails3 generate bindings -clean=true -ts -i`), no binding delta; full authoritative suite exit 0 (10 Go packages ok, vitest 10 files / 78 tests); 7 of 7 acceptance lines mapped to code and tests in handoff-F.md.
- **Decided:** ran a throwaway live google probe (real `translate.Service` + real engine + real gtx endpoint, 14 requests, key-free, deleted afterwards, never staged). Live result: es-MX / pt-BR / pt-PT targets round-trip (pt-BR and pt-PT in their own dialects; es-MX is generic Spanish, the documented gtx cost); variant sources are reported back as chosen; with no preference detection labels stay bare `es` / `pt`; after one pick, 3 auto-detected sends per family stayed es-MX / pt-PT.
- **Decided:** ran 4 extra mutation bite checks beyond T's one (preference store, en-US LLM name, source alias, frontend en-US name); each made the intended tests fail with the intended message, each reverted, tree clean.
- **Assumed:** A's two "Notes for O" defaults stand, because decisions.md holds no O ruling on either: detected-label scope is google plus the LLM engines (baidu/youdao native-code reverse lookup stays the #53 follow-up, engine registry is its home), and the LLM round trip is loopback.
- **Hedged:** the LLM leg is loopback only. A live run needs a real provider key, and a live credential in the app under test is out of bounds for this run; disclosed, not hidden. deepl / tencent / apple detection and the baidu/youdao label are by code reading only. "Disabled visibly" is proven by the two `<option disabled=...>` bindings plus the unit-tested `isTargetDisabled`, not by a real window (headless mandate); a `ui-walkthrough`, if run, is where it gets eyes.
- **Evidence:** docs/handoffs/43/handoff-F.md (verbatim excerpts with file:line, live probe output, mutation table, suite output). Model: Sonnet 5 (`claude-sonnet-5`), effort max, single call, no outside model. `archChanged: false` (no production file differs from 140cba2).

## #43 T-green (2026-09-25)
- Decided: GREEN, no blocking Tier 2 issue; no test repaired. Handoff: docs/handoffs/43/handoff-T-green.md.
- Assumed: F's mutation bite checks stand in for a fresh bite run.
- Hedged: disabled-option UI rendering and live LLM legs unverified headlessly (left to U).
- Evidence: full suite exit 0 (Go all ok, vitest 78/78); diff 140cba2..HEAD touches only docs.

## #43 A-dup (Phase 7, anti-duplication gate, 2026-09-25): PASS
- Verdict PASS, 0 block / 3 warn. Handoff: docs/handoffs/43/handoff-A-dup.md. Diff 8e228bd..892efb6.
- Decided: no parallel production path, because no production file differs from 8e228bd (only docs and three new test files). Each acceptance line stays with the owner named in the handoff-A baseline.
- Hedged (warn, test hygiene only): inline google loopback servers in `service_variants_e2e_test.go` sit beside `newLoopbackService`; `language_display_names_test.go` and `variants.e2e.test.ts` are new files overlapping `language_capability_test.go`, `lang.test.ts` and `detectedLang.test.ts`. Optional follow-up folds, not rework.

## #44 A (Phase 3, up-front plan review, 2026-09-25): BLOCK
- Verdict BLOCK, 3 block / 7 warn. Handoff: docs/handoffs/44/handoff-A.md (includes an extend-vs-new baseline in place of the absent survey Reuse map).
- Decided: the brief's premise that fresh installs default to English is false. `settings.DefaultSettings()` sets `DefaultTo: zh` (`internal/settings/service.go:207`) and writes it to disk at startup, and `loadDefaults()` then replaces the component's `EN`. The default-target owner is the backend default config.
- Decided: the X→X guard belongs at the single per-engine seam `translate.Service.translateWithEngine`, which both the input and screenshot paths already share and where `resultFrom` computes the detected source. It must not be a per-window frontend guard duplicated in TranslateWindow and ScreenshotWindow. The frontend only reflects `TranslateResult.To`.
- Decided: the auto-flip is per request. It never persists `default_to` and never teaches the variant store (the #53 rule that only an explicit pick persists or teaches). The upgrade sweep is met by the runtime guard, not by rewriting persisted settings.
- Open for O: the English→English fallback rule; the engine-coverage scope (LLM engines report `auto`, baidu/youdao report native codes, so the guard cannot fire for them); whether to pick auto-flip over a new swap affordance (a new affordance would need the D phase). `swap()` and `swapLangs()` stay out of bounds for #44 because #13 owns them.

## #44 A (Phase 3, up-front plan review, round 2, 2026-09-25): BLOCK
- Verdict BLOCK again, 3 block / 7 warn, same findings. Handoff: docs/handoffs/44/handoff-A.md (updated in place with a round-2 header).
- Decided: re-review found no amendment. `docs/handoffs/44-brief.md` is byte-identical to a1c7d78 and decisions.md has no O ruling on round 1's Notes for O. All three block findings re-verified at head 575f70b: `DefaultTo: string(model.ZH)` (`internal/settings/service.go:207`) plus the startup re-save; no named owner for the guard (the answer is the `translateWithEngine` seam); flip persistence unspecified.
- Decided: round 2 added one more hardcoded target literal to finding 1's sweep list, the screenshot image-pushed payload `To: model.ZH` at `internal/translate/service.go:372`, next to `:394` and `:412`.
- Open for O: this is the second consecutive BLOCK. O must amend the brief per Notes for O items 1-5 before re-running A. A third BLOCK escalates to the operator.

## #44 A (Phase 3, up-front plan review, round 3, 2026-09-25): BLOCK -- escalate
- Verdict BLOCK a third time, 3 block / 7 warn, same findings. Handoff: docs/handoffs/44/handoff-A.md (round-3 header and summary; findings table unchanged).
- Decided: still no amendment. `docs/handoffs/44-brief.md` is byte-identical to a1c7d78, no O ruling in this journal, no comments on issue #44. All block evidence re-verified at head 55f3ccc (`settings/service.go:207,239,300-305`; `translate/service.go:372,394,412`).
- Open for the operator (escalation per the >2-blocks rule): adopt Notes for O items 1-3 into the brief (backend default target -> `en` with the ZH fallbacks following it; guard owned by `translateWithEngine`, frontend only reflects `TranslateResult.To`; flip is per request, never persists or teaches), and choose the English->English fallback rule and the engine-coverage scope. Re-running A on an unchanged brief is pointless.

## #44 O rulings on A's three plan-review BLOCKs (2026-09-25 MDT)
- Decided: adopt A's Notes for O items 1-5 into docs/handoffs/44-brief.md (new "Plan" section; issue text kept verbatim above it). Backend default target becomes `en` with the ZH fallbacks following it; the X->X guard is owned by `translateWithEngine` and reported through `TranslateResult.To`; the flip is per request and never persists or teaches; the upgrade sweep is met by the runtime guard.
- Decided (principal choices, safe defaults): auto-flip rather than a new swap affordance (no UI surface); English->English does not flip; guarantee scoped to engines that report a recognized detection, remainder disclosed.
- Decided: `swap()` / `swapLangs()` are out of bounds for #44 (#13 owns them).
- Evidence: rounds 1-3 handoff at docs/handoffs/44/handoff-A.md; the brief was unchanged across all three because the automated driver has no step that amends it. Fresh run launched (not a resume) because a resume would replay the cached BLOCK.

## #44 A (Phase 3, up-front plan review, round 4 -- first review of the amended brief, 2026-09-25): BLOCK (1 narrow block)
- Verdict BLOCK, 1 block / 5 warn. Handoff: docs/handoffs/44/handoff-A.md (rewritten for the amended brief at 7fbfb0d).
- Decided: the Plan section resolves all three earlier blocks (backend default target -> en with the ZH fallbacks following it; guard at `translateWithEngine` on a `Normalize().Base()` compare; flip per request, never persists or teaches). EN->EN, the engine-coverage scope and the #13 boundary are consistent with the code.
- Decided (block, self-correction of A's own round-1 wording): "reflect `activeResult.to` in the target select" means writing `toLang`. That makes the flip sticky for the session (contradicts Plan 3). In ScreenshotWindow it also fires the `toLang` `$effect` -> `EventScreenshotRetranslate`, a second full fan-out with extra `saveHistory` calls (contradicts the one-history-entry pin). The fix is to show the flipped target per result, display-only, through a pure util next to `detectedLang.ts`. It must never assign `toLang`, persist, teach, or emit a retranslate.
- Open for O: amend Plan item 2 plus one pin in item 7 (exact text in handoff Notes for O). Re-review is expected to PASS.

## #44 A (Phase 3, up-front plan review, round 5, 2026-09-25): BLOCK -- escalate
- Verdict BLOCK, 1 block / 5 warn, same findings as round 4. Handoff: docs/handoffs/44/handoff-A.md (round-5 header and Notes for O; findings table unchanged).
- Decided: no amendment. `docs/handoffs/44-brief.md` has not changed since 7fbfb0d, and decisions.md has no O ruling on round 4's Notes for O. The block is re-verified at 335d905: both target selects render `toLang` (`TranslateWindow.svelte:499`, `ScreenshotWindow.svelte:416`), and ScreenshotWindow's `toLang` `$effect` (`:93-110`) emits `EventScreenshotRetranslate`. So "reflect `activeResult.to` in the target select" still makes the flip sticky and causes a second fan-out with extra history entries.
- Open for the operator (escalation): amend Plan item 2 so the flipped target is shown per result, display-only, through a pure util next to `detectedLang.ts`, with no `toLang` write, no persist or teach, and no retranslate. Add the pin to item 7. The exact text is in the handoff's Notes for O. The driver cannot apply this edit itself, so re-running A on an unchanged brief is pointless.

## #44 A (Phase 3, up-front plan review, round 6, 2026-09-25): BLOCK -- escalate
- Verdict BLOCK, 1 block / 5 warn, same findings as rounds 4-5. Handoff: docs/handoffs/44/handoff-A.md (round-6 header; findings unchanged).
- Decided: still no amendment. `docs/handoffs/44-brief.md` last changed at 7fbfb0d; Plan item 2 still says "reflects `activeResult.to` in the target select". Re-verified at 7623847: `TranslateWindow.svelte:499` `bind:value={toLang}`, `ScreenshotWindow.svelte:416` `value={toLang}`, `toLang` `$effect` emitting `EventScreenshotRetranslate` at `ScreenshotWindow.svelte:93-110`.
- Open for the operator: apply the two-sentence amendment in the handoff's Notes for O (per-result display-only label; pin that `toLang` is unchanged and no retranslate is emitted). The driver cannot make this edit; re-running A without it will BLOCK again.

## #44 O ruling on A round-4 block (2026-09-25 MDT)
- Decided: adopt A's two-sentence fix. The frontend shows the flipped target as a per-result display-only label; it never writes toLang/fromLang, persists, teaches, or emits a retranslate. New T pin: toLang unchanged and no retranslate after a flip. A's five warns (re-run context bounded and non-recursive, one private default-target resolver, envelope To stays the requested target, dst_lang unchanged, mixed flip outcomes) go to F and T.
- Evidence: A round 4-6 handoff at docs/handoffs/44/handoff-A.md; the block was my plan wording ("reflects activeResult.to in the target select"), not a code disagreement. Rounds 5-6 repeated only because the driver cannot amend the brief.

## #44 A (Phase 3, up-front plan review, round 7, 2026-09-25): PASS
- Verdict PASS, 0 block / 3 warn. Handoff: docs/handoffs/44/handoff-A.md (rewritten for the amended brief at 35c13e9).
- Decided: the round-4 block is resolved. Plan item 2 now shows the flipped target as a per-result display-only label (pure util next to `detectedLang.ts`) that never writes `toLang`/`fromLang`, persists, teaches or emits `EventScreenshotRetranslate`; item 7 pins it. Cited code re-verified unchanged since 7fbfb0d.
- Open for F/T (warns): compare detected vs target at `Normalize().Base()` after `resultFrom` qualification; route the single re-run through one shared engine-call helper, no copy of the timeout block and no recursion; mixed per-engine flip outcomes are expected and disclosed.

## #58 A (Phase 3, up-front plan review, 2026-09-25): PASS
- Verdict PASS, 0 block / 6 warn. Handoff: docs/handoffs/58/handoff-A.md. It includes an extend-vs-new baseline because there is no survey Reuse map.
- Decided: no new production object. `Dict` moves from `zh-CN.ts:375` to a type-only `frontend/src/i18n/keys.ts`, and its consumers (`en-US.ts`, `index.svelte.ts`) repoint to it. The new tests are a colocated vitest test in `frontend/src/i18n/` and an internal Go test in `internal/i18n`, which reads the shipped merged JSON through the existing `localesFS` embed.
- Direction for T/F: the vitest runtime deep-key comparison is the primary enforcement. The authoritative suite does not run tsc; tsc runs only in `make check`. `keys.ts` must not be derived from either catalog. The locale fence is verified by value, not by an empty textual diff.
- Hedged: parity already holds in both runtimes at 9cca47c (verified: Go en==zh key sets, split==merged), so neither test can be naturally RED. T proves the tests catch drift by mutation (drop a key per side per runtime). Open for O: whether `_comment` counts as a Go key; the default is to include it.
- Out of scope, noticed: `make i18n-frontend` targets the nonexistent `frontend/src/locales/split` and is a no-op; nothing asserts Go split==merged.

## #58 F (Phase 6, 2026-09-25)
- **Decided:** implemented against T's RED with no Go change. New type-only `frontend/src/i18n/keys.ts` owns `Dict`; `zh-CN.ts` and `en-US.ts` both declare `: Dict` and import it from `./keys`; `index.svelte.ts` imports it from `./keys`; `zh-CN.ts` no longer exports it. Work item 3 (Go parity) is fully delivered by T's `locales_parity_test.go`, so there is nothing for F to implement in Go.
- **Decided:** `keys.ts` was generated mechanically from the pre-change zh key tree (throwaway script in the scratchpad, never staged), not retyped by hand, and its key order is the old contract's. It is proven type-identical to the old `typeof zh` by a strict identity check with a failing negative control (a copy missing `engine.vision` fails with TS2322). 333 `string` leaves, the same count as each catalog.
- **Decided:** `: Dict` on both catalogs (mirroring the existing `en: Dict`), not `satisfies`. Enforcement placement follows A finding 1: the vitest deep-key comparison is primary, and the tsc annotation is the second net (`make check`, `ci.yaml:78`).
- **Decided (drift the nets do not share):** a key added to both catalogs but not to `keys.ts`, or dropped from `keys.ts` only, passes vitest and fails tsc (mutations M5/M6). It is contract hygiene, not user-visible drift, so it is not in the acceptance criteria and F did not widen scope for it.
- **Decided (outside the brief):** did not touch the stray `frontend/pnpm-lock.yaml` (+25 lines of `@pnpm/exe` entries versus master 9247202) that the driver's Brief commit 9cca47c carried in. Any `pnpm` run in `frontend/` here rewrites it (global pnpm 11.3.0 vs the pinned 12.5.1); F restored it to HEAD after each run so the F commit carries no churn. Flagged for O to drop from the PR.
- **Assumed:** A's "Notes for O" default stands, since decisions.md holds no O ruling: `_comment` is part of the Go key set (T's test includes it).
- **Hedged:** (1) mutations were applied in place on the worktree with a sha256-verified byte-identical restore, not on a literal scratch branch; same evidence, and T used the same method. (2) `svelte-check` cannot run here (TS 7 without TS 6, predates this story, not a gate). (3) T's acceptance-1 regex matches only the literal `export type Dict`, so it would miss a re-export form; F added none (verified by grep). Observation for T, not a wrong test. (4) Noticed: decisions.md has no "#58 T-red" entry (d89a5e5 touched only `handoff-T-red.md` and the two test files); T's outcome is recorded in `handoff-T-red.md`.
- **Evidence:** docs/handoffs/58/handoff-F.md (verbatim command output, mutation table, acceptance map). RED `5 failed | 3 passed` reproduced before the edit, then `8 passed (8)`. Full `test.unit.command` exit 0: Go 11 packages ok, vitest 11 files / 86 tests. `tsc --noEmit` exit 0, `prettier --check` clean, dev-mode `vite build` ok (195 modules). Locale fence by value: zh-CN and en-US `deepStrictEqual` against the `git show HEAD:` snapshots, 333 leaves each, identical ordered-leaf sha256. Model: Sonnet 5 (`claude-sonnet-5`), effort max, single call, no outside model. `archChanged: true` (new module boundary, reversed dependency direction of the type contract).

## #58 A-dup (Phase 7, 2026-09-25)
- Verdict PASS, 0 block / 3 warn. Handoff: docs/handoffs/58/handoff-A-dup.md. Diff b4d9fb6..24f0f9c.
- Decided: F relocated `Dict` rather than creating a parallel contract. The old `typeof zh` definition is deleted, `keys.ts` is the single definition, all three consumers import from `./keys`, and nothing re-exports it. The Go parity test reuses the `localesFS` embed. It adds no parallel merge/read path, and its test-local `flattenKeys` does not duplicate `error.go` `flatten`, which serves a different concern.
- Warns: (1) `keys.ts` is a deliberate third copy of the key tree; M5/M6 drift is caught only by tsc. (2) The stray `frontend/pnpm-lock.yaml` +25 lines from Brief commit 9cca47c should be restored to master before the PR. (3) T's acceptance-1 regex misses re-export forms.

## #13 A (Phase 3, up-front plan review, 2026-09-25): PASS
- Verdict PASS, 0 block / 4 warn. Handoff: docs/handoffs/13/handoff-A.md (brief at 670a41f).
- Decided: the plan extends existing objects and adds no parallel path. `swapLangs.ts` follows the pure injected-predicate helper pattern (`detectedLang.ts`, `flippedTarget.ts`, `targetCapability.ts`). The reverse translation reuses `doTranslate()`, following the `EventInputFill` precedent at `TranslateWindow.svelte:273-277`. Persistence reuses `persistLangs()`. The never-teach rule is kept, and the source-contract tests follow `translateWindowGear.test.ts`.
- Assumed: pinned-path semantics stay as the brief wrote them (the button is always enabled when `from` is pinned). `loading` does not disable the swap. O has recorded no ruling on either point.
- Hedged (warns for F/T): (1) `isSelectable` should compose `targetLanguages` membership with `!isTargetDisabled(allEngines, c)` (#52), so the swap never lands on a disabled target option. (2) Compute the helper result once as a `$derived` and use it for both the button's `disabled` and `swap()`. (3) Late events from the previous fan-out can merge into the reversed round. This race predates #13; note it and do not fix it here. (4) The file name `swapLangs.ts` is close to ScreenshotWindow's local `swapLangs`; add a clarifying module header.
- Evidence: TranslateWindow.svelte:209, 220-231, 267-277, 350-362, 411-441, 502-536; langLearn.test.ts:60-73; utils/targetCapability.ts.

## #13 T-red (Phase 4, 2026-09-25)
- Decided: valid RED. swapLangs.test.ts (10 unit cases) fails on missing module; swapWindow.test.ts fails 5 of 6 on real assertions.
- Assumed: source-contract regexes (`input = activeDisplay`, `doTranslate()` inside swap body) are the accepted pin given no render harness (#28).
- Hedged: the "never teaches" case passes pre-feature (guard only). Helper-use regex also accepts a `swapPair` name.
- Evidence: docs/handoffs/13/handoff-T-red.md.

## #13 F (Phase 6, 2026-09-25)
- **Decided:** implemented against T's RED with two production files. New pure `frontend/src/utils/swapLangs.ts` (`swapLanguages`, plain-string in and out, header names #13 and disowns ScreenshotWindow's local `swapLangs()`). In `TranslateWindow.svelte` one `swapPair` `$derived` now feeds both the button's `disabled` and `swap()`, and `swap()` is: no-op on null, assign the pair, `if (activeDisplay !== '') input = activeDisplay`, `persistLangs()`, `doTranslate()`. The hardcoded `TRANSLATE_LANG.ZH` fallback is gone. No i18n, CSS or backend change.
- **Decided:** A's warns 1, 2 and 4 applied, because decisions.md holds no O ruling on A's Notes for O. `isSelectable` is `targetLanguages.some(...) && !isTargetDisabled(allEngines, code)` (list membership plus the #52 capability gate; the helper has no engine knowledge). One derivation serves markup and handler. The module header disambiguates the name from ScreenshotWindow's `swapLangs()`.
- **Decided (constraint found):** `swap()` destructures (`const { from, to } = pair`), because `toLang = pair.to` matches the existing display-only regex in `flippedNotice.test.ts` and fails it (verified on a scratch copy). A comment in `swap()` records why.
- **Assumed:** the pinned path stays ungated and `loading` does not disable the button (A's defaults, no O ruling). `to` is always a concrete language (the brief's invariant); a hand-edited `default_to: "auto"` is not handled. `archChanged: true` under the "when in doubt" rule: literally a new module with a new exported interface, so Phase 7 should look at it. Its weight is low (one new leaf util in the existing pure-helper layer, one caller, no dependency direction changed, and A approved the module in Phase 3). O can downgrade it.
- **Hedged:** (1) `svelte-check` cannot run here (TS 7 without TS 6, predates this story), so the svelte-side expressions were checked with a scratch `tsc` file (its negative control fails as expected) and a dev `vite build`. (2) The shipped contract is still T's source-contract tests. To see the reactive behavior they cannot, I ran a throwaway jsdom mount of the real component with mocked bindings (17 checks, all pass; 9 of the first 14 fail on HEAD) and a 16-mutant pass on scratch copies. Both live in the scratchpad, are not staged and are not part of the change. The survivors are gaps in T's suite (component predicate composition, pinned path not consulting `isSelectable`, AC3's "no result: input unchanged") and are listed in handoff-F for T. (3) A warn 3 (late events from the previous round) is not fixed: pressing swap mid-flight re-sends the unchanged input with the exchanged pair, which is AC3 taken literally. Open for O only if the hand test hits it.
- **Evidence:** docs/handoffs/13/handoff-F.md (verbatim output). RED reproduced first (`5 failed | 1 passed`), then targeted GREEN (5 files, 35 tests), then the full authoritative command exit 0: Go 11 packages ok, vitest 17 files / 121 tests. `tsc --noEmit` exit 0, `prettier --check` clean on the new file (the svelte file's one pre-existing violation is unchanged), dev `vite build` ok (197 modules). Model: Sonnet 5 (`claude-sonnet-5`), single call, no outside model.

## [T] #13 Phase 7 (GREEN)
- Decided: PASS; repaired 4 test gaps myself (tests only).
- Assumed: F's jsdom probe stands in for AC5 given no render harness.
- Hedged: none.
- Evidence: full suite 125 passed; two scratch mutants now caught; tsc 0.

## #13 A-dup (Phase 7, anti-duplication gate, 2026-09-25): PASS
- Verdict PASS, 0 block / 2 warn. Handoff: docs/handoffs/13/handoff-A-dup.md. Diff fcf6238..54f7247.
- Decided: F extended the objects in the Reuse map and built no parallel path. `swap()` and the button were edited in place. The reverse translation goes through the existing `doTranslate()`, persistence through `persistLangs()`, and the capability gate through `isTargetDisabled`. The new `utils/swapLangs.ts` is mandated by AC1 and matches the pure-helper layer. A's Phase 3 warns 1, 2 and 4 were applied.
- Assumed: ScreenshotWindow's divergent `swapLangs()` is acceptable because the brief explicitly scoped it out.
- Hedged (warns): (1) the "nothing detected" predicate is duplicated between `swapLangs.ts:77` and `detectedLang.ts:31`; extract `hasDetection` if a third consumer appears. (2) ScreenshotWindow could adopt `swapLanguages` (with `detectedFrom: ''`) without changing its behavior. This is a follow-up candidate for O.
- Evidence: git diff fcf6238..54f7247 (TranslateWindow.svelte, utils/swapLangs.ts); ScreenshotWindow.svelte:138-143; detectedLang.ts:31.
