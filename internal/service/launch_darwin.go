//go:build darwin

package service

import "cnb.cool/dtapp/kai/pkg/swiftbridge"

// ObserveLaunch starts recording how the app is launched. Call it after swiftbridge.Init and
// before app.Run (the launch event is only readable while AppKit delivers it).
func ObserveLaunch() {
	if swiftbridge.Available() && swiftbridge.KaiLaunchObserve != nil {
		swiftbridge.KaiLaunchObserve()
	}
}

// DetectLaunchKind reports how the app was launched; LaunchUnknown when it cannot tell.
func DetectLaunchKind() LaunchKind {
	if !swiftbridge.Available() || swiftbridge.KaiLaunchKind == nil {
		return LaunchUnknown
	}
	return launchKindFromCode(swiftbridge.KaiLaunchKind())
}
