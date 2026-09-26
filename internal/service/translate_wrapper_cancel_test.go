package service

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/translate"
)

// Issue #109 (criterion 4): the binding the frontend calls exists on TranslateWrapper and reports
// false, without error or panic, for an unknown request or engine.
func TestTranslateWrapperCancelTranslateUnknownIsFalse(t *testing.T) {
	w := NewTranslateWrapper(translate.NewService(engine.NewRegistry(), nil, nil, nil))
	if w.CancelTranslate("no-such-request", "") {
		t.Error("CancelTranslate(unknown, \"\") = true, want false")
	}
	if w.CancelTranslate("no-such-request", "google") {
		t.Error("CancelTranslate(unknown, google) = true, want false")
	}
}
