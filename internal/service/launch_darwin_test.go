//go:build darwin

package service

import "testing"

// In a test process no launch event was ever observed. Whatever state the bridge is in (not loaded,
// loaded without the launch symbols, or loaded and idle), the detector must fail open: it must
// never report a login launch, because that would hide the translate window on a launch the user
// made. LaunchUnknown and LaunchUser both show the window.
func TestDetectLaunchKindFailsOpenWithoutALaunchEvent(t *testing.T) {
	ObserveLaunch() // must not panic whether or not the bridge is loaded
	got := DetectLaunchKind()
	if got == LaunchLogin {
		t.Fatalf("DetectLaunchKind() = LaunchLogin with no launch event observed; it must fail open")
	}
	if d := DecideLaunch(got, false); !d.Show {
		t.Errorf("with no launch event the window must be shown, got %+v for %v", d, got)
	}
}
