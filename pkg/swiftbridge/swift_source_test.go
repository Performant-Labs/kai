package swiftbridge

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"unsafe"
)

// Issue #111 criteria 2, 4, 9 (Tester). This is a TEXT check of the Swift sources, not a behaviour
// test: CI has no Swift compiler and no Translation framework, so it is the one guard on the Swift
// half that runs there. It reads the files the way internal/engine/llm_timeout_removed_test.go
// reads the engines.

// Compile-time contract (criterion 2): the token parameter and the cancel entry point exist with
// these exact signatures on every OS (load.go on macOS, load_other.go elsewhere).
var (
	_ func(src, dst, text string, token int64, out unsafe.Pointer, outCap int32) int32 = KaiTranslate
	_ func(token int64) int32                                                          = KaiTranslateCancel
)

func readSwift(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("internal/swift/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// translateBody returns the text of kai_translate(...) up to the next @_cdecl.
func translateBody(t *testing.T, src string) string {
	t.Helper()
	i := strings.Index(src, "func kai_translate(")
	if i < 0 {
		t.Fatal("kai_translate not found in apple_translate.swift")
	}
	rest := src[i:]
	if j := strings.Index(rest[1:], "@_cdecl"); j >= 0 {
		rest = rest[:j+1]
	}
	return rest
}

func TestTranslateWaitHasNoTimer(t *testing.T) {
	src := readSwift(t, "apple_translate.swift")
	if body := translateBody(t, src); strings.Contains(body, ".now() + 20") || strings.Contains(body, "timeout:") {
		t.Error("kai_translate still waits with a timeout; the wait must be unbounded (cancel is the way out)")
	}
	if strings.Contains(src, ".now() + 20") {
		t.Error("apple_translate.swift still contains \".now() + 20\"")
	}
	waits := regexp.MustCompile(`(?m)^.*sema\.wait.*$`).FindAllString(src, -1)
	if len(waits) != 2 {
		t.Fatalf("%d sema.wait lines, want exactly 2 (translate: no timeout; kai_available_languages: .now() + 30): %q", len(waits), waits)
	}
	var bare, thirty int
	for _, w := range waits {
		switch {
		case strings.Contains(w, "timeout:") && strings.Contains(w, ".now() + 30"):
			thirty++
		case !strings.Contains(w, "timeout:"):
			bare++
		}
	}
	if bare != 1 || thirty != 1 {
		t.Errorf("waits = %q: want one without timeout: and one still .now() + 30 (kai_available_languages is out of scope)", waits)
	}
}

func TestCancelEntryPointIsDeclared(t *testing.T) {
	src := readSwift(t, "apple_translate.swift")
	if !strings.Contains(src, `@_cdecl("kai_translate_cancel")`) {
		t.Error("apple_translate.swift does not declare @_cdecl(\"kai_translate_cancel\")")
	}
}

func TestCancelledCodeLiteralsMatch(t *testing.T) {
	m := regexp.MustCompile(`BRIDGE_ERR_CANCELLED\s*:\s*String\s*=\s*"([^"]*)"`).FindStringSubmatch(readSwift(t, "bridge_errors.swift"))
	if m == nil {
		t.Fatal("bridge_errors.swift declares no BRIDGE_ERR_CANCELLED")
	}
	if m[1] != BridgeErrCancelled {
		t.Errorf("Swift BRIDGE_ERR_CANCELLED = %q, Go BridgeErrCancelled = %q: literals must match", m[1], BridgeErrCancelled)
	}
	if BridgeErrCancelled != "cancelled" {
		t.Errorf("BridgeErrCancelled = %q, want \"cancelled\"", BridgeErrCancelled)
	}
}

func TestBridgeLogHasCancelKeys(t *testing.T) {
	src := readSwift(t, "bridge_log.swift")
	for _, key := range []string{"translate.cancel", "translate.drain"} {
		if n := strings.Count(src, `"`+key+`"`); n < 2 {
			t.Errorf("bridge_log.swift has %d entries for %q, want one in each language table (2)", n, key)
		}
	}
}
