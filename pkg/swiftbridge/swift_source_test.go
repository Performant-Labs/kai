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
	// Issue #173 item 8 added kai_warm_translate, a third blocking entry point (bare wait, no
	// timeout — Go calls it off the main goroutine at launch, so blocking here is fine): it
	// warms TranslationSessionCache for the default language pair. That's 2 bare waits now
	// (kai_translate's job.sema.wait, kai_warm_translate's sema.wait) plus the one timed wait
	// (kai_available_languages, unchanged and out of scope).
	if len(waits) != 3 {
		t.Fatalf("%d sema.wait lines, want exactly 3 (kai_translate + kai_warm_translate: no timeout; kai_available_languages: .now() + 30): %q", len(waits), waits)
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
	if bare != 2 || thirty != 1 {
		t.Errorf("waits = %q: want two without timeout: and one still .now() + 30 (kai_available_languages is out of scope)", waits)
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

// Issue #173 item 8, PR review finding: a session.translate() call is unbounded and
// uncancellable (issue #111). An earlier revision of TranslationSessionCache was a single
// actor serializing every (source, target) pair behind one queue, so one stuck call for pair
// A would head-of-line-block every later call for pair B too — reproduced live during this
// PR's hand-testing (a translate() still running after 10+ minutes). Pins the per-pair design
// that replaced it: one SessionEntry actor per pair (so two calls for the SAME pair still
// serialize, the safety property that matters — Apple does not document TranslationSession as
// safe for concurrent use), with TranslationSessionCache itself downgraded from an actor to a
// plain class (a lock only guards the synchronous dictionary lookup, never an await), so
// different pairs no longer share a queue.
func TestSessionCacheIsPerPairNotGlobal(t *testing.T) {
	src := readSwift(t, "apple_translate.swift")
	if !strings.Contains(src, "actor SessionEntry") {
		t.Error("apple_translate.swift must declare `actor SessionEntry` (one actor per language pair)")
	}
	if strings.Contains(src, "actor TranslationSessionCache") {
		t.Error("TranslationSessionCache must not be `actor` (that reintroduces a single queue " +
			"serializing every language pair — see the head-of-line-blocking finding this pins)")
	}
	if !strings.Contains(src, "final class TranslationSessionCache") {
		t.Error("apple_translate.swift must declare `final class TranslationSessionCache` (per-pair " +
			"design: a plain class dispatching to per-pair SessionEntry actors)")
	}
}

// Issue #200: the local language detector entry point. It is a pure NaturalLanguage computation,
// so it must stay clear of everything with a main-thread or wait rule: no AppKit, no semaphore,
// no Task. It is declared, registered on the Go side and stubbed on the other OSes.
func TestDetectLanguageEntryPoint(t *testing.T) {
	src := readSwift(t, "apple_translate.swift")
	i := strings.Index(src, `@_cdecl("kai_detect_language")`)
	if i < 0 {
		t.Fatal(`apple_translate.swift does not declare @_cdecl("kai_detect_language")`)
	}
	body := src[i:]
	if j := strings.Index(body[1:], "@_cdecl"); j >= 0 {
		body = body[:j+1]
	}
	if !strings.Contains(body, "NLLanguageRecognizer") {
		t.Error("kai_detect_language does not use NLLanguageRecognizer")
	}
	for _, banned := range []string{"sema", "Task", "NSApp", "NSWorkspace", "DispatchQueue.main", "MainActor"} {
		if strings.Contains(body, banned) {
			t.Errorf("kai_detect_language uses %q: it must be a plain synchronous computation", banned)
		}
	}
	load, err := os.ReadFile("load.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(load), `register(&KaiDetectLanguage, "kai_detect_language")`) {
		t.Error("load.go does not register kai_detect_language")
	}
	other, err := os.ReadFile("load_other.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(other), "KaiDetectLanguage") {
		t.Error("load_other.go has no KaiDetectLanguage stub")
	}
}
