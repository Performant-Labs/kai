package i18n

import "testing"

// Issue #76: English is the default interface language. Every path that used to fall back to
// Chinese (the process default, an unknown locale passed to SetLocale/TWithLocale/ResolveLocale)
// now falls back to English; an explicit zh-CN still selects Chinese.

func restoreLocale(t *testing.T) {
	t.Helper()
	saved := GetLocale()
	t.Cleanup(func() { SetLocale(saved) })
}

func TestDefaultLocaleIsEnglish(t *testing.T) {
	if got := GetLocale(); got != string(EN_US) {
		t.Fatalf("default locale = %q, want %q", got, EN_US)
	}
}

func TestUnknownLocaleFallsBackToEnglish(t *testing.T) {
	restoreLocale(t)
	for _, in := range []string{"", "fr", "garbage"} {
		SetLocale(string(ZH_CN))
		SetLocale(in)
		if got := GetLocale(); got != string(EN_US) {
			t.Errorf("SetLocale(%q) -> %q, want %q", in, got, EN_US)
		}
		if got := ResolveLocale(in); got != string(EN_US) {
			t.Errorf("ResolveLocale(%q) = %q, want %q", in, got, EN_US)
		}
	}
}

func TestExplicitChineseIsHonored(t *testing.T) {
	restoreLocale(t)
	SetLocale(string(ZH_CN))
	if got := GetLocale(); got != string(ZH_CN) {
		t.Errorf("SetLocale(zh-CN) -> %q, want %q", got, ZH_CN)
	}
	if got := ResolveLocale(string(ZH_CN)); got != string(ZH_CN) {
		t.Errorf("ResolveLocale(zh-CN) = %q, want %q", got, ZH_CN)
	}
}

func TestTWithLocaleUnknownUsesEnglishText(t *testing.T) {
	restoreLocale(t)
	const key = "app.description"
	en := TWithLocale(string(EN_US), key)
	zh := TWithLocale(string(ZH_CN), key)
	if en == key || zh == key || en == zh {
		t.Fatalf("probe key %q must exist with distinct en/zh text (en=%q zh=%q)", key, en, zh)
	}
	if got := TWithLocale("fr", key); got != en {
		t.Errorf("TWithLocale(fr) = %q, want the English text %q", got, en)
	}
}

// Issue #76: "auto" follows the system language instead of being an unrecognized value that
// silently became Chinese. Only a Chinese system language selects Chinese.
func TestAutoFollowsSystemLanguage(t *testing.T) {
	restoreLocale(t)
	saved := systemLocaleFn
	t.Cleanup(func() { systemLocaleFn = saved })

	cases := []struct {
		system Locale
		want   string
	}{
		{EN_US, string(EN_US)},
		{ZH_CN, string(ZH_CN)},
	}
	for _, c := range cases {
		systemLocaleFn = func() Locale { return c.system }
		SetLocale(string(EN_US))
		SetLocale("auto")
		if got := GetLocale(); got != c.want {
			t.Errorf("auto with system %q -> %q, want %q", c.system, got, c.want)
		}
	}
}

func TestClassifySystemLanguage(t *testing.T) {
	cases := map[string]Locale{
		"zh-Hans-CN": ZH_CN, "zh_CN": ZH_CN, "zh-Hant-TW": ZH_CN, "ZH": ZH_CN,
		"en-US": EN_US, "en_GB.UTF-8": EN_US, "fr-FR": EN_US, "": EN_US, "C": EN_US,
	}
	for in, want := range cases {
		if got := classifySystemLanguage(in); got != want {
			t.Errorf("classifySystemLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseAppleLanguages(t *testing.T) {
	out := "(\n    \"en-US\",\n    \"zh-Hans-CN\"\n)\n"
	if got := parseAppleLanguages(out); got != "en-US" {
		t.Errorf("parseAppleLanguages = %q, want the first entry en-US", got)
	}
	if got := parseAppleLanguages("garbage"); got != "" {
		t.Errorf("parseAppleLanguages(garbage) = %q, want empty", got)
	}
}
