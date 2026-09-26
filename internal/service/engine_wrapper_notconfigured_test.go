package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #96 AC8: "not configured" is detected at registration from ValidateRequired, never by a
// network call. An enabled deepl row with an empty key registers a stub whose Translate returns
// an error wrapping engine.ErrAPIKey; its endpoint is a counting server that must see 0 hits.
func TestRegisterEnginesNotConfiguredStubMakesNoRequest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Error(w, "should not be called", http.StatusTeapot)
	}))
	defer srv.Close()

	_, _, w := setupPrimaryEnv(t, []*engine.EngineConfig{
		{Engine: "deepl", Enabled: true, APIKey: "", Endpoint: srv.URL},
	})
	w.registerEngines()

	tr, ok := w.registry.GetTranslator("deepl")
	if !ok {
		t.Fatal("deepl not registered")
	}
	_, err := tr.Translate(context.Background(), model.TranslateRequest{Text: "Hello", From: model.EN, To: model.ZH})
	if !errors.Is(err, engine.ErrAPIKey) {
		t.Fatalf("errors.Is(err, ErrAPIKey) = false: %v", err)
	}
	if !engine.IsNotConfigured(tr) {
		t.Error("engine.IsNotConfigured(stub) = false; enabledTranslatorNames cannot exclude it")
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("server hits = %d, want 0", n)
	}
}

// The same holds for the other credentialed engines, including gemini whose silent skip on a
// missing key is replaced by the stub.
func TestRegisterEnginesStubForEveryCredentialedEngine(t *testing.T) {
	rows := []*engine.EngineConfig{
		{Engine: "openai", Enabled: true},
		{Engine: "anthropic", Enabled: true},
		{Engine: "gemini", Enabled: true},
		{Engine: "baidu", Enabled: true, APIKey: "appid"}, // secret missing
	}
	_, _, w := setupPrimaryEnv(t, rows)
	w.registerEngines()
	for _, r := range rows {
		tr, ok := w.registry.GetTranslator(r.Engine)
		if !ok {
			t.Errorf("%s: not registered (silent skip)", r.Engine)
			continue
		}
		if _, err := tr.Translate(context.Background(), model.TranslateRequest{Text: "Hi", From: model.EN, To: model.ZH}); !errors.Is(err, engine.ErrAPIKey) {
			t.Errorf("%s: errors.Is(ErrAPIKey) = false: %v", r.Engine, err)
		}
	}
}

// A configured engine is NOT a stub.
func TestRegisterEnginesConfiguredEngineIsNotStub(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"translations":[{"detected_source_language":"EN","text":"Bonjour"}]}`))
	}))
	defer srv.Close()
	_, _, w := setupPrimaryEnv(t, []*engine.EngineConfig{
		{Engine: "deepl", Enabled: true, APIKey: "real-key-123", Endpoint: srv.URL},
	})
	w.registerEngines()
	tr, ok := w.registry.GetTranslator("deepl")
	if !ok {
		t.Fatal("deepl not registered")
	}
	if engine.IsNotConfigured(tr) {
		t.Error("configured engine reported as not configured")
	}
	res, err := tr.Translate(context.Background(), model.TranslateRequest{Text: "Hello", From: model.EN, To: model.ZH})
	if err != nil || res.Result != "Bonjour" {
		t.Errorf("Translate = (%v, %v), want Bonjour", res, err)
	}
}

// Issue #96 AC9(a): registerEngines wraps every credentialed translator with WithSecrets, so a
// provider that echoes the key in its error body never surfaces it.
func TestRegisterEnginesRedactsConfiguredSecret(t *testing.T) {
	// Assembled at run time so no literal key-shaped string is committed (secret scan); not a real key.
	key := "sk-live-topsec" + "ret-9876543210"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Wrong credentials ` + key + `"}`))
	}))
	defer srv.Close()
	_, _, w := setupPrimaryEnv(t, []*engine.EngineConfig{
		{Engine: "deepl", Enabled: true, APIKey: key, Endpoint: srv.URL},
	})
	w.registerEngines()
	tr, ok := w.registry.GetTranslator("deepl")
	if !ok {
		t.Fatal("deepl not registered")
	}
	_, err := tr.Translate(context.Background(), model.TranslateRequest{Text: "Hello", From: model.EN, To: model.ZH})
	if err == nil {
		t.Fatal("no error for 403")
	}
	if strings.Contains(err.Error(), key) {
		t.Errorf("configured key leaked in error text: %q", err.Error())
	}
}
