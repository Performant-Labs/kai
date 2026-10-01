package buildinfo

import (
	"os"
	"regexp"
	"testing"
)

// issue #178: the in-app updater must poll the public Performant-Labs/kai repo, never upstream (dtapp/kai, dtapps/kai), and
// must not configure a CNB source.
func TestUpdaterRepoIsThePublicRepo(t *testing.T) {
	if UpdaterGithubRepo != "Performant-Labs/kai" {
		t.Errorf("UpdaterGithubRepo = %q, want Performant-Labs/kai", UpdaterGithubRepo)
	}
}

// main.go is not part of the authoritative go test target, so guard its source text here: no quoted
// upstream repo path, no CNB option, and the repo comes from buildinfo.
func TestMainGoDoesNotReferenceUpstreamOrCnb(t *testing.T) {
	src, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, pat := range []string{`"dtapps?/kai"`, `CnbRepo`, `CnbToken`, `buildinfo\.CnbToken`} {
		if regexp.MustCompile(pat).Match(src) {
			t.Errorf("main.go still matches %s", pat)
		}
	}
	if !regexp.MustCompile(`buildinfo\.UpdaterGithubRepo`).Match(src) {
		t.Error("main.go must take the updater repo from buildinfo.UpdaterGithubRepo")
	}
}
