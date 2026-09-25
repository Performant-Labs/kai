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

// cnbProvider implements the updater.Provider for the CNB source (prerelease + stable).
type cnbProvider struct {
	client        *http.Client        // HTTP client (injected from the package-global GetClient at construction)
	lg            *slog.Logger        // logger (injected from the package-global GetLogger at construction)
	repo          string              // CNB repo path
	assetMatcher  github.AssetMatcher // asset matcher (the official type)
	checksumFile  string              // checksum file name, verifies artifact integrity after download
	gitCommitFile string              // pre-release git commit file name
	buildTimeFile string              // pre-release build time file name
	token         string              // CNB access token (Bearer)
	buildTime     time.Time           // this machine's build time
	gitCommit     string              // this machine's git commit
	prerelease    bool                // whether the nightly (pre-release) channel is subscribed
}

// t is a convenience method rendering i18n copy with the current package-global locale.
func (c *cnbProvider) t(key string, data ...any) string { return T(key, data...) }

// apiRequest calls a CNB API endpoint (list/detail) with Accept: application/json and
// Bearer auth (when token is non-empty).
func (c *cnbProvider) apiRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	return req, nil
}

// fileRequest downloads a CNB file (/releases/download/...) with Accept:
// application/octet-stream.
// No Authorization header: the CNB file endpoint authorizes via the time-limited signed URL
// after a 302 redirect; sending Bearer actually returns 400.
func (c *cnbProvider) fileRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")
	return req, nil
}

// Name implements the updater.Provider interface, returning "cnb".
func (c *cnbProvider) Name() string { return string(SourceCNB) }

// Check implements the updater.Provider interface.
// Prerelease off (prerelease=false): only the stable channel is checked (checkStable excludes
// pre-releases, comparing version numbers).
// Prerelease on (prerelease=true): the newest published entry becomes the candidate, judged by
// its own type —
// a pre-release (tag contains "-") compares downloaded gitCommit/buildTime; a stable version
// compares version numbers via isNewer.
// I.e. "whatever type the latest version is, that's how it's judged" — no separate stable
// second pass, no picking by publish time.
func (c *cnbProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	c.lg.Debug(c.t("updater_start"))
	// Prerelease off: only the stable channel (checkStable already excludes pre-releases).
	if !c.prerelease {
		rel, err := c.checkStable(ctx, req)
		if err != nil {
			return nil, err
		}
		c.lg.Info(c.t("updater_check_done"))
		return rel, nil
	}

	// Prerelease on: take the newest entry (pre-release or stable) and judge by its own
	// type.
	c.lg.Debug(c.t("updater_check_nightly_channel"))
	return c.checkPrerelease(ctx, req)
}

// Download implements the updater.Provider interface, reusing the shared download logic.
func (c *cnbProvider) Download(ctx context.Context, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	return downloadRelease(ctx, c.lg, c.client, cnbDownloadURL, c.repo, rel, dst, onProgress, "", c.fileRequest)
}

// fetchChecksum fetches and parses this source’s checksum sidecar, reusing the shared
// logic.
func (c *cnbProvider) fetchChecksum(ctx context.Context, downloadURLTpl, repo string, rel *updater.Release, sidecar, directURL string) ([]byte, bool) {
	return fetchReleaseChecksum(ctx, c.lg, c.client, downloadURLTpl, repo, rel, sidecar, directURL, c.fileRequest)
}

// checkNightly checks CNB nightly (pre-release) updates: takes the newest tag and compares
// time and commit.
func (c *cnbProvider) checkPrerelease(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	if c.token == "" {
		c.lg.Debug(c.t("updater_nightly_no_token"))
		return nil, fmt.Errorf("%s", c.t("updater_err_no_token"))
	}

	tagsURL := strings.ReplaceAll(cnbReleaseTagList, "{repo}", c.repo)
	httpReq, err := c.apiRequest(ctx, tagsURL)
	if err != nil {
		return nil, fmt.Errorf("%s", c.t("updater_err_request_create", map[string]any{"Err": err.Error()}))
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		c.lg.Warn(c.t("updater_nightly_unreachable", "Error", err.Error()))
		return nil, fmt.Errorf("%s", c.t("updater_err_request_failed", map[string]any{"Err": err.Error()}))
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized {
		c.lg.Warn(c.t("updater_nightly_unauthorized"))
		return nil, fmt.Errorf("%s", c.t("updater_err_unauthorized"))
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		c.lg.Warn(c.t("updater_warn_api_error", "Status", resp.StatusCode, "Body", string(body)))
		return nil, fmt.Errorf("%s", c.t("updater_err_api", map[string]any{"Status": resp.StatusCode, "Body": string(body)}))
	}
	var tagList []cnbReleaseListItem
	if err := json.NewDecoder(resp.Body).Decode(&tagList); err != nil {
		c.lg.Warn(c.t("updater_err_decode", "Err", err.Error()))
		return nil, fmt.Errorf("%s", c.t("updater_err_decode", map[string]any{"Err": err.Error()}))
	}
	if len(tagList) == 0 {
		c.lg.Warn(c.t("updater_warn_no_tags"))
		return nil, fmt.Errorf("%s", c.t("updater_err_no_tags"))
	}
	// Prerelease on: take the newest published entry (pre-release or stable) and judge by
	// its own type.
	sortReleasesByPublishedAt(tagList)
	newest := tagList[0]
	tag := strings.TrimPrefix(newest.Name, "v")
	if tag == "" {
		tag = newest.Name
	}
	// CNB provides a prerelease boolean directly; use it to distinguish pre-releases.
	isPre := newest.Prerelease

	releaseURL := strings.ReplaceAll(strings.ReplaceAll(cnbReleaseTagURL, "{repo}", c.repo), "{tag}", newest.TagName)
	relReq, err := c.apiRequest(ctx, releaseURL)
	if err != nil {
		return nil, fmt.Errorf("%s", c.t("updater_err_request_create", map[string]any{"Err": err.Error()}))
	}
	relResp, err := c.client.Do(relReq)
	if err != nil {
		return nil, fmt.Errorf("%s", c.t("updater_err_request_failed", map[string]any{"Err": err.Error()}))
	}
	defer relResp.Body.Close()
	if relResp.StatusCode != http.StatusOK {
		c.lg.Warn(c.t("updater_warn_get_release_failed", "Tag", tag))
		return nil, fmt.Errorf("%s", c.t("updater_err_get_release_failed", map[string]any{"Status": relResp.StatusCode}))
	}
	var tagDetail cnbReleaseTagDetail
	if err := json.NewDecoder(relResp.Body).Decode(&tagDetail); err != nil {
		return nil, fmt.Errorf("%s", c.t("updater_err_decode", map[string]any{"Err": err.Error()}))
	}

	publishedAt, err := time.Parse(time.RFC3339, newest.PublishedAt)
	if err != nil {
		c.lg.Debug(c.t("updater_nightly_no_build_time"))
		return nil, fmt.Errorf("%s", c.t("updater_err_nightly_no_published_at"))
	}

	// Get assets (first find the upgrade artifact file name via the matcher, for later
	// download/verification).
	assets := cnbReleaseAssetsToReleaseAssets(tagDetail.Assets)
	idx := c.assetMatcher(req, assets)
	if idx < 0 || idx >= len(assets) {
		// The newest entry’s upgrade artifact doesn’t match this machine’s platform/arch:
		// the candidate doesn’t apply — treat as up-to-date (not a provider failure).
		c.lg.Debug(c.t("updater_nightly_no_asset", "Tag", tag, "Platform", req.Platform, "Arch", req.Arch))
		return nil, nil
	}
	filename := tagDetail.Assets[idx].Name

	// Stable (the newest entry is not a pre-release): compare version numbers directly;
	// no update needed = up-to-date.
	if !isPre {
		if req.CurrentVersion != "" && !isNewer(tag, req.CurrentVersion) {
			c.lg.Debug(c.t("updater_stable_not_newer", "Tag", tag))
			return nil, nil
		}
		out, berr := buildStableRelease(&updater.Release{}, newest.TagName, tag, tagDetail.Body, cnbReleasePageURL(c.repo, tagDetail.TagName), publishedAt, filename, 0)
		if berr != nil {
			c.lg.Debug(c.t("updater_stable_build_skipped", "Tag", tag, "Err", berr.Error()))
			return nil, fmt.Errorf("%s", c.t("updater_err_build_release", map[string]any{"Err": berr.Error()}))
		}
		hash, ok := c.fetchChecksum(ctx, cnbDownloadURL, c.repo, out, c.checksumFile, "")
		if !ok {
			c.lg.Warn(c.t("updater_checksum_fetch_failed"))
			return nil, fmt.Errorf("%s", c.t("updater_err_nightly_checksum_unavailable"))
		}
		out.Verification = &updater.Verification{DigestAlgo: "sha256", Digest: hash}
		c.lg.Info(c.t("updater_stable_ready", "Tag", tag, "Asset", filename))
		return out, nil
	}

	// Pre-release (the newest entry is one): download gitCommit / buildTime and decide from
	// their content.
	out := &updater.Release{}
	out, err = buildNightlyRelease(c.buildTime, c.gitCommit, out, newest.TagName, tag, tagDetail.Body, cnbReleasePageURL(c.repo, tagDetail.TagName), publishedAt, filename, 0, "")
	if err != nil {
		c.lg.Debug(c.t("updater_nightly_build_skipped"), "reason", err)
		return nil, fmt.Errorf("%s", c.t("updater_err_build_release", map[string]any{"Err": err.Error()}))
	}

	// Pre-releases additionally download the gitCommit / buildTime files and decide from
	// their content.
	remoteCommit, okCommit := fetchGitCommitFile(ctx, c.lg, c.client, cnbDownloadURL, c.repo, out, c.gitCommitFile, "", c.fileRequest)
	remoteBuildTime, okTime := fetchBuildTimeFile(ctx, c.lg, c.client, cnbDownloadURL, c.repo, out, c.buildTimeFile, "", c.fileRequest)
	if !okCommit || !okTime {
		c.lg.Warn(c.t("updater_err_prerelease_meta_missing"), "Tag", tag, "Commit", okCommit, "BuildTime", okTime)
		return nil, fmt.Errorf("%s", c.t("updater_err_prerelease_meta_missing"))
	}
	// Compare gitCommit first: equal (short hash / full hash prefix matching) means no
	// update (nil,nil = up-to-date).
	if commitEqual(c.gitCommit, remoteCommit) {
		c.lg.Debug(c.t("updater_err_nightly_same_commit", "Commit", remoteCommit))
		return nil, nil
	}
	// gitCommit differs: compare buildTime; updatable only when local < remote.
	if c.buildTime.IsZero() {
		c.lg.Debug(c.t("updater_nightly_no_local_build_time"))
		return nil, fmt.Errorf("%s", c.t("updater_err_local_build_time_empty"))
	}
	if !c.buildTime.Before(remoteBuildTime) {
		c.lg.Debug(c.t("updater_nightly_skipped", "PublishedAt", c.buildTime.Format(time.RFC3339), "BuildTime", remoteBuildTime.Format(time.RFC3339)))
		c.lg.Debug(c.t("updater_err_nightly_not_newer"))
		return nil, nil
	}

	hash, ok := c.fetchChecksum(ctx, cnbDownloadURL, c.repo, out, c.checksumFile, "")
	if !ok {
		c.lg.Warn(c.t("updater_checksum_fetch_failed"))
		return nil, fmt.Errorf("%s", c.t("updater_err_nightly_checksum_unavailable"))
	}
	out.Verification = &updater.Verification{DigestAlgo: "sha256", Digest: hash}
	c.lg.Info(c.t("updater_nightly_ready", "Tag", tag, "Platform", req.Platform, "Arch", req.Arch))
	return out, nil
}

// checkStable checks CNB stable updates: walks the tags for a newer non-prerelease
// version.
func (c *cnbProvider) checkStable(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	if c.token == "" {
		c.lg.Debug(c.t("updater_nightly_no_token"))
		return nil, fmt.Errorf("%s", c.t("updater_err_no_token"))
	}
	tagsURL := strings.ReplaceAll(cnbReleaseTagList, "{repo}", c.repo)
	httpReq, err := c.apiRequest(ctx, tagsURL)
	if err != nil {
		return nil, fmt.Errorf("%s", c.t("updater_err_request_create", map[string]any{"Err": err.Error()}))
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("%s", c.t("updater_err_request_failed", map[string]any{"Err": err.Error()}))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		c.lg.Warn(c.t("updater_warn_api_error", "Status", resp.StatusCode, "Body", string(body)))
		return nil, fmt.Errorf("%s", c.t("updater_err_api", map[string]any{"Status": resp.StatusCode, "Body": string(body)}))
	}
	var tagList []cnbReleaseListItem
	if err := json.NewDecoder(resp.Body).Decode(&tagList); err != nil {
		c.lg.Warn(c.t("updater_err_decode", "Err", err.Error()))
		return nil, fmt.Errorf("%s", c.t("updater_err_decode", map[string]any{"Err": err.Error()}))
	}
	if len(tagList) == 0 {
		c.lg.Warn(c.t("updater_warn_no_tags"))
		return nil, fmt.Errorf("%s", c.t("updater_err_no_stable_tags"))
	}
	sortReleasesByPublishedAt(tagList)

	for _, item := range tagList {
		tag := strings.TrimPrefix(item.Name, "v") // judgment/display keeps the HEAD-era logic (strip v from the release title)
		if tag == "" {
			tag = item.Name
		}
		// Skip pre-releases and drafts, keeping stable versions only
		if item.Prerelease || item.Draft {
			continue
		}
		if req.CurrentVersion != "" && !isNewer(tag, req.CurrentVersion) {
			c.lg.Debug(c.t("updater_stable_not_newer", "Tag", tag))
			continue
		}
		releaseURL := strings.ReplaceAll(strings.ReplaceAll(cnbReleaseTagURL, "{repo}", c.repo), "{tag}", item.TagName)
		relReq, err := c.apiRequest(ctx, releaseURL)
		if err != nil {
			c.lg.Debug(c.t("updater_stable_req_failed", "Tag", tag, "Err", err.Error()))
			continue
		}
		relResp, err := c.client.Do(relReq)
		if err != nil {
			c.lg.Debug(c.t("updater_stable_req_failed", "Tag", tag, "Err", err.Error()))
			continue
		}
		var tagDetail cnbReleaseTagDetail
		jerr := json.NewDecoder(relResp.Body).Decode(&tagDetail)
		relResp.Body.Close()
		if jerr != nil {
			c.lg.Debug(c.t("updater_stable_decode_failed", "Tag", tag, "Err", jerr.Error()))
			continue
		}
		publishedAt, perr := time.Parse(time.RFC3339, item.PublishedAt)
		if perr != nil {
			c.lg.Debug(c.t("updater_stable_time_parse_failed", "Tag", tag, "Err", perr.Error()))
			publishedAt = time.Time{}
		}
		assets := cnbReleaseAssetsToReleaseAssets(tagDetail.Assets)
		idx := c.assetMatcher(req, assets)
		if idx < 0 || idx >= len(assets) {
			c.lg.Debug(c.t("updater_stable_no_matching_skip", "Tag", tag))
			continue
		}
		filename := tagDetail.Assets[idx].Name
		out, berr := buildStableRelease(&updater.Release{}, item.TagName, tag, tagDetail.Body, cnbReleasePageURL(c.repo, tagDetail.TagName), publishedAt, filename, 0)
		if berr != nil {
			c.lg.Debug(c.t("updater_stable_build_skipped", "Tag", tag, "Err", berr.Error()))
			continue
		}
		hash, ok := c.fetchChecksum(ctx, cnbDownloadURL, c.repo, out, c.checksumFile, "")
		if !ok {
			c.lg.Warn(c.t("updater_checksum_fetch_failed"), "tag", tag)
			continue
		}
		out.Verification = &updater.Verification{DigestAlgo: "sha256", Digest: hash}
		c.lg.Info(c.t("updater_stable_ready", "Tag", tag, "Asset", filename))
		return out, nil
	}
	c.lg.Debug(c.t("updater_stable_no_asset", "Tag", "", "Platform", req.Platform, "Arch", req.Arch))
	// No newer stable after the walk = up to date (nil,nil = up-to-date). With prerelease on,
	// this function is one candidate path;
	// whether to use it is arbitrated by the Check layer together with the prerelease
	// candidate.
	c.lg.Debug(c.t("updater_err_no_stable_matched"))
	return nil, nil
}

// cnbReleaseAssetsToReleaseAssets normalizes CNB release assets into the official
// github.ReleaseAsset so the official AssetMatcher applies directly. CNB's Size is already
// int64 — no parsing.
// Note: CNB download URLs are uniformly built from templates (base https://cnb.cool, see
// cnbDownloadURL),
// not from the response's browser_download_url, so only Name/Size are mapped here.
func cnbReleaseAssetsToReleaseAssets(atts []cnbReleaseAsset) []github.ReleaseAsset {
	out := make([]github.ReleaseAsset, 0, len(atts))
	for _, a := range atts {
		out = append(out, github.ReleaseAsset{
			Name: a.Name,
			Size: a.Size,
		})
	}
	return out
}

// cnbReleasePageURL builds the CNB release page URL (for Metadata display/links).
// Format: https://cnb.cool/{repo}/-/releases/tags/{tag}
func cnbReleasePageURL(repo, tag string) string {
	if repo == "" || tag == "" {
		return ""
	}
	return "https://cnb.cool/" + repo + "/-/releases/tags/" + tag
}
