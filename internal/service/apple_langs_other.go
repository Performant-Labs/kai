//go:build !darwin

package service

// SystemLanguages: system translation is unavailable on non-macOS platforms, returns an
// empty list.
func (w *ConfigWrapper) SystemLanguages() []string {
	return nil
}
