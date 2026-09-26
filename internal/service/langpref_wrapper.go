package service

import (
	"cnb.cool/dtapp/kai/internal/langpref"
	"cnb.cool/dtapp/kai/internal/model"
)

// LangPrefWrapper is the thin adapter over the language-variant preference store (issue #53):
// RPC passthrough only, no wails lifecycle. The frontend feeds the store from its language
// selects (Learn); everything else — qualifying detected languages at translation time, ending
// the session on window close — happens in the backend and needs no binding.
type LangPrefWrapper struct {
	store *langpref.Store
}

// NewLangPrefWrapper constructs the preference Wrapper over the store the translate service
// qualifies with (one shared instance, wired in main.go).
func NewLangPrefWrapper(store *langpref.Store) *LangPrefWrapper {
	return &LangPrefWrapper{store: store}
}

// Learn records an explicit language selection: picking a dialect (es-MX, pt-BR, pt-PT, ...) in
// any language select makes it the preferred variant of its language for the rest of the window
// session, so later auto-detected text in that language is qualified as the variant. Bases and
// languages without dialects are ignored by the store, so the frontend passes every explicit
// select change through — and only those: swapping the languages and loading saved defaults are
// not selections and must never call it.
func (w *LangPrefWrapper) Learn(lang model.Language) {
	w.store.Learn(lang)
}
