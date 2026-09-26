package service

import (
	"os"
	"strings"
	"testing"
)

// Hand test of #82 (2026-09-26): closing the translate window brought up Settings, which the user
// had not opened. main.go created the Settings window visible at every launch ("settings page as
// the main screen, shown at startup"); it sat behind other apps, and when the translate window
// hid, macOS activated Kai's remaining visible window, Settings. Kai is a menu-bar app
// (ActivationPolicyAccessory): Settings must start hidden and open only from the gear or the tray.
// main.go is package main and outside the authoritative suite, so this pins its source.

func readMainGo(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	return string(b)
}

func TestSettingsWindowStartsHidden(t *testing.T) {
	src := readMainGo(t)
	start := strings.Index(src, "settingsWindow.Center()")
	end := strings.Index(src, "translateWindow = app.Window.NewWithOptions")
	if start < 0 || end < 0 || end < start {
		t.Fatalf("startup block not found (Center at %d, translate window at %d)", start, end)
	}
	if !strings.Contains(src[start:end], "settingsWindow.Hide()") {
		t.Fatal("main.go must hide the Settings window right after creating it (startup)")
	}
}

func TestTraySettingsOpensThroughShowSettings(t *testing.T) {
	// The tray menu is built outside main()'s scope, so it reaches windowSvc.ShowSettings through
	// the same kai:window:show event the frontend gear uses.
	// A window hidden since creation needs showAndFocus (double Show) to appear on the first try,
	// and ShowSettings also lowers a pinned translate window (#69).
	src := readMainGo(t)
	i := strings.Index(src, `trayMenu.Add(i18n.T("menu.settings"))`)
	if i < 0 {
		t.Fatal("tray Settings item not found")
	}
	j := strings.Index(src[i:], "})")
	if j < 0 || !strings.Contains(src[i:i+j], `app.Event.Emit(kevents.EventWindowShow, "settings")`) {
		t.Fatal("the tray Settings item must open Settings through the window-show event (windowSvc.ShowSettings)")
	}
}
