package events

import "cnb.cool/dtapp/kai/internal/model"

// App event name constants, preventing typos.
const (
	// EventWindowShow: the frontend summons a window, payload: string ("settings" | "main")
	EventWindowShow = "kai:window:show"

	// EventWindowClosing is broadcast when a window close (native red X / custom title-bar
	// close) is triggered; each window clears its own state as needed (e.g. the translate
	// window clears input and results). payload: the window name (model.WindowTranslate
	// etc.); each window only handles its own close, avoiding accidental clearing when a
	// different window closes.
	// Aligned with the frontend's EventWindowClosing in frontend/src/utils/events.ts — the
	// single source of truth.
	EventWindowClosing = "kai:window:closing"

	// EventLocaleChanged is broadcast after the UI language changes, payload:
	// LocaleChangedPayload
	EventLocaleChanged = "kai:locale:changed"

	// EventThemeChanged is broadcast on theme change, payload: ThemeChangedPayload
	EventThemeChanged = "kai:theme:changed"

	// EventHotkeysChanged is broadcast after hotkeys re-register, payload: []string
	EventHotkeysChanged = "kai:hotkeys:changed"

	// EventInputFill backfills the main window input with the selection, payload: string
	// (the selected text)
	EventInputFill = "kai:input:fill"

	// EventTranslateResult: multi-engine translations return results one by one, payload:
	// model.TranslateResult
	EventTranslateResult = "kai:translate:result"

	// EventWindowScreenshot: a hotkey triggered screenshot translate; after the backend
	// persists the region screenshot it summons the screenshot window to receive results.
	EventWindowScreenshot = "kai:window:screenshot"

	// EventScreenshotOCR: the screenshot translate flow progressed — after the backend
	// captures the region→OCR→translates, it delivers results to the screenshot window.
	// payload: ScreenshotResult{Image, Text, Translations, To}
	EventScreenshotOCR = "kai:screenshot:ocr"

	// EventScreenshotRecapture: triggered by the frontend "recapture" button; the backend
	// hides the window and reruns the screenshot translate flow.
	EventScreenshotRecapture = "kai:screenshot:recapture"

	// EventScreenshotRetranslate: triggered after the frontend changes language; reuses the
	// most recent OCR text, skipping screenshot/OCR, retranslating with the new language and
	// pushing results incrementally. payload: ScreenshotRetranslatePayload
	EventScreenshotRetranslate = "kai:screenshot:retranslate"

	// EventEnginesChanged is broadcast after engines are added/removed/enabled/disabled or
	// their config changes, notifying all windows (especially the translate window) to
	// re-fetch the engine list. payload: EngineChangedPayload (changed engine ID + enabled
	// state).
	// Aligned with the frontend's EventEnginesChanged in frontend/src/utils/events.ts — the
	// single source of truth.
	EventEnginesChanged = "kai:engines:changed"

	// EventAutoClipboardChanged is broadcast when the input-translate window's "auto-read
	// clipboard and translate" switch changes.
	// payload: bool (on=true / off=false). The settings page uses it to disable/restore the
	// two copy-key switches live.
	// Aligned with the frontend's EventAutoClipboardChanged in frontend/src/utils/events.ts —
	// the single source of truth.
	EventAutoClipboardChanged = "kai:auto-clipboard:changed"
)

// Session identifiers for the screenshot/OCR cache: separating entry points so they never
// clobber each other.
const (
	// ScreenshotSessionScreenshot is the screenshot translate window (hotkey/menu/recapture
	// button all target ScreenshotWindow).
	ScreenshotSessionScreenshot = "screenshot"
	// ScreenshotSessionInput is screenshot OCR within the input translate page (reserved;
	// isolated from the screenshot translate window).
	ScreenshotSessionInput = "input"
)

// LocaleChangedPayload carries the UI language change event.
// Note: this is the UI display language — an entirely separate system from the translation
// languages (model.Language: auto/zh/en/...); never mix them.
// Mode is the user-configured UI language mode (auto / zh-CN / en-US);
// Language is the actually active UI language (zh-CN / en-US; derived from the system locale
// when auto).
type LocaleChangedPayload struct {
	Mode     string `json:"mode"`     // UI language mode: auto | zh-CN | en-US
	Language string `json:"language"` // Actually active UI language: zh-CN | en-US
}

// ThemeChangedPayload carries the theme change event.
// Mode is the user-configured mode (from model's ThemeAuto/ThemeLight/ThemeDark:
// auto/light/dark);
// Theme is the real system appearance (dark/light, derived from Env.IsDarkMode()).
type ThemeChangedPayload struct {
	Mode  string `json:"mode"`  // settings: auto | light | dark
	Theme string `json:"theme"` // settings: dark | light
}

// ScreenshotRetranslatePayload carries the screenshot-retranslate-after-language-change
// event.
// Session identifies the cache origin (ScreenshotSessionScreenshot /
// ScreenshotSessionInput); the backend uses it to fetch that entry point's most recent OCR
// text, keeping different entry points from bleeding into each other.
// From/To are the target translation language pair (From may be Auto); the backend reuses
// the most recent OCR text to retranslate.
type ScreenshotRetranslatePayload struct {
	Session string         `json:"session"`
	From    model.Language `json:"from"`
	To      model.Language `json:"to"`
}
