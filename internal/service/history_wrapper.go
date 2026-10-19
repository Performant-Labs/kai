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

// engineIDByName resolves an engine name (the value the frontend filters on) to its config
// id. An empty name matches any engine (0 = the filter's "any" value). A name no configured
// engine has resolves to -1, an id no row carries, so it matches nothing (issue #60).
func (w *HistoryWrapper) engineIDByName(ctx context.Context, engine string) int64 {
	if engine == "" || w.configStore == nil {
		return 0
	}
	eng, err := w.configStore.GetEngineByName(ctx, engine)
	if err != nil || eng == nil {
		return -1
	}
	return eng.ID
}

// GetHistory queries translation history with pagination and the history screen's filters
// (issue #60): keyword matches source and result text, fromLang/toLang compare the stored
// pair, engine names the engine. An empty filter value matches anything.
func (w *HistoryWrapper) GetHistory(keyword, fromLang, toLang, engine string, offset, limit int) []HistoryItem {
	if w.store == nil {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := w.store.QueryByFilter(ctx, historystore.HistoryFilter{
		Keyword:  keyword,
		FromLang: fromLang,
		ToLang:   toLang,
		EngineID: w.engineIDByName(ctx, engine),
	}, int64(limit), int64(offset))
	if err != nil {
		return nil
	}
	out := make([]HistoryItem, 0, len(rows))
	for _, r := range rows {
		engineName := ""
		if w.configStore != nil && r.EngineID != 0 {
			if eng, e := w.configStore.GetEngineByID(ctx, r.EngineID); e == nil && eng != nil {
				engineName = eng.Engine
			}
		}
		out = append(out, HistoryItem{
			ID:        r.ID,
			Text:      r.Text,
			Result:    r.Result,
			FromLang:  r.FromLang,
			ToLang:    r.ToLang,
			Engine:    engineName,
			FromOCR:   r.FromOcr != 0,
			CreatedAt: r.CreatedAt,
		})
	}
	return out
}

// CountHistory returns the total history count matching the keyword and filters, for the
// frontend's page count.
func (w *HistoryWrapper) CountHistory(keyword, fromLang, toLang, engine string) int64 {
	if w.store == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n, err := w.store.CountByFilter(ctx, historystore.HistoryFilter{
		Keyword:  keyword,
		FromLang: fromLang,
		ToLang:   toLang,
		EngineID: w.engineIDByName(ctx, engine),
	})
	if err != nil {
		return 0
	}
	return n
}

// DeleteHistory deletes one history entry.
func (w *HistoryWrapper) DeleteHistory(id int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return w.store.DeleteHistory(ctx, id)
}

// ClearHistory clears all history.
func (w *HistoryWrapper) ClearHistory() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return w.store.ClearHistory(ctx)
}
