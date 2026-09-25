//go:build darwin

package service

import (
	"log/slog"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// isAccessibilityEnabled checks whether macOS accessibility is granted to the current binary.
func (s *AppService) isAccessibilityEnabled() bool {
	// Degrade safely when the dylib isn't loaded: treat as not authorized (no panic).
	if !swiftbridge.Available() {
		s.log.Warn(i18n.T("log.swiftbridge_unavailable"))
		return false
	}
	enabled := swiftbridge.KaiAccessibilityEnabled() != 0
	s.log.Info(i18n.T("log.accessibility_query"), slog.Bool(i18n.T("log.field_result"), enabled))
	return enabled
}

// openAccessibilitySettings requests accessibility permission via the system dialog (darwin
// only).
func (s *AppService) openAccessibilitySettings() {
	s.log.Info(i18n.T("log.accessibility_request"))
	if !swiftbridge.Available() {
		return
	}
	swiftbridge.KaiAccessibilityRequest()
}

// isScreenRecordingEnabled checks whether macOS screen recording is granted to the current
// binary (required by screenshot OCR).
func (s *AppService) isScreenRecordingEnabled() bool {
	if !swiftbridge.Available() {
		s.log.Warn(i18n.T("log.swiftbridge_unavailable"))
		return false
	}
	enabled := swiftbridge.KaiScreenRecordingEnabled() != 0
	s.log.Info(i18n.T("log.screenrecording_query"), slog.Bool(i18n.T("log.field_result"), enabled))
	return enabled
}

// openScreenRecordingSettings pops the system "Screen Recording" permission dialog (darwin
// only).
func (s *AppService) openScreenRecordingSettings() {
	s.log.Info(i18n.T("log.screenrecording_request"))
	if !swiftbridge.Available() {
		return
	}
	swiftbridge.KaiScreenRecordingRequest()
}

// TODO: input-monitoring related (isInputMonitoringEnabled / openInputMonitoringSettings)
// currently unused, commented out. Restore when robotgo is needed to simulate the copy key.
// // isInputMonitoringEnabled checks whether macOS "Input Monitoring" is granted to the
// current binary.
// func (s *AppService) isInputMonitoringEnabled() bool {
// 	enabled := C.kai_input_monitoring_enabled() != 0
// 	s.log.Info(i18n.T("log.input_monitoring_query"), slog.Bool("result", enabled))
// 	return enabled
// }
//
// // openInputMonitoringSettings opens the system "Security & Privacy > Input Monitoring"
// settings pane (darwin only).
// func (s *AppService) openInputMonitoringSettings() {
// 	s.log.Info("[Kai-Bridge-Cgo] input monitoring permission: opening system settings pane")
// 	C.kai_input_monitoring_request()
// }
