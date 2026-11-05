//go:build enginelimits && darwin

package enginelimits

import (
	"fmt"
	"strings"
	"testing"
)

func synth(seed int) string {
	var b strings.Builder
	for i := 0; b.Len() < 1500; i++ {
		fmt.Fprintf(&b, "Section %d.%d: the community shipped %d changes this year, and the maintainers reviewed them carefully before every release. ", seed, i, seed*100+i)
	}
	return b.String()
}

func TestZZSame(t *testing.T) {
	_ = gatedProber(t)
	s := latin
	s.toCode = "es-MX"
	direct(s, "warm up", 1)
	for i := 1; i <= 3; i++ {
		_, d, err := direct(s, synth(i), int64(10+i))
		t.Logf("ZZ bridge synth(%d) 1500 chars: %v err=%v", i, d.Round(1e6), err)
	}
}
