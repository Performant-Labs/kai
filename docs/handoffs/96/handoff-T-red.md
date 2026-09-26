# [T] #96 Phase 4 handoff: RED tests (failure reasons)

- **Date:** 2026-09-26
- **Branch:** issue-96-implementation (worktree `.worktrees/0096-failure-reasons`), base master d591857 + brief/wireframe/A commits
- **Verdict:** PASS (valid RED). Nothing committed; test files staged by explicit path.

## A precondition
handoff-A.md verdict PASS (0 block, 8 warn). Warn defaults applied in the tests: finding 1 (WithSecrets forwards `SupportsAutoSource`, delegates `Name()`), finding 2 (E9 / deepl / anthropic texts asserted byte-identical), finding 3 (`failurePayload` From empty; stream entry From == req.From), finding 4 (`engine.IsNotConfigured` predicate), finding 5 (structural first parameter, tested without a cast), finding 6 (see "Inherited from #95"). Wireframe approved (handoff-D.md).

## Names the tests pin (F must create exactly these)
- `engine.HTTPError{Status int; Code, Message, Kind string}`, `engine.ErrUnsupportedPair`, `engine.WithSecrets(t Translator, secrets ...string) Translator`, `engine.IsNotConfigured(t Translator) bool`.
- `engine.appleBridgeError(code, detail string) error` (untagged, `internal/engine/apple_errors.go`).
- `translate.ClassifyEngineError(err error) string`, `translate.SanitizeDetail(string) string`, `translate.failurePayload(name string, req model.TranslateRequest, err error) model.TranslateResult`.
- Wire kind strings are used as literals in the tests (`not_configured`, `auth`, `quota`, `rate_limit`, `unavailable`, `network`, `pair`, `too_long`, `engine`), so the constant names in `internal/model` are F's choice.
- Frontend: `failureMessage(result, t, engineLabel) -> { headline, detail, action }`, `error_kind` read as generated, i18n keys `failedNotConfigured/Quota/RateLimit/Unavailable/TooLong/Unsupported`.

## Tests authored
| File | Test | Criterion | Tier / why |
|---|---|---|---|
| internal/translate/errors_test.go (rewritten) | TestClassifyEngineError (23 rows incl. zh-CN `翻译超时` deadline, url.Error/net.OpError, wraps ErrUnsupportedPair, text fallbacks, nil) | AC2 | Unit: pure function, the brief's table wrapped as callEngine wraps it |
| same | TestClassifyEngineErrorStructuredBeatsText | AC2 order | Unit: status beats "timeout"/"unauthorized" text; ErrAPIKey beats "dial tcp" |
| internal/translate/sanitize_test.go | TestSanitizeDetail (10 rows), TestSanitizeDetailTruncatesToRunes | AC9b | Unit: query/userinfo/fragment, key=, Bearer, DeepL-Auth-Key, sk-, HTML marker, whitespace, 300 runes |
| internal/translate/service_failure_payload_test.go | TestFailurePayloadCarriesReason, TestTranslateAllStreamFailureEntryHasReason, TestTranslateAllStreamTextOnlyFailure | AC3, finding 3 | Service with real Registry and fake failing engine, app nil (as service_xx_guard_test.go) |
| internal/engine/errors_test.go | TestEngineTransportFailureWrapsCauseAndHasNoFormatArtifacts (8 engines, closed loopback port) | AC4 | Integration over real HTTP failure, one table |
| same | TestUnsupportedTargetErrorWrapsPairSentinelTextUnchanged | AC7, finding 2 | Unit |
| same | TestMissingKeyPathsWrapAPIKeySentinel (deepl, anthropic; counting server) | AC8 second net | Unit + 0 hits |
| same | TestWithSecretsRedactsAndKeepsChain, ...EmptySecretDoesNotMangleText, ...PassesSuccessThrough, ...ForwardsAutoSourceSupport | AC9a, finding 1 | Unit |
| internal/engine/google_errors_test.go | TestGoogleNon2xxReturnsHTTPError (429 HTML, 403, 503, 414) | AC5 | httptest, status, Kind rate_limit for 403/429, no HTML/`q=`/query text/`%!(` |
| internal/engine/apple_errors_test.go | TestAppleBridgeError | AC6 | Untagged pure function |
| internal/service/engine_wrapper_notconfigured_test.go | TestRegisterEnginesNotConfiguredStubMakesNoRequest, ...StubForEveryCredentialedEngine, ...ConfiguredEngineIsNotStub, ...RedactsConfiguredSecret | AC8, AC9a, AC8/E10 | setupPrimaryEnv-style real configstore + counting httptest server; gemini silent skip; baidu with a missing secret |
| frontend/src/utils/resultPane.test.ts (failureMessage block rewritten) | 15 tests: one per kind (headline key + {engine}, detail, action), pair apple vs other, unknown kind, retired camelCase ignored, null/absent, bindings-shaped input | AC10, AC11 | Vitest, real snake_case payload, injected t |
| frontend/src/components/failureSurfacing.test.ts (new) | failed-branch contract, dot title/aria-label, TranslateCard, no `errorKind` in frontend/src, i18n keys and {engine} | AC10, AC12, AC13, AC14 | Source contract, translateWindowGear.test.ts style (no render harness) |

Not covered here, on purpose: AC15 (log lines unchanged; F's diff review), AC16 (T-green), and `enabledTranslatorNames` skipping stubs. The last one cannot be pinned without knowing F's stub constructor (its exclusion is unexported logic in `translate`); T-green must check it by reading the diff and, if F exposes a constructor, adding a test then.

## RED confirmation
Command (per package, then the authoritative suite pieces): `go test -vet=off ./internal/translate ./internal/engine ./internal/service -count=1` and `NODE_OPTIONS=--no-experimental-webstorage pnpm --dir frontend test`.

Go, three packages fail to build, all on the new symbols the brief lists (accepted RED for a brand-new export):
```
internal/translate: undefined: engine.ErrUnsupportedPair / engine.HTTPError (errors_test.go:27,37...)   FAIL [build failed]
internal/engine:    undefined: appleBridgeError, ErrUnsupportedPair, HTTPError, WithSecrets              FAIL [build failed]
internal/service:   undefined: engine.IsNotConfigured (engine_wrapper_notconfigured_test.go:40,85)      FAIL [build failed]
```
The compile failure hides the per-test reasons, so the assertions were exercised in isolation with throwaway shims that were removed (nothing left in the tree):
- Transport table with only existing symbols (other test files moved aside, run, restored): FAIL for baidu, tencent, youdao, deepl, openai (text `...failed to send request%!(EXTRA *url.Error=Post "http://127.0.0.1:...": dial tcp ...` and `errors.As(net.Error) = false`), anthropic and gemini (`errors.As(net.Error) = false`, cause formatted with `%s`). google passes (already returns the raw transport error), kept as a guard.
- Service tests with a temporary `IsNotConfigured` shim returning false: FAIL for the right reasons. deepl: `errors.Is(err, ErrAPIKey) = false: deepl error: missing API Key...`; anthropic: SDK credential-lookup error, not ErrAPIKey; gemini: `not registered (silent skip)`; secret redaction: `configured key leaked in error text: "DeepL: API error Wrong credentials sk-live-topsecret-...%!(EXTRA string=...)"`. `ConfiguredEngineIsNotStub` passes with the shim (a guard, not a RED test).
- Vitest: `Test Files 2 failed | 21 passed; Tests 23 failed | 192 passed`. All 23 failures are assertion failures: `failureMessage` returns a string (headline `undefined`), no `failureMessage(` call or `.headline` in the failed branch, no muted detail, TranslateCard has no `failureMessage(`, camelCase read found in 2 frontend files, the six new i18n keys undefined, `failedAuth`/`failedNetwork` lack `{engine}`. The 192 existing tests still pass.

## Inherited from #95 (finding 6)
`frontend/src/components/flatResultPane.test.ts` does not exist on this branch (#95 is on `issue-95-implementation`, unmerged), so it was not edited. On the rebase D7 requires, its `t('translate.failed')` assertion must become `failureMessage(` (keep its `--app-danger` and `noActiveEngine` checks). The failed-branch test here reads the branch by its `pane === 'failed'` marker, not by line, so it survives #95's move.

## Notes for F
- The failed-branch test finds the branch by depth-counting Svelte `{#if}` blocks; `failureMessage(` may be called in the branch or in a `{@const}` or script variable as long as the branch renders `.headline` and `.detail` and gates the button on `action === 'settings'` with `ShowSettings(`.
- Dot `title=`/`aria-label=` must reach `failureMessage(` or `.headline` within about 900 chars of the attribute.
- openai and anthropic SDKs retry connection errors; the transport table takes about 3 s.
- The kind for text-only `engine`/unknown may call `t('translate.failed')` with or without `{engine}`; tests accept both.

## Ready for F
Yes. No production code changed. Staged: the 9 test files listed above.
