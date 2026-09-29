package translate

import (
	"strings"
	"unicode/utf8"

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
// It is the same floor #161 uses for correcting a pin (minPinCheckRunes): language detection is
// unreliable on shorter text.
const switchMinRunes = minPinCheckRunes

// switchMinConfidence is the lowest local-detection confidence that can switch anything. Measured
// on clear sentences NaturalLanguage answers 0.99 or more; ambiguous or mixed text lands well
// below.
const switchMinConfidence = 0.8

// switchDetectRunes bounds the text the detector reads. The language of a text shows within its
// first few thousand characters, and the input can be far larger (up to the translate input cap),
// so a huge paste does not make detection slower.
const switchDetectRunes = 2000

// PlanSourceSwitch decides what the language pair becomes for text that has just arrived. When it
// answers Switched, From is the detected language (qualified with the variant the user works in,
// and always one the source dropdown offers, so bare es is es-MX) and To is req.From, the old
// source. Otherwise nothing changes. See the file comment for the rules.
func (s *Service) PlanSourceSwitch(req model.SourceSwitchRequest) model.SourceSwitch {
	none := model.SourceSwitch{}
	if !s.autoSwitchEnabled() || isAutoSource(req.From) {
		return none
	}
	trimmed := strings.TrimSpace(req.Text)
	if utf8.RuneCountInString(trimmed) < switchMinRunes {
		return none
	}
	detected, ok := s.detectForSwitch(trimmed, req.Detected)
	if !ok {
		return none
	}
	// A bare detection cannot name a dialect, so it covers a pin of any of its variants: es is not
	// a correction of es-MX, and pt is none of pt-BR / pt-PT.
	if detected.Covers(req.From) {
		return none
	}
	from := detected
	if detected.Covers(req.To) {
		// The text is in the target's language: the same swap, and the target's own dialect stays.
		from = req.To
	} else {
		from = s.resultFrom(model.Auto, detected).SelectableOr(detected)
	}
	if from.SameAs(req.From) {
		return none
	}
	return model.SourceSwitch{Switched: true, From: from, To: req.From}
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
func (s *Service) detectForSwitch(trimmed string, hint model.Language) (model.Language, bool) {
	sample := trimmed
	if utf8.RuneCountInString(sample) > switchDetectRunes {
		sample = string([]rune(sample)[:switchDetectRunes])
	}
	lang, confidence, ok := s.detect(sample)
	if ok {
		if confidence < switchMinConfidence {
			return "", false
		}
		return recognized(lang)
	}
	return recognized(hint)
}

// recognized reports lang when the app knows it (model.ParseLanguage), as that language; an empty,
// auto or unknown code names none, like an engine's own native code in translateWithEngine.
func recognized(lang model.Language) (model.Language, bool) {
	if isAutoSource(lang) {
		return "", false
	}
	return model.ParseLanguage(string(lang))
}
