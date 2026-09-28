//go:build !darwin

package service

import "unsafe"

// SetWindowNotRestorable is a no-op on non-darwin platforms (issue #163: macOS Secure State
// Restoration has no Windows/Linux equivalent here).
func SetWindowNotRestorable(unsafe.Pointer) {}
