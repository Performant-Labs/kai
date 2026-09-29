//go:build !darwin

package engine

import (
	"context"
	"fmt"
	"log/slog"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// appleTranslator is a placeholder for non-macOS platforms (system translation is macOS-only).
type appleTranslator struct{}

// NewApple returns an unsupported system translation engine on non-macOS platforms.
func NewApple() Translator {
	return &appleTranslator{}
}

func (s *appleTranslator) Name() string { return "apple" }

// Translate: system translation is not supported on non-macOS platforms.
func (s *appleTranslator) Translate(_ context.Context, _ model.TranslateRequest) (*model.TranslateResult, error) {
	return nil, fmt.Errorf(i18n.T("err.apple_unsupported_platform"))
}

// SetLogConfig configures engine-layer log output. Non-darwin platforms have no Swift bridge
// logging system, so it goes straight to the Go standard slog (default output to stderr).
// The signature matches the darwin version so callers stay uniform.
func SetLogConfig(dir, level string, _ int, _ bool) {
	if dir != "" {
		// Non-darwin platforms don't redirect to a file yet — just record the intent, avoiding
		// CGO/platform-specific file locking.
		slog.Info(i18n.T("log.apple_setlog_std"),
			"dir", dir, "level", level)
	}
}

// SetBridgeLocale: non-darwin platforms have no Swift bridge layer, so there is nothing to
// sync; empty implementation keeps the signature consistent.
func SetBridgeLocale(_ string) {}

// WarmTranslate: non-darwin platforms have no Apple engine (system translation is macOS-only,
// see Translate above); empty implementation keeps the signature consistent with
// apple_darwin.go's so main.go can call it unconditionally.
func WarmTranslate(_, _ string) {}

// DetectLanguage has no local detector off macOS (issue #200): it always reports none, and the
// translate service falls back to the detection a translation result carries.
func DetectLanguage(_ string) (model.Language, float64, bool) {
	return "", 0, false
}
