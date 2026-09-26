package translate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync"
	"testing"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/historystore"
	"cnb.cool/dtapp/kai/internal/langpref"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #80: same source and target language shows the source text. Every case goes through the
// real google engine against a loopback gtx server (httptest), so "the engine was not called" is
// a request count on the server, and the history store is a real file under t.TempDir().

// gtxServer is a loopback gtx endpoint. It answers every request with result and the detected
// source language, or with HTTP 500 when fail is set, and records each request's tl.
type gtxServer struct {
	mu       sync.Mutex
	tls      []string
	detected string
	result   string
	fail     bool
}

func (g *gtxServer) requests() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.tls...)
}

func (g *gtxServer) count() int { return len(g.requests()) }

// gtxOpts configures the loopback server: the language it reports as detected, the translation
// it returns, and whether it answers HTTP 500.
type gtxOpts struct {
	detected string
	result   string
	fail     bool
}

func newGtx(t *testing.T, o gtxOpts) (*gtxServer, string, *http.Client) {
	t.Helper()
	g := &gtxServer{detected: o.detected, result: o.result, fail: o.fail}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.ParseQuery(r.URL.RawQuery)
		g.mu.Lock()
		g.tls = append(g.tls, q.Get("tl"))
		g.mu.Unlock()
		if g.fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`[[["` + g.result + `","Hola",null,null,1]],null,"` + g.detected + `"]`))
	}))
	t.Cleanup(srv.Close)
	return g, srv.URL, srv.Client()
}

// newIdentityServiceWith builds a service over a real history store and config store under
// t.TempDir() with the given translator registered.
func newIdentityServiceWith(t *testing.T, tr engine.Translator) (*Service, *historystore.Store) {
	t.Helper()
	dir := t.TempDir()
	hist, err := historystore.Open(filepath.Join(dir, "history.db"))
	if err != nil {
		t.Fatalf("historystore.Open: %v", err)
	}
	t.Cleanup(func() { _ = hist.Close() })
	cfg, err := configstore.Open(filepath.Join(dir, "config.db"))
	if err != nil {
		t.Fatalf("configstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = cfg.Close() })
	reg := engine.NewRegistry()
	reg.RegisterTranslator(tr)
	svc := NewService(reg, hist, nil, nil)
	svc.SetConfigStore(cfg)
	return svc, hist
}

// newIdentityService wires the real google engine to a fresh loopback gtx server.
func newIdentityService(t *testing.T, o gtxOpts) (*Service, *gtxServer, *historystore.Store) {
	t.Helper()
	g, endpoint, client := newGtx(t, o)
	svc, hist := newIdentityServiceWith(t, engine.NewGoogle(endpoint, client))
	return svc, g, hist
}

// detectingEngine is the ONE in-process Translator in this file. It exists only for the
// failure-with-detection path (engine.WithDetectedSource): today only the Apple engine attaches a
// detected source to an error, and Apple's bridge is macOS-only and cannot run in CI. Google
// never does, so no loopback server can drive it. The detection-on-error path itself is covered
// by internal/engine/detected_source_test.go; this only proves the service's decision on it.
type detectingEngine struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (d *detectingEngine) Name() string { return "google" }

func (d *detectingEngine) Translate(context.Context, model.TranslateRequest) (*model.TranslateResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls++
	return nil, d.err
}

func (d *detectingEngine) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.calls
}

const idText = "Hola mundo"

func idHistoryRows(t *testing.T, hist *historystore.Store) int {
	t.Helper()
	rows, err := hist.QueryByKeyword(context.Background(), "Hola", 50, 0)
	if err != nil {
		t.Fatalf("QueryByKeyword: %v", err)
	}
	return len(rows)
}

func idTranslate(svc *Service, from, to model.Language) (*model.TranslateResult, error) {
	return svc.Translate(model.TranslateRequest{Text: idText, From: from, To: to, EngineName: "google"})
}

func assertIdentity(t *testing.T, label string, res *model.TranslateResult, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: unexpected error: %v", label, err)
	}
	if res == nil {
		t.Fatalf("%s: nil result", label)
	}
	if !res.Identity {
		t.Errorf("%s: Identity = false, want true", label)
	}
	if res.Result != idText || res.Text != idText {
		t.Errorf("%s: Result/Text = %q/%q, want the source text %q", label, res.Result, res.Text, idText)
	}
	if res.Error != "" {
		t.Errorf("%s: Error = %q, want empty", label, res.Error)
	}
}

// Criterion 1 + 7: explicit same language never reaches the engine.
func TestExplicitSameLanguageShowsSourceText(t *testing.T) {
	pairs := [][2]model.Language{
		{"es", "es-MX"}, {"es-MX", "es"}, {"es-MX", "es-MX"},
		{"zh", "zh-CN"}, {"zh-CN", "zh"}, {"en", "en"}, {"ES", "es-mx"}, {model.ZH, model.ZH},
	}
	for _, p := range pairs {
		svc, g, hist := newIdentityService(t, gtxOpts{result: "engine-text"})
		res, err := idTranslate(svc, p[0], p[1])
		label := string(p[0]) + "/" + string(p[1])
		assertIdentity(t, label, res, err)
		if res != nil && (res.From != p[0] || res.To != p[1]) {
			t.Errorf("%s: From/To = %q/%q, want as requested", label, res.From, res.To)
		}
		if res != nil && res.Engine != "google" {
			t.Errorf("%s: Engine = %q, want google", label, res.Engine)
		}
		if n := g.count(); n != 0 {
			t.Errorf("%s: gtx server got %d requests, want 0", label, n)
		}
		if n := idHistoryRows(t, hist); n != 0 {
			t.Errorf("%s: %d history rows, want 0 (identity is never saved)", label, n)
		}
	}
}

// Criterion 2: different dialects are not identity; an engine failure stays a failure.
func TestDifferentDialectsAreNotIdentity(t *testing.T) {
	pairs := [][2]model.Language{
		{"pt-BR", "pt-PT"}, {"pt-PT", "pt-BR"}, {"es-MX", "es-ES"}, {"es-MX", "es-419"},
	}
	for _, p := range pairs {
		svc, g, hist := newIdentityService(t, gtxOpts{result: "Hola!"})
		res, err := idTranslate(svc, p[0], p[1])
		label := string(p[0]) + "/" + string(p[1])
		if err != nil || res == nil {
			t.Fatalf("%s: (%v, %v)", label, res, err)
		}
		if res.Identity {
			t.Errorf("%s: Identity = true, want false", label)
		}
		if res.Result != "Hola!" {
			t.Errorf("%s: Result = %q, want the engine's text", label, res.Result)
		}
		if n := g.count(); n != 1 {
			t.Errorf("%s: gtx server got %d requests, want 1", label, n)
		}
		if n := idHistoryRows(t, hist); n != 1 {
			t.Errorf("%s: %d history rows, want exactly 1", label, n)
		}

		svc2, g2, hist2 := newIdentityService(t, gtxOpts{fail: true})
		res2, err2 := idTranslate(svc2, p[0], p[1])
		if res2 != nil || err2 == nil {
			t.Errorf("%s: failure = (%v, %v), want nil result and an error", label, res2, err2)
		}
		if n := g2.count(); n != 1 {
			t.Errorf("%s: failing gtx server got %d requests, want 1", label, n)
		}
		if n := idHistoryRows(t, hist2); n != 0 {
			t.Errorf("%s: %d history rows after a failure, want 0", label, n)
		}
	}
}

// Criterion 3: whitespace-only text still reaches the engine.
func TestWhitespaceTextNeverIdentity(t *testing.T) {
	svc, g, _ := newIdentityService(t, gtxOpts{fail: true})
	res, err := svc.Translate(model.TranslateRequest{Text: "  ", From: model.EN, To: model.EN, EngineName: "google"})
	if res != nil || err == nil {
		t.Errorf("got (%v, %v), want nil result and the engine's error", res, err)
	}
	if n := g.count(); n != 1 {
		t.Errorf("gtx server got %d requests, want 1", n)
	}
}

// Criterion 4: auto source, the engine fails but reports the detection, detection == target.
// Only Apple attaches a detected source to a failure, and its bridge is macOS-only, so this is
// driven by the in-process detectingEngine (see its comment); everything else uses google.
func TestAutoFailureWithMatchingDetectionShowsSourceText(t *testing.T) {
	cases := []struct{ detected, target model.Language }{
		{"en", "en"}, {"zh", "zh"}, {"zh", "zh-CN"}, {"es", "es-MX"}, {"es", "es-ES"},
	}
	for _, c := range cases {
		d := &detectingEngine{err: engine.WithDetectedSource(errors.New("Unable to Translate"), c.detected)}
		svc, hist := newIdentityServiceWith(t, d)
		res, err := idTranslate(svc, model.Auto, c.target)
		label := string(c.detected) + "->" + string(c.target)
		assertIdentity(t, label, res, err)
		if res != nil {
			if res.To != c.target {
				t.Errorf("%s: To = %q, want %q", label, res.To, c.target)
			}
			if res.From != c.detected {
				t.Errorf("%s: From = %q, want the detection", label, res.From)
			}
		}
		if n := d.count(); n != 1 {
			t.Errorf("%s: engine called %d times, want exactly 1 (no re-run)", label, n)
		}
		if n := idHistoryRows(t, hist); n != 0 {
			t.Errorf("%s: %d history rows, want 0", label, n)
		}
	}
}

// Criterion 5: no detection, or a detection that differs from the target: the error stands.
func TestAutoFailureWithoutMatchingDetectionIsPlainFailure(t *testing.T) {
	// No detection: a real HTTP 500 from the loopback gtx server.
	svc, g, _ := newIdentityService(t, gtxOpts{fail: true})
	res, err := idTranslate(svc, model.Auto, model.EN)
	if res != nil || err == nil {
		t.Errorf("no detection: got (%v, %v), want nil result and the engine error", res, err)
	}
	if n := g.count(); n != 1 {
		t.Errorf("no detection: gtx server got %d requests, want 1", n)
	}

	// A failing engine's detection that differs from the target (Apple-only path, in-process).
	plain := errors.New("Unable to Translate")
	d := &detectingEngine{err: engine.WithDetectedSource(plain, model.ES)}
	svc2, _ := newIdentityServiceWith(t, d)
	res2, err2 := idTranslate(svc2, model.Auto, model.ZH)
	if res2 != nil || err2 == nil || !errors.Is(err2, plain) {
		t.Errorf("detection es vs target zh: got (%v, %v), want nil result and the engine error", res2, err2)
	}
	if n := d.count(); n != 1 {
		t.Errorf("detection es vs target zh: engine called %d times, want 1", n)
	}

	// A pinned different language failing is a plain failure too.
	svc3, g3, _ := newIdentityService(t, gtxOpts{fail: true})
	if res, err := idTranslate(svc3, model.EN, model.ZH); res != nil || err == nil {
		t.Errorf("pinned en->zh failure: got (%v, %v), want the error", res, err)
	}
	if n := g3.count(); n != 1 {
		t.Errorf("pinned en->zh failure: gtx server got %d requests, want 1", n)
	}
}

// Criterion 6: auto source, the engine succeeds and reports a detection equal to the target.
func TestAutoSuccessWithMatchingDetectionShowsSourceText(t *testing.T) {
	cases := []struct{ detected, target model.Language }{
		{"en", "en"}, {"zh", "zh"}, {"zh", "zh-CN"}, {"es", "es-MX"},
	}
	for _, c := range cases {
		svc, g, hist := newIdentityService(t, gtxOpts{detected: string(c.detected), result: "a paraphrase"})
		res, err := idTranslate(svc, model.Auto, c.target)
		label := string(c.detected) + "->" + string(c.target)
		assertIdentity(t, label, res, err)
		if res != nil && res.To != c.target {
			t.Errorf("%s: To = %q, want %q", label, res.To, c.target)
		}
		got := g.requests()
		if len(got) != 1 {
			t.Errorf("%s: gtx server got %d requests (%v), want 1", label, len(got), got)
		}
		for _, tl := range got {
			if tl == "en" && c.target != model.EN {
				t.Errorf("%s: a request went toward en: the #44 re-run is back", label)
			}
		}
		if n := idHistoryRows(t, hist); n != 0 {
			t.Errorf("%s: %d history rows, want 0", label, n)
		}
	}
}

// Criterion 6: a success whose detection differs from the target is the engine's answer.
func TestAutoSuccessWithDifferentDetectionIsEngineAnswer(t *testing.T) {
	svc, _, hist := newIdentityService(t, gtxOpts{detected: "zh-CN", result: "Hello"})
	res, err := idTranslate(svc, model.Auto, model.EN)
	if err != nil || res == nil {
		t.Fatalf("got (%v, %v)", res, err)
	}
	// Only the service sets Identity (review warn 4): a different-language answer is never flagged.
	if res.Identity || res.Result != "Hello" {
		t.Errorf("Identity/Result = %v/%q, want false/Hello", res.Identity, res.Result)
	}
	if n := idHistoryRows(t, hist); n != 1 {
		t.Errorf("%d history rows, want 1", n)
	}
}

// Review warn 1: the decision compares the engine's bare detection, not the preference-qualified
// one. With es qualified to es-MX by the preference store, detected es into an es-ES target is
// still the same language; From is the qualified value for display.
func TestDecisionUsesBareDetectionNotQualified(t *testing.T) {
	svc, _, _ := newIdentityService(t, gtxOpts{detected: "es", result: "paraphrase"})
	prefs := langpref.New()
	prefs.Learn(model.ESMX)
	svc.SetLangPrefs(prefs)
	res, err := idTranslate(svc, model.Auto, "es-ES")
	assertIdentity(t, "es detected, es-ES target, pref es-MX", res, err)
	if res != nil && res.From != model.ESMX {
		t.Errorf("From = %q, want es-MX (qualified for display)", res.From)
	}
}

// Criterion 9: the screenshot fan-out path.
func TestTranslateAllStreamExplicitSameLanguage(t *testing.T) {
	svc, g, hist := newIdentityService(t, gtxOpts{result: "engine-text"})
	out := svc.translateAllStream(svc.requests.open("shot", "t80"), model.TranslateRequest{Text: idText, From: "es", To: "es-MX"}, "img", "es-MX")
	if len(out) != 1 {
		t.Fatalf("got %d translations, want 1", len(out))
	}
	tr := out[0]
	if !tr.Identity || tr.Result != idText || tr.Error != "" {
		t.Errorf("entry = %+v, want an identity entry with the source text and no failure", tr)
	}
	if n := g.count(); n != 0 {
		t.Errorf("gtx server got %d requests, want 0", n)
	}
	if n := idHistoryRows(t, hist); n != 0 {
		t.Errorf("%d history rows, want 0", n)
	}
}

// Criterion 9 + 8: the screenshot path with an auto source (ScreenshotTranslate) whose engine
// detects the target language is an identity entry and is not saved.
func TestTranslateAllStreamAutoDetectedSame(t *testing.T) {
	svc, g, hist := newIdentityService(t, gtxOpts{detected: "es", result: "a paraphrase"})
	out := svc.translateAllStream(svc.requests.open("shot", "t80"), model.TranslateRequest{Text: idText, From: model.Auto, To: "es-MX"}, "img", "es-MX")
	if len(out) != 1 || !out[0].Identity || out[0].Result != idText {
		t.Fatalf("out = %+v, want one identity entry", out)
	}
	if n := g.count(); n != 1 {
		t.Errorf("gtx server got %d requests, want 1", n)
	}
	if n := idHistoryRows(t, hist); n != 0 {
		t.Errorf("%d history rows, want 0", n)
	}
}

// The same screenshot path when the engine fails but reports the detection (Apple-only, so the
// in-process detectingEngine): still an identity entry after exactly one call.
func TestTranslateAllStreamAutoFailureWithDetectionSame(t *testing.T) {
	d := &detectingEngine{err: engine.WithDetectedSource(errors.New("Unable to Translate"), "es")}
	svc, hist := newIdentityServiceWith(t, d)
	out := svc.translateAllStream(svc.requests.open("shot", "t80"), model.TranslateRequest{Text: idText, From: model.Auto, To: "es-MX"}, "img", "es-MX")
	if len(out) != 1 || !out[0].Identity || out[0].Result != idText {
		t.Fatalf("out = %+v, want one identity entry", out)
	}
	if n := d.count(); n != 1 {
		t.Errorf("engine called %d times, want 1", n)
	}
	if n := idHistoryRows(t, hist); n != 0 {
		t.Errorf("%d history rows, want 0", n)
	}
}

// A pinned source is never turned into identity by the engine's reported detection (Google
// reports one even when the source is pinned): a dialect pair stays a real translation.
func TestPinnedSourceIgnoresReportedDetection(t *testing.T) {
	cases := []struct{ from, to, detected model.Language }{
		{"pt-BR", "pt-PT", "pt"},
		{"es-MX", "es-ES", "es"},
	}
	for _, c := range cases {
		svc, g, _ := newIdentityService(t, gtxOpts{detected: string(c.detected), result: "Ola"})
		res, err := idTranslate(svc, c.from, c.to)
		if err != nil {
			t.Fatalf("%s->%s: %v", c.from, c.to, err)
		}
		if res.Identity {
			t.Errorf("%s->%s: Identity = true for a pinned source", c.from, c.to)
		}
		if res.Result != "Ola" {
			t.Errorf("%s->%s: Result = %q, want the engine text", c.from, c.to, res.Result)
		}
		if res.From != c.from {
			t.Errorf("%s->%s: From = %q, want the pinned source", c.from, c.to, res.From)
		}
		if g.count() != 1 {
			t.Errorf("%s->%s: gtx requests = %d, want 1", c.from, c.to, g.count())
		}
	}
}
