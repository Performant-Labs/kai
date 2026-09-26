[S] #96 Phase 9 handoff: spec audit (failure reasons)

- **Date:** 2026-09-26
- **Branch:** issue-96-implementation (worktree `.worktrees/0096-failure-reasons`), staged index over master d591857 + brief/wireframe/A commits
- **Brief:** docs/handoffs/96-brief.md. **Wireframe:** docs/handoffs/96/wireframe.html (approved 2026-09-26, handoff-D.md, Q1-Q5 as drawn)
- **Preconditions:** handoff-A.md PASS (0 block); handoff-T-green.md PASS (0 blocking); handoff-A-dup.md PASS; handoff-U.md PASS
- **Verdict:** PASS

## Independent re-run (not copied from F/T)
- Authoritative command, exact, `NODE_OPTIONS=--no-experimental-webstorage`: exit 0. Swift bridge built; Go `ok` for all 11 packages (configstore, engine, historystore, httplogstore, i18n, langpref, model, service, settings, translate, wails-updater-providers); vitest `Test Files 23 passed (23)`, `Tests 215 passed (215)`.
- `pnpm exec tsc --noEmit` in frontend: exit 0.
- `grep -n 'i18n.T("err\.[a-z_]*"), ' internal/engine/*.go`: every remaining call's argument count matches the catalog verbs in en-US and zh-CN (checked `err.baidu_api_error`, `tencent_api_error` 2x`%s`; `deepl/openai/youdao_api_error` 1x`%s`; `google_http` `%d`; `gemini_client` `%v`). No doubled args remain in `internal/engine`.

## Spec compliance (acceptance criteria)
| AC | Status | Where |
|---|---|---|
| 1 wireframe approved | met | handoff-D.md Approval |
| 2 classifier order, `ClassifyEngineError(error)` | met | internal/translate/errors.go: ErrAPIKey, ErrUnsupportedPair, `*HTTPError` (Kind, then status map), DeadlineExceeded/net.Error, then `classifyText`. Table incl. zh-CN `翻译超时` row in errors_test.go |
| 3 one `failurePayload`, both fan-outs | met | service.go `failurePayload`; TranslateMulti and translateAllStream both call it; screenshot sets `From = req.From` after (A finding 3) |
| 4 transport wraps cause, no `%!(` | met | `"%s: %w"` in deepl/baidu/tencent/youdao/openai/google; `withText` chain for anthropic/gemini; engine errors_test transport table |
| 5 google `HTTPError`, no body/query, 403/429 `rate_limit` | met | google.go `googleHTTPError`; body not read on non-2xx; google_errors_test.go |
| 6 apple `no_source_lang` wraps pair sentinel, untagged | met | apple_errors.go; apple_errors_test.go |
| 7 E9 wraps `ErrUnsupportedPair`, text unchanged | met | language_capability.go via `withText` |
| 8 not-configured at registration, 0 requests | met | engine_wrapper.go `register` closure (ValidateRequired miss gives stub; also Gemini constructor failure); deepl/anthropic second nets wrap ErrAPIKey; counting-server test |
| 9 secrets: WithSecrets + SanitizeDetail | met | secrets.go (chain kept via Unwrap, forwards optional interfaces); sanitize.go regexes cover api_key/access_token/token/secret/sign/key, Bearer, DeepL-Auth-Key, sk-, URL userinfo/query/fragment, HTML marker, whitespace, 300 runes |
| 10 `error_kind`, no camelCase in production | met | resultPane.ts; no production `.ts`/`.svelte` contains the old name |
| 11 `failureMessage` -> {headline, detail, action} | met | resultPane.ts; action only for not_configured/auth; pair apple vs other |
| 12 failed pane + dot | met | TranslateWindow.svelte: danger headline, muted detail (omitted when empty), Settings button `ShowSettings()`; dot title/aria-label `engineName · headline`, fallback `engineFailed` |
| 13 TranslateCard | met | headline replaces badge when `tr.error`; muted detail row; no Settings button; `screenshot.translateFailed` kept otherwise |
| 14 i18n keys | met | six new keys + `failedAuth`/`failedNetwork` take `{engine}`; en/zh match wireframe section E |
| 15 logs unchanged apart from redaction | met | both `slog` calls untouched; the redacted top-level `Error()` is what they print |
| 16 suite + tsc | met | re-run above |

## Secrets in shown details
Every path to `TranslateResult.Error` goes through `failurePayload` -> `SanitizeDetail`, and every credentialed translator is `WithSecrets`-wrapped at registration. Stub errors are `ErrAPIKey` (no secret text). The Gemini constructor-failure stub is not `WithSecrets`-wrapped, but its text passes through `SanitizeDetail`, and `genai.NewClient` errors do not echo the key; acceptable. The dot tooltip and aria-label show the headline only, never the detail (Q3). No secret reaches a shown detail on any path I traced.

## Scope against the brief
In scope. No provider-specific parsing beyond google and apple (D5/D6). No Swift change, no retry, no #80 behavior, no Settings-page change. Store-package `%!(EXTRA` sites (21) correctly left out. Files touched match the brief's Files list, plus the three new files A finding 7 asked for (`secrets.go`, `notconfigured.go`, `sanitize.go`).

## Deviations from wireframe, ruled
1. **Detail carries `callEngine`'s prefix** (e.g. `Translation failed(deepl): Engine is missing API Key configuration`), where the wireframe drew `DeepL: missing API key`. Ruling: conforms. AC11 defines detail as `result.error`, AC3/AC9 define that as the sanitized raw error, and the wireframe header says error details are examples. Stripping would drop the meaning of `Translation timeout(...)`. Listed for the principal's hand test; a change there is a new story, not rework.
2. **`(HTML response)`** vs the wireframe's `(HTML response omitted)`: AC9's text wins. Conforms.
3. **Failed card without `error` also uses the stacked header**: consistent with D3 as drawn. Conforms.

## TEST-QUALITY audit (T's tests)
Overall sound: each Go test pins one behavior through real seams (a real Registry, httptest servers, a real configstore in `t.TempDir`); the classifier table wraps inputs the way `callEngine` does; the transport, stub, secret and screenshot-path tests were shown to fail for the right reason (T-red shims, F's 18 mutants, T-green's stub mutant). Non-blocking nits:
1. `frontend/src/utils/resultPane.test.ts:332` (#81 `paneState` fixture) still builds its error payload with the retired camelCase field. It is harmless (`paneState` does not read it), but the fixture no longer matches the real payload. It should say `error_kind`.
2. `resultPane.test.ts` "only not_configured and auth carry the settings action" repeats what the per-kind loop already asserts on `action`. This is a duplicate test.
3. `TestRegisterEnginesStubForEveryCredentialedEngine`: the openai and baidu rows pass even without the stub, because those engines have their own in-engine `ErrAPIKey` check. Only the anthropic and gemini rows tell the stub apart, and the test does not assert `engine.IsNotConfigured` per row.
4. `failureSurfacing.test.ts` is a source-regex contract (`/muted/`, a 900-character look-back). It is shape-coupled, but it is the tier the brief mandated, since there is no render harness (#28).
5. The Gemini constructor-failure stub has no test and no known trigger. Accepted: it is defensive only.

## For the principal to confirm by hand (no browser, #28)
1. Wrapping of long headlines in the card header slot (D1/D2), en and zh.
2. Failed pane: block vertically centered with the button (Q4); detail wraps at about 260 px (B2).
3. Dot tooltip placement and text on a real hover (C1/C2).
4. The Settings button opens Settings, and the translate window stays open.
5. A real key left unset (not_configured) and a real rejected key, end to end; Apple pair copy.
6. The detail's `Translation failed(<engine>): ` prefix under the headline (deviation 1) reads acceptably, in en and zh.
7. Switching language mid-session re-renders the headlines.

## Merge-order note for O (brief D7)
#95 is still open (PR #108, `issue-95-implementation`), so this branch is not rebased onto it. Whichever of #95 and #96 merges second has to reconcile three things, as handoff-F Known issues describes:
- the one adjacent hunk in the failed branch of `TranslateWindow.svelte` (keep #96's content and add #95's `p-4`);
- both sides' appends to `decisions.md`;
- #95's `flatResultPane.test.ts` assertion of `t('translate.failed')`, which must become `failure.headline`.

None of this is a defect in this diff.

## Verdict
PASS. Every acceptance criterion is met and was re-verified. There are no REWORK items. The five test nits above are optional tidy-ups and do not gate merge.
