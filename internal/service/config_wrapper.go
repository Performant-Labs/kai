package service

import (
	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/settings"

	"cnb.cool/dtapp/kai/internal/hotkey"
)

// ConfigWrapper handles persistence and reads of UI config (language/theme/default
// engine/hotkeys/TTS/exec keys), plus resolution and external queries of language/theme.
// Engine config (Engines) is managed independently by EngineWrapper.
// Only exposes RPCs — none of the wails lifecycle trio.
type ConfigWrapper struct {
	settingsSvc *settings.Service
	app         *application.App
	hotkeyMgr   *hotkey.Manager
}

// NewConfigWrapper constructs the config Wrapper. app and hotkeyMgr may be injected after
// the startup orchestration.
func NewConfigWrapper(st *settings.Service, app *application.App, hm *hotkey.Manager) *ConfigWrapper {
	return &ConfigWrapper{settingsSvc: st, app: app, hotkeyMgr: hm}
}

// SetApp injects the app once it is ready.
func (w *ConfigWrapper) SetApp(app *application.App) {
	w.app = app
}

// Theme returns the user-configured theme (auto/light/dark, falling back to auto when
// unconfigured).
func (w *ConfigWrapper) Theme() string {
	if w.settingsSvc.Get() == nil {
		return string(model.ThemeAuto)
	}
	return w.settingsSvc.Get().Theme
}

// GetTheme returns the current theme config (may contain auto).
func (w *ConfigWrapper) GetTheme() string {
	if w.settingsSvc.Get() == nil || w.settingsSvc.Get().Theme == "" {
		return string(model.ThemeAuto)
	}
	return w.settingsSvc.Get().Theme
}

// GetSystemTheme returns the actual theme (dark/light) resolved from the current system
// appearance.
// matchMedia('prefers-color-scheme') inside the Wails webview may be unreliable on macOS, so
// the backend provides the single trusted source via application.Env.IsDarkMode().
func (w *ConfigWrapper) GetSystemTheme() string {
	if w.app != nil && w.app.Env.IsDarkMode() {
		return string(model.ThemeDark)
	}
	return string(model.ThemeLight)
}

// SetTheme persists the theme config.
func (w *ConfigWrapper) SetTheme(theme string) error {
	cfg := w.settingsSvc.Get()
	if cfg == nil {
		return nil
	}
	cfg.Theme = theme
	if err := w.settingsSvc.Save(); err != nil {
		return err
	}
	if w.app != nil {
		w.app.Event.Emit(events.EventThemeChanged, events.ThemeChangedPayload{
			Mode:  cfg.Theme,
			Theme: w.GetSystemTheme(),
		})
	}
	return nil
}

// SaveConfig persists UI config to settings.json; language/theme changes sync globally.
func (w *ConfigWrapper) SaveConfig(cfg *settings.Settings) error {
	cur := w.settingsSvc.Get()
	if cur == nil {
		return nil
	}
	langChanged := cur.Language != cfg.Language
	themeChanged := cur.Theme != cfg.Theme
	cur.Language = cfg.Language
	cur.Theme = cfg.Theme
	cur.DefaultTo = cfg.DefaultTo
	cur.DefaultFrom = cfg.DefaultFrom
	cur.DefaultEngine = cfg.DefaultEngine
	cur.Hotkeys = cfg.Hotkeys
	cur.TTS = cfg.TTS
	cur.ExecKeys = cfg.ExecKeys
	cur.AutoClipboard = cfg.AutoClipboard
	cur.AutoSwitchSource = cfg.AutoSwitchSource
	cur.DoubleCopyTranslate = cfg.DoubleCopyTranslate
	fontSizeChanged := cur.FontSize != settings.NormalizeFontSize(cfg.FontSize)
	cur.FontSize = settings.NormalizeFontSize(cfg.FontSize)
	cur.CopyKeySnapshot = cfg.CopyKeySnapshot
	cur.AnalyticsEnabled = cfg.AnalyticsEnabled
	if err := w.settingsSvc.Save(); err != nil {
		return err
	}
	// After hotkey config sync, re-register directly so saving takes effect immediately
	if w.hotkeyMgr != nil {
		w.hotkeyMgr.Register()
	}
	if langChanged {
		i18n.SetLocale(cur.Language)
	}
	if w.app != nil {
		if langChanged {
			w.app.Event.Emit(events.EventLocaleChanged, events.LocaleChangedPayload{
				Mode:     cur.Language,
				Language: cur.Language,
			})
		}
		if themeChanged {
			w.app.Event.Emit(events.EventThemeChanged, events.ThemeChangedPayload{
				Mode:  cur.Theme,
				Theme: w.GetSystemTheme(),
			})
		}
		if fontSizeChanged {
			w.app.Event.Emit(events.EventFontSizeChanged, cur.FontSize)
		}
	}
	return nil
}

// GetDoubleCopyStatus reports the double Cmd+C listener's state for the Settings page: "off",
// "running", "missing_permission" (the setting is on but Input Monitoring is not granted),
// "unsupported" or "error" (issue #199).
func (w *ConfigWrapper) GetDoubleCopyStatus() string {
	if w.hotkeyMgr == nil {
		return "off"
	}
	return w.hotkeyMgr.DoubleCopyStatus()
}

// NamedItem is an entry with a display name (for frontend dropdowns/lists).
type NamedItem struct {
	Value string `json:"value"` // Option value (identifier)
	Name  string `json:"name"`  // Option display name
}

// GetLanguages returns the languages the source/target dropdowns offer (value=code,
// name=display name): the SELECTABLE set — the recognized bare bases es / pt are left out.
func (w *ConfigWrapper) GetLanguages(lang string) []NamedItem {
	codes := model.SelectableLanguages()
	items := make([]NamedItem, 0, len(codes))
	for _, c := range codes {
		items = append(items, NamedItem{Value: string(c), Name: string(c)})
	}
	return items
}

// GetConfig returns the current config pointer.
func (w *ConfigWrapper) GetConfig() *settings.Settings {
	return w.settingsSvc.Get()
}
