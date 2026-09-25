//go:build windows && !server

// Package webview provides Windows WebView2 browser-argument fallbacks.
// Only the GPU fallback args that may help the sporadic 80010108 crash are injected;
// Runtime-missing detection was removed (that error occurs sporadically even on machines
// with the Runtime installed — a detection layer would only hurt normal users).
// Non-Windows platforms get empty implementations from webview_other.go.
package webview

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// BrowserArgs returns the global WebView2 browser launch args injected on Windows.
//
// Background: the sporadic 80010108 (RPC_E_DISCONNECTED) crash ("The object invoked has
// disconnected from its clients") occurs even on machines with WebView2 installed; a
// high-frequency trigger is GPU process trouble (graphics driver / hardware-accelerated GPU
// scheduling / abnormal DComp state after waking from sleep), which crashes the WebView2
// renderer before the controller-creation callback, handing the callback a disconnected
// object. Injecting --disable-gpu and --disable-gpu-compositing downgrades rendering to the
// software path, cutting off this sporadic crash path. The cost is losing hardware
// acceleration (negligible for a lightweight translation UI).
// WebView2 shares a single browser environment, so these args apply globally to all windows
// (including the check-for-updates window).
// Non-Windows platforms (_other.go) return nil.
func BrowserArgs() []string {
	return []string{
		"--disable-gpu",
		"--disable-gpu-compositing",
	}
}

// ApplyOptions writes the WebView2 browser fallback args into application.Options on
// Windows.
// Note: AdditionalBrowserArgs lives inside the Options.Windows (WindowsOptions) sub-struct,
// not the top-level Options (the macOS struct has no Windows field), so it is only
// accessible from //go:build windows files. Non-Windows gets a no-op from _other.go, so
// main.go can call it unconditionally.
func ApplyOptions(opts *application.Options) {
	opts.Windows.AdditionalBrowserArgs = BrowserArgs()
}
