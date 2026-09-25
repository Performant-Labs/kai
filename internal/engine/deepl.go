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

// deeplLang maps internal language codes to the uppercase codes DeepL accepts (ZH/EN/...).
// DeepL doesn't support auto; returning an empty string lets DeepL auto-detect the source.
func deeplLang(code string) string {
	switch strings.ToLower(code) {
	case "zh", "zh-cn", "zh_cn":
		return "ZH"
	case "en":
		return "EN"
	case "ja":
		return "JA"
	case "ko":
		return "KO"
	case "fr":
		return "FR"
	case "de":
		return "DE"
	case "es":
		return "ES"
	case "ru":
		return "RU"
	case "auto", "":
		return "" // auto-detect
	default:
		// Already a DeepL-style uppercase code; return as-is
		return strings.ToUpper(code)
	}
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
		return nil, fmt.Errorf(i18n.T("err.deepl_missing_apikey"))
	}
	form := url.Values{}
	form.Set("text", req.Text)
	tl := deeplLang(string(req.To))
	if tl == "" {
		tl = "ZH"
	}
	form.Set("target_lang", tl)
	if sl := deeplLang(string(req.From)); sl != "" {
		form.Set("source_lang", sl)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, d.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf(i18n.T("err.deepl_request"), err, err)
	}
	httpReq.Header.Set("Authorization", "DeepL-Auth-Key "+d.apiKey)
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := d.client.Do(httpReq)
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

	src := dr.Translations[0].DetectedSourceLanguage
	if src == "" {
		src = strings.ToLower(string(req.From))
	}
	return &model.TranslateResult{
		Engine: "deepl",
		From:   model.Language(strings.ToLower(src)),
		To:     req.To,
		Text:   req.Text,
		Result: dr.Translations[0].Text,
	}, nil
}
