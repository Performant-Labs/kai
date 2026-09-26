package engine

import (
	"context"
	"encoding/json"
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

// gtxParagraphsBody is a gtx en->fr response captured live on 2026-09-26 (issue #144) for
// "Hello, world. It works!\n\nSecond paragraph; line one\nline two.": the segment dst strings
// carry the blank line and the line break.
const gtxParagraphsBody = `[[["Bonjour le monde. ","Hello, world. ",null,null,10],["Ça marche!\n\n","It works!\n\n",null,null,10],["Deuxième paragraphe ; ","Second paragraph; ",null,null,3],["première ligne\n","line one\n",null,null,3],["ligne deux.","line two.",null,null,3]],null,"en",null,null,null,null,[]]`

// Regression guard (#144): the gtx parser keeps every paragraph break and line break of a real
// response byte for byte (the display bug was in the frontend, not here).
func TestGoogleParserKeepsParagraphBreaks(t *testing.T) {
	var r googleResponse
	if err := json.Unmarshal([]byte(gtxParagraphsBody), &r); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	const want = "Bonjour le monde. Ça marche!\n\nDeuxième paragraphe ; première ligne\nligne deux."
	if r.Translated != want {
		t.Errorf("Translated = %q, want %q", r.Translated, want)
	}
	if r.DetectedLang != "en" {
		t.Errorf("DetectedLang = %q, want en", r.DetectedLang)
	}

	// Synthetic (not captured live): breaks at both ends of the text are kept too, so a
	// strings.TrimSpace (or any whitespace collapse) added to the parser fails here.
	var edge googleResponse
	body := `[[["\nUn.\n\n","\nOne.\n\n",null,null,3],["Deux.\n","Two.\n",null,null,3]],null,"en"]`
	if err := json.Unmarshal([]byte(body), &edge); err != nil {
		t.Fatalf("Unmarshal (edge): %v", err)
	}
	if want := "\nUn.\n\nDeux.\n"; edge.Translated != want {
		t.Errorf("Translated (edge) = %q, want %q", edge.Translated, want)
	}
}
