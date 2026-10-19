package service

import (
	"context"
	"time"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/historystore"
)

// HistoryItem is a single history entry returned to the frontend (time as a millisecond
// timestamp, engine ID resolved to a readable name).
type HistoryItem struct {
	ID        int64  `json:"id"`         // History record auto-increment primary key
	EngineID  int64  `json:"engine_id"`  // Engine config ID (0 when unknown); the engine filter key
	Text      string `json:"text"`       // Source text
	Result    string `json:"result"`     // Translation result
	FromLang  string `json:"from_lang"`  // Source language code
	ToLang    string `json:"to_lang"`    // Target language code
	Engine    string `json:"engine"`     // Engine identifier used
	FromOCR   bool   `json:"from_ocr"`   // Whether it came from OCR recognition
	CreatedAt int64  `json:"created_at"` // Creation time (millisecond timestamp)
}

// HistoryWrapper is the thin adapter over translation history: holds historystore.Store +
// configstore, doing only RPC passthrough and DTO conversion.
// Does not implement the wails lifecycle trio.
type HistoryWrapper struct {
	store       *historystore.Store
	configStore *configstore.Store
}

// NewHistoryWrapper constructs the history Wrapper.
func NewHistoryWrapper(store *historystore.Store, cs *configstore.Store) *HistoryWrapper {
	return &HistoryWrapper{store: store, configStore: cs}
}

// HistoryFilter narrows the list (issue #60). A zero field means "any".
type HistoryFilter struct {
	Keyword  string `json:"keyword"`
	FromLang string `json:"from_lang"`
	ToLang   string `json:"to_lang"`
	EngineID int64  `json:"engine_id"`
}

// HistoryPage is one page of results plus the total under the same filter.
type HistoryPage struct {
	Items []HistoryItem `json:"items"`
	Total int64         `json:"total"`
}

// HistoryPair is a language pair present in the history.
type HistoryPair struct {
	FromLang string `json:"from_lang"`
	ToLang   string `json:"to_lang"`
}

// HistoryEngine is an engine present in the history, with its resolved name.
type HistoryEngine struct {
	ID     int64  `json:"id"`
	Engine string `json:"engine"`
}

// HistoryFacets is what the filter dropdowns offer: only values that exist in the data.
type HistoryFacets struct {
	Pairs   []HistoryPair   `json:"pairs"`
	Engines []HistoryEngine `json:"engines"`
}

func (w *HistoryWrapper) engineName(ctx context.Context, id int64) string {
	if w.configStore == nil || id == 0 {
		return ""
	}
	if eng, err := w.configStore.GetEngineByID(ctx, id); err == nil && eng != nil {
		return eng.Engine
	}
	return ""
}

// QueryHistory returns one page of history under the filter, newest first, with the total.
func (w *HistoryWrapper) QueryHistory(f HistoryFilter, offset, limit int) HistoryPage {
	page := HistoryPage{Items: []HistoryItem{}}
	if w.store == nil {
		return page
	}
	if limit <= 0 {
		limit = 50
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sf := historystore.Filter{Keyword: f.Keyword, FromLang: f.FromLang, ToLang: f.ToLang, EngineID: f.EngineID}
	rows, err := w.store.Query(ctx, sf, int64(limit), int64(offset))
	if err != nil {
		return page
	}
	for _, r := range rows {
		page.Items = append(page.Items, HistoryItem{
			ID:        r.ID,
			Text:      r.Text,
			Result:    r.Result,
			FromLang:  r.FromLang,
			ToLang:    r.ToLang,
			Engine:    w.engineName(ctx, r.EngineID),
			EngineID:  r.EngineID,
			FromOCR:   r.FromOcr != 0,
			CreatedAt: r.CreatedAt,
		})
	}
	if n, err := w.store.Count(ctx, sf); err == nil {
		page.Total = n
	}
	return page
}

// HistoryFacets lists the language pairs and engines present in the history.
func (w *HistoryWrapper) HistoryFacets() HistoryFacets {
	out := HistoryFacets{Pairs: []HistoryPair{}, Engines: []HistoryEngine{}}
	if w.store == nil {
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if pairs, err := w.store.Pairs(ctx); err == nil {
		for _, p := range pairs {
			out.Pairs = append(out.Pairs, HistoryPair{FromLang: p.FromLang, ToLang: p.ToLang})
		}
	}
	if ids, err := w.store.EngineIDs(ctx); err == nil {
		for _, id := range ids {
			if id == 0 {
				continue
			}
			out.Engines = append(out.Engines, HistoryEngine{ID: id, Engine: w.engineName(ctx, id)})
		}
	}
	return out
}

// DeleteHistory deletes one history entry.
func (w *HistoryWrapper) DeleteHistory(id int64) error {
	if w.store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return w.store.DeleteHistory(ctx, id)
}

// ClearHistory clears all history.
func (w *HistoryWrapper) ClearHistory() error {
	if w.store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return w.store.ClearHistory(ctx)
}
