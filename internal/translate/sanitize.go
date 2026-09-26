package translate

import (
	"regexp"
	"strings"
)

// detailMaxRunes bounds the failure detail shown to the user. The logs keep the full text.
const detailMaxRunes = 300

// htmlMarker replaces an HTML body in the failure detail.
const htmlMarker = "(HTML response)"

var (
	// htmlStartRe finds where an HTML page or fragment starts. Only tags that begin one are listed,
	// so text such as "<nil>" or "<int>" is left alone.
	htmlStartRe = regexp.MustCompile(`(?i)<\s*(?:!doctype|html|head|body|title|meta|script|style|div|span|h[1-6]|p|center|table|pre|br)\b`)

	// urlRe matches an absolute URL inside free text. The class stops at whitespace, quotes and
	// closing parentheses and braces, so the quotes Go puts around a URL in a *url.Error are not
	// swallowed. ']' stays inside it: an IPv6 literal host (http://[::1]:8080/p?x=1) has one.
	urlRe = regexp.MustCompile("(?i)\\b[a-z][a-z0-9+.-]*://[^\\s\"'<>`)}]+")

	// secretParamRe matches a credential-looking name=value pair.
	secretParamRe = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|token|secret|sign|key)=[^\s&"'<>]+`)
	// bearerRe matches an Authorization bearer token.
	bearerRe = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	// deeplAuthRe matches DeepL's Authorization scheme, whose key may contain ':' (":fx").
	deeplAuthRe = regexp.MustCompile(`(?i)\bDeepL-Auth-Key\s+[^\s"']+`)
	// skKeyRe matches an OpenAI-style secret key.
	skKeyRe = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}`)
)

// SanitizeDetail makes an engine error text safe and short enough to show the user (issue #96). It
// is applied once, in failurePayload, to the text of the error that reaches the frontend; the
// logs keep the full text, redacted only of the engine's own configured secrets (engine.WithSecrets).
//
// In this order it: replaces an HTML body, from its first tag on, with "(HTML response)"; drops
// userinfo, query and fragment from every URL (the query of a gtx request is the source text);
// redacts key=/api_key=/token=/secret=/sign= values, Bearer tokens, DeepL-Auth-Key values and
// sk-... keys; collapses whitespace; and truncates to 300 runes, never inside a rune.
func SanitizeDetail(s string) string {
	if loc := htmlStartRe.FindStringIndex(s); loc != nil {
		s = s[:loc[0]] + htmlMarker
	}
	s = urlRe.ReplaceAllStringFunc(s, stripURL)
	s = secretParamRe.ReplaceAllString(s, "${1}=***")
	s = bearerRe.ReplaceAllString(s, "Bearer ***")
	s = deeplAuthRe.ReplaceAllString(s, "DeepL-Auth-Key ***")
	s = skKeyRe.ReplaceAllString(s, "sk-***")
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > detailMaxRunes {
		s = string(r[:detailMaxRunes-1]) + "…"
	}
	return s
}

// stripURL drops the userinfo, the query and the fragment from one matched URL. It works on the
// text, not through net/url, so a URL that does not parse is still cleaned.
func stripURL(u string) string {
	// The fragment and the query go first: either can contain '@' or '/'.
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	schemeEnd := strings.Index(u, "://") + len("://")
	rest := u[schemeEnd:]
	authority := rest
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		authority = rest[:i]
	}
	if at := strings.LastIndexByte(authority, '@'); at >= 0 {
		rest = rest[at+1:]
	}
	return u[:schemeEnd] + rest
}
