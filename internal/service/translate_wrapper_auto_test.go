package service

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/translate"
)

// Issue #57: the binding the window calls once typed text has settled exists, and committing an
// unknown request writes nothing and raises nothing.
func TestTranslateWrapperCommitAutoHistoryForAnUnknownRequest(t *testing.T) {
	w := NewTranslateWrapper(translate.NewService(engine.NewRegistry(), nil, nil, nil))
	if got := w.CommitAutoHistory("nope"); got != 0 {
		t.Fatalf("saved %d rows for an unknown request, want 0", got)
	}
}
