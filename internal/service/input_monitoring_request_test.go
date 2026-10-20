package service

import (
	"log/slog"
	"testing"

	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// Issue #23: the Input Monitoring row gets a Grant button, which calls this binding. Without the
// Swift bridge loaded (CI, tests, other platforms) it must be a harmless no-op, never a panic on a
// nil function pointer.
//
// Issue #101: "without the bridge" has to be made true here, not assumed. Another test in this
// package loads the real dylib (swiftbridge.Init) and its request pointer outlives that test; with
// it set, this call ran the REAL request, which asks macOS for Input Monitoring and opens System
// Settings on the developer's screen on every `go test ./internal/...`.
func TestOpenInputMonitoringSettingsIsSafeWithoutTheBridge(t *testing.T) {
	prev := swiftbridge.KaiInputMonitoringRequest
	t.Cleanup(func() { swiftbridge.KaiInputMonitoringRequest = prev })
	swiftbridge.KaiInputMonitoringRequest = nil

	s := &AppService{log: slog.Default()}
	s.OpenInputMonitoringSettings()
}
