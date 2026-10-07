package translate

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #48 over the wire: RetranslateWithContext through a real OpenAI engine pointed at a
// loopback server, nothing faked between the service and the HTTP request. The fake-engine tests
// in context_translate_test.go pin the decisions; these pin that the text, the previous
// translation and the context really reach the model, and that the answer really comes back.

type chatCapture struct {
	mu   sync.Mutex
	body string
	hits int
}

func (c *chatCapture) handler(status int, reply string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.body, c.hits = string(b), c.hits+1
		c.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}
}

func (c *chatCapture) snapshot() (string, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.body, c.hits
}

func openAIReply(text string) string {
	b, _ := json.Marshal(map[string]any{
		"id": "x", "object": "chat.completion",
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": text}}},
	})
	return string(b)
}

func newContextHTTPService(t *testing.T, srv *httptest.Server, fc *fakeCorrector, extra ...engine.Translator) *Service {
	t.Helper()
	reg := engine.NewRegistry()
	reg.RegisterTranslator(engine.NewOpenAI(&engine.EngineConfig{APIKey: "sk-test", Endpoint: srv.URL}, srv.Client()))
	for _, e := range extra {
		reg.RegisterTranslator(e)
	}
	svc := NewService(reg, nil, nil, nil)
	if fc != nil {
		svc.corrector = fc
	} else {
		svc.corrector = nil
	}
	return svc
}

func TestContextTranslateOverHTTPSendsTextPreviousAndContextAndReturnsTheAnswer(t *testing.T) {
	cap := &chatCapture{}
	srv := httptest.NewServer(cap.handler(http.StatusOK, openAIReply("  I need tomorrow's meeting  ")))
	defer srv.Close()
	svc := newContextHTTPService(t, srv, nil)

	got := svc.RetranslateWithContext(ctxReq("openai", "junta means a meeting", "be formal"))
	if got.Status != model.ContextTranslateOK || got.Result != "I need tomorrow's meeting" || got.Engine != "openai" || got.Fallback {
		t.Fatalf("got %+v, want ok from openai", got)
	}
	body, hits := cap.snapshot()
	if hits != 1 {
		t.Fatalf("the server was hit %d times, want 1", hits)
	}
	for _, want := range []string{"Necesito la junta de mañana", "I need the board of tomorrow", "junta means a meeting", "be formal", "Mexican Spanish"} {
		if !strings.Contains(body, want) {
			t.Errorf("the request body lacks %q:\n%s", want, body)
		}
	}
}

func TestContextTranslateOverHTTPUsesTheCloudEngineWhenSystemIsSelected(t *testing.T) {
	cap := &chatCapture{}
	srv := httptest.NewServer(cap.handler(http.StatusOK, openAIReply("cloud answer")))
	defer srv.Close()
	// The Translation framework cannot be put behind a loopback server (it is in-process), and the
	// on-device model is unavailable here: only the cloud engine is real.
	svc := newContextHTTPService(t, srv, &fakeCorrector{status: engine.CorrectionAppleIntelligenceOff}, &fakeSystem{name: "apple"})

	got := svc.RetranslateWithContext(ctxReq("apple", "be formal"))
	if got.Status != model.ContextTranslateOK || got.Result != "cloud answer" || got.Engine != "openai" || !got.Fallback {
		t.Fatalf("got %+v, want openai as a fallback", got)
	}
	if _, hits := cap.snapshot(); hits != 1 {
		t.Errorf("the server was hit %d times, want 1", hits)
	}
}

func TestContextTranslateOverHTTPFailureCarriesNoProviderTextOrInput(t *testing.T) {
	cap := &chatCapture{}
	srv := httptest.NewServer(cap.handler(http.StatusInternalServerError, `{"error":{"message":"boom: Necesito la junta de mañana"}}`))
	defer srv.Close()
	svc := newContextHTTPService(t, srv, nil)

	got := svc.RetranslateWithContext(ctxReq("openai", "be formal"))
	if got.Status != model.ContextTranslateFailed || got.Result != "" {
		t.Fatalf("got %+v, want failed", got)
	}
	// The answer is fixed text: neither the provider's message nor the user's text can come back in it.
	if strings.Contains(got.Error, "boom") || strings.Contains(got.Error, "junta") {
		t.Errorf("the error carries provider or user text: %q", got.Error)
	}
}
