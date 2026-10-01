//go:build !darwin

package service

// isAccessibilityEnabled: non-darwin platforms need no permission, always true.
func (s *AppService) isAccessibilityEnabled() bool {
	return true
}

// openAccessibilitySettings is a no-op on non-darwin platforms.
func (s *AppService) openAccessibilitySettings() {}

// isScreenRecordingEnabled: non-darwin platforms need no permission, always true.
func (s *AppService) isScreenRecordingEnabled() bool {
	return true
}

// openScreenRecordingSettings is a no-op on non-darwin platforms.
func (s *AppService) openScreenRecordingSettings() {}

// isInputMonitoringEnabled: non-darwin platforms need no permission, always true.
func (s *AppService) isInputMonitoringEnabled() bool {
	return true
}

// openInputMonitoringSettings is a no-op on non-darwin platforms.
func (s *AppService) openInputMonitoringSettings() {}
