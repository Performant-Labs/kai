package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/hotkey"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/network"
	"cnb.cool/dtapp/kai/internal/settings"
)

var (
	errEngineStoreNotReady = errors.New(i18n.T("err.service_engine_store_not_ready"))
	log                    = slog.Default()
)

// builtinEngine marks engines that are "system built-in, config-free" (persisted, with real
// IDs). Semantics: cannot be deleted, but the switch can be toggled (participates in OCR
// single-select / translation default).
var builtinEngine = map[string]bool{
	"apple":  true, // macOS system translation
	"vision": true, // macOS system OCR (Vision.framework)
}

// EngineItem is an engine entry returned to the frontend (with enabled state and display
// name).
type EngineItem struct {
	ID      int64  `json:"id"`      // Engine auto-increment primary key
	Name    string `json:"name"`    // Engine display name
	Enabled bool   `json:"enabled"` // Whether enabled
}

// EngineWrapper is the thin adapter over engine config: it holds registry + configstore +
// settings + hotkey and is responsible for runtime registration/persistence of engine
// enable/disable and for serving schemas. It only exposes RPCs — none of the wails
// lifecycle trio.
type EngineWrapper struct {
	registry         *engine.Registry
	configStore      *configstore.Store
	settingsSvc      *settings.Service
	settingsProvider func() settings.Settings
	app              *application.App
	hotkeyMgr        *hotkey.Manager
}

// NewEngineWrapper constructs the engine Wrapper. app and hotkeyMgr may be injected after
// the startup orchestration.
func NewEngineWrapper(
	reg *engine.Registry,
	store *configstore.Store,
	st *settings.Service,
	app *application.App,
	hm *hotkey.Manager,
) *EngineWrapper {
	return &EngineWrapper{
		registry:         reg,
		configStore:      store,
		settingsSvc:      st,
		settingsProvider: func() settings.Settings { return *st.Get() },
		app:              app,
		hotkeyMgr:        hm,
	}
}

// SetApp injects the app once it is ready.
func (w *EngineWrapper) SetApp(app *application.App) {
	w.app = app
}

// registerEngines registers the enabled translate/OCR engines per config.
// Engine config is read from config.db (an independent data source, never mixed into
// settings); settingsProvider is only used to build the HTTP client with custom DNS + proxy.
func (w *EngineWrapper) registerEngines() {
	cfg := w.settingsProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := w.configStore.LoadEngines(ctx)
	if err != nil {
		slog.Default().Error(i18n.T("log.service_load_engine_config_failed"), slog.Any("error", err))
		return
	}
	engines := engine.EngineMap(configstore.EnginesToConfig(rows))

	// Custom HTTP client (custom DNS + proxy)
	newClient := func(timeout time.Duration) *http.Client {
		c := network.BuildHTTPClient(cfg)
		c.Timeout = timeout
		return c
	}

	// apple: macOS system translation (Translation.framework), key-free, no network client.
	if e, ok := engines["apple"]; ok && e.Enabled {
		w.registry.RegisterTranslator(engine.NewApple())
	}
	if e, ok := engines["google"]; ok && e.Enabled {
		w.registry.RegisterTranslator(engine.NewGoogle(e.Endpoint, newClient(15*time.Second)))
	}
	if e, ok := engines["deepl"]; ok && e.Enabled {
		w.registry.RegisterTranslator(engine.NewDeepL(e, newClient(15*time.Second)))
	}
	if e, ok := engines["openai"]; ok && e.Enabled {
		w.registry.RegisterTranslator(engine.NewOpenAI(e, newClient(30*time.Second)))
	}
	if e, ok := engines["anthropic"]; ok && e.Enabled {
		// Inject the global HTTP client (custom DNS/proxy/logging), consistent with
		// openai/gemini.
		e.HTTPClient = newClient(60 * time.Second)
		w.registry.RegisterTranslator(engine.NewAnthropic(e))
	}
	if e, ok := engines["gemini"]; ok && e.Enabled {
		// Inject the global HTTP client (custom DNS/proxy/logging) so the Gemini SDK never
		// touches the useragent-wrapped global http.DefaultTransport and panics.
		e.HTTPClient = newClient(60 * time.Second)
		if g, err := engine.NewGemini(e); err == nil {
			w.registry.RegisterTranslator(g)
		}
	}
	if e, ok := engines["baidu"]; ok && e.Enabled {
		w.registry.RegisterTranslator(engine.NewBaidu(e, newClient(15*time.Second)))
	}
	if e, ok := engines["tencent"]; ok && e.Enabled {
		w.registry.RegisterTranslator(engine.NewTencent(e, newClient(15*time.Second)))
	}
	if e, ok := engines["youdao"]; ok && e.Enabled {
		w.registry.RegisterTranslator(engine.NewYoudao(e, newClient(15*time.Second)))
	}
	// OCR single-select: only one OCR engine may be registered into the Registry at a time.
	// Prefer the user-enabled tesseract from the settings page; otherwise register vision
	// (system OCR) if it is enabled in the store.
	// Both vision / tesseract are persisted (engine table); the engines map's Enabled decides
	// which one registers.
	if e, ok := engines["tesseract"]; ok && e.Enabled {
		w.registry.RegisterOcr(engine.NewTesseractOCR(e))
	} else if e, ok := engines["vision"]; ok && e.Enabled {
		w.registry.RegisterOcr(engine.NewVisionOCR(e))
	}
}

// reregisterHotkeys re-registers the global hotkeys (called after engine config changes).
func (w *EngineWrapper) reregisterHotkeys() {
	if w.hotkeyMgr == nil {
		return
	}
	w.hotkeyMgr.Register()
}

// GetEngines returns all registered engines (translation + OCR) as (value, type name, type)
// entries, ordered strictly by the auto-increment id of config.db's engines table (not the
// code registration order).
func (w *EngineWrapper) GetEngines() []EngineListItem {
	metas := w.registry.AllEngines()
	metaMap := make(map[string]engine.EngineMeta, len(metas))
	for _, m := range metas {
		metaMap[m.Name] = m
	}
	ctx := context.Background()
	dbEngs, err := w.configStore.LoadEngines(ctx)
	if err != nil {
		log.Error(i18n.T("log.service_load_engine_list_fallback"), slog.Any("error", err))
		return registryEngineListItems(metas)
	}
	items := make([]EngineListItem, 0, len(dbEngs))
	for _, e := range dbEngs {
		m, ok := metaMap[e.Engine]
		if !ok {
			continue
		}
		items = append(items, EngineListItem{
			Value:     m.Name,
			Name:      m.Name,
			Kind:      string(m.Kind),
			Supported: m.Supported,
		})
	}
	return items
}

// PrimaryTranslateEngine returns the "primary translation engine" name the translate window
// should bind on first paint. This is the authoritative implementation of the primary-engine
// resolution rules (the backend's single source of truth; the frontend resolvePrimaryEngine
// is its mirror, kept consistent by the divergence check in test (d)).
// Resolution order: settings.default_engine valid (kind=translate, enabled, platform
// supported) -> it; otherwise fall back to the first enabled translation engine
// (configstore id order, same as GetEngines); none -> "".
// "Invalid / disabled / unknown" all fall back silently — no error.
// Note: "enabled" comes from configstore's enabled column (live), not the registry's
// registration state — a disabled engine is not immediately unregistered from the registry,
// and GetEngines()'s Supported only expresses platform support, which can't serve as the
// enabled basis; hence the direct configStore.LoadEngines query here.
func (w *EngineWrapper) PrimaryTranslateEngine() string {
	return w.activeTranslateEngine("")
}

// ActiveTranslateEngine returns the "current engine" name bound in the translate window's
// result pane (issue #9, design §1): one more layer than PrimaryTranslateEngine — last-used.
// lastUsed valid (kind=translate, enabled column=1, platform supported) -> it;
// otherwise settings.default_engine valid -> it;
// otherwise the first enabled translation engine (configstore id order);
// none -> "".
// lastUsed is the previously used engine persisted in the frontend's localStorage
// (kai:translate:lastEngine); an empty string is equivalent to PrimaryTranslateEngine (the
// degenerate case).
// Resolution shares activeTranslateEngine with PrimaryTranslateEngine (no duplicate
// enabled/translate/supported filtering); "enabled" likewise comes from configstore's
// enabled column.
func (w *EngineWrapper) ActiveTranslateEngine(lastUsed string) string {
	return w.activeTranslateEngine(lastUsed)
}

// activeTranslateEngine is the shared resolution core of #8 (default_engine ?? first
// enabled) and #9 (the + last-used layer): candidate names are tried layer by layer in
// lastUsed, defaultEngine order; when none hit, it falls back to the first enabled translate
// engine (configstore id order); none -> "".
// "Invalid / disabled / unknown" all fall back silently — no error.
func (w *EngineWrapper) activeTranslateEngine(lastUsed string) string {
	metas := w.registry.AllEngines()
	metaMap := make(map[string]engine.EngineMeta, len(metas))
	for _, m := range metas {
		metaMap[m.Name] = m
	}
	ctx := context.Background()
	dbEngs, err := w.configStore.LoadEngines(ctx)
	if err != nil {
		log.Error(i18n.T("log.service_load_engine_list_fallback"), slog.Any("error", err))
		return ""
	}
	for _, name := range []string{lastUsed, w.settingsProvider().DefaultEngine} {
		if name == "" {
			continue
		}
		for _, e := range dbEngs {
			if e.Enabled == 0 {
				continue
			}
			m, ok := metaMap[e.Engine]
			if !ok || m.Kind != engine.KindTranslator || !m.Supported {
				continue
			}
			if e.Engine == name {
				return e.Engine
			}
		}
	}
	for _, e := range dbEngs {
		if e.Enabled == 0 {
			continue
		}
		m, ok := metaMap[e.Engine]
		if !ok {
			continue
		}
		if m.Kind == engine.KindTranslator && m.Supported {
			return e.Engine
		}
	}
	return ""
}

// registryEngineListItems builds frontend engine entries in raw registry order (only a
// failure fallback).
func registryEngineListItems(metas []engine.EngineMeta) []EngineListItem {
	items := make([]EngineListItem, 0, len(metas))
	for _, m := range metas {
		items = append(items, EngineListItem{
			Value:     m.Name,
			Name:      m.Name,
			Kind:      string(m.Kind),
			Supported: m.Supported,
		})
	}
	return items
}

// GetAllEngines returns "all" engines from config.db's engines table with their enabled
// state.
// List data comes straight from the database (configStore.LoadEngines) — never through a
// settings copy and never merged with the code's KnownEngines — engine config is config.db's
// independent data source, unrelated to the UI-preference settings.
// Add/enable/delete all go through the form and their respective DB operations.
// kind comes from the KnownEngines catalog (engine kind is registered in code, not stored in
// the database).
func (w *EngineWrapper) GetAllEngines() []AllEngineItem {
	kindMap := make(map[string]string, len(engine.KnownEngines()))
	for _, m := range engine.KnownEngines() {
		kindMap[m.Name] = string(m.Kind)
	}
	ctx := context.Background()
	engs, err := w.configStore.LoadEngines(ctx)
	if err != nil {
		slog.Default().Error(i18n.T("log.service_load_engine_list_failed"), slog.Any("error", err))
		return nil
	}
	// Note: the "empty table → reset default engines" fallback is deliberately NOT done here.
	// That fallback runs exactly once in the startup orchestration entry loadEngines(),
	// avoiding wrongly resetting engines the user disabled (or that failed validation) back
	// to enabled during list refreshes / toggle round-trips.
	items := make([]AllEngineItem, 0, len(engs))
	for _, e := range engs {
		kind := kindMap[e.Engine]
		if kind == "" {
			kind = string(engine.KindTranslator)
		}
		// System built-in engines (vision system OCR / apple system translation) are persisted;
		// mark them builtin uniformly here: cannot be deleted, but the switch can be toggled
		// (participates in OCR single-select / translation default).
		items = append(items, AllEngineItem{
			ID:        e.ID,
			Value:     e.Engine,
			Name:      e.Engine,
			Kind:      kind,
			Enabled:   e.Enabled != 0,
			Supported: engine.EngineSupported(e.Engine),
			Builtin:   builtinEngine[e.Engine],
		})
	}
	return items
}

// GetKnownEngines returns the catalog of "addable" engines (from the code's KnownEngines),
// for the settings page's add-form "engine type" dropdown. The list only renders existing DB
// entries; the dropdown options must come from the code catalog so names are recognizable by
// the Registry and translation actually works.
func (w *EngineWrapper) GetKnownEngines() []EngineListItem {
	metas := engine.KnownEngines()
	items := make([]EngineListItem, 0, len(metas))
	for _, m := range metas {
		items = append(items, EngineListItem{
			Value:     m.Name,
			Name:      m.Name,
			Kind:      string(m.Kind),
			Supported: m.Supported,
		})
	}
	return items
}

// GetEngineSchema returns the "real" config-field schema of the given engine, for the
// frontend to render the right-hand form dynamically.
func (w *EngineWrapper) GetEngineSchema(engineName string) engine.EngineSchema {
	return engine.GetEngineSchema(engineName)
}

// GetOcrLangs returns the OCR-engine (vision / tesseract) language-code options, for the
// frontend's OCR-specific UI to render the langs multi-select. Decoupled from Extra(JSON),
// maintained solely in code.
func (w *EngineWrapper) GetOcrLangs() []string {
	return engine.OcrLangsOptions()
}

// CheckTesseract probes whether tesseract is installed locally, returning install state and
// executable path, for the settings page to show the Tesseract engine's
// "installed/not installed" hint.
func (w *EngineWrapper) CheckTesseract() engine.TesseractStatus {
	return engine.TesseractInstalled()
}

// GetEngineConfig returns one engine's full config by ID (including persisted
// api_key/endpoint etc.), for the settings page's right-hand form to backfill stored values.
// Returns nil when not found.
func (w *EngineWrapper) GetEngineConfig(id int64) *engine.EngineConfig {
	if w.configStore == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cfg, err := w.configStore.GetEngineByID(ctx, id)
	if err != nil {
		slog.Default().Error(i18n.T("log.service_read_engine_config_failed"), slog.Int64("id", id), slog.Any("error", err))
		return nil
	}
	return cfg
}

// AddEngine adds a single engine config to config.db, returning the DB-assigned ID.
func (w *EngineWrapper) AddEngine(cfg *engine.EngineConfig) (int64, error) {
	if w.configStore == nil {
		return 0, errEngineStoreNotReady
	}
	if miss := engine.ValidateRequired(cfg); miss != nil {
		return 0, fmt.Errorf(i18n.T("err.service_missing_field"), miss.LabelKey)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id, err := w.configStore.InsertEngineConfig(ctx, cfg)
	if err != nil {
		return 0, err
	}
	w.registerEngines()
	w.reregisterHotkeys()
	// Backend-wide broadcast: notify all windows (especially the translate window) that
	// engines changed, so they re-fetch the list.
	w.app.Event.Emit(events.EventEnginesChanged, EngineChangedPayload{ID: cfg.ID, Enabled: cfg.Enabled})
	return id, nil
}

// UpdateEngineConfig updates all config fields of a single engine by ID.
func (w *EngineWrapper) UpdateEngineConfig(cfg *engine.EngineConfig) error {
	if w.configStore == nil {
		return errEngineStoreNotReady
	}
	if miss := engine.ValidateRequired(cfg); miss != nil {
		return fmt.Errorf(i18n.T("err.service_missing_field"), miss.LabelKey)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.configStore.UpdateEngineConfig(ctx, cfg); err != nil {
		return err
	}
	w.registerEngines()
	w.reregisterHotkeys()
	// Backend-wide broadcast: notify all windows (especially the translate window) that
	// engines changed, so they re-fetch the list.
	w.app.Event.Emit(events.EventEnginesChanged, EngineChangedPayload{ID: cfg.ID, Enabled: cfg.Enabled})
	return nil
}

// ToggleEngineEnabled toggles a single engine's enabled/disabled state by ID.
// When enabling (enabled=true), required params are validated; missing ones reject the
// enable with a hint.
func (w *EngineWrapper) ToggleEngineEnabled(id int64, enabled bool) error {
	if w.configStore == nil {
		return errEngineStoreNotReady
	}
	var cfg *engine.EngineConfig
	if enabled {
		ctx0, cancel0 := context.WithTimeout(context.Background(), 5*time.Second)
		var err error
		cfg, err = w.configStore.GetEngineByID(ctx0, id)
		cancel0()
		if err != nil {
			return err
		}
		if miss := engine.ValidateRequired(cfg); miss != nil {
			return fmt.Errorf(i18n.T("err.service_missing_field"), miss.LabelKey)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := w.configStore.SetEngineEnabled(ctx, id, enabled); err != nil {
		return err
	}
	// OCR single-select: only when enabling an OCR engine, automatically disable other
	// enabled OCR engines (e.g. tesseract in the store), guaranteeing at most one OCR engine
	// enabled at a time. Enabling a translation engine must not trigger this.
	if enabled && cfg != nil && engine.KindOfEngine(cfg.Engine) == engine.KindOCR {
		if err := w.disableOtherOcrs(ctx, id); err != nil {
			return err
		}
	}
	w.registerEngines()
	w.reregisterHotkeys()
	// Backend-wide broadcast: notify all windows (especially the translate window) that the
	// engines' enabled state changed.
	w.app.Event.Emit(events.EventEnginesChanged, EngineChangedPayload{ID: id, Enabled: enabled})
	return nil
}

// disableOtherOcrs disables all enabled OCR engines except exceptID (store configs).
func (w *EngineWrapper) disableOtherOcrs(ctx context.Context, exceptID int64) error {
	engs, err := w.configStore.LoadEngines(ctx)
	if err != nil {
		return err
	}
	for _, e := range engs {
		if e.ID == exceptID || e.Enabled == 0 {
			continue
		}
		if engine.KindOfEngine(e.Engine) != engine.KindOCR {
			continue
		}
		if err := w.configStore.SetEngineEnabled(ctx, e.ID, false); err != nil {
			return err
		}
	}
	return nil
}

// RemoveEngine deletes a single engine config by ID.
// System built-in engines (apple / vision) cannot be deleted: they are config-free and their
// existence is guaranteed by code; deleting them would break OCR single-select / system
// translation. The frontend already hides the delete button for builtin entries; this adds a
// backend guard so other call paths can't delete them by mistake.
func (w *EngineWrapper) RemoveEngine(id int64) error {
	if w.configStore == nil {
		return errEngineStoreNotReady
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Backend guard: deleting system built-in engines is forbidden.
	if engs, err := w.configStore.LoadEngines(ctx); err == nil {
		for _, e := range engs {
			if e.ID == id && builtinEngine[e.Engine] {
				return fmt.Errorf(i18n.T("err.service_builtin_cannot_delete"))
			}
		}
	}
	if err := w.configStore.DeleteEngineByID(ctx, id); err != nil {
		return err
	}
	w.registerEngines()
	w.reregisterHotkeys()
	// Backend-wide broadcast: notify all windows (especially the translate window) that an
	// engine was deleted.
	w.app.Event.Emit(events.EventEnginesChanged, EngineChangedPayload{ID: id, Enabled: false})
	return nil
}

// loadEngines loads engine config from config.db into settings (the single source of truth)
// and registers them into the registry.
func (w *EngineWrapper) loadEngines() error {
	if w.configStore == nil {
		return errEngineStoreNotReady
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	engs, err := w.configStore.LoadEngines(ctx)
	if err != nil {
		return err
	}

	if len(engs) == 0 {
		log.Info(i18n.T("log.engine_table_empty_init"))
		def := engine.DefaultEngineConfigs()
		if err := w.configStore.InitDefaultEngines(ctx, def); err != nil {
			return err
		}
		if _, err := w.configStore.LoadEngines(ctx); err != nil {
			return err
		}
	}
	w.registerEngines()
	return nil
}

// EngineChangedPayload is the engine-config-change broadcast payload.
type EngineChangedPayload struct {
	ID      int64 `json:"id"`      // The engine ID that changed
	Enabled bool  `json:"enabled"` // The enabled state after the change
}

// EngineListItem backs the settings page's "engines" grouping: service names left, configs
// right.
// Kind distinguishes translate / ocr; the frontend groups and renders different markers
// accordingly.
type EngineListItem struct {
	Value     string `json:"value"`     // Engine identifier
	Name      string `json:"name"`      // Display name
	Kind      string `json:"kind"`      // translate | ocr
	Supported bool   `json:"supported"` // Whether the current platform supports it (e.g. apple is darwin-only)
}

// AllEngineItem is a settings-page "all configurable services" entry (enabled or not).
type AllEngineItem struct {
	ID        int64  `json:"id"`        // Engine auto-increment primary key (0 for built-ins)
	Value     string `json:"value"`     // Engine identifier
	Name      string `json:"name"`      // Display name
	Kind      string `json:"kind"`      // translate | ocr
	Enabled   bool   `json:"enabled"`   // Whether enabled
	Supported bool   `json:"supported"` // Whether the current platform supports it
	Builtin   bool   `json:"builtin"`   // System built-in config-free engine (e.g. vision system OCR); cannot be deleted, but the switch can be toggled
}
