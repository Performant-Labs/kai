package translate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/historystore"
	"cnb.cool/dtapp/kai/internal/langpref"
	"cnb.cool/dtapp/kai/internal/model"
)

// newHistoryService wires the real Service + google engine (loopback) + real history and
// config stores. It also proves historystore.Open accepts its embedded migration.
func newHistoryService(t *testing.T, detected string) (*Service, *historystore.Store) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[[["Hola","Hello",null,null,1]],null,` + detected + `]`))
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
	return svc, hist
}

func sendAuto(t *testing.T, svc *Service) {
	t.Helper()
	if _, err := svc.Translate(model.TranslateRequest{Text: "Hello", From: model.Auto, To: model.EN, EngineName: "google"}); err != nil {
		t.Fatalf("Translate: %v", err)
	}
}

func historyFroms(t *testing.T, hist *historystore.Store) []string {
	t.Helper()
	rows, err := hist.QueryByKeyword(context.Background(), "Hola", 50, 0)
	if err != nil {
		t.Fatalf("QueryByKeyword: %v", err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, r.FromLang)
	}
	return out
}

// #53 (A warn 6): history records the carried From (not "auto"), qualified by the preference,
// and the dedupe key follows the qualified value.
func TestHistoryRecordsCarriedQualifiedFrom(t *testing.T) {
	svc, hist := newHistoryService(t, `"es"`)
	store := langpref.New()
	svc.SetLangPrefs(store)

	sendAuto(t, svc)
	if got := historyFroms(t, hist); len(got) != 1 || got[0] != "es" {
		t.Fatalf("no preference: history from_lang = %v, want [es]", got)
	}

	store.Learn(model.ESMX)
	sendAuto(t, svc)
	// Row order is not asserted: created_at has millisecond resolution and both rows can tie.
	if got := historyFroms(t, hist); len(got) != 2 || (got[0] != "es-MX" && got[1] != "es-MX") {
		t.Fatalf("after Learn(es-MX): history from_lang = %v, want es and es-MX (2 rows)", got)
	}

	sendAuto(t, svc)
	if got := historyFroms(t, hist); len(got) != 2 {
		t.Errorf("identical send must dedupe on the qualified key, got %d rows: %v", len(got), got)
	}
}
