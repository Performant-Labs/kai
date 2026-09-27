//go:build enginelimits && darwin

package enginelimits

// The Apple quality probe (issues #152/#153, epic #151): does translation quality degrade before
// Apple's Translation.framework fails outright, on input that still succeeds? Opt-in exactly like
// TestProbeApple: build tag enginelimits (darwin) AND KAI_ENGINE_PROBE=1. See
// docs/quality-limits.md for the exact command and the knobs. It shares TestMain, the prober, the
// scripts (latin/cjk/latinES/japanese) and buildInput with probe_test.go, and the budget/settle/
// status mechanics (shouldStart, projectPoint, settle, statusLine) with the latency probe's
// session, rather than reimplementing any of it — this file adds only what #152/#153 need beyond
// that: the per-point flow (forward call, structural check, round trip, optional DeepL reference)
// and its own plan (buildQualityPlan, plan.go), which has no ladder repeats or boundary phase.
//
// At each planned (pair, size) point:
//  1. translate forward with Apple (prober.call) and run checkStructure on the result, logging
//     enough of the output for a human to read directly when a defect fired (#152's "direct read");
//  2. translate the forward result back to the source language (a second Apple call) and score it
//     against the original input with similarity (plan.go) — a round-trip drift score;
//  3. with KAI_ENGINE_PROBE_DEEPL=1, call DeepL on the same source text (engine.NewDeepL, config
//     loaded the same way internal/service/engine_wrapper.go loads it) and score Apple's forward
//     result against DeepL's with the same similarity helper (#153's divergence signal). Off by
//     default: #152's checks above cost nothing and make no DeepL call.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"cnb.cool/dtapp/kai/internal/buildinfo"
	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/model"
)

// reversed returns the script with source and target swapped, for the round-trip call: the same
// prose, the same pair's two codes, in the other direction. Sentences and separators are unchanged
// (only used to build the *forward* input; the round trip translates whatever Apple returned).
func (s script) reversed() script {
	s.from, s.to = s.to, s.from
	s.fromCode, s.toCode = s.toCode, s.fromCode
	return s
}

// translateFunc is one text translated from -> to, the shape deeplRef returns: enough to score
// against Apple's own result with similarity, and nothing engine-specific leaks into qualitySession.
type translateFunc func(ctx context.Context, text string, from, to model.Language) (string, error)

// deeplRef opens the app's own config database (buildinfo.DBDir, ~/.kai.dev or ~/.kai) and builds
// the DeepL engine from its "deepl" row, exactly the way internal/service/engine_wrapper.go and
// internal/translate/service.go load an engine: configstore.Open then GetEngineByName. Called only
// when cfg.deepl (KAI_ENGINE_PROBE_DEEPL=1); a missing or key-less row stops the run with the cause,
// since the operator asked for the DeepL reference explicitly.
func deeplRef(t *testing.T) translateFunc {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("probe prerequisite failed: %s=1 but $HOME could not be resolved: %v", qualityDeeplEnv, err)
	}
	dbPath := filepath.Join(buildinfo.DBDir(home), "config.db")
	store, err := configstore.Open(dbPath)
	if err != nil {
		t.Fatalf("probe prerequisite failed: %s=1 but opening %s failed: %v", qualityDeeplEnv, dbPath, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	row, err := store.GetEngineByName(ctx, "deepl")
	if err != nil {
		t.Fatalf("probe prerequisite failed: %s=1 but reading the deepl engine from %s failed: %v", qualityDeeplEnv, dbPath, err)
	}
	if row == nil || row.APIKey == "" {
		t.Fatalf("probe prerequisite failed: %s=1 but %s has no deepl engine with an API key configured. "+
			"Configure DeepL in Kai's own Settings first (internal/engine/deepl.go reads it the normal way; nothing new to wire).",
			qualityDeeplEnv, dbPath)
	}
	tr := engine.NewDeepL(row, http.DefaultClient)
	return func(ctx context.Context, text string, from, to model.Language) (string, error) {
		res, err := tr.Translate(ctx, model.TranslateRequest{Text: text, From: from, To: to})
		if err != nil {
			return "", err
		}
		return res.Result, nil
	}
}

// snippet returns up to n runes of s, so a PROBE log line carries enough of a defective output for
// a human to read directly (#152's "direct read" requirement), without dumping the whole thing.
func snippet(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// snippetList joins snippet(x, each) for every x, for logging a short list of repeated units.
func snippetList(xs []string, each int) string {
	if len(xs) == 0 {
		return "-"
	}
	parts := make([]string, len(xs))
	for i, x := range xs {
		parts[i] = snippet(x, each)
	}
	return strings.Join(parts, " | ")
}

// qualitySession is one #152/#153 run: as many quality-probe points as the budget and the ready
// scripts allow, in plan order (cheapest size first per pair). It has no ladder repeats and no
// boundary phase (buildQualityPlan is a flat list), so it is simpler than session, but reuses the
// same budget/settle/status building blocks that session uses.
type qualitySession struct {
	p         *prober
	cfg       probeConfig
	started   time.Time
	scripts   map[string]script
	deepl     translateFunc // nil unless cfg.deepl and configured (deeplRef)
	msPerRune map[string][]float64
	lastKind  string // drives settle before the next point; "" before the first
	partial   bool
}

func (qs *qualitySession) elapsed() time.Duration { return time.Since(qs.started) }
func (qs *qualitySession) elapsedS() int          { return int(qs.elapsed().Seconds()) }
func (qs *qualitySession) budgetLeftS() int {
	return max(0, int((qs.cfg.budget - qs.elapsed()).Seconds()))
}

// warm is the one-sentence request settle drains the framework's queue with, the same pattern
// session.warm uses.
func (qs *qualitySession) warm(s script) func() (time.Duration, error) {
	return func() (time.Duration, error) {
		res, d, err := qs.p.call(s, s.warm, precheckCeiling)
		if err == nil && res.Result == "" {
			err = errors.New("empty result")
		}
		qs.p.t.Logf("PROBE quality_warm pair=%s latency_ms=%d error=%q", s.pair(), d.Milliseconds(), errText(err))
		return d, err
	}
}

// writeStatus appends one line to the progress file, reusing appendStatus/statusLine as-is (the
// latency probe's watcher can tail the same file).
func (qs *qualitySession) writeStatus(pt qualityPoint, kind string, latency time.Duration, load1 float64) {
	if err := appendStatus(qs.cfg.statusPath, statusLine{
		Time: time.Now(), ElapsedS: qs.elapsedS(), Phase: "quality", Pair: pt.pair, Runes: pt.runes,
		LatencyMs: latency.Milliseconds(), Kind: kind, Load1: load1, Next: "-", BudgetLeftS: qs.budgetLeftS(),
	}); err != nil {
		qs.p.t.Logf("PROBE quality_status_error path=%q error=%q", qs.cfg.statusPath, err.Error())
	}
}

// admit lets the framework settle after the previous point (settle: a pause, or a queue drain after
// an error) and reports whether the budget still has room for pt. A quality point costs roughly
// double a plain probe point (forward call plus round trip), so it projects 2x projectPoint's
// latency-probe estimate; the DeepL call, when on, runs over the network and is not Apple-bound, so
// it is not part of this projection.
func (qs *qualitySession) admit(pt qualityPoint) bool {
	if qs.elapsed() >= qs.cfg.budget {
		return false
	}
	if qs.lastKind != "" {
		began := time.Now()
		drain := settle(qs.lastKind, realClock{}, qs.warm(qs.scripts[pt.pair]))
		qs.p.t.Logf("PROBE quality_settle after=%s queue_drain=%s waited_ms=%d", qs.lastKind, drain, time.Since(began).Milliseconds())
	}
	projected := 2 * projectPoint(pt.runes, qs.msPerRune[pt.pair])
	return shouldStart(qs.elapsed(), qs.cfg.budget, projected)
}

// measure runs one quality-probe point: forward translate, structural check, round trip, and
// (cfg.deepl) the DeepL reference call. Any Apple-call failure stops that point (logged, status
// written) rather than the whole session: a run that fails midway still leaves every point before
// it usable.
func (qs *qualitySession) measure(s script, pt qualityPoint) {
	t := qs.p.t
	text, paragraphs := s.buildInput(pt.runes)
	load1 := loadAvg(t)[0]

	fwd, fwdLatency, err := qs.p.call(s, text, pointCeiling)
	if err != nil {
		qs.lastKind = kindError
		t.Logf("PROBE quality_error pair=%s runes=%d stage=forward latency_ms=%d error=%q", pt.pair, pt.runes, fwdLatency.Milliseconds(), err.Error())
		qs.writeStatus(pt, "quality_forward_error", fwdLatency, load1)
		return
	}
	qs.lastKind = kindPass
	qs.msPerRune[pt.pair] = append(qs.msPerRune[pt.pair], float64(fwdLatency.Milliseconds())/float64(pt.runes))

	d := checkStructure(text, fwd.Result, paragraphs)
	t.Logf("PROBE quality structural pair=%s runes=%d paragraphs=%d dropped=%v reordered=%t repeated_sentences=%d repeated_paragraphs=%d latency_ms=%d",
		pt.pair, pt.runes, paragraphs, d.Dropped, d.Reordered, len(d.RepeatedSentences), len(d.RepeatedParagraphs), fwdLatency.Milliseconds())
	if d.any() {
		// Enough of the actual output for a human to read directly and judge fluency (#152's brief),
		// not just the boolean verdict above.
		t.Logf("PROBE quality structural_detail pair=%s runes=%d repeated_sentences=%q repeated_paragraphs=%q output_sample=%q",
			pt.pair, pt.runes, snippetList(d.RepeatedSentences, 200), snippetList(d.RepeatedParagraphs, 200), snippet(fwd.Result, 1200))
	}

	back, backLatency, err := qs.p.call(s.reversed(), fwd.Result, pointCeiling)
	if err != nil {
		qs.lastKind = kindError
		t.Logf("PROBE quality_error pair=%s runes=%d stage=roundtrip latency_ms=%d error=%q", pt.pair, pt.runes, backLatency.Milliseconds(), err.Error())
		qs.writeStatus(pt, "quality_roundtrip_error", fwdLatency+backLatency, load1)
		return
	}
	qs.lastKind = kindPass
	roundTrip := similarity(text, back.Result)
	t.Logf("PROBE quality roundtrip pair=%s runes=%d score=%.4f fwd_latency_ms=%d back_latency_ms=%d",
		pt.pair, pt.runes, roundTrip, fwdLatency.Milliseconds(), backLatency.Milliseconds())

	if qs.deepl != nil {
		dctx, cancel := context.WithTimeout(context.Background(), pointCeiling)
		deeplText, derr := qs.deepl(dctx, text, s.from, s.to)
		cancel()
		if derr != nil {
			t.Logf("PROBE quality_error pair=%s runes=%d stage=deepl error=%q", pt.pair, pt.runes, derr.Error())
		} else {
			divergence := similarity(fwd.Result, deeplText)
			t.Logf("PROBE quality deepl_divergence pair=%s runes=%d score=%.4f", pt.pair, pt.runes, divergence)
		}
	}

	qs.writeStatus(pt, "quality_pass", fwdLatency+backLatency, load1)
}

// run works through plan in order, admitting each point against the budget and stopping (not
// failing) the session once the cap is reached.
func (qs *qualitySession) run(plan []qualityPoint) {
	for i, pt := range plan {
		s, ok := qs.scripts[pt.pair]
		if !ok {
			qs.p.t.Fatalf("probe bug: the quality plan has pair %s, which has no ready script", pt.pair)
		}
		if !qs.admit(pt) {
			qs.exhausted(plan[i:])
			return
		}
		qs.measure(s, pt)
	}
}

// exhausted logs what the budget cap left unmeasured and marks the session partial.
func (qs *qualitySession) exhausted(remaining []qualityPoint) {
	qs.partial = true
	parts := make([]string, len(remaining))
	for i, pt := range remaining {
		parts[i] = fmt.Sprintf("%s %d", pt.pair, pt.runes)
	}
	qs.p.t.Logf("PROBE quality_budget_exhausted elapsed_s=%d remaining=%q", qs.elapsedS(), strings.Join(parts, ", "))
}

// finish logs the session's end state. It runs deferred, so a run the probe stops early (budget or
// a Fatalf) still reports what it measured as partial.
func (qs *qualitySession) finish() {
	t := qs.p.t
	if t.Failed() {
		qs.partial = true
	}
	l := loadAvg(t)
	t.Logf("PROBE quality_done elapsed_s=%d partial=%t load1=%.2f", qs.elapsedS(), qs.partial, l[0])
}

// TestProbeQuality is #152/#153's probe. It shares its gate (KAI_ENGINE_PROBE=1), prerequisites and
// scripts with TestProbeApple, but plans and runs independently (buildQualityPlan, qualitySession):
// it never calls buildPlan or session, and TestProbeApple never calls buildQualityPlan or
// qualitySession, so a session of one never contends with a session of the other beyond the
// prerequisites they share.
func TestProbeQuality(t *testing.T) {
	if os.Getenv(gateEnv) != "1" {
		t.Skip("set " + gateEnv + "=1 to run the Apple quality probe (the full command is in docs/quality-limits.md)")
	}
	started := time.Now()
	cfg, err := probeConfigFromEnv(os.Getenv)
	if err != nil {
		t.Fatalf("probe settings: %v", err)
	}
	for _, s := range scripts {
		for _, sentence := range append([]string{s.warm}, s.sentences...) {
			if strings.ContainsAny(sentence, reservedDigits) {
				t.Fatalf("probe bug: %s sentence %q contains a digit or a numeral; digits are reserved for the paragraph numbers", s.name, sentence)
			}
		}
	}
	logHost(t, cfg)
	waitQuiet(t, cfg)
	p := &prober{t: t, tr: engine.NewApple()}
	_, ready := p.ready([]script{latin, cjk, latinES}, japanese)
	includeJA := slices.ContainsFunc(ready, func(s script) bool { return s.pair() == pairJAEN })
	plan := buildQualityPlan(cfg, includeJA)
	t.Logf("PROBE quality_plan points=%d include_ja=%t deepl=%t", len(plan), includeJA, cfg.deepl)
	if len(plan) == 0 {
		t.Fatalf("probe plan: no quality-probe points (check %s)", maxRunesEnv)
	}

	var deepl translateFunc
	if cfg.deepl {
		deepl = deeplRef(t)
	}

	qs := &qualitySession{p: p, cfg: cfg, started: started, scripts: map[string]script{}, deepl: deepl, msPerRune: map[string][]float64{}}
	for _, s := range ready {
		qs.scripts[s.pair()] = s
	}
	defer qs.finish()
	qs.run(plan)
}
