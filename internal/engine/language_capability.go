package engine

import (
	"errors"
	"strings"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// Per-engine language capability registry (issue #52) — the single source of truth for which
// recognized languages each translation engine can translate INTO, and for the request code the
// engine expects. Every engine's mapping function (googleLang, baiduLang, ...) is a thin lookup
// into it; there is deliberately no second, independent code table anywhere else.
//
// Two sides, two rules:
//   - Source side: every engine accepts every recognized language. A dialect (es-MX, pt-BR,
//     pt-PT, ...) is aliased to its base language (model.Language.Base) and resolved with the
//     base's code, so an engine without the dialect still takes text written in it.
//   - Target side: exact match only. A dialect target the engine has no entry for is refused
//     (see targetCode) — never quietly served as its base language. The frontend reads the same
//     decisions through SupportedTargets and disables what an engine cannot do.
//
// model.Auto is a source-side sentinel, not a language; each engine keeps its own way of saying
// "auto-detect" and Auto is not recorded here.

// LanguageCapability is one translation engine's decision for one recognized language.
type LanguageCapability struct {
	// Supported reports whether the engine can translate INTO exactly this language (a dialect is
	// never satisfied by its base language). It says nothing about the source side, which
	// always resolves through model.Language.Base.
	Supported bool
	// Code is the engine's request code for the language; empty when !Supported. LLM engines
	// have no wire code: theirs is the model.Language value itself, which names the language in
	// the prompt through the "lang.<code>" i18n key (see srcName / dstName).
	Code string
}

// yes records a supported language with the engine's request code; no records a deliberate
// "not supported" decision (a missing entry is NOT a decision — the exhaustiveness test in CI
// fails on it, which is what forces a capability decision for every new language).
func yes(code string) LanguageCapability { return LanguageCapability{Supported: true, Code: code} }

var no = LanguageCapability{}

// languageRegistry maps translation-engine name → language → decision. OCR engines have no
// entry (they do not translate).
var languageRegistry = map[string]map[model.Language]LanguageCapability{
	// apple — macOS Translation.framework.
	//
	// The three dialects are supported: verified on macOS 27.0 (build 26A428, Translation
	// framework), where LanguageAvailability.supportedLanguages lists es-MX, pt (= pt-Latn-BR) and
	// pt-PT, and TranslationSession(installedSource: en, target: …) returned distinct dialect
	// output for each ("My computer is on the bus", "mobile phone"):
	//
	//	es-MX → "Mi computadora …  teléfonos celulares"   es → "Mi ordenador …  teléfonos móviles"
	//	pt-BR → "Meu computador … ônibus … celulares"      pt-PT → "O meu computador … autocarro … telemóveis"
	//
	// (bare es is Castilian on Apple's side and pt is Brazilian; es-419 is resolved by the
	// framework to es-MX). The set of dialects is an OS-release property, verified here for
	// macOS 27 only: a release that lacks one makes the framework fail the pair, which the
	// per-engine failure reporting surfaces (translate.ErrorKindPair).
	"apple": {
		model.ZH: yes("zh-Hans"), model.EN: yes("en"), model.JA: yes("ja"), model.KO: yes("ko"),
		model.FR: yes("fr"), model.DE: yes("de"), model.RU: yes("ru"),
		model.ES: yes("es"), model.ESMX: yes("es-MX"),
		model.PT: yes("pt"), model.PTBR: yes("pt-BR"), model.PTPT: yes("pt-PT"),
	},

	// google — the key-free gtx web endpoint. Verified live on 2026-09-25 (06:14–06:15 MDT):
	// GET /translate_a/single?client=gtx&sl=en&tl=<code>&dt=t, HTTP 200 for every code, on "My
	// computer is on the bus. I ate breakfast and then took the train to my mobile phone shop." and
	// on "The car, the juice, the cell phone, the computer, the strawberry and the popcorn.":
	//
	//	pt-PT → European: "autocarro … pequeno-almoço … comboio … telemóveis" (its own
	//	        pt_pt-PT rewriter model in the response) — a distinct dialect → supported
	//	pt-BR → byte-identical to pt: Brazilian, "ônibus … café da manhã … trem … celulares" — the
	//	        dialect that was asked for → supported
	//	es-MX → byte-identical to es, es-419 and es-ES (same response checksum for all four:
	//	        "computadora … celular … jugo"): gtx accepts the tag and ignores the region, so
	//	        it serves generic Spanish for any Spanish dialect. Principal ruling 2026-09-25:
	//	        serve it, sending the request code "es" (the default Spanish), so a google-only
	//	        setup keeps a Spanish target. The dialect is not honored; that is the known cost
	//
	// (An earlier probe at 05:53 MDT was answered with HTTP 429 and the automated-queries page,
	// even for a control request; that cleared on its own and nothing was bypassed.) The source
	// side is unaffected: variants are sent as their base code. Bare es / pt are not selectable,
	// so this entry is what gives a google-only setup (the Windows/Linux default) a Spanish
	// target; engines that honor es-MX (apple on macOS 27, an LLM) return the real dialect.
	"google": {
		model.ZH: yes("zh-CN"), model.EN: yes("en"), model.JA: yes("ja"), model.KO: yes("ko"),
		model.FR: yes("fr"), model.DE: yes("de"), model.RU: yes("ru"),
		model.ES: yes("es"), model.ESMX: yes("es"),
		model.PT: yes("pt"), model.PTBR: yes("pt-BR"), model.PTPT: yes("pt-PT"),
	},

	// deepl — DeepL API. Target codes per DeepL's supported-languages reference (fetched
	// 2026-09-25): ES, ES-419 (Latin American Spanish), PT-BR, PT-PT; there is no ES-MX. es-MX is
	// refused rather than substituted by ES-419, which is a broader dialect (same exact-match rule
	// as every engine). The bare PT below is the source-side code; PT-BR / PT-PT are the target
	// codes. Not exercised live (needs an API key).
	"deepl": {
		model.ZH: yes("ZH"), model.EN: yes("EN"), model.JA: yes("JA"), model.KO: yes("KO"),
		model.FR: yes("FR"), model.DE: yes("DE"), model.RU: yes("RU"),
		model.ES: yes("ES"), model.ESMX: no,
		model.PT: yes("PT"), model.PTBR: yes("PT-BR"), model.PTPT: yes("PT-PT"),
	},

	// LLM engines: the target is stated by name in the prompt, so every recognized language,
	// dialects included, is supported.
	"openai":    llmLanguages,
	"anthropic": llmLanguages,
	"gemini":    llmLanguages,

	// baidu / tencent / youdao: their language lists carry the bare Spanish and Portuguese only;
	// the dialects are not offered as targets.
	"baidu": {
		model.ZH: yes("zh"), model.EN: yes("en"), model.JA: yes("jp"), model.KO: yes("kor"),
		model.FR: yes("fra"), model.DE: yes("de"), model.RU: yes("ru"),
		model.ES: yes("spa"), model.ESMX: no,
		model.PT: yes("pt"), model.PTBR: no, model.PTPT: no,
	},
	"tencent": {
		model.ZH: yes("zh"), model.EN: yes("en"), model.JA: yes("ja"), model.KO: yes("ko"),
		model.FR: yes("fr"), model.DE: yes("de"), model.RU: yes("ru"),
		model.ES: yes("es"), model.ESMX: no,
		model.PT: yes("pt"), model.PTBR: no, model.PTPT: no,
	},
	"youdao": {
		model.ZH: yes("zh-CHS"), model.EN: yes("en"), model.JA: yes("ja"), model.KO: yes("ko"),
		model.FR: yes("fr"), model.DE: yes("de"), model.RU: yes("ru"),
		model.ES: yes("es"), model.ESMX: no,
		model.PT: yes("pt"), model.PTBR: no, model.PTPT: no,
	},
}

// llmLanguages is shared by openai / anthropic / gemini. It is written out (not derived from
// model.AllLanguages) so that adding a language still needs a conscious decision here too.
var llmLanguages = map[model.Language]LanguageCapability{
	model.ZH: yes(string(model.ZH)), model.EN: yes(string(model.EN)), model.JA: yes(string(model.JA)),
	model.KO: yes(string(model.KO)), model.FR: yes(string(model.FR)), model.DE: yes(string(model.DE)),
	model.RU: yes(string(model.RU)),
	model.ES: yes(string(model.ES)), model.ESMX: yes(string(model.ESMX)),
	model.PT: yes(string(model.PT)), model.PTBR: yes(string(model.PTBR)), model.PTPT: yes(string(model.PTPT)),
}

// LookupLanguage returns the engine's decision for language l. ok=false means no decision is
// recorded: an engine without a translation registry (unknown / OCR), or a language that was
// added to the model without a capability decision — CI's exhaustiveness test fails on the
// latter for every translator.
func LookupLanguage(engineName string, l model.Language) (LanguageCapability, bool) {
	c, ok := languageRegistry[engineName][l]
	return c, ok
}

// SupportedTargets lists the selectable, non-auto languages the engine can translate into, in
// model.SelectableLanguages order. It is what the frontend receives (AllEngineItem.TargetLanguages)
// to decide which target options to disable; nil for an engine without a translation registry.
func SupportedTargets(engineName string) []model.Language {
	table, ok := languageRegistry[engineName]
	if !ok {
		return nil
	}
	out := make([]model.Language, 0, len(table))
	for _, l := range model.SelectableLanguages() {
		if l == model.Auto {
			continue
		}
		if c, ok := table[l]; ok && c.Supported {
			out = append(out, l)
		}
	}
	return out
}

// isAuto reports whether code asks for source auto-detection ("" or auto, any case).
func isAuto(code string) bool { return code == "" || strings.EqualFold(code, string(model.Auto)) }

// sourceCode returns the engine's request code for a SOURCE language: a dialect is aliased to
// its base (model.Language.Base) and resolved with the base's code. The alias is applied first
// because es-419 is an alias of es without being a recognized language of its own. Codes outside
// the recognized set are handed to fallback, which keeps each engine's legacy pass-through for
// them.
func sourceCode(engineName, code string, fallback func(string) string) string {
	if l, ok := model.ParseLanguage(string(model.Language(code).Base())); ok {
		if c, ok := LookupLanguage(engineName, l); ok && c.Supported {
			return c.Code
		}
	}
	return fallback(code)
}

// targetCode returns the engine's request code for a TARGET language: exact match only. A
// recognized language the engine has no supported entry for — a dialect it lacks — is an error,
// never degraded to the base language. Codes outside the recognized set go to fallback, as in
// sourceCode.
func targetCode(engineName, code string, fallback func(string) string) (string, error) {
	l, ok := model.ParseLanguage(code)
	if !ok {
		return fallback(code), nil
	}
	if c, ok := LookupLanguage(engineName, l); ok && c.Supported {
		return c.Code, nil
	}
	return "", unsupportedTargetError(engineName, l)
}

// unsupportedTargetError is the visible refusal for a target the engine cannot translate into.
func unsupportedTargetError(engineName string, l model.Language) error {
	return errors.New(i18n.T("err.engine_unsupported_target", "engine", engineName, "lang", languageLabel(l)))
}

// languageLabel is the language's display name in the current backend locale ("lang.<code>").
func languageLabel(l model.Language) string { return i18n.T("lang." + string(l)) }

// echoLanguage renders a request language back as a result language when an engine's response
// does not report one: recognized codes keep their canonical spelling (pt-BR stays pt-BR), the
// rest is lower-cased as before.
func echoLanguage(l model.Language) model.Language {
	if r, ok := model.ParseLanguage(string(l)); ok {
		return r
	}
	return model.Language(strings.ToLower(string(l)))
}

// identity is the pass-through fallback for engines that never rewrote unrecognized codes.
func identity(code string) string { return code }
