package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

var (
	// ErrEmptyImage means the image data is empty.
	ErrEmptyImage = errors.New(i18n.T("err.ocr_empty_image"))
	// ErrNoScreenshot means the current platform doesn't support system screenshots.
	ErrNoScreenshot = errors.New(i18n.T("err.ocr_screenshot_unsupported"))
	// ErrTesseractNotFound means the tesseract binary wasn't found on this machine.
	ErrTesseractNotFound = errors.New(i18n.T("err.ocr_tesseract_not_found"))
)

// tesseractCandidates: GUI apps (e.g. the packaged Kai.app) launched from launchd don't
// inherit the shell's PATH, so common install locations are probed as well
// (Homebrew Apple Silicon / Intel).
var tesseractCandidates = []string{
	"/opt/homebrew/bin/tesseract",
	"/usr/local/bin/tesseract",
	"/usr/bin/tesseract",
}

// resolveTesseract returns a usable tesseract executable path; an empty string when not found.
func resolveTesseract() string {
	if p, err := exec.LookPath("tesseract"); err == nil {
		return p
	}
	for _, c := range tesseractCandidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// TesseractOCR is a local OCR engine based on the system tesseract command (pure Go exec,
// no CGO). Requires tesseract installed on the machine (mac: brew install tesseract;
// linux: apt install tesseract-ocr).
// It holds its engine config; OCR-specific params (langs / timeout) are read uniformly from
// Extra(JSON), parsed with the same parseOCRExtra as vision to keep the extra format
// consistent.
type TesseractOCR struct {
	name   string
	config *EngineConfig // Holds the owning engine config; reads langs / timeout from Extra(JSON)
	bin    string        // tesseract executable path (user-specified or auto-detected)
}

// TesseractStatus describes the local tesseract install probe result, for the frontend to
// show install state per OS.
type TesseractStatus struct {
	Installed bool   `json:"installed"` // Whether a tesseract executable was found
	Path      string `json:"path"`      // The detected executable path (empty when not installed)
	Version   string `json:"version"`   // The detected version (empty when not installed), from `tesseract --version`
	OS        string `json:"os"`        // Current OS (darwin/linux/windows), lets the frontend pick the matching install command
}

// TesseractInstalled probes whether tesseract is installed locally, returning path, version
// and OS. Uses the same probe logic as NewTesseractOCR (PATH + common install locations);
// on a hit it additionally runs `tesseract --version` to extract the version (first line
// looks like tesseract 5.3.4).
func TesseractInstalled() TesseractStatus {
	status := TesseractStatus{OS: runtime.GOOS}
	if p := resolveTesseract(); p != "" {
		status.Installed = true
		status.Path = p
		status.Version = tesseractVersion(p)
	}
	return status
}

// tesseractVersion runs `tesseract --version` and extracts the version (first line looks like
// "tesseract 5.3.4"). Returns an empty string on parse failure (doesn't affect the "installed"
// verdict).
func tesseractVersion(bin string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return ""
	}
	// Example first line: tesseract 5.3.4  leptonica-1.83.0  ...
	first, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	fields := strings.FieldsSeq(first)
	for f := range fields {
		// Version looks like 5.3.4 (contains dots, purely numeric segments)
		if strings.Count(f, ".") >= 1 && !strings.ContainsAny(f, " /\\") {
			return f
		}
	}
	return ""
}

// NewTesseractOCR constructs the OCR engine. cfg is the tesseract engine's EngineConfig
// (containing langs / timeout_sec from Extra(JSON)); when bin (Endpoint) is empty, the local
// tesseract is auto-detected.
func NewTesseractOCR(cfg *EngineConfig) *TesseractOCR {
	bin := ""
	if cfg != nil {
		bin = cfg.Endpoint
	}
	if bin == "" {
		bin = resolveTesseract()
	}
	return &TesseractOCR{name: "tesseract", config: cfg, bin: bin}
}

// ocrOptions resolves the effective langs / timeout from the current config and this request.
// Priority: explicit req override > engine Extra config > built-in defaults (chi_sim+eng / 60s).
func (t *TesseractOCR) ocrOptions(req model.OcrRequest) (langs string, timeoutSec int) {
	langs = DefaultOCRLangs["tesseract"]
	timeoutSec = DefaultOCRTimeoutSec
	e := parseOCRExtra(t.name, optExtra(t.config))
	if e.Langs != "" {
		langs = e.Langs
	}
	if e.TimeoutSec > 0 {
		timeoutSec = e.TimeoutSec
	}
	if req.TimeoutSec > 0 {
		timeoutSec = req.TimeoutSec
	}
	return
}

// optExtra safely reads EngineConfig.Extra; returns an empty string when cfg is nil.
func optExtra(cfg *EngineConfig) string {
	if cfg == nil {
		return ""
	}
	return cfg.Extra
}

// Name returns the engine name.
func (t *TesseractOCR) Name() string { return t.name }

// Recognize extracts text from the image bytes.
func (t *TesseractOCR) Recognize(ctx context.Context, req model.OcrRequest) (*model.OcrResult, error) {
	if len(req.ImageData) == 0 {
		return nil, ErrEmptyImage
	}
	langs, timeoutSec := t.ocrOptions(req)

	// Use the ctx carried by req as the base, layered with the engine config's timeout cap
	// (including the +10s headroom agreed with Swift/vision).
	if _, ok := ctx.Deadline(); !ok && timeoutSec > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSec+10)*time.Second)
		defer cancel()
	}

	tmp, err := os.CreateTemp("", "kai-ocr-*.png")
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(req.ImageData); err != nil {
		return nil, err
	}
	tmp.Close()

	outBase := strings.TrimSuffix(tmp.Name(), ".png")
	bin := t.bin
	if bin == "" {
		bin = "tesseract" // Fallback, so PATH problems surface in the error
	}
	cmd := exec.CommandContext(ctx, bin, tmp.Name(), outBase, "-l", langs, "--psm", "6")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		var msg string
		if t.bin == "" {
			msg = ErrTesseractNotFound.Error()
		} else {
			msg = fmt.Sprintf("%s(%v): %s", i18n.T("err.ocr_tesseract_exec_failed"), err, stderr.String())
		}
		return nil, &OcrError{Msg: msg}
	}

	txtPath := outBase + ".txt"
	raw, err := os.ReadFile(txtPath)
	if err != nil {
		return nil, err
	}
	_ = os.Remove(txtPath)

	text := strings.TrimSpace(string(raw))
	regions := []model.OcrRegion{{Text: text, Conf: 0, Box: nil}}
	return &model.OcrResult{
		Engine:  t.name,
		Text:    text,
		Regions: regions,
	}, nil
}

// OcrError is an OCR execution error.
type OcrError struct{ Msg string }

func (e *OcrError) Error() string { return e.Msg }

// CaptureRegion pops the system's interactive region screenshot (user drag-selects) and
// returns the cropped PNG bytes. In this mode the user drags a rectangle on screen with the
// mouse; on release it is written to a temp file. Requires screen-recording permission.
// Relies on macOS's bundled screencapture (nothing to install).
func CaptureRegion(ctx context.Context) ([]byte, error) {
	switch runtime.GOOS {
	case "darwin":
		tmp, err := os.CreateTemp("", "kai-region-*.png")
		if err != nil {
			return nil, err
		}
		path := tmp.Name()
		tmp.Close()
		defer os.Remove(path)
		// -i interactive mode (user drag-selects), -x silent, no shutter sound.
		// Note: -R means "capture the given rectangle (non-interactive)" and needs
		// -R x,y,w,h values; it cannot be combined with -i — a bare "-i -R" makes
		// screencapture exit status 1 immediately. Interactive drag-select uses -i only.
		cmd := exec.CommandContext(ctx, "screencapture", "-i", "-x", path)
		if err := cmd.Run(); err != nil {
			// screencapture exits non-zero when the user presses ESC to cancel; treat as
			// cancellation, not a system error.
			return nil, fmt.Errorf("%s: %w", i18n.T("err.ocr_region_capture_failed"), err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", i18n.T("err.ocr_read_region_failed"), err)
		}
		if len(data) == 0 {
			// screencapture exited 0 but produced an empty file (e.g. zero-area selection);
			// return a clear error so downstream OCR doesn't misreport.
			return nil, ErrEmptyImage
		}
		return data, nil
	default:
		return nil, ErrNoScreenshot
	}
}

// CaptureScreenshot captures the full screen with the system screenshot tool, returning PNG bytes.
func CaptureScreenshot() ([]byte, error) {
	tmp, err := os.CreateTemp("", "kai-shot-*.png")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("screencapture", "-x", path)
	case "linux":
		// ImageMagick import captures the full screen
		cmd = exec.Command("import", "-window", "root", path)
	case "windows":
		// TODO(M6): use a built-in tool, e.g. powershell calling ScreenCapture
		return nil, ErrNoScreenshot
	default:
		return nil, ErrNoScreenshot
	}
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
