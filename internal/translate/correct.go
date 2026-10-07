package translate

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// Correcting the source text (issue #208).
//
// Text arrives in a few ways (the hotkey / tray / clipboard fill, a paste, Translate on typed text,
// and the double Cmd+C trigger), and all of them already ask the backend one question before they
// translate: PlanSourceSwitch, for the language pair. CorrectSource is asked right before it, from
// the same places, and is the ONE place that decides whether the text is corrected: when the
// setting is on and a provider can run, the text is corrected first, and the existing flow (the
// #200 source switch, then the translation) goes on with the corrected text. The answer carries the
// corrected text, the original and what changed, computed here with a word diff and never by the
// model.
//
// The correction is its own mode. It is never on the translation path: Translate / TranslateMulti
// never call it, so #44's rule (a text is never presented as its own translation) and #80's
// identity result are untouched. It writes nothing: it does not touch the settings, the default
// language pair or the variant preference store. The user's text is never logged, only its length.
//
// Whatever goes wrong (the setting off, an Auto source, a short or long text, a provider that
// cannot run, an error, a refusal, a timeout, an output that fails a guard, an output equal to the
// input) is the same answer: no correction, the text as it came.

const (
	// correctMinRunes is the shortest text, in code points after trimming, that is corrected. A
	// shorter one is too little for the model to judge, and a single word is not a sentence.
	correctMinRunes = 8
	// correctMaxRunes is the longest text corrected in one go. The on-device model has a small
	// context window and answers a paragraph in seconds; a long text is translated as it came (the
	// chunked translation, #84, handles it) rather than half-corrected.
	correctMaxRunes = 2000
	// defaultCorrectTimeout bounds one correction. The model normally answers in a few seconds; a
	// stuck one must not hold up the translation, which then goes ahead with the original.
	defaultCorrectTimeout = 30 * time.Second
	// The output guards. A grammar fix changes a few words, so an answer more than about half as
	// long again as the input, or shorter than about three fifths of it, is the model doing
	// something else (explaining, answering, dropping content). A few code points of slack keep a
	// short text from tripping on one added word.
	correctLenSlack = 10
	correctMaxNumer = 14 // out may be up to 1.4 x in
	correctMaxDenom = 10
	correctMinNumer = 6 // out may be down to 0.6 x in
	correctMinDenom = 10
)

// refusalPrefixes are how a model that declines usually starts its answer, lower-cased. A
// corrected text starting with one that the input did not start with is a refusal, not a
// correction.
var refusalPrefixes = []string{
	"i'm sorry", "i am sorry", "sorry,", "i cannot", "i can't", "i can’t", "i'm unable", "i am unable",
	"as an ai", "lo siento", "no puedo", "no me es posible", "disculpa", "disculpe",
}

// CorrectSource corrects the grammar and word choice of text that has just arrived, when the
// setting is on and a provider is available. See the file comment for the rules; the answer's
// Status says which one decided.
func (s *Service) CorrectSource(req model.CorrectionRequest) model.Correction {
	trimmed := strings.TrimSpace(req.Text)
	none := func(status model.CorrectionStatus) model.Correction {
		return model.Correction{Status: status, Text: req.Text, Original: req.Text}
	}
	switch {
	case !s.correctionEnabled():
		return none(model.CorrectionOff)
	case isAutoSource(req.From):
		return none(model.CorrectionAutoSource)
	case utf8.RuneCountInString(trimmed) < correctMinRunes:
		return none(model.CorrectionTooShort)
	case utf8.RuneCountInString(trimmed) > correctMaxRunes:
		return none(model.CorrectionTooLong)
	}
	lang := s.correctionLanguage(trimmed, req)
	if s.corrector == nil {
		out := none(model.CorrectionUnavailable)
		out.Reason = string(engine.CorrectionUnavailable)
		return out
	}
	if st := s.corrector.Availability(lang); !st.IsAvailable() {
		out := none(model.CorrectionUnavailable)
		out.Reason = string(st)
		out.Language = lang
		return out
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), s.correctTimeoutOrDefault())
	defer cancel()
	corrected, err := s.corrector.Correct(ctx, engine.CorrectRequest{Instructions: CorrectionInstructions(lang), Text: trimmed})
	elapsed := time.Since(start)
	if err != nil {
		// The error is logged by kind only: a provider's own message must not be able to carry the text.
		slog.Warn(i18n.T("log.correct_failed"), "kind", correctionErrKind(err), "text_len", len(trimmed), "ms", elapsed.Milliseconds())
		out := none(model.CorrectionFailed)
		out.Language = lang
		return out
	}
	corrected = strings.TrimSpace(corrected)
	if reason := s.rejectCorrection(trimmed, corrected, lang); reason != "" {
		slog.Warn(i18n.T("log.correct_rejected"), "reason", reason, "text_len", len(trimmed), "out_len", len(corrected), "ms", elapsed.Milliseconds())
		out := none(model.CorrectionRejected)
		out.Reason = reason
		out.Language = lang
		return out
	}
	changes := diffWords(trimmed, corrected)
	if len(changes) == 0 {
		slog.Debug(i18n.T("log.correct_done"), "status", model.CorrectionUnchanged, "text_len", len(trimmed), "ms", elapsed.Milliseconds())
		out := none(model.CorrectionUnchanged)
		out.Language = lang
		return out
	}
	slog.Debug(i18n.T("log.correct_done"), "status", model.CorrectionCorrected, "text_len", len(trimmed), "changes", len(changes), "ms", elapsed.Milliseconds())
	return model.Correction{
		Corrected: true,
		Status:    model.CorrectionCorrected,
		Text:      corrected,
		Original:  req.Text,
		Language:  lang,
		Changes:   changes,
	}
}

// CorrectionAvailability reports whether the correction can run on this machine, and if not, why.
// It does not depend on the setting (the toolbar checkbox needs it while the setting is off), nor
// on a language (the model alone; the language is checked when a text is corrected).
func (s *Service) CorrectionAvailability() model.CorrectionAvailability {
	if s.corrector == nil {
		return model.CorrectionAvailability{Reason: string(engine.CorrectionUnavailable)}
	}
	st := s.corrector.Availability("")
	if st.IsAvailable() {
		return model.CorrectionAvailability{Available: true}
	}
	return model.CorrectionAvailability{Reason: string(st)}
}

// correctionEnabled reads the setting: off unless the settings say on. No settings service is off.
func (s *Service) correctionEnabled() bool {
	if s.settings == nil {
		return false
	}
	cfg := s.settings.Get()
	return cfg != nil && cfg.CorrectSourceText
}

func (s *Service) correctTimeoutOrDefault() time.Duration {
	if s.correctTimeout > 0 {
		return s.correctTimeout
	}
	return defaultCorrectTimeout
}

// correctionLanguage is the language the text is corrected in: the pinned source, unless the
// local detector is confident the text is in another language (then that language, qualified with
// the variant the user works in, exactly as the source switch would name it), so English text under
// a Spanish pin is corrected as English and never turned into Spanish. Text the detector is unsure
// about (a Spanish and English mix) stays in the pinned language.
func (s *Service) correctionLanguage(trimmed string, req model.CorrectionRequest) model.Language {
	detected, _, reason := s.detectForSwitch(trimmed, req.Detected)
	if reason != "" || detected.Covers(req.From) {
		return req.From
	}
	return s.resultFrom(model.Auto, detected).SelectableOr(detected)
}

// rejectCorrection is the output guard: "" when the corrected text may be used, else the reason it
// may not (empty, a refusal, wildly longer or shorter than the input, or in another language than
// the one it was asked in).
func (s *Service) rejectCorrection(input, output string, lang model.Language) string {
	if output == "" {
		return "empty"
	}
	lower := strings.ToLower(output)
	inLower := strings.ToLower(input)
	for _, p := range refusalPrefixes {
		if strings.HasPrefix(lower, p) && !strings.HasPrefix(inLower, p) {
			return "refusal"
		}
	}
	// Dropping only the trailing punctuation the text ended with ("Mientras tanto," -> "Mientras
	// tanto") is the model being pedantic about a fragment, not a fix.
	if out := strings.TrimSpace(output); out != "" && out == strings.TrimRightFunc(input, trailingPunct) && out != input {
		return "punctuation"
	}
	in, out := utf8.RuneCountInString(input), utf8.RuneCountInString(output)
	if out > in*correctMaxNumer/correctMaxDenom+correctLenSlack || out < in*correctMinNumer/correctMinDenom-correctLenSlack {
		return "length"
	}
	// The answer must still be in the language it was asked in. A model that translated instead of
	// correcting answers in another one; only a confident verdict rejects.
	if det, conf, ok := s.detect(output); ok && conf >= switchMinConfidence {
		if l, known := recognized(det); known && !l.Covers(lang) {
			return "language"
		}
	}
	return ""
}

// trailingPunct is a rune that ends a text without being part of its words: whitespace or
// punctuation.
func trailingPunct(r rune) bool { return unicode.IsSpace(r) || unicode.IsPunct(r) }

// correctionErrKind names a provider error for the log, without its text.
func correctionErrKind(err error) string {
	switch {
	case errors.Is(err, engine.ErrCorrectionRefused):
		return "refused"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	}
	return "error"
}
