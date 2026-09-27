package enginelimits

// Tests for the pure planning, statistics and bookkeeping helpers of the Apple probe (issue #119,
// part A). They live in an untagged file and exercise untagged helpers (plan.go), so they run in
// the authoritative suite: no Apple call, no gate, no external linking, no real sleeps (the queue
// drain takes an injected clock). The probe itself stays behind the enginelimits tag and
// KAI_ENGINE_PROBE=1.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- Planner -----------------------------------------------------------------------------------

func defaultConfig(t *testing.T) probeConfig {
	t.Helper()
	cfg, err := probeConfigFromEnv(func(string) string { return "" })
	if err != nil {
		t.Fatalf("probeConfigFromEnv with no knobs set: %v", err)
	}
	return cfg
}

// sizesOf returns, per pair, how many times each size appears in phase.
func sizesOf(plan []planPoint, phase string) map[string]map[int]int {
	out := map[string]map[int]int{}
	for _, p := range plan {
		if p.phase != phase {
			continue
		}
		if out[p.pair] == nil {
			out[p.pair] = map[int]int{}
		}
		out[p.pair][p.runes]++
	}
	return out
}

func wantLadder(t *testing.T, plan []planPoint, want map[string][]int, repeats int) {
	t.Helper()
	got := sizesOf(plan, phaseLadder)
	for pair, sizes := range want {
		if len(got[pair]) != len(sizes) {
			t.Errorf("pair %s: ladder sizes %v, want exactly %v", pair, got[pair], sizes)
		}
		for _, n := range sizes {
			if got[pair][n] != repeats {
				t.Errorf("pair %s size %d: planned %d times, want %d (every ladder size exactly `repeats` times)", pair, n, got[pair][n], repeats)
			}
		}
	}
	for pair := range got {
		if _, ok := want[pair]; !ok {
			t.Errorf("unexpected ladder pair %s in the plan", pair)
		}
	}
	// Repeat numbers for one (pair, size) are 1..repeats, each once.
	seen := map[string]bool{}
	for _, p := range plan {
		if p.phase != phaseLadder {
			continue
		}
		key := fmt.Sprintf("%s/%d/%d", p.pair, p.runes, p.repeat)
		if p.repeat < 1 || p.repeat > repeats || seen[key] {
			t.Errorf("ladder point %s: repeat %d out of 1..%d or duplicated", key, p.repeat, repeats)
		}
		seen[key] = true
	}
}

// Within one pair the ladder runs cheap sizes first.
func wantCheapFirst(t *testing.T, plan []planPoint) {
	t.Helper()
	last := map[string]int{}
	for _, p := range plan {
		if p.phase != phaseLadder {
			continue
		}
		if p.runes < last[p.pair] {
			t.Errorf("pair %s: ladder size %d planned after %d; cheap sizes must come first", p.pair, p.runes, last[p.pair])
		}
		last[p.pair] = p.runes
	}
}

func TestPlanDefaultLadderThenBoundary(t *testing.T) {
	cfg := defaultConfig(t)
	plan, err := buildPlan(cfg)
	if err != nil {
		t.Fatalf("buildPlan(default): %v", err)
	}
	wantLadder(t, plan, map[string][]int{
		"en>zh-Hans": {400, 1600, 6400, 20000},
		"zh-Hans>en": {400, 1600, 6400},
		"en>es":      {400, 1600, 6400},
	}, 3)
	wantCheapFirst(t, plan)

	// Exactly one planned boundary point: zh-Hans>en at 12,800, a single run, and it is last (the
	// rest of the boundary is bisected at run time from its result).
	var boundary []planPoint
	for i, p := range plan {
		if p.phase != phaseBoundary {
			continue
		}
		boundary = append(boundary, p)
		if i != len(plan)-1 {
			t.Errorf("boundary point at index %d of %d; the boundary must come after every ladder point", i, len(plan))
		}
	}
	if len(boundary) != 1 || boundary[0].pair != "zh-Hans>en" || boundary[0].runes != 12800 || boundary[0].repeat != 1 {
		t.Errorf("boundary points %+v, want exactly one {pair zh-Hans>en, runes 12800, repeat 1}", boundary)
	}
	if len(plan) != 3*(4+3+3)+1 {
		t.Errorf("plan has %d points, want %d (three repeats of ten ladder sizes, plus one boundary start)", len(plan), 3*(4+3+3)+1)
	}
}

func TestPlanRepeatsHonouredAndBelowThreeRejected(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.repeats = 4
	plan, err := buildPlan(cfg)
	if err != nil {
		t.Fatalf("buildPlan(repeats=4): %v", err)
	}
	wantLadder(t, plan, map[string][]int{
		"en>zh-Hans": {400, 1600, 6400, 20000},
		"zh-Hans>en": {400, 1600, 6400},
		"en>es":      {400, 1600, 6400},
	}, 4)
	for _, r := range []int{2, 1, 0, -1} {
		cfg.repeats = r
		if plan, err := buildPlan(cfg); err == nil {
			t.Errorf("buildPlan(repeats=%d) = %d points, want an error (the deliverable needs at least 3 runs per size)", r, len(plan))
		}
	}
}

func TestPlanJapaneseIsOptionalAndLast(t *testing.T) {
	cfg := defaultConfig(t)
	plan, err := buildPlan(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plan {
		if p.pair == "ja>en" {
			t.Fatalf("ja>en planned without includeJA: %+v", p)
		}
	}
	cfg.includeJA = true
	plan, err = buildPlan(cfg)
	if err != nil {
		t.Fatal(err)
	}
	wantLadder(t, plan, map[string][]int{
		"en>zh-Hans": {400, 1600, 6400, 20000},
		"zh-Hans>en": {400, 1600, 6400},
		"en>es":      {400, 1600, 6400},
		"ja>en":      {400, 1600, 6400},
	}, 3)
	// Phase 3 runs after the boundary: every ja>en point comes after the last non-ja point.
	lastOther, firstJA := -1, len(plan)
	for i, p := range plan {
		if p.pair == "ja>en" {
			firstJA = min(firstJA, i)
		} else {
			lastOther = i
		}
	}
	if firstJA < lastOther {
		t.Errorf("ja>en point at index %d before other work at index %d; the optional pair runs last", firstJA, lastOther)
	}
}

func TestPlanSkipBoundary(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.skipBoundary = true
	plan, err := buildPlan(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if b := sizesOf(plan, phaseBoundary); len(b) != 0 {
		t.Errorf("skipBoundary: boundary points %v still planned", b)
	}
	wantLadder(t, plan, map[string][]int{
		"en>zh-Hans": {400, 1600, 6400, 20000},
		"zh-Hans>en": {400, 1600, 6400},
		"en>es":      {400, 1600, 6400},
	}, 3)
}

func TestPlanMaxRunesFilter(t *testing.T) {
	t.Run("smoke run at 1000 keeps only the 400 ladder", func(t *testing.T) {
		cfg := defaultConfig(t)
		cfg.maxRunes = 1000
		plan, err := buildPlan(cfg)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range plan {
			if p.runes != 400 || p.phase != phaseLadder {
				t.Errorf("maxRunes=1000 planned %+v; only 400-rune ladder points fit", p)
			}
		}
		if len(plan) != 3*3 {
			t.Errorf("maxRunes=1000: %d points, want 9 (400 runes x 3 repeats x 3 pairs)", len(plan))
		}
	})
	t.Run("6400 drops 20000 and the boundary", func(t *testing.T) {
		cfg := defaultConfig(t)
		cfg.maxRunes = 6400
		plan, err := buildPlan(cfg)
		if err != nil {
			t.Fatal(err)
		}
		wantLadder(t, plan, map[string][]int{
			"en>zh-Hans": {400, 1600, 6400},
			"zh-Hans>en": {400, 1600, 6400},
			"en>es":      {400, 1600, 6400},
		}, 3)
		if b := sizesOf(plan, phaseBoundary); len(b) != 0 {
			t.Errorf("maxRunes=6400: boundary %v planned above the bound", b)
		}
	})
	t.Run("a bound below the smallest ladder size is an error, never an empty plan", func(t *testing.T) {
		cfg := defaultConfig(t)
		cfg.maxRunes = 300
		if plan, err := buildPlan(cfg); err == nil {
			t.Errorf("maxRunes=300: got %d points and no error, want an error", len(plan))
		}
	})
}

// --- Knobs -------------------------------------------------------------------------------------

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestProbeConfigDefaults(t *testing.T) {
	cfg := defaultConfig(t)
	if cfg.repeats != 3 {
		t.Errorf("default repeats = %d, want 3", cfg.repeats)
	}
	if cfg.budget != 90*time.Minute {
		t.Errorf("default budget = %s, want 90m", cfg.budget)
	}
	if cfg.maxRunes != 0 || cfg.skipBoundary || cfg.includeJA {
		t.Errorf("defaults %+v: want no rune bound, boundary on, ja>en off", cfg)
	}
	if cfg.statusPath != statusPath("") {
		t.Errorf("default status path %q, want statusPath(\"\") = %q", cfg.statusPath, statusPath(""))
	}
}

func TestProbeConfigOverrides(t *testing.T) {
	cfg, err := probeConfigFromEnv(envOf(map[string]string{
		"KAI_ENGINE_PROBE_REPEATS":       "4",
		"KAI_ENGINE_PROBE_BUDGET_MIN":    "30",
		"KAI_ENGINE_PROBE_MAX_RUNES":     "1000",
		"KAI_ENGINE_PROBE_SKIP_BOUNDARY": "1",
		"KAI_ENGINE_PROBE_STATUS":        "/some/where/status.jsonl",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.repeats != 4 || cfg.budget != 30*time.Minute || cfg.maxRunes != 1000 || !cfg.skipBoundary || cfg.statusPath != "/some/where/status.jsonl" {
		t.Errorf("overrides not applied: %+v", cfg)
	}
	cfg, err = probeConfigFromEnv(envOf(map[string]string{"KAI_ENGINE_PROBE_SKIP_BOUNDARY": "0"}))
	if err != nil || cfg.skipBoundary {
		t.Errorf("SKIP_BOUNDARY=0: skipBoundary=%t err=%v, want false and no error", cfg.skipBoundary, err)
	}
}

func TestProbeConfigRejectsBadValues(t *testing.T) {
	for _, bad := range []map[string]string{
		{"KAI_ENGINE_PROBE_REPEATS": "three"},
		{"KAI_ENGINE_PROBE_BUDGET_MIN": "0"},
		{"KAI_ENGINE_PROBE_BUDGET_MIN": "-5"},
		{"KAI_ENGINE_PROBE_BUDGET_MIN": "ninety"},
		{"KAI_ENGINE_PROBE_MAX_RUNES": "lots"},
	} {
		if cfg, err := probeConfigFromEnv(envOf(bad)); err == nil {
			t.Errorf("probeConfigFromEnv(%v) = %+v, want an error", bad, cfg)
		}
	}
}

func TestStatusPathDefaultIsOutsideTheRepo(t *testing.T) {
	def := statusPath("")
	if def == "" || !strings.HasPrefix(def, filepath.Clean(os.TempDir())) {
		t.Errorf("statusPath(\"\") = %q, want a file under the temp dir %q", def, os.TempDir())
	}
	if strings.Contains(def, ".worktrees") {
		t.Errorf("statusPath(\"\") = %q depends on a checkout path", def)
	}
	if got := statusPath("/x/y.jsonl"); got != "/x/y.jsonl" {
		t.Errorf("statusPath(override) = %q, want the override verbatim", got)
	}
}

// --- Time cap ----------------------------------------------------------------------------------

func TestShouldStart(t *testing.T) {
	const capD = 90 * time.Minute
	cases := []struct {
		name               string
		elapsed, projected time.Duration
		want               bool
	}{
		{"well inside", 10 * time.Minute, 10 * time.Minute, true},
		{"just under: 80m + 1.25*7m = 88m45s", 80 * time.Minute, 7 * time.Minute, true},
		{"just over: 80m + 1.25*(8m+1s) > 90m", 80 * time.Minute, 8*time.Minute + time.Second, false},
		{"the 1.25 factor bites: 80m + 9m fits unscaled, not scaled", 80 * time.Minute, 9 * time.Minute, false},
		{"from zero, 1.25*73m > 90m", 0, 73 * time.Minute, false},
		{"elapsed at the cap", capD, 0, false},
		{"elapsed past the cap", capD + time.Minute, 0, false},
	}
	for _, c := range cases {
		if got := shouldStart(c.elapsed, capD, c.projected); got != c.want {
			t.Errorf("%s: shouldStart(%s, %s, %s) = %t, want %t", c.name, c.elapsed, capD, c.projected, got, c.want)
		}
	}
}

func TestProjectPoint(t *testing.T) {
	if seedMsPerRune < 26 {
		t.Errorf("seedMsPerRune = %v; a conservative seed is at least the slowest rate on record (26 ms per CJK rune, #111)", seedMsPerRune)
	}
	want := time.Duration(1000 * float64(seedMsPerRune) * float64(time.Millisecond))
	if got := projectPoint(1000, nil); got != want {
		t.Errorf("projectPoint(1000, no data) = %s, want the seed projection %s", got, want)
	}
	if got := projectPoint(1000, []float64{9, 30, 10}); got != 10*time.Second {
		t.Errorf("projectPoint(1000, [9 30 10]) = %s, want 10s (median 10 ms per rune)", got)
	}
	if got := projectPoint(1000, []float64{8, 10}); got != 9*time.Second {
		t.Errorf("projectPoint(1000, [8 10]) = %s, want 9s (even-count median 9 ms per rune)", got)
	}
}

// --- Statistics --------------------------------------------------------------------------------

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func TestSummarize(t *testing.T) {
	t.Run("odd count", func(t *testing.T) {
		s := summarize([]sample{{latency: ms(300)}, {latency: ms(100)}, {latency: ms(200)}})
		if s.min != ms(100) || s.median != ms(200) || s.max != ms(300) {
			t.Errorf("min/median/max = %s/%s/%s, want 100ms/200ms/300ms", s.min, s.median, s.max)
		}
	})
	t.Run("even count takes the mean of the two middle values", func(t *testing.T) {
		s := summarize([]sample{{latency: ms(400)}, {latency: ms(100)}, {latency: ms(200)}, {latency: ms(300)}})
		if s.min != ms(100) || s.median != ms(250) || s.max != ms(400) {
			t.Errorf("min/median/max = %s/%s/%s, want 100ms/250ms/400ms", s.min, s.median, s.max)
		}
	})
	t.Run("a noisy run is kept in the list but not in the median", func(t *testing.T) {
		s := summarize([]sample{{latency: ms(100)}, {latency: ms(200)}, {latency: ms(300)}, {latency: ms(9000), noisy: true}})
		if s.median != ms(200) {
			t.Errorf("median = %s, want 200ms (the noisy 9000ms run excluded)", s.median)
		}
		if len(s.runs) != 4 || s.runs[3] != ms(9000) {
			t.Errorf("runs = %v, want all four latencies in order, the noisy one included", s.runs)
		}
	})
	t.Run("empty input is a zero value, not a panic", func(t *testing.T) {
		s := summarize(nil)
		if s.min != 0 || s.median != 0 || s.max != 0 || len(s.runs) != 0 {
			t.Errorf("summarize(nil) = %+v, want zero", s)
		}
	})
}

func TestNoisyAndQuietLoad(t *testing.T) {
	for _, c := range []struct {
		load1        float64
		noisy, quiet bool
	}{
		{0.5, false, true},
		{10.0, false, true},
		{10.01, false, false},
		{12.0, false, false},
		{12.01, true, false},
		{27, true, false},
	} {
		if got := isNoisy(c.load1); got != c.noisy {
			t.Errorf("isNoisy(%v) = %t, want %t (a run starting above load 12 is noisy)", c.load1, got, c.noisy)
		}
		if got := quietToStart(c.load1); got != c.quiet {
			t.Errorf("quietToStart(%v) = %t, want %t (a session starts only at load 10 or below)", c.load1, got, c.quiet)
		}
	}
}

// --- Maximum that passes, sessions -------------------------------------------------------------

func TestMaxPassing(t *testing.T) {
	pass := func(n int) sizeResult { return sizeResult{runes: n, pass: true} }
	fail := func(n int) sizeResult { return sizeResult{runes: n} }
	noisyFail := func(n int) sizeResult { return sizeResult{runes: n, noisy: true} }
	noisyPass := func(n int) sizeResult { return sizeResult{runes: n, pass: true, noisy: true} }
	cases := []struct {
		name       string
		in         []sizeResult
		wantMax    int
		wantCapped bool
	}{
		{"one failed run disqualifies its size",
			[]sizeResult{pass(400), pass(400), pass(400), pass(1600), pass(1600), fail(1600)}, 400, false},
		{"a noisy failure does not disqualify",
			[]sizeResult{pass(400), pass(1600), pass(1600), noisyFail(1600)}, 1600, false},
		{"a size with only noisy runs has no evidence",
			[]sizeResult{pass(400), noisyPass(1600), noisyPass(1600)}, 400, false},
		{"20,000 with no failure is a capped pass",
			[]sizeResult{pass(6400), pass(20000), pass(20000), pass(20000)}, 20000, true},
		{"below the cap is not capped",
			[]sizeResult{pass(400), pass(6400), fail(20000)}, 6400, false},
		{"nothing passed", []sizeResult{fail(400)}, 0, false},
		{"no data", nil, 0, false},
	}
	for _, c := range cases {
		gotMax, gotCapped := maxPassing(c.in, 20000)
		if gotMax != c.wantMax || gotCapped != c.wantCapped {
			t.Errorf("%s: maxPassing = (%d, %t), want (%d, %t)", c.name, gotMax, gotCapped, c.wantMax, c.wantCapped)
		}
	}
}

func TestCombineSessions(t *testing.T) {
	for _, c := range []struct {
		a, b, want int
		disagree   bool
	}{
		{10000, 11000, 10000, false},
		{11000, 10000, 10000, false},
		{10000, 13000, 10000, true},
		{13000, 10000, 10000, true},
		{18200, 18200, 18200, false},
	} {
		got, dis := combineSessions(c.a, c.b)
		if got != c.want || dis != c.disagree {
			t.Errorf("combineSessions(%d, %d) = (%d, %t), want (%d, %t)", c.a, c.b, got, dis, c.want, c.disagree)
		}
	}
}

// --- Boundary bisection ------------------------------------------------------------------------

func TestBracketDone(t *testing.T) {
	for _, c := range []struct {
		pass, fail int
		want       bool
	}{
		{18200, 19100, true},  // 900 <= 5% of 18,200 (910): the #111 answer is a finished bracket
		{16400, 19100, false}, // 2700 > 820
		{100, 140, true},      // the step floor of 50 applies below 1000 runes
		{100, 200, false},
	} {
		if got := bracketDone(c.pass, c.fail); got != c.want {
			t.Errorf("bracketDone(%d, %d) = %t, want %t", c.pass, c.fail, got, c.want)
		}
	}
	if stepPct != 5 || stepFloor != 50 {
		t.Errorf("stepPct=%d stepFloor=%d; the brief keeps the existing 5%% / 50-rune stop rule", stepPct, stepFloor)
	}
}

func TestNextBoundary(t *testing.T) {
	type step struct{ pass, fail, next int }
	// No failure yet: climb the #111 seeds above the last pass, then stop capped at 20,000.
	for _, s := range []step{{12800, 0, 16400}, {16400, 0, 18200}, {18200, 0, 19100}, {19100, 0, 20000}} {
		next, done := nextBoundary(s.pass, s.fail)
		if done || next != s.next {
			t.Errorf("nextBoundary(%d, %d) = (%d, %t), want (%d, false)", s.pass, s.fail, next, done, s.next)
		}
	}
	if next, done := nextBoundary(20000, 0); !done {
		t.Errorf("nextBoundary(20000, 0) = (%d, false), want done (capped at 20,000)", next)
	}
	// A failure known: bisect at the midpoint until the bracket is done.
	for _, s := range []step{{12800, 16400, 14600}, {6400, 12800, 9600}, {16400, 18200, 17300}} {
		next, done := nextBoundary(s.pass, s.fail)
		if done || next != s.next {
			t.Errorf("nextBoundary(%d, %d) = (%d, %t), want (%d, false)", s.pass, s.fail, next, done, s.next)
		}
	}
	if next, done := nextBoundary(18200, 19100); !done {
		t.Errorf("nextBoundary(18200, 19100) = (%d, false), want done (bracket within 5%%)", next)
	}
}

// --- Output buffer -----------------------------------------------------------------------------

// truncatedJSONErr reproduces the engine's error for a payload cut off by the output buffer:
// apple_darwin.go wraps json.Unmarshal's error with %w.
func truncatedJSONErr(t *testing.T) error {
	t.Helper()
	var v map[string]any
	err := json.Unmarshal([]byte(`{"result":"1. The committee rev`), &v)
	if err == nil {
		t.Fatal("setup: a truncated payload decoded")
	}
	return fmt.Errorf("result parse error: %w", err)
}

func TestBufferBound(t *testing.T) {
	trunc := truncatedJSONErr(t)
	var v map[string]any
	syntax := fmt.Errorf("result parse error: %w", json.Unmarshal([]byte(`{"result":x}`), &v))
	cases := []struct {
		name      string
		projected int
		err       error
		want      bool
	}{
		{"JSON truncation with a projection above the buffer", 70000, trunc, true},
		{"JSON truncation with a projection under the buffer", 40000, trunc, false},
		{"marker-missing failure (no error)", 70000, nil, false},
		{"another engine error", 70000, errors.New("Unable to Translate"), false},
		{"a JSON syntax error that is not truncation", 70000, syntax, false},
		{"projection exactly at the payload maximum does not exceed it", appleOutBufPayloadMax, trunc, false},
		{"projection one byte over", appleOutBufPayloadMax + 1, trunc, true},
	}
	for _, c := range cases {
		if got := bufferBound(0, c.projected, c.err); got != c.want {
			t.Errorf("%s: bufferBound(0, %d, %v) = %t, want %t", c.name, c.projected, c.err, got, c.want)
		}
	}
}

func TestProjectOutBytes(t *testing.T) {
	if got := projectOutBytes(19100, []float64{3.0, 4.0, 3.5}); got != 66850 {
		t.Errorf("projectOutBytes(19100, [3 4 3.5]) = %d, want 66850 (median 3.5 bytes per input rune)", got)
	}
	if got := projectOutBytes(19100, nil); got != 0 {
		t.Errorf("projectOutBytes with no passing points = %d, want 0 (no projection)", got)
	}
}

// --- Status file -------------------------------------------------------------------------------

var statusKeys = []string{"time", "elapsed_s", "phase", "pair", "runes", "utf8_bytes", "repeat", "latency_ms", "out_bytes", "kind", "load1", "next", "budget_left_s"}

func TestAppendStatusWritesOneJSONLinePerCallAndAppends(t *testing.T) {
	boise, err := time.LoadLocation("America/Boise")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "status.jsonl")
	if err := os.WriteFile(path, []byte("{\"earlier\":true}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 9, 26, 17, 40, 0, 0, boise)
	if err := appendStatus(path, statusLine{Time: at, Phase: phaseLadder, Pair: "en>zh-Hans", Runes: 400}); err != nil {
		t.Fatalf("appendStatus #1: %v", err)
	}
	if err := appendStatus(path, statusLine{Time: at.Add(time.Minute), Phase: phaseDone}); err != nil {
		t.Fatalf("appendStatus #2: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) != 3 || lines[0] != `{"earlier":true}` {
		t.Fatalf("file lines %q: want the earlier line kept and two appended lines", lines)
	}
	for i, line := range lines[1:] {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %d is not one JSON object: %v (%q)", i+2, err, line)
		}
		for _, k := range statusKeys {
			if _, ok := m[k]; !ok {
				t.Errorf("line %d lacks field %q: %s", i+2, k, line)
			}
		}
		ts, _ := m["time"].(string)
		parsed, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			t.Errorf("line %d time %q is not RFC 3339: %v", i+2, ts, err)
		} else if _, off := parsed.Zone(); off != -6*3600 {
			t.Errorf("line %d time %q lost its zone offset (want -06:00, MDT)", i+2, ts)
		}
	}
	var first map[string]any
	_ = json.Unmarshal([]byte(lines[1]), &first)
	if first["phase"] != phaseLadder || first["pair"] != "en>zh-Hans" || first["runes"] != float64(400) {
		t.Errorf("first status line %v does not carry the values written", first)
	}
}

func TestAppendStatusCreatesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.jsonl")
	if err := appendStatus(path, statusLine{Time: time.Now(), Phase: phaseDone}); err != nil {
		t.Fatalf("appendStatus on a missing file: %v", err)
	}
	b, err := os.ReadFile(path)
	if err != nil || strings.Count(string(b), "\n") != 1 {
		t.Errorf("new file = %q (err %v), want exactly one line", b, err)
	}
}

// --- Queue drain -------------------------------------------------------------------------------

// fakeClock is the injected clock: Sleep only advances time, so the drain logic runs in
// microseconds. warmCall builds a warm request that also advances it by its latency.
type fakeClock struct {
	now   time.Time
	slept time.Duration
}

func (c *fakeClock) Now() time.Time                   { return c.now }
func (c *fakeClock) Sleep(d time.Duration)            { c.now = c.now.Add(d); c.slept += d }
func (c *fakeClock) since(t0 time.Time) time.Duration { return c.now.Sub(t0) }

type warmScript struct {
	c      *fakeClock
	lat    []time.Duration // per call; the last repeats
	err    error
	starts []time.Duration // when each call started, relative to t0
	t0     time.Time
}

func (w *warmScript) call() (time.Duration, error) {
	w.starts = append(w.starts, w.c.since(w.t0))
	d := w.lat[min(len(w.starts)-1, len(w.lat)-1)]
	w.c.now = w.c.now.Add(d)
	return d, w.err
}

func newWarm(lat []time.Duration, err error) (*fakeClock, *warmScript) {
	c := &fakeClock{now: time.Date(2026, 9, 26, 18, 0, 0, 0, time.UTC)}
	return c, &warmScript{c: c, lat: lat, err: err, t0: c.now}
}

func TestSettleAfterAPassPausesFiveSeconds(t *testing.T) {
	c, w := newWarm([]time.Duration{time.Second}, nil)
	if got := settle(kindPass, c, w.call); got != drainNone {
		t.Errorf("settle(pass) = %q, want %q", got, drainNone)
	}
	if c.slept != 5*time.Second || len(w.starts) != 0 {
		t.Errorf("after a pass: slept %s with %d warm calls, want exactly 5s and no warm call", c.slept, len(w.starts))
	}
}

func TestSettleAfterAnErrorWaitsForTheQueue(t *testing.T) {
	s := time.Second
	cases := []struct {
		name      string
		lat       []time.Duration
		err       error
		want      string
		wantCalls int // 0: not checked
	}{
		{"first warm call fast", []time.Duration{2 * s}, nil, drainOK, 1},
		{"queue drains on the third call", []time.Duration{20 * s, 12 * s, 4 * s}, nil, drainOK, 3},
		{"exactly 5 s is not under 5 s", []time.Duration{5 * s, 3 * s}, nil, drainOK, 2},
		{"never under 5 s: timeout", []time.Duration{25 * s}, nil, drainTimeout, 0},
		{"a fast failing warm call is not drained", []time.Duration{s}, errors.New("Unable to Translate"), drainTimeout, 0},
	}
	for _, cse := range cases {
		c, w := newWarm(cse.lat, cse.err)
		got := settle(kindError, c, w.call)
		if got != cse.want {
			t.Errorf("%s: settle(error) = %q, want %q", cse.name, got, cse.want)
		}
		if cse.wantCalls != 0 && len(w.starts) != cse.wantCalls {
			t.Errorf("%s: %d warm calls, want %d", cse.name, len(w.starts), cse.wantCalls)
		}
		for i, st := range w.starts {
			if st >= 60*s {
				t.Errorf("%s: warm call %d started at %s, after the 60 s drain limit", cse.name, i+1, st)
			}
		}
	}
	if drainTimeout != "timeout" {
		t.Errorf("drainTimeout = %q; the handoff records it as queue_drain=timeout", drainTimeout)
	}
}

func TestIsSmokeRun(t *testing.T) {
	for _, c := range []struct {
		maxRunes int
		want     bool
	}{
		{0, false},    // no bound: a real session
		{1000, true},  // the smoke run
		{400, true},   // smaller still
		{1001, false}, // a capped real session is still a session
		{6400, false},
		{20000, false},
	} {
		if got := isSmokeRun(probeConfig{maxRunes: c.maxRunes}); got != c.want {
			t.Errorf("isSmokeRun(maxRunes=%d) = %t, want %t (only a run bounded to %d runes or fewer skips the quiet wait)", c.maxRunes, got, c.want, smokeMaxRunes)
		}
	}
}

// --- The quality probe's plan (#152/#153) ----------------------------------------------------

func TestProbeConfigDeeplDefaultOffAndOverride(t *testing.T) {
	if cfg := defaultConfig(t); cfg.deepl {
		t.Errorf("default deepl = %t, want false: #153's DeepL calls must be opt-in", cfg.deepl)
	}
	cfg, err := probeConfigFromEnv(envOf(map[string]string{"KAI_ENGINE_PROBE_DEEPL": "1"}))
	if err != nil || !cfg.deepl {
		t.Errorf("KAI_ENGINE_PROBE_DEEPL=1: deepl=%t err=%v, want true and no error", cfg.deepl, err)
	}
	cfg, err = probeConfigFromEnv(envOf(map[string]string{"KAI_ENGINE_PROBE_DEEPL": "0"}))
	if err != nil || cfg.deepl {
		t.Errorf("KAI_ENGINE_PROBE_DEEPL=0: deepl=%t err=%v, want false and no error", cfg.deepl, err)
	}
	if cfg, err := probeConfigFromEnv(envOf(map[string]string{"KAI_ENGINE_PROBE_DEEPL": "yes"})); err == nil {
		t.Errorf("probeConfigFromEnv(DEEPL=yes) = %+v, want an error", cfg)
	}
}

func TestBuildQualityPlanAllPairsAllSizesOneRunEach(t *testing.T) {
	cfg := defaultConfig(t)
	plan := buildQualityPlan(cfg, false)
	wantSizes := []int{400, 1600, 6400, 12800, 16400, 18200}
	wantPairs := []string{"en>zh-Hans", "zh-Hans>en", "en>es"}
	if len(plan) != len(wantPairs)*len(wantSizes) {
		t.Fatalf("buildQualityPlan(includeJA=false) has %d points, want %d (%d pairs x %d sizes, one run each)",
			len(plan), len(wantPairs)*len(wantSizes), len(wantPairs), len(wantSizes))
	}
	i := 0
	for _, pair := range wantPairs {
		for _, size := range wantSizes {
			if plan[i].pair != pair || plan[i].runes != size {
				t.Errorf("point %d = %+v, want {pair %s, runes %d} (cheapest size first, in pair order)", i, plan[i], pair, size)
			}
			i++
		}
	}
	for _, p := range plan {
		if p.pair == "ja>en" {
			t.Errorf("ja>en planned without includeJA: %+v", p)
		}
	}
}

func TestBuildQualityPlanIncludesJapaneseWhenReady(t *testing.T) {
	cfg := defaultConfig(t)
	plan := buildQualityPlan(cfg, true)
	got := 0
	for _, p := range plan {
		if p.pair == "ja>en" {
			got++
		}
	}
	if got != len(qualityLadderSizes) {
		t.Errorf("ja>en points = %d, want %d (one per ladder size)", got, len(qualityLadderSizes))
	}
}

func TestBuildQualityPlanMaxRunesFilter(t *testing.T) {
	cfg := defaultConfig(t)
	cfg.maxRunes = 1000
	plan := buildQualityPlan(cfg, true)
	for _, p := range plan {
		if p.runes > 1000 {
			t.Errorf("point %+v exceeds KAI_ENGINE_PROBE_MAX_RUNES=1000", p)
		}
	}
	for _, pair := range []string{"en>zh-Hans", "zh-Hans>en", "en>es"} {
		found := false
		for _, p := range plan {
			if p.pair == pair && p.runes == 400 {
				found = true
			}
		}
		if !found {
			t.Errorf("pair %s: the 400-rune point is missing under MAX_RUNES=1000", pair)
		}
	}
}

// --- Similarity (#152) --------------------------------------------------------------------------

func TestSimilarity(t *testing.T) {
	cases := []struct {
		name    string
		a, b    string
		want    float64
		wantAbs float64 // 0 means want exactly; otherwise the max acceptable |got-want|
	}{
		{"both empty", "", "", 1.0, 0},
		{"identical latin", "the quick brown fox", "the quick brown fox", 1.0, 0},
		{"identical cjk", "今天天气很好，所以我们打算步行去市场。", "今天天气很好，所以我们打算步行去市场。", 1.0, 0},
		{"completely different, equal length", "aaaa", "bbbb", 0.0, 0},
		{"one empty", "", "abcdef", 0.0, 0},
		{"single edit out of ten", "abcdefghij", "abcdefghix", 0.9, 0.001},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := similarity(c.a, c.b)
			if got < 0 || got > 1 {
				t.Fatalf("similarity(%q, %q) = %v, out of [0,1]", c.a, c.b, got)
			}
			if c.wantAbs == 0 {
				if got != c.want {
					t.Errorf("similarity(%q, %q) = %v, want exactly %v", c.a, c.b, got, c.want)
				}
			} else if diff := got - c.want; diff > c.wantAbs || diff < -c.wantAbs {
				t.Errorf("similarity(%q, %q) = %v, want %v +/- %v", c.a, c.b, got, c.want, c.wantAbs)
			}
		})
	}
	// Partial overlap: sharing half the runes scores strictly between 0 and 1.
	if s := similarity("abcdefgh", "abcdwxyz"); s <= 0 || s >= 1 {
		t.Errorf("similarity(partial overlap) = %v, want strictly between 0 and 1", s)
	}
	// CJK: compared by rune, not byte, so a one-character edit in a CJK string costs exactly the
	// distance of one rune, the same weight a one-character Latin edit gets, not two or three (as a
	// byte-wise comparison of UTF-8 would score a multi-byte character).
	cjkA := "委员会审查了年度报告"
	cjkB := "委员会审查了年度报告" // identical
	if s := similarity(cjkA, cjkB); s != 1.0 {
		t.Errorf("similarity(identical CJK) = %v, want 1.0", s)
	}
	cjkC := []rune(cjkA)
	cjkC[len(cjkC)-1] = '批' // change exactly one rune
	wantOneRuneEdit := 1 - 1.0/float64(len([]rune(cjkA)))
	if s := similarity(cjkA, string(cjkC)); s != wantOneRuneEdit {
		t.Errorf("similarity(CJK, one rune changed) = %v, want %v (1 - 1/%d runes)", s, wantOneRuneEdit, len([]rune(cjkA)))
	}
}

// --- Structural integrity (#152) ------------------------------------------------------------------

func TestCheckStructureCleanOutputHasNoDefect(t *testing.T) {
	input := "1. First paragraph.\n\n2. Second paragraph.\n\n3. Third paragraph."
	output := "1. Translated first.\n\n2. Translated second.\n\n3. Translated third."
	if d := checkStructure(input, output, 3); d.any() {
		t.Errorf("checkStructure(clean) = %+v, want no defect", d)
	}
}

func TestCheckStructureDroppedParagraph(t *testing.T) {
	input := "1. First paragraph.\n\n2. Second paragraph.\n\n3. Third paragraph."
	output := "1. Translated first.\n\n3. Translated third." // paragraph 2 is missing entirely
	d := checkStructure(input, output, 3)
	if len(d.Dropped) != 1 || d.Dropped[0] != 2 {
		t.Errorf("Dropped = %v, want [2]", d.Dropped)
	}
	if d.Reordered {
		t.Errorf("Reordered = true, want false: the surviving markers (1, 3) are still in order")
	}
	if !d.any() {
		t.Errorf("any() = false, a dropped paragraph is a defect")
	}
}

func TestCheckStructureReorderedMarkers(t *testing.T) {
	input := "1. First paragraph.\n\n2. Second paragraph.\n\n3. Third paragraph."
	output := "1. Translated first.\n\n3. Translated third.\n\n2. Translated second." // 3 before 2
	d := checkStructure(input, output, 3)
	if !d.Reordered {
		t.Errorf("Reordered = false, want true: markers appear as 1, 3, 2")
	}
	if len(d.Dropped) != 0 {
		t.Errorf("Dropped = %v, want none: every marker is present", d.Dropped)
	}
}

func TestCheckStructureRepeatedSentence(t *testing.T) {
	input := "1. The cat sat on the mat. It was warm there.\n\n2. The dog ran in the park."
	// The first sentence of paragraph 1 is duplicated in the output.
	output := "1. The cat sat on the mat. The cat sat on the mat. It was warm there.\n\n2. The dog ran in the park."
	d := checkStructure(input, output, 2)
	if len(d.RepeatedSentences) != 1 || d.RepeatedSentences[0] != "The cat sat on the mat" {
		t.Errorf("RepeatedSentences = %q, want exactly [%q]", d.RepeatedSentences, "The cat sat on the mat")
	}
	if !d.any() {
		t.Errorf("any() = false, a repeated sentence is a defect")
	}
}

func TestCheckStructureRepeatedParagraph(t *testing.T) {
	input := "1. First paragraph text.\n\n2. Second paragraph text."
	output := "1. First paragraph text.\n\n2. Second paragraph text.\n\n1. First paragraph text." // whole paragraph 1 repeated
	d := checkStructure(input, output, 2)
	if len(d.RepeatedParagraphs) != 1 {
		t.Errorf("RepeatedParagraphs = %q, want exactly one repeated paragraph", d.RepeatedParagraphs)
	}
	if !d.any() {
		t.Errorf("any() = false, a repeated paragraph is a defect")
	}
}

func TestCheckStructureSentenceRepeatedOnceInInputIsNotFlagged(t *testing.T) {
	// A sentence that already repeats in the INPUT (e.g. a refrain) is not itself a translation
	// defect: only a sentence that is unique in the input but duplicated in the output counts.
	input := "1. We agree. We agree.\n\n2. Something else entirely."
	output := "1. We agree. We agree.\n\n2. Something else entirely."
	d := checkStructure(input, output, 2)
	if len(d.RepeatedSentences) != 0 {
		t.Errorf("RepeatedSentences = %q, want none: \"We agree\" already repeats in the input", d.RepeatedSentences)
	}
}

// #152's real defect was found and fixed before the probe's first real run: it compared OUTPUT text
// (a translation) against INPUT text (the untranslated source) by exact string equality — but input
// and output are never the same language, so that comparison can essentially never match, and the
// repetition check could never fire on a real translation. The three tests below exercise the actual
// cross-language case, standing in Chinese-shaped placeholder text for "a real translation" so the
// fix is proven against text that genuinely shares nothing textual with the input, the way a real
// Apple call's result does.

func TestCheckStructureRepeatedSentenceCrossLanguage(t *testing.T) {
	// Nothing in output is textually equal to anything in input — a real translation. Output's
	// second sentence is degenerately duplicated; a same-string comparison against input could never
	// catch this because no output sentence ever equals an input sentence.
	input := "1. The cat sat on the mat. It was warm there.\n\n2. The dog ran in the park."
	output := "1. 猫在垫子上. 猫在垫子上. 那里很暖和.\n\n2. 狗在公园里跑."
	d := checkStructure(input, output, 2)
	if len(d.RepeatedSentences) != 1 || d.RepeatedSentences[0] != "猫在垫子上" {
		t.Errorf("RepeatedSentences = %q, want exactly [%q]", d.RepeatedSentences, "猫在垫子上")
	}
	if !d.any() {
		t.Errorf("any() = false, a repeated sentence is a defect even across languages")
	}
}

func TestCheckStructureLegitimatePoolCyclingIsNotFlagged(t *testing.T) {
	// At a large probe size, script.paragraph/stream (probe_test.go) legitimately cycles the
	// sentence pool ("(cursor+i)%len(s.sentences)"), so INPUT itself can legitimately repeat a
	// sentence — here sentence A recurs, exactly as pool cycling would produce. A faithful
	// translation repeats it proportionally too (same rate, different text). That is not a defect:
	// output's self-repetition rate is no higher than input's own baseline.
	input := "1. Sentence A. Sentence B.\n\n2. Sentence A. Sentence B." // A repeats once, by design (pool wraparound)
	output := "1. 句子甲. 句子乙.\n\n2. 句子甲. 句子乙."                            // faithfully repeats it once too
	d := checkStructure(input, output, 2)
	if d.any() {
		t.Errorf("checkStructure(faithful repeat of legitimately-cycled input) = %+v, want no defect", d)
	}
}

func TestCheckStructureDegenerationBeyondInputsOwnRateIsFlagged(t *testing.T) {
	// Input has some legitimate repetition (pool cycling), but output repeats far more than that
	// baseline — real NMT degeneration, not a faithful echo of the source's own cycling.
	input := "1. Sentence A. Sentence B. Sentence C. Sentence D.\n\n2. Sentence A. Sentence E. Sentence F. Sentence G."
	output := "1. 句子甲. 句子甲. 句子甲. 句子甲.\n\n2. 句子甲. 句子甲. 句子甲. 句子甲." // collapsed to one sentence, repeated
	d := checkStructure(input, output, 2)
	if len(d.RepeatedSentences) == 0 {
		t.Errorf("RepeatedSentences = none, want at least one: output collapsed far past input's own repetition rate")
	}
	if !d.any() {
		t.Errorf("any() = false, want a defect: output's self-repetition rate is far above input's baseline")
	}
}
