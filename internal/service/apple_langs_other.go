//go:build !darwin

package service

// SystemLanguages returns the language packs available to the system translation engine:
// on macOS, the Translation.framework language packs; on other platforms, an empty list.
func (w *ConfigWrapper) SystemLanguages() []string {
	return nil
}
