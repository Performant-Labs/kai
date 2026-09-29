package swiftbridge

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"unsafe"
)

// Issue #208: the Foundation Models entry points. A TEXT check of the Swift source (CI has no
// Swift compiler): what the two entry points must and must not do. The behaviour against the real
// model is covered by the guarded test in internal/engine.

// Compile-time contract: the function pointers exist with these signatures on every OS (load.go on
// macOS, load_other.go elsewhere).
var (
	_ func(instructions, text string, out unsafe.Pointer, outCap int32) int32 = KaiCorrect
	_ func(locale string, out unsafe.Pointer, outCap int32) int32             = KaiCorrectAvailability
)

func TestCorrectEntryPoints(t *testing.T) {
	src := readSwift(t, "apple_correct.swift")
	for _, sym := range []string{"kai_correct", "kai_correct_availability"} {
		if !strings.Contains(src, `@_cdecl("`+sym+`")`) {
			t.Errorf("apple_correct.swift does not declare @_cdecl(%q)", sym)
		}
	}
	// Guided generation: the model returns a value of a @Generable type, not free text to be
	// scraped for a preamble or quotes.
	if !strings.Contains(src, "@Generable") || !strings.Contains(src, "generating:") {
		t.Error("kai_correct does not use guided generation (@Generable + respond(generating:))")
	}
	// Deterministic output: greedy sampling, so the same text corrects the same way twice.
	if !strings.Contains(src, ".greedy") {
		t.Error("kai_correct does not sample greedily")
	}
	// Never the main thread: the call runs from a service goroutine and waits on a semaphore.
	for _, banned := range []string{"DispatchQueue.main", "MainActor", "NSApp.", "RunLoop.main"} {
		if strings.Contains(src, banned) {
			t.Errorf("apple_correct.swift uses %q: the model call must not touch the main thread", banned)
		}
	}
	// The wait is bounded: a model that never answers cannot hold a goroutine forever.
	waits := regexp.MustCompile(`(?m)^.*sema\.wait.*$`).FindAllString(src, -1)
	if len(waits) != 1 || !strings.Contains(waits[0], "timeout:") {
		t.Errorf("waits = %q, want exactly one, with a timeout", waits)
	}
	// The user's text is never written to the bridge log: a log line may interpolate a length
	// (input.count) but never the text, the instructions or the answer themselves.
	for _, m := range regexp.MustCompile(`(?m)^.*bridgeFileLog.*$`).FindAllString(src, -1) {
		m = strings.NewReplacer("input.count", "N", "answer.count", "N").Replace(m)
		for _, v := range []string{`\(input`, `\(text`, `\(system`, `\(answer`, `\(response`} {
			if strings.Contains(m, v) {
				t.Errorf("a bridge log line may carry the user's text: %q", m)
			}
		}
	}
	// The status words the Go side maps must all be what the Swift side can say.
	for _, s := range []string{"available", "model_not_ready", "apple_intelligence_off", "unsupported_hardware", "unsupported_language"} {
		if !strings.Contains(src, `"`+s+`"`) {
			t.Errorf("apple_correct.swift never says %q", s)
		}
	}
	for _, s := range []string{BridgeErrCorrectRefused, BridgeErrCorrectFailed, BridgeErrCorrectTimeout} {
		if !strings.Contains(readSwift(t, "bridge_errors.swift"), `"`+s+`"`) {
			t.Errorf("bridge_errors.swift has no %q constant", s)
		}
	}
}

func TestCorrectPointersAreRegistered(t *testing.T) {
	load, err := os.ReadFile("load.go")
	if err != nil {
		t.Fatal(err)
	}
	other, err := os.ReadFile("load_other.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"KaiCorrect", "KaiCorrectAvailability"} {
		if !strings.Contains(string(load), name) {
			t.Errorf("load.go has no %s", name)
		}
		if !strings.Contains(string(other), name) {
			t.Errorf("load_other.go has no %s stub", name)
		}
	}
	for _, sym := range []string{"kai_correct", "kai_correct_availability"} {
		if !strings.Contains(string(load), `"`+sym+`"`) {
			t.Errorf("load.go does not register %q", sym)
		}
	}
}
