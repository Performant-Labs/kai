package engine

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// anthropicTranslator is the Anthropic Claude translation engine, built on the official
// anthropic-sdk-go.
// Config: APIKey=sk-ant-..., Endpoint=base URL (default https://api.anthropic.com),
// Extra=JSON ({"model":"claude-3-5-sonnet-20241022","timeout_sec":30}); backward compatible
// with the old plain model-name string.
type anthropicTranslator struct {
	client  anthropic.Client
	model   anthropic.Model
	timeout time.Duration
}

// NewAnthropic constructs the Anthropic engine from the engine config.
// It reuses the project-wide global HTTP client (injected by the service layer, with custom
// DNS/proxy/logging/contribution reporting), consistent with engines like openai/gemini —
// all going through the network.BuildHTTPClient network policy.
func NewAnthropic(cfg *EngineConfig) *anthropicTranslator {
	ex := parseLLMExtra(cfg.Extra)
	opts := []option.RequestOption{
		option.WithAPIKey(cfg.APIKey),
	}
	// Inject the global HTTP client, cloned into an independent instance with the
	// engine-level timeout (synced to the HTTP layer), rather than mutating the shared global
	// client's Timeout directly; on nil, fall back to the SDK default client.
	if cfg.HTTPClient != nil {
		opts = append(opts, option.WithHTTPClient(cloneHTTPClientWithTimeout(cfg.HTTPClient, ex.TimeoutSec)))
	}
	if cfg.Endpoint != "" && cfg.Endpoint != AnthropicDefaultBaseURL {
		opts = append(opts, option.WithBaseURL(cfg.Endpoint))
	}
	modelName := anthropic.Model(ex.Model) // nolint:unconvert // the conversion provides compile-time type safety
	if modelName == "" {
		modelName = "claude-3-5-sonnet-20241022"
	}
	return &anthropicTranslator{
		client:  anthropic.NewClient(opts...),
		model:   modelName,
		timeout: time.Duration(ex.TimeoutSec) * time.Second,
	}
}

// Name returns the engine identifier.
func (e *anthropicTranslator) Name() string { return "anthropic" }

func (e *anthropicTranslator) translate(ctx context.Context, text, from, to string) (string, error) {
	if e.model == "" {
		return "", fmt.Errorf(i18n.T("err.anthropic_model_required"))
	}
	// Engine-level request timeout (default 30s, configurable via Extra.timeout_sec).
	if e.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, e.timeout)
		defer cancel()
	}

	system := i18n.T("engine.openai_system")
	userPrompt := i18n.T("engine.openai_prompt")
	userContent := fmt.Sprintf(userPrompt, srcName(from), dstName(to), text)

	params := anthropic.MessageNewParams{
		Model:     e.model,
		MaxTokens: int64(8192),
		System: []anthropic.TextBlockParam{
			{Text: system},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userContent)),
		},
	}

	msg, err := e.client.Messages.New(ctx, params)
	if err != nil {
		return "", fmt.Errorf(i18n.T("err.anthropic_api_error"), err.Error())
	}

	var sb strings.Builder
	for _, block := range msg.Content {
		tb := block.AsText()
		if string(tb.Type) == "text" {
			sb.WriteString(tb.Text)
		}
	}
	return sb.String(), nil
}

func (e *anthropicTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	from := string(req.From)
	to := string(req.To)
	if from == "" || from == "auto" {
		from = "auto"
	}
	result, err := e.translate(ctx, req.Text, from, to)
	if err != nil {
		return nil, err
	}
	if result == "" {
		return nil, fmt.Errorf(i18n.T("err.anthropic_empty"))
	}
	return &model.TranslateResult{
		Engine: "anthropic",
		From:   req.From,
		To:     req.To,
		Text:   req.Text,
		Result: strings.TrimSpace(result),
	}, nil
}
