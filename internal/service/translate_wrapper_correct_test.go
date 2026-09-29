package service

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/translate"
)

// Issue #208: the two bindings the frontend calls exist on TranslateWrapper, and with the setting
// off (the default, and here no settings service at all) a correction changes nothing.
func TestTranslateWrapperCorrectSourceOffReturnsTheTextAsItCame(t *testing.T) {
	w := NewTranslateWrapper(translate.NewService(engine.NewRegistry(), nil, nil, nil))
	text := "Ellos no sabe donde esta la biblioteca"
	got := w.CorrectSource(model.CorrectionRequest{Text: text, From: model.ESMX})
	if got.Corrected || got.Text != text || got.Status != model.CorrectionOff {
		t.Fatalf("got %+v, want the text as it came with status off", got)
	}
	av := w.CorrectionAvailability()
	if av.Available && av.Reason != "" {
		t.Fatalf("available with a reason: %+v", av)
	}
	if !av.Available && av.Reason == "" {
		t.Fatalf("unavailable without a reason: %+v", av)
	}
}
