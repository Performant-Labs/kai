// Package selection provides selection reading / selection coordinates (pure business
// logic, no dependency on the wails lifecycle).
// It covers: reading the current selection text (cross-platform, with clipboard fallback)
// and locating by selection coordinates.
// Cross-platform selection coordinates (currentSelectionPoint / primaryScreenSize) and the
// per-system text capture (currentSelectionOSA / GetSelectedText) live in this package's
// per-platform files.
package selection

import (
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/settings"
)

// Service is the selection domain service.
type Service struct {
	app         *application.App
	settingsSvc *settings.Service
}

// NewService constructs the selection service. app is for clipboard reads/writes;
// settingsSvc for platform-related config.
func NewService(app *application.App, st *settings.Service) *Service {
	return &Service{app: app, settingsSvc: st}
}

// SetApp injects app once it is ready (startup orchestration phase).
func (s *Service) SetApp(app *application.App) {
	s.app = app
}

// ReadClipboardText reads the system clipboard text cross-platform (generic
// implementation; platform differences in the per-platform copy files).
func (s *Service) ReadClipboardText() string {
	return readClipboardText(s.app)
}

// WriteToClipboard writes text to the system clipboard cross-platform.
func (s *Service) WriteToClipboard(text string) error {
	if s.app == nil {
		return nil
	}
	if !s.app.Clipboard.SetText(text) {
		return fmt.Errorf(i18n.T("err.selection_write_clipboard"))
	}
	return nil
}

// TODO(2026-08-11): SelectedTextViaSystem is disabled. It was the system text-capture entry
// used only by the "summon main window" hotkey (the case h.selSvc != nil branch in
// manager.go); that branch is commented out due to suspected machine issues, so this method
// currently has no active callers. The implementation is kept — restore it together with the
// manager.go branch.
// Note: the selection floating window / selection translation still go through
// ReadSelection → currentSelectionOSA (Swift kai_selected_text), independent of this path
// and unaffected.
//
// SelectedTextViaSystem reads the current foreground app's selection via system text capture
// directly (no copy key/clipboard dependency).
// macOS uses the Swift bridge (AXUIElement, kai_selected_text), Windows uses UI Automation,
// other platforms return an empty string. Lets the "summon main window" hotkey grab the
// selection when no copy key is configured.
// func (s *Service) SelectedTextViaSystem() string {
// 	return currentSelectionOSA()
// }

// ReadSelection prefers the selection (OSA / system text capture), falling back to the
// clipboard on failure.
func (s *Service) ReadSelection() string {
	text := currentSelection()
	if text != "" {
		return text
	}
	return s.ReadClipboardText()
}

// ScreenSize returns the primary screen resolution.
func (s *Service) ScreenSize() (float64, float64) {
	return primaryScreenSize()
}

// SelectionPoint returns the current selection anchor's screen coordinates (nil when no
// selection).
// Lets exec keys attach coordinates when pushing selections, sharing the same coordinate
// source as emitSelection.
func (s *Service) SelectionPoint() *application.Point {
	return currentSelectionPoint()
}

// currentSelection prefers the selection, falling back to the clipboard (cross-platform
// generic; platform implementations in the per-platform files).
func currentSelection() string {
	if text := currentSelectionOSA(); text != "" {
		return text
	}
	return readClipboardText(nil)
}
