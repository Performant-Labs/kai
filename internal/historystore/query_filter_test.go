package historystore

import (
	"context"
	"path/filepath"
	"testing"
)

// Issue #60: the history screen filters by language pair and engine on top of the keyword
// search. These tests pin the filtered queries against a real SQLite database in t.TempDir()
// (per Epic #6), exercising the same generated SQL the app runs.

// insertRows inserts (text, result, from, to, engineID, ocr) tuples with increasing created_at
// and returns nothing; failures abort the test.
func insertRows(t *testing.T, s *Store, rows [][6]any) {
	t.Helper()
	ctx := context.Background()
	for i, r := range rows {
		if _, err := s.InsertHistory(ctx, InsertHistoryParams{
			Text:      r[0].(string),
			Result:    r[1].(string),
			FromLang:  r[2].(string),
			ToLang:    r[3].(string),
			EngineID:  int64(r[4].(int)),
			FromOcr:   int64(r[5].(int)),
			CreatedAt: int64(1000 + i),
		}); err != nil {
			t.Fatalf("insert row %d: %v", i, err)
		}
	}
}

func openFileStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestQueryByFilterLanguagePair(t *testing.T) {
	s := openFileStore(t)
	insertRows(t, s, [][6]any{
		{"Hello", "Hola", "en", "es", 1, 0},
		{"Hola", "Hello", "es", "en", 2, 0},
		{"Goodbye", "Adiós", "en", "es", 2, 0},
		{"Bonjour", "Hola", "fr", "es", 1, 1},
	})
	ctx := context.Background()

	from, err := s.QueryByFilter(ctx, HistoryFilter{FromLang: "en"}, 50, 0)
	if err != nil {
		t.Fatalf("QueryByFilter FromLang: %v", err)
	}
	if len(from) != 2 {
		t.Fatalf("FromLang=en: got %d rows, want 2", len(from))
	}
	for _, r := range from {
		if r.FromLang != "en" {
			t.Fatalf("FromLang=en returned a row with from=%q", r.FromLang)
		}
	}

	to, err := s.QueryByFilter(ctx, HistoryFilter{ToLang: "en"}, 50, 0)
	if err != nil {
		t.Fatalf("QueryByFilter ToLang: %v", err)
	}
	if len(to) != 1 || to[0].Text != "Hola" {
		t.Fatalf("ToLang=en: got %+v, want the one 'Hola' row", to)
	}

	pair, err := s.QueryByFilter(ctx, HistoryFilter{FromLang: "en", ToLang: "es"}, 50, 0)
	if err != nil {
		t.Fatalf("QueryByFilter pair: %v", err)
	}
	if len(pair) != 2 {
		t.Fatalf("en->es: got %d rows, want 2", len(pair))
	}
}

func TestQueryByFilterEngine(t *testing.T) {
	s := openFileStore(t)
	insertRows(t, s, [][6]any{
		{"Hello", "Hola", "en", "es", 1, 0},
		{"Hola", "Hello", "es", "en", 2, 0},
		{"Goodbye", "Adiós", "en", "es", 2, 0},
	})
	ctx := context.Background()

	rows, err := s.QueryByFilter(ctx, HistoryFilter{EngineID: 2}, 50, 0)
	if err != nil {
		t.Fatalf("QueryByFilter EngineID: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("EngineID=2: got %d rows, want 2", len(rows))
	}
	for _, r := range rows {
		if r.EngineID != 2 {
			t.Fatalf("EngineID=2 returned a row with engine_id=%d", r.EngineID)
		}
	}
}

func TestQueryByFilterKeywordCombines(t *testing.T) {
	s := openFileStore(t)
	insertRows(t, s, [][6]any{
		{"Hello world", "Hola mundo", "en", "es", 1, 0},
		{"Hello there", "Hola amigo", "en", "es", 2, 0},
		{"Goodbye world", "Adiós mundo", "en", "es", 1, 0},
	})
	ctx := context.Background()

	rows, err := s.QueryByFilter(ctx, HistoryFilter{Keyword: "Hello", EngineID: 1}, 50, 0)
	if err != nil {
		t.Fatalf("QueryByFilter keyword+engine: %v", err)
	}
	if len(rows) != 1 || rows[0].Text != "Hello world" {
		t.Fatalf("keyword 'Hello' + engine 1: got %+v, want only 'Hello world'", rows)
	}
}

func TestQueryByFilterEmptyMatchesEverything(t *testing.T) {
	s := openFileStore(t)
	insertRows(t, s, [][6]any{
		{"Hello", "Hola", "en", "es", 1, 0},
		{"Hola", "Hello", "es", "en", 2, 0},
	})
	ctx := context.Background()

	rows, err := s.QueryByFilter(ctx, HistoryFilter{}, 50, 0)
	if err != nil {
		t.Fatalf("QueryByFilter empty: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("empty filter: got %d rows, want 2", len(rows))
	}
	// Newest first, like the unfiltered query.
	if rows[0].CreatedAt <= rows[1].CreatedAt {
		t.Fatalf("empty filter: rows are not newest first: %+v", rows)
	}

	// The empty filter must agree with the plain keyword query.
	plain, err := s.QueryByKeyword(ctx, "", 50, 0)
	if err != nil {
		t.Fatalf("QueryByKeyword: %v", err)
	}
	if len(plain) != len(rows) {
		t.Fatalf("empty filter returned %d rows, QueryByKeyword returned %d", len(rows), len(plain))
	}
}

func TestQueryByFilterPagesWithLimitOffset(t *testing.T) {
	s := openFileStore(t)
	insertRows(t, s, [][6]any{
		{"one", "uno", "en", "es", 1, 0},
		{"two", "dos", "en", "es", 1, 0},
		{"three", "tres", "en", "es", 1, 0},
		{"four", "cuatro", "fr", "es", 2, 0},
	})
	ctx := context.Background()
	f := HistoryFilter{ToLang: "es"}

	page1, err := s.QueryByFilter(ctx, f, 2, 0)
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	page2, err := s.QueryByFilter(ctx, f, 2, 2)
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page1) != 2 || len(page2) != 2 {
		t.Fatalf("pages: got %d and %d rows, want 2 and 2", len(page1), len(page2))
	}
	if page1[0].CreatedAt != 1003 || page1[1].CreatedAt != 1002 || page2[0].CreatedAt != 1001 {
		t.Fatalf("pages: wrong order or rows: p1=%+v p2=%+v", page1, page2)
	}
}

func TestCountByFilterMatchesQuery(t *testing.T) {
	s := openFileStore(t)
	insertRows(t, s, [][6]any{
		{"Hello", "Hola", "en", "es", 1, 0},
		{"Hola", "Hello", "es", "en", 2, 0},
		{"Goodbye", "Adiós", "en", "es", 2, 0},
	})
	ctx := context.Background()

	cases := []HistoryFilter{
		{},
		{FromLang: "en"},
		{ToLang: "en"},
		{EngineID: 2},
		{Keyword: "Hola"},
		{FromLang: "en", EngineID: 2},
	}
	for _, f := range cases {
		rows, err := s.QueryByFilter(ctx, f, 50, 0)
		if err != nil {
			t.Fatalf("QueryByFilter %+v: %v", f, err)
		}
		n, err := s.CountByFilter(ctx, f)
		if err != nil {
			t.Fatalf("CountByFilter %+v: %v", f, err)
		}
		if int(n) != len(rows) {
			t.Fatalf("filter %+v: count says %d, query returned %d", f, n, len(rows))
		}
	}
}
