package engine

import (
	"context"

	"cnb.cool/dtapp/kai/internal/model"
)

// notConfiguredTranslator stands in for an enabled engine that cannot run as configured (issue
// #96). Registering it, instead of the real engine or nothing, gives the engine a slot in the
// fan-out, so its pane and dot report the reason at once instead of timing out with none. It never
// makes a network call.
type notConfiguredTranslator struct {
	name string
	err  error
}

// NewNotConfigured returns the stub for the engine called name. Translate always returns err,
// which is engine.ErrAPIKey (wrapped or as is) for a missing credential, so the classifier reports
// it as not configured. registerEngines uses it when ValidateRequired reports a missing field, and
// for an engine whose constructor failed, which used to be skipped without a trace.
func NewNotConfigured(name string, err error) Translator {
	return &notConfiguredTranslator{name: name, err: err}
}

// Name is the engine the stub stands in for: the registry keys on it, and a later real
// registration of the same engine replaces the stub.
func (s *notConfiguredTranslator) Name() string { return s.name }

// Translate returns the stub's error without any request.
func (s *notConfiguredTranslator) Translate(context.Context, model.TranslateRequest) (*model.TranslateResult, error) {
	return nil, s.err
}

// notConfigured is the marker IsNotConfigured looks for.
func (s *notConfiguredTranslator) notConfigured() bool { return true }

// notConfiguredMarker is the optional interface that IsNotConfigured discovers by type assertion,
// like autoSourceSupporter.
type notConfiguredMarker interface {
	notConfigured() bool
}

// IsNotConfigured reports whether t is a stand-in registered by NewNotConfigured, so a caller that
// counts working engines (the analytics engine list) can skip it. Registration decided this, from
// ValidateRequired, which stays the only definition of "configured"; this predicate only reports
// the decision.
func IsNotConfigured(t Translator) bool {
	if m, ok := t.(notConfiguredMarker); ok {
		return m.notConfigured()
	}
	return false
}
