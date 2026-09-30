//go:build !darwin

package service

import "unsafe"

// SetWindowWebViewBackground is a no-op off macOS (there is no WKWebView to fix).
func SetWindowWebViewBackground(unsafe.Pointer, int32, int32, int32) bool { return false }
