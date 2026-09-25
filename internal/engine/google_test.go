package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// TestGoogleEndpointOverrideAndDetectedLang verifies (the precondition contract for issue
// #11 auto-detection):
//  1. the endpoint override takes effect — requests hit the test server (real HTTP loopback,
//     no transport interception);
//  2. element 2 of the gtx response (detected source language) is written into
//     TranslateResult.From.
func TestGoogleEndpointOverrideAndDetectedLang(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		if !strings.Contains(r.URL.RawQuery, "client=gtx") {
			t.Errorf("expected gtx client param, got %q", r.URL.RawQuery)
		}
		if !strings.Contains(r.URL.RawQuery, "sl=auto") {
			t.Errorf("expected sl=auto, got %q", r.URL.RawQuery)
		}
		// gtx shape: [[["dst","src",null,null,N]], null, "detected language"] — the detected
		// language sits at root index 2.
		_, _ = w.Write([]byte(`[[["Hola","Hello",null,null,1]],null,"es"]`))
	}))
	defer srv.Close()

	g := NewGoogle(srv.URL, srv.Client())
	res, err := g.Translate(context.Background(), model.TranslateRequest{
		Text: "Hello",
		From: model.Language("auto"),
		To:   model.Language("en"),
	})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if !strings.Contains(gotQuery, "tl=en") {
		t.Errorf("request query = %q, want tl=en", gotQuery)
	}
	if res.Result != "Hola" {
		t.Errorf("Result = %q, want Hola", res.Result)
	}
	if res.From != model.Language("es") {
		t.Errorf("From = %q, want es (detected)", res.From)
	}
	if res.Engine != "google" {
		t.Errorf("Engine = %q, want google", res.Engine)
	}
}
