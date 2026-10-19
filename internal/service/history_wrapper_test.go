package service

import (
	"context"
	"path/filepath"
	"testing"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	kevents "cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/historystore"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #60: the History screen's backend surface.

func newHistoryWrapper(t *testing.T) (*HistoryWrapper, int64) {
	t.Helper()
	dir := t.TempDir()
	hs, err := historystore.Open(filepath.Join(dir, "history.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { hs.Close() })
	cs, err := configstore.Open(filepath.Join(dir, "config.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	ctx := context.Background()
	engID, err := cs.InsertEngineConfig(ctx, &engine.EngineConfig{Engine: "google", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	ins := func(text, from, to string, eng int64, at int64) {
		if _, err := hs.InsertHistory(ctx, historystore.InsertHistoryParams{
			Text: text, Result: "r-" + text, FromLang: from, ToLang: to, EngineID: eng, CreatedAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	ins("uno", "es", "en", engID, 1)
	ins("dos", "es", "en", engID, 2)
	ins("three", "en", "es", 0, 3)
	return NewHistoryWrapper(hs, cs), engID
}

func TestQueryHistoryPageAndTotal(t *testing.T) {
	w, engID := newHistoryWrapper(t)
	p := w.QueryHistory(HistoryFilter{}, 0, 2)
	if p.Total != 3 || len(p.Items) != 2 || p.Items[0].Text != "three" {
		t.Fatalf("page = %+v", p)
	}
	p = w.QueryHistory(HistoryFilter{EngineID: engID}, 0, 0)
	if p.Total != 2 || len(p.Items) != 2 {
		t.Fatalf("engine page = %+v", p)
	}
	if p.Items[0].Engine != "google" || p.Items[0].EngineID != engID {
		t.Errorf("engine not resolved: %+v", p.Items[0])
	}
	p = w.QueryHistory(HistoryFilter{Keyword: "zzz"}, 0, 10)
	if p.Items == nil || len(p.Items) != 0 || p.Total != 0 {
		t.Errorf("empty page must have non-nil Items: %+v", p)
	}
}

func TestHistoryFacets(t *testing.T) {
	w, engID := newHistoryWrapper(t)
	f := w.HistoryFacets()
	if len(f.Pairs) != 2 {
		t.Errorf("pairs = %+v", f.Pairs)
	}
	if len(f.Engines) != 1 || f.Engines[0].ID != engID || f.Engines[0].Engine != "google" {
		t.Errorf("engines = %+v (engine 0 must be skipped)", f.Engines)
	}
}

func TestDeleteAndClearHistory(t *testing.T) {
	w, _ := newHistoryWrapper(t)
	first := w.QueryHistory(HistoryFilter{}, 0, 10).Items[0]
	if err := w.DeleteHistory(first.ID); err != nil {
		t.Fatal(err)
	}
	if n := w.QueryHistory(HistoryFilter{}, 0, 10).Total; n != 2 {
		t.Errorf("after delete total = %d", n)
	}
	if err := w.ClearHistory(); err != nil {
		t.Fatal(err)
	}
	if n := w.QueryHistory(HistoryFilter{}, 0, 10).Total; n != 0 {
		t.Errorf("after clear total = %d", n)
	}
}

func TestNilStoreWrapperIsSafe(t *testing.T) {
	w := NewHistoryWrapper(nil, nil)
	if p := w.QueryHistory(HistoryFilter{}, 0, 10); p.Items == nil || p.Total != 0 {
		t.Errorf("page = %+v", p)
	}
	if err := w.DeleteHistory(1); err != nil {
		t.Errorf("DeleteHistory: %v", err)
	}
	if err := w.ClearHistory(); err != nil {
		t.Errorf("ClearHistory: %v", err)
	}
}

func TestHistoryWindowAndEventNames(t *testing.T) {
	if model.WindowHistory != "history" || kevents.EventHistoryUse != "kai:history:use" ||
		kevents.EventHistoryShortcutChanged != "kai:history:shortcut-changed" {
		t.Error("history window/event names changed; the frontend depends on them")
	}
}
