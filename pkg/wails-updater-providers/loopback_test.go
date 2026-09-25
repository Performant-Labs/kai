package wails_updater_providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// issue #7 (Tester role, RED): pkg tests must go through real loopback HTTP
// (httptest.Server + a real *http.Client), reusing testhelper_test.go's
// redirectClient (after host rewriting it still goes through http.DefaultClient's real
// network stack),
// with no separate client/transport mocks; the server side captures query params and the
// Authorization header.

// cnbReleaseFixture is one CNB release tag detail response shape.
func cnbReleaseFixture(tag string) cnbReleaseTagDetail {
	return cnbReleaseTagDetail{
		TagName: tag,
		Body:    "release notes",
		Assets: []cnbReleaseAsset{
			{Name: "updater-linux-amd64.tar.gz", Size: 42},
		},
	}
}

// startCNBLoopback starts a real loopback httptest.Server, replaying the CNB release API by
// path:
//   - GET /{repo}/-/releases          → tag list (stable v2.0.0 + a newer nightly)
//   - GET /{repo}/-/releases/tags/... → tag detail（assets: updater-linux-amd64.tar.gz）
//   - GET /{repo}/-/releases/download/{tag}/SHA256SUMS → the checksum sidecar
//
// Received requests (path/query/Authorization header) are recorded into hits for assertions.
type cnbHits struct {
	t       *testing.T
	paths   []string
	authors []string
}

func (h *cnbHits) record(r *http.Request) {
	h.paths = append(h.paths, r.URL.Path)
	h.authors = append(h.authors, r.Header.Get("Authorization"))
}

func startCNBLoopback(t *testing.T, now time.Time) (*httptest.Server, *cnbHits) {
	t.Helper()
	hits := &cnbHits{t: t}
	mux := http.NewServeMux()

	mux.HandleFunc("/"+testRepo+"/-/releases", func(w http.ResponseWriter, r *http.Request) {
		hits.record(r)
		if got := r.URL.Query().Get("page"); got != "1" {
			http.Error(w, "expected page=1, got "+got, http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode([]cnbReleaseListItem{
			{TagName: "nightly-x1b2c3", Name: "nightly", Prerelease: true, PublishedAt: now.Add(-1 * time.Hour).Format(time.RFC3339)},
			{TagName: "v2.0.0", Name: "release 2.0.0", Prerelease: false, Draft: false, PublishedAt: now.Add(-48 * time.Hour).Format(time.RFC3339)},
		})
	})

	mux.HandleFunc("/"+testRepo+"/-/releases/tags/", func(w http.ResponseWriter, r *http.Request) {
		hits.record(r)
		tag := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		_ = json.NewEncoder(w).Encode(cnbReleaseFixture(tag))
	})

	mux.HandleFunc("/"+testRepo+"/-/releases/download/", func(w http.ResponseWriter, r *http.Request) {
		hits.record(r)
		suffix := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		if suffix != "SHA256SUMS" {
			http.Error(w, "unexpected download "+suffix, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("9f8e7d6c5b4a39281706f5e4d3c2b1a09f8e7d6c5b4a39281706f5e4d3c2b1a0  updater-linux-amd64.tar.gz\n"))
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, hits
}

// TestLoopbackCNBReceivesRealRequests issue #7: once the widened
// go test ./internal/... ./pkg/... covers pkg/wails-updater-providers,
// tests must hit the CNB API over real loopback HTTP (no client/transport mocks),
// capturing /releases?page=1 and the Bearer header; Check must return the v2.0.0 update.
// If the provider switches to a different client/URL, the loopback sees nothing → the
// assertion fails (RED).
func TestLoopbackCNBReceivesRealRequests(t *testing.T) {
	now := time.Now()
	srv, hits := startCNBLoopback(t, now)

	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceCNB)
	mp, err := NewMirrorProvider(&Options{
		CnbRepo:      testRepo,
		CnbToken:     "loopback-token",
		BuildTime:    now.Add(-72 * time.Hour),
		AssetMatcher: NewUpdaterAssetMatcher(),
		ChecksumFile: "SHA256SUMS",
	})
	if err != nil {
		t.Fatalf("failed to construct NewMirrorProvider: %v", err)
	}

	rel, err := mp.Check(context.Background(), updater.CheckRequest{
		Platform:       "linux",
		Arch:           "amd64",
		CurrentVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Check should return an update, got error: %v", err)
	}
	if len(hits.paths) == 0 {
		t.Fatal("loopback server received no requests: provider did not use the injected HTTP client (client/transport mocked or source switched)")
	}
	if hits.paths[0] != "/"+testRepo+"/-/releases" {
		t.Errorf("first request path should be /%s/-/releases, got %q", testRepo, hits.paths[0])
	}
	if hits.authors[0] != "Bearer loopback-token" {
		t.Errorf("releases request should carry a Bearer auth header, got %q", hits.authors[0])
	}
	if rel == nil {
		t.Fatal("should return a release when an update is available")
	}
	if rel.Version != "v2.0.0" {
		t.Errorf("should select stable v2.0.0, got %s", rel.Version)
	}
	if rel.Artifact.Filename != "updater-linux-amd64.tar.gz" {
		t.Errorf("should hit updater-linux-amd64.tar.gz, got %s", rel.Artifact.Filename)
	}
}
