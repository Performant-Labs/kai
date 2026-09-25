package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// Google's key-free public translation endpoint (the web gtx interface).
const googleEndpoint = "https://translate.googleapis.com/translate_a/single"

// googleLang maps a SOURCE language to the code the Google gtx endpoint accepts (a lookup into
// the language capability registry; dialects alias to their base). gtx also works with "zh",
// but "zh-CN" is more reliable, so the registry records that.
func googleLang(code string) string {
	if isAuto(code) {
		return "auto"
	}
	return sourceCode("google", code, identity)
}

// googleTarget maps a TARGET language to its gtx code. Exact match only: a dialect the
// registry does not list for google is refused, never sent as its base language.
func googleTarget(code string) (string, error) {
	if isAuto(code) {
		return "auto", nil
	}
	return targetCode("google", code, identity)
}

// googleTranslator is the Google translation engine (key-free).
type googleTranslator struct {
	endpoint string
	client   *http.Client
}

// NewGoogle creates the Google translation engine. An empty endpoint uses the default public one.
func NewGoogle(endpoint string, client *http.Client) Translator {
	if endpoint == "" {
		endpoint = googleEndpoint
	}
	return &googleTranslator{
		endpoint: endpoint,
		client:   client,
	}
}

func (g *googleTranslator) Name() string { return "google" }

// Translate performs the translation via Google's public endpoint.
func (g *googleTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	if req.Text == "" {
		return nil, fmt.Errorf(i18n.T("err.empty_text"))
	}
	sl := googleLang(string(req.From))
	tl, err := googleTarget(string(req.To))
	if err != nil {
		return nil, err
	}
	if tl == "auto" || tl == "" {
		tl = "zh-CN"
	}

	u := fmt.Sprintf("%s?client=gtx&sl=%s&tl=%s&dt=t&q=%s",
		g.endpoint, url.QueryEscape(sl), url.QueryEscape(tl), url.QueryEscape(req.Text))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "Mozilla/5.0")

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

	// Parse the Google gtx response: [[["dst","src",...],...], "detected_lang", ...]
	var gresp googleResponse
	if err := json.Unmarshal(body, &gresp); err != nil {
		return nil, fmt.Errorf(i18n.T("err.google_parse"), err, err)
	}

	if gresp.Translated == "" {
		return nil, fmt.Errorf(i18n.T("err.google_empty_result"))
	}

	return &model.TranslateResult{
		Engine: "google",
		From:   model.Language(gresp.DetectedLang),
		To:     req.To,
		Text:   req.Text,
		Result: gresp.Translated,
	}, nil
}

// googleResponse represents the Google gtx (dt=t) response.
// Root structure: [ translation-segment array, ..., detected source language, ... ]
// Segment array: [[dst, src, ...], ...] with dst (the translation) at [0] and src (the
// original) at [1].
// Because the response is an irregularly nested array, a custom UnmarshalJSON collapses the
// positional semantics into named fields.
type googleResponse struct {
	Translated   string
	DetectedLang string
}

func (r *googleResponse) UnmarshalJSON(data []byte) error {
	// Root level: element 0 is the translation-segment array, element 2 is the detected source language
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if len(raw) == 0 {
		return nil
	}
	var segs []json.RawMessage
	if err := json.Unmarshal(raw[0], &segs); err != nil {
		return err
	}
	var sb strings.Builder
	for _, seg := range segs {
		// Each segment is [dst, src, null, null, N, ...]; take only the dst (index 0) string field
		var pair []json.RawMessage
		if err := json.Unmarshal(seg, &pair); err != nil || len(pair) == 0 {
			continue
		}
		var s string
		if err := json.Unmarshal(pair[0], &s); err == nil && s != "" {
			sb.WriteString(s)
		}
	}
	r.Translated = sb.String()
	if len(raw) > 2 {
		var d string
		if err := json.Unmarshal(raw[2], &d); err == nil {
			r.DetectedLang = d
		}
	}
	return nil
}
