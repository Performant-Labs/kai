package engine

import (
	"errors"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/i18n"
)

// Issue #96 AC6 (E8): the bridge-code mapping is an untagged pure function so it is testable on
// any OS. no_source_lang wraps ErrUnsupportedPair (a pair problem the bridge already identifies);
// apple_translate keeps its detail in the text (the classifier's text fallback catches "Unable to
// Translate"); other codes are not pair errors.
func TestAppleBridgeError(t *testing.T) {
	t.Run("no_source_lang wraps ErrUnsupportedPair and keeps text", func(t *testing.T) {
		err := appleBridgeError("no_source_lang", "detail-x")
		if err == nil || !errors.Is(err, ErrUnsupportedPair) {
			t.Fatalf("errors.Is(ErrUnsupportedPair) = false: %v", err)
		}
		if !strings.Contains(err.Error(), i18n.T("err.apple_no_source_lang")) || !strings.Contains(err.Error(), "detail-x") {
			t.Errorf("text = %q, want the localized copy plus the detail", err.Error())
		}
	})
	t.Run("apple_translate keeps detail, is not a pair sentinel", func(t *testing.T) {
		err := appleBridgeError("apple_translate", "Unable to Translate")
		if err == nil || errors.Is(err, ErrUnsupportedPair) {
			t.Fatalf("err = %v, want non-nil and not ErrUnsupportedPair", err)
		}
		if !strings.Contains(err.Error(), "Unable to Translate") || !strings.Contains(err.Error(), i18n.T("err.apple_translate_engine")) {
			t.Errorf("text = %q", err.Error())
		}
	})
	for _, code := range []string{"empty_text", "target_required", "some_unknown_code"} {
		t.Run(code+" is not a pair error", func(t *testing.T) {
			err := appleBridgeError(code, "")
			if err == nil || errors.Is(err, ErrUnsupportedPair) || errors.Is(err, ErrAPIKey) {
				t.Errorf("err = %v, want a plain non-nil error", err)
			}
			if strings.Contains(err.Error(), "%!(") {
				t.Errorf("format artifact in %q", err.Error())
			}
		})
	}
}
