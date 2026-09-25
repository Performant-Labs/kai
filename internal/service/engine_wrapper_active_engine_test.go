package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/settings"
)

// issue #9 (Tester role, RED): service-layer resolution of the result pane's "current
// engine" (design §1/§3 test (a)).
//
// Resolution chain last-used ?? primary(default_engine) ?? first-enabled: isomorphic to #8's
// PrimaryTranslateEngine, plus one last-used layer (the translate window's previously used
// engine, persisted by the frontend in localStorage as kai:translate:lastEngine):
//   1. lastUsed valid (in configstore, kind=translate, enabled column=1, platform supported)
//      -> it;
//   2. otherwise default_engine valid -> it (rule step 1 of #8);
//   3. otherwise the first enabled translate engine (configstore id order, rule step 2 of #8);
//   4. none -> "".
//
// The authoritative implementation must add an entry on EngineWrapper accepting last-used
// (named ActiveTranslateEngine per the design in this file); #8's PrimaryTranslateEngine
// stays argument-free with unchanged semantics (= the degenerate case of last-used being an
// empty string).
//
// No mocks: reuses the same package's #8 setupPrimaryEnv (real configstore opening a
// config.db file under t.TempDir(), real settings.Service, real registry; google's Endpoint
// pointing at a real loopback httptest.Server) — borrowed, not modified. The "enabled" state
// comes from configstore's enabled column (resolved after persisting via
// store.SetEngineEnabled), not from the registry's Supported — otherwise openai on Linux
// (Supported always true) would be misjudged as usable.
//
// RED note: EngineWrapper has no ActiveTranslateEngine method yet — this file fails at
// compile time (w.ActiveTranslateEngine undefined). This is a "missing behavior" RED, not an
// environment problem. Removing this file leaves the rest of the internal/service tests
// (including #8's engine_wrapper_primary_test.go) unaffected and passing.

// setupActiveEnv extends setupPrimaryEnv by inserting openai as a new row into the real
// configstore (key-free, endpoint-free, disabled by default): establishing the scenario
// "the list contains an engine not yet enabled, which once enabled can be resolved to, and
// whose auto-increment id sorts after the existing rows". InitDefaultEngines only adds
// missing rows and never enables existing disabled rows, while InsertEngineConfig always
// assigns a larger id to new rows.
func setupActiveEnv(t *testing.T, rows []*engine.EngineConfig) (*configstore.Store, *settings.Service, *EngineWrapper) {
	t.Helper()
	store, svc, w := setupPrimaryEnv(t, rows)
	if _, err := store.InsertEngineConfig(context.Background(), &engine.EngineConfig{Engine: "openai"}); err != nil {
		t.Fatalf("InsertEngineConfig(openai): %v", err)
	}
	return store, svc, w
}

// 1. last-used wins when it is among the enabled translate engines (overriding primary).
func TestActiveTranslateEngineLastUsedWins(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	_, svc, w := setupActiveEnv(t, []*engine.EngineConfig{
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
		{Engine: "deepl", Enabled: true, APIKey: "k"},
	})
	svc.Get().DefaultEngine = "google"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// last-used = deepl (enabled) -> should win, even though primary is google.
	if got := w.ActiveTranslateEngine("deepl"); got != "deepl" {
		t.Fatalf("last-used deepl should resolve to deepl when among enabled translate engines, got %q (ActiveTranslateEngine missing the last-used precedence layer)", got)
	}
}

// 2. after the last-used engine is disabled -> falls back to primary.
func TestActiveTranslateEngineLastUsedDisabledFallsBackToPrimary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	store, svc, w := setupActiveEnv(t, []*engine.EngineConfig{
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
		{Engine: "deepl", Enabled: true, APIKey: "k"},
	})
	svc.Get().DefaultEngine = "deepl"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Disable last-used (google) through the real configstore path.
	ctx := context.Background()
	row, err := store.GetEngineByName(ctx, "google")
	if err != nil || row == nil {
		t.Fatalf("GetEngineByName(google): %v", err)
	}
	if err := store.SetEngineEnabled(ctx, row.ID, false); err != nil {
		t.Fatalf("SetEngineEnabled: %v", err)
	}

	if got := w.ActiveTranslateEngine("google"); got != "deepl" {
		t.Fatalf("disabled last-used should fall back to primary deepl, got %q (disabled last-used not skipped)", got)
	}
}

// 3. both last-used and primary invalid -> the first enabled translate engine (id order).
func TestActiveTranslateEngineAllInvalidFallsBackToFirstEnabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	_, svc, w := setupActiveEnv(t, []*engine.EngineConfig{
		{Engine: "deepl", Enabled: true, APIKey: "k"},
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
	})
	svc.Get().DefaultEngine = "nosuchengine"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// deepl is first in id order and enabled; last-used and primary both invalid -> deepl.
	if got := w.ActiveTranslateEngine("nosuchengine2"); got != "deepl" {
		t.Fatalf("when both last-used and primary are invalid, should fall back to the first enabled translate engine deepl, got %q", got)
	}
}

// 4. all empty -> "" (no enabled translate engine in the list).
func TestActiveTranslateEngineAllEmptyReturnsEmpty(t *testing.T) {
	// No google/deepl: the openai row inserted by setupActiveEnv is disabled, so the list
	// contains no enabled translate engine.
	_, _, w := setupActiveEnv(t, []*engine.EngineConfig{
		{Engine: "tesseract", Enabled: true},
	})

	if got := w.ActiveTranslateEngine(""); got != "" {
		t.Fatalf("should resolve to \"\" when last-used and default_engine are unset and no translate engine is enabled, got %q", got)
	}
}

//  5. ordering: with last-used and primary both unset, take the first enabled translate
//     engine in configstore id order (first in id order = deepl; openai is also enabled but
//     its id sorts last).
func TestActiveTranslateEngineFirstEnabledIsIDOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	store, _, w := setupActiveEnv(t, []*engine.EngineConfig{
		{Engine: "deepl", Enabled: true, APIKey: "k"},
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
	})
	// Enable openai (inserted by setupActiveEnv, last in id order) so the enabled set is
	// {deepl, google, openai}: first-enabled must hit deepl by id order.
	ctx := context.Background()
	row, err := store.GetEngineByName(ctx, "openai")
	if err != nil || row == nil {
		t.Fatalf("GetEngineByName(openai): %v", err)
	}
	if err := store.SetEngineEnabled(ctx, row.ID, true); err != nil {
		t.Fatalf("SetEngineEnabled(openai): %v", err)
	}

	if got := w.ActiveTranslateEngine(""); got != "deepl" {
		t.Fatalf("with last-used/primary unset, should pick the first enabled engine by configstore id order (deepl), got %q (looks like alphabetical order or primary precedence)", got)
	}
}

//  6. last-used points at an engine that "exists but is not enabled" -> falls back to
//     primary (enabled taken from the enabled column, not the registry's Supported: openai
//     on Linux always has Supported true).
func TestActiveTranslateEngineLastUsedNotEnabledFallsBackToPrimary(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(gtxPrimaryFixture))
	}))
	defer srv.Close()

	// openai was inserted by setupActiveEnv and stays disabled.
	_, svc, w := setupActiveEnv(t, []*engine.EngineConfig{
		{Engine: "google", Enabled: true, Endpoint: srv.URL},
	})
	svc.Get().DefaultEngine = "google"
	if err := svc.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if got := w.ActiveTranslateEngine("openai"); got != "google" {
		t.Fatalf("last-used pointing at a disabled engine should fall back to primary google, got %q (looks like Supported used as enabled)", got)
	}
}
