// Package analytics provides unified anonymous usage-stats reporting (based on PostHog).
//
// Design points:
//   - Single reporting outlet: backend events (translation/OCR/updates etc.) call Track
//     directly on the Go side; pure frontend UI events (settings page opened, toggles
//     flipped) are forwarded to Track by the frontend via the Wails event
//     kai:analytics:track — so posthog-js never loads in the frontend webview, and frontend
//     and backend share the same anonymous device ID.
//   - Gating: reporting happens only when buildinfo.PosthogToken is non-empty + a non-dev
//     build (or explicit EnableDevUpload) + the user switch is on
//     (settings.analytics_enabled).
//   - Anonymous: the device ID comes from machineid.ProtectedID("kai") (an HMAC-by-app-name
//     machine fingerprint — anonymous and stable); on failure nothing is reported. No user
//     information is collected.
//   - Privacy: switching off immediately closes the client and stops network activity.
package analytics

import (
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"sync/atomic"

	"cnb.cool/dtapp/kai/internal/buildinfo"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/settings"
	"github.com/denisbrodbeck/machineid"
	"github.com/posthog/posthog-go"
)

// The PostHog token is injected uniformly via buildinfo.PosthogToken through -ldflags; no
// environment variables are read anymore.

// endpoint is the PostHog reporting address (US cloud default; for self-hosting, point it
// at your own instance).
const endpoint = "https://us.i.posthog.com"

// appName is the app identifier carried with reports (fixed to kai; distinguishes
// apps/builds within the same PostHog instance).
const appName = "kai"

// Event name constants (aligned with the product analytics plan).
const (
	EventAppStarted          = "app_started"
	EventAppInstalled        = "app_installed"
	EventTranslateInput      = "translate_input"
	EventTranslateScreenshot = "translate_screenshot"
	EventSettingsOpened      = "settings_opened"
	EventFeatureToggled      = "feature_toggled"
	EventEngineConfigured    = "engine_configured"
	EventUpdateInstalled     = "update_installed"
	EventError               = "error_occurred"
)

var (
	mu        sync.RWMutex
	client    posthog.Client
	hasClient bool
	deviceID  string
	deviceOK  bool
	ss        *settings.Service
)

// devUpload is a local debugging switch allowing dev builds (Dev=true) to also report;
// default false.
// Release builds (Dev=false) are unaffected — always decided by token + user switch.
var devUpload atomic.Bool

// EnableDevUpload turns on reporting for dev builds, for locally debugging the pipeline
// only.
// After calling it, IsDev() no longer blocks — behavior equals a release build. Never call
// this in production.
func EnableDevUpload() {
	devUpload.Store(true)
}

// devModeBlocks reports whether we're currently in the "reporting forbidden" dev state: a
// dev build with dev upload not explicitly enabled.
// All reporting gates go through this method, making future dev-testing policy changes
// easy.
func devModeBlocks() bool {
	return buildinfo.IsDev() && !devUpload.Load()
}

// Init lazily initializes: fetches the anonymous device ID (machineid) and records the
// settings service.
// Must be called after settings.NewService and before app.Run.
// If machineid is unavailable, deviceOK=false and later reporting is blocked by Enabled (a
// failure means no reporting).
func Init(dataDir string, svc *settings.Service) {
	mu.Lock()
	defer mu.Unlock()
	ss = svc
	id, err := machineid.ProtectedID("kai")
	if err != nil {
		slog.Warn(i18n.T("log.analytics_device_id_failed"), "error", err)
		deviceID = ""
		deviceOK = false
		return
	}
	deviceID = id
	deviceOK = true
}

// Enabled reports whether reporting is currently allowed: token non-empty + machineid
// available + non-dev build + the user switch on.
func Enabled() bool {
	if buildinfo.PosthogToken == "" {
		return false
	}
	if !deviceOK {
		return false
	}
	if devModeBlocks() {
		return false
	}
	mu.RLock()
	defer mu.RUnlock()
	return ss != nil && ss.Get() != nil && ss.Get().AnalyticsEnabled
}

// analyticsState returns whether we report and why (an i18n key), for the startup log (not
// using Enabled() directly so that, when off, we can also say "why it's off": no token /
// device ID unavailable / dev build / user switch off).
// Returns an i18n key, translated by the caller via i18n.T, avoiding hardcoded copy.
func analyticsState() (enabled bool, reasonKey string) {
	if buildinfo.PosthogToken == "" {
		return false, "log.analytics_reason_no_token"
	}
	if !deviceOK {
		return false, "log.analytics_reason_no_device"
	}
	if devModeBlocks() {
		return false, "log.analytics_reason_dev"
	}
	mu.RLock()
	on := ss != nil && ss.Get() != nil && ss.Get().AnalyticsEnabled
	mu.RUnlock()
	if !on {
		return false, "log.analytics_reason_user_off"
	}
	return true, "log.analytics_reason_on"
}

// IsFirstLaunch reports whether app_installed hasn't been sent yet (the first-install event
// fires once).
// Based on the AnalyticsInstalled flag persisted in settings; Init must have run first.
func IsFirstLaunch() bool {
	mu.RLock()
	svc := ss
	mu.RUnlock()
	return svc != nil && svc.Get() != nil && !svc.Get().AnalyticsInstalled
}

// ensureClient builds the client on demand (only when the key is non-empty and the build
// is non-dev). Returns immediately if already built.
func ensureClient() posthog.Client {
	mu.RLock()
	if hasClient {
		c := client
		mu.RUnlock()
		return c
	}
	mu.RUnlock()

	key := buildinfo.PosthogToken
	if key == "" || devModeBlocks() {
		return nil
	}
	// Key: we must pass our own Transport. posthog-go's makeHttpClient only asserts
	// http.DefaultTransport.(*http.Transport) when transport==nil, and this project has
	// replaced DefaultTransport with *useragent.Transport (UA/logging wrappers), so that
	// assertion would panic. Passing a non-nil Transport (the project's already-wrapped
	// DefaultTransport) keeps it from touching the default transport — and reporting
	// requests then carry the global UA and land in the HTTP log too.
	//
	// DisableGeoIP: the SDK's default (nil) disables GeoIP attribution (GetDisableGeoIP
	// returns true on nil, auto-attaching $geoip_disable to every event).
	// Kai is a desktop client, so it explicitly sets Ptr(false) to let PostHog do coarse
	// geographic attribution by request IP (country/region/city, no precise location).
	// IsServer: the default (nil) marks events as server-side, and PostHog won't use the
	// request IP for GeoIP attribution on server events; Kai actually runs on user machines,
	// so it should report as a client with Ptr(false), otherwise $is_server is omitted and
	// device-OS attribution misfires.
	c, err := posthog.NewWithConfig(key, posthog.Config{
		Endpoint:               endpoint,
		Transport:              http.DefaultTransport,
		DisableGeoIP:           new(false),
		IsServer:               new(false),
		DefaultEventProperties: posthog.NewProperties().Set("app_name", appName),
	})
	if err != nil {
		slog.Warn(i18n.T("log.analytics_init_client_failed"), "error", err)
		return nil
	}
	mu.Lock()
	client = c
	hasClient = true
	mu.Unlock()
	return c
}

// Track reports a custom event. Switch/build-mode gating is internal — callers need not
// check.
func Track(event string, props map[string]any) {
	if !Enabled() {
		return
	}
	c := ensureClient()
	if c == nil {
		return
	}
	p := posthog.NewProperties()
	for k, v := range props {
		p.Set(k, v)
	}
	if err := c.Enqueue(posthog.Capture{DistinctId: deviceID, Event: event, Properties: p}); err != nil {
		slog.Warn(i18n.T("log.analytics_track_failed"), "event", event, "error", err)
	}
}

// Identify sets device-level person properties (version/OS/language etc.) so later events
// inherit them automatically.
func Identify(props map[string]any) {
	if !Enabled() {
		return
	}
	c := ensureClient()
	if c == nil {
		return
	}
	p := posthog.NewProperties()
	for k, v := range props {
		p.Set(k, v)
	}
	// app_name is a fixed identifier, kept as a person property to distinguish apps/builds
	// within the same PostHog instance.
	p.Set("app_name", appName)
	// project_id comes from buildinfo injection; a person property for grouping across
	// projects in the same PostHog instance.
	if buildinfo.PosthogProjectID != "" {
		p.Set("project_id", buildinfo.PosthogProjectID)
	}
	if err := c.Enqueue(posthog.Identify{DistinctId: deviceID, Properties: p}); err != nil {
		slog.Warn(i18n.T("log.analytics_identify_failed"), "error", err)
	}
}

// Error conveniently reports an error_occurred event.
func Error(kind string, props map[string]any) {
	m := map[string]any{"kind": kind}
	maps.Copy(m, props)
	Track(EventError, m)
}

// AppStarted reports at startup: first launches additionally send app_installed, then
// Identify device properties + send app_started.
func AppStarted(appVersion, uiLang, channel string, isFirstLaunch bool) {
	// Startup log: print the anonymous analytics switch state (easy confirmation from logs
	// of whether reporting is happening).
	on, reasonKey := analyticsState()
	slog.Info(i18n.T("log.analytics_state"), "enabled", on, "reason", i18n.T(reasonKey))

	if isFirstLaunch && Enabled() {
		Track(EventAppInstalled, map[string]any{"app_version": appVersion, "channel": channel})
		// Mark the first-install event as sent to avoid repeats (persisted to settings).
		mu.RLock()
		svc := ss
		mu.RUnlock()
		if svc != nil && svc.Get() != nil {
			svc.Get().AnalyticsInstalled = true
			_ = svc.Save()
		}
	}
	Identify(map[string]any{
		"app_version": appVersion,
		"ui_lang":     uiLang,
		"channel":     channel,
	})
	Track(EventAppStarted, map[string]any{
		"app_version": appVersion,
		"ui_lang":     uiLang,
		"channel":     channel,
	})
}

// SetEnabled is called by the frontend switch: writes back to settings and persists.
func SetEnabled(enabled bool) error {
	mu.Lock()
	svc := ss
	mu.Unlock()
	if svc == nil {
		return nil
	}
	svc.Get().AnalyticsEnabled = enabled
	if err := svc.Save(); err != nil {
		return err
	}
	if enabled {
		_ = ensureClient()
	} else {
		closeClient()
	}
	return nil
}

// Close is called at app shutdown: flushes and closes the client (stops networking).
func Close() {
	closeClient()
}

func closeClient() {
	mu.Lock()
	defer mu.Unlock()
	if hasClient && client != nil {
		_ = client.Close()
		hasClient = false
		client = nil
	}
}

// LenBucket buckets text length, avoiding high-cardinality properties.
func LenBucket(n int) string {
	switch {
	case n <= 0:
		return "0"
	case n <= 20:
		return "1-20"
	case n <= 100:
		return "21-100"
	case n <= 500:
		return "101-500"
	case n <= 2000:
		return "501-2000"
	default:
		return "2000+"
	}
}
