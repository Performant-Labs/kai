package wails_updater_providers

import "strings"

// Locale is a language code type.
type Locale string

// Supported language (locale) constants: keeps callers from scattering bare "zh-CN" /
// "en-US" strings.
const (
	LocaleZhCN Locale = "zh-CN"
	LocaleEnUS Locale = "en-US"
)

// Theme is the update window theme type.
type Theme string

// Update window theme constants: package globals set by SetTheme.
// Callers can inject their app's own theme so the update dialog matches the host app's
// colors instead of relying on the system media query.
const (
	// ThemeDark forces dark.
	ThemeDark Theme = "dark"
	// ThemeLight forces light.
	ThemeLight Theme = "light"
)

// Source is an update source type.
type Source string

// Update source constants: package globals set by SetSource; also used by
// Provider.Name().
const (
	SourceCNB    Source = "cnb"
	SourceGithub Source = "github"
	SourceAuto   Source = "auto"
)

// MetadataKey holds key constants written into updater.Release.Metadata, avoiding scattered
// bare strings.
const (
	// MetadataReleaseHTMLURL is the release page URL (for frontend links/display).
	MetadataReleaseHTMLURL = "release.htmlURL"
)

// normalizeLocale normalizes any locale string into a supported Locale;
// empty or unknown locales go through the alias table, and anything still unmatched falls
// back to zh-CN (the default language).
func normalizeLocale(locale string) Locale {
	if _, ok := i18nMessages[Locale(locale)]; ok {
		return Locale(locale)
	}
	if alias, ok := localeAliases[strings.ToLower(strings.ReplaceAll(locale, "-", "_"))]; ok {
		return Locale(alias)
	}
	return (LocaleZhCN)
}

// normalizeTheme normalizes any theme into a supported Theme;
// empty or unknown themes fall back to ThemeDark (the default dark).
func normalizeTheme(theme Theme) Theme {
	switch theme {
	case ThemeLight, ThemeDark:
		return theme
	default:
		return ThemeDark
	}
}

// normalizeSource normalizes any source preference into a supported Source;
// empty or unknown values fall back to SourceAuto (primary source picked by language).
func normalizeSource(src Source) Source {
	switch src {
	case SourceCNB, SourceGithub, SourceAuto:
		return src
	default:
		return SourceAuto
	}
}
