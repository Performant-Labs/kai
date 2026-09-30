//go:build !darwin

package service

import "testing"

// Off macOS nothing can read a login-launch signal, so the launch kind must stay unknown and the
// window must be shown on every launch. If DetectLaunchKind ever returned LaunchLogin here, the
// translate window would silently stay hidden at every launch on Windows and Linux.
func TestDetectLaunchKindIsUnknownOffMacOS(t *testing.T) {
	ObserveLaunch() // a no-op; must not panic
	if got := DetectLaunchKind(); got != LaunchUnknown {
		t.Fatalf("DetectLaunchKind() = %v off macOS, want LaunchUnknown", got)
	}
	if d := DecideLaunch(DetectLaunchKind(), false); !d.Show {
		t.Errorf("a launch off macOS must show the window, got %+v", d)
	}
}
