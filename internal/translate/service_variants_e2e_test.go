package translate

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/langpref"
	"cnb.cool/dtapp/kai/internal/model"
)

// #43 end-to-end acceptance (Tester, RED phase). Real translate.Service -> real engine ->
// loopback httptest server (real HTTP, no transport interception). Loopback only: no live
// provider call is made (disclosed in handoff-T-red).

var variants = []model.Language{model.ESMX, model.PTBR, model.PTPT}

// Round trip, google, per variant as TARGET: the request carries the dialect code the registry
// verified and the translated text comes back with the requested variant reported as To.
func TestGoogleRoundTripPerVariantTarget(t *testing.T) {
	wantTL := map[model.Language]string{model.ESMX: "es", model.PTBR: "pt-BR", model.PTPT: "pt-PT"}
	for _, v := range variants {
		var q url.Values
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q = r.URL.Query()
			_, _ = w.Write([]byte(`[[["translated-` + string(v) + `","Hello",null,null,1]],null,"en"]`))
		}))
		reg := engine.NewRegistry()
		reg.RegisterTranslator(engine.NewGoogle(srv.URL, srv.Client()))
		svc := NewService(reg, nil, nil, nil)
		res, err := svc.Translate(model.TranslateRequest{Text: "Hello", From: model.EN, To: v, EngineName: "google"})
		srv.Close()
		if err != nil {
			t.Fatalf("%s: Translate: %v", v, err)
		}
		if got := q.Get("tl"); got != wantTL[v] {
			t.Errorf("%s: tl = %q, want %q", v, got, wantTL[v])
		}
		if res.Result != "translated-"+string(v) || res.To != v {
			t.Errorf("%s: result = %q / To = %q, want translated text and To=%s", v, res.Result, res.To, v)
		}
	}
}

// Round trip, google, per variant as explicit SOURCE: the variant is reported back as chosen and
// the wire carries the base language (google has no source dialects).
func TestGoogleRoundTripPerVariantSource(t *testing.T) {
	wantSL := map[model.Language]string{model.ESMX: "es", model.PTBR: "pt", model.PTPT: "pt"}
	for _, v := range variants {
		var q url.Values
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q = r.URL.Query()
			_, _ = w.Write([]byte(`[[["Hello","Hola",null,null,1]],null,"` + wantSL[v] + `"]`))
		}))
		reg := engine.NewRegistry()
		reg.RegisterTranslator(engine.NewGoogle(srv.URL, srv.Client()))
		svc := NewService(reg, nil, nil, nil)
		svc.SetLangPrefs(langpref.New())
		res, err := svc.Translate(model.TranslateRequest{Text: "Hola", From: v, To: model.EN, EngineName: "google"})
		srv.Close()
		if err != nil {
			t.Fatalf("%s: Translate: %v", v, err)
		}
		if got := q.Get("sl"); got != wantSL[v] {
			t.Errorf("%s: sl = %q, want %q", v, got, wantSL[v])
		}
		if res.From != v || res.Result != "Hello" {
			t.Errorf("%s: From = %q, Result = %q, want From=%s and Hello", v, res.From, res.Result, v)
		}
	}
}

// Round trip, LLM engine (openai-compatible over loopback), per variant as TARGET: the prompt the
// model receives names the variant (not a collapsed base language) and its reply is returned.
func TestLLMRoundTripPerVariantTarget(t *testing.T) {
	i18n.SetLocale("en-US")
	t.Cleanup(func() { i18n.SetLocale("zh-CN") })
	frag := map[model.Language]string{model.ESMX: "Mexico", model.PTBR: "Brazil", model.PTPT: "Portugal"}
	for _, v := range variants {
		var prompt string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			prompt = string(body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "x", "object": "chat.completion", "model": "m",
				"choices": []any{map[string]any{"index": 0, "finish_reason": "stop",
					"message": map[string]any{"role": "assistant", "content": "llm-" + string(v)}}},
			})
		}))
		reg := engine.NewRegistry()
		reg.RegisterTranslator(engine.NewOpenAI(&engine.EngineConfig{APIKey: "test-key", Endpoint: srv.URL}, srv.Client()))
		svc := NewService(reg, nil, nil, nil)
		res, err := svc.Translate(model.TranslateRequest{Text: "Hello", From: model.EN, To: v, EngineName: "openai"})
		srv.Close()
		if err != nil {
			t.Fatalf("%s: Translate: %v", v, err)
		}
		if !strings.Contains(prompt, frag[v]) {
			t.Errorf("%s: LLM request should name the variant (%q); body = %s", v, frag[v], prompt)
		}
		if res.Result != "llm-"+string(v) || res.To != v {
			t.Errorf("%s: Result = %q / To = %q, want llm-%s and To=%s", v, res.Result, res.To, v, v)
		}
	}
}

// Preference loop (#53 integration): pick a variant once, then N auto-detected sends all stay
// qualified. Each family keeps its own preference; nothing is consumed by use.
func TestPreferenceLoopStaysQualifiedAcrossNSends(t *testing.T) {
	store := langpref.New()
	store.Learn(model.ESMX) // one explicit selection
	store.Learn(model.PTPT) // a second family, chosen once
	esSvc := newLoopbackService(t, `"es"`)
	esSvc.SetLangPrefs(store)
	ptSvc := newLoopbackService(t, `"pt"`)
	ptSvc.SetLangPrefs(store)
	const n = 8
	for i := 0; i < n; i++ {
		if got := translateAuto(t, esSvc, model.Auto); got != model.ESMX {
			t.Fatalf("es send %d: From = %q, want es-MX", i+1, got)
		}
		if got := translateAuto(t, ptSvc, model.Auto); got != model.PTPT {
			t.Fatalf("pt send %d: From = %q, want pt-PT", i+1, got)
		}
	}
}

// Re-picking within a family replaces the preference: later detected sends follow the new pick.
func TestPreferenceFollowsLatestVariantPick(t *testing.T) {
	store := langpref.New()
	store.Learn(model.PTPT)
	store.Learn(model.PTBR)
	svc := newLoopbackService(t, `"pt"`)
	svc.SetLangPrefs(store)
	for i := 0; i < 3; i++ {
		if got := translateAuto(t, svc, model.Auto); got != model.PTBR {
			t.Fatalf("send %d: From = %q, want pt-BR (latest pick)", i+1, got)
		}
	}
}
