// Package buildinfo exposes build-time variables overridable via -ldflags -X, and owns the
// runtime directory rules and mode detection.
//
// All build-time injected variables (version / build time / dev flag / updater tokens) are
// centralized in this package, injected by CI via
// -ldflags -X cnb.cool/dtapp/kai/internal/buildinfo.xxx so they never scatter into main.
// The app data directory rules (data root / database subdir / logs subdir) are also
// centralized here, provided by DataDir / DBDir / LogDir; main and all other code only call
// them and never concatenate paths themselves.
package buildinfo

import "path/filepath"

// UpdaterGithubRepo is the GitHub repository (owner/name) the in-app updater polls for releases.
// It is the public Performant-Labs/kai repository, never upstream dtapps/kai (issue #178), so an
// anonymous update check can read its releases.
const UpdaterGithubRepo = "Performant-Labs/kai"

// The variables below are injected at packaging time:
//
//	-X cnb.cool/dtapp/kai/internal/buildinfo.Version=1.0.0
//	-X cnb.cool/dtapp/kai/internal/buildinfo.BuildTime=2026-08-06T12:00:00Z
//	-X cnb.cool/dtapp/kai/internal/buildinfo.Dev=false
//	-X cnb.cool/dtapp/kai/internal/buildinfo.GithubToken=xxx
//	-X cnb.cool/dtapp/kai/internal/buildinfo.PosthogToken=phc_xxx
//	-X cnb.cool/dtapp/kai/internal/buildinfo.PosthogProjectID=12345
var (
	Version   = "dev"
	BuildTime = "unknown"
	// Dev flag: "true" uses ~/.kai.dev/, "false" uses ~/.kai/.
	Dev = "true"
	// Updater-related token (GitHub source). Empty when not injected
	// in local dev; does not affect running.
	GithubToken = ""
	// GitCommit is the commit hash injected at build time (injected by CI via -ldflags;
	// empty locally).
	GitCommit = ""
	// PosthogToken is the PostHog project's public token (client-side exposure is
	// acceptable; used as SDK reporting auth).
	// Empty when not injected in local dev; the analytics package combines this + non-dev
	// build + the user switch to decide whether to report.
	PosthogToken = ""
	// PosthogProjectID is the PostHog project ID (metadata/grouping only, not part of SDK
	// auth; may be empty).
	PosthogProjectID = ""
)

// IsDev reports whether this is a dev build
func IsDev() bool { return Dev == "true" || Dev == "1" }

// DataHome returns the data root directory name (switches by mode; no home prefix)
func DataHome() string {
	if IsDev() {
		return ".kai.dev"
	}
	return ".kai"
}

// DataDir returns the app data root: ~/{.kai|.kai.dev}
func DataDir(homeDir string) string {
	return filepath.Join(homeDir, DataHome())
}

// DBDir returns the database directory: the data/ subdir under DataDir (config.db /
// history.db / httplog.db)
func DBDir(homeDir string) string {
	return filepath.Join(DataDir(homeDir), "data")
}

// LogDir returns the logs directory: the logs/ subdir under DataDir (kai.log /
// frontend.log / kai-bridge.log)
func LogDir(homeDir string) string {
	return filepath.Join(DataDir(homeDir), "logs")
}

// singleInstanceProdID is the single-instance lock ID of the released app. It equals the bundle
// identifier. It changed once, from the historical "cnb.cool.dtapp.kai", together with the bundle
// identifier (issue #27); a Kai from before that change does not share the lock with this one, so
// it must be quit before the new one is opened. It must not change again.
const singleInstanceProdID = "com.performantlabs.kai"

// SingleInstanceID returns the Wails single-instance UniqueID. The dev build (Kai-dev) gets its
// own, so it can start while a released Kai is running: with a shared ID the second process
// loses the lock, notifies the first one and exits, i.e. launching Kai-dev would only activate
// the released Kai.
func SingleInstanceID() string {
	if IsDev() {
		return singleInstanceProdID + ".dev"
	}
	return singleInstanceProdID
}
