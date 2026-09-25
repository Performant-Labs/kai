package wails_updater_providers

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// TestCNBStableNeedsUpdate: the CNB source's stable "update needed" scenario (via the public
// entries NewMirrorProvider + NewUpdaterAssetMatcher):
// local 1.0.0, 1.2.0 online (not a prerelease, not a draft); Check should return the stable
// update, hit the updater package, and parse the checksum.
func TestCNBStableNeedsUpdate(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	mux.HandleFunc("/"+testRepo+"/-/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]cnbReleaseListItem{
			{TagName: "nightly-x1b2c3", Name: "nightly", Body: "nightly", Prerelease: true, PublishedAt: now.Add(-1 * time.Hour).Format(time.RFC3339)},
			{TagName: "v1.2.0", Name: "release", Body: "release", Prerelease: false, Draft: false, PublishedAt: now.Add(-48 * time.Hour).Format(time.RFC3339)},
		})
	})

	mux.HandleFunc("/"+testRepo+"/-/releases/tags/", func(w http.ResponseWriter, r *http.Request) {
		tag := strings.TrimPrefix(r.URL.Path, "/"+testRepo+"/-/releases/tags/")
		_ = json.NewEncoder(w).Encode(cnbReleaseTagDetail{
			TagName: tag,
			Assets: []cnbReleaseAsset{
				{Name: "example-darwin-arm64.app.zip", Size: 12345},
				{Name: "updater-darwin-arm64.zip.sig", Size: 256},
				{Name: "SHA256SUMS", Size: 512},
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
			},
		})
	})

	mux.HandleFunc("/"+testRepo+"/-/releases/download/v1.2.0/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  updater-darwin-arm64.zip\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceCNB)
	mp, err := NewMirrorProvider(&Options{
		CnbRepo:      testRepo,
		CnbToken:     "test-token",
		BuildTime:    now.Add(-72 * time.Hour),
		AssetMatcher: NewUpdaterAssetMatcher(),
		ChecksumFile: "SHA256SUMS",
	})
	if err != nil {
		t.Fatalf("failed to construct NewMirrorProvider: %v", err)
	}

	rel, err := mp.Check(context.Background(), updater.CheckRequest{
		Platform:       "darwin",
		Arch:           "arm64",
		CurrentVersion: "1.0.0",
	})
	if err != nil {
		t.Fatalf("Check should return an update when one is available, got error: %v", err)
	}
	if rel.Version != "v1.2.0" {
		t.Fatalf("expected stable v1.2.0 after skipping nightly, got %s", rel.Version)
	}
	if rel.Artifact.Filename != "updater-darwin-arm64.zip" {
		t.Fatalf("expected asset updater-darwin-arm64.zip, got %s", rel.Artifact.Filename)
	}
	if rel.Verification == nil || hex.EncodeToString(rel.Verification.Digest) != "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890" {
		t.Fatalf("checksum not parsed correctly: %+v", rel.Verification)
	}
}

// TestCNBStableNoUpdate: the CNB source's stable "no update" scenario (via the public entries
// NewMirrorProvider + NewUpdaterAssetMatcher):
// local is already the online-latest 1.2.0; Check should return nil,nil (no update
// available).
func TestCNBStableNoUpdate(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	mux.HandleFunc("/"+testRepo+"/-/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]cnbReleaseListItem{
			{TagName: "nightly-x1b2c3", Name: "nightly", Body: "nightly", Prerelease: true, PublishedAt: now.Add(-1 * time.Hour).Format(time.RFC3339)},
			{TagName: "v1.2.0", Name: "release", Body: "release", Prerelease: false, Draft: false, PublishedAt: now.Add(-48 * time.Hour).Format(time.RFC3339)},
		})
	})

	mux.HandleFunc("/"+testRepo+"/-/releases/tags/", func(w http.ResponseWriter, r *http.Request) {
		tag := strings.TrimPrefix(r.URL.Path, "/"+testRepo+"/-/releases/tags/")
		_ = json.NewEncoder(w).Encode(cnbReleaseTagDetail{
			TagName: tag,
			Assets: []cnbReleaseAsset{
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: 512},
			},
		})
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceCNB)
	mp, err := NewMirrorProvider(&Options{
		CnbRepo:      testRepo,
		CnbToken:     "test-token",
		BuildTime:    now.Add(-72 * time.Hour),
		AssetMatcher: NewUpdaterAssetMatcher(),
		ChecksumFile: "SHA256SUMS",
	})
	if err != nil {
		t.Fatalf("failed to construct NewMirrorProvider: %v", err)
	}

	rel, err := mp.Check(context.Background(), updater.CheckRequest{
		Platform:       "darwin",
		Arch:           "arm64",
		CurrentVersion: "1.2.0",
	})
	if err != nil {
		t.Fatalf("Check should return nil, nil (up-to-date) when current is already 1.2.0, got error: %v", err)
	}
	if rel != nil {
		t.Fatalf("should not return a release when up-to-date, got %s", rel.Version)
	}
}
