package service

import (
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
