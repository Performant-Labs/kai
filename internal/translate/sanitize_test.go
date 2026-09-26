package translate

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestSanitizeDetail pins the single emit-point sanitizer of the failure detail (issue #96,
// AC9b): URLs lose userinfo/query/fragment, credential-looking tokens are redacted, an HTML body
// becomes a marker, whitespace collapses, and the result is at most 300 runes.
// Fake credentials for the redaction tests, assembled at run time so no literal key-shaped string is
// committed (the pre-commit secret scan flags literals, and these are not real).
var (
	fakeGoogleKey = "AIza" + "SyABCDEF123456"
	fakeSKKey     = "sk-" + "proj_ABCDEFGH12345678"
)

func TestSanitizeDetail(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		mustContain []string
		mustNot     []string
	}{
		{
			name:        "url query dropped",
			in:          `Get "https://translate.googleapis.com/translate_a/single?client=gtx&q=my+secret+text": EOF`,
			mustContain: []string{"https://translate.googleapis.com/translate_a/single"},
			mustNot:     []string{"q=my", "client=gtx", "?"},
		},
		{ //nolint:gosec // G101: fake userinfo for the redaction test, not a real credential
			name:        "url userinfo dropped",
			in:          `dial https://user:hunter2@example.com/path failed`,
			mustContain: []string{"example.com/path"},
			mustNot:     []string{"hunter2", "user:"},
		},
		{
			name:        "url fragment dropped",
			in:          `see https://example.com/a#token123 now`,
			mustContain: []string{"https://example.com/a"},
			mustNot:     []string{"token123", "#"},
		},
		{
			name:    "key= value redacted",
			in:      "request failed key=" + fakeGoogleKey + " status 400",
			mustNot: []string{fakeGoogleKey},
		},
		{
			name:    "Bearer token redacted",
			in:      `401 with header Bearer abc.def-ghi_123 rejected`,
			mustNot: []string{"abc.def-ghi_123"},
		},
		{
			name:    "DeepL-Auth-Key redacted",
			in:      `Authorization: DeepL-Auth-Key 12345678-aaaa-bbbb:fx rejected`,
			mustNot: []string{"12345678-aaaa-bbbb:fx"},
		},
		{
			name:    "sk- key redacted",
			in:      "Incorrect API key provided: " + fakeSKKey,
			mustNot: []string{fakeSKKey},
		},
		{
			name:        "html body replaced by marker",
			in:          `Google Translate: request failed (HTTP 429) <html><body><h1>Sorry</h1></body></html>`,
			mustContain: []string{"(HTML response)"},
			mustNot:     []string{"<html", "<body", "Sorry"},
		},
		{
			name:        "whitespace collapsed",
			in:          "a  b\n\n\tc   d",
			mustContain: []string{"a b c d"},
			mustNot:     []string{"\n", "\t", "  "},
		},
		{
			name:        "plain message untouched",
			in:          "DeepL: missing API key",
			mustContain: []string{"DeepL: missing API key"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SanitizeDetail(tc.in)
			for _, s := range tc.mustContain {
				if !strings.Contains(got, s) {
					t.Errorf("SanitizeDetail(%q) = %q, want it to contain %q", tc.in, got, s)
				}
			}
			for _, s := range tc.mustNot {
				if strings.Contains(got, s) {
					t.Errorf("SanitizeDetail(%q) = %q, must not contain %q", tc.in, got, s)
				}
			}
		})
	}
}

func TestSanitizeDetailTruncatesToRunes(t *testing.T) {
	in := strings.Repeat("翻", 1000)
	got := SanitizeDetail(in)
	if n := utf8.RuneCountInString(got); n > 300 || n < 250 {
		t.Errorf("rune count = %d, want <= 300 (and not needlessly short)", n)
	}
	if !utf8.ValidString(got) {
		t.Errorf("truncation split a rune: %q", got)
	}
	short := "short message"
	if got := SanitizeDetail(short); got != short {
		t.Errorf("short input changed: %q", got)
	}
}
