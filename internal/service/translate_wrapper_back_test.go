package service

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/translate"
)

// Issue #56: the binding the window calls exists on TranslateWrapper, and it never raises an error:
// an unknown engine is a failed status, not a rejected call.
func TestTranslateWrapperBackTranslateAnswersWithAStatus(t *testing.T) {
	w := NewTranslateWrapper(translate.NewService(engine.NewRegistry(), nil, nil, nil))
	got := w.BackTranslate(model.TranslateRequest{Text: "hello there my friend", From: model.EN, To: model.ESMX, EngineName: "nope", RequestID: "b1"})
	if got.Status != model.BackTranslateFailed || got.RequestID != "b1" {
		t.Fatalf("got %+v, want failed for b1", got)
	}
}
