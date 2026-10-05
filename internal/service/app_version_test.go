package service

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/buildinfo"
)

// Issue #41: the version the app shows is the one the release build injects into buildinfo
// (-X buildinfo.Version=...). GetVersion used to return a package-local "dev" that nothing set, so a
// release build would have shown "dev".
func TestGetVersionReturnsTheInjectedBuildVersion(t *testing.T) {
	old := buildinfo.Version
	t.Cleanup(func() { buildinfo.Version = old })
	buildinfo.Version = "v9.8.7"

	if got := (&AppService{}).GetVersion(); got != "v9.8.7" {
		t.Fatalf("GetVersion() = %q, want the injected buildinfo.Version %q", got, "v9.8.7")
	}
}

// Issue #41: the About card marks a development build, so it is never mistaken for a release.
func TestIsDevBuildFollowsTheBuildFlag(t *testing.T) {
	old := buildinfo.Dev
	t.Cleanup(func() { buildinfo.Dev = old })

	for dev, want := range map[string]bool{"true": true, "1": true, "false": false, "": false} {
		buildinfo.Dev = dev
		if got := (&AppService{}).IsDevBuild(); got != want {
			t.Errorf("Dev=%q: IsDevBuild() = %v, want %v", dev, got, want)
		}
	}
}
