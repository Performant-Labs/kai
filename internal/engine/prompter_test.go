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
