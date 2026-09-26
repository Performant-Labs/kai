# handoff-A-dup: #96 failure reasons (Phase 7, anti-duplication gate)

- **Date:** 2026-09-26
- **Diff reviewed:** staged index on worktree `.worktrees/0096-failure-reasons` against master d591857 (production files listed in handoff-F "Files changed"), after T-green
- **Reuse map (brief, docs/handoffs/96-brief.md:568-575):** `ClassifyEngineError`, `engine.ErrAPIKey`, `engine.ValidateRequired`, `failureMessage`, `failedPair`/`failedNetwork`/`failedAuth`, `ShowSettings` + `titlebar.settings`, `restoreSession`
- **Verdict:** PASS (0 block, 4 warn)

## Summary

F extended every object the Reuse map named and built no parallel path.

- **One classifier.** `translate.ClassifyEngineError` changed signature in place (`string` to `error`). Its old substring body survives as the private `classifyText` fallback, not as a second classifier. The only non-classifier code that names a kind is `googleHTTPError` setting `HTTPError.Kind`, which D1 sanctions ("optional override"). No engine and no frontend file picks a kind from text (`grep model.ErrorKind internal`: only `translate/errors.go` and the Google override).
- **One not-configured sentinel.** `engine.ErrAPIKey` is reused by the stub, DeepL and Anthropic. No new sentinel. `ErrUnsupportedPair` is new and D1-mandated.
- **One definition of "configured".** `registerEngines` calls `ValidateRequired` in one `register` closure used by all nine engines. `IsNotConfigured` only reports what registration decided. Anthropic's new in-engine empty-key check is the D2 "second net" and wraps the same sentinel (its catalog key `err.anthropic_missing_apikey` already existed on master).
- **One renderer.** `failureMessage` in `utils/resultPane.ts` changed in place and is the only renderer for the failed pane, the dot tooltip/aria-label and `TranslateCard`. No component has its own kind-to-copy table. `failedNetwork`/`failedAuth`/`failedPair` are reused. The new keys are the brief's new categories. `ShowSettings()` and `titlebar.settings` are reused. No new Go API.
- **One payload builder, one sanitize point, one redaction point.** `failurePayload` replaced both hand-built `TranslateResult` literals (`TranslateMulti`, `translateAllStream`). `SanitizeDetail` is applied only there. `WithSecrets` is applied only in `registerEngines`. A grep of `internal/` and `pkg/` for redact/sanitize/scrub/Replacer finds no pre-existing helper that these duplicate.
- **One text-preserving wrapper.** `withText` (A's `markErr`, renamed) is the single helper, used by all eight sites (unsupported target, Apple, DeepL, Anthropic x2, Gemini, Google, `WithSecrets`). No site hand-rolls its own `Unwrap` type. `i18n.Error` (the only other `Unwrap` in the tree) is correctly not reused, because of its `\x1f` envelope (Phase 3).
- **No duplicated Apple mapping.** `apple_darwin.go` lost its code-to-message switch arms. The mapping lives only in `appleBridgeError`. The residual switch in `apple_darwin.go` only logs.
- Dependency direction is unchanged: engine to model, translate to engine and model, service to engine. Kind constants have one Go home (`internal/model`) and no aliases are left in `translate`.

## Findings

| # | Severity | Location | Dimension | Finding | Suggested fix |
|---|---|---|---|---|---|
| 1 | warn | `frontend/src/components/TranslateWindow.svelte` dot `title` / `aria-label` | pattern consistency (restated logic) | The #81 A-dup warn about the duplicated four-arm ternary still applies. #96 adds the same `dotFailure ? dotFailure.headline : t('translate.engineFailed')` arm to both attributes, so the label is now written out twice. It computes `dotFailure` once, which is good, but the two strings can still drift apart. | One `{@const dotLabel = ...}` beside `dotFailure`, bound to both `title` and `aria-label`. This is a follow-up tidy-up and does not block. |
| 2 | warn | `frontend/src/utils/resultPane.ts` `ErrorKind` union | cross-cutting (two sources of truth) | The nine wire kinds are listed in Go (`internal/model`) and again in TS. The Go constants are untyped `const` strings, so the bindings generate no enum, and a hand mirror is the only option. A finding 5 prescribed this, and unknown values fall to `default`, so drift only degrades to generic copy. | Keep as is. If a later story makes the kinds a named Go type, the TS union can come from the generated bindings. |
| 3 | warn | `internal/engine/errors.go` `HTTPError.Error()` | abstraction level | No production path shows `HTTPError`'s own text. Google, the only producer, puts `withText` localized copy over it. The formatter is untested (handoff-F, T item 5). It is D1 contract surface for the D6 follow-ups, so it is not dead code, but it is speculative today. | None now. The first D6 follow-up that returns a bare `*HTTPError` should pin its text. |
| 4 | warn | `internal/engine/anthropic.go` `apiKey` field | cross-cutting ("configured" checks) | Anthropic now stores the key only to check that it is empty, beside `ValidateRequired`, which already rejects this case at registration. This is sanctioned by D2 (in-engine checks stay as a second net and wrap the same sentinel) and matches DeepL. It is the only engine that gained a new second-net check. | None. If a future story removes the second nets, remove this one with DeepL's. |

## Notes for O

No block, so there is nothing to amend. Warn 1 is the only one worth a small follow-up (a single `dotLabel` const). Warns 2 to 4 are recorded so later stories (the D6 follow-ups, any typed `ErrorKind`) know where the mirrors are. Handoff-F's deviations (the `callEngine` prefix in the detail, the `(HTML response)` marker) are spec and UX questions for S/U, not duplication.

## Evidence

- `git diff --cached` of `internal/engine/{errors,secrets,notconfigured,apple_errors,apple_darwin,google,deepl,anthropic,gemini,language_capability}.go`, `internal/translate/{errors,sanitize,service}.go`, `internal/service/engine_wrapper.go`, `internal/model/model.go`, `frontend/src/utils/resultPane.ts`, `frontend/src/components/{TranslateWindow,TranslateCard}.svelte`.
- `grep -rniE "redact|sanitiz|scrub|Unwrap\(\) error|Replacer" internal pkg` (non-test): the only hit outside the new files is `internal/i18n/error.go:80`.
- `grep -rn model.ErrorKind internal` (non-test): `translate/errors.go` and `engine/google.go:124` only.
- `git grep anthropic_missing_apikey master`: the key predates #96.
