package events

import "cnb.cool/dtapp/kai/internal/model"

// App event name constants, preventing typos.
const (
	// EventWindowShow: the frontend summons a window, payload: string ("settings" | "main")
	EventWindowShow = "kai:window:show"

	// EventWindowClosing is broadcast when a window close (native red X / custom title-bar
	// close) is triggered; each window reacts to its own close as needed (e.g. the screenshot
	// window clears its image and results; the translate window keeps its text and results
	// across a close since issue #81). payload: the window name (model.WindowTranslate
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

	// EventFontSizeChanged is broadcast after the text size changes (issue #195), payload: int,
	// the new size in percent (one of settings.FontSizeSteps). Every window applies it live.
	EventFontSizeChanged = "kai:fontsize:changed"

	// EventHotkeysChanged is broadcast after hotkeys re-register, payload: []string
	EventHotkeysChanged = "kai:hotkeys:changed"

	// EventInputFill backfills the main window input with the selection, payload: string
	// (the selected text)
	EventInputFill = "kai:input:fill"

	// EventTranslateResult: multi-engine translations return results one by one, payload:
	// model.TranslateResult. It is the one terminal event of an engine: a translation, a
	// failure (Error set) or a cancel (Cancelled set), tagged with the request's RequestID
	// (issue #109). Exactly one is emitted per started engine, except for a superseded request,
	// which emits nothing.
	EventTranslateResult = "kai:translate:result"

	// EventTranslateProgress carries the non-terminal facts of a running engine (issue #109):
	// it started, or (chunked translation, #84) some parts of it are done. payload:
	// TranslateProgressPayload. Aligned with the frontend's EventTranslateProgress in
	// frontend/src/utils/events.ts.
	EventTranslateProgress = "kai:translate:progress"

	// EventWindowScreenshot: a hotkey triggered screenshot translate; after the backend
	// persists the region screenshot it summons the screenshot window to receive results.
	EventWindowScreenshot = "kai:window:screenshot"

	// EventScreenshotOCR: the screenshot translate flow progressed — after the backend
	// captures the region→OCR→translates, it delivers results to the screenshot window.
	// payload: ScreenshotResult{Image, Text, Translations, To, RequestID}; every push of one run
	// carries the same RequestID (issue #109).
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

	// EventDoubleCopyPermissionMissing is emitted when "translate on double Cmd+C" is switched on
	// but macOS Input Monitoring is not granted to Kai, so the listener cannot start (issue #199).
	// No payload. The translate window shows a toast that says which pane to open. Aligned with
	// the frontend's EventDoubleCopyPermissionMissing in frontend/src/utils/events.ts.
	EventDoubleCopyPermissionMissing = "kai:doublecopy:permission-missing"

	// EventAccessibilityMissing is emitted when the hotkey or tray copy-key capture finds the macOS
	// Accessibility permission missing, so the simulated Cmd+C was not posted (issue #194). No
	// payload. The translate window shows a toast naming what to enable and where. It is throttled
	// on the Go side, so it is not repeated on every keypress. Aligned with the frontend's
	// EventAccessibilityMissing in frontend/src/utils/events.ts.
	EventAccessibilityMissing = "kai:accessibility:missing"

	// EventCopyKeyFailed is emitted when the copy-key branch of TriggerInput (issue #175 item
	// 5) simulates the copy key but pollClipboardText never sees the clipboard change —
	// CopySelection returns "". No payload.
	//
	// Before this event existed, that failure was silent on the frontend: the translate
	// window still Show()/Focus()es (so the user sees it come to the front), but with nothing
	// new to fill, EventInputFill never fires and the window keeps showing whatever text was
	// already in it (issue #81's retained-session design: the input box is only replaced by
	// Clear, a new EventInputFill or a new translate — never cleared just because the window
	// was hidden and reshown). A user who doesn't notice the input didn't change believes the
	// old text IS the new selection — "translates the wrong text with no indication anything
	// is wrong" is exactly the bug report. This event lets the frontend show a toast instead,
	// so a failed capture is visibly a failure, not indistinguishable from a stale success.
	EventCopyKeyFailed = "kai:copykey:failed"
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

// Phases of a TranslateProgressPayload.
const (
	// ProgressPhaseStarted is emitted once per engine when its call begins, before any result.
	ProgressPhaseStarted = "started"
	// ProgressPhaseChunk reports that Done of Total parts of the engine's work are finished. #109
	// defines and tests it; the chunked translation (#84) is what sends it.
	ProgressPhaseChunk = "chunk"
)

// TranslateProgressPayload carries the EventTranslateProgress event (issue #109).
// RequestID and Engine say whose progress it is. StartedAtMs is the backend clock (epoch
// milliseconds) at which the engine's call began: informational only, because the frontend
// times an engine from the moment it received the started event and never compares clocks. Done
// and Total are only meaningful for ProgressPhaseChunk.
type TranslateProgressPayload struct {
	RequestID   string `json:"request_id"`
	Engine      string `json:"engine"`
	Phase       string `json:"phase"` // ProgressPhaseStarted | ProgressPhaseChunk
	StartedAtMs int64  `json:"started_at_ms"`
	Done        int    `json:"done"`
	Total       int    `json:"total"`
}

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
