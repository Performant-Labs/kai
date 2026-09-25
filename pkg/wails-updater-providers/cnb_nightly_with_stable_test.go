package wails_updater_providers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// TestCNBNightlyWithStableNeedsUpdate: the CNB source's "update needed" scenario (via the
// public entries):
// with the nightly channel subscribed (prerelease=true) and both a stable version and a
// nightly online, Check should return
// the nightly (nightly-x1b2c3), not get overridden by the stable. Verifies nightly is the
// priority channel and the update is judged from the nightly itself.
func TestCNBNightlyWithStableNeedsUpdate(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	// releases list: both a stable and a nightly (the nightly newest).
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
				{Name: "updater-windows-amd64.zip", Size: 7776665},
				{Name: "updater-darwin-arm64.zip.sig", Size: 256},
				{Name: "SHA256SUMS", Size: 512},
				{Name: "GIT_COMMIT", Size: 41},
				{Name: "BUILD_TIME", Size: 28},
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/nightly-x1b2c3/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  updater-darwin-arm64.zip\n"))
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/nightly-x1b2c3/GIT_COMMIT", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789abcdef0123456789abcdef01234567\n"))
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/nightly-x1b2c3/BUILD_TIME", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(now.Format(time.RFC3339) + "\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceCNB)
	mp, err := NewMirrorProvider(&Options{
		CnbRepo:       testRepo,
		CnbToken:      "test-token",
		BuildTime:     now.Add(-72 * time.Hour),
		Prerelease:    true, // subscribe to the nightly channel
		AssetMatcher:  NewUpdaterAssetMatcher(),
		ChecksumFile:  "SHA256SUMS",
		GitCommitFile: "GIT_COMMIT",
		BuildTimeFile: "BUILD_TIME",
	})
	if err != nil {
		t.Fatalf("failed to construct NewMirrorProvider: %v", err)
	}
	req := updater.CheckRequest{Platform: "darwin", Arch: "arm64", CurrentVersion: "1.1.0"}
	rel, err := mp.Check(context.Background(), req)
	t.Logf("[CNB] current version (currentVersion=%q, buildTime)=%s, needsUpdate=%v, candidate=%s", req.CurrentVersion, mp.buildTime.Format(time.RFC3339), rel != nil, safeVersion(rel))
	if err != nil {
		t.Fatalf("Check should return the nightly update when nightly is subscribed and a stable exists, got error: %v", err)
	}
	if rel.Version != "nightly-x1b2c3" {
		t.Fatalf("expected nightly version nightly-x1b2c3, got %s", rel.Version)
	}
	if rel.Artifact.Filename != "updater-darwin-arm64.zip" {
		t.Fatalf("expected asset updater-darwin-arm64.zip, got %s", rel.Artifact.Filename)
	}
}

// TestCNBNightlyWithStableNoUpdate: the CNB source's "no update" scenario (via the public
// entries):
// the nightly channel is subscribed and the local nightly is already latest (buildTime later
// than the nightly); Check should return nil,nil.
func TestCNBNightlyWithStableNoUpdate(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	mux.HandleFunc("/"+testRepo+"/-/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]cnbReleaseListItem{
			{TagName: "nightly-x1b2c3", Name: "nightly", Body: "nightly", Prerelease: true, PublishedAt: now.Add(-48 * time.Hour).Format(time.RFC3339)},
			{TagName: "v1.2.0", Name: "release", Body: "release", Prerelease: false, Draft: false, PublishedAt: now.Add(-72 * time.Hour).Format(time.RFC3339)},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/tags/", func(w http.ResponseWriter, r *http.Request) {
		tag := strings.TrimPrefix(r.URL.Path, "/"+testRepo+"/-/releases/tags/")
		_ = json.NewEncoder(w).Encode(cnbReleaseTagDetail{
			TagName: tag,
			Assets: []cnbReleaseAsset{
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: 512},
				{Name: "GIT_COMMIT", Size: 41},
				{Name: "BUILD_TIME", Size: 28},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/nightly-x1b2c3/BUILD_TIME", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(now.Add(-48*time.Hour).Format(time.RFC3339) + "\n"))
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/nightly-x1b2c3/GIT_COMMIT", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789abcdef0123456789abcdef01234567\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceCNB)
	mp, err := NewMirrorProvider(&Options{
		CnbRepo:       testRepo,
		CnbToken:      "test-token",
		BuildTime:     now.Add(1 * time.Hour), // newer than both the nightly and stable
		Prerelease:    true,
		AssetMatcher:  NewUpdaterAssetMatcher(),
		ChecksumFile:  "SHA256SUMS",
		GitCommitFile: "GIT_COMMIT",
		BuildTimeFile: "BUILD_TIME",
	})
	if err != nil {
		t.Fatalf("failed to construct NewMirrorProvider: %v", err)
	}
	req := updater.CheckRequest{Platform: "darwin", Arch: "arm64", CurrentVersion: "1.1.0"}
	rel, err := mp.Check(context.Background(), req)
	t.Logf("[CNB] current version (currentVersion=%q, buildTime)=%s, needsUpdate=%v, candidate=%s", req.CurrentVersion, mp.buildTime.Format(time.RFC3339), rel != nil, safeVersion(rel))
	if err != nil {
		t.Fatalf("Check should return nil, nil (up-to-date) when both nightly and stable are latest, got error: %v", err)
	}
	if rel != nil {
		t.Fatalf("should not return a release when up-to-date, got %s", rel.Version)
	}
}

// TestCNBNightlyWithStableNoStableFallbackOnLatest: guard for the CNB source's "with
// prerelease on, stable also competes" (via the public entries):
// nightly is subscribed, but the nightly's publish time is not newer than the local buildTime
// (local is newer),
// so the nightly is judged no-update (nil,nil); the stable 1.2.0 is newer than local with
// matching assets,
// so under the "both versions compete" semantics Check should return the stable 1.2.0
// candidate (not up-to-date).
// Guards the "with prerelease on, stable still participates" semantics: switching back to
// prerelease-only (dropping stable) turns this red.
func TestCNBNightlyWithStableNoStableFallbackOnLatest(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	mux.HandleFunc("/"+testRepo+"/-/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]cnbReleaseListItem{
			{TagName: "nightly-x1b2c3", Name: "nightly", Body: "nightly", Prerelease: true, PublishedAt: now.Add(-48 * time.Hour).Format(time.RFC3339)},
			{TagName: "v1.2.0", Name: "release", Body: "release", Prerelease: false, Draft: false, PublishedAt: now.Add(-72 * time.Hour).Format(time.RFC3339)},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/tags/", func(w http.ResponseWriter, r *http.Request) {
		tag := strings.TrimPrefix(r.URL.Path, "/"+testRepo+"/-/releases/tags/")
		_ = json.NewEncoder(w).Encode(cnbReleaseTagDetail{
			TagName: tag,
			Assets: []cnbReleaseAsset{
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: 512},
				{Name: "GIT_COMMIT", Size: 41},
				{Name: "BUILD_TIME", Size: 28},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/nightly-x1b2c3/BUILD_TIME", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(now.Add(-48*time.Hour).Format(time.RFC3339) + "\n"))
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/nightly-x1b2c3/GIT_COMMIT", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789abcdef0123456789abcdef01234567\n"))
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/1.2.0/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  updater-darwin-arm64.zip\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceCNB)
	mp, err := NewMirrorProvider(&Options{
		CnbRepo:       testRepo,
		CnbToken:      "test-token",
		BuildTime:     now.Add(1 * time.Hour), // local buildTime later than the nightly’s publish time; nightly judged no-update
		Prerelease:    true,
		AssetMatcher:  NewUpdaterAssetMatcher(),
		ChecksumFile:  "SHA256SUMS",
		GitCommitFile: "GIT_COMMIT",
		BuildTimeFile: "BUILD_TIME",
	})
	if err != nil {
		t.Fatalf("failed to construct NewMirrorProvider: %v", err)
	}
	// Prerelease on: the newest entry = nightly-x1b2c3 (a pre-release); local buildTime is
	// later than its publish time → judged no-update.
	// The new logic judges by "the newest entry's type" and does not fall back to the stable
	// second pass: the nightly needs no update, so it's up-to-date.
	req := updater.CheckRequest{Platform: "darwin", Arch: "arm64", CurrentVersion: "1.1.0"}
	rel, err := mp.Check(context.Background(), req)
	t.Logf("[CNB] current version (currentVersion=%q, buildTime)=%s, needsUpdate=%v, candidate=%s", req.CurrentVersion, mp.buildTime.Format(time.RFC3339), rel != nil, safeVersion(rel))
	if err != nil {
		t.Fatalf("Check should return up-to-date (nil, nil) when nightly needs no update, got error: %v", err)
	}
	if rel != nil {
		t.Fatalf("Check should return nil (no stable fallback) when the latest nightly needs no update, got %s", rel.Version)
	}
}

// TestCNBNightlyWithStableNoStableFallbackOnAssetMiss: guard for the CNB source's "with
// prerelease on, stable also competes" (via the public entries):
// nightly is subscribed, but the nightly's newest tag has no assets matching this platform
// (windows/amd64 only), so checkPrerelease fails;
// the stable 1.2.0's assets match darwin/arm64 and are newer than local, so under the "both
// versions compete" semantics it should return stable 1.2.0.
// Guards the "with prerelease on, a nightly failure still lets stable participate"
// semantics.
func TestCNBNightlyWithStableNoStableFallbackOnAssetMiss(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	mux.HandleFunc("/"+testRepo+"/-/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]cnbReleaseListItem{
			{TagName: "night-x1", Prerelease: true, PublishedAt: now.Add(-1 * time.Hour).Format(time.RFC3339)},
			{TagName: "1.2.0", Prerelease: false, Draft: false, PublishedAt: now.Add(-72 * time.Hour).Format(time.RFC3339)},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/tags/night-x1", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(cnbReleaseTagDetail{
			TagName: "night-x1",
			Assets: []cnbReleaseAsset{
				{Name: "updater-windows-amd64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: 512},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/tags/1.2.0", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(cnbReleaseTagDetail{
			TagName: "1.2.0",
			Assets: []cnbReleaseAsset{
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: 512},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/-/releases/download/1.2.0/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  updater-darwin-arm64.zip\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceCNB)
	mp, err := NewMirrorProvider(&Options{
		CnbRepo:       testRepo,
		CnbToken:      "test-token",
		BuildTime:     now.Add(-72 * time.Hour),
		Prerelease:    true,
		AssetMatcher:  NewUpdaterAssetMatcher(),
		ChecksumFile:  "SHA256SUMS",
		GitCommitFile: "GIT_COMMIT",
		BuildTimeFile: "BUILD_TIME",
	})
	if err != nil {
		t.Fatalf("failed to construct NewMirrorProvider: %v", err)
	}
	// Prerelease on: the newest entry = night-x1 (a pre-release), but its assets are
	// windows-only and don't match the local darwin → that candidate is unusable.
	// The new logic judges by "the newest entry's type" and does not fall back to the stable
	// 1.2.0 second pass: no usable candidate = up-to-date.
	req := updater.CheckRequest{Platform: "darwin", Arch: "arm64", CurrentVersion: "1.1.0"}
	rel, err := mp.Check(context.Background(), req)
	t.Logf("[CNB] current version (currentVersion=%q, buildTime)=%s, needsUpdate=%v, candidate=%s", req.CurrentVersion, mp.buildTime.Format(time.RFC3339), rel != nil, safeVersion(rel))
	if err != nil {
		t.Fatalf("Check should return up-to-date (nil, nil) when the nightly asset does not match, got error: %v", err)
	}
	if rel != nil {
		t.Fatalf("Check should return nil (no stable fallback) when the latest nightly asset does not match, got %s", rel.Version)
	}
}
