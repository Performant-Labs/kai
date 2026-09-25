package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// issue #7 (Tester role, RED): the google engine must honor cfg.Endpoint, falling back to
// the current default endpoint DefaultEndpoint when left empty; requests hit a real loopback
// httptest.Server (no client/transport mocks), and the gtx-shaped response decodes into
// TranslateResult.From.

// gtxFixture is a Google /translate_a/single?client=gtx response shape matching the read
// contract of googleResponse.UnmarshalJSON:
//
//	root[0] = translation-segment array, each segment [dst, src, null, null, N] with dst at [0];
//	root[2] = the detected source language (detectedLang).
//
// The translation = all segments' dst concatenated = "Hello world!".
const gtxFixture = `[[["Hello","Bonjour","","","0"],[" world!","le monde","","","1"]],null,"en"]`

// startGtxServer starts a loopback httptest.Server that records the received query params
// into got (*url.Values) and replies with a gtx-shaped response (detected lang = "en",
// translation "Hello world!").
func startGtxServer(t *testing.T, got *url.Values) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Query().Get("client") != "gtx" {
			http.Error(w, "missing client=gtx", http.StatusBadRequest)
			return
		}
		if got != nil {
			*got = r.URL.Query()
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxFixture))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestGoogleHonorsConfiguredEndpoint: when cfg.Endpoint points at the loopback, NewGoogle
// must build the engine with it and Translate's real request must hit that server (if the
// engine ignored the endpoint, the request would go to https://translate.googleapis.com and
// the loopback would see nothing → the assertion fails). TranslateResult.From must come from
// the gtx response's detected language "en".
func TestGoogleHonorsConfiguredEndpoint(t *testing.T) {
	var got url.Values
	srv := startGtxServer(t, &got)

	cfg := &EngineConfig{Endpoint: srv.URL}
	tr := NewGoogle(cfg.Endpoint, http.DefaultClient)
	if tr == nil {
		t.Fatal("NewGoogle returned nil")
	}

	res, err := tr.Translate(context.Background(), model.TranslateRequest{
		Text: "Bonjour le monde",
		From: model.Auto,
		To:   model.EN,
	})
	if err != nil {
		t.Fatalf("Translate failed (engine may not honor cfg.Endpoint; request never reached the loopback): %v", err)
	}
	if got == nil {
		t.Fatal("loopback server received no requests: engine did not use cfg.Endpoint")
	}
	if q := got.Get("sl"); q != "auto" {
		t.Errorf("gtx request sl should be auto (source language auto), got %q", q)
	}
	if q := got.Get("tl"); q != "en" {
		t.Errorf("gtx request tl should be en, got %q", q)
	}
	if q := got.Get("q"); q != "Bonjour le monde" {
		t.Errorf("gtx request q should echo the source text, got %q", q)
	}
	if res.From != model.EN {
		t.Errorf("TranslateResult.From should come from the gtx response's detected language en, got %q", res.From)
	}
	if res.Result != "Hello world!" {
		t.Errorf("translated text should be Hello world!, got %q", res.Result)
	}
	if res.Engine != "google" {
		t.Errorf("Engine should be google, got %q", res.Engine)
	}
	if res.To != model.EN {
		t.Errorf("To should stay the requested en, got %q", res.To)
	}
}

// TestGoogleEmptyEndpointFallsBackToDefault: when cfg.Endpoint is left empty, NewGoogle must
// fall back to the current default endpoint (engine.DefaultEndpoint).
// No real request is made here (that would need internet); the test only verifies the
// fallback engine's internal endpoint equals DefaultEndpoint.
func TestGoogleEmptyEndpointFallsBackToDefault(t *testing.T) {
	// Empty Endpoint: cfg.Endpoint == ""; NewGoogle must fall back to DefaultEndpoint.
	cfg := &EngineConfig{}
	tr := NewGoogle(cfg.Endpoint, http.DefaultClient)
	g, ok := tr.(*googleTranslator)
	if !ok {
		t.Fatalf("NewGoogle should return *googleTranslator, got %T", tr)
	}
	if g.endpoint != DefaultEndpoint {
		t.Errorf("empty cfg.Endpoint should fall back to default endpoint %s, got %q", DefaultEndpoint, g.endpoint)
	}
}

// TestGoogleZHCodeNormalisation: the gtx endpoint's stability requirement around "zh" —
// googleLang must normalize zh / zh-CN / zh_CN into zh-CN for the sl parameter.
// Verified by capturing the real request params on the loopback (no transport mock).
func TestGoogleZHCodeNormalisation(t *testing.T) {
	for _, from := range []model.Language{model.ZH} {
		t.Run(string(from), func(t *testing.T) {
			var got url.Values
			srv := startGtxServer(t, &got)
			cfg := &EngineConfig{Endpoint: srv.URL}
			tr := NewGoogle(cfg.Endpoint, http.DefaultClient)
			if _, err := tr.Translate(context.Background(), model.TranslateRequest{
				Text: "hello",
				From: from,
				To:   model.EN,
			}); err != nil {
				t.Fatalf("Translate failed: %v", err)
			}
			if got == nil {
				t.Fatal("loopback server received no requests")
			}
			if q := got.Get("sl"); q != "zh-CN" {
				t.Errorf("sl should be normalized to zh-CN, got %q", q)
			}
		})
	}
}
