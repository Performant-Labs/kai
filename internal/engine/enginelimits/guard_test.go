package enginelimits

// Contract tests for the Apple probe's packaging (issue #119, part A). Untagged, so they run in the
// authoritative suite and read files as text only.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// repoRoot resolves the repository root the same way bridgeStamp does (the package is three
// levels down).
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func readText(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// probeWiring matches what would make a suite, a Make/Task target or a workflow run the probe or
// the cancel checks: the enginelimits build tag passed with -tags, or the gate variable. The
// package path alone is not a match: the authoritative command's ./internal/... already covers the
// package, where the tests skip themselves without the tag.
var probeWiring = regexp.MustCompile(`-tags[= ]+["']?[^\s"']*\benginelimits\b|KAI_ENGINE_PROBE`)

// probeWiredIn returns every place under root that wires the probe in: the pipeline's
// test.unit.command, the Makefile, any Taskfile and any GitHub workflow.
func probeWiredIn(t *testing.T, root string) []string {
	t.Helper()
	var hits []string
	check := func(where, text string) {
		if loc := probeWiring.FindString(text); loc != "" {
			hits = append(hits, where+": "+loc)
		}
	}
	if b, err := os.ReadFile(filepath.Join(root, ".opencode", "pipeline.config.json")); err == nil {
		var cfg struct {
			Test struct {
				Unit struct {
					Command []string `json:"command"`
				} `json:"unit"`
			} `json:"test"`
		}
		if err := json.Unmarshal(b, &cfg); err != nil {
			t.Fatalf("pipeline.config.json: %v", err)
		}
		if len(cfg.Test.Unit.Command) == 0 {
			t.Fatalf("pipeline.config.json has no test.unit.command to check")
		}
		check("test.unit.command", strings.Join(cfg.Test.Unit.Command, " "))
	}
	var files []string
	files = append(files, filepath.Join(root, "Makefile"))
	tf, _ := filepath.Glob(filepath.Join(root, "Taskfile*"))
	files = append(files, tf...)
	wf, _ := filepath.Glob(filepath.Join(root, ".github", "workflows", "*"))
	files = append(files, wf...)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue // a missing Makefile is not wiring
		}
		rel, _ := filepath.Rel(root, f)
		check(rel, string(b))
	}
	return hits
}

func TestProbeNotWiredIntoSuiteOrCI(t *testing.T) {
	if hits := probeWiredIn(t, repoRoot(t)); len(hits) != 0 {
		t.Errorf("the Apple probe / cancel checks are wired into the suite or CI (they must stay opt-in): %v", hits)
	}
}

// The guard has teeth: each kind of wiring in a fixture repo is reported, and a plain package path
// is not.
func TestProbeWiringGuardDetectsWiring(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, ".opencode"), 0o755))
	must(os.MkdirAll(filepath.Join(root, ".github", "workflows"), 0o755))
	must(os.WriteFile(filepath.Join(root, ".opencode", "pipeline.config.json"),
		[]byte(`{"test":{"unit":{"command":["sh","-c","go test ./internal/... -count=1"]}}}`), 0o644))
	if hits := probeWiredIn(t, root); len(hits) != 0 {
		t.Fatalf("a plain ./internal/... command was reported as wiring: %v", hits)
	}
	must(os.WriteFile(filepath.Join(root, ".opencode", "pipeline.config.json"),
		[]byte(`{"test":{"unit":{"command":["sh","-c","go test -tags enginelimits ./internal/..."]}}}`), 0o644))
	must(os.WriteFile(filepath.Join(root, "Makefile"), []byte("probe:\n\tKAI_ENGINE_PROBE=1 go test ./...\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, "Taskfile.yml"), []byte("cmds: [go test -tags=\"darwin,enginelimits\" ./...]\n"), 0o644))
	must(os.WriteFile(filepath.Join(root, ".github", "workflows", "ci.yml"), []byte("env:\n  KAI_ENGINE_PROBE: \"1\"\n"), 0o644))
	if hits := probeWiredIn(t, root); len(hits) != 4 {
		t.Errorf("guard found %d of 4 wirings: %v", len(hits), hits)
	}
}

// The payload maximum mirrors the engine's inline 64 KiB output buffer (apple_darwin.go, owned by
// #106); the bridge writes at most cap-1 bytes. This pins the mirror so #106 changing the buffer
// fails here instead of drifting silently.
func TestAppleOutBufMirrorsTheEngineBuffer(t *testing.T) {
	if appleOutBufPayloadMax != 1<<16-1 {
		t.Errorf("appleOutBufPayloadMax = %d, want 65535 (1<<16 buffer, NUL-terminated)", appleOutBufPayloadMax)
	}
	src := readText(t, filepath.Join(repoRoot(t), "internal", "engine", "apple_darwin.go"))
	start := strings.Index(src, "func (s *appleTranslator) Translate(")
	if start < 0 {
		t.Fatal("apple_darwin.go: Translate not found")
	}
	body := src[start:]
	if end := strings.Index(body[1:], "\nfunc "); end >= 0 {
		body = body[:end+1]
	}
	if !strings.Contains(body, "make([]byte, 1<<16)") {
		t.Errorf("apple_darwin.go Translate no longer allocates make([]byte, 1<<16): update appleOutBufPayloadMax (and re-measure, #106)")
	}
}

// One named constant, no second literal: across the helpers and the probe (the package's non-test
// Go files plus probe_test.go, comments stripped) the 64 KiB figure appears once, and the file that
// declares it points at apple_darwin.go and #106. cancel_test.go is out of scope: it calls the
// bridge directly with a buffer of its own, and #119 leaves the cancel checks untouched.
func TestAppleOutBufIsOneNamedConstant(t *testing.T) {
	lit := regexp.MustCompile(`\b65_?535\b|\b1\s*<<\s*16\b|\b65_?536\b`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	count, declFile := 0, ""
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") && f != "probe_test.go" {
			continue
		}
		for _, line := range strings.Split(readText(t, f), "\n") {
			code := line
			if i := strings.Index(code, "//"); i >= 0 {
				code = code[:i]
			}
			if n := len(lit.FindAllString(code, -1)); n > 0 {
				count += n
				if strings.Contains(code, "appleOutBufPayloadMax") {
					declFile = f
				}
			}
		}
	}
	if count != 1 || declFile == "" {
		t.Errorf("the 64 KiB buffer figure appears %d times in the package's code (declared in %q); want exactly one, the appleOutBufPayloadMax declaration", count, declFile)
		return
	}
	text := readText(t, declFile)
	if !strings.Contains(text, "apple_darwin.go") || !strings.Contains(text, "#106") {
		t.Errorf("%s declares appleOutBufPayloadMax without a comment pointing at apple_darwin.go and #106", declFile)
	}
}

// doc.go must not claim the package has no production code once untagged helpers exist.
func TestDocGoMatchesThePackageContents(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var untagged []string
	for _, f := range files {
		if f == "doc.go" || strings.HasSuffix(f, "_test.go") {
			continue
		}
		if !strings.HasPrefix(readText(t, f), "//go:build") {
			untagged = append(untagged, f)
		}
	}
	if len(untagged) > 0 && strings.Contains(readText(t, "doc.go"), "no production code") {
		t.Errorf("doc.go says the package %q, but untagged non-test files %v exist", "contains no production code", untagged)
	}
}

// The run guide in doc.go and in docs/engine-limits.md gives a -timeout that covers the whole budget
// plus one point's ceiling (a point may start just inside the cap and run to its ceiling), and never
// the old 3h; and it documents every knob.
func TestRunGuideTimeoutAndKnobs(t *testing.T) {
	cfg, err := probeConfigFromEnv(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	floor := cfg.budget + pointCeiling
	timeout := regexp.MustCompile(`-timeout\s+(\S+)`)
	for _, path := range []string{"doc.go", filepath.Join(repoRoot(t), "docs", "engine-limits.md")} {
		text := readText(t, path)
		found := 0
		for _, line := range strings.Split(text, "\n") {
			if !strings.Contains(line, "-run TestProbeApple") {
				continue
			}
			m := timeout.FindStringSubmatch(line)
			if m == nil {
				t.Errorf("%s: probe command without -timeout: %s", path, strings.TrimSpace(line))
				continue
			}
			found++
			d, err := time.ParseDuration(strings.Trim(m[1], "`."))
			if err != nil {
				t.Errorf("%s: -timeout %q: %v", path, m[1], err)
				continue
			}
			if d < floor || d > 2*time.Hour {
				t.Errorf("%s: probe -timeout %s, want at least the budget plus one point ceiling (%s) and at most 2h (never the old 3h)", path, d, floor)
			}
		}
		if found == 0 {
			t.Errorf("%s: no TestProbeApple command with a -timeout", path)
		}
		for _, knob := range []string{"KAI_ENGINE_PROBE_REPEATS", "KAI_ENGINE_PROBE_BUDGET_MIN", "KAI_ENGINE_PROBE_MAX_RUNES", "KAI_ENGINE_PROBE_SKIP_BOUNDARY", "KAI_ENGINE_PROBE_STATUS"} {
			if !strings.Contains(text, knob) {
				t.Errorf("%s: the run guide does not document %s", path, knob)
			}
		}
	}
}

// TestQualityProbeRunGuideTimeoutAndKnobs mirrors TestRunGuideTimeoutAndKnobs for the quality
// probe (TestProbeQuality, epic #151, issues #152/#153): its run guide is docs/quality-limits.md.
// It reuses most of the latency probe's knobs (never KAI_ENGINE_PROBE_REPEATS or
// KAI_ENGINE_PROBE_SKIP_BOUNDARY: the quality probe never repeats a point and has no boundary
// phase) and adds KAI_ENGINE_PROBE_DEEPL (#153's opt-in DeepL reference calls), which this test
// checks is documented alongside the others.
func TestQualityProbeRunGuideTimeoutAndKnobs(t *testing.T) {
	cfg, err := probeConfigFromEnv(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	floor := cfg.budget + pointCeiling
	timeout := regexp.MustCompile(`-timeout\s+(\S+)`)
	path := filepath.Join(repoRoot(t), "docs", "quality-limits.md")
	text := readText(t, path)
	found := 0
	for _, line := range strings.Split(text, "\n") {
		if !strings.Contains(line, "-run TestProbeQuality") {
			continue
		}
		m := timeout.FindStringSubmatch(line)
		if m == nil {
			t.Errorf("%s: probe command without -timeout: %s", path, strings.TrimSpace(line))
			continue
		}
		found++
		d, err := time.ParseDuration(strings.Trim(m[1], "`."))
		if err != nil {
			t.Errorf("%s: -timeout %q: %v", path, m[1], err)
			continue
		}
		if d < floor {
			t.Errorf("%s: probe -timeout %s, want at least the budget plus one point ceiling (%s)", path, d, floor)
		}
	}
	if found == 0 {
		t.Errorf("%s: no TestProbeQuality command with a -timeout", path)
	}
	for _, knob := range []string{"KAI_ENGINE_PROBE_BUDGET_MIN", "KAI_ENGINE_PROBE_MAX_RUNES", "KAI_ENGINE_PROBE_STATUS", "KAI_ENGINE_PROBE_DEEPL"} {
		if !strings.Contains(text, knob) {
			t.Errorf("%s: the run guide does not document %s", path, knob)
		}
	}
}
