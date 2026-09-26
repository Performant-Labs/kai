// Package enginelimits holds the opt-in probe that measures where an engine breaks on large
// input (issue #83, revised by #111) and the opt-in checks of the Apple cancel path (issue #111). It
// contains no production code: both are tests, compiled only with the build tag enginelimits on
// darwin, and each skips itself unless KAI_ENGINE_PROBE=1. Without the tag this package is just this
// file, so it always builds on every GOOS and the authoritative suite never runs them.
//
// They are deliberately not wired into CI, the Makefile, the Taskfile or the pipeline's test
// command. They need a Mac with the Swift bridge built and the English and Chinese (Simplified)
// translation languages installed, and they take many minutes. A run without those prerequisites
// fails in ways that look like engine limits (0 languages, "Unable to Translate"), or never ends: the
// bridge has no wait of its own since #111, so a framework that does not answer keeps the call open.
// The probe therefore checks the prerequisites first, puts its own ceiling on every call, and stops
// with the cause instead of reporting a limit.
//
// Build the bridge, then run from the repository root:
//
//	(cd pkg/swiftbridge/scripts && bash ./build.sh)
//
//	KAI_ENGINE_PROBE=1 CGO_ENABLED=1 go test -tags enginelimits -ldflags=-linkmode=external -run TestProbeApple -timeout 3h -count=1 -v ./internal/engine/enginelimits/
//
// The probe searches up to 20,000 runes per script, which takes about an hour on the recorded host;
// KAI_ENGINE_PROBE_MAX_RUNES lowers that bound for a shorter run. The cancel checks in cancel_test.go
// take about two minutes:
//
//	KAI_ENGINE_PROBE=1 CGO_ENABLED=1 go test -tags enginelimits -ldflags=-linkmode=external -run 'TestAppleCancel|TestAppleNotCutOff' -timeout 30m -count=1 -v ./internal/engine/enginelimits/
//
// Both the external linker and the main-thread TestMain in probe_test.go are required: with either
// one missing the framework reports no installed languages, or a translation never returns.
//
// The probe prints its findings as "PROBE key=value" log lines and the cancel checks as "CANCEL"
// lines. The cancel checks' raw lines are recorded in docs/engine-limits.md. For a measured apple
// row, the probe's numbers and raw lines are recorded there too, and the largest size the Apple
// engine accepted (the smaller of the two searches) is the Limit of the apple row in
// internal/engine/input_budget.go. The apple row is not measured yet: until issue #119 records a
// full probe run, its Limit is the largest Latin size seen to pass in the #111 throwaway checks and
// its CJK figure is only a lower bound, so the smaller-of-two-searches rule does not hold for it.
package enginelimits
