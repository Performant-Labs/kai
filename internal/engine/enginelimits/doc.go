// Package enginelimits holds the opt-in probe that measures where an engine breaks on large
// input (issue #83, revised by #111 and #119) and the opt-in checks of the Apple cancel path (issue
// #111). Both are tests, compiled only with the build tag enginelimits on darwin, and each skips
// itself unless KAI_ENGINE_PROBE=1. Besides this file, the package's one non-test file is plan.go:
// the probe's pure planning, statistics and bookkeeping helpers. They make no Apple call and import
// neither the engine nor the bridge, so plan_test.go tests them in the authoritative suite, next to
// guard_test.go. Nothing imports this package. Without the tag it builds on every GOOS, and the
// suite never starts a translation.
//
// The probe and the cancel checks are deliberately not wired into CI, the Makefile, the Taskfile or
// the pipeline's test command (guard_test.go checks that). They need a Mac with the Swift bridge
// built and the English and Chinese (Simplified) translation languages installed (the probe also
// needs Spanish; Japanese is optional), and they take many minutes. A run without those
// prerequisites fails in ways that look like engine limits (0 languages, "Unable to Translate"), or
// never ends: the bridge has no wait of its own since #111, so a framework that does not answer
// keeps the call open. The probe therefore checks the prerequisites first, puts its own ceiling on
// every call, and stops with the cause instead of reporting a limit.
//
// A probe session runs a fixed plan, cheapest first, so its time cap cuts the least valuable data.
// Phase 1 is a ladder per pair, every size repeated: en>zh-Hans at 400, 1,600, 6,400 and 20,000
// runes, zh-Hans>en and en>es at 400, 1,600 and 6,400. Phase 2 is single zh-Hans>en runs that
// bracket its failure: 12,800 runes, then the #111 sizes up to 20,000 until one fails, then
// bisection to within 5%. Phase 3 is the ja>en ladder, only when Japanese is installed and the
// session so far took under 60 minutes. Record two sessions at least an hour apart; the second may
// leave out phase 2.
//
// Build the bridge, then run from the repository root on a quiet Mac (a session waits up to five
// minutes for a 1-minute load average of 3 or below, and stops if the Mac stays busy):
//
//	(cd pkg/swiftbridge/scripts && bash ./build.sh)
//
//	KAI_ENGINE_PROBE=1 CGO_ENABLED=1 caffeinate -is go test -tags enginelimits -ldflags=-linkmode=external -run TestProbeApple -timeout 115m -count=1 -v ./internal/engine/enginelimits/
//
// The timeout covers the 90-minute budget, plus the 20-minute ceiling of a point that starts just
// inside it, plus slack. The knobs, environment variables read only after the gate:
//
//	KAI_ENGINE_PROBE_REPEATS        runs per ladder size (default 3; fewer is refused)
//	KAI_ENGINE_PROBE_BUDGET_MIN     the session's wall-clock cap in minutes (default 90); with a
//	                                larger cap, raise the go test timeout to the cap plus 25 minutes
//	KAI_ENGINE_PROBE_MAX_RUNES      drops every planned size above it; 1000 is the smoke run (the
//	                                nine 400-rune points, about 3 minutes), which records the load
//	                                but does not wait for a quiet Mac
//	KAI_ENGINE_PROBE_SKIP_BOUNDARY  1 leaves out phase 2 (the second session)
//	KAI_ENGINE_PROBE_STATUS         the progress file (default kai-probe-status.jsonl in the temp
//	                                dir; the PROBE config line prints the path)
//
// After every run the probe appends one JSON line to the progress file (time, elapsed_s, phase,
// pair, runes, utf8_bytes, repeat, latency_ms, out_bytes, kind, load1, next, budget_left_s), one
// more when the budget stops it, and a last one with phase done. Tail the file to follow a session.
//
// The cancel checks in cancel_test.go take about two minutes:
//
//	KAI_ENGINE_PROBE=1 CGO_ENABLED=1 go test -tags enginelimits -ldflags=-linkmode=external -run 'TestAppleCancel|TestAppleNotCutOff' -timeout 30m -count=1 -v ./internal/engine/enginelimits/
//
// Both the external linker and the main-thread TestMain in probe_test.go are required: with either
// one missing the framework reports no installed languages, or a translation never returns.
//
// The probe prints its findings as "PROBE key=value" log lines and the cancel checks as "CANCEL"
// lines. The cancel checks' raw lines are recorded in docs/engine-limits.md. For a measured apple
// row, the probe's numbers and raw lines are recorded there too, and the largest size the Apple
// engine accepted (the smaller of the Latin and CJK maximums) is the Limit of the apple row in
// internal/engine/input_budget.go. The apple row is not measured yet: until issue #119 records a
// full probe run, its Limit is the largest Latin size seen to pass in the #111 throwaway checks and
// its CJK figure is only a lower bound, so the smaller-of-two-maximums rule does not hold for it.
//
// quality_probe_test.go (TestProbeQuality, epic #151, issues #152/#153) asks a different question
// on the same size ladder: not whether a call succeeds, but whether the translation is any good.
// It reuses this file's session/script/prober infrastructure and plan.go's buildQualityPlan and
// similarity/checkStructure helpers, under the same tag and gate. See docs/quality-limits.md for
// its run guide and knobs, including KAI_ENGINE_PROBE_DEEPL (#153's opt-in DeepL reference calls;
// unset or 0 costs nothing).
package enginelimits
