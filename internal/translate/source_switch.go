package translate

import (
	"log/slog"
	"regexp"
	"strings"
	"unicode/utf8"

	"cnb.cool/dtapp/kai/internal/buildinfo"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// The automatic source switch (issue #200).
//
// Text often arrives in another language than the source dropdown shows. When the source is
// pinned, the dropdown should follow the text: the source becomes the language the text is in, the
// target becomes the language that was in the source dropdown (the old target is replaced by it),
// and the window translates. This file is the ONE place that decides it, for every way text
// arrives (the hotkey / tray / clipboard fill, a paste, Translate on typed text, and the double
// Cmd+C trigger when it exists): the caller sends the text and the pair it shows, and applies the
// answer. The frontend compares no languages and counts no characters.
//
// It replaces #161's "the source dropdown never changes" for a pinned source (translateWithEngine
// still corrects the translation itself, so a request that goes out with a pin the text does not
// match is still translated from the right language). An Auto source has no dropdown entry to
// switch: it never switches, and #161's "auto-detected" note stays.
//
// Detection is local and runs before any engine: engine.DetectLanguage (NaturalLanguage through the
// Swift bridge on macOS), which reports a confidence. Where there is none, or it recognizes
// nothing, a detection the caller already holds (req.Detected, the one a #161 result carried) is the
// fallback: it carries no confidence, so it only ever fills a gap and never overrides a local
// verdict, low confidence included.
//
// The decision reads settings.Settings.AutoSwitchSource and touches nothing else: it does not teach
// the variant preference store (an automatic switch is not a choice the user made) and does not
// write the persisted default pair.

// switchMinRunes is the shortest text, in code points after trimming, that can switch anything.
// It is lower than #161's floor for correcting a pin (minPinCheckRunes): a switch also needs the
// local detector's confidence (switchMinConfidence), which a short ambiguous text does not reach,
// whereas a short phrase the detector is sure of ("In the meantime,") must not be "translated" from
// the wrong pinned language and handed back unchanged.
const switchMinRunes = 12

// switchMinConfidence is the lowest local-detection confidence that can switch anything. Measured
// on clear sentences NaturalLanguage answers 0.99 or more; ambiguous or mixed text lands well
// below.
const switchMinConfidence = 0.8

// switchMinConfidenceTarget is the lowest confidence that can switch when the detected language is
// the TARGET language (issue #54). A text already in the language it would be translated into comes
// back as an identity result, which is useless, whereas a wrong switch is cheap and undoable, so
// this bar is lower than switchMinConfidence. A misplaced accent alone drops NaturalLanguage's
// confidence on a clear Spanish sentence to about 0.74.
const switchMinConfidenceTarget = 0.5

// switchDetectRunes bounds the text the detector reads. The language of a text shows within its
// first few thousand characters, and the input can be far larger (up to the translate input cap),
// so a huge paste does not make detection slower.
const switchDetectRunes = 2000

// urlPattern matches a link in text: a scheme and everything up to whitespace, or a www. host.
var urlPattern = regexp.MustCompile(`(?i)\b(?:[a-z][a-z0-9+.-]*://|www\.)\S+`)

// PlanSourceSwitch decides what the language pair becomes for text that has just arrived. When it
// answers Switched, From is the detected language (qualified with the variant the user works in,
// and always one the source dropdown offers, so bare es is es-MX) and To is req.From, the old
// source. Otherwise nothing changes. See the file comment for the rules.
func (s *Service) PlanSourceSwitch(req model.SourceSwitchRequest) model.SourceSwitch {
	plan, d := s.planSourceSwitch(req)
	// One line per decision (issue #16): the detection and why it ended as it did. Never the text
	// (issue #203): only its length.
	// The build too (issue #16), so the line stands alone in a pasted log.
	slog.Info(i18n.T("log.source_switch_plan"), append([]any{
		"reason", plan.Reason, "detected", string(d.lang), "confidence", d.confidence,
		"runes", d.runes, "by", d.by, "from", string(req.From), "to", string(req.To),
		"result_from", string(plan.From),
	}, buildinfo.LogAttrs()...)...)
	return plan
}

// switchDetail is what a decision saw, for its log line.
type switchDetail struct {
	lang       model.Language // what the detector or the hint named, as it named it
	confidence float64        // the local detector's; 0 for a hint or nothing
	runes      int            // code points of the trimmed text
	by         string         // "local", "hint" or "none"
}

func (s *Service) planSourceSwitch(req model.SourceSwitchRequest) (model.SourceSwitch, switchDetail) {
	d := switchDetail{by: "none"}
	no := func(reason string) (model.SourceSwitch, switchDetail) {
		return model.SourceSwitch{Reason: reason}, d
	}
	// Links are not text in a language: the address of a Spanish page is not Spanish text, so the
	// floor and the detector read what is left once they are taken out.
	trimmed := strings.TrimSpace(urlPattern.ReplaceAllString(req.Text, " "))
	d.runes = utf8.RuneCountInString(trimmed)
	if !s.autoSwitchEnabled() {
		return no(model.SwitchReasonDisabled)
	}
	if isAutoSource(req.From) {
		return no(model.SwitchReasonSourceAuto)
	}
	if d.runes < switchMinRunes {
		return no(model.SwitchReasonTooShort)
	}
	detected, det, reason := s.detectForSwitch(trimmed, req.Detected)
	d.lang, d.confidence, d.by = det.lang, det.confidence, det.by
	// Issue #54: a text in the TARGET language that is below the general bar still switches: leaving
	// the pair would only give back an identity result.
	if reason == model.SwitchReasonBelowConfidence && det.confidence >= switchMinConfidenceTarget {
		if rec, known := recognized(det.lang); known && rec.Covers(req.To) {
			detected, reason = rec, ""
		}
	}
	if reason != "" {
		return no(reason)
	}
	// A bare detection cannot name a dialect, so it covers a pin of any of its variants: es is not
	// a correction of es-MX, and pt is none of pt-BR / pt-PT.
	if detected.Covers(req.From) {
		return no(model.SwitchReasonSameLanguage)
	}
	from := detected
	if detected.Covers(req.To) {
		// The text is in the target's language: the same swap, and the target's own dialect stays.
		from = req.To
	} else {
		from = s.resultFrom(model.Auto, detected).SelectableOr(detected)
	}
	if from.SameAs(req.From) {
		return no(model.SwitchReasonSameLanguage)
	}
	return model.SourceSwitch{Switched: true, From: from, To: req.From, Reason: model.SwitchReasonSwitched}, d
}

// autoSwitchEnabled reads the setting: on unless the settings say off. No settings service (a test,
// or before wiring) is the default, on.
func (s *Service) autoSwitchEnabled() bool {
	if s.settings == nil {
		return true
	}
	cfg := s.settings.Get()
	return cfg == nil || cfg.AutoSwitchSource
}

// detectForSwitch returns the language the text is in, recognized by the app: the local detector's
// when it is confident, else, only when the local detector had nothing to say, the caller's hint.
// When it names none, reason says why (and the detail still says what was seen).
func (s *Service) detectForSwitch(trimmed string, hint model.Language) (model.Language, switchDetail, string) {
	sample := trimmed
	if utf8.RuneCountInString(sample) > switchDetectRunes {
		sample = string([]rune(sample)[:switchDetectRunes])
	}
	lang, confidence, ok := s.detect(sample)
	if ok {
		d := switchDetail{lang: lang, confidence: confidence, by: "local"}
		if confidence < switchMinConfidence {
			return "", d, model.SwitchReasonBelowConfidence
		}
		rec, known := recognized(lang)
		if !known {
			return "", d, model.SwitchReasonNoDetection
		}
		return rec, d, ""
	}
	d := switchDetail{lang: hint, by: "hint"}
	rec, known := recognized(hint)
	if !known {
		d.by = "none"
		return "", d, model.SwitchReasonNoDetection
	}
	return rec, d, ""
}

// recognized reports lang when the app knows it (model.ParseLanguage), as that language; an empty,
// auto or unknown code names none, like an engine's own native code in translateWithEngine.
func recognized(lang model.Language) (model.Language, bool) {
	if isAutoSource(lang) {
		return "", false
	}
	return model.ParseLanguage(string(lang))
}
