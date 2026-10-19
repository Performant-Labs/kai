//go:build darwin

package service

import (
	"testing"

	"cnb.cool/dtapp/kai/pkg/swiftbridge"
)

// ShowAboutPanelInFront is the only Go path into kai_show_about_panel (issue #8): it must reach
// the bridge and read the 1 from Swift as success, and treat a bridge that is not loaded as a
// no-op. The panel's stacking is not verifiable in go test; only the wiring is.
func TestShowAboutPanelInFrontCallsTheBridge(t *testing.T) {
	prev := swiftbridge.KaiShowAboutPanel
	t.Cleanup(func() { swiftbridge.KaiShowAboutPanel = prev })

	called := false
	swiftbridge.KaiShowAboutPanel = func() int32 { called = true; return 1 }
	if !ShowAboutPanelInFront() || !called {
		t.Fatal("a 1 from Swift means the panel was shown and must read as success")
	}

	swiftbridge.KaiShowAboutPanel = func() int32 { return 0 }
	if ShowAboutPanelInFront() {
		t.Error("a 0 from Swift (no about panel found) must read as failure")
	}
}

func TestShowAboutPanelInFrontIsANoopWithoutTheBridge(t *testing.T) {
	prev := swiftbridge.KaiShowAboutPanel
	t.Cleanup(func() { swiftbridge.KaiShowAboutPanel = prev })

	swiftbridge.KaiShowAboutPanel = nil
	if ShowAboutPanelInFront() {
		t.Error("a bridge that is not loaded (nil function) must be a no-op")
	}
}
