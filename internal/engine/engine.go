package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// Translator is the unified translation-engine interface.
type Translator interface {
	Name() string
	Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error)
}

// autoSourceSupporter is an optional interface: engines declare whether they support
// "auto-detect source language" (from=auto). Engines that don't implement it are treated as
// supporting it by default (cloud engines usually do); system translation (Translation.framework)
// requires an explicit source language due to API limits, so it returns false explicitly.
type autoSourceSupporter interface {
	SupportsAutoSource() bool
}

// SupportsAutoSource reports whether the engine supports auto-detecting the source language;
// engines that don't implement the optional interface are treated as supporting it.
func SupportsAutoSource(t Translator) bool {
	if s, ok := t.(autoSourceSupporter); ok {
		return s.SupportsAutoSource()
	}
	return true
}

// ValidateRequired checks, per the engine's field schema, that fields with Required=true are
// filled in when enabling/saving. Returns the first missing field (LabelKey for the frontend
// to translate the hint); nil when all are satisfied.
// Only Required fields are validated; non-required fields with a default value pass even if empty.
func ValidateRequired(cfg *EngineConfig) *EngineFieldSchema {
	if cfg == nil {
		return nil
	}
	schema := GetEngineSchema(cfg.Engine)
	for i := range schema.Fields {
		f := &schema.Fields[i]
		if !f.Required {
			continue
		}
		if valueOfField(cfg, f.Field) == "" {
			return f
		}
	}
	return nil
}

// valueOfField reads the EngineConfig field matching the schema's Field name.
func valueOfField(cfg *EngineConfig, field string) string {
	switch field {
	case "api_key":
		return cfg.APIKey
	case "secret":
		return cfg.Secret
	case "extra":
		return cfg.Extra
	case "endpoint":
		return cfg.Endpoint
	default:
		return ""
	}
}

// EngineConfig holds one engine's credentials/config (single source of truth; the settings
// package reuses this type so engine and settings never import each other in a cycle).
// ID is the config.db primary key: 0 means not yet persisted (newly added by the frontend),
// >0 means an existing row (frontend updates/deletes key off it).
// Engine config has moved to config.db persistence, no longer settings.json (hence no
// mapstructure tags; json tags kept for frontend RPC).
type EngineConfig struct {
	ID       int64  `json:"id"`                 // Auto-increment primary key of the engine config
	Engine   string `json:"engine"`             // Engine identifier
	Enabled  bool   `json:"enabled"`            // Whether enabled
	APIKey   string `json:"api_key,omitempty"`  // API token ID (used as the auth credential)
	Secret   string `json:"secret,omitempty"`   // API token secret (used as the signing key)
	Extra    string `json:"extra,omitempty"`    // Extra extension config (JSON string)
	Endpoint string `json:"endpoint,omitempty"` // Custom endpoint URL (optional)
	// HTTPClient optionally injects the global HTTP client (with custom DNS/proxy/logging).
	// Injected by the service layer; engines built on the Google API (Gemini etc.) must get it,
	// otherwise the SDK touches the useragent-wrapped global http.DefaultTransport and panics.
	// When nil, the engine falls back to building its own client with an independent
	// *http.Transport.
	HTTPClient *http.Client `json:"-"`
}

// Engine-layer shared errors (single source of truth; settings/service reuse these, avoiding
// a cycle where engine imports settings)
var (
	ErrAPIKey   = fmt.Errorf(i18n.T("err.no_apikey"))
	ErrNoEngine = fmt.Errorf(i18n.T("err.no_engine"))
	ErrNoOCR    = fmt.Errorf(i18n.T("err.no_ocr"))
)

// defaultEngineNames lists the engines built in and working out of the box (the rest must be
// "added" by the user in the settings page). system (system translation) and google are
// key-free out of the box; macOS additionally presets vision (system OCR, zero-install and
// offline) so mac always has a working OCR engine from the start.
// deepl needs an API Key, so it is not preset by default — that would create an invalid
// "declared enabled but missing credentials" engine; openai / baidu / tencent / youdao /
// tesseract are likewise not preset and can be added in the settings page.
var defaultEngineNames = func() map[string]bool {
	m := map[string]bool{
		"apple":  true,
		"google": true,
	}
	// System OCR (vision) is macOS-only; it ships as a default engine only on that platform.
	if runtime.GOOS == "darwin" {
		m["vision"] = true
	}
	return m
}()

// DefaultEngineConfigs returns the default engine configs (single source of truth).
// The engine list comes from KnownEngines, no longer handwritten; only the built-in engines
// in defaultEngineNames are taken, and fields with a schema Default are pre-filled with real
// defaults (e.g. the DeepL free-tier URL, ready for the user to add later), so the persisted
// default config carries real values instead of empty ones. The remaining engines (including
// deepl) default to off in the settings page's "available services" list, for the user to add
// and fill in keys.
func DefaultEngineConfigs() []*EngineConfig {
	metas := KnownEngines()
	out := make([]*EngineConfig, 0, len(defaultEngineNames))
	for _, m := range metas {
		// Only built-in default engines; the rest are left for the user to add via the UI
		if !defaultEngineNames[m.Name] {
			continue
		}
		// Engines unavailable on the current platform (e.g. apple system translation on
		// Windows/Linux) are not preset — no point defaulting to an engine that can't work.
		// Users can manually add engines supported on their platform.
		if !EngineSupported(m.Name) {
			continue
		}
		cfg := &EngineConfig{
			Engine:  m.Name,
			Enabled: true, // Built-in key-free engines are enabled by default
		}
		// vision (system OCR) keeps its OCR-specific params in Extra(JSON): language correction
		// on by default + 60s timeout.
		// Note: vision uses the macOS system Vision framework with automatic language detection
		// and needs no langs field (langs is tesseract-only; both share the Extra struct merely
		// for format consistency — vision ignores langs).
		if m.Name == "vision" {
			cfg.Extra = `{"correct_text":true,"timeout_sec":60}`
		}
		// Pre-fill fields with a Default real value (endpoint / language codes etc.)
		for _, f := range GetEngineSchema(m.Name).Fields {
			if f.Default != "" {
				switch f.Field {
				case "endpoint":
					cfg.Endpoint = f.Default
				case "secret":
					cfg.Secret = f.Default
				case "extra":
					cfg.Extra = f.Default
				case "api_key":
					cfg.APIKey = f.Default
				}
			}
		}
		out = append(out, cfg)
	}
	return out
}

// Default endpoints per engine (single source of truth). Constructor fallback logic and the
// settings-page schema Defaults both reference these, keeping the magic strings from
// scattering across deepl.go / openai.go / schema and drifting apart.
const (
	// DeepLFreeEndpoint is the DeepL free-tier default endpoint (Pro users must switch to
	// api.deepl.com in settings)
	DeepLFreeEndpoint = "https://api-free.deepl.com/v2/translate"
	// OpenAIDefaultBaseURL is the default base URL for the OpenAI-compatible API (the SDK
	// appends /chat/completions)
	OpenAIDefaultBaseURL = "https://api.openai.com/v1"
	// DefaultEndpoint is Google's default public endpoint
	DefaultEndpoint = "https://translate.googleapis.com/translate_a/single"
	// BaiduDefaultEndpoint is the Baidu Translate open-platform default endpoint
	BaiduDefaultEndpoint = "https://fanyi-api.baidu.com/api/trans/vip/translate"
	// TencentDefaultEndpoint is the Tencent Machine Translation default endpoint
	TencentDefaultEndpoint = "https://tmt.tencentcloudapi.com"
	// YoudaoDefaultEndpoint is the Youdao Zhiyun default endpoint
	YoudaoDefaultEndpoint = "https://openapi.youdao.com/api"
	// AnthropicDefaultBaseURL is the default base URL for the Anthropic Claude API (the SDK
	// appends /v1/messages internally)
	AnthropicDefaultBaseURL = "https://api.anthropic.com"
	// GeminiDefaultEndpoint is the Gemini API default endpoint (full scheme+host; the SDK
	// appends /v1beta/models/... internally)
	GeminiDefaultEndpoint = "https://generativelanguage.googleapis.com"
)

// OcrEngine is the unified OCR-engine interface.
type OcrEngine interface {
	Name() string
	Recognize(ctx context.Context, req model.OcrRequest) (*model.OcrResult, error)
}

// Registry is the engine registry.
type Registry struct {
	translators map[string]Translator
	ocrs        map[string]OcrEngine
}

// NewRegistry creates a new registry.
func NewRegistry() *Registry {
	return &Registry{
		translators: make(map[string]Translator),
		ocrs:        make(map[string]OcrEngine),
	}
}

// RegisterTranslator registers a translation engine.
func (r *Registry) RegisterTranslator(t Translator) {
	r.translators[t.Name()] = t
}

// RegisterOcr registers an OCR engine.
func (r *Registry) RegisterOcr(o OcrEngine) {
	r.ocrs[o.Name()] = o
}

// GetTranslator looks up a translation engine.
func (r *Registry) GetTranslator(name string) (Translator, bool) {
	t, ok := r.translators[name]
	return t, ok
}

// GetOcr looks up an OCR engine.
func (r *Registry) GetOcr(name string) (OcrEngine, bool) {
	o, ok := r.ocrs[name]
	return o, ok
}

// TranslatorNames lists registered translation-engine names.
func (r *Registry) TranslatorNames() []string {
	names := make([]string, 0, len(r.translators))
	for n := range r.translators {
		names = append(names, n)
	}
	return names
}

// OcrNames lists registered OCR-engine names.
func (r *Registry) OcrNames() []string {
	names := make([]string, 0, len(r.ocrs))
	for n := range r.ocrs {
		names = append(names, n)
	}
	return names
}

// EngineKind is the engine kind: translation or OCR.
type EngineKind string

const (
	// KindTranslator is a translation engine.
	KindTranslator EngineKind = "translate"
	// KindOCR is an OCR engine.
	KindOCR EngineKind = "ocr"
)

// EngineMeta is engine metadata (for grouped display in the frontend settings page).
type EngineMeta struct {
	Name      string     `json:"name"`      // Display name
	Kind      EngineKind `json:"kind"`      // Engine kind (translate | ocr)
	Supported bool       `json:"supported"` // Whether the current platform supports it (e.g. apple is darwin-only)
}

// EngineSupported reports whether the engine works on the current platform (exported for the
// service layer to reuse). apple (macOS system translation) and vision (macOS system OCR) are
// darwin-only; all other engines are cross-platform.
func EngineSupported(name string) bool {
	switch name {
	case "apple", "vision":
		return runtime.GOOS == "darwin"
	}
	return true
}

// engineSupported is an in-package alias keeping call sites stylistically consistent.
func engineSupported(name string) bool { return EngineSupported(name) }

// KnownEngines returns metadata for all "known" engines (registered/enabled or not).
// Used by the settings page to list every configurable service for the user to enable/fill in
// credentials, rather than only showing enabled ones.
// The order is the display order in the settings page.
func KnownEngines() []EngineMeta {
	names := []string{
		"apple",
		"google",
		"deepl",
		"openai",
		"anthropic",
		"gemini",
		"baidu",
		"tencent",
		"youdao",
		"tesseract",
		"vision",
	}
	out := make([]EngineMeta, 0, len(names))
	for _, n := range names {
		out = append(out, EngineMeta{
			Name:      n,
			Kind:      KindOfEngine(n),
			Supported: engineSupported(n),
		})
	}
	return out
}

// KindOfEngine returns the engine kind (tesseract / vision are OCR, everything else is
// translation). Exported for the service layer to reuse. apple (macOS system translation)
// counts as translation.
func KindOfEngine(name string) EngineKind {
	switch name {
	case "tesseract", "vision":
		return KindOCR
	}
	return KindTranslator
}

// EngineMap converts an engine-config slice into a map keyed by engine name, for name-based
// access (e.g. EngineMap(cfg.Engines)["google"]).
func EngineMap(engines []*EngineConfig) map[string]*EngineConfig {
	out := make(map[string]*EngineConfig, len(engines))
	for _, e := range engines {
		if e != nil {
			out[e.Engine] = e
		}
	}
	return out
}

// AllEngines returns metadata for all registered engines (translation + OCR), grouped by kind.
// Used by the settings page's "engines" grouping: service names on the left, configs on the
// right, with translate/OCR distinguished.
func (r *Registry) AllEngines() []EngineMeta {
	out := make([]EngineMeta, 0, len(r.translators)+len(r.ocrs))
	for n := range r.translators {
		out = append(out, EngineMeta{Name: n, Kind: KindTranslator, Supported: engineSupported(n)})
	}
	for n := range r.ocrs {
		out = append(out, EngineMeta{Name: n, Kind: KindOCR, Supported: engineSupported(n)})
	}
	return out
}

// DefaultOCREngineName returns the first registered OCR-engine name, for OCR calls without an
// explicit engine (hotkey/frontend). Returns an empty string when no OCR engine is available.
func (r *Registry) DefaultOCREngineName() string {
	for _, m := range r.AllEngines() {
		if m.Kind == KindOCR {
			return m.Name
		}
	}
	return ""
}

// EngineFieldType is a config-field type (the frontend renders different controls per type).
type EngineFieldType string

const (
	// FieldString is plain text.
	FieldString EngineFieldType = "string"
	// FieldSecret is sensitive (password box).
	FieldSecret EngineFieldType = "secret"
)

// FieldWidget overrides the control shape. When empty, rendering follows Type by default
// (string→text, secret→password); when set, a structured control is rendered instead, shared
// by OCR and similar engines so "all config items are declared in engineSchemas".
type FieldWidget string

const (
	// WidgetOCRLangs is an OCR recognition-language multi-select (options from
	// OcrLangsOptions()); the value is joined with "+" into Extra.langs
	WidgetOCRLangs FieldWidget = "ocr_langs"
	// WidgetOCRTimeout is the OCR timeout in seconds (positive integer), written to
	// Extra.timeout_sec
	WidgetOCRTimeout FieldWidget = "ocr_timeout"
	// WidgetOCRCorrect is the OCR language-correction toggle, written to Extra.correct_text
	// (only meaningful for vision)
	WidgetOCRCorrect FieldWidget = "ocr_correct"
	// WidgetOCRRetry is the Vision OCR failure-retry count (positive integer), written to
	// Extra.retry_count (only meaningful for vision)
	WidgetOCRRetry FieldWidget = "ocr_retry"
	// WidgetOCRStatus is the tesseract install-status probe card (with an editable custom
	// binary-path endpoint), tesseract only
	WidgetOCRStatus FieldWidget = "ocr_status"
	// WidgetLLMModel is the LLM translation engine's model name, a standalone text input whose
	// value is merged into Extra.model
	WidgetLLMModel FieldWidget = "llm_model"
)

// EngineFieldSchema describes a single config field an engine needs.
// Field maps to EngineConfig's real field name (api_key / secret / endpoint / extra), so the
// frontend reads/writes the corresponding field directly instead of forcing every engine
// through the same one-size-fits-all form.
type EngineFieldSchema struct {
	Field          string          `json:"field"`           // Target field: api_key / secret / endpoint / extra
	LabelKey       string          `json:"label_key"`       // i18n key (frontend reads settings.engine_field.<name>)
	PlaceholderKey string          `json:"placeholder_key"` // i18n key (frontend reads settings.engine_ph.<name>; may be empty)
	Type           EngineFieldType `json:"type"`            // string / secret
	Widget         FieldWidget     `json:"widget"`          // Structured-control override (ocr_*); empty renders per Type
	Required       bool            `json:"required"`        // Whether required when enabling the engine
	Default        string          `json:"default"`         // Real default for optional URL/address fields; the frontend pre-fills the display when empty
	Options        []string        `json:"options"`         // Optional enum values (e.g. tesseract codes chi_sim/eng). When set, the frontend renders a multi-select whose values are joined with "+" into the field
	HintKey        string          `json:"hint_key"`        // Optional: i18n key for the hint text below the field
}

// EngineSchema is an engine's full set of config fields (order = render order).
type EngineSchema struct {
	// Kind is the engine kind (translate / ocr), single source of truth; the frontend
	// distinguishes rendering by it, replacing the scattered AllEngineItem.kind checks.
	Kind EngineKind `json:"kind"`
	// Builtin marks a system built-in engine (e.g. apple system translation / vision system
	// OCR): nothing to configure, cannot be removed. The frontend renders a uniform
	// "system built-in" status card from this, replacing per-engine hardcoded checks.
	Builtin bool `json:"builtin"`
	// Fields is the list of fields rendered in the frontend config form (order = render
	// order). Each entry maps to one input (endpoint / api_key / secret / language etc.),
	// from which the frontend generates the form dynamically.
	Fields []EngineFieldSchema `json:"fields"`
}

// engineSchemas centrally defines the fields each engine "really" needs. The frontend renders
// dynamically from this, avoiding the old one-size-fits-all form that showed a pile of
// irrelevant fields for every engine (e.g. tesseract showing an API Key, openai's
// secret/extra having unclear semantics).
//
// Based on what each engine's NewXxx constructor actually reads:
//   - apple/google: public endpoints, key-free (google's endpoint is customizable in settings;
//     empty uses the default)
//   - deepl: endpoint defaults to the free-tier endpoint (switchable to Pro); api_key is
//     required (the free tier still requires registering for one)
//   - openai: api_key + endpoint (as Base URL, default https://api.openai.com/v1, the SDK
//     appends /chat/completions) + extra (as model)
//   - baidu/tencent/youdao: appkey/appid = api_key, secret key = secret
//   - tesseract: endpoint (optional tesseract binary path); OCR-specific params like language
//     codes/timeout live uniformly in Extra(JSON)
var engineSchemas = map[string]EngineSchema{
	// apple is the macOS built-in translation engine (Translation.framework): nothing to
	// configure, cannot be removed.
	"apple": {
		Kind:    KindTranslator,
		Builtin: true,
		Fields:  nil,
	},
	// vision is the macOS built-in OCR engine (Vision.framework): nothing to configure,
	// cannot be removed. Its OCR params (language correction / timeout) are declared here so
	// the frontend renders them in schema order, shared with tesseract.
	"vision": {
		Kind:    KindOCR,
		Builtin: true,
		Fields: []EngineFieldSchema{
			{
				Field:    "extra",
				Widget:   WidgetOCRCorrect,
				LabelKey: "settings.engineOcrCorrect",
				HintKey:  "settings.engineOcrCorrectDesc",
			},
			{
				Field:    "extra",
				Widget:   WidgetOCRTimeout,
				LabelKey: "settings.engineOcrTimeout",
				HintKey:  "settings.engineOcrTimeoutDesc",
			},
			{
				Field:    "extra",
				Widget:   WidgetOCRRetry,
				LabelKey: "settings.engineOcrRetry",
				HintKey:  "settings.engineOcrRetryDesc",
			},
		},
	},
	"google": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field: "endpoint",

				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.google_endpoint",
				Type:           FieldString,
				Required:       false,
				Default:        DefaultEndpoint,
			},
		},
	},
	"deepl": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.deepl_endpoint",
				Type:           FieldString,
				Required:       false,
				Default:        DeepLFreeEndpoint,
			},
			{
				Field:          "api_key",
				LabelKey:       "settings.engine_field.api_key",
				PlaceholderKey: "settings.engine_ph.deepl_api_key",
				Type:           FieldSecret,
				Required:       true,
			},
		},
	},
	"openai": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.openai_endpoint",
				Type:           FieldString,
				Required:       false,
				Default:        OpenAIDefaultBaseURL,
			},
			{
				Field:          "api_key",
				LabelKey:       "settings.engine_field.api_key",
				PlaceholderKey: "settings.engine_ph.openai_api_key",
				Type:           FieldSecret,
				Required:       true,
			},
			{
				Field:          "llm_model",
				Widget:         WidgetLLMModel,
				LabelKey:       "settings.engine_field.model",
				PlaceholderKey: "settings.engine_ph.openai_model",
				Type:           FieldString,
				Required:       false,
				Default:        "gpt-4o-mini",
			},
		},
	},
	"baidu": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.baidu_endpoint",
				Type:           FieldString,
				Required:       false,
				Default:        BaiduDefaultEndpoint,
			},
			{
				Field:          "api_key",
				LabelKey:       "settings.engine_field.app_id",
				PlaceholderKey: "settings.engine_ph.baidu_app_id",
				Type:           FieldString,
				Required:       true,
			},
			{
				Field:          "secret",
				LabelKey:       "settings.engine_field.app_secret",
				PlaceholderKey: "settings.engine_ph.baidu_app_secret",
				Type:           FieldSecret,
				Required:       true,
			},
		},
	},
	"tencent": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.tencent_endpoint",
				Type:           FieldString,
				Required:       false,
				Default:        TencentDefaultEndpoint,
			},
			{
				Field:          "api_key",
				LabelKey:       "settings.engine_field.secret_id",
				PlaceholderKey: "settings.engine_ph.tencent_secret_id",
				Type:           FieldString,
				Required:       true,
			},
			{
				Field:          "secret",
				LabelKey:       "settings.engine_field.secret_key",
				PlaceholderKey: "settings.engine_ph.tencent_secret_key",
				Type:           FieldSecret,
				Required:       true,
			},
		},
	},
	"youdao": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.youdao_endpoint",
				Type:           FieldString, Required: false, Default: YoudaoDefaultEndpoint},
			{
				Field:          "api_key",
				LabelKey:       "settings.engine_field.app_key",
				PlaceholderKey: "settings.engine_ph.youdao_app_key",
				Type:           FieldString, Required: true},
			{
				Field:          "secret",
				LabelKey:       "settings.engine_field.app_secret",
				PlaceholderKey: "settings.engine_ph.youdao_app_secret",
				Type:           FieldSecret, Required: true},
		},
	},
	"anthropic": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.anthropic_endpoint",
				Type:           FieldString,
				Required:       false,
				Default:        AnthropicDefaultBaseURL,
			},
			{
				Field:          "api_key",
				LabelKey:       "settings.engine_field.api_key",
				PlaceholderKey: "settings.engine_ph.anthropic_api_key",
				Type:           FieldSecret,
				Required:       true,
			},
			{
				Field:          "llm_model",
				Widget:         WidgetLLMModel,
				LabelKey:       "settings.engine_field.model",
				PlaceholderKey: "settings.engine_ph.anthropic_model",
				Type:           FieldString,
				Required:       false,
				Default:        "claude-3-5-sonnet-20241022",
			},
		},
	},
	"gemini": {
		Kind: KindTranslator,
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				LabelKey:       "settings.engine_field.endpoint",
				PlaceholderKey: "settings.engine_ph.gemini_endpoint",
				Type:           FieldString,
				Required:       false,
				Default:        GeminiDefaultEndpoint,
			},
			{
				Field:          "api_key",
				LabelKey:       "settings.engine_field.api_key",
				PlaceholderKey: "settings.engine_ph.gemini_api_key",
				Type:           FieldSecret,
				Required:       true,
			},
			{
				Field:          "llm_model",
				Widget:         WidgetLLMModel,
				LabelKey:       "settings.engine_field.model",
				PlaceholderKey: "settings.engine_ph.gemini_model",
				Type:           FieldString,
				Required:       false,
				Default:        "gemini-2.0-flash",
			},
		},
	},
	"tesseract": {
		Kind: KindOCR,
		// All config items are declared here; order = frontend render order (single source of
		// truth):
		//   1) ocr_status  install-status probe card (with an editable custom binary-path endpoint)
		//   2) ocr_langs   recognition-language multi-select (written to Extra.langs)
		//   3) ocr_timeout OCR timeout (written to Extra.timeout_sec)
		Fields: []EngineFieldSchema{
			{
				Field:          "endpoint",
				Widget:         WidgetOCRStatus,
				LabelKey:       "settings.engine_field.binary",
				PlaceholderKey: "settings.engine_ph.tesseract_binary",
				Type:           FieldString,
				Required:       false,
			},
			{
				Field:    "extra",
				Widget:   WidgetOCRLangs,
				LabelKey: "settings.engineOcrLangs",
				HintKey:  "settings.engineLangsHintTesseract",
			},
			{
				Field:    "extra",
				Widget:   WidgetOCRTimeout,
				LabelKey: "settings.engineOcrTimeout",
				HintKey:  "settings.engineOcrTimeoutDesc",
			},
		},
	},
}

// ocrLangsOptions lists language-code options for OCR engines (vision / tesseract), used by
// the frontend's OCR-specific UI to render the langs multi-select. Decoupled from the schema
// and maintained solely in code.
var ocrLangsOptions = []string{"chi_sim", "chi_tra", "eng", "jpn", "kor", "fra", "deu", "spa", "rus", "por", "ita"}

// OcrLangsOptions returns the OCR-engine language-code options (for the frontend's langs
// multi-select).
func OcrLangsOptions() []string {
	out := make([]string, len(ocrLangsOptions))
	copy(out, ocrLangsOptions)
	return out
}

// GetEngineSchema returns the config-field schema for the given engine; an empty schema when
// undefined.
func GetEngineSchema(name string) EngineSchema {
	if s, ok := engineSchemas[name]; ok {
		return s
	}
	return EngineSchema{}
}

// ocrExtra is the unified JSON parse result of an OCR engine's Extra.
// All OCR engines (vision / tesseract) share the same Extra(JSON) structure so the JSON
// changes in one place.
//   - langs:      language codes, e.g. "chi_sim+eng" ("+"-separated).
//   - timeoutSec: OCR timeout in seconds; <=0 falls back to the default 60.
//   - correct:    whether language correction is on; only meaningful for vision, ignored by
//     tesseract. nil/true = on.
//   - retryCount: Vision OCR failure-fallback retry count (only meaningful for vision,
//     ignored by tesseract). This is the "extra retries" count, excluding the first attempt;
//     <=0 (or missing field) falls back to the default 2. Explicit 0 disables retries.
type ocrExtra struct {
	Langs      string `json:"langs"`        // Language codes ("+"-separated); missing falls back to the default
	TimeoutSec int    `json:"timeout_sec"`  // OCR timeout in seconds; <=0 uses the default 60
	Correct    *bool  `json:"correct_text"` // Language correction (nil/true = on), vision only
	RetryCount *int   `json:"retry_count"`  // Vision OCR failure-fallback "extra retries" (excluding the first); nil = default 2, explicit 0 = off
}

// DefaultOCRLangs holds per-OCR-engine default language codes (fallback when Extra is absent
// or lacks langs).
var DefaultOCRLangs = map[string]string{
	"vision":    "chi_sim+eng",
	"tesseract": "chi_sim+eng",
}

// DefaultOCRTimeoutSec is the default OCR timeout in seconds.
const DefaultOCRTimeoutSec = 60

// DefaultOCRRetryCount is the default Vision OCR failure-fallback retry count.
const DefaultOCRRetryCount = 2

// parseOCRExtra parses an OCR engine's Extra(JSON) uniformly. Shared by both engines to keep
// the extra format consistent.
// Backward compatible: when Extra is a plain string language code (not JSON), the whole
// string becomes the langs fallback and the timeout falls back to the default.
// timeoutSec/correct may be explicitly overridden by the request req (handled in each
// Recognize).
func parseOCRExtra(engineName, extra string) ocrExtra {
	out := ocrExtra{
		Langs:      DefaultOCRLangs[engineName],
		TimeoutSec: DefaultOCRTimeoutSec,
	}
	if extra == "" {
		return out
	}
	// Prefer JSON parsing (the unified scheme).
	var je ocrExtra
	if err := json.Unmarshal([]byte(extra), &je); err == nil {
		if je.Langs != "" {
			out.Langs = je.Langs
		}
		if je.TimeoutSec > 0 {
			out.TimeoutSec = je.TimeoutSec
		}
		if je.Correct != nil {
			out.Correct = je.Correct
		}
		if je.RetryCount != nil {
			out.RetryCount = je.RetryCount
		}
		return out
	}
	// Backward compat: old plain-string language codes (e.g. "chi_sim+eng").
	out.Langs = extra
	return out
}

// llmExtra is the unified JSON parse result of LLM translation engines' (openai / anthropic /
// gemini) Extra. All LLM engines share the same Extra(JSON) structure so the JSON changes in
// one place.
//
// It has no timeout (issue #109): an LLM request runs until it answers or the user cancels it.
// A timeout_sec key that older versions stored in an engine row is neither read nor migrated;
// decoding simply skips it, so such a row still parses to its model and imposes nothing.
type llmExtra struct {
	Model string `json:"model"` // Model name (e.g. gpt-4o-mini, claude-3-5-sonnet-20241022, gemini-2.0-flash)
}

// parseLLMExtra parses an LLM engine's Extra(JSON) uniformly.
// Backward compatible: when Extra is a plain model-name string (not JSON), the whole string
// becomes the model fallback.
func parseLLMExtra(extra string) llmExtra {
	out := llmExtra{}
	if extra == "" {
		return out
	}
	// Prefer JSON parsing (the unified scheme).
	var je llmExtra
	if err := json.Unmarshal([]byte(extra), &je); err == nil {
		if je.Model != "" {
			out.Model = je.Model
		}
		return out
	}
	// Backward compat: old plain-string model names (e.g. "gpt-4o-mini").
	out.Model = extra
	return out
}
