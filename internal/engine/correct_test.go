package engine

import (
	"errors"
	"net/http"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #208: the correction is a separate provider interface. Untagged, pure parsing so it is
// testable on any OS; the darwin provider itself is covered by correct_apple_darwin_test.go.

func TestParseCorrection(t *testing.T) {
	got, err := parseCorrection([]byte(`{"text":"Necesito hacer el seguimiento."}` + "\x00"))
	if err != nil || got != "Necesito hacer el seguimiento." {
		t.Fatalf("got (%q, %v)", got, err)
	}
	for name, raw := range map[string]string{
		"refused":        `{"code":"correct_refused","detail":"guardrail"}`,
		"failed":         `{"code":"correct_failed","detail":"x"}`,
		"timeout":        `{"code":"correct_timeout","detail":""}`,
		"empty text":     `{"text":""}`,
		"malformed":      `not json`,
		"empty payload":  `{}`,
		"unknown code":   `{"code":"whatever","detail":"y"}`,
		"blank text":     `{"text":"   "}`,
		"null text":      `{"text":null}`,
		"error and text": `{"text":"a b c d e f g h","code":"correct_failed","detail":""}`,
	} {
		if got, err := parseCorrection([]byte(raw)); err == nil {
			t.Errorf("%s: got (%q, nil), want an error", name, got)
		}
	}
	if _, err := parseCorrection([]byte(`{"code":"correct_refused","detail":"g"}`)); !errors.Is(err, ErrCorrectionRefused) {
		t.Errorf("a refusal is not ErrCorrectionRefused: %v", err)
	}
}

func TestParseCorrectionAvailability(t *testing.T) {
	cases := map[string]CorrectionStatus{
		`{"status":"available"}`:                 CorrectionAvailable,
		`{"status":"model_not_ready"}`:           CorrectionModelNotReady,
		`{"status":"apple_intelligence_off"}`:    CorrectionAppleIntelligenceOff,
		`{"status":"unsupported_hardware"}`:      CorrectionUnsupportedHardware,
		`{"status":"unsupported_language"}`:      CorrectionUnsupportedLanguage,
		`{"status":"something new"}`:             CorrectionUnavailable,
		`{}`:                                     CorrectionUnavailable,
		`garbage`:                                CorrectionUnavailable,
		`{"code":"correct_failed","detail":"x"}`: CorrectionUnavailable,
		`{"status":"available"}` + "\x00":        CorrectionAvailable,
	}
	for raw, want := range cases {
		if got := parseCorrectionAvailability([]byte(raw)); got != want {
			t.Errorf("%q -> %q, want %q", raw, got, want)
		}
	}
}

func TestOnlyAvailableIsAvailable(t *testing.T) {
	if !CorrectionAvailable.IsAvailable() {
		t.Fatal("available is not available")
	}
	for _, s := range []CorrectionStatus{"", CorrectionModelNotReady, CorrectionAppleIntelligenceOff,
		CorrectionUnsupportedHardware, CorrectionUnsupportedLanguage, CorrectionUnsupportedPlatform, CorrectionUnavailable} {
		if s.IsAvailable() {
			t.Errorf("%q counts as available", s)
		}
	}
}

// The classical engines (and the LLM engines used as translators) cannot correct text: none of
// them may implement Corrector, so nothing can route a correction to one by accident.
func TestNoTranslatorImplementsCorrector(t *testing.T) {
	translators := []Translator{
		NewApple(),
		NewGoogle("", http.DefaultClient),
		NewDeepL(&EngineConfig{}, http.DefaultClient),
		NewBaidu(&EngineConfig{}, http.DefaultClient),
		NewTencent(&EngineConfig{}, http.DefaultClient),
		NewYoudao(&EngineConfig{}, http.DefaultClient),
		NewOpenAI(&EngineConfig{}, http.DefaultClient),
		NewAnthropic(&EngineConfig{}),
	}
	for _, tr := range translators {
		if _, ok := tr.(Corrector); ok {
			t.Errorf("translation engine %q implements Corrector", tr.Name())
		}
	}
}

// Off macOS (and with no Swift bridge) the Apple provider reports unavailable and never crashes.
func TestAppleCorrectorUnavailableWithoutBridge(t *testing.T) {
	c := newAppleCorrector(func() bool { return false })
	if st := c.Availability(model.ESMX); st.IsAvailable() {
		t.Fatalf("no bridge, availability = %q", st)
	}
	if _, err := c.Correct(t.Context(), CorrectRequest{Instructions: "x", Text: "hola mundo amigo"}); err == nil {
		t.Fatal("Correct without a bridge returned no error")
	}
}
