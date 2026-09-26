// Package langpref is the backend-owned language variant preference store (issue #53).
//
// A dialect preference is a fact about the USER, not about a text: someone who picks Spanish
// (Mexico) once means Mexican Spanish for every Spanish text they translate afterwards. The store
// remembers that as a base → variant map (es → es-MX) and qualifies detected languages through it
// at translation time, so an engine that reports a bare "es" is used and shown as es-MX without
// any further interaction.
//
// The rules, in one place:
//   - Learned from EXPLICIT selections only (Learn). A base language, a language without
//     dialects and anything unrecognized never teaches anything, so bare es / pt cannot become a
//     preference. Callers may therefore pass every explicit selection through unfiltered.
//   - Applied at qualification (Qualify): the detected code is normalized first (an alias such as
//     es-419 folds to its base, model.Language.Normalize), then looked up — the learned variant
//     when there is one, otherwise the plain base language.
//   - Last explicit selection wins within a family (pt-BR, then pt-PT → pt-PT).
//   - Session-scoped: the store lives in memory for the life of the process (the app keeps a
//     learned variant until it quits; Reset exists for callers that need to end a session
//     earlier, and production code does not call it today). The API is small and
//     value-based on purpose — keyed by canonical model.Language, Learn idempotent, Qualify a pure
//     read — so settings persistence can later sit behind it (seed a store by replaying Learn,
//     write through from Learn) without changing any caller.
//
// The package depends on model only; wiring (which windows share a store, when it is reset) lives
// in main.go, and the translate service consumes it through translate.Service.SetLangPrefs.
package langpref

import (
	"sync"

	"cnb.cool/dtapp/kai/internal/model"
)

// Store holds the user's learned variant preferences for one session. It is safe for concurrent
// use (Learn arrives from the frontend while translations qualify on engine goroutines). The zero
// value is an empty, ready-to-use store; New is the conventional constructor.
type Store struct {
	mu sync.RWMutex
	// prefs maps a base language to the variant the user last chose explicitly (es → es-MX).
	prefs map[model.Language]model.Language
}

// New returns an empty store: no preferences learned, so Qualify only normalizes.
func New() *Store { return &Store{} }

// Learn records an explicit language selection. Picking a dialect (es-MX, pt-BR, pt-PT, ...) sets
// that dialect as the preferred variant of its base language, replacing an earlier choice
// (last explicit selection wins). Anything that is not a dialect of a recognized base — a base
// language itself, auto, a language without dialects, an empty or unrecognized code — is ignored:
// only picking a concrete variant expresses a variant preference.
//
// The code is parsed through model.ParseLanguage, so spelling and case do not matter (pt-br
// learns pt-BR). Learn must only be fed explicit user selections: swapping the two languages and
// hydrating the selects from saved settings are not choices and must never reach it.
func (s *Store) Learn(l model.Language) {
	variant, ok := model.ParseLanguage(string(l))
	if !ok {
		return
	}
	base := variant.Base()
	if base == variant {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.prefs == nil {
		s.prefs = make(map[model.Language]model.Language)
	}
	s.prefs[base] = variant
}

// Qualify resolves a detected language to what the user means by it: the learned variant of its
// family when there is one, otherwise the normalized base language (an alias such as es-419
// collapses to es, never to a guessed variant). Codes that are not recognized — a language the
// app has no names for, auto, an empty code — pass through unchanged.
//
// A nil *Store holds no preferences, so Qualify on it only normalizes; that is how a
// translate.Service without a store still carries a clean detected language.
func (s *Store) Qualify(l model.Language) model.Language {
	base := l.Normalize()
	if s == nil {
		return base
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if variant, ok := s.prefs[base]; ok {
		return variant
	}
	return base
}

// Reset forgets every learned preference, so the next session starts empty. Not called in
// production today: the session is the process, so a restart already starts empty.
func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prefs = nil
}
