package service

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// LaunchKind says how this process was started (issue #17).
type LaunchKind int

const (
	// LaunchUnknown is the zero value: the launch kind could not be determined (non-macOS, the
	// Swift bridge unavailable, or no launch event seen). It is treated like a user launch.
	LaunchUnknown LaunchKind = iota
	// LaunchUser is a launch by the user: Dock, Finder, Spotlight, open.
	LaunchUser
	// LaunchLogin is a launch as a login item (the launch-at-login setting).
	LaunchLogin
)

// LaunchDecision is what to do with the translate window at startup.
type LaunchDecision struct {
	Show   bool // show the translate window
	Center bool // centre it first (the very first launch)
}

// DecideLaunch is the pure startup decision: a launch at login stays quiet (nobody asked for a
// window); every other launch, including one that cannot be classified, shows the translate
// window, centred when it is the very first launch.
func DecideLaunch(kind LaunchKind, firstLaunch bool) LaunchDecision {
	if kind == LaunchLogin {
		return LaunchDecision{}
	}
	return LaunchDecision{Show: true, Center: firstLaunch}
}

// launchKindFromCode maps the bridge's kai_launch_kind code (0 unknown, 1 user, 2 login) to a
// LaunchKind; anything unexpected is LaunchUnknown.
func launchKindFromCode(code int32) LaunchKind {
	switch code {
	case 1:
		return LaunchUser
	case 2:
		return LaunchLogin
	default:
		return LaunchUnknown
	}
}

// IsFirstRun reports whether dataDir has no settings.json yet, i.e. this is the very first
// launch on this data folder. It must run before settings.NewService, which writes the file.
// analytics.IsFirstLaunch is not used: its flag is only set when analytics is enabled, so with
// analytics off (dev builds, opted-out users) it would stay "first launch" forever.
func IsFirstRun(dataDir string) bool {
	_, err := os.Stat(filepath.Join(dataDir, "settings.json"))
	return errors.Is(err, fs.ErrNotExist)
}
