package wails_updater_providers

import (
	"bufio"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// gitCommitRe validates GIT_COMMIT file content: a single line hex commit hash (short or
// full 40 chars).
// Both CI and local use `git rev-parse --short HEAD` (7-char short hash by default) and
// compare by string equality,
// so this only validates "a legal hex hash" — 40 chars not enforced.
var gitCommitRe = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// commitEqual decides whether the local and remote git commits point at the same commit.
// One side may be a short hash (e.g. CI's `git rev-parse --short HEAD`, 7 chars by default)
// and the other a full 40-char hash, so "prefix matching" is used instead of plain equality:
// if the shorter is a prefix of the longer (or they are identical), it is the same commit —
// avoiding a false "different" verdict that would force an update.
func commitEqual(local, remote string) bool {
	if local == "" || remote == "" {
		return false
	}
	if len(local) <= len(remote) {
		return strings.HasPrefix(remote, local)
	}
	return strings.HasPrefix(local, remote)
}

// sha256Re validates a hash picked from the SHA256SUMS sidecar: 64 hex chars.
var sha256Re = regexp.MustCompile(`^[0-9a-fA-F]{64}$`)

// Download URL templates: {repo} is replaced at runtime with the actual repo path (e.g.
// example-org/example-repo),
// {tag} with the version (tag_name), and {file} with the asset file name.
// CNB's public download base is https://cnb.cool (same origin as the response's
// browser_download_url; no auth needed);
// GitHub's public download base is https://github.com. Both are built from templates.
const (
	cnbDownloadURL    = "https://cnb.cool/{repo}/-/releases/download/{tag}/{file}"
	ghDownloadURL     = "https://github.com/{repo}/releases/download/{tag}/{file}"
	cnbReleaseTagList = "https://api.cnb.cool/{repo}/-/releases?page=1&page_size=20"
	cnbReleaseTagURL  = "https://api.cnb.cool/{repo}/-/releases/tags/{tag}"
	ghReleaseLatest   = "https://api.github.com/repos/{repo}/releases/latest"
	ghReleasesList    = "https://api.github.com/repos/{repo}/releases?per_page=100"
)

// buildURL renders a download URL from the template: {tag} -> tag, {file} -> file.
func buildURL(tpl, tag, file string) string {
	u := strings.ReplaceAll(tpl, "{tag}", tag)
	u = strings.ReplaceAll(u, "{file}", file)
	return u
}

// downloadRelease is the shared download logic: downloads the release's upgrade artifact to
// dst, reporting progress via onProgress.
// Prefers directURL (the asset's real download URL, e.g. CNB's browser_download_url);
// when directURL is empty it falls back to template-joining downloadURLTpl + repo + version +
// filename (the GitHub path).
// The caller pre-builds the request via newReq before issuing (CNB attaches Accept etc.;
// GitHub uses a plain GET),
// and this function only executes the request, reads the body and reports progress — it never
// news a request itself.
func downloadRelease(ctx context.Context, lg *slog.Logger, client *http.Client, downloadURLTpl, repo string, rel *updater.Release, dst io.Writer, onProgress func(written, total int64), directURL string, newReq func(ctx context.Context, url string) (*http.Request, error)) error {
	filename := rel.Artifact.Filename
	if filename == "" {
		return fmt.Errorf("%s", T("updater_err_artifact_filename_empty"))
	}
	url := directURL
	if url == "" {
		url = buildURL(strings.ReplaceAll(downloadURLTpl, "{repo}", repo), rel.Version, filename)
	}

	req, err := newReq(ctx, url)
	if err != nil {
		return fmt.Errorf("%s: %w", T("updater_err_download_request"), err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", T("updater_err_download_conn"), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, 512))
		if rerr != nil {
			lg.Warn(T("updater_err_download_read_body", "Err", rerr.Error()))
		}
		return fmt.Errorf("%s", T("updater_err_download_failed", map[string]any{"Status": resp.StatusCode, "Body": string(body)}))
	}

	total := resp.ContentLength
	var written int64
	reader := bufio.NewReader(resp.Body)
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-ctx.Done():
			lg.Debug(T("updater_download_canceled"), "url", url)
			return ctx.Err()
		default:
		}
		n, rerr := reader.Read(buf)
		if n > 0 {
			wn, werr := dst.Write(buf[:n])
			written += int64(wn)
			if werr != nil {
				return fmt.Errorf("%s: %w", T("updater_err_download_io"), werr)
			}
			if onProgress != nil {
				onProgress(written, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("%s: %w", T("updater_err_download_io"), rerr)
		}
	}
	return nil
}

// fetchReleaseChecksum is the shared checksum fetch logic: downloads the SHA256SUMS sidecar
// and parses out
// the target file's hash. Returns (hash bytes, found). When directURL is non-empty it is
// preferred as the sidecar's real address,
// otherwise a template join is used.
func fetchReleaseChecksum(ctx context.Context, lg *slog.Logger, client *http.Client, downloadURLTpl, repo string, rel *updater.Release, sidecar, directURL string, newReq func(ctx context.Context, url string) (*http.Request, error)) ([]byte, bool) {
	checksumURL := directURL
	if checksumURL == "" {
		checksumURL = buildURL(strings.ReplaceAll(downloadURLTpl, "{repo}", repo), rel.Version, sidecar)
	}
	lg.Debug(T("updater_checksum_download", "URL", checksumURL))

	req, err := newReq(ctx, checksumURL)
	if err != nil {
		lg.Warn(T("updater_checksum_fetch_failed"), "error", err)
		return nil, false
	}
	resp, err := client.Do(req)
	if err != nil {
		lg.Warn(T("updater_checksum_source_failed", "URL", checksumURL, "Error", err.Error()))
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		lg.Warn(T("updater_checksum_no_url", "Tag", rel.Version))
		return nil, false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		lg.Warn(T("updater_checksum_fetch_failed"), "error", err)
		return nil, false
	}

	target := rel.Artifact.Filename
	for line := range strings.SplitSeq(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hash := fields[0]
		name := strings.TrimSpace(strings.TrimPrefix(line, hash))
		if strings.Contains(name, " ") {
			name = strings.TrimSpace(strings.SplitN(name, " ", 2)[1])
		}
		if name == target {
			if !sha256Re.MatchString(hash) {
				lg.Warn(T("updater_checksum_invalid", "URL", checksumURL, "File", target, "Hash", hash))
				return nil, false
			}
			raw, err := hex.DecodeString(hash)
			if err != nil {
				lg.Warn(T("updater_checksum_invalid", "URL", checksumURL, "File", target, "Hash", hash))
				return nil, false
			}
			return raw, true
		}
	}

	lg.Warn(T("updater_checksum_parse_failed", "URL", checksumURL, "Target", target))
	return nil, false
}

// fetchGitCommitFile downloads the pre-release's git commit file (GIT_COMMIT),
// validates it as a single-line 40-char hex hash and returns it. Returns ("", false) when the
// file is missing, the download fails, or the content is invalid.
func fetchGitCommitFile(ctx context.Context, lg *slog.Logger, client *http.Client, downloadURLTpl, repo string, rel *updater.Release, filename, directURL string, newReq func(ctx context.Context, url string) (*http.Request, error)) (string, bool) {
	url := directURL
	if url == "" {
		url = buildURL(strings.ReplaceAll(downloadURLTpl, "{repo}", repo), rel.Version, filename)
	}
	lg.Debug(T("updater_gitcommit_download", "URL", url))

	req, err := newReq(ctx, url)
	if err != nil {
		lg.Warn(T("updater_gitcommit_fetch_failed"), "error", err)
		return "", false
	}
	resp, err := client.Do(req)
	if err != nil {
		lg.Warn(T("updater_checksum_source_failed", "URL", url, "Error", err.Error()))
		return "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		lg.Warn(T("updater_gitcommit_no_url", "Tag", rel.Version, "File", filename))
		return "", false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		lg.Warn(T("updater_gitcommit_fetch_failed"), "error", err)
		return "", false
	}
	commit := strings.TrimSpace(string(body))
	if !gitCommitRe.MatchString(commit) {
		lg.Warn(T("updater_gitcommit_invalid", "URL", url, "Content", commit))
		return "", false
	}
	return commit, true
}

// fetchBuildTimeFile downloads the pre-release's build time file (BUILD_TIME),
// parses it as an RFC3339 time and returns it. Returns (time.Time{}, false) when the file is
// missing, the download fails, or parsing fails.
func fetchBuildTimeFile(ctx context.Context, lg *slog.Logger, client *http.Client, downloadURLTpl, repo string, rel *updater.Release, filename, directURL string, newReq func(ctx context.Context, url string) (*http.Request, error)) (time.Time, bool) {
	url := directURL
	if url == "" {
		url = buildURL(strings.ReplaceAll(downloadURLTpl, "{repo}", repo), rel.Version, filename)
	}
	lg.Debug(T("updater_buildtime_download", "URL", url))

	req, err := newReq(ctx, url)
	if err != nil {
		lg.Warn(T("updater_buildtime_fetch_failed"), "error", err)
		return time.Time{}, false
	}
	resp, err := client.Do(req)
	if err != nil {
		lg.Warn(T("updater_checksum_source_failed", "URL", url, "Error", err.Error()))
		return time.Time{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		lg.Warn(T("updater_buildtime_no_url", "Tag", rel.Version, "File", filename))
		return time.Time{}, false
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		lg.Warn(T("updater_buildtime_fetch_failed"), "error", err)
		return time.Time{}, false
	}
	raw := strings.TrimSpace(string(body))
	parsed, perr := time.Parse(time.RFC3339, raw)
	if perr != nil {
		lg.Warn(T("updater_buildtime_parse_failed", "URL", url, "Content", raw, "Error", perr.Error()))
		return time.Time{}, false
	}
	return parsed, true
}

// isNewer compares version strings: any difference between remote and current counts as an
// update.
func isNewer(remote, current string) bool {
	if remote == "" {
		return false
	}
	if current == "" {
		return true
	}
	return remote != current
}
