package engine

import (
	"errors"

	"cnb.cool/dtapp/kai/internal/model"
)

// An engine that fails can still know what language the text was in. Apple's bridge detects the
// source before it translates (NaturalLanguage) and, when the framework then refuses the pair,
// used to throw that detection away. WithDetectedSource attaches it to the failure and
// DetectedSourceOf reads it back, so the translate service can tell "the text is already in the
// target language" (issue #80) from a real failure without a language detector of its own. The
// engine only reports the fact; deciding what it means stays with the service.

// detectedSourceError is an error that carries the source language its engine detected. Its text
// is the wrapped error's, byte for byte, and it unwraps to it, so errors.Is / errors.As and every
// %w wrap on the way up (callEngine's "%s(%s): %w") still reach the original error.
type detectedSourceError struct {
	err  error
	from model.Language
}

func (e *detectedSourceError) Error() string { return e.err.Error() }
func (e *detectedSourceError) Unwrap() error { return e.err }

// WithDetectedSource returns err carrying the source language the engine detected in the text,
// bare as the engine reports it (es, zh, en). A nil err stays nil, and an empty or auto from is
// not a detection, so err comes back as it was.
func WithDetectedSource(err error, from model.Language) error {
	if err == nil || isAuto(string(from)) {
		return err
	}
	return &detectedSourceError{err: err, from: from}
}

// DetectedSourceOf returns the detected source language an engine attached to err, looking
// through wrapping; ok is false when err carries none (a plain failure, or nil).
func DetectedSourceOf(err error) (model.Language, bool) {
	var d *detectedSourceError
	if errors.As(err, &d) {
		return d.from, true
	}
	return "", false
}
