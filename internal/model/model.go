package model

// Language is a language code (ISO 639-1 or engine-specific)
type Language string

// Locale is the UI language — a completely different concern from the translation Language;
// it uses its own type.
// Values are BCP 47 region codes (zh-CN/en-US/auto), used only for backend i18n and the
// frontend UI language switch.
// Never mix with or convert to/from the translation Language (zh/en/...).
type Locale string

const (
	LocaleAuto Locale = "auto"  // UI language follows the system
	LocaleZHCN Locale = "zh-CN" // UI in Chinese
	LocaleENUS Locale = "en-US" // UI in English
)

// Theme is the UI theme, used only for appearance switching; values are auto/light/dark.
type Theme string

const (
	ThemeAuto  Theme = "auto"  // Follow the system
	ThemeLight Theme = "light" // Light
	ThemeDark  Theme = "dark"  // Dark
)

// Translation language codes (Translate Language): used for translation requests/results,
// engine calls, and history records.
// Values are the short codes translation engines expect (e.g. zh/en/ja), completely
// unrelated to the UI language — never mix with the UI locale below.
const (
	Auto Language = "auto" // Auto-detect source language
	ZH   Language = "zh"   // Chinese
	EN   Language = "en"   // English
	JA   Language = "ja"   // Japanese
	KO   Language = "ko"   // Korean
	FR   Language = "fr"   // French
	DE   Language = "de"   // German
	ES   Language = "es"   // Spanish
	RU   Language = "ru"   // Russian
)

// allLanguages lists all supported languages (including Auto); the order is the frontend
// dropdown display order.
var allLanguages = []Language{Auto, ZH, EN, JA, KO, FR, DE, ES, RU}

// AllLanguages returns the slice of all supported language constants (including Auto).
// Lets the config layer derive dropdown options, avoiding hardcoded language-code lists
// everywhere.
func AllLanguages() []Language {
	out := make([]Language, len(allLanguages))
	copy(out, allLanguages)
	return out
}

// TranslateRequest is the unified translation request
type TranslateRequest struct {
	Text       string   `json:"text"`   // Source text to translate
	From       Language `json:"from"`   // Source language (auto = auto-detect)
	To         Language `json:"to"`     // Target language
	EngineName string   `json:"engine"` // Translation engine identifier to use
}

// TranslateResult is a single translation result
type TranslateResult struct {
	Engine    string     `json:"engine"`               // Translation engine identifier
	From      Language   `json:"from"`                 // The actually detected source language
	To        Language   `json:"to"`                   // Target language
	Text      string     `json:"text"`                 // Source text
	Result    string     `json:"result"`               // Translation
	Phonetic  string     `json:"phonetic"`             // Pronunciation/phonetics
	Dict      []DictItem `json:"dict"`                 // Dictionary detail entries
	FromOCR   bool       `json:"from_ocr"`             // Whether it came from OCR recognition
	Error     string     `json:"error,omitempty"`      // Raw engine error on failure (issue #42; empty on success)
	ErrorKind string     `json:"error_kind,omitempty"` // Engine failure category (pair/network/auth/engine, issue #42)
}

// DictItem is a dictionary entry
type DictItem struct {
	Word    string `json:"word"`    // Word
	Pos     string `json:"pos"`     // Part of speech (e.g. n./v.)
	Explain string `json:"explain"` // Definition
}

// OcrRequest is an OCR request
type OcrRequest struct {
	ImageData []byte `json:"-"`      // Image binary data (not serialized)
	Engine    string `json:"engine"` // OCR engine identifier to use
	// CorrectText is Vision's usesLanguageCorrection (language correction).
	// true = on (more accurate, the default); false = off (faster, lower chance of sporadic
	// hangs, slightly less accurate).
	// A pointer so "unset" can be distinguished from "false"; when unset, engines use the
	// default true.
	CorrectText *bool `json:"correct_text,omitempty"`
	// TimeoutSec is the Vision OCR timeout in seconds. <=0 lets engines use their own
	// defaults (vision defaults to 60s).
	TimeoutSec int `json:"timeout_sec,omitempty"`
	// RetryCount is the Vision OCR failure-fallback retry count (for transient rejections
	// like CRImageReaderError).
	// <=0 lets engines use the default 2 (only meaningful for vision).
	RetryCount int `json:"retry_count,omitempty"`
}

// OcrResult is an OCR result
type OcrResult struct {
	Engine  string      `json:"engine"`  // OCR engine identifier
	Text    string      `json:"text"`    // All recognized text
	Regions []OcrRegion `json:"regions"` // Per-region details
}

// OcrRegion is a single text region
type OcrRegion struct {
	Text string  `json:"text"` // Region text
	Conf float64 `json:"conf"` // Recognition confidence
	Box  []int   `json:"box"`  // Region bounding box [x1,y1,x2,y2]
}

// TranslateMultiResult confirms the start of a multi-engine parallel translation; Count is
// the number of engines started.
// Actual results stream to the frontend one by one via the EventTranslateResult event.
type TranslateMultiResult struct {
	Count   int               `json:"count"`   // Number of engines started
	Results []TranslateResult `json:"results"` // Initial result set (including engine placeholders)
}

// HistoryItem is a translation history entry
type HistoryItem struct {
	ID        int64    `json:"id"`         // History record auto-increment primary key
	Text      string   `json:"text"`       // Source text
	Result    string   `json:"result"`     // Translation
	From      Language `json:"from"`       // Source language
	To        Language `json:"to"`         // Target language
	Engine    string   `json:"engine"`     // Engine identifier used
	FromOCR   bool     `json:"from_ocr"`   // Whether it came from OCR recognition
	CreatedAt int64    `json:"created_at"` // Creation time (millisecond timestamp)
}

// ScreenshotResult is the full screenshot translate result, pushed to the screenshot
// window via EventScreenshotOCR.
type ScreenshotResult struct {
	Image        string            `json:"image"`        // Region screenshot PNG as a base64 data URL (frontend renders <img> directly)
	Text         string            `json:"text"`         // Source text recognized by OCR
	Translations []TranslateResult `json:"translations"` // Per-engine translations
	To           Language          `json:"to"`           // Target language
	Error        string            `json:"error"`        // Flow failure reason (when non-empty the frontend stops spinning and shows the error)
}
