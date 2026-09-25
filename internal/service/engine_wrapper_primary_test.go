package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/settings"
)

// issue #8 (Tester role, RED): PrimaryTranslateEngine's resolution rules.
//
// Rules (design doc §1, the authoritative implementation):
//   1. settings' default_engine is non-empty, and that engine is in the engine list with
//      kind=translate and enabled -> return it;
//   2. otherwise (unset / name not in list / kind not translate / disabled)
//      -> fall back to the first enabled translate engine in the list (configstore id order);
//   3. no usable engine -> empty string.
//
// No mocks: engine config lands in real SQLite under t.TempDir (configstore.Open), the
// google engine's Endpoint points at a real loopback httptest.Server registered into a real
// Registry; settings uses a real settings.Service (a real settings.json file). app/hotkeyMgr
// are passed as nil: the constructor allows nil injection, and this test only calls
// GetEngines and PrimaryTranslateEngine — neither touches app/hotkeyMgr.
//
// RED note: EngineWrapper has no PrimaryTranslateEngine method yet and settings.Settings has
// no DefaultEngine field — this fails at compile time (undefined). This is a
// "missing behavior" RED.
// GetEngines/GetAllEngines today return no enabled state (GetEngines filters by registry,
// GetAllEngines is not exposed to the resolver), so the implementation must resolve by
// configstore's enabled column inside PrimaryTranslateEngine itself (GetEngines' id-order
// semantics stay unchanged).

const gtxPrimaryFixture = `[[["Hello","Bonjour","","","0"]],null,"en"]`

// setupPrimaryEnv builds a real configstore + settings + registry inside t.TempDir, creating
// and registering the specified engine rows. Returns (store, svc, wrapper, cleanup).
func setupPrimaryEnv(t *testing.T, rows []*engine.EngineConfig) (*configstore.Store, *settings.Service, *EngineWrapper) {
	t.Helper()
	store, err := configstore.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("configstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ctx := context.Background()
	if err := store.InitDefaultEngines(ctx, rows); err != nil {
		t.Fatalf("InitDefaultEngines: %v", err)
	}

	svc, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatalf("settings.NewService: %v", err)
	}

	reg := engine.NewRegistry()
	for _, e := range rows {
		if !e.Enabled {
			continue
		}
		switch e.Engine {
		case "google":
			reg.RegisterTranslator(engine.NewGoogle(e.Endpoint, http.DefaultClient))
		case "deepl":
			reg.RegisterTranslator(engine.NewDeepL(e, http.DefaultClient))
		}
	}

	return store, svc, NewEngineWrapper(reg, store, svc, (*application.App)(nil), nil)
}

// TestPrimaryTranslateEngineFallbackInvalidName (b): default_engine points at a name not in
// the engine list -> resolution must fall back to the first enabled translate engine in the
// list.
func TestPrimaryTranslateEngineFallbackInvalidName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	_, svc, w := setupPrimaryEnv(t, []*engine.EngineConfig{
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
		{Engine: "deepl", Enabled: true, APIKey: "k"},
	})

	svc.Get().DefaultEngine = "nosuchengine"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := w.PrimaryTranslateEngine(); got != "google" {
		t.Fatalf("invalid default_engine should fall back to the first enabled translate engine google, got %q (PrimaryTranslateEngine missing this fallback)", got)
	}
}

// TestPrimaryTranslateEngineFallsBackWhenPrimaryDisabled (c): the primary is later disabled
// -> resolution falls back to the next enabled translate engine (no error, config kept).
func TestPrimaryTranslateEngineFallsBackWhenPrimaryDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	store, svc, w := setupPrimaryEnv(t, []*engine.EngineConfig{
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
		{Engine: "deepl", Enabled: true, APIKey: "k"},
	})

	// Set primary = google (enabled); resolution should hit it.
	svc.Get().DefaultEngine = "google"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := w.PrimaryTranslateEngine(); got != "google" {
		t.Fatalf("enabled primary should resolve to google, got %q", got)
	}

	// Disable the primary (google) through the real configstore path.
	ctx := context.Background()
	row, err := store.GetEngineByName(ctx, "google")
	if err != nil || row == nil {
		t.Fatalf("GetEngineByName(google): %v", err)
	}
	if err := store.SetEngineEnabled(ctx, row.ID, false); err != nil {
		t.Fatalf("SetEngineEnabled: %v", err)
	}

	// Resolution must fall back again: deepl is the next (id order) enabled translate engine
	// in the list.
	if got := w.PrimaryTranslateEngine(); got != "deepl" {
		t.Fatalf("disabled primary should re-resolve to deepl, got %q (disabling did not trigger fallback)", got)
	}
}

// TestPrimaryTranslateEngineUnsetFallsBackToFirstEnabled: default_engine unset (empty
// string, zero value) -> falls back to the first enabled translate engine. The unset branch
// of rule step 2.
func TestPrimaryTranslateEngineUnsetFallsBackToFirstEnabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	_, _, w := setupPrimaryEnv(t, []*engine.EngineConfig{
		{Engine: "deepl", Enabled: true, APIKey: "k"},
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
	})

	// deepl is first in id order and enabled; google second. primary unset -> deepl.
	if got := w.PrimaryTranslateEngine(); got != "deepl" {
		t.Fatalf("unset default_engine should fall back to the first enabled translate engine deepl, got %q", got)
	}
}
