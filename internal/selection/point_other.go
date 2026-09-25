//go:build !darwin && !windows

package selection

import (
	"github.com/wailsapp/wails/v3/pkg/application"
)

// currentSelectionPoint: selection coordinates are not yet supported on non-darwin/windows
// platforms; returns nil.
func currentSelectionPoint() *application.Point {
	return nil
}

// primaryScreenSize returns 0,0 on non-darwin/windows platforms.
func primaryScreenSize() (float64, float64) {
	return 0, 0
}

// currentSelectionOSA returns an empty string on non-darwin/windows platforms (no system
// text capture).
func currentSelectionOSA() string {
	return ""
}
