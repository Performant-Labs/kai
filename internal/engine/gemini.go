package engine

import (
	"context"
	"fmt"
	"strings"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"

	genai "google.golang.org/genai"
)

// geminiTranslator is the Google Gemini translation engine, built on the official
// google.golang.org/genai SDK.
type geminiTranslator struct {
	client *genai.Client
	model  string
}

// NewGemini constructs the Gemini engine from the engine config.
// ClientConfig receives both APIKey and the global HTTPClient: the new SDK correctly injects
// the key into requests made by the custom HTTPClient (the old SDK dropped the key with a
// custom HTTPClient, causing "API key is required"). The global client must be injected
// (cfg.HTTPClient comes from the service layer), otherwise an error is returned — no bare
// direct connections, preserving the project's DNS/proxy/logging network policy.
func NewGemini(cfg *EngineConfig) (*geminiTranslator, error) {
	ex := parseLLMExtra(cfg.Extra)
	if cfg.HTTPClient == nil {
		return nil, fmt.Errorf(i18n.T("err.gemini_uninitialized"))
	}
	// The global client carries no deadline of its own, and the SDK adds none (its per-request
	// timeout option is unset): a request ends when the model answers or the caller cancels the
	// ctx (issue #109).
	cc := &genai.ClientConfig{
		APIKey:     cfg.APIKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: cfg.HTTPClient,
	}
	// Only override when the user explicitly configured a non-default Base URL (Endpoint
	// stores the full Base URL).
	if cfg.Endpoint != "" && cfg.Endpoint != GeminiDefaultEndpoint {
		cc.HTTPOptions.BaseURL = cfg.Endpoint
	}

	client, err := genai.NewClient(context.Background(), cc)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("err.gemini_client"), err)
	}

	model := ex.Model
	if model == "" {
		model = "gemini-1.5-flash"
	}
	return &geminiTranslator{
		client: client,
		model:  model,
	}, nil
}

// Name returns the engine identifier.
func (e *geminiTranslator) Name() string { return "gemini" }

func (e *geminiTranslator) translate(ctx context.Context, text, from, to string) (string, error) {
	if e.model == "" {
		return "", fmt.Errorf(i18n.T("err.gemini_model_required"))
	}

	system := i18n.T("engine.openai_system")
	userPrompt := i18n.T("engine.openai_prompt")
	userContent := fmt.Sprintf(userPrompt, srcName(from), dstName(to), text)

	contents := []*genai.Content{
		{Role: genai.RoleUser, Parts: []*genai.Part{{Text: userContent}}},
	}
	config := &genai.GenerateContentConfig{
		SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: system}}},
		MaxOutputTokens:   8192,
	}

	resp, err := e.client.Models.GenerateContent(ctx, e.model, contents, config)
	if err != nil {
		// The SDK's error is kept in the chain (its transport cause, and later its typed API
		// error) while the message stays what it was.
		return "", withText(fmt.Sprintf(i18n.T("err.gemini_api_error"), err.Error()), err)
	}
	result := resp.Text()
	if strings.TrimSpace(result) == "" {
		return "", fmt.Errorf(i18n.T("err.gemini_empty"))
	}
	return result, nil
}

func (e *geminiTranslator) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	from := string(req.From)
	to := string(req.To)
	if from == "" || from == "auto" {
		from = "auto"
	}
	result, err := e.translate(ctx, req.Text, from, to)
	if err != nil {
		return nil, err
	}
	return &model.TranslateResult{
		Engine: "gemini",
		From:   req.From,
		To:     req.To,
		Text:   req.Text,
		Result: strings.TrimSpace(result),
	}, nil
}
