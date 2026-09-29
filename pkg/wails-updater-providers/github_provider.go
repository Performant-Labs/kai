package wails_updater_providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// githubProvider implements the updater.Provider for the GitHub source (nightly + stable).
type githubProvider struct {
	client        *http.Client        // HTTP client (injected from the package-global GetClient at construction)
	lg            *slog.Logger        // logger (injected from the package-global GetLogger at construction)
	repo          string              // GitHub repo path
	assetMatcher  github.AssetMatcher // asset matcher (the official type)
	checksumFile  string              // checksum file name, verifies artifact integrity after download
	gitCommitFile string              // pre-release git commit file name
	buildTimeFile string              // pre-release build time file name
	token         string              // GitHub access token (Bearer)
	buildTime     time.Time           // this machine's build time
	gitCommit     string              // this machine's git commit
	prerelease    bool                // whether the nightly (pre-release) channel is subscribed
}

// t is a convenience method rendering i18n copy with the current package-global locale.
func (g *githubProvider) t(key string, data ...any) string { return T(key, data...) }

// apiRequest calls a GitHub API endpoint with Accept: application/json and Bearer auth
// (when token is non-empty).
func (g *githubProvider) apiRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	return req, nil
}

// fileRequest downloads a GitHub file with Accept: application/octet-stream and Bearer
// auth (when token is non-empty).
func (g *githubProvider) fileRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	return req, nil
}

// Name implements the updater.Provider interface, returning "github".
func (g *githubProvider) Name() string { return string(SourceGithub) }

// Check implements the updater.Provider interface.
// With prerelease on (prerelease=true): both the prerelease and stable candidates compete
// (the later publish time wins);
// with prerelease off, only the stable channel is checked (checkStable already excludes
// pre-releases).
func (g *githubProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	g.lg.Debug(g.t("updater_start"))
	// Prerelease off: only the stable channel (checkStable already excludes pre-releases).
	if !g.prerelease {
		rel, err := g.checkStable(ctx, req)
		if err != nil {
			return nil, err
		}
		g.lg.Info(g.t("updater_check_done"))
		return rel, nil
	}

	// Prerelease on: take the newest published entry as the candidate and judge by its own
	// type —
	// a pre-release compares downloaded gitCommit/buildTime; a stable version compares
	// version numbers via isNewer.
	// I.e. "whatever type the latest version is, that's how it's judged" — no separate stable
	// second pass, no picking by publish time.
	g.lg.Debug(g.t("updater_check_nightly_channel"))
	return g.checkPrerelease(ctx, req)
}

// Download implements the updater.Provider interface, reusing the shared download logic.
func (g *githubProvider) Download(ctx context.Context, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	return downloadRelease(ctx, g.lg, g.client, ghDownloadURL, g.repo, rel, dst, onProgress, "", g.fileRequest)
}

// fetchChecksum fetches and parses this source’s checksum sidecar, reusing the shared
// logic.
func (g *githubProvider) fetchChecksum(ctx context.Context, downloadURLTpl, repo string, rel *updater.Release, sidecar string) ([]byte, bool) {
	return fetchReleaseChecksum(ctx, g.lg, g.client, downloadURLTpl, repo, rel, sidecar, "", g.fileRequest)
}

// checkPrerelease checks GitHub Pre-release updates: walks the release list filtering
// pre-releases,
// takes the newest by publish time, then decides update-needed from the gitCommit/buildTime
// file contents.
func (g *githubProvider) checkPrerelease(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	// GitHub's /releases/latest only returns stable versions (no pre-releases), so
	// Pre-releases require walking the release list, filtering Prerelease==true entries and
	// taking the newest by publish time (descending) as the candidate.
	// Pre-releases aren't limited to ones named nightly — anything marked pre-release
	// competes.
	listURL := strings.ReplaceAll(ghReleasesList, "{repo}", g.repo)
	httpReq, err := g.apiRequest(ctx, listURL)
	if err != nil {
		return nil, fmt.Errorf("%s", g.t("updater_err_request_create", map[string]any{"Err": err.Error()}))
	}
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s", g.t("updater_err_request_failed", map[string]any{"Err": err.Error()}))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		g.lg.Warn(g.t("updater_warn_unauthorized"))
		return nil, fmt.Errorf("%s", g.t("updater_err_unauthorized"))
	}
	if resp.StatusCode == http.StatusNotFound {
		// A private repo (anonymous callers get 404, not 401) or a repo with no published release
		// yet: nothing to offer. That is "no update", not a failure worth a warning or a dialog
		// (issue #178).
		g.lg.Debug("no releases visible for the update repo (HTTP 404); treating as up to date", "repo", g.repo)
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		g.lg.Warn(g.t("updater_warn_api_error", "Status", resp.StatusCode, "Body", string(body)))
		return nil, fmt.Errorf("%s", g.t("updater_err_api", map[string]any{"Status": resp.StatusCode, "Body": string(body)}))
	}
	var releases []githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		g.lg.Warn(g.t("updater_err_decode", "Err", err.Error()))
		return nil, fmt.Errorf("%s", g.t("updater_err_decode", map[string]any{"Err": err.Error()}))
	}
	// Prerelease on: take the newest published entry (pre-release or stable) and judge by
	// its own type.
	sortReleasesByPublishedAt(releases)
	rel := releases[0]
	publishedAt, err := time.Parse(time.RFC3339, rel.PublishedAt)
	if err != nil {
		g.lg.Debug(g.t("updater_nightly_no_build_time"))
		return nil, fmt.Errorf("%s", g.t("updater_err_nightly_no_published_at"))
	}

	assets := githubAssetsToReleaseAssets(rel.Assets)
	idx := g.assetMatcher(req, assets)
	if idx < 0 || idx >= len(assets) {
		// The newest entry’s upgrade artifact doesn’t match this machine’s platform/arch:
		// the candidate doesn’t apply — treat as up-to-date (not a provider failure).
		g.lg.Debug(g.t("updater_nightly_no_asset", "Tag", rel.TagName, "Platform", req.Platform, "Arch", req.Arch))
		return nil, nil
	}
	filename := rel.Assets[idx].Name
	sizeOf := rel.Assets[idx].Size

	// Stable (the newest entry is not a pre-release): compare version numbers directly;
	// no update needed = up-to-date.
	if !rel.Prerelease {
		tag := strings.TrimPrefix(rel.TagName, "v")
		if !isNewer(tag, req.CurrentVersion) {
			g.lg.Debug(g.t("updater_stable_not_newer", "Tag", tag))
			return nil, nil
		}
		out, berr := buildStableRelease(&updater.Release{}, rel.TagName, rel.Name, rel.Body, rel.HTMLURL, publishedAt, filename, sizeOf)
		if berr != nil {
			g.lg.Debug(g.t("updater_stable_build_skipped", "Tag", rel.TagName, "Err", berr.Error()))
			return nil, fmt.Errorf("%s", g.t("updater_err_build_release", map[string]any{"Err": berr.Error()}))
		}
		hash, ok := g.fetchChecksum(ctx, ghDownloadURL, g.repo, out, g.checksumFile)
		if !ok {
			g.lg.Warn(g.t("updater_checksum_fetch_failed"), "tag", rel.TagName)
			return nil, fmt.Errorf("%s", g.t("updater_err_stable_checksum_unavailable"))
		}
		out.Verification = &updater.Verification{DigestAlgo: "sha256", Digest: hash}
		g.lg.Info(g.t("updater_stable_ready", "Tag", rel.TagName, "Asset", filename))
		return out, nil
	}

	out := &updater.Release{}
	out, err = buildNightlyRelease(g.buildTime, g.gitCommit, out, rel.TagName, rel.Name, rel.Body, rel.HTMLURL, publishedAt, filename, sizeOf, rel.TargetCommitish)
	if err != nil {
		g.lg.Debug(g.t("updater_nightly_build_skipped"), "reason", err)
		return nil, fmt.Errorf("%s", g.t("updater_err_build_release", map[string]any{"Err": err.Error()}))
	}

	// Pre-releases additionally download the gitCommit / buildTime files and decide from
	// their content.
	remoteCommit, okCommit := fetchGitCommitFile(ctx, g.lg, g.client, ghDownloadURL, g.repo, out, g.gitCommitFile, "", g.fileRequest)
	remoteBuildTime, okTime := fetchBuildTimeFile(ctx, g.lg, g.client, ghDownloadURL, g.repo, out, g.buildTimeFile, "", g.fileRequest)
	if !okCommit || !okTime {
		g.lg.Warn(g.t("updater_err_prerelease_meta_missing"), "Tag", rel.TagName, "Commit", okCommit, "BuildTime", okTime)
		return nil, fmt.Errorf("%s", g.t("updater_err_prerelease_meta_missing"))
	}
	// Compare gitCommit first: equal (short hash / full hash prefix matching) means no
	// update (nil,nil = up-to-date).
	if commitEqual(g.gitCommit, remoteCommit) {
		g.lg.Debug(g.t("updater_err_nightly_same_commit", "Commit", remoteCommit))
		return nil, nil
	}
	// gitCommit differs: compare buildTime; updatable only when local < remote.
	if g.buildTime.IsZero() {
		g.lg.Debug(g.t("updater_nightly_no_local_build_time"))
		return nil, fmt.Errorf("%s", g.t("updater_err_local_build_time_empty"))
	}
	if !g.buildTime.Before(remoteBuildTime) {
		g.lg.Debug(g.t("updater_nightly_skipped", "PublishedAt", g.buildTime.Format(time.RFC3339), "BuildTime", remoteBuildTime.Format(time.RFC3339)))
		g.lg.Debug(g.t("updater_err_nightly_not_newer"))
		return nil, nil
	}

	hash, ok := g.fetchChecksum(ctx, ghDownloadURL, g.repo, out, g.checksumFile)
	if !ok {
		g.lg.Warn(g.t("updater_checksum_fetch_failed"))
		return nil, fmt.Errorf("%s", g.t("updater_err_nightly_checksum_unavailable"))
	}
	out.Verification = &updater.Verification{DigestAlgo: "sha256", Digest: hash}
	g.lg.Info(g.t("updater_nightly_ready", "Tag", rel.TagName, "Platform", req.Platform, "Arch", req.Arch))
	return out, nil
}

// checkStable checks GitHub stable updates: fetches the latest release and compares
// versions.
func (g *githubProvider) checkStable(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	releaseURL := strings.ReplaceAll(ghReleaseLatest, "{repo}", g.repo)
	httpReq, err := g.apiRequest(ctx, releaseURL)
	if err != nil {
		return nil, fmt.Errorf("%s", g.t("updater_err_request_create", map[string]any{"Err": err.Error()}))
	}
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s", g.t("updater_err_request_failed", map[string]any{"Err": err.Error()}))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		g.lg.Warn(g.t("updater_warn_unauthorized"))
		return nil, fmt.Errorf("%s", g.t("updater_err_unauthorized"))
	}
	if resp.StatusCode == http.StatusNotFound {
		// A private repo (anonymous callers get 404, not 401) or a repo with no published release
		// yet: nothing to offer. That is "no update", not a failure worth a warning or a dialog
		// (issue #178).
		g.lg.Debug("no releases visible for the update repo (HTTP 404); treating as up to date", "repo", g.repo)
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		g.lg.Warn(g.t("updater_warn_api_error", "Status", resp.StatusCode, "Body", string(body)))
		return nil, fmt.Errorf("%s", g.t("updater_err_api", map[string]any{"Status": resp.StatusCode, "Body": string(body)}))
	}
	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		g.lg.Warn(g.t("updater_err_decode", "Err", err.Error()))
		return nil, fmt.Errorf("%s", g.t("updater_err_decode", map[string]any{"Err": err.Error()}))
	}
	if rel.Draft {
		g.lg.Warn(g.t("updater_stable_no_asset", "Tag", rel.TagName, "Platform", req.Platform, "Arch", req.Arch))
		return nil, fmt.Errorf("%s", g.t("updater_err_no_stable_release"))
	}
	tag := strings.TrimPrefix(rel.TagName, "v")
	if req.CurrentVersion != "" && !isNewer(tag, req.CurrentVersion) {
		g.lg.Debug(g.t("updater_stable_not_newer", "Tag", tag))
		// latest not newer than current = up to date (nil,nil = up-to-date, not a provider
		// failure).
		return nil, nil
	}
	publishedAt, perr := time.Parse(time.RFC3339, rel.PublishedAt)
	if perr != nil {
		g.lg.Debug(g.t("updater_stable_time_parse_failed", "Tag", rel.TagName, "Err", perr.Error()))
		publishedAt = time.Time{}
	}
	assets := githubAssetsToReleaseAssets(rel.Assets)
	idx := g.assetMatcher(req, assets)
	if idx < 0 || idx >= len(assets) {
		g.lg.Warn(g.t("updater_stable_no_asset", "Tag", rel.TagName, "Platform", req.Platform, "Arch", req.Arch))
		return nil, fmt.Errorf("%s", g.t("updater_err_no_matching_stable_asset"))
	}
	filename := rel.Assets[idx].Name
	sizeOf := rel.Assets[idx].Size
	out, berr := buildStableRelease(&updater.Release{}, rel.TagName, rel.Name, rel.Body, rel.HTMLURL, publishedAt, filename, sizeOf)
	if berr != nil {
		g.lg.Debug(g.t("updater_stable_build_skipped", "Tag", rel.TagName, "Err", berr.Error()))
		return nil, fmt.Errorf("%s", g.t("updater_err_build_release", map[string]any{"Err": berr.Error()}))
	}
	hash, ok := g.fetchChecksum(ctx, ghDownloadURL, g.repo, out, g.checksumFile)
	if !ok {
		g.lg.Warn(g.t("updater_checksum_fetch_failed"), "tag", rel.TagName)
		return nil, fmt.Errorf("%s", g.t("updater_err_stable_checksum_unavailable"))
	}
	out.Verification = &updater.Verification{DigestAlgo: "sha256", Digest: hash}
	g.lg.Info(g.t("updater_stable_ready", "Tag", rel.TagName, "Asset", filename))
	return out, nil
}

// githubAssetsToReleaseAssets normalizes GitHub release assets into the official
// github.ReleaseAsset so the official AssetMatcher applies directly.
func githubAssetsToReleaseAssets(assets []githubAsset) []github.ReleaseAsset {
	out := make([]github.ReleaseAsset, 0, len(assets))
	for _, a := range assets {
		out = append(out, github.ReleaseAsset{
			Name: a.Name,
			Size: a.Size,
		})
	}
	return out
}
