package wails_updater_providers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// TestGitHubNightlyWithStableNeedsUpdate: the GitHub source's "update needed" scenario (via
// the public entries):
// with the nightly channel subscribed (prerelease=true) and both a stable version and a
// nightly online, Check should return
// the nightly (nightly-x1b2c3), not get overridden by the stable. Verifies nightly is the
// priority channel and the update is judged from the nightly itself.
func TestGitHubNightlyWithStableNeedsUpdate(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	// /releases/latest returns the formal stable v1.2.0 (not a prerelease; only checkStable
	// uses it).
	mux.HandleFunc("/repos/"+testRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName:     "v1.2.0",
			Name:        "release 1.2.0",
			Prerelease:  false,
			PublishedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
			Assets: []githubAsset{
				{Name: "example-darwin-arm64.app.zip", Size: 12345},       // decoy: no updater- prefix
				{Name: "updater-windows-amd64.zip", Size: int64(7776665)}, // decoy: wrong platform
				{Name: "updater-darwin-arm64.zip.sig", Size: 256},
				{Name: "SHA256SUMS", Size: int64(512)},
				{Name: "updater-darwin-arm64.zip", Size: int64(9988776)},
			},
		})
	})
	// Release list: stable v1.2.0 plus a prerelease nightly (checkPrerelease filters
	// pre-releases from here).
	mux.HandleFunc("/repos/"+testRepo+"/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]githubRelease{
			{
				TagName:     "nightly-x1b2c3",
				Name:        "nightly build",
				Prerelease:  true,
				PublishedAt: now.Add(-1 * time.Hour).Format(time.RFC3339),
				Assets: []githubAsset{
					{Name: "example-darwin-arm64.app.zip", Size: 12345},       // decoy: no updater- prefix
					{Name: "updater-windows-amd64.zip", Size: int64(7776665)}, // decoy: wrong platform
					{Name: "updater-darwin-arm64.zip.sig", Size: 256},
					{Name: "SHA256SUMS", Size: int64(512)},
					{Name: "GIT_COMMIT", Size: 41},
					{Name: "BUILD_TIME", Size: 28},
					{Name: "updater-darwin-arm64.zip", Size: int64(9988776)},
				},
			},
			{
				TagName:     "v1.2.0",
				Name:        "release 1.2.0",
				Prerelease:  false,
				PublishedAt: now.Add(-48 * time.Hour).Format(time.RFC3339),
				Assets: []githubAsset{
					{Name: "updater-darwin-arm64.zip", Size: 9988776},
				},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/releases/download/nightly-x1b2c3/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  updater-darwin-arm64.zip\n"))
	})
	mux.HandleFunc("/"+testRepo+"/releases/download/nightly-x1b2c3/GIT_COMMIT", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789abcdef0123456789abcdef01234567\n"))
	})
	mux.HandleFunc("/"+testRepo+"/releases/download/nightly-x1b2c3/BUILD_TIME", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(now.Format(time.RFC3339) + "\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceGithub)
	mp, err := NewMirrorProvider(&Options{
		GithubRepo:    testRepo,
		GithubToken:   "test-token",
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
	t.Logf("[GitHub] current version (currentVersion=%q, buildTime=%s), needsUpdate=%v, candidate=%s", req.CurrentVersion, mp.buildTime.Format(time.RFC3339), rel != nil, safeVersion(rel))
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

// TestGitHubNightlyWithStableNoUpdate: the GitHub source's "no update" scenario (via the
// public entries):
// the nightly channel is subscribed and the local nightly is already latest (buildTime later
// than the nightly); Check should return an error.
func TestGitHubNightlyWithStableNoUpdate(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	mux.HandleFunc("/repos/"+testRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName:     "v1.2.0",
			Name:        "release 1.2.0",
			Prerelease:  false,
			PublishedAt: now.Add(-72 * time.Hour).Format(time.RFC3339),
			Assets: []githubAsset{
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: int64(512)},
			},
		})
	})
	mux.HandleFunc("/repos/"+testRepo+"/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]githubRelease{
			{
				TagName:     "nightly-x1b2c3",
				Name:        "nightly build",
				Prerelease:  true,
				PublishedAt: now.Add(-48 * time.Hour).Format(time.RFC3339),
				Assets: []githubAsset{
					{Name: "updater-darwin-arm64.zip", Size: 9988776},
					{Name: "SHA256SUMS", Size: int64(512)},
					{Name: "GIT_COMMIT", Size: 41},
					{Name: "BUILD_TIME", Size: 28},
				},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/releases/download/nightly-x1b2c3/BUILD_TIME", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(now.Add(-48*time.Hour).Format(time.RFC3339) + "\n"))
	})
	mux.HandleFunc("/"+testRepo+"/releases/download/nightly-x1b2c3/GIT_COMMIT", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789abcdef0123456789abcdef01234567\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceGithub)
	mp, err := NewMirrorProvider(&Options{
		GithubRepo:    testRepo,
		GithubToken:   "test-token",
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
	t.Logf("[GitHub] current version (currentVersion=%q, buildTime=%s), needsUpdate=%v, candidate=%s", req.CurrentVersion, mp.buildTime.Format(time.RFC3339), rel != nil, safeVersion(rel))
	if err != nil {
		t.Fatalf("Check should return nil, nil (up-to-date) when both nightly and stable are latest, got error: %v", err)
	}
	if rel != nil {
		t.Fatalf("should not return a release when up-to-date, got %s", rel.Version)
	}
}

// TestGitHubNightlyWithStableNoStableFallbackOnLatest: guard for the GitHub source's "with
// prerelease on, stable also competes" (via the public entries):
// nightly is subscribed, but the nightly's publish time is not newer than the local buildTime
// (local is newer), so the nightly is judged no-update (nil,nil);
// the stable 1.2.0 is newer than local with matching assets, so under the "both versions
// compete" semantics Check should return the stable 1.2.0 candidate.
func TestGitHubNightlyWithStableNoStableFallbackOnLatest(t *testing.T) {
	now := time.Now()
	mux := http.NewServeMux()

	mux.HandleFunc("/repos/"+testRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName:     "v1.2.0",
			Name:        "release 1.2.0",
			Prerelease:  false,
			PublishedAt: now.Add(-72 * time.Hour).Format(time.RFC3339),
			Assets: []githubAsset{
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: int64(512)},
			},
		})
	})
	mux.HandleFunc("/repos/"+testRepo+"/releases", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]githubRelease{
			{
				TagName:     "nightly-x1b2c3",
				Name:        "nightly build",
				Prerelease:  true,
				PublishedAt: now.Add(-48 * time.Hour).Format(time.RFC3339), // older than local
				Assets: []githubAsset{
					{Name: "updater-darwin-arm64.zip", Size: 9988776},
					{Name: "SHA256SUMS", Size: int64(512)},
					{Name: "GIT_COMMIT", Size: 41},
					{Name: "BUILD_TIME", Size: 28},
				},
			},
		})
	})
	mux.HandleFunc("/"+testRepo+"/releases/download/nightly-x1b2c3/BUILD_TIME", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(now.Add(-48*time.Hour).Format(time.RFC3339) + "\n"))
	})
	mux.HandleFunc("/"+testRepo+"/releases/download/nightly-x1b2c3/GIT_COMMIT", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("0123456789abcdef0123456789abcdef01234567\n"))
	})
	mux.HandleFunc("/"+testRepo+"/releases/download/v1.2.0/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  updater-darwin-arm64.zip\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceGithub)
	mp, err := NewMirrorProvider(&Options{
		GithubRepo:    testRepo,
		GithubToken:   "test-token",
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
	t.Logf("[GitHub] current version (currentVersion=%q, buildTime=%s), needsUpdate=%v, candidate=%s", req.CurrentVersion, mp.buildTime.Format(time.RFC3339), rel != nil, safeVersion(rel))
	if err != nil {
		t.Fatalf("Check should return up-to-date (nil, nil) when nightly needs no update, got error: %v", err)
	}
	if rel != nil {
		t.Fatalf("Check should return nil (no stable fallback) when the latest nightly needs no update, got %s", rel.Version)
	}
}

// TestGitHubNightlyWithStableNoStableFallbackOnError: guard for the GitHub source's "with
// prerelease on, stable also competes" (via the public entries):
// nightly is subscribed but the nightly channel errors outright (the release-list endpoint
// returns 500; prerelease fails);
// the stable 1.2.0 is usable with matching assets, so under the "both versions compete"
// semantics Check should return the stable 1.2.0 candidate (rather than erroring).
// Note: GitHub's nightly (/releases) and stable (/releases/latest) are different endpoints,
// so their statuses can be controlled separately.
func TestGitHubNightlyWithStableNoStableFallbackOnError(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/repos/"+testRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName:     "v1.2.0",
			Name:        "release 1.2.0",
			Prerelease:  false,
			PublishedAt: time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
			Assets: []githubAsset{
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: int64(512)},
			},
		})
	})
	mux.HandleFunc("/repos/"+testRepo+"/releases", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/"+testRepo+"/releases/download/v1.2.0/SHA256SUMS", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890  updater-darwin-arm64.zip\n"))
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceGithub)
	mp, err := NewMirrorProvider(&Options{
		GithubRepo:    testRepo,
		GithubToken:   "test-token",
		BuildTime:     time.Now().Add(-72 * time.Hour),
		Prerelease:    true,
		AssetMatcher:  NewUpdaterAssetMatcher(),
		ChecksumFile:  "SHA256SUMS",
		GitCommitFile: "GIT_COMMIT",
		BuildTimeFile: "BUILD_TIME",
	})
	if err != nil {
		t.Fatalf("failed to construct NewMirrorProvider: %v", err)
	}
	// Prerelease on: the newest entry goes through the /releases list endpoint; that endpoint
	// 500s → the whole thing errors, without falling back to the /releases/latest stable.
	req := updater.CheckRequest{Platform: "darwin", Arch: "arm64", CurrentVersion: "1.1.0"}
	rel, err := mp.Check(context.Background(), req)
	t.Logf("[GitHub] current version (currentVersion=%q, buildTime=%s), needsUpdate=%v, candidate=%s", req.CurrentVersion, mp.buildTime.Format(time.RFC3339), rel != nil, safeVersion(rel))
	if err == nil {
		t.Fatalf("Check should return an error (no stable fallback) when the nightly channel (/releases) fails, got rel=%v", rel)
	}
	if rel != nil {
		t.Fatalf("expected nil rel on error, got %s", rel.Version)
	}
}
