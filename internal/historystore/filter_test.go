package historystore

import (
	"context"
	"path/filepath"
	"testing"
)

// Issue #60, PR #67 review: the engine-name → engine-id resolution and the carrying of the
// text filters into HistoryFilter lived only inside internal/service's wrapper, which cannot
// be built on the Linux worker. The rules live here, as pure functions over a lookup, so this
// file pins them where they can run: an empty engine name matches any engine, a name the
// lookup cannot resolve matches nothing, a resolved name filters by its id, and every filter
// argument reaches the HistoryFilter the store queries with.

// A lookup that knows "google" (id 1) and "apple" (id 2) and nothing else.
func fakeLookup(name string) (int64, bool) {
	known := map[string]int64{"google": 1, "apple": 2}
	id, ok := known[name]
	return id, ok
}

func TestResolveEngineID(t *testing.T) {
	cases := []struct {
		engine string
		want   int64
		why    string
	}{
		{"", 0, "an empty name matches any engine (HistoryFilter's zero value)"},
		{"google", 1, "a name the lookup resolves filters by its id"},
		{"apple", 2, "a second known engine resolves to its own id"},
		{"no-such-engine", -1, "a name no configured engine has matches nothing"},
	}
	for _, c := range cases {
		got := ResolveEngineID(c.engine, fakeLookup)
		if got != c.want {
			t.Errorf("ResolveEngineID(%q) = %d, want %d: %s", c.engine, got, c.want, c.why)
		}
	}
	// A nil lookup resolves nothing: every non-empty name matches nothing.
	if got := ResolveEngineID("google", nil); got != -1 {
		t.Errorf("ResolveEngineID with a nil lookup = %d, want -1", got)
	}
	if got := ResolveEngineID("", nil); got != 0 {
		t.Errorf("ResolveEngineID('') with a nil lookup = %d, want 0", got)
	}
}

func TestFilterFromCarriesEveryArgument(t *testing.T) {
	f := FilterFrom("Hola", "es", "en", "google", fakeLookup)
	if f != (HistoryFilter{Keyword: "Hola", FromLang: "es", ToLang: "en", EngineID: 1}) {
		t.Fatalf("FilterFrom did not carry the arguments into the filter: got %+v", f)
	}
	// Empty values become the match-anything filter.
	if f := FilterFrom("", "", "", "", fakeLookup); f != (HistoryFilter{}) {
		t.Fatalf("FilterFrom of empty values must be the empty filter, got %+v", f)
	}
	// An unresolvable engine name becomes the match-nothing id.
	if f := FilterFrom("", "", "", "no-such-engine", fakeLookup); f.EngineID != -1 {
		t.Fatalf("FilterFrom of an unknown engine must carry EngineID -1, got %+v", f)
	}
}

// The resolved rules against a real store: the filter a name builds must select exactly the
// rows that engine produced, and an unknown name must select none of them.
func TestFilterFromSelectsLikeTheStoreQueries(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "history.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()
	ctx := context.Background()
	for _, r := range [][6]string{
		{"uno", "one", "es", "en", "google", "0"},
		{"dos", "two", "es", "en", "apple", "0"},
		{"trois", "three", "fr", "en", "google", "0"},
	} {
		if _, err := s.InsertHistory(ctx, InsertHistoryParams{
			Text: r[0], Result: r[1], FromLang: r[2], ToLang: r[3],
			EngineID:  map[string]int64{"google": 1, "apple": 2}[r[4]],
			CreatedAt: 1000,
		}); err != nil {
			t.Fatalf("insert %q: %v", r[0], err)
		}
	}

	rows, err := s.QueryByFilter(ctx, FilterFrom("", "", "", "google", fakeLookup), 50, 0)
	if err != nil {
		t.Fatalf("QueryByFilter(known engine): %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("the google filter selected %d rows, want its 2: %+v", len(rows), rows)
	}

	rows, err = s.QueryByFilter(ctx, FilterFrom("", "", "", "no-such-engine", fakeLookup), 50, 0)
	if err != nil {
		t.Fatalf("QueryByFilter(unknown engine): %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("an unknown engine name must match nothing, got %d rows: %+v", len(rows), rows)
	}

	rows, err = s.QueryByFilter(ctx, FilterFrom("trois", "fr", "en", "google", fakeLookup), 50, 0)
	if err != nil {
		t.Fatalf("QueryByFilter(all filters): %v", err)
	}
	if len(rows) != 1 || rows[0].Text != "trois" {
		t.Fatalf("the combined filter selected %+v, want only 'trois'", rows)
	}
}
