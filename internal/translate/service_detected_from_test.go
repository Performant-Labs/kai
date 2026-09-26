package translate

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/model"
)

// newLoopbackService wires a real translate.Service to the real google engine talking to a
// loopback httptest server (real HTTP, no transport interception). detected is the raw JSON
// value of the gtx detected-language slot (e.g. `"es"` or `null`).
func newLoopbackService(t *testing.T, detected string) *Service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[[["Hola","Hello",null,null,1]],null,` + detected + `]`))
	}))
	t.Cleanup(srv.Close)
	reg := engine.NewRegistry()
	reg.RegisterTranslator(engine.NewGoogle(srv.URL, srv.Client()))
	return NewService(reg, nil, nil, nil)
}

// #53 scope addition: with the source on auto, the engine-detected language must reach the
// result (today translateWithEngine discards it and returns the request value "auto").
func TestTranslateCarriesDetectedFromWhenSourceAuto(t *testing.T) {
	svc := newLoopbackService(t, `"es"`)
	res, err := svc.Translate(model.TranslateRequest{Text: "Hello", From: model.Auto, To: model.EN, EngineName: "google"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if res.From != model.ES {
		t.Errorf("From = %q, want es (engine-detected, carried through the service)", res.From)
	}
}

// Fallback: the engine reports no detected language, so the request value is kept.
func TestTranslateKeepsAutoWhenEngineReportsNoDetection(t *testing.T) {
	svc := newLoopbackService(t, `null`)
	res, err := svc.Translate(model.TranslateRequest{Text: "Hello", From: model.Auto, To: model.EN, EngineName: "google"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if res.From != model.Auto {
		t.Errorf("From = %q, want auto (fallback to the request value)", res.From)
	}
}

// An explicit source is never replaced by whatever the engine reports.
func TestTranslateKeepsExplicitFrom(t *testing.T) {
	svc := newLoopbackService(t, `"es"`)
	res, err := svc.Translate(model.TranslateRequest{Text: "Hello", From: model.FR, To: model.EN, EngineName: "google"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if res.From != model.FR {
		t.Errorf("From = %q, want fr (explicit source untouched)", res.From)
	}
}
