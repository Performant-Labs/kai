package main

import (
	"context"
	"embed"
	"log"
	"log/slog"
	"math/rand"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"cnb.cool/dtapp/kai/internal/analytics"
	"cnb.cool/dtapp/kai/internal/buildinfo"
	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	kevents "cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/execkey"
	"cnb.cool/dtapp/kai/internal/historystore"
	"cnb.cool/dtapp/kai/internal/hotkey"
	"cnb.cool/dtapp/kai/internal/httplogstore"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/langpref"
	"cnb.cool/dtapp/kai/internal/logutil"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/network"
	"cnb.cool/dtapp/kai/internal/selection"
	"cnb.cool/dtapp/kai/internal/service"
	"cnb.cool/dtapp/kai/internal/settings"
	"cnb.cool/dtapp/kai/internal/translate"
	"cnb.cool/dtapp/kai/internal/webview"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
	kupdater "cnb.cool/dtapp/kai/pkg/wails-updater-providers"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/trayicon.png
var trayIcon []byte

// parseBuildTime parses the RFC3339 string injected at build time into a time.Time.
// Dev builds inject placeholder strings like "unknown"; on parse failure it returns the
// zero value, which the updater uses (buildTime.IsZero()) to skip nightly comparison.
func parseBuildTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// resolveUpdaterLocale resolves the language from settings (may be "auto") to a concrete
// value accepted by the third-party library. "auto" is resolved via the i18n active locale
// (already resolved from the system language project-wide); non-auto values pass through.
func resolveUpdaterLocale(lang string) string {
	if lang == string(model.LocaleAuto) || lang == "" {
		return i18n.GetLocale()
	}
	return lang
}

// resolveUpdaterTheme resolves the theme from settings (may be "auto") to a concrete value
// accepted by the third-party library. "auto" resolves via the real system appearance
// (IsDarkMode) to light/dark; non-auto values pass through.
func resolveUpdaterTheme(theme string, app *application.App) string {
	if theme == string(model.ThemeAuto) || theme == "" {
		if app != nil && app.Env.IsDarkMode() {
			return string(model.ThemeDark)
		}
		return string(model.ThemeLight)
	}
	return theme
}

func init() {
	// Register custom event types for backend emit / frontend listeners (mirrors certflow's
	// RegisterEvent pattern). The emitted payload type must exactly match the registered type,
	// otherwise Wails3 validateCustomEvent panics.
	application.RegisterEvent[kevents.LocaleChangedPayload](kevents.EventLocaleChanged)
	application.RegisterEvent[kevents.ThemeChangedPayload](kevents.EventThemeChanged)
	application.RegisterEvent[string](kevents.EventWindowShow)
	application.RegisterEvent[string](kevents.EventWindowClosing)
	application.RegisterEvent[bool](kevents.EventAutoClipboardChanged)
	application.RegisterEvent[[]string](kevents.EventHotkeysChanged)
	application.RegisterEvent[string](kevents.EventInputFill)
	application.RegisterEvent[model.TranslateResult](kevents.EventTranslateResult)
	application.RegisterEvent[model.ScreenshotResult](kevents.EventScreenshotOCR)
	application.RegisterEvent[struct{}](kevents.EventWindowScreenshot)
	application.RegisterEvent[struct{}](kevents.EventScreenshotRecapture)
	application.RegisterEvent[kevents.ScreenshotRetranslatePayload](kevents.EventScreenshotRetranslate)
}

// formatBuildTime parses the UTC RFC3339 time injected at build time (e.g. 2006-01-02T15:04:05Z)
// into a readable local-timezone format (2006-01-02 15:04:05); on parse failure it returns the
// input unchanged, and an empty string returns "-".
func formatBuildTime(raw string) string {
	if raw == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func main() {
	// ── Phase one: create the data directories (runs earliest; the user language is not yet
	// known, so failure messages are hardcoded rather than localized) ──
	// Dev builds (buildinfo.IsDev() is true, i.e. `wails3 dev` / VERSION not injected) use a
	// separate .kai.dev directory, isolated from the release .kai, so debug data never
	// pollutes production data (mirrors certflow).
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("failed to get user home directory: %v", err)
	}
	dataDir := buildinfo.DataDir(homeDir)
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("failed to create data directory: %v", err)
	}
	// Database files (config.db / history.db / httplog.db) all live in the data/ subdirectory
	// of DataDir, i.e. ~/.kai/data/ (or ~/.kai.dev/data/ for dev). buildinfo.DBDir owns the layout.
	dbDir := buildinfo.DBDir(homeDir)
	if err := os.MkdirAll(dbDir, 0o755); err != nil {
		log.Fatalf("failed to create database directory: %v", err)
	}

	// ── Phase one-and-a-half: dynamically load the Swift bridge layer (purego runtime Dlopen) ──
	// Must complete before any kai_* call (SetLogConfig/SetBridgeLocale/OCR/Translate etc.).
	// Init("") loads libkai_bridge.dylib from the same directory as this source file by default
	// (in dev it lives in pkg/swiftbridge/); when packaged into an app bundle the build script
	// copies it in and passes an absolute path.
	// Load failure is not fatal (logged only): function variables with missing symbols stay nil
	// and error out inside their package when called, so the app still compiles and starts its
	// remaining features on non-macOS or when the dylib is missing.
	if err := swiftbridge.Init(""); err != nil {
		log.Printf("WARN: failed to load Swift bridge dynamically (some macOS-only features unavailable): %v", err)
	}

	// ── Phase two: load settings (logging/i18n depend on it; must run before database init) ──
	settingsService, err := settings.NewService(dataDir)
	if err != nil {
		log.Fatalf("failed to load settings: %v", err)
	}
	// Initialize anonymous analytics (loads the device ID; whether anything is actually reported
	// is decided at Track time by the switch + build mode).
	analytics.Init(dataDir, settingsService)
	logCfg := settingsService.Get().Log

	// Updater provider handle (assigned during init); on runtime language/theme changes it is
	// synced dynamically via SetLocale/SetTheme so the update dialog copy and colors follow live.
	var updaterProvider *kupdater.MirrorProvider
	// app is promoted to a function-level variable so the OnChange closures registered above
	// (before application.New) can reference it.
	var app *application.App

	// Main log: day-rotated writes to dataDir/logs/kai.log (tail -f anytime),
	// with level/retention days/compression all taken from the log section of settings.json —
	// nothing hardcoded.
	logRotator := initLogging(homeDir, logCfg)

	// Global safety net: catches panics not recovered in the main flow or any goroutine.
	// Writes an ERROR log first (ensuring it lands before Close flushes), then closes the log
	// file and exits, so the process never disappears silently without a trace.
	// Must be registered after initLogging.
	defer func() {
		if r := recover(); r != nil {
			slog.Error(i18n.T("log.global_panic"), "panic", r)
			slog.Error(i18n.T("log.global_panic_stack"), "stack", string(debug.Stack()))
			_ = logRotator.Close()
		}
	}()
	slog.Info(i18n.T("log.app_starting", "Version", buildinfo.Version, "BuildTime", formatBuildTime(buildinfo.BuildTime), "GitCommit", buildinfo.GitCommit))
	slog.Info(i18n.T("log.data_dir", "Dir", dataDir))

	// Frontend logs are written separately to dataDir/logs/frontend.log (frontend console /
	// JS errors are forwarded here). Initial level/retention days/compression also come from
	// the log section of settings.json; applyLogConfig keeps them in sync from then on.
	frontendLogFW, err := logutil.NewFrontendWriter(buildinfo.LogDir(homeDir), logutil.ParseLevel(logCfg.Level), logCfg.RetentionDays, logCfg.Compress)
	if err != nil {
		log.Printf(i18n.T("log.frontend_log_init_failed"), err)
	}
	frontendLogSvc := logutil.NewFrontendLogService(frontendLogFW)

	// ── Phase three: database initialization (must run after phase two "load settings",
	// once logging/i18n are ready) ──
	// Initialize the HTTP request log store: must be armed before engine registration
	// (BuildHTTPClient→WrapTransport), otherwise httplog is not enabled and the logging layer
	// is not wrapped, so translate requests never reach the store.
	httpLogCfg := settingsService.Get().HttpLog
	slog.Info(i18n.T("log.http_log_status"), "enabled", httpLogCfg.Enabled, "retention_days", httpLogCfg.RetentionDays, "dbDir", dbDir)
	if err := httplogstore.Init(dbDir, httpLogCfg.Enabled); err != nil {
		slog.Warn(i18n.T("log.http_log_init_failed"), "error", err)
	} else {
		slog.Info(i18n.T("log.http_log_initialized"), "enabled", httpLogCfg.Enabled, "db", dbDir+"/httplog.db")
	}
	// Start periodic cleanup of expired logs: Init has connDSN ready only when enabled;
	// with retention_days<=0 StartCleanup returns immediately — safe.
	httplogstore.StartCleanup(httpLogCfg.RetentionDays, slog.Default())

	i18n.SetLocale(settingsService.Get().Language)

	// Runtime language switching: when settings.json is changed externally / hot-reloaded,
	// OnChange syncs the backend i18n locale so backend errors/log copy follow the UI language
	// (complements the proactive sync inside SaveConfig). Also syncs log level / cleanup policy
	// (log section).
	settingsService.OnChange(func(cfg *settings.Settings) {
		i18n.SetLocale(cfg.Language)
		applyLogConfig(logRotator, frontendLogSvc, cfg.Log, homeDir)
		// Sync language/theme changes to the updater (auto resolves to the real language/system
		// appearance), refreshing subsequent Check/Download copy.
		if updaterProvider != nil {
			kupdater.SetLocale(kupdater.Locale(resolveUpdaterLocale(cfg.Language)))
			// Sync theme changes to the updater (auto resolves to the real system appearance, light/dark).
			kupdater.SetTheme(kupdater.Theme(resolveUpdaterTheme(cfg.Theme, app)))
			// Refresh the built-in updater window (the library rebuilds it in place to apply the
			// latest language/theme copy).
			kupdater.SetUpdaterLocaleTheme(app)
		}
	})

	// Apply the log config once at startup (level/cleanup policy/compression may be overridden
	// by the log section of settings.json), and sync the same LogConfig to the Swift bridge
	// layer so kai-bridge.log matches kai.log.
	applyLogConfig(logRotator, frontendLogSvc, settingsService.Get().Log, homeDir)

	// Explicit dependency injection for domain packages and thin wrappers (replaces the old
	// ServiceContext mega-container). At construction time app does not exist yet, so nil is
	// passed as a placeholder; after application.New, AppService.SetApp injects it uniformly.
	reg := engine.NewRegistry()
	// History DB / engine config DB (phase three: databases)
	histDB, err := historystore.Open(filepath.Join(dbDir, "history.db"))
	if err != nil {
		log.Fatalf(i18n.T("log.open_history_db_failed"), err)
	}
	cfgDB, err := configstore.Open(filepath.Join(dbDir, "config.db"))
	if err != nil {
		log.Fatalf(i18n.T("log.open_config_db_failed"), err)
	}

	trSvc := translate.NewService(reg, histDB, settingsService, nil)
	trSvc.SetConfigStore(cfgDB)
	// Language-variant preferences (issue #53): one process-wide, in-memory store shared by
	// both translate windows. The translate service qualifies detected languages through it, the
	// LangPrefWrapper below feeds it from the frontend's explicit language selections. The session
	// is the process: a learned variant is kept until the app quits (hiding a window does not
	// forget it), and the next launch starts empty.
	langPrefs := langpref.New()
	trSvc.SetLangPrefs(langPrefs)
	selSvc := selection.NewService(nil, settingsService)

	// Top-level service references (resolved lazily via closures; assigned by runtime)
	var appSvc *service.AppService
	var windowSvc *service.WindowWrapper
	// Notification service: wraps permission checks and safe sending, reusing the
	// Wails-registered singleton.
	var notifySvc *service.NotificationService

	// Exec key: after a copy key fires, the selection is fed back into the main window
	ekCtrl := execkey.NewExecKeyController(settingsService, nil, selSvc)

	// Hotkey manager: callback closures bridged to the domain service
	hm := hotkey.NewManager(nil, settingsService, ekCtrl,
		func() application.Window { return translateWindow },
		func() error { _, err := trSvc.ScreenshotOCR(reg.DefaultOCREngineName()); return err },
		func() error {
			_, err := trSvc.ScreenshotTranslate(kevents.ScreenshotSessionScreenshot)
			return err
		},
		func() application.Window { return screenshotWindow },
		func(active []string) {
			if appSvc != nil {
				appSvc.EmitHotkeysChanged(active)
			}
		},
	)

	windowSvc = service.NewWindowWrapper(nil)
	configSvc := service.NewConfigWrapper(settingsService, nil, hm)
	engineSvc := service.NewEngineWrapper(reg, cfgDB, settingsService, nil, hm)
	historySvc := service.NewHistoryWrapper(histDB, cfgDB)
	translateSvc := service.NewTranslateWrapper(trSvc)
	langPrefSvc := service.NewLangPrefWrapper(langPrefs)
	appSvc = service.NewAppService(settingsService, trSvc, ekCtrl, hm, reg, histDB, cfgDB, nil)
	// The wails notifications singleton must be initialized first (notifications.New is what
	// assigns NotificationService_); otherwise it is a nil pointer and, when passed to
	// application.NewService, wails calls reflect.Value.Type on the nil pointer during method
	// binding → panic "reflect.Value.Type on zero Value".
	notifications.New()
	notifySvc = service.NewNotificationService(notifications.NotificationService_)

	appOpts := application.Options{
		Name:        "Kai",
		Description: i18n.T("app.description"),
		Services: []application.Service{
			// Core services (translate/OCR/accessibility/lifecycle), centrally orchestrating startup
			application.NewService(appSvc),
			// Domain thin wrappers (frontend calls per domain)
			application.NewService(configSvc),
			application.NewService(engineSvc),
			application.NewService(historySvc),
			application.NewService(translateSvc),
			application.NewService(langPrefSvc),
			application.NewService(windowSvc),
			// Frontend log bridge: receives frontend console / JS errors, writes logs/frontend.log
			application.NewService(frontendLogSvc),
			// Native desktop notifications (NotificationService wrapper with permission checks;
			// macOS goes through UNUserNotificationCenter, no longer forwarded via frontend
			// Web Notification)
			application.NewService(notifySvc),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			// Accessory: agent app — no Dock icon and no menu bar from launch, tray only.
			// Set by Wails during Cocoa initialization, earlier than a runtime HideAppIcon,
			// so no flicker.
			ActivationPolicy: application.ActivationPolicyAccessory,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		// Single instance: built into Wails v3 (macOS uses flock + NSDistributedNotification to
		// notify the first instance). On a second launch, the second process triggers
		// OnSecondInstanceLaunch and exits itself; the first instance brings the translate window
		// (the app's main surface) to the foreground, avoiding multi-instance contention over the
		// database/hotkeys/tray.
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "cnb.cool.dtapp.kai",
			OnSecondInstanceLaunch: func(data application.SecondInstanceData) {
				slog.Info(i18n.T("log.single_instance_second_launch"))
				// Kai is an Accessory app (no persistent main window, tray only); on second launch,
				// bring the existing instance's translate window to the foreground (issue #69: the
				// translate window, not Settings, matching a tray click's show). Go through
				// WindowWrapper.ShowTranslateWindow (showAndFocus), never a bare Show().Focus(): the
				// translate window is created hidden, so Wails builds its webview lazily and a lone
				// Show() would only build it without displaying. Show/Focus is dispatched safely on
				// the main thread.
				windowSvc.ShowTranslateWindow()
			},
		},
	}
	// Windows-specific: inject WebView2 browser arguments (GPU fallback to avoid sporadic 80010108).
	// AdditionalBrowserArgs is a Windows-only field of application.Options, so webview.ApplyOptions
	// sets it per platform (a no-op on non-Windows).
	webview.ApplyOptions(&appOpts)
	app = application.New(appOpts)
	appSvc.SetApp(app) // Inject app uniformly into the domain services and wrappers held inside AppService
	// Wrapper instances bound directly on the main.go side (a different set from the ones inside
	// AppService) also need app to run Event.Emit broadcasts, so each is injected here.
	ekCtrl.SetApp(app) // Propagate app to execKeyCtrl and its selection.Service (clipboard reading depends on it)
	configSvc.SetApp(app)
	engineSvc.SetApp(app)
	windowSvc.SetApp(app)
	trSvc.SetApp(app) // Propagate app to translate.Service; screenshot OCR/translate results rely on Event.Emit broadcasts to the frontend

	// Frontend summons windows via runtime.EventsEmit('kai:window:show', 'settings'|'translate')
	app.Event.On(kevents.EventWindowShow, func(e *application.CustomEvent) {
		name, _ := e.Data.(string)
		switch name {
		case "translate":
			windowSvc.ShowTranslateWindow()
		case "settings":
			windowSvc.ShowSettings()
		case "screenshot":
			showScreenshotWindow()
		}
	})

	// Frontend "recapture screenshot" button: hides the window, then reruns the
	// region screenshot→OCR→translate flow.
	app.Event.On(kevents.EventScreenshotRecapture, func(e *application.CustomEvent) {
		if screenshotWindow != nil {
			screenshotWindow.Hide()
		}
		go func() {
			if _, err := trSvc.ScreenshotTranslate(kevents.ScreenshotSessionScreenshot); err != nil {
				slog.Error(i18n.T("log.screenshot_retake_failed"), slog.Any("error", err))
			}
		}()
	})

	// Fired after the frontend changes language: reuses the most recent OCR source text,
	// skipping screenshot/OCR, and retranslates with the new language, pushing incrementally.
	app.Event.On(kevents.EventScreenshotRetranslate, func(e *application.CustomEvent) {
		p, ok := e.Data.(kevents.ScreenshotRetranslatePayload)
		if !ok {
			slog.Error(i18n.T("log.screenshot_retranslate_failed"), slog.String("reason", "invalid payload"))
			return
		}
		go func() {
			if err := trSvc.ScreenshotRetranslate(p.Session, p.From, p.To); err != nil {
				slog.Error(i18n.T("log.screenshot_retranslate_failed"), slog.Any("error", err))
			}
		}()
	})

	// Native title-bar tone for the three persistent windows (settings / translate / screenshot):
	// set from the "in-app theme" at creation, consistent with the updater window
	// (pkg/wails-updater-providers). Wails beta.15 has no public API on either the Go or the
	// frontend runtime for "switch a persistent window's theme at runtime", so this only
	// guarantees "follow the in-app theme at startup"; live runtime following is attempted by
	// the frontend theme store via window.runtime.Window.SetDarkTheme etc. (if the runtime
	// supports it).
	startupDark := resolveUpdaterTheme(settingsService.Get().Theme, app) == "dark"
	macAppearance := application.NSAppearanceNameAqua
	winTheme := application.Light
	if startupDark {
		macAppearance = application.NSAppearanceNameDarkAqua
		winTheme = application.Dark
	}

	// Settings window. Kai is a menu-bar app (ActivationPolicyAccessory): Settings starts hidden
	// and opens only from the gear or the tray (see the Hide below).
	settingsWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   model.WindowSettings,
		Title:  i18n.T("window.settings_title"),
		Width:  1280,
		Height: 800,
		URL:    "/settings.html",
		Mac: application.MacWindow{
			Appearance: macAppearance,
		},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: false,
			Theme:           winTheme,
		},
		// Title bar minimize/maximize/close buttons
		MinimiseButtonState: application.ButtonHidden,
		MaximiseButtonState: application.ButtonHidden,
		CloseButtonState:    application.ButtonEnabled,
	})
	_ = settingsWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		// Issue #69: opening Settings lowers a pinned translate window (service.LowerForSettings);
		// tell it Settings is closing so it re-applies its pin.
		if app != nil {
			app.Event.Emit(kevents.EventWindowClosing, model.WindowSettings)
		}
		settingsWindow.Hide()
	})
	settingsWindow.Center()
	// Start hidden (hand test of #82, 2026-09-26). Shown at startup it sat behind other apps, and
	// closing the translate window made macOS bring it forward although the user never opened it.
	// Created then hidden (not Hidden:true), as for the translate window below, to avoid the
	// Windows WebView2 COM race; windowSvc.ShowSettings shows it with showAndFocus.
	settingsWindow.Hide()

	// Input translate window: two-pane layout (issue #10, locked decision: always side by
	// side) — default/minimum widths enlarged, no longer pinned to 420 (old MaxWidth removed);
	// height is left to the window itself, with each pane scrolling internally.
	translateWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      model.WindowTranslate,
		Title:     i18n.T("window.translate_title"),
		Width:     960,
		Height:    640,
		MinWidth:  780,
		MinHeight: 520,
		URL:       "/translate.html",
		Mac: application.MacWindow{
			Appearance: macAppearance,
		},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: false,
			Theme:           winTheme,
		},
		// Title bar minimize/maximize/close buttons
		MinimiseButtonState: application.ButtonEnabled,
		MaximiseButtonState: application.ButtonHidden,
		CloseButtonState:    application.ButtonEnabled,
	})
	// Don't create with Hidden:true; create then Hide() immediately, avoiding the 80010108 COM
	// race crash on Windows caused by delayed WebView2 controller creation.
	translateWindow.Hide()
	// Red X = hide the window (don't quit): RegisterHook Cancels the close before WindowClosing's
	// destroy listener and hides instead. This keeps the red X, keeps the window alive, and it
	// can be Shown again anytime.
	_ = translateWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		// Red X = hide the window (don't quit): first broadcast kai:window:closing, then Hide.
		// The window is not destroyed (Svelte components stay mounted). Since issue #81 the
		// translate window clears nothing on this broadcast: its text and results are retained
		// (and stored, so they survive a restart too), so the next invocation shows the last
		// translation; only its Clear button empties them.
		if app != nil {
			app.Event.Emit(kevents.EventWindowClosing, model.WindowTranslate)
		}
		// Issue #53: this hook deliberately does NOT reset the language-variant preferences. The
		// window is only hidden here, and the point of the preference is to survive the next
		// hotkey press (principal's ruling 2026-09-25: keep until the app quits).
		translateWindow.Hide()
	})
	translateWindow.Center()

	// Screenshot translate window: image on the left, translation on the right.
	// Summoned by the screenshot hotkey/EventScreenshotOCR.
	screenshotWindow = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      model.WindowScreenshot,
		Title:     i18n.T("window.screenshot_title"),
		Width:     900,
		Height:    600,
		MinWidth:  600,
		MinHeight: 400,
		URL:       "/screenshot.html",
		Mac: application.MacWindow{
			Appearance: macAppearance,
		},
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: false,
			Theme:           winTheme,
		},
		// Title bar minimize/maximize/close buttons
		MinimiseButtonState: application.ButtonEnabled,
		MaximiseButtonState: application.ButtonHidden,
		CloseButtonState:    application.ButtonEnabled,
	})
	// Hide immediately on creation: real initialization completes (build impl + load WebView +
	// set Shadow/AlwaysOnTop) and then it is hidden — equivalent to prebuilding. Later the
	// screenshot path's Show() is a lightweight orderFront and no longer gets stuck in the
	// Hidden state machine.
	screenshotWindow.Hide()
	_ = screenshotWindow.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		event.Cancel()
		// Red X = hide the window (don't quit): first broadcast kai:window:closing so the
		// frontend clears the screenshot and translation, then Hide. The window is not
		// destroyed (Svelte components stay mounted), so the next invocation starts from a
		// clean state.
		if app != nil {
			app.Event.Emit(kevents.EventWindowClosing, model.WindowScreenshot)
		}
		screenshotWindow.Hide()
	})
	registerTray(app, hm, configSvc, windowSvc, settingsService)

	// After a language change, rebuild the tray menu copy with the latest language (the tray is
	// native and can only be rebuilt by the backend). Prefer the language carried in the event
	// payload (mode may be auto, resolved by i18n to a concrete language), avoiding dependence
	// on config persistence ordering that would leave the menu showing the old language.
	app.Event.On(kevents.EventLocaleChanged, func(e *application.CustomEvent) {
		if e != nil && e.Data != nil {
			if p, ok := e.Data.(kevents.LocaleChangedPayload); ok {
				if p.Mode != "" {
					i18n.SetLocale(p.Mode)
				} else if p.Language != "" {
					i18n.SetLocale(p.Language)
				}
				// Sync the updater library's global language (package-level global, read directly by
				// matcher/window) so update-check logs and dialog copy follow language switches.
				kupdater.SetLocale(kupdater.Locale(resolveUpdaterLocale(settingsService.Get().Language)))
				// Refresh the built-in updater window (the library rebuilds it in place to apply
				// the latest language copy).
				kupdater.SetUpdaterLocaleTheme(app)
				// Sync the current UI language to the Swift bridge layer so kai-bridge.log debug
				// logs follow the switch.
				engine.SetBridgeLocale(i18n.GetLocale())
			}
		}
		rebuildTrayMenu(app, hm, configSvc, settingsService)
	})

	// After hotkey enabled-state changes (broadcast after save + re-registration), rebuild the
	// tray menu to show the matching items dynamically.
	app.Event.On(kevents.EventHotkeysChanged, func(e *application.CustomEvent) {
		rebuildTrayMenu(app, hm, configSvc, settingsService)
	})

	// App self-update
	// https://v3.wails.io/guides/updater/
	// The updater reuses the global HTTP client (UA injection, proxy, custom DNS) rather than
	// building its own bare client, so it keeps the global injections and observability.
	// The updater lives in its own package (pkg/wails-updater-providers): slog/client injected.
	// The updater's Locale/Theme/Source do not accept auto (the third-party library removed the
	// auto values), so settings' auto must be resolved to real values: language from the i18n
	// active locale, theme from the real system appearance (IsDarkMode) as light/dark. These are
	// written into the library globals (SetLocale/SetTheme/SetSource), which the library
	// internals (matcher/provider/window) read directly.
	updLocale := resolveUpdaterLocale(settingsService.Get().Language)
	updTheme := resolveUpdaterTheme(settingsService.Get().Theme, app)
	updClient := network.BuildHTTPClient(*settingsService.Get())
	// Library globals: language/theme/primary source/logger/HTTP client (set once; switchable
	// at runtime via SetXxx).
	kupdater.SetLogger(slog.Default())
	kupdater.SetClient(updClient)
	kupdater.SetLocale(kupdater.Locale(updLocale))
	kupdater.SetTheme(kupdater.Theme(updTheme))
	kupdater.SetSource(kupdater.Source(settingsService.Get().Updater.Source))
	updOpts := kupdater.Options{
		CnbRepo:     "dtapp/kai",
		GithubRepo:  "dtapps/kai",
		GithubToken: buildinfo.GithubToken,
		CnbToken:    buildinfo.CnbToken,
		BuildTime:   parseBuildTime(buildinfo.BuildTime),
		GitCommit:   buildinfo.GitCommit,
		Prerelease:  settingsService.Get().Updater.Prerelease,

		// Custom asset matcher: match only the updater- prefixed upgrade archives.
		// The matcher reads the library global language directly; on language switch just call
		// SetLocale and the matcher closure follows automatically.
		AssetMatcher: kupdater.NewUpdaterAssetMatcher()}
	updaterProvider, err = kupdater.NewMirrorProvider(&updOpts)
	if err != nil {
		slog.Error(i18n.T("log.updater_init_failed"), "error", err)
	} else {
		// Built-in updater window (BYO): created and managed by the library
		// (wails-updater-providers) itself; it listens for wails:updater:resize (triggered by
		// the HTML side's ResizeObserver) and calls SetSize for content-adaptive sizing (the
		// framework's Builtin mode uses hardcoded constant sizes that don't track the notes
		// content, and the HTML side cannot call Window.SetSize directly). The handle does not
		// implement WindowSizer, so the framework's transition() won't overwrite our size with
		// hardcoded constants.
		// The window defaults to Hidden and doesn't pop at startup; Close is repurposed as
		// Hide — clicking x doesn't destroy the instance.
		winOpt := kupdater.OpenUpdaterWindow(app)
		if err := app.Updater.Init(updater.Config{
			CurrentVersion: buildinfo.Version,
			Providers:      []updater.Provider{updaterProvider},
			Window:         winOpt,
		}); err != nil {
			slog.Error(i18n.T("log.updater_init_failed"), "error", err)
		} else {
			// Update ready: log it; the user picks "restart to install" from the tray menu.
			// Runtime callback: language follows the library global (already updated on switch),
			// not a construction-time snapshot.
			app.Event.On(updater.EventUpdateReady, func(e *application.CustomEvent) {
				slog.Info(i18n.T("log.updater_ready"))
			})
			// Silent check at startup (skipped on dev builds to avoid noise).
			if !buildinfo.IsDev() {
				checkUpdateOnStart(app, notifySvc)
			}
		}
	}

	// Frontend anonymous analytics event reporting: frontend UI events (settings page opened,
	// toggle flipped) are handed to Go via this event for unified reporting, avoiding loading
	// posthog-js in the frontend webview; frontend and backend share the same anonymous
	// device ID.
	app.Event.On("kai:analytics:track", func(e *application.CustomEvent) {
		m, ok := e.Data.(map[string]any)
		if !ok {
			return
		}
		ev, _ := m["event"].(string)
		if ev == "" {
			return
		}
		props, _ := m["props"].(map[string]any)
		analytics.Track(ev, props)
	})

	// App shutdown: clean up httplog resources (stop periodic cleanup + close the database)
	// and flush analytics reporting.
	app.OnShutdown(func() {
		appSvc.ServiceShutdown()
		analytics.Close()
	})

	// System theme change (Wails3 official events.Common.ThemeChanged, implemented natively
	// per platform). The official docs require listening for system-level events via
	// app.Event.OnApplicationEvent (not a plain Event.On). The callback derives the real
	// appearance from the trustworthy application.Env.IsDarkMode() and forwards it via the
	// unified EventThemeChanged (matchMedia inside the webview is unreliable on macOS).
	// The payload carries both the user-configured mode and the real system appearance theme,
	// so the frontend can handle auto-following with a single event.
	app.Event.OnApplicationEvent(events.Common.ThemeChanged, func(event *application.ApplicationEvent) {
		if configSvc.Theme() == string(model.ThemeAuto) {
			tray.SetIcon(selectTrayIcon(app))
		}
		sysTheme := model.ThemeLight
		if app != nil && app.Env.IsDarkMode() {
			sysTheme = model.ThemeDark
		}
		// Sync the real system appearance change to the updater (auto mode follows the system),
		// refreshing dialog colors.
		if updaterProvider != nil {
			kupdater.SetTheme(kupdater.Theme(resolveUpdaterTheme(configSvc.Theme(), app)))
			// Refresh the built-in updater window colors (auto mode follows system appearance
			// changes).
			kupdater.SetUpdaterLocaleTheme(app)
		}
		app.Event.Emit(kevents.EventThemeChanged, kevents.ThemeChangedPayload{
			Mode:  configSvc.Theme(),
			Theme: string(sysTheme),
		})
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// Package-level window/tray references, so menus can be rebuilt on language change
var (
	tray             *application.SystemTray
	translateWindow  application.Window
	settingsWindow   application.Window
	screenshotWindow application.Window
)

// showScreenshotWindow summons the screenshot translate window to the foreground (the red X
// only hides, never destroys). Same mechanism as ShowTranslateWindow (service.showAndFocus):
// two consecutive Show() calls build the impl + actually show, then Focus().
// The whole sequence runs inside an InvokeAsync main-thread closure, avoiding thread issues
// from calling Wails native window methods directly on a background goroutine.
func showScreenshotWindow() {
	if screenshotWindow == nil {
		return
	}
	application.InvokeAsync(func() {
		screenshotWindow.Show()
		screenshotWindow.Show()
		screenshotWindow.Focus()
	})
}

func registerTray(app *application.App, hm *hotkey.Manager, configSvc *service.ConfigWrapper, windowSvc *service.WindowWrapper, ss *settings.Service) {
	tray = app.SystemTray.New()
	tray.SetIcon(selectTrayIcon(app))
	// Do NOT use AttachWindow: on macOS, activating the app by clicking the tray also restores
	// all windows (settings etc.). Toggle the translate window manually instead, avoiding opening
	// every window at once. The translate window is the app's main surface (issue #69); Settings
	// is reached from the tray's right-click menu or the gear in the translate window. The show
	// branch goes through windowSvc.ShowTranslateWindow (the window is created hidden, see
	// ToggleTranslateWindow), and a tray click captures nothing: no clipboard/selection read,
	// that stays with the input hotkey.
	tray.OnClick(func() {
		service.ToggleTranslateWindow(translateWindow, windowSvc.ShowTranslateWindow)
	})
	buildTrayMenu(app, hm, configSvc, ss)
}

// buildTrayMenu builds the tray menu in the current language (rebuilt on language/hotkey
// enabled-state changes). Menu items appear dynamically based on each configured hotkey's
// "enabled" flag: only enabled items are added to the tray menu.
func buildTrayMenu(app *application.App, hm *hotkey.Manager, configSvc *service.ConfigWrapper, ss *settings.Service) {
	// lang is passed as an empty string so i18n.T falls back to the global locale set by
	// SetLocale (including auto resolution), ensuring the menu rebuilt right after a language
	// broadcast uses the latest language, independent of config persistence ordering.
	trayMenu := app.Menu.New()
	// First item: app name + current version, disabled (not clickable)
	trayMenu.Add(i18n.T("app.name_version", "Version", buildinfo.Version)).SetEnabled(false)
	// Separator
	trayMenu.AddSeparator()
	// Dynamically add menu items based on hotkey enabled state: input translate / screenshot
	// translate. Insert separators between them as needed, always keeping one before "Settings".
	cfg := configSvc.GetConfig()
	inputEnabled := cfg != nil && cfg.Hotkeys.Input.Enabled
	screenshotEnabled := cfg != nil && cfg.Hotkeys.Screenshot.Enabled
	// Always show "Input Translate" and "Screenshot Translate", but make them clickable per
	// enabled state (disabled = greyed out). Hiding them outright would hide the feature entry
	// point; greyed-out is more intuitive: users know the feature exists, it's just not
	// enabled right now.
	trayMenu.Add(i18n.T("menu.input_translate")).SetEnabled(inputEnabled).OnClick(func(ctx *application.Context) {
		// Equivalent to pressing the "input translate" hotkey: takes the copy-key/system
		// text-capture branch and delivers to the input box.
		hm.TriggerInput()
	})
	trayMenu.Add(i18n.T("menu.screenshot_translate")).SetEnabled(screenshotEnabled).OnClick(func(ctx *application.Context) {
		// Equivalent to pressing the "screenshot translate" hotkey: region screenshot→OCR→
		// translate→open the screenshot window.
		hm.TriggerScreenshot()
	})
	// Separator between the translate menu items and "Settings" below
	trayMenu.AddSeparator()
	// Launch-at-login toggle: right above "Settings". Checked state comes from the Wails
	// Autostart library's current state; clicking lets the framework flip checked automatically
	// and the callback calls the library's Enable/Disable (the library manages persistence
	// itself).
	if enabled, err := app.Autostart.IsEnabled(); err == nil {
		trayMenu.AddCheckbox(i18n.T("menu.auto_start"), enabled).OnClick(func(ctx *application.Context) {
			if ctx.IsChecked() {
				if e := app.Autostart.Enable(); e != nil {
					slog.Warn(i18n.T("log.autostart_enable_failed"), slog.String("error", e.Error()))
				}
			} else {
				if e := app.Autostart.Disable(); e != nil {
					slog.Warn(i18n.T("log.autostart_disable_failed"), slog.String("error", e.Error()))
				}
			}
		})
	}
	// Settings, open the settings window
	trayMenu.Add(i18n.T("menu.settings")).OnClick(func(ctx *application.Context) {
		// Same path as the gear (kai:window:show "settings" -> windowSvc.ShowSettings): lowers a
		// pinned translate window (#69) and shows Settings with showAndFocus, which a window
		// hidden since startup needs to appear on the first click.
		app.Event.Emit(kevents.EventWindowShow, "settings")
	})
	// Separator
	trayMenu.AddSeparator()
	// Check for updates: opens the built-in upgrade window (updater_window.html template); the
	// user confirms, it downloads and installs, and applies after the ready restart.
	trayMenu.Add(i18n.T("menu.check_update")).OnClick(func(ctx *application.Context) {
		if app.Updater.State() != updater.StateUnconfigured {
			// In BYO mode the framework doesn't show the window. Wails destroys a closed window
			// and removes it from the registry, so we can't rely on app.Window.Get for its
			// handle to Show; go through the in-package ShowUpdaterWindow uniformly, ensuring
			// the window is alive/rebuilt before showing, so "check for updates again after
			// closing" still pops up.
			kupdater.ShowUpdaterWindow(app)
			_ = app.Updater.CheckAndInstall(context.Background())
		}
	})
	// Prerelease update channel toggle: right next to "Check for updates"; toggling writes back
	// to settings and persists. Initial checked state reads the current config; clicking lets
	// Wails flip checked automatically, and the callback reads the new value and persists it.
	trayMenu.AddCheckbox(i18n.T("menu.prerelease"), ss.Get().Updater.Prerelease).OnClick(func(ctx *application.Context) {
		ss.Get().Updater.Prerelease = ctx.IsChecked()
		if err := ss.Save(); err != nil {
			slog.Error(i18n.T("log.save_prerelease_failed"), slog.String("error", err.Error()))
		}
	})
	// Separator
	trayMenu.AddSeparator()
	trayMenu.Add(i18n.T("menu.quit")).OnClick(func(ctx *application.Context) {
		app.Quit()
	})
	tray.SetMenu(trayMenu)
	tray.SetTooltip(i18n.T("menu.tooltip"))
}

// rebuildTrayMenu rebuilds the tray menu on language/hotkey enabled-state changes (dynamic menu items)
func rebuildTrayMenu(app *application.App, hm *hotkey.Manager, configSvc *service.ConfigWrapper, ss *settings.Service) {
	if translateWindow == nil || settingsWindow == nil {
		return
	}
	buildTrayMenu(app, hm, configSvc, ss)
}

// checkUpdateOnStart checks for updates asynchronously after startup and notifies when one is
// available (mirrors certflow).
func checkUpdateOnStart(app *application.App, notifySvc *service.NotificationService) {
	go func() {
		// Safety net: same as clicking "check for updates", preventing an Updater native-layer
		// panic from taking down the main process.
		defer func() {
			if r := recover(); r != nil {
				slog.Error(i18n.T("log.check_update_panic"), "panic", r)
			}
		}()
		// Random delay between 1 and 3 minutes, mirroring certflow (math/rand is sufficient).
		minDuration := 1 * time.Minute
		maxDuration := 3 * time.Minute
		randomDuration := minDuration + time.Duration(rand.Intn(int(maxDuration-minDuration)))
		time.Sleep(randomDuration)
		rel, err := app.Updater.Check(context.Background())
		if err != nil {
			slog.Warn(i18n.T("log.check_update_failed"), "error", err)
			return
		}
		if rel == nil {
			return // No update
		}
		// Send a native desktop notification (Wails notifications service, not forwarded via
		// the frontend). Permission checks and fallback logic are consolidated in
		// service.NotificationService; callers only care about what to send.
		notifySvc.Notify(notifications.NotificationOptions{
			ID:       "kai-update-available",
			Title:    i18n.T("notification.update_available_title"),
			Subtitle: i18n.T("notification.update_available_subtitle", "version", rel.Version),
		})
	}()
}

// selectTrayIcon picks the tray icon based on the system dark state. Uses the Wails3
// Environment API to probe the current appearance — reliable cross-platform. A dark icon
// branch is reserved: add build/darwin/trayicon_dark.png and embed it below to follow the
// system dark mode automatically.
func selectTrayIcon(app *application.App) []byte {
	// Reserved dark icon branch: add build/darwin/trayicon_dark.png and embed it above to
	// follow the system dark mode.
	// if app != nil && app.Env.IsDarkMode() && len(trayIconDark) > 0 {
	// 	return trayIconDark
	// }
	return trayIcon
}

// initLogging points the default slog logger at dataDir/logs/kai.log (day-rotated) while
// also fanning out to stderr for easy terminal viewing during development. The log level is
// controlled entirely by settings.json's log.level (applied by applyLogConfig after startup;
// no dev/release-specific overrides).
// Returns *logutil.Rotator so settings hot-updates can adjust level / cleanup policy dynamically.
func initLogging(homeDir string, logCfg settings.LogConfig) *logutil.Rotator {
	logLevel := logutil.ParseLevel(logCfg.Level)
	logDir := buildinfo.LogDir(homeDir)
	rotator, err := logutil.NewRotator(logDir, logLevel, logCfg.RetentionDays, logCfg.Compress)
	if err != nil {
		// Directory/file unusable: degrade to stderr-only so the app still starts and records
		// critical logs.
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel})))
		log.Printf(i18n.T("log.rotate_log_init_failed"), err)
		return rotator
	}
	slog.SetDefault(slog.New(rotator.Handler()))
	log.Printf(i18n.T("log.log_initialized"), filepath.Join(logDir, "kai.log"))
	return rotator
}

// applyLogConfig dynamically adjusts the runtime log level and cleanup policy from
// settings.LogConfig. The level is controlled entirely by the settings file (settings.json's
// log.level); no dev/release-specific overrides. dataDir is used to sync the same config
// (directory + level/retention days/compression) to the Swift bridge layer, so kai-bridge.log
// and frontend.log follow the same policy as the main app log kai.log (level filtering, day
// rotation, cleanup, compression).
func applyLogConfig(r *logutil.Rotator, fl *logutil.FrontendLogService, cfg settings.LogConfig, homeDir string) {
	if r == nil {
		return
	}
	level := logutil.ParseLevel(cfg.Level)
	r.SetLevel(level)
	r.UpdateRetention(cfg.RetentionDays, cfg.Compress)
	if fl != nil {
		fl.SetLevel(level)
	}
	// Sync to the Swift bridge layer (kai-bridge.log); the level likewise comes from the
	// settings file.
	engine.SetLogConfig(buildinfo.LogDir(homeDir), cfg.Level, cfg.RetentionDays, cfg.Compress)
	// Sync the current UI language so the bridge layer's debug logs switch between
	// Chinese/English with the system language.
	engine.SetBridgeLocale(i18n.GetLocale())
	slog.Info(i18n.T("log.log_config_applied"), "level", level.String(), "retention_days", cfg.RetentionDays, "compress", cfg.Compress)
}
