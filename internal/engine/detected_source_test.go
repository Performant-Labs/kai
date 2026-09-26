package engine

import (
	"errors"
	"fmt"
	"testing"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #80: an engine that fails but knows the detected source attaches it to the error.
func TestDetectedSourceOfFindsThroughWraps(t *testing.T) {
	base := errors.New("Unable to Translate")
	err := WithDetectedSource(base, model.ZH)
	wrapped := fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", err))
	got, ok := DetectedSourceOf(wrapped)
	if !ok || got != model.ZH {
		t.Fatalf("DetectedSourceOf = (%q, %v), want (zh, true)", got, ok)
	}
	if !errors.Is(wrapped, base) {
		t.Error("the original error is no longer reachable with errors.Is (Unwrap missing)")
	}
}

func TestDetectedSourceOfPlainAndNil(t *testing.T) {
	if l, ok := DetectedSourceOf(errors.New("plain")); ok {
		t.Errorf("plain error reported detection %q", l)
	}
	if l, ok := DetectedSourceOf(nil); ok {
		t.Errorf("nil error reported detection %q", l)
	}
	if err := WithDetectedSource(nil, model.EN); err != nil {
		t.Errorf("WithDetectedSource(nil) = %v, want nil", err)
	}
}

func TestWithDetectedSourceKeepsMessageByteForByte(t *testing.T) {
	base := errors.New("Apple engine error (Unable to Translate)")
	if got := WithDetectedSource(base, model.ES).Error(); got != base.Error() {
		t.Errorf("Error() = %q, want %q", got, base.Error())
	}
}

// The Go half of the Apple bridge change. appleBridgeError is the untagged mapping of a bridge
// error (code, detail, from) to a Go error; it must run on any OS.
func TestAppleBridgeErrorCarriesDetection(t *testing.T) {
	err := appleBridgeError("apple_translate", "Unable to Translate", "zh")
	if err == nil {
		t.Fatal("appleBridgeError returned nil")
	}
	want := i18n.T("err.apple_translate_engine") + " (Unable to Translate)"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q (text unchanged)", err.Error(), want)
	}
	if l, ok := DetectedSourceOf(err); !ok || l != model.ZH {
		t.Errorf("DetectedSourceOf = (%q, %v), want (zh, true)", l, ok)
	}
}

func TestAppleBridgeErrorNoSourceLangCarriesDetection(t *testing.T) {
	err := appleBridgeError("no_source_lang", "", "es")
	if l, ok := DetectedSourceOf(err); !ok || l != model.ES {
		t.Errorf("DetectedSourceOf = (%q, %v), want (es, true)", l, ok)
	}
}

func TestAppleBridgeErrorWithoutFromHasNoDetection(t *testing.T) {
	err := appleBridgeError("apple_translate", "Unable to Translate", "")
	if err == nil {
		t.Fatal("appleBridgeError returned nil")
	}
	if l, ok := DetectedSourceOf(err); ok {
		t.Errorf("explicit-source failure (no from) reported detection %q", l)
	}
}
