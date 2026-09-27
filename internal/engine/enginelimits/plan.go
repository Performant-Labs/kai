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
	"strings"
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

	// qualityDeeplEnv gates issue #153's DeepL reference calls inside the quality probe
	// (TestProbeQuality, quality_probe_test.go): 1 turns them on. Left unset (or 0), #152's
	// structural and round-trip checks run and cost nothing — no DeepL call is ever made. It is
	// read here, like every other knob, not by an ad hoc os.Getenv elsewhere.
	qualityDeeplEnv = "KAI_ENGINE_PROBE_DEEPL"
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
	deepl        bool          // #153: the quality probe also calls DeepL as a reference and scores divergence
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
	switch v := getenv(qualityDeeplEnv); v {
	case "", "0":
	case "1":
		cfg.deepl = true
	default:
		return probeConfig{}, fmt.Errorf("%s=%q: want 1 (call DeepL as a reference translator) or 0", qualityDeeplEnv, v)
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

// --- The quality probe's plan (issues #152/#153, epic #151) --------------------------------------

// qualityLadderSizes is the six input sizes both quality probes measure: the epic's own ladder
// (#151), the same sizes #119 already measured latency at. Unlike the latency probe's ladders
// (which vary by pair, and go to capRunes only for en>zh-Hans), every pair gets exactly these six
// sizes, one run each — a quality trend across size, not a latency spread.
var qualityLadderSizes = []int{400, 1_600, 6_400, 12_800, 16_400, 18_200}

// qualityPairs is every pair the quality probes measure: all four scripts TestProbeApple already
// knows (en>zh-Hans, zh-Hans>en, en>es, ja>en), ja>en included only when it is ready, the same way
// TestProbeApple's own plan does.
var qualityPairs = []string{pairENZH, pairZHEN, pairENES, pairJAEN}

// qualityPoint is one planned quality-probe measurement: one pair at one size. There is no repeat
// field (unlike planPoint): #151 wants one run per point, not #119's three.
type qualityPoint struct {
	pair  string
	runes int
}

// buildQualityPlan returns every (pair, size) quality-probe point, cheapest size first within each
// pair, for the pairs in qualityPairs that are ready (includeJA gates ja>en exactly as
// TestProbeApple's own precheck does). cfg.maxRunes (KAI_ENGINE_PROBE_MAX_RUNES) drops sizes above
// it, the same smoke-run convention buildPlan uses, so KAI_ENGINE_PROBE_MAX_RUNES=1000 leaves only
// the 400-rune points. cfg.repeats plays no part: the quality probe never repeats a point.
func buildQualityPlan(cfg probeConfig, includeJA bool) []qualityPoint {
	fits := func(n int) bool { return cfg.maxRunes == 0 || n <= cfg.maxRunes }
	var plan []qualityPoint
	for _, pair := range qualityPairs {
		if pair == pairJAEN && !includeJA {
			continue
		}
		for _, n := range qualityLadderSizes {
			if fits(n) {
				plan = append(plan, qualityPoint{pair: pair, runes: n})
			}
		}
	}
	return plan
}

// --- Similarity (issue #152) ----------------------------------------------------------------------

// similarity returns a deterministic likeness score in [0,1] between a and b: 1 minus the
// Levenshtein edit distance (insertions, deletions and substitutions, each cost 1) between their
// runes, normalized by the longer string's rune count. Two empty strings score 1.0 (identical, not
// undefined); otherwise the score is exact — 1.0 only for identical strings, and it falls as the
// strings diverge. It operates on runes, never bytes, so one CJK character counts as one unit the
// same way one Latin character does; comparing UTF-8 bytes instead would let a single multi-byte
// character's edit weigh two or three times a Latin one's, and would let byte-level slicing split a
// character in the middle. This is deliberately not an LLM judge: no external call, no cost, fully
// deterministic, so #152's round-trip score and #153's Apple/DeepL divergence score are free and
// repeatable.
func similarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 && len(rb) == 0 {
		return 1.0
	}
	longest := max(len(ra), len(rb))
	return 1 - float64(levenshtein(ra, rb))/float64(longest)
}

// levenshtein returns the edit distance between a and b (insert, delete, substitute, each cost 1),
// the classic two-row dynamic-programming form: O(len(a)*len(b)) time, O(min(len(a),len(b))) space.
func levenshtein(a, b []rune) int {
	if len(a) < len(b) {
		a, b = b, a
	}
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	cur := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(cur[j-1]+1, min(prev[j]+1, prev[j-1]+cost))
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// --- Structural integrity (issue #152) --------------------------------------------------------

// structuralDefect summarizes the structural defects checkStructure found in one translation, each
// a known NMT failure mode at scale (#151's epic). A zero value (any() false) means the checked
// output has none of them.
type structuralDefect struct {
	Dropped            []int    // paragraph numbers 1..paragraphs present in the input's markers, absent from the output
	Reordered          bool     // paragraph markers present in the output, but not in the input's order
	RepeatedSentences  []string // sentences appearing more than once in the output, exactly once in the input
	RepeatedParagraphs []string // paragraphs appearing more than once in the output, exactly once in the input
}

// any reports whether any defect fired.
func (d structuralDefect) any() bool {
	return len(d.Dropped) > 0 || d.Reordered || len(d.RepeatedSentences) > 0 || len(d.RepeatedParagraphs) > 0
}

// markerSequence returns every digit run in text, in the order it appears, using the same
// digit-parsing rule firstMissingMarker uses (clamped so an absurdly long digit run cannot wrap
// around into a false match). Unlike firstMissingMarker, which stops at the first gap, this keeps
// the whole sequence, in order and with duplicates, so checkStructure can also detect reordering
// and (via the paragraph text itself) repetition.
func markerSequence(text string) []int {
	var out []int
	cur, in := 0, false
	flush := func() {
		if in {
			out = append(out, cur)
		}
		cur, in = 0, false
	}
	for _, r := range text {
		if r >= '0' && r <= '9' {
			cur, in = min(cur*10+int(r-'0'), 1<<30), true
			continue
		}
		flush()
	}
	flush()
	return out
}

// splitParagraphs splits text on the blank line buildInput separates paragraphs with, dropping
// empty pieces.
func splitParagraphs(text string) []string {
	var out []string
	for _, p := range strings.Split(text, "\n\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// splitSentences splits text on the sentence terminators the probe's scripts use (Latin '.', '!',
// '?' and their CJK full-width equivalents), dropping empty pieces. A heuristic, not a parser: good
// enough to compare a sentence's presence and count between input and output.
func splitSentences(text string) []string {
	isTerminator := func(r rune) bool {
		switch r {
		case '.', '!', '?', '。', '！', '？':
			return true
		}
		return false
	}
	var out []string
	for _, s := range strings.FieldsFunc(text, isTerminator) {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// selfRepeatedUnits returns each unit (sentences or paragraphs, already split and trimmed) that
// appears more than once within units, in first-occurrence order, each named once regardless of how
// many times it recurs.
func selfRepeatedUnits(units []string) []string {
	count := map[string]int{}
	var order []string
	for _, u := range units {
		if count[u] == 0 {
			order = append(order, u)
		}
		count[u]++
	}
	var repeated []string
	for _, u := range order {
		if count[u] > 1 {
			repeated = append(repeated, u)
		}
	}
	return repeated
}

// selfRepeatFraction returns the share of units, by count, that are a second-or-later occurrence of
// a unit already seen earlier in the same slice: (total - distinct) / total. 0 for an empty slice
// (nothing to repeat), never negative, less than 1 unless every unit is identical.
func selfRepeatFraction(units []string) float64 {
	if len(units) == 0 {
		return 0
	}
	seen := map[string]bool{}
	distinct := 0
	for _, u := range units {
		if !seen[u] {
			seen[u] = true
			distinct++
		}
	}
	return float64(len(units)-distinct) / float64(len(units))
}

// repeatMargin is how much higher the OUTPUT's own self-repetition rate must be than the INPUT's own
// self-repetition rate before checkStructure counts it as translation-introduced repetition. Margin,
// not a bare "any repeat", because the probe's own sentence pool legitimately cycles at large sizes
// (script.paragraph/stream, probe_test.go: "(cursor+i)%len(s.sentences)"), so a large-size input can
// legitimately repeat a source sentence — and a faithful translation of that repeated sentence then
// legitimately repeats too. Comparing self-repetition RATES, not raw text across the two languages
// (input and output are never the same language), is what makes this check work at every size: a
// real degeneration shows up as output repeating distinctly more than its own source already did.
const repeatMargin = 0.15

// checkStructure extends firstMissingMarker's paragraph-marker check (probe_test.go) with the three
// defects #152 asks for: dropped paragraphs (a marker in 1..paragraphs missing from the output),
// reordered markers (present, but not in input order), and repeated sentences or paragraphs — the
// known large-input NMT failure mode, detected via selfRepeatFraction: input and output are never
// the same language, so a defect can never be "the same text appears in both"; it is "output repeats
// noticeably more, within itself, than input already legitimately does within itself." input is the
// untranslated probe text; output is the translation to check; paragraphs is buildInput's paragraph
// count for this point.
func checkStructure(input, output string, paragraphs int) structuralDefect {
	var d structuralDefect
	present := map[int]bool{}
	var order []int
	for _, n := range markerSequence(output) {
		if n < 1 || n > paragraphs || present[n] {
			continue
		}
		present[n] = true
		order = append(order, n)
	}
	for n := 1; n <= paragraphs; n++ {
		if !present[n] {
			d.Dropped = append(d.Dropped, n)
		}
	}
	for i := 1; i < len(order); i++ {
		if order[i] < order[i-1] {
			d.Reordered = true
			break
		}
	}
	inSent, outSent := splitSentences(input), splitSentences(output)
	if selfRepeatFraction(outSent)-selfRepeatFraction(inSent) > repeatMargin {
		d.RepeatedSentences = selfRepeatedUnits(outSent)
	}
	inPara, outPara := splitParagraphs(input), splitParagraphs(output)
	if selfRepeatFraction(outPara)-selfRepeatFraction(inPara) > repeatMargin {
		d.RepeatedParagraphs = selfRepeatedUnits(outPara)
	}
	return d
}
