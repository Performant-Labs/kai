package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"

	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// Text correction providers (issue #208). Correcting the grammar and word choice of a text, in its
// own language, is a language-model job: none of the classical translation engines can do it, so
// the operation is its own interface, Corrector, and no Translator implements it (a test pins
// that). The first and only provider is Apple's on-device Foundation Models, through the Swift
// bridge; later providers (issue #208, phase 2) implement the same interface.
//
// The provider knows nothing about prompts or languages of the app: the translate service builds
// the instructions (translate.CorrectionInstructions, the one place the wording lives) and the
// provider only runs them and returns the text. A provider never blocks the main thread (it is
// called from a service goroutine) and never crashes: anything that goes wrong is an error or an
// availability status the UI can explain.

// CorrectionStatus is what a provider reports about itself: available, or the reason it is not.
// The values are the strings the UI words (translate.correctUnavailable* keys), so a new reason
// needs a new sentence there.
type CorrectionStatus string

const (
	CorrectionAvailable            CorrectionStatus = "available"
	CorrectionModelNotReady        CorrectionStatus = "model_not_ready"        // the model is still downloading or preparing
	CorrectionAppleIntelligenceOff CorrectionStatus = "apple_intelligence_off" // the user has Apple Intelligence off
	CorrectionUnsupportedHardware  CorrectionStatus = "unsupported_hardware"   // this Mac cannot run the model
	CorrectionUnsupportedLanguage  CorrectionStatus = "unsupported_language"   // the model does not support the text's language
	CorrectionUnsupportedPlatform  CorrectionStatus = "unsupported_platform"   // not macOS
	CorrectionUnavailable          CorrectionStatus = "unavailable"            // bridge missing, or a status this build does not know
)

// IsAvailable reports whether a provider with this status can correct text right now.
func (s CorrectionStatus) IsAvailable() bool { return s == CorrectionAvailable }

// CorrectRequest is one correction call: the instructions (the system prompt, built by the
// translate service) and the text to correct.
type CorrectRequest struct {
	Instructions string
	Text         string
}

// Corrector is a provider that can correct a text. Availability reports whether it can run for
// lang (empty: for any language, i.e. the model alone); Correct returns the corrected text, or
// ErrCorrectionRefused when the model declined, or any other error when the call failed. The
// caller guards the output; a provider returns what the model said.
type Corrector interface {
	Name() string
	Availability(lang model.Language) CorrectionStatus
	Correct(ctx context.Context, req CorrectRequest) (string, error)
}

// ErrCorrectionRefused means the model declined to correct the text (a safety guardrail, or an
// explicit refusal). The caller treats it as "no correction".
var ErrCorrectionRefused = errors.New("the model declined to correct the text")

var errCorrectionFailed = errors.New("the correction failed")

// correctionPayload mirrors the bridge's kai_correct answer: {"text":"..."}, or the bridge error
// shape {"code":"...","detail":"..."}.
type correctionPayload struct {
	Text *string `json:"text"`
	swiftbridge.BridgeError
}

// parseCorrection reads the bridge's answer to a correction. Only a payload with non-blank text
// and no error code is a correction; a refusal is ErrCorrectionRefused, anything else an error.
func parseCorrection(raw []byte) (string, error) {
	raw = bytes.TrimRight(raw, "\x00")
	var p correctionPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", errCorrectionFailed
	}
	switch p.Code {
	case "":
	case swiftbridge.BridgeErrCorrectRefused:
		return "", ErrCorrectionRefused
	default:
		return "", errCorrectionFailed
	}
	if p.Text == nil || strings.TrimSpace(*p.Text) == "" {
		return "", errCorrectionFailed
	}
	return *p.Text, nil
}

// parseCorrectionAvailability reads the bridge's availability answer: {"status":"..."}. A payload
// that is not a known status is CorrectionUnavailable, never available.
func parseCorrectionAvailability(raw []byte) CorrectionStatus {
	raw = bytes.TrimRight(raw, "\x00")
	var p struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return CorrectionUnavailable
	}
	switch s := CorrectionStatus(p.Status); s {
	case CorrectionAvailable, CorrectionModelNotReady, CorrectionAppleIntelligenceOff,
		CorrectionUnsupportedHardware, CorrectionUnsupportedLanguage:
		return s
	}
	return CorrectionUnavailable
}

// appleCorrector is the Apple Foundation Models provider. The bridge calls themselves are per-OS
// (bridgeAvailability / bridgeCorrect in correct_apple_darwin.go and correct_apple_other.go); this
// type is the same everywhere, so its decisions are testable on any OS.
type appleCorrector struct {
	up func() bool
}

func newAppleCorrector(bridgeUp func() bool) Corrector { return &appleCorrector{up: bridgeUp} }

// NewAppleCorrector returns the Apple on-device model provider. Off macOS, or with the Swift bridge
// not loaded, it reports unavailable.
func NewAppleCorrector() Corrector { return newAppleCorrector(bridgeCorrectUp) }

func (c *appleCorrector) Name() string { return "apple-foundation-models" }

func (c *appleCorrector) Availability(lang model.Language) CorrectionStatus {
	if runtime.GOOS != "darwin" {
		return CorrectionUnsupportedPlatform
	}
	if !c.up() {
		return CorrectionUnavailable
	}
	return parseCorrectionAvailability(bridgeCorrectAvailability(string(lang)))
}

func (c *appleCorrector) Correct(ctx context.Context, req CorrectRequest) (string, error) {
	if !c.up() {
		return "", errCorrectionFailed
	}
	if err := ctx.Err(); err != nil {
		return "", context.Cause(ctx)
	}
	raw, err := bridgeCorrect(ctx, req)
	if err != nil {
		return "", err
	}
	return parseCorrection(raw)
}
