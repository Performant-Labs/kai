package wails_updater_providers

import (
	"strings"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// Custom asset matching: matches only "upgrade-only files" (file names starting with
// updater-).
// Packaging/installer files (Windows -install.exe, macOS .app.zip, Linux
// AppImage/deb/rpm/pkg.tar.zst) are for first installs only, not self-updates;
// upgrade files are uniformly compressed (.zip on Windows/macOS, .tar.gz on Linux)
// containing a single binary, which the updater downloads, verifies (SHA256SUMS) and swaps in.
// NewUpdaterAssetMatcher constructs the "upgrade-only file" matcher.
// Note: this function reads the package global language (GetLocale) directly; the caller can
// call SetLocale at runtime
// to switch the matcher's log language dynamically, with no Options pointer held.
// This is opt-in — without an AssetMatcher, AssetMatcherOrDefault falls back to the official
// github.DefaultAssetMatcher; only after injecting this matcher does it follow the package
// global language.
func NewUpdaterAssetMatcher() github.AssetMatcher {
	lg := GetLogger()
	return func(req updater.CheckRequest, assets []github.ReleaseAsset) int {
		// Every match reads the current package-global language (T reads GetLocale
		// internally), enabling dynamic language following.
		plat := strings.ToLower(req.Platform)
		arch := strings.ToLower(req.Arch)
		lg.Debug(T("updater_matcher_start", "Plat", plat, "Arch", arch, "Count", len(assets)))
		for i, a := range assets {
			name := strings.ToLower(a.Name)
			lg.Debug(T("updater_matcher_check", "Index", i, "Name", a.Name))
			// Only upgrade-only files (updater- prefix) participate in self-update
			if !strings.HasPrefix(name, "updater-") {
				lg.Debug(T("updater_matcher_skip_not_updater"))
				continue
			}
			if strings.HasSuffix(name, ".sig") || strings.HasSuffix(name, ".asc") || strings.HasSuffix(name, ".zsync") {
				lg.Debug(T("updater_matcher_skip_sig"))
				continue
			}
			// Upgrade files must be compressed archives (.zip / .tar.gz / .tgz)
			if !strings.HasSuffix(name, ".zip") &&
				!strings.HasSuffix(name, ".tar.gz") &&
				!strings.HasSuffix(name, ".tgz") {
				lg.Debug(T("updater_matcher_skip_format"))
				continue
			}
			if plat != "" && !strings.Contains(name, plat) {
				lg.Debug(T("updater_matcher_skip_plat", "Plat", plat))
				continue
			}
			if arch != "" && !strings.Contains(name, arch) {
				lg.Debug(T("updater_matcher_skip_arch", "Arch", arch))
				continue
			}
			lg.Debug(T("updater_matcher_hit", "Index", i, "Name", a.Name))
			return i
		}
		lg.Debug(T("updater_matcher_none"))
		return -1
	}
}
