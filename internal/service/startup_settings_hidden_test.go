package service

import (
	"os"
	"strings"
	"testing"
)

// Hand test of #82 (2026-09-26): closing the translate window brought up Settings, which the user
// had not opened. main.go created the Settings window visible at every launch ("settings page as
// the main screen, shown at startup"); it sat behind other apps, and when the translate window
// hid, macOS activated Kai's remaining visible window, Settings. Kai is a menu-bar app (tray,
// plus a Dock icon since issue #165's ActivationPolicyRegular): Settings must start hidden and
// open only from the gear or the tray.
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

// Issue #165, item 7 (principal decision, 2026-09-28): Kai gets a normal Dock icon and shows up
// in Cmd+Tab. ActivationPolicyAccessory and ActivationPolicyRegular are the same macOS switch
// that controls both "no Dock icon" and "absent from Cmd+Tab" together — there is no partial
// option — so this pins the deliberate choice and guards against a silent revert to Accessory.
func TestMacActivationPolicyIsRegularForCmdTabVisibility(t *testing.T) {
	src := readMainGo(t)
	if !strings.Contains(src, "ActivationPolicy: application.ActivationPolicyRegular") {
		t.Fatal("main.go must set Mac.ActivationPolicy to ActivationPolicyRegular (issue #165: Cmd+Tab visibility)")
	}
	if strings.Contains(src, "application.ActivationPolicyAccessory") {
		t.Fatal("main.go must not reference ActivationPolicyAccessory any more (issue #165 switched to Regular)")
	}
}

// LSUIElement is LaunchServices' own bundle-level switch for "no Dock icon, no Cmd+Tab entry",
// read before main.go's runtime ActivationPolicy has a chance to run — it must agree with
// main.go's ActivationPolicyRegular, or it would keep hiding Kai from the Dock/Cmd+Tab regardless
// of the runtime setting.
func TestMacInfoPlistsDoNotSetLSUIElement(t *testing.T) {
	for _, rel := range []string{"../../build/darwin/Info.plist", "../../build/darwin/Info.dev.plist"} {
		b, err := os.ReadFile(rel)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		// The <key> tag, not a bare substring match: both plists carry an explanatory XML comment
		// that names LSUIElement on purpose, documenting why it was removed.
		if strings.Contains(string(b), "<key>LSUIElement</key>") {
			t.Errorf("%s must not set the LSUIElement key any more (issue #165: Cmd+Tab visibility, kept in sync with main.go's ActivationPolicyRegular)", rel)
		}
	}
}
