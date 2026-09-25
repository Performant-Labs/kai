package wails_updater_providers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// Options controls updater behavior; everything is injected by the caller.
// Note: language/Locale, theme/Theme, primary source/Source, and the update window
// name/size/event names have been promoted to "library-global config" (see the var block and
// Set/Get package functions below) and no longer appear in this struct;
// the library reads the package globals directly, so callers don't thread parameters through
// every layer — at runtime just call SetXxx to switch.
type Options struct {
	// CnbRepo is the CNB repo path (e.g. your-org/your-repo). Required when Source is
	// SourceCNB/SourceAuto and CNB is selected, otherwise NewMirrorProvider errors.
	CnbRepo string
	// GithubRepo is the GitHub repo path (e.g. your-org/your-repo). Required when Source is
	// SourceGithub/SourceAuto and GitHub is selected, otherwise NewMirrorProvider errors.
	GithubRepo string
	// GithubToken is the GitHub access token (needed for private repos or rate limits).
	GithubToken string
	// CnbToken is the CNB access token (needed for private repos or rate limits).
	CnbToken string
	// BuildTime is this machine's build time, used for nightly version time comparison
	// (upgrade only when the remote publish time is newer).
	// Example: time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	// or time.Parse(time.RFC3339, "2026-08-15T12:00:00Z").
	BuildTime time.Time
	// GitCommit is this machine's build git commit, used to skip nightlies with the same
	// commit.
	GitCommit string
	// Prerelease is whether pre-release versions are allowed.
	Prerelease bool
	// AssetMatcher is a custom asset matcher (optional). The type is the official
	// github.AssetMatcher:
	// func(req updater.CheckRequest, assets []github.ReleaseAsset) int, returning
	// the index of the matched asset, or -1 for no match. When nil, the official
	// github.DefaultAssetMatcher is used automatically (matches by platform/arch, skips
	// signature and checksum sidecar files).
	// The official type is reused directly, so a caller with a matcher from the official
	// github provider can migrate with zero changes.
	// The library provides NewUpdaterAssetMatcher in matcher.go (matches only the updater-
	// prefixed upgrade assets);
	// the language is taken automatically from the package global GetLocale(); to enable it,
	// pass AssetMatcher: kupdater.NewUpdaterAssetMatcher explicitly.
	// Note: NewUpdaterAssetMatcher reads the package global GetLocale() on every match, so
	// calling SetLocale at runtime switches the matcher log language dynamically (e.g. main
	// calls kupdater.SetLocale when the host language changes).
	AssetMatcher github.AssetMatcher
	// ChecksumFile is the checksum file name (verifies artifact integrity after download).
	// Default "SHA256SUMS".
	// Content format: one line per entry, "<sha256 hex hash>  <asset file name>", e.g.:
	//   a1b2c3d4...  kai-darwin-amd64.tar.gz
	//   9f8e7d6c...  kai-windows-amd64.zip
	ChecksumFile string
	// GitCommitFile is the git commit file name attached to pre-release versions.
	// Split out of the original SHA256SUMS into its own file, making it easy to verify the
	// commit a pre-release corresponds to.
	// Falls back to "GIT_COMMIT" when empty.
	// Content format: a single line with the bare commit hash (40 hex chars), e.g.:
	//   9f3c1a2b7e4d5c6a8b9c0d1e2f3a4b5c6d7e8f90
	GitCommitFile string
	// BuildTimeFile is the build time file name attached to pre-release versions.
	// Split out of the original SHA256SUMS into its own file, making it easy to verify a
	// pre-release's build time.
	// Falls back to "BUILD_TIME" when empty.
	// Content format: a single RFC3339 time (UTC), e.g.:
	//   2026-08-15T12:00:00Z
	BuildTimeFile string
}

// AssetMatcherOrDefault returns the caller's custom AssetMatcher; on nil it falls back to
// the official github.DefaultAssetMatcher (matches by platform/arch, skips signature and
// checksum sidecar files).
func (o Options) AssetMatcherOrDefault() github.AssetMatcher {
	if o.AssetMatcher != nil {
		return o.AssetMatcher
	}
	return github.DefaultAssetMatcher
}

// ChecksumFileOrDefault returns the checksum file name; falls back to "SHA256SUMS" when
// empty.
func (o *Options) ChecksumFileOrDefault() string {
	if o.ChecksumFile == "" {
		return "SHA256SUMS"
	}
	return o.ChecksumFile
}

// GitCommitFileOrDefault returns the caller’s custom GitCommitFile; falls back to
// "GIT_COMMIT" when empty.
func (o Options) GitCommitFileOrDefault() string {
	if o.GitCommitFile == "" {
		return "GIT_COMMIT"
	}
	return o.GitCommitFile
}

// BuildTimeFileOrDefault returns the caller’s custom BuildTimeFile; falls back to
// "BUILD_TIME" when empty.
func (o Options) BuildTimeFileOrDefault() string {
	if o.BuildTimeFile == "" {
		return "BUILD_TIME"
	}
	return o.BuildTimeFile
}

// ===================== Library-global config (package vars) =====================
// Language/Locale, theme/Theme, primary source/Source, and the update window
// name/size/event names are all "library globals",
// read/written externally via the exported Set/Get package functions; the library internals
// (matcher, provider, window) read them
// directly — callers never stuff these into Options and thread them through layers. Runtime
// switches (following system language/appearance, the user changing source) just call SetXxx;
// no provider rebuild needed.

var (
	globalLocale      Locale = LocaleZhCN             // current library-global language; falls back to the default (zh-CN) when unset
	globalTheme       Theme  = ThemeDark              // current library-global theme; falls back to the default (dark) when unset
	globalSource      Source = SourceAuto             // current library-global primary source preference (auto picks CNB/GitHub by language)
	globalWindowName         = "updater-window"       // built-in update window name
	globalWindowW            = 520                    // built-in update window width (pixels; matches the JS side’s resizeToContent WINDOW_WIDTH so expansion doesn’t jump between 348 and 520)
	globalWindowH            = 660                    // built-in update window fixed height (pixels; Wails’ inline shim drops Events.Emit payloads so height data can’t be returned — hence fixed height + CSS filling the background)
	globalResizeEvent        = "wails:updater:resize" // HTML-to-window resize communication event name
	globalLogger             = slog.Default()         // current library-global logger; defaults to slog.Default()
	globalClient             = http.DefaultClient     // current library-global HTTP client; defaults to http.DefaultClient
)

// SetLocale sets the library-global language; empty values normalize to the default
// (en-US).
// For runtime language switches (e.g. following the system/user settings), just call this —
// no provider rebuild.
func SetLocale(locale Locale) { globalLocale = normalizeLocale(string(locale)) }

// GetLocale returns the current library-global language (falls back to the default en-US
// when unset).
func GetLocale() Locale { return normalizeLocale(string(globalLocale)) }

// SetTheme sets the library-global theme; empty values normalize to the default (light).
func SetTheme(theme Theme) { globalTheme = normalizeTheme(theme) }

// GetTheme returns the current library-global theme (falls back to the default light when
// unset).
func GetTheme() Theme { return normalizeTheme(globalTheme) }

// SetSource sets the library-global primary source preference (auto/cnb/github).
func SetSource(v Source) { globalSource = normalizeSource(v) }

// GetSource returns the current library-global primary source preference.
func GetSource() Source { return globalSource }

// SetWindowName sets the built-in update window name (usually left default; overridden in
// special cases).
func SetWindowName(name string) {
	if name != "" {
		globalWindowName = name
	}
}

// GetWindowName returns the built-in update window name.
func GetWindowName() string { return globalWindowName }

// SetWindowSize sets the built-in update window size (width/height in pixels; <=0
// ignored).
func SetWindowSize(w, h int) {
	if w > 0 {
		globalWindowW = w
	}
	if h > 0 {
		globalWindowH = h
	}
}

// GetWindowSize returns the built-in update window size (width, height).
func GetWindowSize() (int, int) { return globalWindowW, globalWindowH }

// SetResizeEvent sets the resize event name for built-in update window/HTML communication.
func SetResizeEvent(name string) {
	if name != "" {
		globalResizeEvent = name
	}
}

// GetResizeEvent returns the built-in update window’s resize event name.
func GetResizeEvent() string { return globalResizeEvent }

// SetLogger sets the library-global logger; falls back to slog.Default() on nil.
func SetLogger(lg *slog.Logger) {
	if lg != nil {
		globalLogger = lg
	} else {
		globalLogger = slog.Default()
	}
}

// GetLogger returns the current library-global logger (defaults to slog.Default()).
func GetLogger() *slog.Logger { return globalLogger }

// SetClient sets the library-global HTTP client; falls back to http.DefaultClient on nil.
func SetClient(c *http.Client) {
	if c != nil {
		globalClient = c
	} else {
		globalClient = http.DefaultClient
	}
}

// GetClient returns the current library-global HTTP client (defaults to
// http.DefaultClient).
func GetClient() *http.Client { return globalClient }

// MirrorProvider merges the CNB/GitHub dual sources, choosing the primary by the global
// source preference.
// Language/theme/source are all read from package globals (GetLocale/GetTheme/GetSource) —
// none are held as fields —
// so calling SetXxx at runtime makes subsequent Check/Download/Window follow live.
type MirrorProvider struct {
	opts    *Options // caller-injected config pointer
	cnbRepo string   // CNB repo path
	ghRepo  string   // GitHub repo path

	cnbProvider    *cnbProvider    // CNB sub-source (active when source=CNB/auto selects CNB)
	githubProvider *githubProvider // GitHub sub-source (active when source=Github/auto selects GitHub)

	buildTime time.Time // this machine's build time, for nightly time comparison
	gitCommit string    // this machine's git commit, for skipping same-commit nightlies

	cnbToken      string              // CNB access token
	githubToken   string              // GitHub access token
	prerelease    bool                // whether pre-releases (nightlies) are allowed
	assetMatcher  github.AssetMatcher // asset matcher (the official type)
	checksumFile  string              // checksum file name, verifies artifact integrity after download; default SHA256SUMS
	gitCommitFile string              // git commit file name attached to pre-releases; default GIT_COMMIT
	buildTimeFile string              // build time file name attached to pre-releases; default BUILD_TIME
}

// NewMirrorProvider builds the dual-source updater from Options.
// Repos/tokens/build info/matcher come from opts; the logger and HTTP client come from the
// package globals GetLogger/GetClient (injected by the caller via SetLogger/SetClient before
// construction).
func NewMirrorProvider(opts *Options) (*MirrorProvider, error) {
	// Construction only validates source legality (the primary is not pinned here); the
	// actual primary is computed at Check/Download
	// time from the package globals GetSource/GetLocale, so it follows runtime SetXxx
	// changes.
	if _, err := decideSource(GetLogger()); err != nil {
		return nil, err
	}

	m := &MirrorProvider{
		opts:          opts,
		cnbRepo:       opts.CnbRepo,
		ghRepo:        opts.GithubRepo,
		cnbToken:      opts.CnbToken,
		githubToken:   opts.GithubToken,
		prerelease:    opts.Prerelease,
		assetMatcher:  opts.AssetMatcherOrDefault(),
		checksumFile:  opts.ChecksumFileOrDefault(),
		gitCommitFile: opts.GitCommitFileOrDefault(),
		buildTimeFile: opts.BuildTimeFileOrDefault(),
		buildTime:     opts.BuildTime,
		gitCommit:     opts.GitCommit,
	}

	// source is only used at construction to pick which sub-source to initialize (in auto
	// mode, one is initialized by current language;
	// the other is lazily initialized on demand during Check if it gets selected).
	source, _ := decideSource(GetLogger())
	switch source {
	case SourceCNB:
		if opts.CnbRepo == "" {
			return nil, fmt.Errorf("%s", T("updater_init_repo_empty", map[string]any{"Source": string(SourceCNB)}))
		}
		// Empty-token checks are deferred to Check time — construction doesn't block (so
		// NewMirrorProvider can always produce a usable provider).
		m.cnbProvider = &cnbProvider{
			client:        GetClient(),
			lg:            GetLogger(),
			repo:          opts.CnbRepo,
			assetMatcher:  opts.AssetMatcherOrDefault(),
			checksumFile:  opts.ChecksumFileOrDefault(),
			gitCommitFile: opts.GitCommitFileOrDefault(),
			buildTimeFile: opts.BuildTimeFileOrDefault(),
			token:         opts.CnbToken,
			buildTime:     opts.BuildTime,
			gitCommit:     opts.GitCommit,
			prerelease:    opts.Prerelease,
		}
	case SourceGithub:
		if opts.GithubRepo == "" {
			return nil, fmt.Errorf("%s", T("updater_init_repo_empty", map[string]any{"Source": string(SourceGithub)}))
		}
		m.githubProvider = &githubProvider{
			client:        GetClient(),
			lg:            GetLogger(),
			repo:          opts.GithubRepo,
			assetMatcher:  opts.AssetMatcherOrDefault(),
			checksumFile:  opts.ChecksumFileOrDefault(),
			gitCommitFile: opts.GitCommitFileOrDefault(),
			buildTimeFile: opts.BuildTimeFileOrDefault(),
			token:         opts.GithubToken,
			buildTime:     opts.BuildTime,
			gitCommit:     opts.GitCommit,
			prerelease:    opts.Prerelease,
		}
	}
	return m, nil
}

// Name implements the updater.Provider interface.
func (m *MirrorProvider) Name() string { return "mirror" }

// decideSource resolves the actual primary source from the current package globals
// GetSource/GetLocale.
// With source=auto/empty, the choice is by language: Chinese goes to CNB, everything else to
// GitHub.
func decideSource(lg *slog.Logger) (Source, error) {
	cfgSource := GetSource()
	locale := GetLocale()
	switch cfgSource {
	case "", SourceAuto:
		// In auto mode the source is picked by language: Chinese goes to CNB, everything
		// else to GitHub.
		if locale == LocaleZhCN {
			lg.Debug(T("updater_source_auto_selected", "Source", string(SourceCNB)))
			return SourceCNB, nil
		}
		lg.Debug(T("updater_source_auto_selected", "Source", string(SourceGithub)))
		return SourceGithub, nil
	case SourceCNB:
		return SourceCNB, nil
	case SourceGithub:
		return SourceGithub, nil
	default:
		return "", fmt.Errorf("%s", T("updater_init_unknown_source", map[string]any{"Source": string(cfgSource)}))
	}
}

// resolveSource resolves the actual primary source at runtime from the current package
// globals GetSource/GetLocale,
// lazily initializing the corresponding sub-source (in auto mode, a language switch may
// select the sub-source that wasn't initialized at construction).
// Returns the primary source and its sub-source; src == "" means resolution failed.
func (m *MirrorProvider) resolveSource() (Source, error) {
	src, err := decideSource(GetLogger())
	if err != nil {
		return "", err
	}
	switch src {
	case SourceCNB:
		if m.cnbProvider == nil {
			m.cnbProvider = &cnbProvider{
				repo:          m.cnbRepo,
				assetMatcher:  m.assetMatcher,
				checksumFile:  m.checksumFile,
				gitCommitFile: m.gitCommitFile,
				buildTimeFile: m.buildTimeFile,
				token:         m.cnbToken,
				buildTime:     m.buildTime,
				gitCommit:     m.gitCommit,
				prerelease:    m.prerelease,
				client:        GetClient(),
				lg:            GetLogger(),
			}
		}
		return SourceCNB, nil
	case SourceGithub:
		if m.githubProvider == nil {
			m.githubProvider = &githubProvider{
				client:        GetClient(),
				lg:            GetLogger(),
				repo:          m.ghRepo,
				assetMatcher:  m.assetMatcher,
				checksumFile:  m.checksumFile,
				gitCommitFile: m.gitCommitFile,
				buildTimeFile: m.buildTimeFile,
				token:         m.githubToken,
				buildTime:     m.buildTime,
				gitCommit:     m.gitCommit,
				prerelease:    m.prerelease,
			}
		}
		return SourceGithub, nil
	default:
		return "", fmt.Errorf("%s", T("updater_init_no_source"))
	}
}

// Check implements the updater.Provider interface, dispatching to the corresponding
// sub-source by primary source.
// The primary is computed fresh each time (reading package-level GetLocale/GetSource), so a
// language switch is followed without rebuilds.
func (m *MirrorProvider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	src, err := m.resolveSource()
	if err != nil {
		return nil, err
	}
	switch src {
	case SourceCNB:
		return m.cnbProvider.Check(ctx, req)
	case SourceGithub:
		return m.githubProvider.Check(ctx, req)
	default:
		return nil, fmt.Errorf("%s", T("updater_init_no_source"))
	}
}

// Download implements the updater.Provider interface, dispatching to the corresponding
// sub-source by primary source.
// The primary is computed fresh each time (reading package-level GetLocale/GetSource), so a
// language switch is followed without rebuilds.
func (m *MirrorProvider) Download(ctx context.Context, rel *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	src, err := m.resolveSource()
	if err != nil {
		return err
	}
	switch src {
	case SourceCNB:
		return m.cnbProvider.Download(ctx, rel, dst, onProgress)
	case SourceGithub:
		return m.githubProvider.Download(ctx, rel, dst, onProgress)
	default:
		return fmt.Errorf("%s", T("updater_init_no_source"))
	}
}

// buildStableRelease constructs the stable release (source-agnostic).
func buildStableRelease(rel *updater.Release, tag, name, notes, htmlURL string, publishedAt time.Time, filename string, size int64) (*updater.Release, error) {
	if tag == "" {
		return nil, fmt.Errorf("%s", T("updater_err_release_tag_empty"))
	}
	if rel == nil {
		rel = &updater.Release{}
	}
	rel.Version = tag
	rel.Name = name
	rel.Notes = notes
	rel.PublishedAt = publishedAt
	rel.Metadata = map[string]any{
		MetadataReleaseHTMLURL: htmlURL,
	}
	rel.Artifact = updater.Artifact{
		Filename: filename,
		Size:     size,
	}
	return rel, nil
}

// buildNightlyRelease constructs the nightly release (source-agnostic).
// installedBuildTime / installedGitCommit support the update-needed decision (one extra
// time/commit verification layer over buildStableRelease).
func buildNightlyRelease(installedBuildTime time.Time, installedGitCommit string, rel *updater.Release, tag, name, notes, htmlURL string, publishedAt time.Time, filename string, size int64, remoteCommit string) (*updater.Release, error) {
	if tag == "" {
		return nil, fmt.Errorf("%s", T("updater_err_nightly_tag_empty"))
	}
	if rel == nil {
		rel = &updater.Release{}
	}
	// Note: the update-needed decision (same gitCommit / buildTime not newer) does not happen
	// here;
	// the caller's checkPrerelease decides from the downloaded gitCommitFile / buildTimeFile
	// and returns nil,nil (up-to-date).
	// This function only constructs the Release, returning a real error only for missing
	// required fields (tag/publish time).
	if publishedAt.IsZero() {
		return nil, fmt.Errorf("%s", T("updater_err_nightly_missing_published_at"))
	}
	rel.Version = tag
	rel.Name = name
	rel.Notes = notes
	rel.PublishedAt = publishedAt
	rel.Metadata = map[string]any{
		MetadataReleaseHTMLURL: htmlURL,
	}
	rel.Artifact = updater.Artifact{
		Filename: filename,
		Size:     size,
	}
	return rel, nil
}

// publishedAtGetter is the sort generic constraint: any type that can produce a
// PublishedAt string can take part in sorting.
type publishedAtGetter interface {
	GetPublishedAt() string
}

// sortReleasesByPublishedAt sorts by publish time descending (newest first); shared by CNB
// and GitHub.
func sortReleasesByPublishedAt[T publishedAtGetter](list []T) {
	sort.Slice(list, func(i, j int) bool {
		ti, ei := time.Parse(time.RFC3339, list[i].GetPublishedAt())
		tj, ej := time.Parse(time.RFC3339, list[j].GetPublishedAt())
		if ei != nil || ej != nil {
			return list[i].GetPublishedAt() > list[j].GetPublishedAt()
		}
		return ti.After(tj)
	})
}
