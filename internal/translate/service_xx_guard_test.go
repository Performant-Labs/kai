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
	return newXXServiceOpts(t, detected, "")
}

// newXXServiceOpts is newXXService plus failTL: a request whose tl equals it is still recorded and
// then answered HTTP 500 (empty failTL means every request succeeds).
func newXXServiceOpts(t *testing.T, detected, failTL string) (*Service, *xxServer, *historystore.Store) {
	t.Helper()
	rec := &xxServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.ParseQuery(r.URL.RawQuery)
		tl := q.Get("tl")
		rec.mu.Lock()
		rec.tls = append(rec.tls, tl)
		rec.mu.Unlock()
		if failTL != "" && tl == failTL {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
			return
		}
		// Constant per target, never the request value itself: the assertion only needs the re-run's
		// output ("out-en") to be distinguishable from the first pass's.
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

// Detected Chinese into a Chinese target (incl. the legacy zh-CN spelling) is re-run once
// toward English, reports the flipped target, and saves exactly one history row (the flipped one).
func TestDetectedSourceEqualsTargetFlipsToEnglish(t *testing.T) {
	for _, target := range []model.Language{model.ZH, "zh-CN"} {
		svc, rec, hist := newXXService(t, "zh-CN")
		res := xxTranslate(t, svc, model.Auto, target)
		if res.To != model.EN {
			t.Errorf("target %q: To = %q, want en (flipped, reported)", target, res.To)
		}
		if res.Result != "out-en" {
			t.Errorf("target %q: Result = %q, want out-en (the re-run's translation)", target, res.Result)
		}
		if got := rec.targets(); len(got) != 2 {
			t.Errorf("target %q: engine called %d times (%v), want exactly 2 (original + one re-run)", target, len(got), got)
		}
		if got := historyTos(t, hist); len(got) != 1 || got[0] != "en" {
			t.Errorf("target %q: history to_lang = %v, want exactly [en]", target, got)
		}
	}
}

// es detected with an es-MX target is the same language family: X->X, flips to English.
func TestDetectedSpanishIntoSpanishVariantFlips(t *testing.T) {
	svc, _, _ := newXXService(t, "es")
	res := xxTranslate(t, svc, model.Auto, model.ESMX)
	if res.To != model.EN {
		t.Errorf("To = %q, want en (es detected, es-MX target is X->X)", res.To)
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
}

// The guard only applies to an auto source: an explicit zh source with a zh target is the
// user's own request and is left alone.
func TestExplicitSourceNeverFlipped(t *testing.T) {
	svc, rec, _ := newXXService(t, "zh-CN")
	res := xxTranslate(t, svc, model.ZH, model.ZH)
	if res.To != model.ZH {
		t.Errorf("To = %q, want zh (explicit source, no guard)", res.To)
	}
	if got := rec.targets(); len(got) != 1 {
		t.Errorf("engine called %d times, want 1", len(got))
	}
}

// A failed re-run is an engine failure, never the identity result. The guard discards the first
// (same-language) pass, so when the fallback call fails Translate must surface that failure with
// no result and nothing saved; falling back to the first pass would present exactly the X->X
// "translation" issue #44 exists to remove.
func TestFailedReRunIsNotPresentedAsIdentityResult(t *testing.T) {
	svc, rec, hist := newXXServiceOpts(t, "zh-CN", "en")
	res, err := svc.Translate(model.TranslateRequest{Text: "Hola", From: model.Auto, To: model.ZH, EngineName: "google"})
	if err == nil {
		t.Fatalf("Translate error = nil (result %+v), want the re-run's failure", res)
	}
	if res != nil {
		t.Errorf("result = %+v, want nil (the discarded identity pass must not be presented)", res)
	}
	if got := rec.targets(); len(got) != 2 || got[1] != "en" {
		t.Errorf("engine requests toward %v, want the original then exactly one re-run toward en", got)
	}
	if got := historyTos(t, hist); len(got) != 0 {
		t.Errorf("history to_lang = %v, want none (a failed re-run saves nothing)", got)
	}
}
