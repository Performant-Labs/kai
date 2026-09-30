//go:build darwin

package service

import (
	"log/slog"
	"testing"

	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// The Grant button must reach the Swift request that registers Kai under Input Monitoring (issue #23).
// This fails if the bridge call is removed from OpenInputMonitoringSettings.
func TestOpenInputMonitoringSettingsCallsTheSwiftRequest(t *testing.T) {
	prev := swiftbridge.KaiInputMonitoringRequest
	t.Cleanup(func() { swiftbridge.KaiInputMonitoringRequest = prev })

	calls := 0
	swiftbridge.KaiInputMonitoringRequest = func() int32 { calls++; return 0 }
	(&AppService{log: slog.Default()}).OpenInputMonitoringSettings()
	if calls != 1 {
		t.Fatalf("the Swift request was called %d times, want exactly 1", calls)
	}

	swiftbridge.KaiInputMonitoringRequest = nil
	(&AppService{log: slog.Default()}).OpenInputMonitoringSettings() // a bridge that is not loaded: no panic
}
