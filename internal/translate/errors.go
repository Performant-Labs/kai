package translate

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/model"
)

// ClassifyEngineError sorts an engine failure into the user-facing category the frontend renders
// (model.ErrorKind*, issues #42 and #96). It is the only classifier: engines attach structured
// facts (a status, a provider code, a sentinel) and never pick a kind, and the frontend maps a kind
// to copy without reading the error text.
//
// Structured signals come first, in this order, and each is read with errors.Is / errors.As, so it
// survives every wrap on the way up (callEngine's "%s(%s): %w", engine.WithSecrets):
//
//  1. engine.ErrAPIKey                  -> not_configured
//  2. engine.ErrUnsupportedPair         -> pair
//  3. *engine.HTTPError                 -> its Kind if set, else the status map (see httpErrorKind)
//  4. context.DeadlineExceeded, net.Error -> network
//
// Only then does it fall back to substring matching on the error text (classifyText), which stays
// for text-only errors: Apple's apple_translate detail, and every engine error that has not been
// given a structured cause yet. A nil error is an engine error.
func ClassifyEngineError(err error) string {
	if err == nil {
		return model.ErrorKindEngine
	}
	switch {
	case errors.Is(err, engine.ErrAPIKey):
		return model.ErrorKindNotConfigured
	case errors.Is(err, engine.ErrUnsupportedPair):
		return model.ErrorKindPair
	}
	if he, ok := errors.AsType[*engine.HTTPError](err); ok {
		return httpErrorKind(he)
	}
	// The service's own timeout wraps context.DeadlineExceeded under a localized prefix, so the
	// deadline is recognized by identity, not by the word "timeout" (which only en-US carries).
	if _, ok := errors.AsType[net.Error](err); ok || errors.Is(err, context.DeadlineExceeded) {
		return model.ErrorKindNetwork
	}
	return classifyText(err.Error())
}

// httpErrorKind is the kind of a provider HTTP failure: the engine's own Kind when it set one (it
// knows its provider better than this map does), otherwise the generic status map. A status the
// map does not know is an engine error.
func httpErrorKind(he *engine.HTTPError) string {
	if he.Kind != "" {
		return he.Kind
	}
	switch {
	case he.Status == http.StatusUnauthorized, he.Status == http.StatusForbidden:
		return model.ErrorKindAuth
	case he.Status == http.StatusPaymentRequired:
		return model.ErrorKindQuota
	case he.Status == http.StatusTooManyRequests:
		return model.ErrorKindRateLimit
	case he.Status == http.StatusRequestEntityTooLarge, he.Status == http.StatusRequestURITooLong:
		return model.ErrorKindTooLong
	case he.Status >= 500 && he.Status <= 599:
		return model.ErrorKindUnavailable
	default:
		return model.ErrorKindEngine
	}
}

// classifyText is the substring fallback for errors that carry no structured cause. Engine error
// copy is not standardized, so exact parsing is impractical; the cost of a misclassification is
// one extra generic message (the engine fallback), which is acceptable.
func classifyText(errText string) string {
	lower := strings.ToLower(errText)
	switch {
	case containsAny(lower,
		"unable to translate",
		"language pair",
		"unsupported language",
		"pair not",
	):
		return model.ErrorKindPair
	case containsAny(lower,
		"timeout",
		"connection refused",
		"connection reset",
		"no such host",
		"dial tcp",
		"network is unreachable",
		"tls",
		"proxy",
	):
		return model.ErrorKindNetwork
	case containsAny(lower,
		"401",
		"403",
		"unauthorized",
		"forbidden",
		"invalid api key",
		"invalid key",
		"authentication",
	):
		return model.ErrorKindAuth
	default:
		return model.ErrorKindEngine
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}
