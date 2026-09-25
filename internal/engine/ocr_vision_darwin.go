//go:build darwin

package engine

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// VisionOCR calls macOS's system Vision.framework for offline OCR (zero-install, no local
// tesseract needed).
// It loads the Swift bridge dynamic library at runtime via purego (pkg/swiftbridge). After
// changing Swift code, just rebuild internal/swift/build.sh (produces the .dylib and copies
// it into pkg/swiftbridge); the runtime Dlopen then loads the latest code, no relinking.
type VisionOCR struct {
	name   string
	config *EngineConfig // Holds the owning engine config; reads OCR-specific params from Extra(JSON)
}

// NewVisionOCR constructs the system OCR engine. cfg is the vision engine's EngineConfig
// (containing the OCR params in Extra).
func NewVisionOCR(cfg *EngineConfig) *VisionOCR {
	return &VisionOCR{name: "vision", config: cfg}
}

// Name returns the engine name.
func (v *VisionOCR) Name() string { return v.name }

// ocrOptions resolves the effective correct / timeout / retry from the current config and
// this request.
// Priority: explicit req override > engine Extra config > built-in defaults (true / 60s / 2).
// Extra(JSON) is parsed with the unified parseOCRExtra, keeping the extra format consistent
// with tesseract.
// retry is the "extra retries count (excluding the first attempt)": Extra explicitly 0 =>
// retries disabled (first attempt only); Extra missing the field (nil) => falls back to the
// default 2; req.RetryCount>0 overrides explicitly.
func (v *VisionOCR) ocrOptions(req model.OcrRequest) (correct bool, timeoutSec int, retryCount int) {
	correct = true
	timeoutSec = DefaultOCRTimeoutSec
	retryCount = DefaultOCRRetryCount
	e := parseOCRExtra(v.name, optExtra(v.config))
	if e.TimeoutSec > 0 {
		timeoutSec = e.TimeoutSec
	}
	if e.RetryCount != nil {
		retryCount = *e.RetryCount // 0 allowed (retries disabled)
	}
	if e.Correct != nil {
		correct = *e.Correct
	}
	if req.CorrectText != nil {
		correct = *req.CorrectText
	}
	if req.TimeoutSec > 0 {
		timeoutSec = req.TimeoutSec
	}
	if req.RetryCount > 0 {
		retryCount = req.RetryCount
	}
	return
}

// Recognize runs Vision OCR on the image bytes.
func (v *VisionOCR) Recognize(ctx context.Context, req model.OcrRequest) (*model.OcrResult, error) {
	if len(req.ImageData) == 0 {
		return nil, ErrEmptyImage
	}

	correct, timeoutSec, retryCount := v.ocrOptions(req)

	b64 := base64.StdEncoding.EncodeToString(req.ImageData)

	// Degrade safely when the dylib isn't loaded (non-macOS / missing / wrong path), avoiding
	// a nil function-pointer panic.
	if !swiftbridge.Available() {
		return nil, fmt.Errorf(i18n.T("err.swiftbridge_unavailable"))
	}
	outBuf := make([]byte, 1<<20) // 1MB output buffer, enough for region details of large-image OCR
	// Call the Swift bridge: unsafe.Pointer is required for the C/Swift interop.
	n := swiftbridge.KaiOCR(b64, unsafe.Pointer(&outBuf[0]), int32(len(outBuf)), boolToInt32(correct), int32(timeoutSec), int32(retryCount)) //nolint:gosec
	slog.Debug(i18n.T("log.vision_ocr_call"), "n", int(n), "out_cap", len(outBuf), "correct", correct, "timeout", timeoutSec, "retry", retryCount)
	if n < 0 {
		slog.Error(i18n.T("err.vision_ocr_buffer"), "n", int(n))
		return nil, fmt.Errorf(i18n.T("err.vision_ocr_buffer"))
	}

	payload := bytes.TrimRight(outBuf[:n], "\x00")
	var resp swiftbridge.OCRSuccess
	if err := json.Unmarshal(payload, &resp); err != nil {
		slog.Error(i18n.T("err.vision_ocr_parse"), "raw", string(payload), "error", err)
		return nil, fmt.Errorf("%s: %w", i18n.T("err.vision_ocr_parse"), err)
	}
	if resp.Code != "" {
		// Swift custom error: render user-visible copy via Go-side i18n by error code, with
		// detail as the technical context.
		// Known codes map to err.apple_<code>; unknown codes fall back to the generic OCR
		// engine error copy.
		var msg string
		switch resp.Code {
		case swiftbridge.BridgeErrNullImage:
			msg = i18n.T("err.apple_null_image")
		case swiftbridge.BridgeErrDecodeFailed:
			msg = i18n.T("err.apple_decode_failed")
		case swiftbridge.BridgeErrEmptyImage:
			msg = i18n.T("err.apple_empty_image")
		case swiftbridge.BridgeErrBitmapCtxFailed:
			msg = i18n.T("err.apple_bitmap_ctx_failed")
		case swiftbridge.BridgeErrBitmapRedrawFailed:
			msg = i18n.T("err.apple_bitmap_redraw_failed")
		case swiftbridge.BridgeErrOcrTimeout:
			msg = i18n.T("err.apple_ocr_timeout")
		case swiftbridge.BridgeErrAppleOcr:
			slog.Error(i18n.T("err.vision_ocr_engine"), "detail", resp.Detail)
			msg = i18n.T("err.vision_ocr_engine")
		default:
			slog.Error(i18n.T("err.vision_ocr_engine"), "code", resp.Code, "detail", resp.Detail)
			msg = i18n.T("err.vision_ocr_engine")
		}
		if resp.Detail != "" {
			msg = msg + " (" + resp.Detail + ")"
		}
		return nil, fmt.Errorf("%s", msg)
	}

	regions := make([]model.OcrRegion, 0, len(resp.Regions))
	for _, r := range resp.Regions {
		// Swift returns box as [x, y, w, h]; the model's OcrRegion.Box is [x1,y1,x2,y2].
		box := r.Box
		if len(box) == 4 {
			box = []int{box[0], box[1], box[0] + box[2], box[1] + box[3]}
		}
		regions = append(regions, model.OcrRegion{Text: r.Text, Conf: r.Conf, Box: box})
	}

	slog.Debug(i18n.T("log.vision_ocr_done"), "text_len", len(resp.Text), "regions", len(regions), "correct", correct, "timeout_sec", timeoutSec)
	return &model.OcrResult{
		Engine:  v.name,
		Text:    resp.Text,
		Regions: regions,
	}, nil
}

// boolToInt32 converts a Go bool to a C int (1/0) for purego's int32 parameters.
func boolToInt32(b bool) int32 {
	if b {
		return 1
	}
	return 0
}
