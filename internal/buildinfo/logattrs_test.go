package buildinfo

import "testing"

// Issue #16: a log line that names the build lets one pasted kai.log say which build produced it,
// whichever day it was written (the startup line is in another day's file after a rotation).
func TestLogAttrsNameTheBuild(t *testing.T) {
	oldV, oldC := Version, GitCommit
	t.Cleanup(func() { Version, GitCommit = oldV, oldC })

	Version, GitCommit = "v1.2.3", "abc1234"
	if got := LogAttrs(); len(got) != 4 || got[0] != "version" || got[1] != "v1.2.3" || got[2] != "commit" || got[3] != "abc1234" {
		t.Fatalf("LogAttrs() = %v, want [version v1.2.3 commit abc1234]", got)
	}

	// A build without a commit (a local one) says so instead of logging an empty value.
	GitCommit = ""
	if got := LogAttrs(); got[3] != "unknown" {
		t.Fatalf("empty commit logged as %v, want unknown", got[3])
	}
}
