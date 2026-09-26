package model

import "strings"

// Language is a language code (ISO 639-1, or a BCP 47 dialect tag such as es-MX)
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
// Values are the short codes translation engines expect (e.g. zh/en/ja) or, for dialect
// variants, BCP 47 tags (es-MX/pt-BR/pt-PT), completely unrelated to the UI language — never
// mix with the UI locale above.
const (
	Auto Language = "auto"  // Auto-detect source language
	ZH   Language = "zh"    // Chinese
	EN   Language = "en"    // English
	JA   Language = "ja"    // Japanese
	KO   Language = "ko"    // Korean
	FR   Language = "fr"    // French
	DE   Language = "de"    // German
	ES   Language = "es"    // Spanish (recognized base of the Spanish dialects; not selectable)
	ESMX Language = "es-MX" // Spanish (Mexico)
	PT   Language = "pt"    // Portuguese (recognized base of the Portuguese dialects; not selectable)
	PTBR Language = "pt-BR" // Portuguese (Brazil)
	PTPT Language = "pt-PT" // Portuguese (Portugal)
	RU   Language = "ru"    // Russian
)

// allLanguages is the RECOGNIZED set (including Auto): every value the app understands, i.e.
// what engines and detection may emit and what has a display name. Dialect variants sit right
// after their base so language families stay adjacent (…, ES, ESMX, PT, PTBR, PTPT, RU).
// Recognized is a superset of selectable: see selectableExcluded.
var allLanguages = []Language{Auto, ZH, EN, JA, KO, FR, DE, ES, ESMX, PT, PTBR, PTPT, RU}

// selectableExcluded lists recognized languages that are NOT offered in the language
// dropdowns: the bare bases of a dialect family. Detection emits them on every auto send and
// they anchor alias normalization (Language.Base), but the user picks a concrete dialect.
var selectableExcluded = map[Language]bool{ES: true, PT: true}

// languageBase is the alias table: dialect → base language. Keys are lower-cased BCP 47 tags
// (Language.Base matches case-insensitively). es-419 (Latin American Spanish) is an alias only:
// it is deliberately not a recognized Language of its own.
var languageBase = map[string]Language{
	"es-mx":  ES,
	"es-419": ES,
	"pt-br":  PT,
	"pt-pt":  PT,
}

// Base returns the base language of a dialect variant (es-MX/es-419 → es, pt-BR/pt-PT → pt);
// every other language, including the bases themselves, maps to itself.
//
// The alias serves the SOURCE side only: engine source codes and detection normalization, so an
// engine lacking a dialect still accepts text written in it. It must never be used to degrade a
// TARGET — an engine without a dialect target has to refuse it, not quietly serve the base.
func (l Language) Base() Language {
	if b, ok := languageBase[strings.ToLower(strings.ReplaceAll(string(l), "_", "-"))]; ok {
		return b
	}
	return l
}

// Normalize returns the canonical base language a raw code stands for: a dialect alias folds to
// its base (Base: es-MX / es-419 → es) and a recognized spelling takes its canonical form
// (ParseLanguage: ES → es, zh_CN → zh), so every spelling of one language compares and looks up
// alike. A code that is not recognized, auto included, comes back as it was (after Base), so an
// unknown detection is still shown as reported.
//
// It is the "normalized first" step of qualifying a detected language (issue #53): the alias
// table (Base) and the canonical-code table (ParseLanguage) stay the only two places that know
// how to canonicalize a code; callers compose them here instead of re-deriving either.
func (l Language) Normalize() Language {
	if n, ok := ParseLanguage(string(l.Base())); ok {
		return n
	}
	return l.Base()
}

// ParseLanguage maps a raw language code onto a recognized language: recognized codes match
// case-insensitively with "_" read as "-" (pt-br → pt-BR, so region subtags survive the round
// trip) and the legacy Chinese spellings zh-CN / zh_CN fold onto zh. ok=false: not a recognized
// language — and never true for auto, which is a sentinel, not a language.
//
// It is the single canonical-code parser (issue #53 moved it here from the engine package, where
// it was the unexported resolveLanguage): engines, the translate service and the variant
// preference store all share it rather than growing a second case-folding loop.
func ParseLanguage(code string) (Language, bool) {
	c := strings.ReplaceAll(strings.TrimSpace(code), "_", "-")
	if strings.EqualFold(c, "zh-CN") {
		return ZH, true
	}
	for _, l := range allLanguages {
		if l != Auto && strings.EqualFold(string(l), c) {
			return l, true
		}
	}
	return "", false
}

// SelectableOr returns l unless it is a recognized language that is not selectable (bare es /
// pt); that is replaced by the first selectable variant of its family (es → es-MX,
// pt → pt-BR), or by fallback when the family offers none. Codes outside the recognized set are
// returned unchanged and stay the engines' business, as before.
// It is the read-path coercion for persisted choices that predate the dialect variants.
func (l Language) SelectableOr(fallback Language) Language {
	if !selectableExcluded[l] {
		return l
	}
	for _, s := range SelectableLanguages() {
		if s.Base() == l {
			return s
		}
	}
	return fallback
}

// AllLanguages returns the RECOGNIZED languages (including Auto), in display order — a
// superset of SelectableLanguages. Use it wherever a value must be understood (detection,
// alias bases, display names, engine capability decisions); use SelectableLanguages for
// anything a user picks.
func AllLanguages() []Language {
	out := make([]Language, len(allLanguages))
	copy(out, allLanguages)
	return out
}

// SelectableLanguages returns the languages offered in the source/target dropdowns (including
// Auto), in the same order as AllLanguages minus the non-selectable bases (bare es / pt). It
// lets the config layer derive dropdown options, avoiding hardcoded language-code lists
// everywhere.
func SelectableLanguages() []Language {
	out := make([]Language, 0, len(allLanguages))
	for _, l := range allLanguages {
		if !selectableExcluded[l] {
			out = append(out, l)
		}
	}
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
