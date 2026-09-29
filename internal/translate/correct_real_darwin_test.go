//go:build darwin

package translate

import (
	"context"
	"testing"
	"time"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/langpref"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/settings"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// Issue #208: the REAL Apple Foundation Models provider, through the real Swift bridge, the real
// instructions and the real service. It skips (never fails) where the model cannot run: no bridge
// library, Apple Intelligence off, an unsupported Mac, or a language the model does not support.
// It records what the model said, and asserts no quality: whether the Spanish is good is for the
// owner to judge from the log (go test -run RealFoundationModels -v).
func TestRealFoundationModelsCorrection(t *testing.T) {
	if err := swiftbridge.Init(""); err != nil || !swiftbridge.Available() {
		t.Skipf("the Swift bridge is not loaded: %v", err)
	}
	provider := engine.NewAppleCorrector()
	if st := provider.Availability(""); !st.IsAvailable() {
		t.Skipf("Foundation Models are unavailable on this Mac: %s", st)
	}
	if st := provider.Availability(model.ESMX); !st.IsAvailable() {
		t.Skipf("Foundation Models do not support es-MX here: %s", st)
	}

	st, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st.Get().CorrectSourceText = true
	svc := NewService(engine.NewRegistry(), nil, st, nil)
	svc.SetLangPrefs(langpref.New())

	sentences := []struct{ name, text string }{
		{"already correct", "Necesito hablar con el gerente mañana temprano para revisar el contrato."},
		{"grammar and tense", "Ayer nosotros vamos a la tienda y compramos muchas cosas que no necesitabamos."},
		{"spanglish", "Necesito hacer el follow up con el cliente antes del deadline"},
	}
	for _, s := range sentences {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		start := time.Now()
		raw, rawErr := provider.Correct(ctx, engine.CorrectRequest{Instructions: CorrectionInstructions(model.ESMX), Text: s.text})
		rawMs := time.Since(start).Milliseconds()
		cancel()
		t.Logf("[%s] input:  %q", s.name, s.text)
		t.Logf("[%s] raw model output (%d ms, err=%v): %q", s.name, rawMs, rawErr, raw)

		start = time.Now()
		got := svc.CorrectSource(model.CorrectionRequest{Text: s.text, From: model.ESMX})
		t.Logf("[%s] CorrectSource (%d ms): status=%s corrected=%v language=%s reason=%q text=%q changes=%+v",
			s.name, time.Since(start).Milliseconds(), got.Status, got.Corrected, got.Language, got.Reason, got.Text, got.Changes)

		// Whatever the model said, the service never returns a broken answer.
		if got.Text == "" || got.Original != s.text {
			t.Errorf("[%s] broken answer: %+v", s.name, got)
		}
		if !got.Corrected && got.Text != s.text {
			t.Errorf("[%s] not corrected, yet the text changed: %q", s.name, got.Text)
		}
	}
}
