[T] #96 Phase 7 handoff: verify GREEN + Tier 2

- **Date:** 2026-09-26
- **Verdict:** PASS. Authoritative suite GREEN: all 11 Go packages ok, frontend 23 files / 215 tests passed (NODE_OPTIONS=--no-experimental-webstorage). No production change needed.

## Test repairs (T-owned, F flagged)
1. `failureSurfacing.test.ts` AC10: the walk now skips `*.test.ts`; the invariant is "no production source reads the camelCase name". Test files legitimately mention it (a negative case, a #81 fixture).
2. `sessionRetention.test.ts` (#81): anchored on `failure.headline` instead of `t('translate.failed')` (AC12 moved that literal into `failureMessage`); still requires the nearest preceding branch marker to mention `failed`.
3. Gap closed: `TestEnabledTranslatorNamesSkipsNotConfiguredStub` (internal/translate). Mutation (count stubs again) fails it with `[real stub], want [real]`; restored.
4. `HTTPError.Error()` text untested: accepted, nothing displays it.

## Tier 1 (independent)
- Authoritative command: exit 0, all green. `tsc --noEmit` exit 0. `gofmt -l internal pkg` empty.
- `go vet` on internal/translate: only the pre-existing non-constant-format-string class.

## Tier 2
- Coverage per criterion: classification (translate/errors_test), sanitize, engine error shapes (engine/*_errors_test), secrets redaction, stub registration (service test), payload from both fan-outs, frontend failureMessage/pane/dot/card contracts, i18n keys. Each has a committed test; F's 18 mutation probes (17 killed) plus my stub mutant confirm tests pin behavior.
- Security: secrets redacted by WithSecrets and URL query stripped by SanitizeDetail; tested.
- Error handling: `%w` chains preserved; classifier order per brief.
- Noted for later phases (not blocking): detail carries the callEngine prefix (F deviation 1); HTML marker `(HTML response)` matches AC9; #95 merge needs the `flatResultPane` assertion updated (F item 3); layout unseen (headless).

## Files touched by T this phase
frontend/src/components/failureSurfacing.test.ts, frontend/src/components/sessionRetention.test.ts, internal/translate/service_failure_payload_test.go (staged by path).
