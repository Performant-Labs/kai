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

// GetHistory queries translation history with pagination.
func (w *HistoryWrapper) GetHistory(keyword string, offset, limit int) []HistoryItem {
	if w.store == nil {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := w.store.QueryByKeyword(ctx, keyword, int64(limit), int64(offset))
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

// CountHistory returns the total history count matching the keyword, for the frontend's page
// count.
func (w *HistoryWrapper) CountHistory(keyword string) int64 {
	if w.store == nil {
		return 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	n, err := w.store.CountByKeyword(ctx, keyword)
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
