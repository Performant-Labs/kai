package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Issue #48: the LLM engines can run a system + user prompt of the caller's own (the retranslation
// in a context the user gave). System translation cannot, and must not pretend to.
var (
	_ Prompter = (*openaiTranslator)(nil)
	_ Prompter = (*anthropicTranslator)(nil)
	_ Prompter = (*geminiTranslator)(nil)
)

func TestOpenAIPromptSendsSystemAndUserAndReturnsTheAnswer(t *testing.T) {
	var got struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"  Reunión  "}}]}`))
	}))
	defer srv.Close()

	tr := NewOpenAI(&EngineConfig{APIKey: "sk-test", Endpoint: srv.URL}, srv.Client())
	p, ok := tr.(Prompter)
	if !ok {
		t.Fatal("the OpenAI engine is not a Prompter")
	}
	out, err := p.Prompt(context.Background(), "SYS", "USER")
	if err != nil {
		t.Fatal(err)
	}
	if out != "Reunión" {
		t.Errorf("answer = %q, want the trimmed model answer", out)
	}
	if len(got.Messages) != 2 || got.Messages[0].Role != "system" || got.Messages[0].Content != "SYS" ||
		got.Messages[1].Role != "user" || got.Messages[1].Content != "USER" {
		t.Errorf("messages = %+v, want system SYS then user USER", got.Messages)
	}
}

func TestOpenAIPromptWithoutKeyIsNotSent(t *testing.T) {
	tr := NewOpenAI(&EngineConfig{}, nil)
	if _, err := tr.(Prompter).Prompt(context.Background(), "s", "u"); err == nil {
		t.Fatal("a prompt without an API key was sent")
	}
}

func TestSystemTranslationIsNotAPrompter(t *testing.T) {
	if _, ok := any(&appleTranslator{}).(Prompter); ok {
		t.Fatal("the Translation framework cannot follow a prompt, it must not be a Prompter")
	}
}

// The Anthropic and Gemini engines against real loopback servers (no SDK or HTTP mocks): the
// system and user prompts reach the wire, and the model's answer comes back.

func TestAnthropicPromptSendsSystemAndUserAndReturnsTheAnswer(t *testing.T) {
	var got struct {
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"m","content":[{"type":"text","text":"Reunión"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()

	tr := NewAnthropic(&EngineConfig{APIKey: "test", Endpoint: srv.URL, HTTPClient: srv.Client()})
	out, err := tr.Prompt(context.Background(), "SYS", "USER")
	if err != nil {
		t.Fatal(err)
	}
	if out != "Reunión" {
		t.Errorf("answer = %q, want Reunión", out)
	}
	if len(got.System) != 1 || got.System[0].Text != "SYS" {
		t.Errorf("system = %+v, want SYS", got.System)
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "user" || len(got.Messages[0].Content) != 1 || got.Messages[0].Content[0].Text != "USER" {
		t.Errorf("messages = %+v, want one user message USER", got.Messages)
	}
}

func TestAnthropicPromptWithoutKeyIsNotSent(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()
	tr := NewAnthropic(&EngineConfig{Endpoint: srv.URL, HTTPClient: srv.Client()})
	if _, err := tr.Prompt(context.Background(), "s", "u"); err == nil {
		t.Fatal("a prompt without an API key was sent")
	}
	if called {
		t.Error("a request reached the server without an API key")
	}
}

func TestGeminiPromptSendsSystemAndUserAndReturnsTheAnswer(t *testing.T) {
	var got struct {
		SystemInstruction struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"systemInstruction"`
		Contents []struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"Reunión"}]},"finishReason":"STOP"}]}`))
	}))
	defer srv.Close()

	tr, err := NewGemini(&EngineConfig{APIKey: "key", Endpoint: srv.URL, HTTPClient: srv.Client()})
	if err != nil {
		t.Fatal(err)
	}
	out, err := tr.Prompt(context.Background(), "SYS", "USER")
	if err != nil {
		t.Fatal(err)
	}
	if out != "Reunión" {
		t.Errorf("answer = %q, want Reunión", out)
	}
	if len(got.SystemInstruction.Parts) != 1 || got.SystemInstruction.Parts[0].Text != "SYS" {
		t.Errorf("systemInstruction = %+v, want SYS", got.SystemInstruction)
	}
	if len(got.Contents) != 1 || len(got.Contents[0].Parts) != 1 || got.Contents[0].Parts[0].Text != "USER" {
		t.Errorf("contents = %+v, want one part USER", got.Contents)
	}
}
