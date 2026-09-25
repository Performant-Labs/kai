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
