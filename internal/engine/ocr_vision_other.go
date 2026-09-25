//go:build !darwin

package engine

import (
	"context"

	"cnb.cool/dtapp/kai/internal/model"
)

// VisionOCR is a placeholder type for non-macOS platforms. System OCR (Vision.framework) is
// macOS-only; other platforms should use tesseract (NewTesseractOCR, which needs a local
// tesseract dependency).
type VisionOCR struct{}

// NewVisionOCR constructs the system OCR engine. Only the darwin platform has the real
// implementation (taking cfg to read the OCR params); this placeholder lets engine_wrapper.go
// compile on Windows/Linux (signature kept consistent with the darwin version). cfg is unused
// on this platform (VisionOCR is never registered on Windows/Linux).
func NewVisionOCR(cfg *EngineConfig) *VisionOCR {
	return nil
}

// Name returns the engine name.
func (v *VisionOCR) Name() string { return "vision" }

// Recognize is the placeholder implementation for non-macOS platforms. Vision.framework is
// macOS-only, and engine_wrapper.go only registers VisionOCR on darwin, so this method is
// never called on Windows/Linux. Returns an empty result rather than an error, purely to
// satisfy the OcrEngine interface signature.
func (v *VisionOCR) Recognize(_ context.Context, _ model.OcrRequest) (*model.OcrResult, error) {
	return &model.OcrResult{}, nil
}
