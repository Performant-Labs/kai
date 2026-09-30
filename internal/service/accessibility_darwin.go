//go:build darwin

package service

import (
	"log/slog"
	"unsafe"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// permissionFromQuery reads a Swift permission query for the Settings > Shortcuts rows. An answer
// that cannot be read is NOT shown as granted (the same as an unloaded bridge, which already reads
// as not granted here): the row offers "Grant access", which is harmless, instead of claiming a
// permission nobody confirmed.
func permissionFromQuery(query func(unsafe.Pointer, int32) int32) bool {
	enabled, ok := swiftbridge.QueryEnabled(query)
	return ok && enabled
}

// isAccessibilityEnabled checks whether macOS accessibility is granted to the current binary.
func (s *AppService) isAccessibilityEnabled() bool {
	// Degrade safely when the dylib isn't loaded: treat as not authorized (no panic).
	if !swiftbridge.Available() {
		s.log.Warn(i18n.T("log.swiftbridge_unavailable"))
		return false
	}
	enabled := permissionFromQuery(swiftbridge.KaiAccessibilityEnabled)
	s.log.Debug(i18n.T("log.accessibility_query"), slog.Bool(i18n.T("log.field_result"), enabled))
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
	enabled := permissionFromQuery(swiftbridge.KaiScreenRecordingEnabled)
	s.log.Debug(i18n.T("log.screenrecording_query"), slog.Bool(i18n.T("log.field_result"), enabled))
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

// inputMonitoringFromQuery reads the Input Monitoring answer for the Settings page. A bridge that
// is not loaded, a nil query and an answer that cannot be read (the old no-buffer -1) all give
// "not granted", never "granted" (issue #14). Split from the service method so that is testable
// without the real dylib.
func inputMonitoringFromQuery(available bool, query func(unsafe.Pointer, int32) int32) bool {
	return available && permissionFromQuery(query)
}

// isInputMonitoringEnabled checks whether macOS Input Monitoring is granted to the current binary
// (the translate-on-double-Cmd+C listener needs it). Unlike the request functions above, the query
// only reads the state and never shows a prompt.
func (s *AppService) isInputMonitoringEnabled() bool {
	if !swiftbridge.Available() {
		s.log.Warn(i18n.T("log.swiftbridge_unavailable"))
		return false
	}
	enabled := inputMonitoringFromQuery(true, swiftbridge.KaiInputMonitoringEnabled)
	s.log.Debug(i18n.T("log.input_monitoring_query"), slog.Bool(i18n.T("log.field_result"), enabled))
	return enabled
}

// openInputMonitoringSettings asks macOS to list Kai under Input Monitoring and opens that pane
// (darwin only). Only the Grant button calls it, never the 3-second poll.
func (s *AppService) openInputMonitoringSettings() {
	s.log.Info(i18n.T("log.input_monitoring_request"))
	if !swiftbridge.Available() {
		return
	}
	swiftbridge.KaiInputMonitoringRequest()
}
