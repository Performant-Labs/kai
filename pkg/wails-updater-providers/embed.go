package wails_updater_providers

import (
	_ "embed"
	"strings"
	"text/template"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

//go:embed updater_window.html
var updaterWindowHTMLRaw string

// Window returns the complete built-in update window config (the HTML already has copy
// injected per the current package-global locale/theme).
// Callers assign it straight to updater.Config.Window — no need to assemble BuiltinWindow
// themselves.
// Options/CSS stay zero-valued so the framework falls back to the default look (small,
// centered, resizable).
func (m *MirrorProvider) Window() *updater.BuiltinWindow {
	return &updater.BuiltinWindow{
		HTML: renderWindowHTML(nil),
		Options: updater.WindowOptions{
			Title: T("window_title_check"),
		},
	}
}

// renderWindowHTML injects copy into the embedded updater_window.html template per the
// current package-global locale/theme/current version and returns it.
// Colors follow the app theme GetTheme(): ThemeDark injects "dark", ThemeLight injects
// "light" (the HTML uses
// body[data-theme="..."] to decide content colors), consistent with recreateNativeWindow's
// BackgroundColour.
// locale/theme are read from globals inside T()/GetTheme() — no parameters needed (read live
// on every render, so language/colors hot-update).
// CurrentVersion is read from app.Updater.CurrentVersion(); a nil app injects an empty
// string.
// Template parse/exec failures fall back to the original embedded HTML (a graceful
// degradation, not fatal).
func renderWindowHTML(app *application.App) string {
	theme := GetTheme()
	tmpl, err := template.New("updaterWindow").Parse(updaterWindowHTMLRaw)
	if err != nil {
		return updaterWindowHTMLRaw
	}
	currentVersion := ""
	if app != nil && app.Updater != nil {
		currentVersion = app.Updater.CurrentVersion()
	}
	data := map[string]string{
		"Theme":          themeHTMLTheme(theme),
		"Lang":           string(GetLocale()),
		"CurrentVersion": currentVersion,
	}
	var sb strings.Builder
	if err := tmpl.Execute(&sb, data); err != nil {
		return updaterWindowHTMLRaw
	}
	return sb.String()
}

// themeHTMLTheme converts a Theme into the value for <body data-theme="...">.
// ThemeDark → "dark"；ThemeLight → "light"。
func themeHTMLTheme(theme Theme) string {
	if theme == ThemeDark {
		return "dark"
	}
	return "light"
}
