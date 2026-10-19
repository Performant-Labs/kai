//go:build !darwin

package service

// ShowAboutPanelInFront is a no-op off macOS (there is no standard About panel).
func ShowAboutPanelInFront() bool { return false }
