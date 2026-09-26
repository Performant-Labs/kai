package i18n

import (
	"embed"
	"encoding/json"
	"sync"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.json
var localesFS embed.FS

type Locale string

const (
	ZH_CN Locale = "zh-CN"
	EN_US Locale = "en-US"
)

var (
	mu     sync.RWMutex
	locale Locale = EN_US

	bundle *i18n.Bundle
)

func init() {
	bundle = i18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)
	loadMessages()
}

func loadMessages() {
	entries, err := localesFS.ReadDir("locales")
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		data, err := localesFS.ReadFile("locales/" + entry.Name())
		if err != nil {
			continue
		}
		bundle.ParseMessageFileBytes(data, entry.Name())
	}
}

func localizer() *i18n.Localizer {
	mu.RLock()
	defer mu.RUnlock()
	return i18n.NewLocalizer(bundle, string(locale))
}

// SetLocale sets the current locale.
//
// "auto" follows the system language (only a Chinese system language selects Chinese); any other
// unrecognized value is English, the default interface language.
func SetLocale(l string) {
	// Resolve "auto" before taking the lock: detection may run a subprocess.
	var resolved Locale
	switch Locale(l) {
	case ZH_CN:
		resolved = ZH_CN
	case Auto:
		resolved = systemLocaleFn()
	default:
		resolved = EN_US
	}
	mu.Lock()
	defer mu.Unlock()
	locale = resolved
}

// GetLocale returns the current locale string.
func GetLocale() string {
	mu.RLock()
	defer mu.RUnlock()
	return string(locale)
}

// T translates a key with named template parameters.
// Usage: i18n.T("err.empty_text")
//
//	i18n.T("err.configstore_get_engine_id", "id", 123)
//	i18n.T("notification.update_available_subtitle", "version", "1.2.3")
func T(key string, templateData ...any) string {
	l := localizer()

	data := make(map[string]any)
	for i := 0; i < len(templateData)-1; i += 2 {
		if k, ok := templateData[i].(string); ok {
			// Guard against a nil interface: go-i18n calls reflect.Value.Type on values while
			// rendering templates; a zero-valued interface{} yields a zero Value and triggers
			// the "reflect.Value.Type on zero Value" panic.
			// Falling back to an empty string both avoids the panic and renders {{.k}} as a
			// placeholder instead of crashing.
			if templateData[i+1] == nil {
				data[k] = ""
				continue
			}
			data[k] = templateData[i+1]
		}
	}

	msg, err := l.Localize(&i18n.LocalizeConfig{
		MessageID:    key,
		TemplateData: data,
	})
	if err != nil {
		defaultLocalizer := i18n.NewLocalizer(bundle, string(EN_US))
		msg, err = defaultLocalizer.Localize(&i18n.LocalizeConfig{
			MessageID:    key,
			TemplateData: data,
		})
		if err != nil {
			return key
		}
	}
	return msg
}

// TWithLocale translates a key using the given locale.
func TWithLocale(loc string, key string, templateData ...any) string {
	mu.Lock()
	saved := locale
	locale = Locale(loc)
	if locale != ZH_CN {
		locale = EN_US
	}
	mu.Unlock()

	result := T(key, templateData...)

	mu.Lock()
	locale = saved
	mu.Unlock()

	return result
}

// ResolveLocale converts a frontend locale into the backend locale.
func ResolveLocale(loc string) string {
	if loc == string(ZH_CN) {
		return string(ZH_CN)
	}
	return string(EN_US)
}

// SupportedLocales returns all supported locale codes.
func SupportedLocales() []string {
	return []string{string(EN_US), string(ZH_CN)}
}

// GetCurrentLocale returns the current locale string.
func GetCurrentLocale() string {
	mu.RLock()
	defer mu.RUnlock()
	return string(locale)
}
