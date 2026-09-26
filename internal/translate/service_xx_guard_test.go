package translate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"testing"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/historystore"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #44: the X->X guard. A fake gtx server reports a fixed detected source and echoes the
// requested target language in the translation, so a test can see which target each call used.
type xxServer struct {
	mu  sync.Mutex
	tls []string
}

func (x *xxServer) targets() []string {
	x.mu.Lock()
	defer x.mu.Unlock()
	return append([]string(nil), x.tls...)
}

func newXXService(t *testing.T, detected string) (*Service, *xxServer, *historystore.Store) {
	t.Helper()
	rec := &xxServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.ParseQuery(r.URL.RawQuery)
		tl := q.Get("tl")
		rec.mu.Lock()
		rec.tls = append(rec.tls, tl)
		rec.mu.Unlock()
		// Constant per target: distinguishes an engine answer toward en from any other.
		out := "out-other"
		if tl == "en" {
			out = "out-en"
		}
		_, _ = w.Write([]byte(`[[["` + out + `","Hola",null,null,1]],null,"` + detected + `"]`))
	}))
	t.Cleanup(srv.Close)
	hist, err := historystore.Open("")
	if err != nil {
		t.Fatalf("historystore.Open: %v", err)
	}
	t.Cleanup(func() { _ = hist.Close() })
	cfg, err := configstore.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("configstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = cfg.Close() })
	reg := engine.NewRegistry()
	reg.RegisterTranslator(engine.NewGoogle(srv.URL, srv.Client()))
	svc := NewService(reg, hist, nil, nil)
	svc.SetConfigStore(cfg)
	return svc, rec, hist
}

func historyTos(t *testing.T, hist *historystore.Store) []string {
	t.Helper()
	rows, err := hist.QueryByKeyword(context.Background(), "Hola", 50, 0)
	if err != nil {
		t.Fatalf("QueryByKeyword: %v", err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.ToLang)
	}
	return out
}

func xxTranslate(t *testing.T, svc *Service, from, to model.Language) *model.TranslateResult {
	t.Helper()
	res, err := svc.Translate(model.TranslateRequest{Text: "Hola", From: from, To: to, EngineName: "google"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	return res
}

// Issue #80 (inverts #44): detected Chinese into a Chinese target (incl. the legacy zh-CN
// spelling) shows the source text as an identity result: one engine call, no re-run toward
// English, To as requested, and nothing saved to history.
func TestDetectedSourceEqualsTargetShowsSourceText(t *testing.T) {
	for _, target := range []model.Language{model.ZH, "zh-CN"} {
		svc, rec, hist := newXXService(t, "zh-CN")
		res := xxTranslate(t, svc, model.Auto, target)
		if res.To != target {
			t.Errorf("target %q: To = %q, want the requested target", target, res.To)
		}
		if !res.Identity || res.Result != "Hola" {
			t.Errorf("target %q: Identity/Result = %v/%q, want true/the source text", target, res.Identity, res.Result)
		}
		if got := rec.targets(); len(got) != 1 {
			t.Errorf("target %q: engine called %d times (%v), want exactly 1 (no re-run)", target, len(got), got)
		}
		if got := historyTos(t, hist); len(got) != 0 {
			t.Errorf("target %q: history to_lang = %v, want none", target, got)
		}
	}
}

// Issue #80 (inverts #44): es detected with an es-MX target is the same language: source text.
func TestDetectedSpanishIntoSpanishVariantShowsSourceText(t *testing.T) {
	svc, rec, _ := newXXService(t, "es")
	res := xxTranslate(t, svc, model.Auto, model.ESMX)
	if res.To != model.ESMX || !res.Identity {
		t.Errorf("To/Identity = %q/%v, want es-MX/true", res.To, res.Identity)
	}
	if got := rec.targets(); len(got) != 1 {
		t.Errorf("engine called %d times (%v), want 1", len(got), got)
	}
}

// A persisted zh target keeps working for non-Chinese text: no flip, one call.
func TestDifferentLanguageTargetHonored(t *testing.T) {
	svc, rec, hist := newXXService(t, "es")
	res := xxTranslate(t, svc, model.Auto, model.ZH)
	if res.To != model.ZH {
		t.Errorf("To = %q, want zh (no X->X, target honored)", res.To)
	}
	if got := rec.targets(); len(got) != 1 {
		t.Errorf("engine called %d times, want 1", len(got))
	}
	if got := historyTos(t, hist); len(got) != 1 || got[0] != "zh" {
		t.Errorf("history to_lang = %v, want [zh]", got)
	}
}

// English detected into English: the fallback is en, so there is nothing to flip to.
func TestEnglishToEnglishDoesNotFlip(t *testing.T) {
	svc, rec, _ := newXXService(t, "en")
	res := xxTranslate(t, svc, model.Auto, model.EN)
	if res.To != model.EN {
		t.Errorf("To = %q, want en", res.To)
	}
	if got := rec.targets(); len(got) != 1 {
		t.Errorf("engine called %d times, want 1 (no re-run for en->en)", len(got))
	}
	if !res.Identity {
		t.Error("Identity = false, want true (same language shows the source text)")
	}
}

// Issue #80: an explicit zh->zh request is never flipped to English: it is an identity result,
// the engine is not called, and To stays zh.
func TestExplicitSourceNeverFlipped(t *testing.T) {
	svc, rec, _ := newXXService(t, "zh-CN")
	res := xxTranslate(t, svc, model.ZH, model.ZH)
	if res.To != model.ZH {
		t.Errorf("To = %q, want zh", res.To)
	}
	if !res.Identity {
		t.Error("Identity = false, want true")
	}
	if got := rec.targets(); len(got) != 0 {
		t.Errorf("engine called %d times, want 0", len(got))
	}
}
