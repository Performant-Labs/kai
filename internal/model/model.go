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
// after their base so language families stay adjacent (…, ES, ESMX, PT, PTPT, PTBR, RU).
// Recognized is a superset of selectable: see selectableExcluded.
// The selectable dropdowns lead with English, Spanish (Mexico), Portuguese (Portugal) (issue
// #165, principal, 2026-09-28); Chinese stays last (principal, 2026-09-26).
var allLanguages = []Language{Auto, EN, ES, ESMX, PT, PTPT, PTBR, JA, KO, FR, DE, RU, ZH}

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

// SameAs reports whether l and other are the same language for the "same source and target"
// rule (issue #80): the source text is then the result, with no engine call. It is deliberately
// stricter than Normalize equality: Normalize folds every dialect to its base, so pt-BR and pt-PT
// (or es-MX and es-419) would count as one language although the user named two concrete variants
// and an engine can translate between them.
//
// Two codes are the same language when their canonical forms are equal (case, "_" against "-" and
// the legacy zh-CN spelling do not matter), or when one is the bare base of the other in the alias
// table (es and es-MX, pt and pt-BR). Empty and auto name no language, so they are the same as
// nothing, themselves included. Two identical unrecognized codes are the same language; what an
// engine makes of them is the engine's business.
func (l Language) SameAs(other Language) bool {
	a, ok := l.canonical()
	if !ok {
		return false
	}
	b, ok := other.canonical()
	if !ok {
		return false
	}
	return a == b || a.Base() == b || b.Base() == a
}

// Covers reports whether l, read as a bare language code, stands for other's language (issue
// #80). It is the comparison for a language an engine DETECTED in the text: a detection is the
// bare language (es, zh, pt) and cannot name a dialect, so it never tells the target's dialect
// apart and must not fail to match for lack of one. l covers other when the two are the same
// language by SameAs, or when l is bare and other is one of its regional variants (es covers
// es-MX, es-419 and es-ES).
//
// It is not symmetric on purpose. A detection that does name a dialect (pt-BR) covers only that
// dialect, not pt-PT, so a dialect pair is left to the engine, as it is for a pinned source. And
// it is for detections only: a pinned source, bare or not, is decided by SameAs, where es against
// es-ES is not the same language.
func (l Language) Covers(other Language) bool {
	if l.SameAs(other) {
		return true
	}
	a, ok := l.canonical()
	if !ok || strings.Contains(string(a), "-") {
		return false
	}
	b, ok := other.canonical()
	if !ok {
		return false
	}
	primary, _, _ := strings.Cut(string(b), "-")
	return Language(primary) == a
}

// canonical returns the comparable form of a language code: the recognized language ParseLanguage
// maps it to, else the trimmed code lower-cased with "_" read as "-". ok is false for "" and auto,
// which name no language.
func (l Language) canonical() (Language, bool) {
	code := strings.TrimSpace(string(l))
	if code == "" || strings.EqualFold(code, string(Auto)) {
		return "", false
	}
	if n, ok := ParseLanguage(code); ok {
		return n, true
	}
	return Language(strings.ToLower(strings.ReplaceAll(code, "_", "-"))), true
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

// selectableOrDefault is the fixed default dialect a bare base coerces to (es → es-MX,
// pt → pt-BR), independent of the dropdowns' display order (allLanguages / SelectableLanguages):
// issue #165 reordered the dropdowns to lead with pt-PT, but persisted legacy "pt" values must
// keep resolving to the same dialect they always have, not silently switch with a display reorder.
var selectableOrDefault = map[Language]Language{ES: ESMX, PT: PTBR}

// SelectableOr returns l unless it is a recognized language that is not selectable (bare es /
// pt); that is replaced by its fixed default selectable variant (es → es-MX, pt → pt-BR, see
// selectableOrDefault), or by fallback when the family offers none. Codes outside the recognized
// set are returned unchanged and stay the engines' business, as before.
// It is the read-path coercion for persisted choices that predate the dialect variants.
func (l Language) SelectableOr(fallback Language) Language {
	if !selectableExcluded[l] {
		return l
	}
	if d, ok := selectableOrDefault[l]; ok {
		return d
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
	// RequestID names the request (issue #109). The frontend generates it before the call, because
	// a fast engine can emit its result before the binding call returns, so an id handed back by
	// the call would race the first event. It tags every event of the request and is what
	// CancelTranslate takes. Empty: the backend generates one.
	RequestID string `json:"request_id,omitempty"`
}

// Engine failure categories (TranslateResult.ErrorKind, issues #42 and #96). They live here, not
// in translate, because engine.HTTPError.Kind names one and engine cannot import translate. The
// string values are the wire format: the frontend maps each one to its own copy and falls back to
// the generic headline for a value it does not know. translate.ClassifyEngineError is the only
// place that picks a kind for an error.
const (
	ErrorKindNotConfigured = "not_configured" // The engine has no credentials set (no request was made)
	ErrorKindAuth          = "auth"           // The provider rejected the credentials (401/403, invalid API key)
	ErrorKindQuota         = "quota"          // The account is out of credits or quota (402)
	ErrorKindRateLimit     = "rate_limit"     // Too many requests (429 without a quota signal, a Google gtx block)
	ErrorKindUnavailable   = "unavailable"    // The provider is down (5xx)
	ErrorKindNetwork       = "network"        // Network/endpoint unreachable (timeout, DNS, connection refused, TLS)
	ErrorKindPair          = "pair"           // Language pair unavailable or unsupported
	ErrorKindTooLong       = "too_long"       // The text is over the provider's length limit (413/414)
	ErrorKindEngine        = "engine"         // Any other engine error (fallback)
)

// TranslateResult is a single translation result
type TranslateResult struct {
	Engine    string     `json:"engine"`               // Translation engine identifier
	From      Language   `json:"from"`                 // The actually detected source language
	To        Language   `json:"to"`                   // Target language (always the one requested)
	Text      string     `json:"text"`                 // Source text
	Result    string     `json:"result"`               // Translation
	Phonetic  string     `json:"phonetic"`             // Pronunciation/phonetics
	Dict      []DictItem `json:"dict"`                 // Dictionary detail entries
	FromOCR   bool       `json:"from_ocr"`             // Whether it came from OCR recognition
	Identity  bool       `json:"identity,omitempty"`   // Result is the source text, not a translation: source and target are the same language (issue #80; set by the translate service only, never by an engine)
	Error     string     `json:"error,omitempty"`      // Sanitized engine error on failure (issues #42, #96; empty on success)
	ErrorKind string     `json:"error_kind,omitempty"` // Engine failure category, one of the ErrorKind* values (issues #42, #96)
	RequestID string     `json:"request_id,omitempty"` // The request this result belongs to (issue #109); the frontend ignores results of any other request
	// Cancelled marks an engine the user cancelled (issue #109). It is a flag beside Error, not a
	// kind of it: a cancelled payload has no Error and no ErrorKind, and it is never a failure.
	// Result may hold a partial translation (the contract chunked translation, #84, fills).
	Cancelled bool `json:"cancelled,omitempty"`
	// DetectedFrom is the auto-detected language a translation was made from, the same qualified
	// value as From (issue #161; set by the translate service only, never by an engine, like
	// Identity). It is set when an auto request's engine detected a recognized language, and when
	// the service corrected a pinned source because the text was in another language. It is empty
	// otherwise: on an identity result, when a pin stood, and when nothing usable was detected.
	// The translate window shows it as a note.
	DetectedFrom Language `json:"detected_from,omitempty"`
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
	// RequestID is the request's id: the caller's TranslateRequest.RequestID echoed back, or the
	// one the backend generated when none was sent (issue #109).
	RequestID string `json:"request_id"`
	// Engines lists the engines actually started (never null). The request is over when each of
	// them has reported once (result, failure or cancel); the frontend settles from this list,
	// not from its own view of which engines are enabled (issue #109).
	Engines []string `json:"engines"`
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

// SourceSwitchRequest asks translate.Service.PlanSourceSwitch whether text that arrived should
// change the language pair (issue #200): the text, the pair the window shows now, and optionally a
// detection the caller already holds (the one a translation result carried, issue #161).
type SourceSwitchRequest struct {
	Text     string   `json:"text"`
	From     Language `json:"from"`
	To       Language `json:"to"`
	Detected Language `json:"detected"`
}

// SourceSwitch is the answer to a SourceSwitchRequest. When Switched, From is the language the text
// is in (a variant the dropdown offers) and To is the old source, which replaces the old target;
// otherwise the pair stays as it is and From / To are empty.
type SourceSwitch struct {
	Switched bool     `json:"switched"`
	From     Language `json:"from"`
	To       Language `json:"to"`
}

// ScreenshotResult is the full screenshot translate result, pushed to the screenshot
// window via EventScreenshotOCR.
type ScreenshotResult struct {
	Image        string            `json:"image"`        // Region screenshot PNG as a base64 data URL (frontend renders <img> directly)
	Text         string            `json:"text"`         // Source text recognized by OCR
	Translations []TranslateResult `json:"translations"` // Per-engine translations
	To           Language          `json:"to"`           // Requested target language
	Error        string            `json:"error"`        // Flow failure reason (when non-empty the frontend stops spinning and shows the error)
	// RequestID names this run of the screenshot flow (issue #109), on every push of the run. The
	// progress event is a broadcast that carries no window, so the screenshot window adopts its
	// request id from these pushes and ignores progress events of any other request.
	RequestID string `json:"request_id"`
}

// CorrectionRequest asks translate.Service.CorrectSource to correct the grammar and word choice of
// text that has just arrived (issue #208): the text, the source language the window shows, and
// optionally a detection the caller already holds (the one a #161 result carried).
type CorrectionRequest struct {
	Text     string   `json:"text"`
	From     Language `json:"from"`
	Detected Language `json:"detected"`
}

// TextChange is one place where a correction changed the text: Before is the words as they were
// written, After the words that replaced them. Either can carry a neighbouring word for context
// (a pure insertion is shown as "casa" -> "casa de"), never more.
type TextChange struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// CorrectionStatus says why a CorrectionRequest was answered the way it was. Only Corrected means
// the text changed; every other value is "translate the text as it came", and tells a test (and a
// log line) which rule decided.
type CorrectionStatus string

const (
	CorrectionCorrected   CorrectionStatus = "corrected"   // the text was changed
	CorrectionUnchanged   CorrectionStatus = "unchanged"   // the model found nothing to fix
	CorrectionOff         CorrectionStatus = "off"         // the setting is off
	CorrectionAutoSource  CorrectionStatus = "auto_source" // the source is Auto: no language to correct in
	CorrectionTooShort    CorrectionStatus = "too_short"   // fewer than 8 code points
	CorrectionTooLong     CorrectionStatus = "too_long"    // more than the model is asked to read in one go
	CorrectionUnavailable CorrectionStatus = "unavailable" // no provider, or it reports it cannot run
	CorrectionFailed      CorrectionStatus = "failed"      // the provider errored, refused or timed out
	CorrectionRejected    CorrectionStatus = "rejected"    // the output failed a guard
)

// Correction is the answer to a CorrectionRequest. When Corrected, Text is the corrected text the
// caller translates, Original the text as it arrived (kept so it can be restored in one click) and
// Changes what differs, computed in Go and never by the model. Otherwise Text is the text as it
// arrived and Changes is empty. Language is the language the text was corrected in (a variant the
// dropdowns offer), empty when nothing was attempted. Reason carries the provider's availability
// reason when Status is unavailable (the UI words it).
type Correction struct {
	Corrected bool             `json:"corrected"`
	Status    CorrectionStatus `json:"status"`
	Reason    string           `json:"reason"`
	Text      string           `json:"text"`
	Original  string           `json:"original"`
	Language  Language         `json:"language"`
	Changes   []TextChange     `json:"changes"`
}

// CorrectionAvailability tells the UI whether the correction can run at all on this machine, and
// when it cannot, why (Reason is an engine.CorrectionStatus value the UI words).
type CorrectionAvailability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}
