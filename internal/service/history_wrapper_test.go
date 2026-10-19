package service

import (
	"context"
	"path/filepath"
	"testing"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/historystore"
)

// PR #67 review: the history filter rules the wrapper carries — the engine-name resolution
// (empty name matches anything, an unknown name matches nothing, a known name selects its
// rows) and the keyword/pair passthrough into historystore.FilterFrom — end to end through
// HistoryWrapper.GetHistory/CountHistory, with real stores under t.TempDir and no mocks (the
// package's own test style). The pure half is internal/historystore/filter_test.go, which
// runs anywhere; this file additionally pins the wrapper's wiring, so it belongs where the
// package builds (darwin). Each assertion fails if the wrapper stops carrying its filter or
// its lookup: an unresolvable name would come back as "any engine" (too many rows) and a
// dropped filter as an unfiltered list.

func setupHistoryWrapper(t *testing.T) (*HistoryWrapper, *historystore.Store) {
	t.Helper()
	cfg, err := configstore.Open(filepath.Join(t.TempDir(), "config.db"))
	if err != nil {
		t.Fatalf("configstore.Open: %v", err)
	}
	t.Cleanup(func() { _ = cfg.Close() })
	hist, err := historystore.Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("historystore.Open: %v", err)
	}
	t.Cleanup(func() { _ = hist.Close() })

	ctx := context.Background()
	if err := cfg.InitDefaultEngines(ctx, []*engine.EngineConfig{
		{Engine: "google", Enabled: true},
		{Engine: "apple", Enabled: true},
	}); err != nil {
		t.Fatalf("InitDefaultEngines: %v", err)
	}

	googleID, err := cfg.GetEngineByName(ctx, "google")
	if err != nil || googleID == nil {
		t.Fatalf("GetEngineByName(google): %v %v", googleID, err)
	}
	appleID, err := cfg.GetEngineByName(ctx, "apple")
	if err != nil || appleID == nil {
		t.Fatalf("GetEngineByName(apple): %v %v", appleID, err)
	}

	for _, r := range []historystore.InsertHistoryParams{
		{Text: "Hello", Result: "Hola", FromLang: "en", ToLang: "es", EngineID: googleID.ID, CreatedAt: 2000},
		{Text: "Adiós", Result: "Goodbye", FromLang: "es", ToLang: "en", EngineID: appleID.ID, CreatedAt: 1000},
	} {
		if _, err := hist.InsertHistory(ctx, r); err != nil {
			t.Fatalf("InsertHistory(%q): %v", r.Text, err)
		}
	}
	return NewHistoryWrapper(hist, cfg), hist
}

func TestHistoryWrapperCarriesFilters(t *testing.T) {
	w, _ := setupHistoryWrapper(t)

	// The empty filter matches anything, newest first, with the engine name resolved in the DTO.
	all := w.GetHistory("", "", "", "", 0, 50)
	if len(all) != 2 || all[0].Text != "Hello" || all[1].Text != "Adiós" {
		t.Fatalf("GetHistory (no filter) = %+v, want both rows newest first", all)
	}
	if all[0].Engine != "google" || all[1].Engine != "apple" {
		t.Fatalf("GetHistory did not resolve engine names: %+v", all)
	}
	n := w.CountHistory("", "", "", "")
	if n != 2 {
		t.Fatalf("CountHistory (no filter) = %d, want 2", n)
	}

	// A known engine name selects that engine's rows only.
	google := w.GetHistory("", "", "", "google", 0, 50)
	if len(google) != 1 || google[0].Text != "Hello" {
		t.Fatalf("GetHistory (google) = %+v, want only 'Hello'", google)
	}
	n = w.CountHistory("", "", "", "google")
	if n != 1 {
		t.Fatalf("CountHistory (google) = %d, want 1", n)
	}

	// A name no configured engine has matches nothing — never the whole history.
	unknown := w.GetHistory("", "", "", "no-such-engine", 0, 50)
	if len(unknown) != 0 {
		t.Fatalf("GetHistory (unknown engine) = %+v, want no rows", unknown)
	}
	n = w.CountHistory("", "", "", "no-such-engine")
	if n != 0 {
		t.Fatalf("CountHistory (unknown engine) = %d, want 0", n)
	}

	// The keyword and the pair are carried through and combine with the engine.
	combined := w.GetHistory("Goodbye", "es", "en", "apple", 0, 50)
	if len(combined) != 1 || combined[0].Text != "Adiós" {
		t.Fatalf("GetHistory (combined) = %+v, want only 'Adiós'", combined)
	}
	miss := w.GetHistory("Goodbye", "es", "en", "google", 0, 50)
	if len(miss) != 0 {
		t.Fatalf("GetHistory (combined miss) = %+v, want no rows", miss)
	}
}
