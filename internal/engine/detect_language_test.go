package engine

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #200: the local language detector's payload. parseDetection reads the bridge's JSON
// ({"lang":"es","confidence":0.99}) and never trusts it: anything that is not a usable detection
// is reported as none, so a broken or empty answer can never switch anything.
func TestParseDetection(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		lang model.Language
		conf float64
		ok   bool
	}{
		{"spanish", `{"lang":"es","confidence":0.996}`, "es", 0.996, true},
		{"script tag kept as reported", `{"lang":"zh-Hans","confidence":1}`, "zh-Hans", 1, true},
		{"trailing NULs from the C buffer", "{\"lang\":\"en\",\"confidence\":0.9}\x00\x00", "en", 0.9, true},
		{"undetermined", `{"lang":"und","confidence":0.4}`, "", 0, false},
		{"empty language", `{"lang":"","confidence":0.9}`, "", 0, false},
		{"no language field", `{"confidence":0.9}`, "", 0, false},
		{"malformed", `{"lang":`, "", 0, false},
		{"bridge error payload", `{"code":"target_required","detail":"x"}`, "", 0, false},
		{"empty", ``, "", 0, false},
		{"confidence out of range", `{"lang":"es","confidence":7}`, "", 0, false},
	} {
		lang, conf, ok := parseDetection([]byte(tc.raw))
		if ok != tc.ok || lang != tc.lang || conf != tc.conf {
			t.Errorf("%s: got (%q, %v, %v), want (%q, %v, %v)", tc.name, lang, conf, ok, tc.lang, tc.conf, tc.ok)
		}
	}
}

func TestDetectLanguageEmptyTextIsNoDetection(t *testing.T) {
	if _, _, ok := DetectLanguage("   \n"); ok {
		t.Fatal("blank text produced a detection")
	}
}
