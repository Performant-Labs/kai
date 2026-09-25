package engine

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
)

// OpenAI-compatible chat-API translation engine (Chat Completions), built on the official
// openai-go SDK.
// Config: APIKey=sk-..., Endpoint=Base URL (default https://api.openai.com/v1),
// Extra=JSON ({"model":"gpt-4o-mini","timeout_sec":30}); backward compatible with the old
// plain model-name string.
// Endpoint just takes the base URL (same as DeepSeek/SiliconFlow and other compatible
// platforms); the SDK appends /chat/completions itself.
type openaiTranslator struct {
	apiKey  string
	model   shared.ChatModel
	timeout time.Duration
	client  openai.Client
}

// normalizeOpenAIBaseURL normalizes the user-entered endpoint into a bare Base URL.
// The v3 SDK's WithBaseURL appends /chat/completions under the hood, so a full
// chat/completions URL the user may have entered must be stripped back to the base here,
// otherwise the path would be duplicated.
// Handles: a base (.../v1), a full URL (.../v1/chat/completions), with or without a trailing
// slash.
func normalizeOpenAIBaseURL(raw string) string {
	ep := strings.TrimSpace(raw)
	if ep == "" {
		return ""
	}
	ep = strings.TrimRight(ep, "/")
	// Already contains /chat/completions — strip it, keeping the bare base
	ep = strings.TrimSuffix(ep, "/chat/completions")
	ep = strings.TrimRight(ep, "/")
	return ep
}

// NewOpenAI creates the OpenAI-compatible translation engine.
func NewOpenAI(cfg *EngineConfig, client *http.Client) Translator {
	ex := parseLLMExtra(cfg.Extra)
	modelName := shared.ChatModel(ex.Model) // nolint:unconvert // the conversion provides compile-time type safety
	if modelName == "" {
		modelName = "gpt-4o-mini"
	}

	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	// Normalize the endpoint into a bare Base URL before handing it to the SDK (the v3 SDK's
	// WithBaseURL appends /chat/completions itself).
	// Both input styles — a base (.../v1) or a full URL (.../v1/chat/completions) — normalize
	// to the underlying base, avoiding the SDK appending again and producing the duplicated
	// /chat/completions/chat/completions path.
	if ep := normalizeOpenAIBaseURL(cfg.Endpoint); ep != "" {
		opts = append(opts, option.WithBaseURL(ep))
	}
	// Reuse the project's unified http.Client, cloned into an independent instance with the
	// engine-level timeout (synced to the HTTP layer), rather than mutating the shared global
	// client's Timeout directly.
	if client != nil {
		opts = append(opts, option.WithHTTPClient(cloneHTTPClientWithTimeout(client, ex.TimeoutSec)))
	}

	return &openaiTranslator{
		apiKey:  cfg.APIKey,
		model:   modelName,
		timeout: time.Duration(ex.TimeoutSec) * time.Second,
		client:  openai.NewClient(opts...),
	}
}

// Name returns the engine identifier.
func (o *openaiTranslator) Name() string { return "openai" }

func (o *openaiTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	if o.apiKey == "" {
		return nil, ErrAPIKey
	}
	// Engine-level request timeout (default 30s, configurable via Extra.timeout_sec).
	// If the upstream ctx expires earlier, whichever expires first wins (Go context semantics).
	if o.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.timeout)
		defer cancel()
	}
	src := string(req.From)
	dst := string(req.To)
	if src == "" || src == "auto" {
		src = srcName("auto")
	}

	prompt := fmt.Sprintf(
		i18n.T("engine.openai_prompt"),
		srcName(src), dstName(dst), req.Text,
	)

	params := openai.ChatCompletionNewParams{
		Model: o.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(i18n.T("engine.openai_system")),
			openai.UserMessage(prompt),
		},
	}

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

	if len(completion.Choices) == 0 {
		return nil, fmt.Errorf(i18n.T("err.openai_api_status"), "no choices")
	}

	msg := completion.Choices[0].Message
	result := strings.TrimSpace(msg.Content)
	// When the model refuses to generate content (safety policy etc.), Content is empty but
	// Refusal has a value; return that as the result, otherwise the frontend shows "request
	// succeeded but no result".
	if result == "" && msg.Refusal != "" {
		result = strings.TrimSpace(msg.Refusal)
	}

	return &model.TranslateResult{
		Engine: "openai",
		From:   req.From,
		To:     req.To,
		Text:   req.Text,
		Result: result,
	}, nil
}

// srcName/dstName convert internal language codes into natural-language names LLMs understand
// better (via i18n, following the UI language).
func srcName(code string) string {
	switch code {
	case "zh", "zh-cn":
		return i18n.T("lang.zh")
	case "en":
		return i18n.T("lang.en")
	case "ja":
		return i18n.T("lang.ja")
	case "ko":
		return i18n.T("lang.ko")
	default:
		return i18n.T("lang.auto")
	}
}

func dstName(code string) string {
	switch code {
	case "zh", "zh-cn":
		return i18n.T("lang.zh")
	case "en":
		return i18n.T("lang.en")
	case "ja":
		return i18n.T("lang.ja")
	case "ko":
		return i18n.T("lang.ko")
	default:
		return code
	}
}
