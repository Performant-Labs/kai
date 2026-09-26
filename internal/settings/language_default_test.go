package settings

import "testing"

// Issue #76: a fresh install's interface language is English, not "auto" (which followed a
// Chinese system locale) and not Chinese.
func TestDefaultSettingsInterfaceLanguageIsEnglish(t *testing.T) {
	if got := DefaultSettings().Language; got != "en-US" {
		t.Fatalf("DefaultSettings().Language = %q, want en-US", got)
	}
}
