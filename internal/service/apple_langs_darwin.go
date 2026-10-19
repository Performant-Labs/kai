//go:build darwin

package service

import (
	"cnb.cool/dtapp/kai/internal/engine"
)

// SystemLanguages returns the language packs available to the system translation engine:
// on macOS, the Translation.framework language packs; on other platforms, an empty list.
func (w *ConfigWrapper) SystemLanguages() []string {
	langs, err := engine.AvailableLanguages()
	if err != nil {
		return nil
	}
	return langs
}
