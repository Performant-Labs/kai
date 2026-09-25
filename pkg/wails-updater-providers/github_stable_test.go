package wails_updater_providers

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// TestGitHubStableNeedsUpdate GitHub 源稳定版「需要更新」场景（走公开入口 NewMirrorProvider + NewUpdaterAssetMatcher）：
// 本地 1.0.0，/releases/latest 返回 1.2.0，Check 应返回稳定版更新并命中 updater 包、解析校验和。
func TestGitHubStableNeedsUpdate(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/repos/"+testRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName:     "v1.2.0",
			Name:        "release 1.2.0",
			Prerelease:  false,
			PublishedAt: time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
			Assets: []githubAsset{
				{Name: "example-darwin-arm64.app.zip", Size: 12345},
				{Name: "updater-darwin-arm64.zip.sig", Size: 256},
				{Name: "SHA256SUMS", Size: 512},
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
			},
		})
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
		GithubRepo:   testRepo,
		GithubToken:  "test-token",
		BuildTime:    time.Now().Add(-72 * time.Hour),
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
		t.Fatalf("expected v1.2.0, got %s", rel.Version)
	}
	if rel.Artifact.Filename != "updater-darwin-arm64.zip" {
		t.Fatalf("expected asset updater-darwin-arm64.zip, got %s", rel.Artifact.Filename)
	}
	if rel.Verification == nil || hex.EncodeToString(rel.Verification.Digest) != "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890" {
		t.Fatalf("checksum not parsed correctly: %+v", rel.Verification)
	}
}

// TestGitHubStableNoUpdate GitHub 源稳定版「不需要更新」场景（走公开入口 NewMirrorProvider + NewUpdaterAssetMatcher）：
// 本地已是线上最新 1.2.0，Check 应返回 error（无可用更新）。
func TestGitHubStableNoUpdate(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/repos/"+testRepo+"/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(githubRelease{
			TagName:     "v1.2.0",
			Name:        "release 1.2.0",
			Prerelease:  false,
			PublishedAt: time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
			Assets: []githubAsset{
				{Name: "updater-darwin-arm64.zip", Size: 9988776},
				{Name: "SHA256SUMS", Size: 512},
			},
		})
	})

	srv := mustServe(t, mux)
	SetClient(redirectClient(srv))
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceGithub)
	mp, err := NewMirrorProvider(&Options{
		GithubRepo:   testRepo,
		GithubToken:  "test-token",
		BuildTime:    time.Now().Add(-72 * time.Hour),
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
