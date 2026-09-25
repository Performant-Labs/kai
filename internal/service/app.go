package service

import (
	"context"
	"log/slog"

	"cnb.cool/dtapp/kai/internal/analytics"
	"cnb.cool/dtapp/kai/internal/buildinfo"
	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/execkey"
	"cnb.cool/dtapp/kai/internal/historystore"
	"cnb.cool/dtapp/kai/internal/hotkey"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/settings"
	"cnb.cool/dtapp/kai/internal/translate"
	"cnb.cool/dtapp/kai/internal/useragent"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// version is the app version (injected at build time).
var version = "dev"

// AppService is the app's core facade (thin Wrapper):
//   - The only implementer of the wails lifecycle trio (ServiceStartup / ServiceShutdown /
//     ServiceName), and the single entry point for the global startup orchestration (engine
//     loading, hotkey registration, language/accessibility initialization).
//   - Holds references to the domain services and Wrappers — the hub for dependency
//     injection.
//   - Only exposes core RPCs (version, accessibility, startup orchestration);
//     translation/history/config/window RPCs belong to the individual Wrappers.
type AppService struct {
	app          *application.App
	settingsSvc  *settings.Service
	translateSvc *translate.Service
	execKeyCtrl  *execkey.ExecKeyController
	hotkeyMgr    *hotkey.Manager
	engineSvc    *EngineWrapper
	historySvc   *HistoryWrapper
	configSvc    *ConfigWrapper
	windowSvc    *WindowWrapper
	registry     *engine.Registry
	log          *slog.Logger
}

// NewAppService constructs the app's core facade. All dependencies are injected explicitly,
// eliminating the shared container.
func NewAppService(
	st *settings.Service,
	tr *translate.Service,
	ek *execkey.ExecKeyController,
	hm *hotkey.Manager,
	reg *engine.Registry,
	histDB *historystore.Store,
	cfgStore *configstore.Store,
	app *application.App,
) *AppService {
	windowSvc := NewWindowWrapper(app)
	configSvc := NewConfigWrapper(st, app, hm)
	historySvc := NewHistoryWrapper(histDB, cfgStore)
	engineSvc := NewEngineWrapper(reg, cfgStore, st, app, hm)
	return &AppService{
		app:          app,
		settingsSvc:  st,
		translateSvc: tr,
		execKeyCtrl:  ek,
		hotkeyMgr:    hm,
		engineSvc:    engineSvc,
		historySvc:   historySvc,
		configSvc:    configSvc,
		windowSvc:    windowSvc,
		registry:     reg,
		log:          slog.Default(),
	}
}

// SetApp injects the app once it is ready (startup orchestration phase).
func (s *AppService) SetApp(app *application.App) {
	s.app = app
}

// SetUserAgent is called by the frontend at window startup, passing the WebView's
// navigator.userAgent to the backend as the default User-Agent for global HTTP requests
// (injected into each engine's http.Client via the useragent package).
func (s *AppService) SetUserAgent(ua string) {
	useragent.Set(ua)
}

// GetVersion returns the app version.
func (s *AppService) GetVersion() string {
	return version
}

// CheckAccessibility checks whether macOS accessibility is authorized (cross-platform:
// non-darwin returns true directly).
func (s *AppService) CheckAccessibility() bool {
	return s.isAccessibilityEnabled()
}

// OpenAccessibilitySettings opens the system accessibility settings pane (darwin only).
func (s *AppService) OpenAccessibilitySettings() {
	s.openAccessibilitySettings()
}

// CheckScreenRecording checks whether macOS screen recording is authorized (needed by
// screenshot translate; cross-platform: non-darwin returns true directly).
func (s *AppService) CheckScreenRecording() bool {
	return s.isScreenRecordingEnabled()
}

// OpenScreenRecordingSettings pops the system "Screen Recording" permission dialog (darwin
// only).
func (s *AppService) OpenScreenRecordingSettings() {
	s.openScreenRecordingSettings()
}

// TODO: input-monitoring related (CheckInputMonitoring / OpenInputMonitoringSettings exported
// methods) currently unused, commented out.
// // CheckInputMonitoring checks whether macOS input monitoring is authorized (cross-platform:
// non-darwin returns true directly).
// func (s *AppService) CheckInputMonitoring() bool {
// 	return s.isInputMonitoringEnabled()
// }
//
// // OpenInputMonitoringSettings opens the system input-monitoring settings pane (darwin only).
// func (s *AppService) OpenInputMonitoringSettings() {
// 	s.openInputMonitoringSettings()
// }

// ServiceName returns the service name (part of the wails lifecycle trio).
func (s *AppService) ServiceName() string {
	return "AppService"
}

// ServiceStartup is the single entry point for app startup orchestration (part of the wails
// lifecycle trio). app has already been injected by main.go via SetApp after
// application.New; it is no longer taken from ctx here.
func (s *AppService) ServiceStartup(_ context.Context, _ application.ServiceOptions) error {
	// Inject app into every domain and Wrapper that depends on it
	s.translateSvc.SetApp(s.app)
	s.execKeyCtrl.SetApp(s.app)
	s.configSvc.SetApp(s.app)
	s.engineSvc.SetApp(s.app)
	s.hotkeyMgr.SetApp(s.app)

	// Load engine config from config.db and register into the registry
	if err := s.engineSvc.loadEngines(); err != nil {
		s.log.Error(i18n.T("log.service_load_engine_config_failed"), slog.Any(i18n.T("log.field_error"), err))
	}

	// Register global hotkeys (key registration logic lives in HotkeyManager)
	s.hotkeyMgr.Register()

	// Initialize language
	i18n.SetLocale(s.settingsSvc.Get().Language)

	// Report anonymous usage stats right at startup (first launch additionally sends
	// app_installed; device-level attributes are inherited via Identify).
	// Dev builds / unconfigured key / user switch off → analytics internally no-ops.
	if s.settingsSvc != nil && s.settingsSvc.Get() != nil {
		analytics.AppStarted(
			buildinfo.Version,
			s.settingsSvc.Get().Language,
			"release", // channel: uniformly release (prod) for now; extend here if distribution channels need distinguishing later
			analytics.IsFirstLaunch(),
		)
	}

	// Accessibility permission hint (on darwin, without permission the copy key / simulated
	// keystrokes won't work)
	if !s.isAccessibilityEnabled() {
		s.log.Warn(i18n.T("log.service_accessibility_unauthorized"))
	}
	return nil
}

// ServiceShutdown cleans up on app shutdown (part of the wails lifecycle trio).
func (s *AppService) ServiceShutdown() error {
	if s.hotkeyMgr != nil {
		s.hotkeyMgr.Unregister()
	}
	return nil
}

// emitHotkeysChanged broadcasts the currently active hotkey list to the frontend for live
// display.
func (s *AppService) emitHotkeysChanged(active []string) {
	if s.app == nil {
		return
	}
	s.app.Event.Emit(events.EventHotkeysChanged, active)
}

// EmitHotkeysChanged is for outer callers (the hotkey manager) to broadcast the currently
// active hotkey list.
func (s *AppService) EmitHotkeysChanged(active []string) {
	s.emitHotkeysChanged(active)
}

// TranslateMulti batch-translates (delegates to translate.Service, exposed to the frontend as
// AppService.TranslateMulti).
func (s *AppService) TranslateMulti(req model.TranslateRequest) (*model.TranslateMultiResult, error) {
	return s.translateSvc.TranslateMulti(req)
}

// ScreenshotOCR is screenshot OCR (no args, uses the default OCR engine, delegates to
// translate.Service).
func (s *AppService) ScreenshotOCR() (*model.OcrResult, error) {
	return s.translateSvc.ScreenshotOCR(s.registry.DefaultOCREngineName())
}
