// Package enginelimits holds the opt-in probe that measures where an engine breaks on large
// input (issue #83). It contains no production code: the probe is a test, compiled only with the
// build tag enginelimits on darwin, and it skips itself unless KAI_ENGINE_PROBE=1. Without the tag
// this package is just this file, so it always builds on every GOOS and the authoritative suite
// never runs the probe.
//
// It is deliberately not wired into CI, the Makefile, the Taskfile or the pipeline's test command.
// It needs a Mac with the Swift bridge built and the English and Chinese (Simplified) translation
// languages installed, it takes many minutes, and a run without those prerequisites fails in ways
// that look like engine limits (0 languages, "Unable to Translate", "returned empty" after 20 s).
// The probe checks the prerequisites first and stops with the cause instead of reporting a limit.
//
// Build the bridge, then run from the repository root:
//
//	(cd pkg/swiftbridge/scripts && bash ./build.sh)
//
//	KAI_ENGINE_PROBE=1 CGO_ENABLED=1 go test -tags enginelimits -ldflags=-linkmode=external -run TestProbeApple -timeout 60m -count=1 -v ./internal/engine/enginelimits/
//
// Both the external linker and the main-thread TestMain in probe_test.go are required: with
// either one missing the framework reports no installed languages or the bridge's 20 s wait
// expires on every call.
//
// The probe prints its findings as "PROBE key=value" log lines. The numbers and the raw lines are
// recorded in docs/engine-limits.md, and the largest size the Apple engine accepted (the smaller
// of the two searches) is the Limit of the apple row in internal/engine/input_budget.go.
package enginelimits
