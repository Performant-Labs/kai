# Brief: #96 failure-reasons

Repo: Performant-Labs/kai-private (never upstream dtapps/kai). Issue: #96. Rigor: in-session. UI surface: yes (wireframe of the failed states for principal approval before tests/code). Kind: feature (backend error classification + frontend failure rendering). Sequenced after #95 (both edit the translate window's result pane; #95 is flattening it on branch `issue-95-implementation`).

## Problem
When an engine fails, the translate window shows only "Translation failed" and the screenshot window only a red "Translation failed" badge. The user cannot tell a missing key from a rejected one, a quota or rate limit, an outage, a network problem, or an unsupported language pair, so they cannot fix it. #42 (closed) built half of this: a substring classifier and a frontend `failureMessage`. Neither reaches the user, and the engine errors underneath are in no state to be classified or shown. The status code is lost, the wrapped cause is lost, and the text is garbled with `%!(EXTRA ...)`.

## Evidence (verbatim, master d591857; excerpts produced with `sed -n 'N,Mp'`)

### E1. The classifier is substring-only (issue claim: correct)
```
internal/translate/errors.go:15-53
// ClassifyEngineError sorts engine-returned error text into a user-facing category.
// Substring matching only — engine error copy is not standardized, so exact parsing is
// impractical; the cost of a misclassification is one extra generic message (the engine
// fallback), which is acceptable.
func ClassifyEngineError(errText string) string {
	lower := strings.ToLower(errText)
	switch {
	case containsAny(lower,
		"unable to translate",
		"language pair",
		"unsupported language",
		"pair not",
	):
		return ErrorKindPair
	case containsAny(lower,
		"timeout",
		"connection refused",
		"connection reset",
		"no such host",
		"dial tcp",
		"network is unreachable",
		"tls",
		"proxy",
	):
		return ErrorKindNetwork
	case containsAny(lower,
		"401",
		"403",
		"unauthorized",
		"forbidden",
		"invalid api key",
		"invalid key",
		"authentication",
	):
		return ErrorKindAuth
	default:
		return ErrorKindEngine
	}
}
```

### E2. Translate window: the failure payload is emitted with `error_kind` (issue claim: correct)
```
internal/translate/service.go:330-354
		go func(reg engine.Translator, name string) {
			res, err := s.translateWithEngine(reg, name, req)
			if err != nil {
				slog.Error(i18n.T("log.translate_multi_engine_failed"), slog.String("engine", name), slog.Any("error", err))
				analytics.Error("translate_failed", map[string]any{"engine": name})
				// issue #42: failures are no longer silently dropped — the same event is pushed
				// with Error/ErrorKind payload, and the frontend shows the per-engine failure
				// reason (categories per ClassifyEngineError).
				// From is left empty: the failure payload doesn't claim a "detected source
				// language", avoiding misuse of the auto-detect label.
				if s.app != nil {
					s.app.Event.Emit(events.EventTranslateResult, model.TranslateResult{
						Engine:    name,
						To:        req.To,
						Text:      req.Text,
						Error:     err.Error(),
						ErrorKind: ClassifyEngineError(err.Error()),
					})
				}
				return
			}
			s.saveHistory(res)
			if s.app != nil {
				s.app.Event.Emit(events.EventTranslateResult, *res)
			}
```
```
internal/model/model.go:182-183
	Error     string     `json:"error,omitempty"`      // Raw engine error on failure (issue #42; empty on success)
	ErrorKind string     `json:"error_kind,omitempty"` // Engine failure category (pair/network/auth/engine, issue #42)
```
```
frontend/bindings/cnb.cool/dtapp/kai/internal/model/models.ts:294-302
    /**
     * Raw engine error on failure (issue #42; empty on success)
     */
    "error"?: string;

    /**
     * Engine failure category (pair/network/auth/engine, issue #42)
     */
    "error_kind"?: string;
```

### E3. The frontend reads `errorKind`, and `failureMessage` is imported but never called (issue claim: correct)
```
frontend/src/utils/resultPane.ts:37-69
/** Single-engine fan-out result entry (TranslateResult shape; result is the translation; empty string / absent means no result). */
export type PaneResult = {
  engine?: string;
  result?: string;
  phonetic?: string;
  /** issue #42: the failure payload's raw error and category (pair/network/auth/engine); absent on success. */
  error?: string;
  errorKind?: string;
  [key: string]: unknown;
};

/**
 * User-facing copy for a failure payload (issue #42): kind → localized key (pair/network/auth get
 * actionable copy; everything else falls back to translate.failed), with the raw error detail
 * appended after " — ".
 * Pure function: t (i18n lookup) is injected by the caller.
 */
export function failureMessage(
  result: PaneResult | null | undefined,
  t: (key: string) => string,
): string {
  const generic = t('translate.failed');
  if (!result?.error) return generic;
  const key =
    result.errorKind === 'pair'
      ? 'translate.failedPair'
      : result.errorKind === 'network'
        ? 'translate.failedNetwork'
        : result.errorKind === 'auth'
          ? 'translate.failedAuth'
          : 'translate.failed';
  return `${t(key)} — ${result.error}`;
}
```
```
frontend/src/components/TranslateWindow.svelte:105-107
    resetEdits,
    failureMessage,
    type DotState,
```
`grep -rn "failureMessage(" frontend/src` returns only `utils/resultPane.ts:54` and `utils/resultPane.test.ts`. The import at TranslateWindow.svelte:106 is dead. The existing tests pin the camelCase name:
```
frontend/src/utils/resultPane.test.ts:215-217
    expect(
      failureMessage({ engine: 'apple', error: 'Unable to Translate', errorKind: 'pair' }, t),
    ).toBe('Language pair unavailable — Unable to Translate');
```

### E4. The failed state renders only generic copy, and so does the dot tooltip
The failed state is the `{:else if pane === 'failed'}` branch of the result pane body. The line numbers are from master; #95 moves them.
```
frontend/src/components/TranslateWindow.svelte:982-989
          {:else if pane === 'failed'}
            <!-- A translation was requested and the active engine failed (absent from results with
               loading already cleared) or returned an empty result: failed state (design §5). No
               retry, no retry button — retrying means the user presses the translate button again
               (re-running the whole fan-out). Never shown for an idle window (issue #81). -->
            <div class="flex h-full flex-col items-center justify-center gap-2 text-center">
              <span class="text-sm" style="color: var(--app-danger)">{t('translate.failed')}</span>
            </div>
```
```
frontend/src/components/TranslateWindow.svelte:855-862
                    title={engineName(e.value) +
                      (st === 'done'
                        ? ' · ' + t('translate.engineDone')
                        : st === 'pending'
                          ? ' · ' + t('translate.enginePending')
                          : st === 'failed'
                            ? ' · ' + t('translate.engineFailed')
                            : '')}
```
The payload is stored whole, so `error` and `error_kind` are available to the template:
```
frontend/src/components/TranslateWindow.svelte:351-354
    const offResult = onEvent(EventTranslateResult, (payload: TranslateResult) => {
      if (payload && payload.engine) {
        results = { ...results, [payload.engine]: payload };
        loading = false;
```
The Settings opener already exists (the #69 gear) and is reused:
```
frontend/src/components/TranslateWindow.svelte:123
  import { ShowSettings } from '@bindings/cnb.cool/dtapp/kai/internal/service/windowwrapper.ts';
```

### E5. Screenshot window: the backend drops the reason before it reaches the card (not in the issue)
The screenshot fan-out's failure placeholder carries no `Error`/`ErrorKind`, so `TranslateCard` has nothing to show beyond a fixed badge:
```
internal/translate/service.go:598-610
			res, err := s.translateWithEngine(reg, meta.Name, req)
			var item model.TranslateResult
			if err != nil {
				slog.Warn(i18n.T("log.translate_screenshot_engine_failed"), slog.String("engine", meta.Name), slog.Any("error", err))
				// Failures also append a placeholder card so the user can see which engine
				// didn't produce a translation.
				item = model.TranslateResult{
					Engine: meta.Name,
					From:   req.From,
					To:     req.To,
					Text:   req.Text,
					Result: "",
				}
```
```
frontend/src/components/TranslateCard.svelte:79-82
      {#if !tr?.result}
        <span class="text-[11px] font-medium text-[var(--app-danger)]"
          >{t('screenshot.translateFailed')}</span
        >
```

### E6. Engine errors lose the cause and render as `%!(EXTRA ...)` (not in the issue)
Most engines pass the cause as extra arguments to an i18n string that has no verb for it (or doubled args left over from a two-verb string). Go does not wrap the cause, and it appends `%!(EXTRA ...)` to the text:
```
internal/engine/deepl.go:96-111
	if err != nil {
		return nil, fmt.Errorf(i18n.T("err.deepl_do"), err, err)
	}
	defer resp.Body.Close()

	var dr deeplResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		return nil, fmt.Errorf(i18n.T("err.deepl_decode"), err, err)
	}
	if resp.StatusCode != http.StatusOK || len(dr.Translations) == 0 {
		msg := dr.Message
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf(i18n.T("err.deepl_api_error"), msg, msg)
	}
```
```
internal/i18n/locales/en-US.json:66-67
  "err.deepl_decode": "DeepL: failed to decode response",
  "err.deepl_do": "DeepL: failed to send request",
```
Same shape: baidu.go:97,103,110,113,116; tencent.go:147,156,168,175,178; youdao.go:104,110,117,120 (`grep -n 'i18n.T("err\.[a-z_]*"), ' internal/engine/*.go`). Proof, a scratch program run on this machine with Go 1.27:
```
fmt.Errorf("DeepL: failed to send request", inner, inner)  // inner = *net.DNSError
-> DeepL: failed to send request%!(EXTRA *net.DNSError=lookup api.deepl.com: no such host, *net.DNSError=lookup api.deepl.com: no such host)
errors.As(e, &*net.DNSError) -> false
fmt.Errorf("Google Translate: request failed (HTTP %d)", 429, "<html>body</html>", 429, "<html>body</html>")
-> Google Translate: request failed (HTTP 429)%!(EXTRA string=<html>body</html>, int=429, string=<html>body</html>)
```
Google keeps the status only as text and appends the raw body. On a gtx block, that body is an HTML page:
```
internal/engine/google.go:80-92
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(i18n.T("err.google_http"), resp.StatusCode, string(body), resp.StatusCode, string(body))
	}
```
```
internal/i18n/locales/en-US.json:82
  "err.google_http": "Google Translate: request failed (HTTP %d)",
```
DeepL decodes the body before it checks the status (E6 excerpt, lines 101-111), so a non-JSON error body loses the status completely. When DeepL does send a JSON body, `msg` replaces the status.

The SDK engines already get typed errors from their SDKs but flatten them. OpenAI keeps only `Message`, and `StatusCode` survives only for 410. Anthropic and Gemini format the error with `%s`:
```
internal/engine/openai.go:117-132
	completion, err := o.client.Chat.Completions.New(ctx, params)
	if err != nil {
		// The SDK's error type carries the status code and message for a clearer report.
		// errors.As handles wrapped errors (errorlint).
		if apiErr, ok := errors.AsType[*openai.Error](err); ok {
			// 410 Gone: OpenAI's standard response for "retired/deprecated models"; but
			// self-hosted compatible services (vLLM / ollama etc.) may also return 410 with
			// unfixed semantics. So instead of bluntly claiming "model retired", pass through
			// the API's real message and hint to check the endpoint/model match.
			if apiErr.StatusCode == http.StatusGone {
				return nil, fmt.Errorf(i18n.T("err.openai_model_gone"), o.model, apiErr.Message)
			}
			return nil, fmt.Errorf(i18n.T("err.openai_api_error"), apiErr.Message)
		}
		return nil, fmt.Errorf(i18n.T("err.openai_do"), err, err)
	}
```
```
internal/engine/anthropic.go:85-88
	msg, err := e.client.Messages.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf(i18n.T("err.anthropic_api_error"), err.Error())
	}
```
```
internal/engine/gemini.go:91-94
	resp, err := e.client.Models.GenerateContent(ctx, e.model, contents, config)
	if err != nil {
		return "", fmt.Errorf(i18n.T("err.gemini_api_error"), err.Error())
	}
```
SDK types that are available (module cache, versions from go.mod): `openai-go/v3 internal/apierror.Error{Code, Message, Param, Type string; StatusCode int}`; `anthropic-sdk-go@v1.75.0 internal/apierror.Error{StatusCode int}` with `Type() shared.ErrorType`, whose values include `authentication_error`, `permission_error`, `rate_limit_error`, `overloaded_error`, `billing_error`; `genai@v1.71.0 APIError{Code int; Message, Status string}` (a value type, not a pointer). Baidu, Tencent and Youdao report errors inside a 200 JSON body and never check the HTTP status:
```
internal/engine/baidu.go:107-114
	body, _ := io.ReadAll(resp.Body)
	var br baiduResponse
	if err := json.Unmarshal(body, &br); err != nil {
		return nil, fmt.Errorf(i18n.T("err.baidu_decode"), err, err)
	}
	if br.ErrorCode != "" {
		return nil, fmt.Errorf(i18n.T("err.baidu_api_error"), br.ErrorCode, br.ErrorMsg, br.ErrorCode, br.ErrorMsg)
	}
```

### E7. The service timeout is misclassified under the zh-CN backend locale (not in the issue)
`callEngine` wraps with `%w`, so typed errors do survive the service layer:
```
internal/translate/service.go:267-277
	case err := <-errCh:
		slog.Debug(i18n.T("log.translate_engine_cost",
			"Engine", engineName, "Ms", time.Since(start).Milliseconds(), "Ok", false,
			"Err", err.Error()))
		return nil, fmt.Errorf("%s(%s): %w", i18n.T("err.translate_failed"), engineName, err)
	case <-ctx.Done():
		slog.Debug(i18n.T("log.translate_engine_cost",
			"Engine", engineName, "Ms", time.Since(start).Milliseconds(), "Ok", false,
			"Err", ctx.Err().Error()))
		return nil, fmt.Errorf("%s(%s): %w", i18n.T("err.translate_timeout"), engineName, ctx.Err())
	}
```
```
internal/i18n/locales/zh-CN.json:139
  "err.translate_timeout": "翻译超时",
```
Under zh-CN the text is `翻译超时(google): context deadline exceeded`. It contains none of E1's network substrings, so it classifies as `engine`. In en-US it matches only because the word "timeout" is in the i18n copy.

### E8. Apple: the bridge has structured codes, but Go flattens them to text, and the detail is OS-localized
```
internal/engine/apple_darwin.go:113-137
	if tr.Code != "" {
		// Swift custom error: render user-visible copy via Go-side i18n by error code, with
		// detail as the technical context.
		// Known codes map to err.apple_<code>; unknown codes fall back to the generic engine
		// error copy, never exposing the raw key string to the user.
		var msg string
		switch tr.Code {
		case swiftbridge.BridgeErrEmptyText:
			msg = i18n.T("err.apple_empty_text")
		case swiftbridge.BridgeErrTargetRequired:
			msg = i18n.T("err.apple_target_required")
		case swiftbridge.BridgeErrNoSourceLang:
			slog.Error(i18n.T("err.apple_no_source_lang"), "from", sl, "to", tl, "detail", tr.Detail)
			msg = i18n.T("err.apple_no_source_lang")
		case swiftbridge.BridgeErrAppleTranslate:
			slog.Error(i18n.T("err.apple_translate_engine"), "from", sl, "to", tl, "detail", tr.Detail)
			msg = i18n.T("err.apple_translate_engine")
		default:
			slog.Error(i18n.T("err.apple_translate_engine"), "from", sl, "to", tl, "code", tr.Code, "detail", tr.Detail)
			msg = i18n.T("err.apple_translate_engine")
		}
		if tr.Detail != "" {
			msg = msg + " (" + tr.Detail + ")"
		}
		return nil, fmt.Errorf("%s", msg)
```
```
pkg/swiftbridge/internal/swift/apple_translate.swift:100-107
    } catch {
      // Apple system-level translation errors: uniformly shaped as
      // {"code":"apple_translate","detail":...},
      // rendered by the Go side's err.apple_translate_engine; detail carries the system's
      // localizedDescription.
      let detail = error.localizedDescription
      resultJSON = bridgeErrorJSON(code: BRIDGE_ERR_APPLE_TRANSLATE, detail: detail)
      bridgeFileLog(bridgeLogText("translate.fail", detail), level: BRIDGE_LOG_ERROR)
```
`no_source_lang` (auto-detect found no installed source pack) is a pair problem that the code already identifies, but it reaches the classifier only as text. `apple_translate` detail is `error.localizedDescription`, so the substring "unable to translate" matches only when macOS runs in English (to be verified on a non-English macOS; see Risks).

### E9. #52's unsupported-target refusal classifies as `engine`, not `pair` (not in the issue)
```
internal/engine/language_capability.go:208-211
// unsupportedTargetError is the visible refusal for a target the engine cannot translate into.
func unsupportedTargetError(engineName string, l model.Language) error {
	return errors.New(i18n.T("err.engine_unsupported_target", "engine", engineName, "lang", languageLabel(l)))
}
```
```
internal/i18n/locales/en-US.json:72
  "err.engine_unsupported_target": "{{.engine}} does not support {{.lang}} as a target language",
```
"does not support ... as a target language" matches none of E1's pair substrings.

### E10. "Not configured": where it can be detected before a call
A sentinel already exists. Four engines return it before any network call:
```
internal/engine/engine.go:97-103
// Engine-layer shared errors (single source of truth; settings/service reuse these, avoiding
// a cycle where engine imports settings)
var (
	ErrAPIKey   = fmt.Errorf(i18n.T("err.no_apikey"))
	ErrNoEngine = fmt.Errorf(i18n.T("err.no_engine"))
	ErrNoOCR    = fmt.Errorf(i18n.T("err.no_ocr"))
)
```
`grep -n "ErrAPIKey" internal/engine/*.go`: openai.go:89, baidu.go:76, tencent.go:134, youdao.go:73. DeepL returns its own unwrapped text (`deepl.go:72`, `err.deepl_missing_apikey`). Anthropic has no check at all (`err.anthropic_missing_apikey` exists in the catalogs but no code uses it). Gemini is not registered when construction fails, so an enabled Gemini with a bad config sends no event, and the pane reaches `failed` only through the 15 s fallback, with no reason:
```
internal/service/engine_wrapper.go:121-128
	if e, ok := engines["gemini"]; ok && e.Enabled {
		// Inject the global HTTP client (custom DNS/proxy/logging) so the Gemini SDK never
		// touches the useragent-wrapped global http.DefaultTransport and panics.
		e.HTTPClient = newClient(60 * time.Second)
		if g, err := engine.NewGemini(e); err == nil {
			w.registry.RegisterTranslator(g)
		}
	}
```
Saving and enabling already validate required credentials against the schema. `ValidateRequired` is the single definition of "configured":
```
internal/engine/engine.go:38-58
// ValidateRequired checks, per the engine's field schema, that fields with Required=true are
// filled in when enabling/saving. Returns the first missing field (LabelKey for the frontend
// to translate the hint); nil when all are satisfied.
// Only Required fields are validated; non-required fields with a default value pass even if empty.
func ValidateRequired(cfg *EngineConfig) *EngineFieldSchema {
	if cfg == nil {
		return nil
	}
	schema := GetEngineSchema(cfg.Engine)
	for i := range schema.Fields {
		f := &schema.Fields[i]
		if !f.Required {
			continue
		}
		if valueOfField(cfg, f.Field) == "" {
			return f
		}
	}
	return nil
}

```
```
internal/service/engine_wrapper.go:420-422
	if miss := engine.ValidateRequired(cfg); miss != nil {
		return fmt.Errorf(i18n.T("err.service_missing_field"), miss.LabelKey)
	}
```
The schema marks `api_key` as `Required: true` for deepl, openai, anthropic and gemini, and marks `api_key` and `secret` required for baidu, tencent and youdao (`engine.go:521,541,580,587,607,614,630,635,654,693`). apple and google have no required fields. An enabled row with an empty key can still reach `registerEngines` (rows written before validation existed, or edited directly), and today it registers anyway.

### E11. Existing copy
```
frontend/src/i18n/keys.ts:63-65
    failedPair: string;
    failedNetwork: string;
    failedAuth: string;
```
```
frontend/src/i18n/keys.ts:76-77
    engineFailed: string;
    failed: string;
```
```
frontend/src/i18n/en-US.ts:55-58
    failedPair:
      'Language pair unavailable — add it in System Settings > General > Language & Region',
    failedNetwork: 'Engine unreachable — check your network or proxy settings',
    failedAuth: "Authentication failed — check this engine's API key",
```
```
frontend/src/i18n/en-US.ts:69-70
    engineFailed: 'Failed',
    failed: 'Translation failed',
```
```
frontend/src/i18n/zh-CN.ts:54-56
    failedPair: '语言对不可用——请在 系统设置 > 通用 > 语言与地区 中下载该语言对',
    failedNetwork: '引擎无法访问——请检查网络或代理设置',
    failedAuth: '认证失败——请检查该引擎的 API 密钥',
```
```
frontend/src/i18n/zh-CN.ts:67-68
    engineFailed: '已失败',
    failed: '翻译失败',
```
`t(path, params)` replaces `{name}` placeholders (`frontend/src/i18n/index.svelte.ts:38-46`), so `{engine}` works in the new copy. `keys.test.ts` already enforces en/zh key parity. `titlebar.settings` ('Settings') is the existing label for opening Settings.

## Category table (from the issue; wire value = `error_kind`)
| error_kind | Category | Headline (draft, en) | Action |
|---|---|---|---|
| `not_configured` | Not configured | "No API key set for {engine}" | Settings button |
| `auth` | Invalid key (401/403, "invalid api key") | "{engine} rejected the API key" | Settings button |
| `quota` | Out of credits / quota (402; provider quota codes in follow-ups) | "{engine} account is out of credits or quota" | none |
| `rate_limit` | Rate limited (429 without a quota signal; google gtx block) | "{engine} is rate-limiting requests; try again shortly" | none |
| `unavailable` | Service down (5xx) | "{engine} is unavailable right now" | none |
| `network` | DNS, timeout, TLS, proxy, context deadline | "Can't reach {engine}: check your network or proxy" | none |
| `pair` | Language pair unsupported | apple: existing `failedPair` (System Settings hint); others: "{engine} can't translate this language pair" | none |
| `too_long` | 413/414, length errors | "Text is too long for {engine}" | none |
| (none) | Same language (#80) | owned by #80, not changed here | none |
| `engine` | Unknown | existing "Translation failed" | none |

For every category the raw detail is shown muted as a second line, after sanitizing (see D4). A failed engine that sent no payload (it hit the 15 s fallback) keeps today's generic copy.

## Acceptance criteria
1. Wireframe (Phase 2) approved by the principal. It covers the translate window's failed pane for `not_configured` (with the Settings button), `rate_limit` and `engine`, each with headline plus muted detail, drawn on #95's flat pane. It also covers the failed-dot tooltip and one failed screenshot card.
2. `ClassifyEngineError(err error) string` (its signature changes from string to error) returns the E-table kind. It checks structured signals first, in this order: `errors.Is(err, engine.ErrAPIKey)` gives `not_configured`; `errors.Is(err, engine.ErrUnsupportedPair)` gives `pair`; `errors.As(err, *engine.HTTPError)` gives its `Kind` if set, otherwise the status map (401/403 `auth`, 402 `quota`, 429 `rate_limit`, 413/414 `too_long`, 5xx `unavailable`, other `engine`); `errors.Is(err, context.DeadlineExceeded)` or `errors.As(err, net.Error)` gives `network`. Only then does it fall back to the E1 substring lists, which stay for text-only errors. Observable: `go test ./internal/translate -run TestClassifyEngineError`, table below.
3. Both fan-outs attach the reason. `TranslateMulti` and `translateAllStream` build the failure entry through one helper, `failurePayload(name, req, err) model.TranslateResult`, which sets `Error` (sanitized) and `ErrorKind`. Observable: `translateAllStream` with a registry holding a failing fake engine returns an entry whose `Error` and `ErrorKind` are non-empty (the app is nil, as in `service_xx_guard_test.go:72`).
4. No engine error text contains `%!(`, and every transport failure wraps its cause. For each HTTP engine (google, deepl, openai, anthropic, gemini, baidu, tencent, youdao) with its endpoint pointed at a closed loopback server, `Translate` returns an error where `errors.As(err, &net.Error)` is true and the text has no `%!(`. Observable: one table test in `internal/engine`.
5. Google: a non-2xx response returns an error that wraps `*engine.HTTPError{Status}`. Its text names the status and contains no response body HTML and no `q=` query. Statuses 403 and 429 set `Kind: rate_limit` (gtx has no key, so neither can mean "bad key"; the status-to-meaning mapping for gtx is to be verified, see Risks). Observable: httptest cases for 429 (HTML body), 403, 503 and 414.
6. Apple: bridge code `no_source_lang` returns an error that wraps `engine.ErrUnsupportedPair`. `apple_translate` whose detail contains "Unable to Translate" still classifies as `pair` through the text fallback. The mapping lives in an untagged pure function (for example `appleBridgeError(code, detail string) error` in `internal/engine/apple_errors.go`) so it is testable on any OS.
7. Unsupported target (E9) wraps `engine.ErrUnsupportedPair`, and its text is unchanged.
8. Not configured is detected at registration with no request made. In `registerEngines`, an enabled engine whose `engine.ValidateRequired(cfg)` reports a missing field is registered as a stub translator that returns an error wrapping `engine.ErrAPIKey` and never calls the network, replacing both the real engine and Gemini's silent skip. DeepL's and Anthropic's in-engine missing-key paths also wrap `engine.ErrAPIKey`. Observable: a `setupPrimaryEnv`-style test (real configstore in `t.TempDir`) with an enabled deepl row, an empty key, and its endpoint set to a counting httptest server. The resulting translator returns `errors.Is(err, engine.ErrAPIKey)`, and the server counts 0 hits.
9. Secrets never reach the detail. (a) At registration, every translator whose config has a non-empty `APIKey` or `Secret` is wrapped by `engine.WithSecrets(t, secrets...)`, whose errors replace those literal values with `***` in `Error()` and keep the chain through `Unwrap`, so `errors.As` still works. (b) `SanitizeDetail(s string) string`, applied once inside `failurePayload`, drops userinfo, query and fragment from every URL. It also redacts `key=`/`api_key=`/`token=`/`secret=`/`sign=` values, `Bearer <x>`, `DeepL-Auth-Key <x>` and `sk-[A-Za-z0-9_-]{8,}`, replaces an HTML body with `(HTML response)`, collapses whitespace, and truncates to 300 runes. Observable: `TestSanitizeDetail` table, and a WithSecrets test where a fake engine echoes its key and the key is absent from `Error()`.
10. Frontend reads the generated field name. `PaneResult` declares `error_kind` (not `errorKind`), and `grep -rn "errorKind" frontend/src --include=*.ts --include=*.svelte` returns nothing.
11. `failureMessage(result, t, engineLabel)` returns `{ headline: string; detail: string; action: 'settings' | null }`. It covers every kind in the table. `action` is `'settings'` for `not_configured` and `auth` only. `detail` is `result.error ?? ''`. A null result, or a result without `error`, returns the generic headline with empty detail. Observable: `resultPane.test.ts` cases built from the real snake-case payload, one per kind.
12. Translate window: the `pane === 'failed'` branch renders `failureMessage(activeResult, ...)`, with the headline in `--app-danger`, the detail as a muted second line (omitted when empty), and a button labelled `t('titlebar.settings')` that calls `ShowSettings()` when `action === 'settings'`. The failed dot's `title`/`aria-label` use `engineName(e) · headline` when that engine has a payload, and fall back to `translate.engineFailed` otherwise.
13. Screenshot window: `TranslateCard` shows the headline in place of `screenshot.translateFailed` when `tr.error` is set, and shows the detail muted (placement per the approved wireframe). `screenshot.translateFailed` stays for entries without `error`.
14. i18n: new keys in `keys.ts`, `en-US.ts` and `zh-CN.ts`: `translate.failedNotConfigured`, `translate.failedQuota`, `translate.failedRateLimit`, `translate.failedUnavailable`, `translate.failedTooLong`, `translate.failedUnsupported`. `failedAuth` and `failedNetwork` change value to take `{engine}`. `failedPair` is unchanged and apple-only. `keys.test.ts` stays green.
15. Logs keep the detail (#42 acceptance): `slog.Error(... "error", err)` in both fan-outs is unchanged, apart from the WithSecrets redaction.
16. Full authoritative suite green (CLAUDE.md command, `NODE_OPTIONS=--no-experimental-webstorage`), and `tsc --noEmit` clean.

Classifier table for AC2 (each wrapped as `callEngine` does, `fmt.Errorf("%s(%s): %w", prefix, name, err)`, with the zh-CN prefix `翻译失败`/`翻译超时`):
| input | want |
|---|---|
| `engine.ErrAPIKey` | not_configured |
| stub/deepl/anthropic missing key (wraps ErrAPIKey) | not_configured |
| `&HTTPError{Status: 401}` / `403` | auth |
| `&HTTPError{Status: 402}` | quota |
| `&HTTPError{Status: 429}` | rate_limit |
| `&HTTPError{Status: 403, Kind: rate_limit}` | rate_limit |
| `&HTTPError{Status: 503}` | unavailable |
| `&HTTPError{Status: 414}` | too_long |
| `context.DeadlineExceeded` under prefix `翻译超时` | network (RED today: engine) |
| `*url.Error{Err: *net.OpError}` | network |
| wraps `ErrUnsupportedPair` (apple no_source_lang, E9 refusal) | pair (E9 RED today: engine) |
| text `[动态桥接] 系统翻译失败: 引擎返回错误 (Unable to Translate)` | pair (fallback kept) |
| text `401 Unauthorized: Invalid API key` | auth (fallback kept) |
| text `some unexpected engine failure` | engine |

## Files
Production:
- `internal/model/model.go`: move the `ErrorKind*` constants here (engine and translate both import model; engine cannot import translate), and add the new kinds. Update the `ErrorKind` field comment, then regenerate bindings (comment-only change in `models.ts`).
- `internal/engine/errors.go` (new): `HTTPError{Status int; Code, Message, Kind string}` with `Error()`, the `ErrUnsupportedPair` sentinel, `WithSecrets`, and the not-configured stub.
- `internal/engine/apple_errors.go` (new, untagged): bridge code mapping. `apple_darwin.go` calls it.
- `internal/engine/{google,deepl,openai,anthropic,gemini,baidu,tencent,youdao}.go`: wrap causes with `%w` and drop the doubled args (all engines). google also returns `HTTPError`. deepl and anthropic missing-key paths wrap `ErrAPIKey`.
- `internal/engine/language_capability.go`: `unsupportedTargetError` wraps `ErrUnsupportedPair`.
- `internal/translate/errors.go`: `ClassifyEngineError(error)` and `SanitizeDetail`.
- `internal/translate/service.go`: `failurePayload`, used at both failure sites.
- `internal/service/engine_wrapper.go`: `registerEngines` registers the stub on a `ValidateRequired` miss, and wraps credentialed engines with `WithSecrets`.
- i18n: `internal/i18n/locales/*.json` only if the `%w` fix needs a verb in a string (prefer `"%s: %w"` in code with the catalog unchanged).
- `frontend/src/utils/resultPane.ts`: `PaneResult.error_kind` and the new `failureMessage`.
- `frontend/src/components/TranslateWindow.svelte`: the failed branch, and the dot title/aria-label.
- `frontend/src/components/TranslateCard.svelte`.
- `frontend/src/i18n/{keys,en-US,zh-CN}.ts`.

Tests:
- `internal/translate/errors_test.go` (rewrite the table) and `internal/translate/sanitize_test.go`.
- A service test for `translateAllStream` or `failurePayload`.
- `internal/engine/errors_test.go` (transport table, WithSecrets, stub), `internal/engine/google_errors_test.go` and `internal/engine/apple_errors_test.go`.
- `internal/service/engine_wrapper_notconfigured_test.go`.
- `frontend/src/utils/resultPane.test.ts` (rewrite the failureMessage block to snake case).
- `frontend/src/components/failureSurfacing.test.ts` (source-contract, in the style of `translateWindowGear.test.ts`): both windows render through `failureMessage`, and there is no `errorKind`.

Reuse map (extend, do not duplicate):
- `ClassifyEngineError` becomes the one classifier (no second one in the frontend or in the engines). Engines only attach structured facts: status, code and an optional `Kind`.
- `engine.ErrAPIKey` is the not-configured sentinel (no new one).
- `engine.ValidateRequired` is the single definition of "configured".
- `failureMessage` is the one renderer for both windows and the dot.
- The existing `failedPair`/`failedNetwork`/`failedAuth` keys are kept and extended.
- `ShowSettings` and `titlebar.settings` are reused (no new Go API, no new label).
- `restoreSession` already drops failure payloads (`translateSession.test.ts:98-110`). Keep it that way.

## Decisions already made (MO)
- D1, typed error shape. There is one struct, `engine.HTTPError{Status, Code, Message, Kind}`, wrapped with `%w` and read with `errors.As`, plus two sentinels (`ErrAPIKey`, existing; `ErrUnsupportedPair`, new) read with `errors.Is`. `Kind` is an optional override, set only by an engine that knows its provider better than the status map (google 403/429). The kind constants live in `internal/model`. Text-only errors (Apple `apple_translate`, anything unwrapped) keep the substring fallback.
- D2, where "not configured" is detected: at registration, from `ValidateRequired`, and never by a network call. The in-engine checks stay as a second net and wrap the same sentinel.
- D3, wire name: the frontend reads `error_kind`, the generated binding name. No camelCase mapping layer. `PaneResult` mirrors the binding.
- D4, secrets: two layers. Literal redaction of the engine's own configured secrets (`WithSecrets`, at registration, which covers logs too), and pattern sanitizing plus truncation at the single emit point (`failurePayload`). Logs are not otherwise changed.
- D5, scope (one PR). In this story: the whole classification pipeline, both windows, the dot, i18n, and the engine-agnostic fixes that apply to every engine because this story starts showing `err.Error()` to users. Those fixes are `%w` wrapping (no `%!(`), network/timeout detection, not-configured at registration, unsupported-target as `pair`, and secret redaction. The provider-specific status/code parsing in this story covers only **google** (status to `HTTPError`) and **apple** (bridge code to sentinel), the two engines configured in the dev data dir. Until the follow-ups land, the other engines still get `not_configured`, `network`, `pair` and `auth` (the text fallback catches "401"/"unauthorized"), and everything else shows as `engine` with a readable, sanitized detail.
- D6, follow-up stories (open after merge, one per group, each with httptest fixtures that cite the provider's documented error shape):
  (a) openai, anthropic and gemini: map the SDK error types listed in E6 (openai `StatusCode`/`Code`/`Type`, where `insufficient_quota` means quota; anthropic `Type()`, where `billing_error` means quota and `overloaded_error` means unavailable; the "credit balance is too low" message is to be verified; genai `Code`/`Status`, where `RESOURCE_EXHAUSTED` means quota or rate limit, to be verified against the Gemini docs).
  (b) deepl: check the status before decoding. DeepL documents 456 as "quota exceeded"; the body shape is to be verified.
  (c) baidu, tencent and youdao: map their in-body error codes (to be verified against each provider's error-code table).
  (d) apple: have the Swift bridge emit the `TranslationError` case identifier instead of `localizedDescription`, so pair detection no longer depends on the macOS language (the case names are to be verified against Apple's Translation docs).
- D7: #95 lands first, and F rebases onto it. Reference the failed state by its `pane === 'failed'` branch, not by line numbers.
- D8: no automatic retry, and no change to which engines run (issue out of scope). The same-language Apple failure stays with #80. Until #80 lands it shows the Apple `pair` copy.
- D9: copy is a draft. The principal approves the final en/zh wording with the wireframe.

## Out of scope
- Provider-specific parsing for deepl, openai, anthropic, gemini, baidu, tencent and youdao (D6 follow-ups).
- The Swift bridge change (D6d).
- #80's same-language behaviour.
- Retries.
- Proxy hint logic (#42 proposal, not in #96).
- The OCR flow error `ScreenshotResult.error` (already shown, ScreenshotWindow.svelte:352-362).
- The Settings page itself.

## Test plan
RED first (Tester), each failing today for the stated reason:
- Go, `TestClassifyEngineError`: does not compile against `ClassifyEngineError(error)` and the new kinds. Once the signature exists, the zh-CN timeout and E9 rows fail (`engine`).
- Go, the transport table: `errors.As(net.Error)` is false and the text contains `%!(EXTRA` for deepl, baidu, tencent, youdao and openai (E6).
- Go, google: 429 returns no `*HTTPError`, and the text contains HTML and `%!(`.
- Go, apple: `appleBridgeError` does not exist.
- Go, not-configured: deepl with an empty key registers the real engine; the error is not `ErrAPIKey`.
- Go, `translateAllStream`: the entry has an empty `Error`/`ErrorKind` (E5).
- Go, `SanitizeDetail` and `WithSecrets`: undefined.
- Vitest: `failureMessage` returns a string and ignores `error_kind` (E3). The source-contract fails: the failed branch has no `failureMessage(`, and `TranslateCard` has none either.

Mutation checks: reverting the snake-case read, dropping `%w` in any one engine, or removing `failurePayload` from the screenshot path must each fail a test. GREEN after F, with the full authoritative suite.

## Risks
- The google gtx error semantics are undocumented (it is an unofficial endpoint). Mapping 403 and 429 to `rate_limit` is a judgement call, to be verified by observation. It must not be verified by hammering the live endpoint.
- The Apple substring fallback works only on English macOS (E8). Non-English systems get `engine` for pair failures until D6d.
- The registration stub makes a misconfigured engine appear in `GetEngines` (registered) where it was absent before. The frontend already shows its dot from the config list, so the only visible change is that it now reports `not_configured` instead of timing out. Check `enabledTranslatorNames` analytics (service.go:527) does not start counting stubs as working engines. If it does, exclude stubs there.
- `WithSecrets` must keep the error chain (`Unwrap`), or the classifier degrades to text for every credentialed engine. The AC9 test covers this.
- Truncating to 300 runes can cut a useful provider message. Logs keep the full (secret-redacted) text.
- Regenerating the bindings touches comment-only drift in other files. Keep only the `model` change, as #52 did (decisions.md).
