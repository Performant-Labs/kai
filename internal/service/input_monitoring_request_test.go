package service

import (
	"log/slog"
	"testing"
)

// Issue #23: the Input Monitoring row gets a Grant button, which calls this binding. Without the
// Swift bridge loaded (CI, tests, other platforms) it must be a harmless no-op, never a panic on a
// nil function pointer.
func TestOpenInputMonitoringSettingsIsSafeWithoutTheBridge(t *testing.T) {
	s := &AppService{log: slog.Default()}
	s.OpenInputMonitoringSettings()
}
