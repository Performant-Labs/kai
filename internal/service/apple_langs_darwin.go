//go:build darwin

package service

import (
	"cnb.cool/dtapp/kai/internal/engine"
)

// SystemLanguages returns the language packs installed for macOS system translation
// (Translation.framework).
// darwin only; for non-darwin platforms see apple_langs_other.go.
func (w *ConfigWrapper) SystemLanguages() []string {
	langs, err := engine.AvailableLanguages()
	if err != nil {
		return nil
	}
	return langs
}
