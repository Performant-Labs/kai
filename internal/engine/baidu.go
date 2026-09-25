package engine

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// baiduTranslator is the Baidu Translate engine (needs appid + key).
// Config: APIKey=appid, Secret=secret key.
type baiduTranslator struct {
	appID    string
	appKey   string
	endpoint string
	client   *http.Client
}

// NewBaidu creates the Baidu Translate engine.
func NewBaidu(cfg *EngineConfig, client *http.Client) Translator {
	ep := cfg.Endpoint
	if ep == "" {
		ep = BaiduDefaultEndpoint
	}
	return &baiduTranslator{
		appID:    cfg.APIKey,
		appKey:   cfg.Secret,
		endpoint: ep,
		client:   client,
	}
}

func (b *baiduTranslator) Name() string { return "baidu" }

// baiduLang maps a SOURCE language to Baidu's code (a lookup into the language capability
// registry; dialects alias to their base, so es-MX → spa and pt-BR → pt, never a case-mangled
// pt-br).
func baiduLang(code string) string {
	if isAuto(code) {
		return "auto"
	}
	return sourceCode("baidu", code, strings.ToLower)
}

// baiduTarget maps a TARGET language to Baidu's code. Exact match only: the dialects are not
// offered by Baidu and are refused, never sent as their base language.
func baiduTarget(code string) (string, error) {
	if isAuto(code) {
		return "auto", nil
	}
	return targetCode("baidu", code, strings.ToLower)
}

type baiduResponse struct {
	From        string `json:"from"`
	To          string `json:"to"`
	TransResult []struct {
		Src string `json:"src"`
		Dst string `json:"dst"`
	} `json:"trans_result"`
	ErrorCode string `json:"error_code"`
	ErrorMsg  string `json:"error_msg"`
}

func (b *baiduTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	if b.appID == "" || b.appKey == "" {
		return nil, ErrAPIKey
	}
	to, err := baiduTarget(string(req.To))
	if err != nil {
		return nil, err
	}
	salt := fmt.Sprintf("%d", time.Now().UnixNano())
	signRaw := b.appID + req.Text + salt + b.appKey
	sum := md5.Sum([]byte(signRaw))
	sign := hex.EncodeToString(sum[:])

	form := url.Values{}
	form.Set("q", req.Text)
	form.Set("from", baiduLang(string(req.From)))
	form.Set("to", to)
	form.Set("appid", b.appID)
	form.Set("salt", salt)
	form.Set("sign", sign)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, b.endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf(i18n.T("err.baidu_request"), err, err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("err.baidu_do"), err, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var br baiduResponse
	if err := json.Unmarshal(body, &br); err != nil {
		return nil, fmt.Errorf(i18n.T("err.baidu_decode"), err, err)
	}
	if br.ErrorCode != "" {
		return nil, fmt.Errorf(i18n.T("err.baidu_api_error"), br.ErrorCode, br.ErrorMsg, br.ErrorCode, br.ErrorMsg)
	}
	if len(br.TransResult) == 0 {
		return nil, fmt.Errorf(i18n.T("err.baidu_empty_result"), string(body), string(body))
	}
	from := model.Language(br.From)
	if from == "" {
		from = echoLanguage(req.From)
	}
	return &model.TranslateResult{
		Engine: "baidu",
		From:   from,
		To:     req.To,
		Text:   req.Text,
		Result: br.TransResult[0].Dst,
	}, nil
}
