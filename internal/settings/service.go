package settings

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/spf13/viper"

	"cnb.cool/dtapp/kai/internal/model"
)

// Settings is the app settings (UI preferences only; engine config is stored independently
// in config.db, not here).
type Settings struct {
	// Language is the UI language: auto / zh-CN / en-US. New installs default to en-US;
	// auto follows the system language (a Chinese system selects Chinese, anything else English).
	// Note: this is the app's display language — separate from the translation languages
	// (model.Language: auto/zh/en/...). Two independent systems; never mix them.
	Language string `json:"language" mapstructure:"language"`
	// Theme is the UI theme: auto / light / dark (auto follows the system appearance).
	Theme string `json:"theme" mapstructure:"theme"`
	// DefaultTo is the default translation target language (e.g. zh / en). A fresh install starts
	// at DefaultTarget (en); a value persisted here is honored as saved (issue #44).
	DefaultTo string `json:"default_to" mapstructure:"default_to"`
	// DefaultFrom is the default translation source language (auto = auto-detect).
	DefaultFrom string `json:"default_from" mapstructure:"default_from"`
	// DefaultEngine is the primary (default) translation engine identifier (an engine name
	// like "google"; an empty string means unset).
	// When unset / pointing at a missing or disabled engine, resolution falls back to the
	// first enabled translation engine.
	DefaultEngine string `json:"default_engine" mapstructure:"default_engine"`
	// Hotkeys are registration-type global hotkeys (fired when the user presses them).
	Hotkeys RegisteredHotkeyConfig `json:"hotkeys" mapstructure:"hotkeys"`
	// ExecKeys are execution-type hotkeys (the program actively simulates pressing them to
	// perform an action).
	ExecKeys ExecKeyConfig `json:"execkeys" mapstructure:"execkeys"`
	// AutoClipboard is the input-translate window's "auto-read clipboard and translate"
	// switch. When on, the frontend polls the system clipboard and fills + translates the
	// input on change; it also automatically disables the copy key (avoiding double
	// triggering), snapshotting its prior state in CopyKeySnapshot.
	AutoClipboard bool `json:"auto_clipboard" mapstructure:"auto_clipboard"`
	// CopyKeySnapshot records the copy key's prior state (enabled/fallback) when
	// AutoClipboard is switched on, restoring from it when AutoClipboard is switched off;
	// nil when never enabled.
	CopyKeySnapshot *ExecKeyEntry `json:"copy_key_snapshot" mapstructure:"copy_key_snapshot"`
	// TTS is the text-to-speech config.
	TTS TTSConfig `json:"tts" mapstructure:"tts"`
	// HttpLog is the HTTP request-log config (not yet synced to the frontend UI).
	HttpLog HttpLogConfig `json:"http_log" mapstructure:"http_log"`
	// Log is the app runtime-log config (level / cleanup / compression).
	Log LogConfig `json:"log" mapstructure:"log"`
	// Proxy is the network-proxy config (not yet synced to the frontend UI).
	Proxy ProxyConfig `json:"proxy" mapstructure:"proxy"`
	// Updater is the auto-update config (only user-decidable items; token/provider are fixed
	// in code).
	Updater UpdaterConfig `json:"updater" mapstructure:"updater"`
	// AnalyticsEnabled is the anonymous usage-stats switch (off by default, opt-in). When
	// off, the PostHog client is never initialized and nothing is reported.
	// Both the config file and the frontend UI share this field (Go reads settings.json; the
	// frontend reads/writes via GetConfig/SaveConfig).
	AnalyticsEnabled bool `json:"analytics_enabled" mapstructure:"analytics_enabled"`
	// AnalyticsInstalled records whether app_installed was already sent (first-install event
	// reported once).
	AnalyticsInstalled bool `json:"analytics_installed" mapstructure:"analytics_installed"`
	// DNSConfigs is the custom DNS resolver config list.
	DNSConfigs []DNSConfig `json:"dns_configs" mapstructure:"dns_configs"`
	// TranslateWindowWidth/Height persist the input-translate window's last user-resized size
	// (issue #173 item 5), read back into WebviewWindowOptions.Width/Height at window
	// creation in main.go so a resize survives a relaunch. Zero means "never resized yet" —
	// main.go falls back to the hardcoded default in that case. Window size is a native
	// (Go-side) property, not frontend state, so unlike Theme/Language above this is written
	// directly by main.go's resize listener (debounced), not via SaveConfig/the frontend.
	// Scoped to the translate window only (the window users actually resize); the settings
	// and screenshot windows keep their fixed defaults — see docs/handoffs/173-brief.md.
	TranslateWindowWidth  int `json:"translate_window_width" mapstructure:"translate_window_width"`
	TranslateWindowHeight int `json:"translate_window_height" mapstructure:"translate_window_height"`
	// Path is the config file path (not persisted; json:"-").
	Path string `json:"-"`
}

// HotkeyEntry is a single registration-type hotkey: stores both the key combination and its
// enabled state.
// The previous struct stored only the key string with no per-item enabled state; it is now
// split into Key (the combination; empty string = unset) and Enabled. Unset Key or disabled
// means never registered.
type HotkeyEntry struct {
	// Key is the key combination; an empty string means unset.
	Key string `json:"key" mapstructure:"key"`
	// Enabled is whether this hotkey is enabled.
	Enabled bool `json:"enabled" mapstructure:"enabled"`
}

// RegisteredHotkeyConfig holds registration-type global hotkeys (all listened to via
// mgr.Register; fired when the user presses them).
// Each hotkey is a HotkeyEntry with key and enabled state.
type RegisteredHotkeyConfig struct {
	// Input is the summon-main-window hotkey (core feature, enabled by default).
	Input HotkeyEntry `json:"input" mapstructure:"input"`
	// Screenshot is the screenshot-translate hotkey.
	Screenshot HotkeyEntry `json:"screenshot" mapstructure:"screenshot"`
}

// ExecKeyEntry is a single execution-type hotkey: the key the program actively simulates
// pressing + whether it is enabled.
type ExecKeyEntry struct {
	// Key is the key combination the program actively simulates pressing.
	Key string `json:"key" mapstructure:"key"`
	// Enabled is whether this exec key is enabled.
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// Fallback: when simulated copy fails (empty clipboard / injection failure), whether to
	// retry with the system default copy key.
	// When on: if the custom copy key didn't take effect, automatically run the system-native
	// copy key once (macOS Cmd+C / elsewhere Ctrl+C) instead of giving up on copying.
	Fallback bool `json:"fallback" mapstructure:"fallback"`
}

// ExecKeyConfig holds execution-type hotkeys: the program actively simulates pressing these
// keys via robotgo to perform actions.
// They are not part of mgr.Register (they are "pressed keys", not "listened-for keys").
type ExecKeyConfig struct {
	// Copy is the copy key: the program simulates pressing it to perform the system copy
	// (e.g. macOS Cmd+C), then reads the clipboard to translate.
	Copy ExecKeyEntry `json:"copy" mapstructure:"copy"`
}

// TTSConfig is the text-to-speech config.
type TTSConfig struct {
	// Engine is the TTS engine identifier (e.g. system).
	Engine string `json:"engine" mapstructure:"engine"`
	// Speed is the speech-rate multiplier; 1.0 is normal speed.
	Speed float64 `json:"speed" mapstructure:"speed"`
}

// HttpLogConfig is the HTTP request-log config (not yet synced to the frontend UI).
type HttpLogConfig struct {
	// Enabled is whether HTTP request logging is on.
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// RetentionDays is the log retention in days.
	RetentionDays int `json:"retention_days" mapstructure:"retention_days"`
}

// LogConfig is the app runtime-log config (written only to settings.json, consumed by main's
// logging module; not synced directly to the frontend UI).
// Level supports debug / info / warn / error; cleanup policy is based on day-rotated log
// files.
type LogConfig struct {
	// Level is the log level: debug / info / warn / error (empty or invalid falls back to
	// info).
	Level string `json:"level" mapstructure:"level"`
	// RetentionDays is how many days to keep log files (<=0 = never clean up).
	RetentionDays int `json:"retention_days" mapstructure:"retention_days"`
	// Compress is whether expired old log files are compressed to .gz (within the retention
	// window).
	Compress bool `json:"compress" mapstructure:"compress"`
}

// DNSConfig is a DNS resolver config.
type DNSConfig struct {
	// Enabled is whether this DNS config is active.
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// Servers is the list of DNS server addresses.
	Servers []string `json:"servers" mapstructure:"servers"`
}

// ProxyConfig is the network-proxy config (not yet synced to the frontend UI).
type ProxyConfig struct {
	// Enabled is whether the proxy is on.
	Enabled bool `json:"enabled" mapstructure:"enabled"`
	// Protocol is the proxy protocol: http / https / socks5.
	Protocol string `json:"protocol" mapstructure:"protocol"`
	// Host is the proxy host address.
	Host string `json:"host" mapstructure:"host"`
	// Port is the proxy port.
	Port int `json:"port" mapstructure:"port"`
	// Username is the proxy auth username (optional).
	Username string `json:"username" mapstructure:"username"`
	// Password is the proxy auth password (optional).
	Password string `json:"password" mapstructure:"password"`
}

// UpdaterConfig holds auto-update config (only user-decidable items like Prerelease /
// Source; token / provider / asset-matching rules are fixed in main.go's code, not
// configured here).
type UpdaterConfig struct {
	// Prerelease is whether pre-release updates may be detected.
	// When on: update checks include the GitHub repo's pre-release versions as candidates.
	Prerelease bool `json:"prerelease" mapstructure:"prerelease"`
	// Source selects the update-check source: empty / "github" / "cnb".
	//   - empty (default): current logic — auto-pick per UI language (English → GitHub,
	//     Chinese → CNB).
	//   - "github": force official GitHub only (with SHA256SUMS verification).
	//   - "cnb": force the CNB mirror only (needs cnbToken; anonymous 401 / unreachable
	//     network is treated as "no update").
	// Note: only affects the "check / download source" choice, not the updater's own install
	// behavior.
	Source string `json:"source" mapstructure:"source"`
}

// Update source value constants (matching UpdaterConfig.Source).
const (
	// UpdaterSourceGitHub forces the official GitHub source.
	UpdaterSourceGitHub = "github"
	// UpdaterSourceCNB forces the CNB mirror source.
	UpdaterSourceCNB = "cnb"
)

// DefaultTarget is the policy default target language (issue #44): what a fresh install
// translates into, and the fallback for a persisted target that cannot be used. It is English
// because the core workflow is foreign-language text read in English (a Chinese default turns a
// Mandarin source into a same-language pass). The frontend windows' own initial value is English
// too, so a fresh install reads English both before and after the settings load.
//
// It is only a default: a target the user saved (default_to) is never rewritten. The
// same-language case for such installs is handled per request, where the detected source is known
// (translate.Service).
const DefaultTarget = model.EN

// DefaultSettings returns the default settings pointer
func DefaultSettings() *Settings {
	return &Settings{
		Language:    string(model.LocaleENUS),
		Theme:       string(model.ThemeAuto),
		DefaultTo:   string(DefaultTarget),
		DefaultFrom: string(model.Auto),
		Hotkeys: RegisteredHotkeyConfig{
			// Input (summon main window): defaults to Alt+A and enabled.
			// Screenshot: off by default; the user enables and binds a key in the settings
			// page.
			Input:      HotkeyEntry{Key: "Alt+A", Enabled: true},
			Screenshot: HotkeyEntry{Key: "Alt+S", Enabled: false},
		},
		ExecKeys: ExecKeyConfig{
			Copy: ExecKeyEntry{Key: defaultCopyHotkey(), Enabled: true, Fallback: false},
		},
		TTS:     TTSConfig{Engine: "system", Speed: 1.0},
		HttpLog: HttpLogConfig{Enabled: true, RetentionDays: 30},
		Log:     LogConfig{Level: "info", RetentionDays: 30, Compress: true},
		Proxy:   ProxyConfig{Enabled: false, Protocol: "http", Port: 8080},
		Updater: UpdaterConfig{Prerelease: true},
		// Anonymous analytics defaults to off (privacy first, opt-in); users can enable it in
		// the settings page.
		// Dev builds and unconfigured keys never report even when enabled.
		AnalyticsEnabled: false,
	}
}

// normalizeLanguages coerces persisted language choices that predate the dialect variants
// (issue #52): bare es / pt are recognized but no longer selectable, so a stored default_to of
// "es" would match no option in the dropdowns. A bare base becomes the first selectable variant
// of its family (es → es-MX, pt → pt-BR); on the source side that is lossless, since every
// engine aliases a variant source to its base. Anything else, unknown codes included, is left
// untouched. A family with no selectable variant falls back to the policy defaults (auto source,
// DefaultTarget target).
func (c *Settings) normalizeLanguages() {
	c.DefaultFrom = string(model.Language(c.DefaultFrom).SelectableOr(model.Auto))
	c.DefaultTo = string(model.Language(c.DefaultTo).SelectableOr(DefaultTarget))
}

// defaultCopyHotkey returns the copy key's default, per OS:
// macOS defaults to Cmd+C (the system-native copy key), other platforms to Ctrl+C.
// The per-OS decision lives in config (here); the execution layer only resolves the
// simulation from the user config (ExecKeys.Copy) — no hardcoded platform keys.
func defaultCopyHotkey() string {
	if runtime.GOOS == "darwin" {
		return "Cmd+C"
	}
	return "Ctrl+C"
}

// OnChangeFunc is a config-change callback (hot reload).
type OnChangeFunc func(newCfg *Settings)

// Service is the settings service (modeled on certflow's viper Service pattern)
type Service struct {
	mu       sync.RWMutex
	cfg      *Settings
	v        *viper.Viper
	filePath string
	onChange OnChangeFunc
	saving   bool // Marks a save in progress, avoiding re-triggering our own callback
}

// NewService creates the settings service: creates directories, reads/writes defaults, and
// watches for file changes.
func NewService(dataDir string) (*Service, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}
	filePath := filepath.Join(dataDir, "settings.json")

	v := viper.New()
	v.SetConfigFile(filePath)
	v.SetConfigType("json")

	s := &Service{
		filePath: filePath,
		cfg:      DefaultSettings(),
		v:        v,
	}

	s.setDefaults()

	if err := v.ReadInConfig(); err != nil {
		// First run: write default config
		if os.IsNotExist(err) {
			if err := s.writeConfig(); err != nil {
				return nil, fmt.Errorf("failed to write default settings: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to read settings: %w", err)
		}
	}

	if err := v.Unmarshal(s.cfg); err != nil {
		return nil, fmt.Errorf("failed to parse settings: %w", err)
	}
	s.cfg.Path = filePath
	s.cfg.normalizeLanguages()

	// Re-save after startup so disk config matches memory (missing defaults are backfilled;
	// extras are overwritten by the write)
	if err := s.writeConfig(); err != nil {
		return nil, fmt.Errorf("failed to write settings: %w", err)
	}

	s.startWatching()
	return s, nil
}

// setDefaults writes defaults into viper (fallback for missing fields)
func (s *Service) setDefaults() {
	def := DefaultSettings()
	s.v.SetDefault("language", def.Language)
	s.v.SetDefault("theme", def.Theme)
	s.v.SetDefault("default_to", def.DefaultTo)
	s.v.SetDefault("default_from", def.DefaultFrom)
	s.v.SetDefault("default_engine", def.DefaultEngine)
	s.v.SetDefault("hotkeys", def.Hotkeys)
	s.v.SetDefault("execkeys", def.ExecKeys)
	s.v.SetDefault("tts", def.TTS)
	s.v.SetDefault("http_log", def.HttpLog)
	s.v.SetDefault("log", def.Log)
	s.v.SetDefault("proxy", def.Proxy)
	s.v.SetDefault("updater", def.Updater)
	s.v.SetDefault("analytics_enabled", def.AnalyticsEnabled)
}

// startWatching watches the config file for changes (500ms debounce)
func (s *Service) startWatching() {
	var (
		debounceTimer *time.Timer
		timerMu       sync.Mutex
	)

	s.v.OnConfigChange(func(e fsnotify.Event) {
		s.mu.Lock()
		if s.saving {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()

		timerMu.Lock()
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
		debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
			s.mu.Lock()
			defer s.mu.Unlock()

			if err := s.v.Unmarshal(s.cfg); err != nil {
				slog.Default().Error("settings hot reload failed", slog.Any("error", err))
				return
			}
			s.cfg.Path = s.filePath
			s.cfg.normalizeLanguages()

			if s.onChange != nil {
				cb := s.onChange
				cfg := s.cfg
				go cb(cfg)
			}
		})
		timerMu.Unlock()
	})

	s.v.WatchConfig()
}

// OnChange registers a config-change callback
func (s *Service) OnChange(fn OnChangeFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onChange = fn
}

// writeConfig writes only the current cfg's fields back to the config file using a fresh
// viper instance, thereby cleaning out deprecated keys lingering in the file (avoiding
// v.WriteConfig writing old keys back wholesale).
// Marks saving to prevent self-triggering the hot reload.
func (s *Service) writeConfig() error {
	s.saving = true
	defer func() { s.saving = false }()

	w := viper.New()
	w.SetConfigType("json")
	w.Set("language", s.cfg.Language)
	w.Set("theme", s.cfg.Theme)
	w.Set("default_to", s.cfg.DefaultTo)
	w.Set("default_from", s.cfg.DefaultFrom)
	w.Set("default_engine", s.cfg.DefaultEngine)
	w.Set("hotkeys", s.cfg.Hotkeys)
	w.Set("execkeys", s.cfg.ExecKeys)
	w.Set("auto_clipboard", s.cfg.AutoClipboard)
	w.Set("copy_key_snapshot", s.cfg.CopyKeySnapshot)
	w.Set("tts", s.cfg.TTS)
	w.Set("http_log", s.cfg.HttpLog)
	w.Set("log", s.cfg.Log)
	w.Set("proxy", s.cfg.Proxy)
	w.Set("updater", s.cfg.Updater)
	w.Set("analytics_enabled", s.cfg.AnalyticsEnabled)
	w.Set("analytics_installed", s.cfg.AnalyticsInstalled)
	w.Set("translate_window_width", s.cfg.TranslateWindowWidth)
	w.Set("translate_window_height", s.cfg.TranslateWindowHeight)

	return w.WriteConfigAs(s.filePath)
}

// Save persists the config
func (s *Service) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeConfig()
}

// Get returns the current config (read-only for callers — do not mutate the returned
// pointer)
func (s *Service) Get() *Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}
