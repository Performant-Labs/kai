package translate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/model"
)

// failingEngine is a fake translator that always fails with a fixed error.
type failingEngine struct {
	name string
	err  error
}

func (f failingEngine) Name() string { return f.name }
func (f failingEngine) Translate(context.Context, model.TranslateRequest) (*model.TranslateResult, error) {
	return nil, f.err
}

// Issue #96 AC3: both fan-outs build the failure entry through failurePayload, which carries the
// sanitized reason and its category. failurePayload leaves From empty (the translate window's
// failure payload never claims a detected source language).
func TestFailurePayloadCarriesReason(t *testing.T) {
	req := model.TranslateRequest{Text: "Hello", From: model.EN, To: model.ZH}
	err := fmt.Errorf("x(google): %w", &engine.HTTPError{Status: 429, Message: "slow down https://h.example/p?key=SECRET123"})
	p := failurePayload("google", req, err)
	if p.Engine != "google" || p.To != model.ZH || p.Text != "Hello" {
		t.Errorf("payload identity fields = %+v", p)
	}
	if p.Result != "" {
		t.Errorf("Result = %q, want empty on failure", p.Result)
	}
	if p.From != "" {
		t.Errorf("From = %q, want empty (no detected-language claim)", p.From)
	}
	if p.ErrorKind != "rate_limit" {
		t.Errorf("ErrorKind = %q, want rate_limit", p.ErrorKind)
	}
	if p.Error == "" {
		t.Fatal("Error is empty")
	}
	if strings.Contains(p.Error, "SECRET123") || strings.Contains(p.Error, "key=") {
		t.Errorf("Error was not sanitized: %q", p.Error)
	}
}

// Issue #96 AC3 (E5): the screenshot fan-out's failure entry carries Error/ErrorKind, and keeps
// From == req.From so TranslateCard's header is unchanged.
func TestTranslateAllStreamFailureEntryHasReason(t *testing.T) {
	reg := engine.NewRegistry()
	reg.RegisterTranslator(failingEngine{name: "fake", err: fmt.Errorf("boom: %w", &engine.HTTPError{Status: 503})})
	svc := NewService(reg, nil, nil, nil)
	req := model.TranslateRequest{Text: "Hello", From: model.EN, To: model.ZH}
	out := svc.translateAllStream(req, "", model.ZH)
	if len(out) != 1 {
		t.Fatalf("entries = %d, want 1", len(out))
	}
	e := out[0]
	if e.Error == "" || e.ErrorKind == "" {
		t.Errorf("entry Error=%q ErrorKind=%q, want both non-empty", e.Error, e.ErrorKind)
	}
	if e.ErrorKind != "unavailable" {
		t.Errorf("ErrorKind = %q, want unavailable", e.ErrorKind)
	}
	if e.Result != "" {
		t.Errorf("Result = %q, want empty placeholder", e.Result)
	}
	if e.From != req.From {
		t.Errorf("From = %q, want %q (card header unchanged)", e.From, req.From)
	}
}

// A plain text-only failure still gets a payload (kind falls back to engine).
func TestTranslateAllStreamTextOnlyFailure(t *testing.T) {
	reg := engine.NewRegistry()
	reg.RegisterTranslator(failingEngine{name: "fake", err: errors.New("some unexpected engine failure")})
	svc := NewService(reg, nil, nil, nil)
	out := svc.translateAllStream(model.TranslateRequest{Text: "Hi", From: model.EN, To: model.ZH}, "", model.ZH)
	if len(out) != 1 || out[0].ErrorKind != "engine" || !strings.Contains(out[0].Error, "some unexpected engine failure") {
		t.Errorf("out = %+v", out)
	}
}

// Issue #96 (A finding 4): a not-configured stub is not an enabled engine.
func TestEnabledTranslatorNamesSkipsNotConfiguredStub(t *testing.T) {
	reg := engine.NewRegistry()
	reg.RegisterTranslator(failingEngine{name: "real", err: errors.New("x")})
	reg.RegisterTranslator(engine.NewNotConfigured("stub", engine.ErrAPIKey))
	svc := NewService(reg, nil, nil, nil)
	got := svc.enabledTranslatorNames()
	if len(got) != 1 || got[0] != "real" {
		t.Errorf("enabledTranslatorNames = %v, want [real]", got)
	}
}
