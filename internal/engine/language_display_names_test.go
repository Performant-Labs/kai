package engine

import (
	"testing"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// #43: display names are correct in en-US and zh-CN for every recognized language (the
// selectable variants and the bare bases kept for detection), and each variant is named
// differently from its base so the language bar never shows two identical entries.
func TestBackendLanguageNamesPerLocale(t *testing.T) {
	t.Cleanup(func() { i18n.SetLocale("zh-CN") })
	want := map[string]map[model.Language]string{
		"en-US": {model.ES: "Spanish", model.ESMX: "Spanish (Mexico)", model.PT: "Portuguese",
			model.PTBR: "Portuguese (Brazil)", model.PTPT: "Portuguese (Portugal)"},
		"zh-CN": {model.ES: "西班牙语", model.ESMX: "西班牙语（墨西哥）", model.PT: "葡萄牙语",
			model.PTBR: "葡萄牙语（巴西）", model.PTPT: "葡萄牙语（葡萄牙）"},
	}
	for loc, names := range want {
		i18n.SetLocale(loc)
		for _, l := range model.AllLanguages() {
			if got := languageLabel(l); got == "" || got == "lang."+string(l) {
				t.Errorf("%s: language %q has no display name (got %q)", loc, l, got)
			}
		}
		for l, name := range names {
			if got := languageLabel(l); got != name {
				t.Errorf("%s: languageLabel(%s) = %q, want %q", loc, l, got, name)
			}
		}
	}
}
