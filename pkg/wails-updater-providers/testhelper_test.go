package wails_updater_providers

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// Generic placeholder repo; no real business info hardcoded.
const testRepo = "example-org/example-repo"

// redirectClient redirects requests to api.cnb.cool / github.com to the mock server.
func redirectClient(srv *httptest.Server) *http.Client {
	host := strings.TrimPrefix(srv.URL, "http://")
	return &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		u := *req.URL
		u.Scheme = "http"
		u.Host = host
		r := req.Clone(req.Context())
		r.URL = &u
		// Test-only: URLs are constructed explicitly by test code (redirected to the mock
		// server); no SSRF risk.
		//nolint:gosec // the URL is built explicitly by test code, not external input; no SSRF risk.
		return http.DefaultClient.Do(r)
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// discardLogger returns a silent logger so tests don’t flood output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mustServe starts the mock server and closes it when the test ends.
func mustServe(t *testing.T, h http.Handler) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

// safeVersion safely reads the candidate’s version, returning "<no candidate>" when rel is
// nil.
func safeVersion(rel *updater.Release) string {
	if rel == nil {
		return "<no candidate>"
	}
	return rel.Version
}
