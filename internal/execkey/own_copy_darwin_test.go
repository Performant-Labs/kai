//go:build darwin

package execkey

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Issue #199: Kai's own simulated Cmd+C must not be counted as the user's double press. Every place
// copy_darwin.go posts a key through robotgo must be immediately preceded by the doublecopy mark, or
// the double-copy listener would translate Kai's own copy of the selection. (The posting itself
// needs a live main thread and the Accessibility grant, so this pins the call order in the source,
// like the repo's other wiring pins; the marking is exercised for real in
// internal/doublecopy/source_darwin_test.go.)
func TestEveryOwnKeyTapIsMarkedFirst(t *testing.T) {
	b, err := os.ReadFile("copy_darwin.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	taps := regexp.MustCompile(`robotgo\.KeyTap\(`).FindAllStringIndex(src, -1)
	if len(taps) < 2 {
		t.Fatalf("found %d robotgo.KeyTap calls, want the default-key and custom-key paths (2)", len(taps))
	}
	for _, loc := range taps {
		before := src[:loc[0]]
		i := strings.LastIndex(before, "doublecopy.MarkOwnCopy()")
		if i < 0 {
			t.Fatalf("a robotgo.KeyTap at offset %d has no doublecopy.MarkOwnCopy() before it", loc[0])
		}
		// Between the mark and the tap there is only the closure opening, never another statement
		// that could take long or another tap.
		between := before[i:]
		if strings.Count(between, "robotgo.KeyTap(") != 0 || strings.Count(between, "\n") > 4 {
			t.Errorf("the mark is not directly before the tap at offset %d:\n%s", loc[0], between)
		}
	}
}
