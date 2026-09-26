// App event name constants to prevent typos (aligned with the Go-side definitions in internal/events/events.go).

// EventWindowShow: frontend asks to bring up a window, payload: 'settings' | 'main'
export const EventWindowShow = 'kai:window:show';

// EventWindowClosing: broadcast when a window's close button (title-bar X) is clicked; each window
// reacts to its own closing as needed (e.g. the screenshot window clears its image and results),
// then closes. The translate window keeps its text and results across a close (issue #81), so it
// clears nothing. payload: optional window name.
export const EventWindowClosing = 'kai:window:closing';

// EventLocaleChanged: broadcast after the UI language changes, payload: LocaleChangedPayload
export const EventLocaleChanged = 'kai:locale:changed';
/** Screenshot-translation result delivery (pushed after the backend's region-screenshot → OCR → translate pipeline finishes): payload is ScreenshotResult */
export const EventScreenshotOCR = 'kai:screenshot:ocr';
/** Frontend requests a new screenshot (the "re-screenshot" button clicked): the backend triggers a new region-screenshot flow */
export const EventScreenshotRecapture = 'kai:screenshot:recapture';
/** Triggered after the frontend changes language: reuses the last OCR'd source text, skipping screenshot/OCR, and retranslates with the new language. payload: ScreenshotRetranslatePayload */
export const EventScreenshotRetranslate = 'kai:screenshot:retranslate';

// Session identifiers for the screenshot/OCR cache: distinguish entry points so they
// don't overwrite each other (aligned with internal/events/events.go).
/** Screenshot-translation window (hotkey/menu/re-screenshot button all target ScreenshotWindow) */
export const ScreenshotSessionScreenshot = 'screenshot';
/** Screenshot OCR within the input-translation page (reserved, isolated from the screenshot-translation window) */
export const ScreenshotSessionInput = 'input';

// EventThemeChanged: theme change broadcast, payload: ThemeChangedPayload
export const EventThemeChanged = 'kai:theme:changed';

// EventHotkeysChanged: broadcast after hotkeys are re-registered, payload: string[]
export const EventHotkeysChanged = 'kai:hotkeys:changed';

// EventInputFill: fills the selection back into the main window's input box, payload: string (selected text)
export const EventInputFill = 'kai:input:fill';

// EventTranslateResult: multi-engine translation results arrive one by one, payload: TranslateResult.
// It is the one terminal event of an engine (a translation, a failure or a cancel), tagged with the
// request's request_id (issue #109); results of any other request are ignored.
export const EventTranslateResult = 'kai:translate:result';

// EventTranslateProgress: the non-terminal facts of a running engine (issue #109), payload:
// TranslateProgressPayload. It is a broadcast to every window, so each window filters by the
// request_id it is waiting on.
export const EventTranslateProgress = 'kai:translate:progress';

// EventEnginesChanged: broadcast after translation engines are added/removed or enabled/disabled in
// settings, telling the translate window and others to refresh their engine lists.
// No payload (pure frontend window-to-window notification; each window re-fetches GetEngines itself).
export const EventEnginesChanged = 'kai:engines:changed';

// EventAutoClipboardChanged: broadcast after the translate window's "auto-read clipboard" toggle changes.
// payload: boolean (on=true / off=false). The settings page uses this to disable/restore the two copy-hotkey
// switches in real time.
export const EventAutoClipboardChanged = 'kai:auto-clipboard:changed';

// Event payload type definitions (aligned with the Go-side structs in internal/events/events.go)

// LocaleChangedPayload: language-change event arguments.
// mode is the user-configured mode (from constants/lang's Lang: auto/zh/en);
// language is the actually effective language (zh/en; derived from the system locale when auto).
export interface LocaleChangedPayload {
  mode: string; // Lang: auto | zh | en
  language: string; // zh | en
}

// ThemeChangedPayload: theme-change event arguments.
// mode is the user-configured mode (from constants/theme's ThemeMode: auto/light/dark);
// theme is the actual system appearance (dark/light, derived from Env.IsDarkMode()).
export interface ThemeChangedPayload {
  mode: string; // ThemeMode: auto | light | dark
  theme: string; // dark | light
}

// TranslateProgressPayload: EventTranslateProgress arguments (aligned with internal/events/events.go).
// phase 'started' is sent once per engine when its call begins, before any result; 'chunk' reports
// done of total parts finished (chunked translation, #84). started_at_ms is the backend clock and
// is informational only: elapsed time is measured from the moment this window received the event.
export interface TranslateProgressPayload {
  request_id: string;
  engine: string;
  phase: string; // 'started' | 'chunk'
  started_at_ms: number;
  done: number;
  total: number;
}

// ScreenshotRetranslatePayload: screenshot-translation change-language-and-retranslate event arguments.
// session identifies the cache source (ScreenshotSessionScreenshot / ScreenshotSessionInput);
// the backend uses it to pick up the latest OCR'd source text for that entry point, preventing
// cross-talk between different entry points.
// from/to is the source/target translation language pair (from may be Auto); the backend reuses the
// last OCR'd source text and retranslates.
export interface ScreenshotRetranslatePayload {
  session: string; // ScreenshotSessionScreenshot | ScreenshotSessionInput
  from: string; // TranslateLang: auto | zh | en | ...
  to: string; // TranslateLang
}
