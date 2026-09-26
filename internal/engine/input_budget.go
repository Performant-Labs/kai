package engine

import (
	"fmt"
	"net/url"
	"unicode/utf8"
)

// Per-engine input budget (issue #83) — the largest chunk the chunker (#84) may hand one
// translator in a single call, stated in the unit that engine actually limits. This table is the
// single source of truth for that number: docs/engine-limits.md is a projection of it that a test
// checks (TestEngineLimitsDocListsEveryTranslator), and there is deliberately no second copy of a
// limit anywhere else.
//
// Limit is the raw ceiling: measured by the probe (internal/engine/enginelimits) or taken from the
// engine's own documentation. Max is what a chunker may actually send: Limit reduced by
// BudgetMarginPercent. The margin is applied to every engine, measured ones included, and is one
// constant so it stays visible and testable instead of being baked into a literal.

// BudgetUnit is what an engine's input limit counts.
type BudgetUnit string

const (
	// UnitRunes counts characters (Unicode code points): utf8.RuneCountInString.
	UnitRunes BudgetUnit = "runes"
	// UnitUTF8Bytes counts the bytes of the UTF-8 text: len(text).
	UnitUTF8Bytes BudgetUnit = "utf8_bytes"
	// UnitQueryEscapedBytes counts the bytes after URL query escaping: len(url.QueryEscape(text)),
	// which is also what url.Values.Encode produces for a form value. It is the unit of an engine
	// whose text travels in a URL or a form body against a byte limit (a CJK character costs 9).
	UnitQueryEscapedBytes BudgetUnit = "query_escaped_bytes"
)

// BudgetSource says how far a Limit can be trusted.
type BudgetSource string

const (
	// SourceMeasured: the largest size the engine accepted in a probe run on the recorded host.
	SourceMeasured BudgetSource = "measured"
	// SourceDocumented: the limit the engine's own documentation states; not yet checked against
	// the live service.
	SourceDocumented BudgetSource = "documented"
	// SourceProvisional: a stand-in, because the engine documents no limit or the figure is
	// derived; the follow-up issue replaces it.
	SourceProvisional BudgetSource = "provisional"
)

// BudgetMarginPercent is the share of Limit a chunker may fill. The remaining 20% is the buffer
// for what a raw ceiling does not model (other request fields, run-to-run latency).
const BudgetMarginPercent = 80

// Budget is one translation engine's input budget.
type Budget struct {
	Unit   BudgetUnit
	Limit  int          // the raw ceiling, in Unit
	Source BudgetSource // how far Limit can be trusted
	// FollowUp is the issue that measures (or re-measures) Limit; 0 only when Source is
	// SourceMeasured.
	FollowUp int
}

// Max is the budget a chunker works to: Limit reduced by BudgetMarginPercent, in Unit.
func (b Budget) Max() int { return b.Limit * BudgetMarginPercent / 100 }

// Measure returns the size of text in the budget's unit. An unrecognized unit is a programmer
// error (a typo in the table, or the zero Budget of a failed lookup) and panics: falling back to
// some unit would make Fits answer true for text the engine rejects, and a chunker would never
// split.
func (b Budget) Measure(text string) int {
	switch b.Unit {
	case UnitRunes:
		return utf8.RuneCountInString(text)
	case UnitUTF8Bytes:
		return len(text)
	case UnitQueryEscapedBytes:
		return len(url.QueryEscape(text))
	default:
		panic(fmt.Sprintf("engine: unknown budget unit %q", b.Unit))
	}
}

// Fits reports whether text is within Max in the budget's unit.
func (b Budget) Fits(text string) bool { return b.Measure(text) <= b.Max() }

// inputBudgets maps translation-engine name → budget. OCR engines have no entry (they do not
// translate). Sources and reasoning per row; the same rows, with their links, are in
// docs/engine-limits.md.
var inputBudgets = map[string]Budget{
	// apple — macOS Translation.framework. MEASURED by the probe (internal/engine/enginelimits) on
	// macOS 27.0 (build 26A428), Apple M1 Max, 2026-09-26; the run is recorded in
	// docs/engine-limits.md "Probe run". Limit is the smaller of the two searches, which is the
	// CJK one: zh-Hans -> en passed 650 runes and failed at 700, en -> zh-Hans passed 2250 and
	// failed at 2300. What binds is the bridge's own 20 s wait (apple_translate.swift), not the
	// framework and not the 64 KiB output buffer, so the number depends on the chip and moves if
	// that wait changes. Re-run the probe on a slower host before trusting it there.
	"apple": {Unit: UnitRunes, Limit: 650, Source: SourceMeasured},

	// google — the key-free gtx endpoint Kai calls (google.go) is undocumented, so no limit is
	// published for it. 5000 is Google's recommended maximum for Cloud Translation, 5K characters
	// (code points), https://docs.cloud.google.com/translate/quotas, applied here to query-escaped
	// bytes. The text travels in the GET URL and escaped bytes are never fewer than code points (a
	// CJK character is 9), so this is stricter than the recommendation on purpose. #87 replaces it.
	"google": {Unit: UnitQueryEscapedBytes, Limit: 5000, Source: SourceProvisional, FollowUp: 87},

	// deepl — "within the request size limit (128KiB)", the whole request body,
	// https://developers.deepl.com/api-reference/translate/request-translation. The text is a form field, so it
	// is measured query-escaped; the other form fields are under 60 bytes, which the margin covers.
	"deepl": {Unit: UnitQueryEscapedBytes, Limit: 131072, Source: SourceDocumented, FollowUp: 88},

	// baidu — q should stay within 6000 bytes per request, about 2000 Chinese characters,
	// https://fanyi-api.baidu.com/doc/21 (the page renders by JavaScript, so the figure was read
	// from a search of it on 2026-09-26, not from the rendered page).
	"baidu": {Unit: UnitUTF8Bytes, Limit: 6000, Source: SourceDocumented, FollowUp: 89},

	// tencent — SourceText: the text length per request must be below 2000, with no unit stated
	// (SDK doc comment, https://pkg.go.dev/github.com/tencentyun/tencentcloud-sdk-go/tencentcloud/tmt/v20180321).
	// Read as characters. If #90 finds it counts bytes, this budget is too large for CJK text by up to 3x.
	"tencent": {Unit: UnitRunes, Limit: 2000, Source: SourceDocumented, FollowUp: 90},

	// youdao — at most 5000 characters per query, https://ai.youdao.com/DOCSIRMA/html/trans/api/wbfy/index.html.
	// Only the signature input is cut to 20 characters (youdao.go); the full text is sent.
	"youdao": {Unit: UnitRunes, Limit: 5000, Source: SourceDocumented, FollowUp: 91},

	// openai / anthropic / gemini — the input context is far larger than any chunk, so the
	// binding limit is the OUTPUT cap. Kai sets 8192 tokens for anthropic (MaxTokens) and gemini
	// (MaxOutputTokens) and none for openai (the model default; model and base URL are
	// user-configurable), so all three share the 8192-token basis. 4096 runes assumes at most 2
	// output tokens per character: an assumption, not a measurement, and conservative for Latin
	// text. None of the three checks the stop / finish reason, so an output cut at the cap comes
	// back as a successful, silently truncated translation. #92, #93 and #94 measure it.
	"openai":    {Unit: UnitRunes, Limit: 4096, Source: SourceProvisional, FollowUp: 92},
	"anthropic": {Unit: UnitRunes, Limit: 4096, Source: SourceProvisional, FollowUp: 93},
	"gemini":    {Unit: UnitRunes, Limit: 4096, Source: SourceProvisional, FollowUp: 94},
}

// InputBudget returns the engine's input budget. ok=false means the engine has none: an OCR
// engine (it does not translate) or an unknown name. Every translator in KnownEngines has one;
// TestInputBudgetForEveryTranslator fails when a new translator is added without a row.
func InputBudget(engineName string) (Budget, bool) {
	b, ok := inputBudgets[engineName]
	return b, ok
}
