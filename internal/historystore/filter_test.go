package historystore

import (
	"context"
	"path/filepath"
	"testing"
)

// Issue #60: the History screen filters by keyword, language pair and engine.

func seed(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	ctx := context.Background()
	rows := []InsertHistoryParams{
		{Text: "hola mundo", Result: "hello world", FromLang: "es", ToLang: "en", EngineID: 1, CreatedAt: 100},
		{Text: "buenos dias", Result: "good morning", FromLang: "es", ToLang: "en", EngineID: 2, CreatedAt: 200},
		{Text: "hello", Result: "hola", FromLang: "en", ToLang: "es", EngineID: 1, CreatedAt: 300},
		{Text: "hello again", Result: "otra vez hola", FromLang: "en", ToLang: "es", EngineID: 1, CreatedAt: 300},
	}
	for _, r := range rows {
		if _, err := s.InsertHistory(ctx, r); err != nil {
			t.Fatalf("InsertHistory: %v", err)
		}
	}
	return s
}

func texts(rows []History) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Text)
	}
	return out
}

func TestQueryFilters(t *testing.T) {
	s := seed(t)
	ctx := context.Background()
	cases := []struct {
		name string
		f    Filter
		want []string
	}{
		{"none, newest first with id tiebreak", Filter{}, []string{"hello again", "hello", "buenos dias", "hola mundo"}},
		{"keyword in text", Filter{Keyword: "mundo"}, []string{"hola mundo"}},
		{"keyword in result", Filter{Keyword: "morning"}, []string{"buenos dias"}},
		{"from lang", Filter{FromLang: "en"}, []string{"hello again", "hello"}},
		{"to lang", Filter{ToLang: "en"}, []string{"buenos dias", "hola mundo"}},
		{"engine", Filter{EngineID: 2}, []string{"buenos dias"}},
		{"combined", Filter{Keyword: "hola", FromLang: "es", EngineID: 1}, []string{"hola mundo"}},
		{"nothing matches", Filter{Keyword: "zzz"}, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rows, err := s.Query(ctx, c.f, 50, 0)
			if err != nil {
				t.Fatalf("Query: %v", err)
			}
			got := texts(rows)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
			n, err := s.Count(ctx, c.f)
			if err != nil {
				t.Fatalf("Count: %v", err)
			}
			if int(n) != len(c.want) {
				t.Errorf("Count = %d, want %d", n, len(c.want))
			}
		})
	}
}

func TestQueryPaging(t *testing.T) {
	s := seed(t)
	rows, err := s.Query(context.Background(), Filter{}, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	got := texts(rows)
	if len(got) != 2 || got[0] != "hello" || got[1] != "buenos dias" {
		t.Errorf("page = %v", got)
	}
}

func TestPairsAndEngineIDs(t *testing.T) {
	s := seed(t)
	ctx := context.Background()
	pairs, err := s.Pairs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(pairs) != 2 {
		t.Fatalf("pairs = %v, want 2 distinct", pairs)
	}
	ids, err := s.EngineIDs(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Errorf("engine ids = %v, want [1 2]", ids)
	}
}
