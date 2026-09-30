package wails_updater_providers

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

// issue #196: the updater only accepts a release asset named updater-*<platform>*<arch>*.zip
// (matcher.go). scripts/release-package.sh is where the release assets are named; these tests pin
// the two together, so a rename on either side fails here.

// The release's asset names for version 0.2.0 on Apple Silicon.
const (
	releaseZip  = "Kai-0.2.0-darwin-arm64.zip"
	releaseDmg  = "Kai-0.2.0-darwin-arm64.dmg"
	releaseSums = "SHA256SUMS"
	updaterZip  = "updater-Kai-0.2.0-darwin-arm64.zip"
)

func matchAsset(t *testing.T, plat, arch string, names ...string) int {
	t.Helper()
	assets := make([]github.ReleaseAsset, 0, len(names))
	for _, n := range names {
		assets = append(assets, github.ReleaseAsset{Name: n})
	}
	SetLogger(discardLogger())
	return NewUpdaterAssetMatcher()(updater.CheckRequest{Platform: plat, Arch: arch}, assets)
}

// Only the updater- zip is picked from a whole release, whatever the order of the assets.
func TestMatcherPicksOnlyTheUpdaterZipFromTheRelease(t *testing.T) {
	orders := [][]string{
		{releaseZip, updaterZip, releaseDmg, releaseSums},
		{releaseSums, releaseDmg, updaterZip, releaseZip},
	}
	for _, names := range orders {
		i := matchAsset(t, "darwin", "arm64", names...)
		if i < 0 || names[i] != updaterZip {
			t.Errorf("assets %v: matched index %d, want %s", names, i, updaterZip)
		}
	}
}

// Each asset that is not the updater zip is refused on its own, and so is the wrong platform or
// architecture.
func TestMatcherRejectsEverythingButTheUpdaterZip(t *testing.T) {
	for _, n := range []string{releaseZip, releaseDmg, releaseSums} {
		if i := matchAsset(t, "darwin", "arm64", n); i != -1 {
			t.Errorf("%s was accepted (index %d); only updater- archives may be", n, i)
		}
	}
	if i := matchAsset(t, "darwin", "arm64", updaterZip); i != 0 {
		t.Errorf("%s was rejected for darwin/arm64", updaterZip)
	}
	if i := matchAsset(t, "windows", "amd64", updaterZip); i != -1 {
		t.Errorf("%s was accepted for windows/amd64", updaterZip)
	}
	if i := matchAsset(t, "darwin", "amd64", updaterZip); i != -1 {
		t.Errorf("%s was accepted for darwin/amd64", updaterZip)
	}
}

// The name release-package.sh really produces is one the matcher accepts: read the one line that
// defines it, so renaming it there (without the prefix, or without the platform) fails here.
func TestReleasePackageScriptNamesTheUpdaterAssetForTheMatcher(t *testing.T) {
	src, err := os.ReadFile("../../scripts/release-package.sh")
	if err != nil {
		t.Fatalf("read release-package.sh: %v", err)
	}
	lines := regexp.MustCompile(`(?m)^updater_zip="([^"]+)"$`).FindAllStringSubmatch(string(src), -1)
	if len(lines) != 1 {
		t.Fatalf("release-package.sh must define updater_zip=\"...\" exactly once, found %d", len(lines))
	}
	nameLine := regexp.MustCompile(`(?m)^name="([^"]+)"$`).FindStringSubmatch(string(src))
	if nameLine == nil {
		t.Fatal(`release-package.sh has no name="..." line`)
	}
	base := strings.ReplaceAll(nameLine[1], "$ver", "0.2.0")
	got := strings.ReplaceAll(lines[0][1], "$name", base)
	if got != updaterZip {
		t.Errorf("release-package.sh produces %q, want %q", got, updaterZip)
	}
	if i := matchAsset(t, "darwin", "arm64", releaseZip, got, releaseDmg, releaseSums); i != 1 {
		t.Errorf("the matcher does not pick %q (got index %d)", got, i)
	}
}
