package translate

import "strings"

// Engine error categories (issue #42): the frontend maps each category to actionable
// localized copy; the raw error detail travels with TranslateResult.Error and is not part of
// the classification.
const (
	ErrorKindPair    = "pair"    // Language pair unavailable (e.g. macOS system translation hasn't downloaded that pair)
	ErrorKindNetwork = "network" // Network/endpoint unreachable (timeout, DNS, connection refused, TLS)
	ErrorKindAuth    = "auth"    // Authentication/authorization failure (401/403, invalid API key)
	ErrorKindEngine  = "engine"  // Other engine errors (fallback)
)

// ClassifyEngineError sorts engine-returned error text into a user-facing category.
// Substring matching only — engine error copy is not standardized, so exact parsing is
// impractical; the cost of a misclassification is one extra generic message (the engine
// fallback), which is acceptable.
func ClassifyEngineError(errText string) string {
	lower := strings.ToLower(errText)
	switch {
	case containsAny(lower,
		"unable to translate",
		"language pair",
		"unsupported language",
		"pair not",
	):
		return ErrorKindPair
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
		return ErrorKindNetwork
	case containsAny(lower,
		"401",
		"403",
		"unauthorized",
		"forbidden",
		"invalid api key",
		"invalid key",
		"authentication",
	):
		return ErrorKindAuth
	default:
		return ErrorKindEngine
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
