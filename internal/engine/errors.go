package engine

import (
	"errors"
	"strconv"
	"strings"
)

// Structured engine errors (issue #96). Engines attach the facts they have (an HTTP status, a
// provider code, a sentinel) and never decide what those mean for the user: translate's
// ClassifyEngineError is the only classifier and reads these with errors.Is / errors.As, so every
// wrap on the way up (callEngine's "%s(%s): %w", WithSecrets) has to keep the chain.

// ErrUnsupportedPair marks a failure that means "this engine cannot translate this language pair":
// an unsupported target (unsupportedTargetError) and the Apple bridge's no_source_lang. It is a
// marker, not a message: every site wraps it with withText so the user reads that site's own
// localized text, never this string.
var ErrUnsupportedPair = errors.New("language pair not supported")

// HTTPError is a provider failure reported as an HTTP status. The zero Status means the provider
// reported the failure inside a 200 body (baidu, tencent, youdao), where only Code and Message
// are known.
type HTTPError struct {
	Status  int    // HTTP status code
	Code    string // Provider-specific error code, when the response carried one
	Message string // Provider-reported message. Never a raw response body: those can be HTML pages.
	// Kind optionally overrides the classifier's status mapping with a model.ErrorKind* value.
	// An engine sets it only when it knows its provider better than the generic map does (Google's
	// keyless gtx endpoint, where a 403 is a rate limit and cannot mean a bad key). Empty means
	// "derive the kind from Status".
	Kind string
}

// Error renders "HTTP <status> <code>: <message>", leaving out whatever is not known.
func (e *HTTPError) Error() string {
	var head []string
	if e.Status > 0 {
		head = append(head, "HTTP "+strconv.Itoa(e.Status))
	}
	if e.Code != "" {
		head = append(head, e.Code)
	}
	text := strings.Join(head, " ")
	switch {
	case e.Message == "":
	case text == "":
		text = e.Message
	default:
		text += ": " + e.Message
	}
	if text == "" {
		return "HTTP error"
	}
	return text
}

// textError is an error whose text is fixed when it is built while its chain still reaches cause,
// so errors.Is / errors.As see through it.
type textError struct {
	msg   string
	cause error
}

func (e *textError) Error() string { return e.msg }
func (e *textError) Unwrap() error { return e.cause }

// withText returns an error that reads as msg and unwraps to cause. It is how a site keeps its
// existing message byte for byte while adding a sentinel or a typed cause to the chain, where
// fmt.Errorf("...%w", cause) would append the cause's own text to it.
func withText(msg string, cause error) error {
	return &textError{msg: msg, cause: cause}
}
