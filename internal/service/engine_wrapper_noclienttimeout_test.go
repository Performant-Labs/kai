package service

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Issue #109 (criterion 1): registerEngines builds every engine's HTTP client without a total
// timeout. The clients are private to the engines, so this pins the construction site's source.
func TestEngineClientsAreBuiltWithoutTimeout(t *testing.T) {
	b, err := os.ReadFile("engine_wrapper.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if strings.Contains(s, "c.Timeout") {
		t.Error("newClient still assigns a client Timeout")
	}
	if m := regexp.MustCompile(`newClient\([^)]`).FindString(s); m != "" {
		t.Errorf("newClient is still called or declared with an argument (%q): the timeout parameter must be gone", m)
	}
	if strings.Contains(s, "newClient := func(timeout") {
		t.Error("newClient still declares a timeout parameter")
	}
}
