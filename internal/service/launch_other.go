//go:build !darwin

package service

// ObserveLaunch is a no-op off macOS.
func ObserveLaunch() {}

// DetectLaunchKind is always LaunchUnknown off macOS: no login-launch signal is read there, so
// the window is shown on every launch.
func DetectLaunchKind() LaunchKind { return LaunchUnknown }
