package engine

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// tencentTranslator is the Tencent Machine Translation (TMT) engine, needing SecretId +
// SecretKey.
// Config: APIKey=SecretId, Secret=SecretKey.
type tencentTranslator struct {
	secretID  string
	secretKey string
	endpoint  string
	client    *http.Client
}

// NewTencent creates the Tencent translation engine.
func NewTencent(cfg *EngineConfig, client *http.Client) Translator {
	ep := cfg.Endpoint
	if ep == "" {
		ep = TencentDefaultEndpoint
	}
	return &tencentTranslator{
		secretID:  cfg.APIKey,
		secretKey: cfg.Secret,
		endpoint:  ep,
		client:    client,
	}
}

func (t *tencentTranslator) Name() string { return "tencent" }

// tencentLang maps a SOURCE language to Tencent's code (a lookup into the language capability
// registry; dialects alias to their base).
func tencentLang(code string) string {
	if isAuto(code) {
		return "auto"
	}
	return sourceCode("tencent", code, strings.ToLower)
}

// tencentTarget maps a TARGET language to Tencent's code. Exact match only: the dialects are not
// offered by Tencent and are refused, never sent as their base language.
func tencentTarget(code string) (string, error) {
	if isAuto(code) {
		return "auto", nil
	}
	return targetCode("tencent", code, strings.ToLower)
}

type tencentRequest struct {
	SourceText string `json:"SourceText"`
	Source     string `json:"Source"`
	Target     string `json:"Target"`
	ProjectId  int64  `json:"ProjectId"`
}

type tencentResponse struct {
	Response struct {
		TargetText string `json:"TargetText"`
		Source     string `json:"Source"`
		Error      *struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		} `json:"Error"`
	} `json:"Response"`
}

// hmacSHA256 computes HMAC-SHA256 and returns the hex string.
func hmacSHA256(key []byte, data string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return hex.EncodeToString(mac.Sum(nil))
}

// sha256Hex computes the SHA256 hex digest.
func sha256Hex(data string) string {
	sum := sha256.Sum256([]byte(data))
	return hex.EncodeToString(sum[:])
}

// signTencent builds the Authorization header per the TC3-HMAC-SHA256 spec.
func signTencent(secretID, secretKey, payload, timestamp, date string) string {
	service := "tmt"
	host := "tmt.tencentcloudapi.com"
	algorithm := "TC3-HMAC-SHA256"

	hashedPayload := sha256Hex(payload)
	canonicalHeaders := fmt.Sprintf("content-type:application/json; charset=utf-8\nhost:%s\n", host)
	signedHeaders := "content-type;host"
	canonicalRequest := strings.Join([]string{
		"POST",
		"/",
		"",
		canonicalHeaders,
		signedHeaders,
		hashedPayload,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/tc3_request", date, service)
	stringToSign := strings.Join([]string{
		algorithm,
		timestamp,
		credentialScope,
		sha256Hex(canonicalRequest),
	}, "\n")

	secretDate := hmacSHA256([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256([]byte(secretDate), service)
	secretSigning := hmacSHA256([]byte(secretService), "tc3_request")
	signature := hmacSHA256([]byte(secretSigning), stringToSign)

	return fmt.Sprintf(
		"%s Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		algorithm, secretID, credentialScope, signedHeaders, signature,
	)
}

func (t *tencentTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	if t.secretID == "" || t.secretKey == "" {
		return nil, ErrAPIKey
	}
	target, err := tencentTarget(string(req.To))
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(tencentRequest{
		SourceText: req.Text,
		Source:     tencentLang(string(req.From)),
		Target:     target,
		ProjectId:  0,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("err.tencent_marshal"), err)
	}

	now := time.Now().UTC()
	timestamp := strconv.FormatInt(now.Unix(), 10)
	date := now.Format("2006-01-02")

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("err.tencent_request"), err)
	}
	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	httpReq.Header.Set("Host", "tmt.tencentcloudapi.com")
	httpReq.Header.Set("X-TC-Action", "TextTranslate")
	httpReq.Header.Set("X-TC-Version", "2018-03-21")
	httpReq.Header.Set("X-TC-Timestamp", timestamp)
	httpReq.Header.Set("X-TC-Region", "ap-guangzhou")
	httpReq.Header.Set("Authorization", signTencent(t.secretID, t.secretKey, string(body), timestamp, date))

	resp, err := t.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("err.tencent_do"), err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var tr tencentResponse
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("err.tencent_decode"), err)
	}
	if tr.Response.Error != nil {
		return nil, fmt.Errorf(i18n.T("err.tencent_api_error"), tr.Response.Error.Code, tr.Response.Error.Message)
	}
	from := model.Language(tr.Response.Source)
	if from == "" {
		from = echoLanguage(req.From)
	}
	return &model.TranslateResult{
		Engine: "tencent",
		From:   from,
		To:     req.To,
		Text:   req.Text,
		Result: tr.Response.TargetText,
	}, nil
}
