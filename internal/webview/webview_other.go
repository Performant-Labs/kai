//go:build !windows

// Package webview provides Windows WebView2 browser-argument fallbacks.
// Non-Windows platforms (macOS / Linux) need no injection; empty implementations provided.
package webview

// BrowserArgs returns nil on non-Windows platforms (no WebView2, no browser args).
func BrowserArgs() []string { return nil }

// ApplyOptions is a no-op on non-Windows platforms (no AdditionalBrowserArgs field).
func ApplyOptions(opts any) {}
