//go:build !darwin

package service

import "unsafe"

// SetWindowAppearance is a no-op off macOS (there is no NSAppearance to set).
func SetWindowAppearance(unsafe.Pointer, bool) bool { return false }
