package engine

import (
	"context"
	"sort"
	"strings"

	"cnb.cool/dtapp/kai/internal/model"
)

// redactionMark replaces a configured secret wherever it appears in an engine error.
const redactionMark = "***"

// secretRedactor is the Translator decorator behind WithSecrets.
type secretRedactor struct {
	inner    Translator
	replacer *strings.Replacer
}

// WithSecrets wraps t so the literal secrets (the engine's configured API key and secret) never
// appear in the text of an error it returns: every occurrence becomes "***" (issue #96). Providers
// echo the key they were sent in some error bodies, and the error text reaches both the logs and
// the failure detail shown to the user.
//
// The chain is kept: the returned error's Unwrap is the original error, so errors.Is / errors.As
// (the classifier's only inputs) still see *HTTPError, the sentinels and net.Error. Only Error()
// changes. Empty secrets are ignored, and with no non-empty secret t is returned as it is.
//
// A decorator hides whatever optional interfaces the engine behind it implements, so each optional
// interface has to be forwarded here: SupportsAutoSource and the not-configured marker. A new
// optional interface added to a credentialed engine must be forwarded too, or it silently stops
// working once the engine is registered.
func WithSecrets(t Translator, secrets ...string) Translator {
	seen := make(map[string]bool, len(secrets))
	var distinct []string
	for _, s := range secrets {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		distinct = append(distinct, s)
	}
	if len(distinct) == 0 {
		return t
	}
	// Longest first: strings.Replacer prefers the earlier pair at one position, so a secret that
	// contains another (a key that starts with the app id, say) is replaced whole.
	sort.SliceStable(distinct, func(i, j int) bool { return len(distinct[i]) > len(distinct[j]) })
	pairs := make([]string, 0, 2*len(distinct))
	for _, s := range distinct {
		pairs = append(pairs, s, redactionMark)
	}
	return &secretRedactor{inner: t, replacer: strings.NewReplacer(pairs...)}
}

// Name delegates to the engine: the registry keys on it.
func (r *secretRedactor) Name() string { return r.inner.Name() }

// Translate runs the engine and redacts the secrets from the text of its error.
func (r *secretRedactor) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	res, err := r.inner.Translate(ctx, req)
	if err != nil {
		return res, withText(r.replacer.Replace(err.Error()), err)
	}
	return res, nil
}

// SupportsAutoSource forwards the optional autoSourceSupporter interface.
func (r *secretRedactor) SupportsAutoSource() bool { return SupportsAutoSource(r.inner) }

// notConfigured forwards the not-configured marker (see IsNotConfigured).
func (r *secretRedactor) notConfigured() bool { return IsNotConfigured(r.inner) }
