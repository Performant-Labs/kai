package service

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/translate"
)

// Issue #48: the binding the chat calls exists on TranslateWrapper. With no context message it
// answers no_context without reaching any engine.
func TestTranslateWrapperRetranslateWithContextNeedsAContext(t *testing.T) {
	w := NewTranslateWrapper(translate.NewService(engine.NewRegistry(), nil, nil, nil))
	got := w.RetranslateWithContext(model.ContextTranslateRequest{Text: "hola amigo mío", From: model.ESMX, To: model.EN, Engine: "apple"})
	if got.Status != model.ContextTranslateNoContext {
		t.Fatalf("got %+v, want no_context", got)
	}
}
