package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// The DeepL translation engine (needs an API Key). The free tier (500k chars/month) also
// requires registering for an API key, with the api-free.deepl.com endpoint; Pro uses
// api.deepl.com. Both authenticate identically — only the endpoint differs.
type deeplTranslator struct {
	endpoint string
	apiKey   string
	client   *http.Client
}

// NewDeepL creates the DeepL engine. An empty endpoint defaults to the free tier.
func NewDeepL(cfg *EngineConfig, client *http.Client) Translator {
	ep := cfg.Endpoint
	if ep == "" {
		ep = DeepLFreeEndpoint
	}
	return &deeplTranslator{
		endpoint: ep,
		apiKey:   cfg.APIKey,
		client:   client,
	}
}

// Name returns the engine identifier.
func (d *deeplTranslator) Name() string { return "deepl" }

// deeplLang maps a SOURCE language to the uppercase code DeepL accepts (ZH/EN/...), a lookup
// into the language capability registry (dialects alias to their base, so es-MX / pt-BR / pt-PT
// are sent as ES / PT). DeepL doesn't support auto; returning an empty string lets DeepL
// auto-detect the source.
func deeplLang(code string) string {
	if isAuto(code) {
		return "" // auto-detect
	}
	// Unrecognized codes: already a DeepL-style code; uppercase it as before.
	return sourceCode("deepl", code, strings.ToUpper)
}

// deeplTarget maps a TARGET language to its DeepL code. Exact match only: a dialect the registry
// does not list for deepl (es-MX) is refused, never sent as its base language. An empty result
// means "no target given" (the caller applies the default).
func deeplTarget(code string) (string, error) {
	if isAuto(code) {
		return "", nil
	}
	return targetCode("deepl", code, strings.ToUpper)
}

type deeplResponse struct {
	Translations []struct {
		DetectedSourceLanguage string `json:"detected_source_language"`
		Text                   string `json:"text"`
	} `json:"translations"`
	Message string `json:"message"`
}

func (d *deeplTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	if d.apiKey == "" {
		return nil, withText(i18n.T("err.deepl_missing_apikey"), ErrAPIKey)
	}
	form := url.Values{}
	form.Set("text", req.Text)
	tl, err := deeplTarget(string(req.To))
	if err != nil {
		return nil, err
	}
	if tl == "" {
		tl = "ZH"
	}
	form.Set("target_lang", tl)
	if sl := deeplLang(string(req.From)); sl != "" {
		form.Set("source_lang", sl)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("err.deepl_request"), err)
	}
	httpReq.Header.Set("Authorization", "DeepL-Auth-Key "+d.apiKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := d.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("err.deepl_do"), err)
	}
	defer resp.Body.Close()

	var dr deeplResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("err.deepl_decode"), err)
	}
	if resp.StatusCode != http.StatusOK || len(dr.Translations) == 0 {
		msg := dr.Message
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf(i18n.T("err.deepl_api_error"), msg)
	}

	// DeepL reports its detection upper-cased (ES, PT, ...); without one, echo the request language.
	from := model.Language(strings.ToLower(dr.Translations[0].DetectedSourceLanguage))
	if from == "" {
		from = echoLanguage(req.From)
	}
	return &model.TranslateResult{
		Engine: "deepl",
		From:   from,
		To:     req.To,
		Text:   req.Text,
		Result: dr.Translations[0].Text,
	}, nil
}
