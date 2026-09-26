package translate

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
)

// TestClassifyEngineError verifies engine error -> user-facing category (issues #42, #96).
// Each error is wrapped the way callEngine wraps it ("%s(%s): %w" with the localized prefix),
// so the structured signals (errors.Is / errors.As) must survive the wrap, and the zh-CN prefix
// must not change the answer. Structured signals win; the E1 substring lists stay as the
// fallback for text-only errors.
func TestClassifyEngineError(t *testing.T) {
	wrapEn := func(err error) error {
		return fmt.Errorf("%s(%s): %w", "Translation failed", "google", err)
	}
	wrapZhTimeout := func(err error) error {
		return fmt.Errorf("%s(%s): %w", "翻译超时", "google", err)
	}
	missingKey := fmt.Errorf("stub: %w", engine.ErrAPIKey)
	unsupportedPair := fmt.Errorf("apple no source: %w", engine.ErrUnsupportedPair)
	dialErr := &url.Error{Op: "Get", URL: "https://x", Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}}

	cases := []struct {
		name string
		err  error
		want string
	}{
		{"ErrAPIKey", wrapEn(engine.ErrAPIKey), "not_configured"},
		{"stub missing key wraps ErrAPIKey", wrapEn(missingKey), "not_configured"},
		{"http 401", wrapEn(&engine.HTTPError{Status: 401}), "auth"},
		{"http 403", wrapEn(&engine.HTTPError{Status: 403}), "auth"},
		{"http 402", wrapEn(&engine.HTTPError{Status: 402}), "quota"},
		{"http 429", wrapEn(&engine.HTTPError{Status: 429}), "rate_limit"},
		{"http 403 with Kind override", wrapEn(&engine.HTTPError{Status: 403, Kind: "rate_limit"}), "rate_limit"},
		{"http 503", wrapEn(&engine.HTTPError{Status: 503}), "unavailable"},
		{"http 500", wrapEn(&engine.HTTPError{Status: 500}), "unavailable"},
		{"http 414", wrapEn(&engine.HTTPError{Status: 414}), "too_long"},
		{"http 413", wrapEn(&engine.HTTPError{Status: 413}), "too_long"},
		{"http 418 other status", wrapEn(&engine.HTTPError{Status: 418}), "engine"},
		{"deadline under zh-CN timeout prefix", wrapZhTimeout(context.DeadlineExceeded), "network"},
		{"url.Error wrapping net.OpError", wrapEn(dialErr), "network"},
		{"wraps ErrUnsupportedPair", wrapEn(unsupportedPair), "pair"},
		{"apple pair text fallback", errors.New(`[动态桥接] 系统翻译失败: 引擎返回错误 (Unable to Translate)`), "pair"},
		{"unsupported language pair text", errors.New(`unsupported language pair es->zh`), "pair"},
		{"connection refused text", errors.New(`Get "https://x": dial tcp 1.2.3.4:443: connect: connection refused`), "network"},
		{"dns text", errors.New(`no such host`), "network"},
		{"tls text", errors.New(`tls: handshake failure`), "network"},
		{"bad key text fallback", errors.New(`401 Unauthorized: Invalid API key`), "auth"},
		{"forbidden text fallback", errors.New(`403 Forbidden`), "auth"},
		{"generic engine error", errors.New(`some unexpected engine failure`), "engine"},
		{"nil error", nil, "engine"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ClassifyEngineError(tc.err); got != tc.want {
				t.Errorf("ClassifyEngineError(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// Structured signals are checked before text: an HTTPError whose message text mentions
// "timeout" or "unauthorized" is classified by its status, not by the substring lists.
func TestClassifyEngineErrorStructuredBeatsText(t *testing.T) {
	err := fmt.Errorf("x(y): %w", &engine.HTTPError{Status: 429, Message: "unauthorized timeout"})
	if got := ClassifyEngineError(err); got != "rate_limit" {
		t.Errorf("got %q, want rate_limit (status beats substring)", got)
	}
	// The API-key sentinel beats a network-looking message.
	err = fmt.Errorf("dial tcp timeout: %w", engine.ErrAPIKey)
	if got := ClassifyEngineError(err); got != "not_configured" {
		t.Errorf("got %q, want not_configured", got)
	}
}
