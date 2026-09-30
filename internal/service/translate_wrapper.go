package service

import (
	"log/slog"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/translate"
)

// TranslateWrapper is the thin adapter over translation / OCR: holds translate.Service,
// doing only RPC passthrough.
// Does not implement the wails lifecycle trio (startup orchestration is AppService's job).
type TranslateWrapper struct {
	svc *translate.Service
}

// NewTranslateWrapper constructs the translate Wrapper.
func NewTranslateWrapper(svc *translate.Service) *TranslateWrapper {
	return &TranslateWrapper{svc: svc}
}

func (w *TranslateWrapper) Translate(req model.TranslateRequest) (*model.TranslateResult, error) {
	return w.svc.Translate(req)
}

func (w *TranslateWrapper) TranslateMulti(req model.TranslateRequest) (*model.TranslateMultiResult, error) {
	return w.svc.TranslateMulti(req)
}

// PlanSourceSwitch decides whether text that just arrived changes the language pair (issue #200):
// the source follows the text and the old source replaces the target. Every way text arrives asks
// through this one call; the answer is applied, never learned or persisted, by the window.
func (w *TranslateWrapper) PlanSourceSwitch(req model.SourceSwitchRequest) model.SourceSwitch {
	return w.svc.PlanSourceSwitch(req)
}

// ReportSourceSwitchSkipped writes the window's own reason for not switching (the frontend drops a
// plan when the text or the pair changed while it was computed, or the language is not offered) to
// the main log, next to the backend's decision line, so one file tells the whole story (issue #16).
// The frontend's own log file was empty in practice, and the reason is a fixed word, never text.
func (w *TranslateWrapper) ReportSourceSwitchSkipped(reason string) {
	slog.Info(i18n.T("log.source_switch_skipped"), "reason", sanitizeSkipReason(reason))
}

// sanitizeSkipReason keeps only a short snake_case word, so nothing the user selected can reach the
// log through this call, whatever the caller sends.
func sanitizeSkipReason(reason string) string {
	if len(reason) == 0 || len(reason) > 40 {
		return "invalid"
	}
	for _, r := range reason {
		if (r < 'a' || r > 'z') && r != '_' {
			return "invalid"
		}
	}
	return reason
}

// CorrectSource corrects the grammar and word choice of text that just arrived, when the "correct
// grammar and wording" setting is on and Apple's on-device model can run (issue #208). Every way
// text arrives asks through this one call, right before PlanSourceSwitch, and translates the answer's
// text; the answer never persists or teaches anything.
func (w *TranslateWrapper) CorrectSource(req model.CorrectionRequest) model.Correction {
	return w.svc.CorrectSource(req)
}

// CorrectionAvailability reports whether the correction can run on this Mac, and why not when it
// cannot, so the toolbar checkbox can say so (issue #208).
func (w *TranslateWrapper) CorrectionAvailability() model.CorrectionAvailability {
	return w.svc.CorrectionAvailability()
}

// CancelTranslate cancels the running translation request requestID (issue #109), or only its
// engine when engine is not empty; the other engines keep running. It reports whether it found
// something still running, and returns false, without any error, for an unknown or finished
// request or engine.
func (w *TranslateWrapper) CancelTranslate(requestID, engine string) bool {
	return w.svc.CancelTranslate(requestID, engine)
}

func (w *TranslateWrapper) Ocr(req model.OcrRequest) (*model.OcrResult, error) {
	return w.svc.Ocr(req)
}

func (w *TranslateWrapper) ScreenshotOCR(engineName string) (*model.OcrResult, error) {
	return w.svc.ScreenshotOCR(engineName)
}

// ScreenshotTranslate is the main screenshot-translate flow: region screenshot→system
// OCR→multi-engine translation→delivered to the screenshot window.
// session identifies the cache origin (events.ScreenshotSessionScreenshot /
// ScreenshotSessionInput).
func (w *TranslateWrapper) ScreenshotTranslate(session string) (*model.ScreenshotResult, error) {
	return w.svc.ScreenshotTranslate(session)
}
