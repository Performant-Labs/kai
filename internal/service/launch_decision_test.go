package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #17: Kai shows the translate window on every launch from the Dock or Finder, stays quiet
// when launched at login, and centres the window on the very first launch. The decision is a
// pure function so it can be tested without a running app.

func TestDecideLaunch(t *testing.T) {
	cases := []struct {
		name  string
		kind  LaunchKind
		first bool
		want  LaunchDecision
	}{
		{"user launch shows", LaunchUser, false, LaunchDecision{Show: true}},
		{"user launch, first ever, shows centred", LaunchUser, true, LaunchDecision{Show: true, Center: true}},
		{"login launch stays quiet", LaunchLogin, false, LaunchDecision{}},
		{"login launch stays quiet even on an empty data folder", LaunchLogin, true, LaunchDecision{}},
		{"unknown launch defaults to showing", LaunchUnknown, false, LaunchDecision{Show: true}},
		{"unknown launch, first ever, shows centred", LaunchUnknown, true, LaunchDecision{Show: true, Center: true}},
	}
	for _, c := range cases {
		if got := DecideLaunch(c.kind, c.first); got != c.want {
			t.Errorf("%s: DecideLaunch(%v, %v) = %+v, want %+v", c.name, c.kind, c.first, got, c.want)
		}
	}
}

func TestLaunchKindZeroValueIsUnknown(t *testing.T) {
	var k LaunchKind
	if k != LaunchUnknown {
		t.Fatal("the zero LaunchKind must be LaunchUnknown so an undetectable launch defaults to showing")
	}
}

func TestLaunchKindFromBridge(t *testing.T) {
	for code, want := range map[int32]LaunchKind{0: LaunchUnknown, 1: LaunchUser, 2: LaunchLogin, 99: LaunchUnknown, -1: LaunchUnknown} {
		if got := launchKindFromCode(code); got != want {
			t.Errorf("launchKindFromCode(%d) = %v, want %v", code, got, want)
		}
	}
}

func TestIsFirstRun(t *testing.T) {
	dir := t.TempDir()
	if !IsFirstRun(dir) {
		t.Error("an empty data folder is a first run")
	}
	if !IsFirstRun(filepath.Join(dir, "does-not-exist")) {
		t.Error("a missing data folder is a first run")
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if IsFirstRun(dir) {
		t.Error("a data folder with settings.json is not a first run")
	}
}

// main.go is package main, outside the authoritative suite, so its source is pinned (same as
// startup_settings_hidden_test.go). The startup show must go through ShowTranslateWindow
// (showAndFocus, never a bare Show().Focus()) and on the main thread (application.InvokeAsync),
// per the crashes of #163 and #167.
func TestStartupShowGoesThroughShowPathOnMainThread(t *testing.T) {
	src := readMainGo(t)
	i := strings.Index(src, "events.Common.ApplicationStarted")
	if i < 0 {
		t.Fatal("main.go must show the window from an ApplicationStarted handler")
	}
	rest := src[i:]
	end := strings.Index(rest, "\n\t})\n")
	if end < 0 {
		t.Fatal("ApplicationStarted handler end not found")
	}
	body := rest[:end]
	for _, want := range []string{"service.DecideLaunch(", "application.InvokeAsync(", "windowSvc.ShowTranslateWindow()"} {
		if !strings.Contains(body, want) {
			t.Errorf("startup handler must contain %q", want)
		}
	}
	if strings.Contains(body, "translateWindow.Show()") || strings.Contains(body, ".Show().Focus()") {
		t.Error("startup handler must not call a bare Show(): use windowSvc.ShowTranslateWindow")
	}
	if strings.Index(body, "application.InvokeAsync(") > strings.Index(body, "windowSvc.ShowTranslateWindow()") {
		t.Error("ShowTranslateWindow must be called inside the InvokeAsync closure")
	}
}

func TestStartupObservesLaunchBeforeRun(t *testing.T) {
	src := readMainGo(t)
	obs := strings.Index(src, "service.ObserveLaunch()")
	run := strings.LastIndex(src, "app.Run()")
	if obs < 0 || run < 0 || obs > run {
		t.Fatalf("main.go must call service.ObserveLaunch() before app.Run() (observe at %d, run at %d)", obs, run)
	}
	if !strings.Contains(src, "service.IsFirstRun(dataDir)") {
		t.Error("main.go must compute the first-run flag with service.IsFirstRun(dataDir)")
	}
}
