package enginelimits

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

// The Apple probe's planning, statistics and bookkeeping (issue #119). These are pure helpers: no
// Apple call, no engine or swiftbridge import, no build tag, so plan_test.go tests them in the
// authoritative suite. The probe that uses them (TestProbeApple in probe_test.go) stays behind the
// enginelimits build tag and KAI_ENGINE_PROBE=1, and so do the settings below: probeConfigFromEnv is
// called only after that gate.

// The knobs of a probe session, environment variables read by probeConfigFromEnv.
const (
	repeatsEnv      = "KAI_ENGINE_PROBE_REPEATS"       // runs per ladder size (default 3, at least 3)
	budgetMinEnv    = "KAI_ENGINE_PROBE_BUDGET_MIN"    // the session's wall-clock cap in minutes (default 90)
	maxRunesEnv     = "KAI_ENGINE_PROBE_MAX_RUNES"     // drop planned sizes above it (the smoke run: 1000)
	skipBoundaryEnv = "KAI_ENGINE_PROBE_SKIP_BOUNDARY" // 1 leaves out phase 2 (the second session)
	statusEnv       = "KAI_ENGINE_PROBE_STATUS"        // the progress file (default: statusPath(""))
)

const (
	defaultRepeats   = 3
	minRepeats       = 3 // the deliverable needs at least three runs per ladder size
	defaultBudgetMin = 90
)

// probeConfig is one probe session's settings.
type probeConfig struct {
	repeats      int           // runs per ladder size; buildPlan refuses fewer than minRepeats
	maxRunes     int           // planned sizes above this are dropped; 0 means no bound
	budget       time.Duration // the session's wall-clock cap
	skipBoundary bool          // leave out phase 2 (the second session may)
	includeJA    bool          // plan phase 3; the probe sets it when its precheck finds Japanese installed
	statusPath   string        // the progress file
}

// probeConfigFromEnv reads the session's knobs through getenv (os.Getenv in the probe). A value
// that does not parse is an error, never a silent default: a typo would otherwise run a session
// other than the one asked for.
func probeConfigFromEnv(getenv func(string) string) (probeConfig, error) {
	cfg := probeConfig{
		repeats:    defaultRepeats,
		budget:     defaultBudgetMin * time.Minute,
		statusPath: statusPath(getenv(statusEnv)),
	}
	if v := getenv(repeatsEnv); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return probeConfig{}, fmt.Errorf("%s=%q: want a whole number of runs per ladder size", repeatsEnv, v)
		}
		cfg.repeats = n
	}
	if v := getenv(budgetMinEnv); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return probeConfig{}, fmt.Errorf("%s=%q: want a whole number of minutes above 0", budgetMinEnv, v)
		}
		cfg.budget = time.Duration(n) * time.Minute
	}
	if v := getenv(maxRunesEnv); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return probeConfig{}, fmt.Errorf("%s=%q: want a whole number of runes (0 for no bound)", maxRunesEnv, v)
		}
		cfg.maxRunes = n
	}
	switch v := getenv(skipBoundaryEnv); v {
	case "", "0":
	case "1":
		cfg.skipBoundary = true
	default:
		return probeConfig{}, fmt.Errorf("%s=%q: want 1 (leave out the boundary phase) or 0", skipBoundaryEnv, v)
	}
	return cfg, nil
}

// --- The plan ----------------------------------------------------------------------------------

// The phases of a planned run, in the order they run. The optional ja>en ladder (phase 3) is a
// ladder too; it is planned after the boundary.
const (
	phaseLadder   = "ladder"   // fixed sizes, each repeated: the spread and the latency curve
	phaseBoundary = "boundary" // single runs that bracket the zh-Hans>en failure
	phaseDone     = "done"     // the status line written when the run ends
)

// The language pairs, named as the plan, the PROBE lines and the status file name them: the
// framework's source and target codes.
const (
	pairENZH = "en>zh-Hans" // Latin source; the pair of the epic's 20,000-character run
	pairZHEN = "zh-Hans>en" // CJK source; where the output buffer binds (#111)
	pairENES = "en>es"      // Latin to Latin: a target that is not CJK and not much longer than the source
	pairJAEN = "ja>en"      // optional, when Japanese is installed
)

// capRunes is the largest size the probe tries, the epic's 20,000-character document. A size that
// passes there is a capped pass (">= 20000" in the doc), not a measured ceiling.
const capRunes = 20_000

// ladder is one pair's fixed sizes, cheapest first.
type ladder struct {
	pair     string
	sizes    []int
	optional bool // phase 3: planned only with includeJA, and after the boundary
}

// ladders is phases 1 and 3. Order matters: the time cap stops the run from the end, so the two
// pairs the issue names come first and the optional pair last.
var ladders = []ladder{
	{pair: pairENZH, sizes: []int{400, 1_600, 6_400, capRunes}},
	{pair: pairZHEN, sizes: []int{400, 1_600, 6_400}},
	{pair: pairENES, sizes: []int{400, 1_600, 6_400}},
	{pair: pairJAEN, sizes: []int{400, 1_600, 6_400}, optional: true},
}

// planPoint is one planned translation.
type planPoint struct {
	phase, pair string
	runes       int // the input size
	repeat      int // 1..repeats for a ladder size; 1 for a boundary point (2 for its confirmation)
}

// buildPlan returns a session's ordered plan: every ladder size of every pair repeated
// cfg.repeats times, cheapest first; then the one planned boundary point (zh-Hans>en at the first
// boundary seed, a single run: the probe bisects from its result at run time); then, with
// includeJA, the ja>en ladder. Sizes above cfg.maxRunes are dropped, which is how the smoke run
// (1000) keeps only the 400-rune points. Fewer than minRepeats runs per size, or a bound that leaves
// no point at all, is an error.
func buildPlan(cfg probeConfig) ([]planPoint, error) {
	if cfg.repeats < minRepeats {
		return nil, fmt.Errorf("%s=%d: the deliverable needs at least %d runs per ladder size", repeatsEnv, cfg.repeats, minRepeats)
	}
	fits := func(n int) bool { return cfg.maxRunes == 0 || n <= cfg.maxRunes }
	var plan []planPoint
	addLadders := func(optional bool) {
		for _, l := range ladders {
			if l.optional != optional || (optional && !cfg.includeJA) {
				continue
			}
			for _, n := range l.sizes {
				if !fits(n) {
					continue
				}
				for r := 1; r <= cfg.repeats; r++ {
					plan = append(plan, planPoint{phase: phaseLadder, pair: l.pair, runes: n, repeat: r})
				}
			}
		}
	}
	addLadders(false)
	if start := boundarySeeds[0]; !cfg.skipBoundary && fits(start) {
		plan = append(plan, planPoint{phase: phaseBoundary, pair: pairZHEN, runes: start, repeat: 1})
	}
	addLadders(true)
	if len(plan) == 0 {
		return nil, fmt.Errorf("%s=%d leaves no planned point: the smallest ladder size is %d runes", maxRunesEnv, cfg.maxRunes, ladders[0].sizes[0])
	}
	return plan, nil
}

// --- The boundary (phase 2) --------------------------------------------------------------------

// The #111 run's answer for zh-Hans>en: 18,200 runes passed and 19,100 failed, on the output buffer.
const (
	prior111Pass = 18_200
	prior111Fail = 19_100
)

// boundarySeeds are the sizes phase 2 climbs while nothing has failed: its planned start, then the
// #111 sizes, then capRunes.
var boundarySeeds = []int{12_800, 16_400, prior111Pass, prior111Fail, capRunes}

// The bisection stops when fail-pass <= max(stepFloor, stepPct% of pass). Every point costs its
// full translation time even when it fails (about 9 minutes for 19,000 CJK runes), and the budget
// applies its own 20% margin, so a bracket of 5% is well inside it.
const (
	stepFloor = 50
	stepPct   = 5
)

// bracketDone reports whether the bracket between the largest passing and the smallest failing
// size is narrow enough to stop.
func bracketDone(pass, fail int) bool { return fail-pass <= max(stepFloor, pass*stepPct/100) }

// nextBoundary is phase 2's one bisection. With no failure yet (fail == 0) it returns the first
// seed above pass, and is done once capRunes has passed (a capped pass). With a failure it returns
// the midpoint of the bracket, and is done once bracketDone. It assumes failure is monotonic in
// size; every point is logged, so a result that is not is visible.
func nextBoundary(pass, fail int) (next int, done bool) {
	if fail == 0 {
		for _, s := range boundarySeeds {
			if s > pass {
				return s, false
			}
		}
		return 0, true
	}
	if bracketDone(pass, fail) {
		return 0, true
	}
	return pass + (fail-pass)/2, false
}

// --- The time cap ------------------------------------------------------------------------------

// seedMsPerRune projects a pair's first point, before it has a run of its own: the slowest rate on
// record, rounded up (1,600 CJK runes in 63.4 s, 39.6 ms per rune, in the #111 run on a loaded
// machine). Small sizes cost more per rune than large ones, so the seed errs long.
const seedMsPerRune = 40.0

// shouldStart reports whether a point projected to take projected may start at elapsed under the
// session cap limit: only when elapsed + 1.25 x projected fits, and never once elapsed has reached
// the cap. A point that has started is never cut off to meet the cap.
func shouldStart(elapsed, limit, projected time.Duration) bool {
	return elapsed < limit && elapsed+projected+projected/4 <= limit
}

// projectPoint projects the time of a point of runes: its size times the median milliseconds per
// rune of the runs so far (the probe keeps them per pair), or times seedMsPerRune with none yet.
func projectPoint(runes int, msPerRune []float64) time.Duration {
	rate := seedMsPerRune
	if len(msPerRune) > 0 {
		rate = median(msPerRune)
	}
	return time.Duration(float64(runes) * rate * float64(time.Millisecond))
}

// --- Statistics --------------------------------------------------------------------------------

// Machine load, the 1-minute load average (vm.loadavg): a session starts only at quietLoad or
// below, and a run that starts above noisyLoad is noisy (listed, left out of the statistics).
const (
	quietLoad = 10.0
	noisyLoad = 12.0
)

// smokeMaxRunes is the largest KAI_ENGINE_PROBE_MAX_RUNES that still counts as the smoke run. A run
// bounded above it is a real (capped) session and waits for a quiet machine like any other.
const smokeMaxRunes = 1000

// isSmokeRun reports that the run only checks the harness (nine 400-rune points): it records the load
// and does not wait for a quiet machine.
func isSmokeRun(cfg probeConfig) bool { return cfg.maxRunes > 0 && cfg.maxRunes <= smokeMaxRunes }

func isNoisy(load1 float64) bool      { return load1 > noisyLoad }
func quietToStart(load1 float64) bool { return load1 <= quietLoad }

// sample is one run's latency.
type sample struct {
	latency time.Duration
	noisy   bool // the run started above noisyLoad
}

// latencyStats summarizes the runs of one size.
type latencyStats struct {
	min, median, max time.Duration   // over the runs that are not noisy; zero when there is none
	runs             []time.Duration // every run, in order, noisy ones included
}

// summarize lists every run and takes min, median and max over the ones that are not noisy (the
// median of an even count is the mean of the two middle values). No samples, or only noisy ones,
// give zero statistics, never a panic.
func summarize(samples []sample) latencyStats {
	var st latencyStats
	var quiet []time.Duration
	for _, s := range samples {
		st.runs = append(st.runs, s.latency)
		if !s.noisy {
			quiet = append(quiet, s.latency)
		}
	}
	if len(quiet) == 0 {
		return st
	}
	st.min, st.max, st.median = slices.Min(quiet), slices.Max(quiet), median(quiet)
	return st
}

// median returns the middle value of xs (the mean of the two middle values for an even count), or
// zero for none. xs is left as it was.
func median[T time.Duration | float64](xs []T) T {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	m := len(s) / 2
	if len(s)%2 == 1 {
		return s[m]
	}
	return (s[m-1] + s[m]) / 2
}

// sizeResult is one run's verdict, for maxPassing.
type sizeResult struct {
	runes       int
	pass, noisy bool
}

// maxPassing returns the largest size at which every run that is not noisy passed: one failed run
// disqualifies its size (all runs count, not the median), a noisy run is no evidence either way, and
// a size with only noisy runs is not a maximum. capped reports that the maximum is capSize itself (a
// capped pass). No passing size gives 0.
func maxPassing(results []sizeResult, capSize int) (best int, capped bool) {
	seen, failed := map[int]bool{}, map[int]bool{}
	for _, r := range results {
		if r.noisy {
			continue
		}
		seen[r.runes] = true
		if !r.pass {
			failed[r.runes] = true
		}
	}
	for n := range seen {
		if !failed[n] && n > best {
			best = n
		}
	}
	return best, best > 0 && best == capSize
}

// sessionDisagreePct is how far two sessions' maxima may differ, as a share of the smaller, before
// they disagree (the #83 runs differed by about 20% on Latin).
const sessionDisagreePct = 20

// combineSessions returns the smaller of two sessions' maxima, and whether they disagree by more
// than sessionDisagreePct.
func combineSessions(a, b int) (int, bool) {
	lo, hi := min(a, b), max(a, b)
	return lo, (hi-lo)*100 > lo*sessionDisagreePct
}

// --- The output buffer -------------------------------------------------------------------------

// appleOutBufPayloadMax is the most the Apple engine can receive: Translate in apple_darwin.go
// allocates a 64 KiB output buffer (#106 owns it) and the bridge writes a NUL-terminated payload
// into it, so one byte less. This mirrors that inline figure, which an untagged file cannot import;
// guard_test.go pins the mirror to the line in apple_darwin.go.
const appleOutBufPayloadMax = 1<<16 - 1

// jsonTruncated is encoding/json's message for input that ends early: the error the engine returns
// (wrapped with %w) for a payload the output buffer cut off.
const jsonTruncated = "unexpected end of JSON input"

// bufferBound reports whether a failed run is evidence that the output buffer binds: its error is
// the JSON decode of a truncated payload, and the output it would have produced exceeds
// appleOutBufPayloadMax. outBytes is a measured output size when there is one (the same input run
// with a larger buffer, as in #111; the probe has none for a failed run and passes 0), and
// projectedBytes the projection from the pair's passing runs (projectOutBytes); the larger counts.
//
// Both are bytes of the translated text (out_bytes), not of the JSON payload the buffer holds. The
// payload adds the {"result":…,"from":…} wrapper and its escapes (each paragraph break is written
// \n\n), about 1-2% more, so a projection a little under the maximum can still have overflowed.
func bufferBound(outBytes, projectedBytes int, err error) bool {
	var syntax *json.SyntaxError
	if !errors.As(err, &syntax) || syntax.Error() != jsonTruncated {
		return false
	}
	return max(outBytes, projectedBytes) > appleOutBufPayloadMax
}

// projectOutBytes projects the output bytes of an input of inRunes: the median output bytes per
// input rune of the passing runs, times inRunes; 0 with no passing run.
func projectOutBytes(inRunes int, outBytesPerRune []float64) int {
	if len(outBytesPerRune) == 0 {
		return 0
	}
	return int(math.Round(float64(inRunes) * median(outBytesPerRune)))
}

// --- The progress file -------------------------------------------------------------------------

// statusLine is one line of the progress file. Every field is always written, so a watcher can
// read any line the same way.
type statusLine struct {
	Time        time.Time `json:"time"` // RFC 3339, with the zone
	ElapsedS    int       `json:"elapsed_s"`
	Phase       string    `json:"phase"`
	Pair        string    `json:"pair"`
	Runes       int       `json:"runes"`
	UTF8Bytes   int       `json:"utf8_bytes"`
	Repeat      int       `json:"repeat"`
	LatencyMs   int64     `json:"latency_ms"`
	OutBytes    int       `json:"out_bytes"`
	Kind        string    `json:"kind"`
	Load1       float64   `json:"load1"`
	Next        string    `json:"next"` // the next planned point
	BudgetLeftS int       `json:"budget_left_s"`
}

// appendStatus appends l to the file at path as one JSON line, creating the file if needed. It
// never truncates: a watcher tails the file, and an earlier session's lines stay.
func appendStatus(path string, l statusLine) error {
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// statusFile is the default progress file's name. It lives in the temp dir, outside any checkout,
// so it can never be committed.
const statusFile = "kai-probe-status.jsonl"

// statusPath returns the progress file: override when it is set, else statusFile in the temp dir.
func statusPath(override string) string {
	if override != "" {
		return override
	}
	return filepath.Join(os.TempDir(), statusFile)
}

// --- Between runs ------------------------------------------------------------------------------

// The kinds of a run's outcome.
const (
	kindPass      = "pass"
	kindError     = "error"     // Translate returned an error
	kindTruncated = "truncated" // no error, but a paragraph number is missing from the output
	kindShort     = "short"     // no error and every number present, but the output is implausibly small
)

// pointCeiling is the probe's own ceiling on one run. The bridge has no limit since #111, so a
// call this long means the framework is not answering, and the probe stops with that cause instead
// of recording a limit. The run guide's go test -timeout covers the budget plus one such point.
const pointCeiling = 20 * time.Minute

// probeClock is the time settle runs on: the wall clock in the probe, a fake one in its tests.
type probeClock interface {
	Now() time.Time
	Sleep(time.Duration)
}

// What settle did, for the log.
const (
	drainNone    = "none"    // the previous run completed: a plain pause
	drainOK      = "ok"      // a warm call came back fast: nothing was queued
	drainTimeout = "timeout" // no fast warm call within drainLimit
)

const (
	settlePause  = 5 * time.Second  // between two runs that completed
	drainFast    = 5 * time.Second  // a warm call under this means the framework's queue is empty
	drainLimit   = 60 * time.Second // no warm call starts this long after the drain began
	drainBackoff = time.Second      // after a warm call that failed, before the next
)

// settle waits between two runs. After a run that completed (a pass, or a result that failed its
// checks) the framework is idle, so it pauses settlePause. After an error it cannot know that: the
// framework may still be working, and would queue the next run behind it. So it sends warm
// (one-sentence) calls until one succeeds in under drainFast, and starts none after drainLimit.
// The probe never cancels a measured run, so a cancel never leaves work behind.
func settle(prevKind string, c probeClock, warm func() (time.Duration, error)) string {
	switch prevKind {
	case kindPass, kindTruncated, kindShort:
		c.Sleep(settlePause)
		return drainNone
	}
	start := c.Now()
	for c.Now().Sub(start) < drainLimit {
		d, err := warm()
		if err == nil && d < drainFast {
			return drainOK
		}
		if err != nil {
			c.Sleep(drainBackoff)
		}
	}
	return drainTimeout
}
