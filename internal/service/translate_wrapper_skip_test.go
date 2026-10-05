package service

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/buildinfo"
)

func captureSkipLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// The window's own reason for not switching must reach the MAIN log (issue #16): the frontend's
// separate log file was empty in practice, and the reasons (text_changed, pair_changed,
// source_not_offered, ...) are among the leading suspects for the Alt-A miss.
func TestReportSourceSwitchSkippedWritesOneInfoLineWithTheReason(t *testing.T) {
	buf := captureSkipLog(t)
	(&TranslateWrapper{}).ReportSourceSwitchSkipped("pair_changed")
	out := strings.TrimSpace(buf.String())
	if strings.Count(out, "\n") != 0 || !strings.Contains(out, "level=INFO") || !strings.Contains(out, "reason=pair_changed") {
		t.Fatalf("want one INFO line carrying reason=pair_changed, got:\n%s", out)
	}
}

// Issue #16: the window's skip line names the build too, like the backend's decision line.
func TestReportSourceSwitchSkippedNamesTheBuild(t *testing.T) {
	oldV, oldC := buildinfo.Version, buildinfo.GitCommit
	t.Cleanup(func() { buildinfo.Version, buildinfo.GitCommit = oldV, oldC })
	buildinfo.Version, buildinfo.GitCommit = "v7.7.7", "feedbee"
	buf := captureSkipLog(t)
	(&TranslateWrapper{}).ReportSourceSwitchSkipped("pair_changed")
	for _, want := range []string{"version=v7.7.7", "commit=feedbee"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("skip line lacks %q: %s", want, buf.String())
		}
	}
}

// Whatever a caller sends, the log must not carry the user's text: only a short snake_case word
// passes through.
func TestReportSourceSwitchSkippedNeverLogsText(t *testing.T) {
	for _, bad := range []string{"Mi contrasena es LibelulaAzul", "", strings.Repeat("a", 41), "has space", "UPPER", "digits123"} {
		buf := captureSkipLog(t)
		(&TranslateWrapper{}).ReportSourceSwitchSkipped(bad)
		if out := buf.String(); strings.Contains(out, bad) && bad != "" || !strings.Contains(out, "reason=invalid") {
			t.Errorf("reason %q must be logged as invalid, got:\n%s", bad, out)
		}
	}
}
