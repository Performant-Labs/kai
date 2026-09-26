package engine

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #96 AC5: a non-2xx gtx response returns an error wrapping *HTTPError{Status}; its text
// names the status and carries neither the response body (an HTML page on a gtx block) nor the
// query (the source text). 403 and 429 set Kind rate_limit (gtx has no key, so neither can mean
// a bad key).
func TestGoogleNon2xxReturnsHTTPError(t *testing.T) {
	const htmlBody = `<html><body><h1>Our systems have detected unusual traffic</h1></body></html>`
	cases := []struct {
		status   int
		wantKind string // "" = no override required
	}{
		{http.StatusTooManyRequests, "rate_limit"},
		{http.StatusForbidden, "rate_limit"},
		{http.StatusServiceUnavailable, ""},
		{http.StatusRequestURITooLong, ""},
	}
	for _, tc := range cases {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/html")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(htmlBody))
			}))
			defer srv.Close()
			g := NewGoogle(srv.URL, srv.Client())
			_, err := g.Translate(context.Background(), model.TranslateRequest{Text: "my private sentence", From: model.EN, To: model.ZH})
			if err == nil {
				t.Fatal("no error for non-2xx")
			}
			var he *HTTPError
			if !errors.As(err, &he) {
				t.Fatalf("errors.As(*HTTPError) = false: %v", err)
			}
			if he.Status != tc.status {
				t.Errorf("Status = %d, want %d", he.Status, tc.status)
			}
			if tc.wantKind != "" && he.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q", he.Kind, tc.wantKind)
			}
			txt := err.Error()
			if !strings.Contains(txt, strconv.Itoa(tc.status)) {
				t.Errorf("text %q does not name the status", txt)
			}
			for _, bad := range []string{"<html", "<body", "unusual traffic", "q=", "my private sentence", "%!("} {
				if strings.Contains(txt, bad) {
					t.Errorf("text %q must not contain %q", txt, bad)
				}
			}
		})
	}
}
