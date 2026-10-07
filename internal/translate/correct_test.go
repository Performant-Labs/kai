package translate

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/langpref"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/settings"
)

// Issue #208: CorrectSource is the ONE place that decides whether, and how, text that has just
// arrived is corrected before it is translated. The provider is a fake (the real one is covered by
// the guarded test in correct_real_darwin_test.go); everything else is the real service, settings
// and word diff.

const (
	badMX      = "Ellos no sabe donde esta la biblioteca de la ciudad"
	fixedMX    = "Ellos no saben dónde está la biblioteca de la ciudad"
	goodMX     = "Necesito hablar con el gerente mañana temprano"
	mixedMX    = "Necesito hacer el follow up con el cliente antes del deadline"
	mixedFixed = "Necesito hacer el seguimiento con el cliente antes de la fecha límite"
)

// fakeCorrector is a provider that answers with a fixed output and counts what it was asked.
type fakeCorrector struct {
	mu     sync.Mutex
	status engine.CorrectionStatus // "" reads as available
	out    string
	err    error
	block  bool // wait for the context instead of answering
	calls  int
	req    engine.CorrectRequest
	langs  []model.Language
}

func (f *fakeCorrector) Name() string { return "fake" }

func (f *fakeCorrector) Availability(lang model.Language) engine.CorrectionStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.langs = append(f.langs, lang)
	if f.status == "" {
		return engine.CorrectionAvailable
	}
	return f.status
}

func (f *fakeCorrector) Correct(ctx context.Context, req engine.CorrectRequest) (string, error) {
	f.mu.Lock()
	f.calls++
	f.req = req
	block, out, err := f.block, f.out, f.err
	f.mu.Unlock()
	if block {
		<-ctx.Done()
		return "", context.Cause(ctx)
	}
	return out, err
}

func (f *fakeCorrector) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// scriptedDetector answers per exact text, else with def.
type scriptedDetector struct {
	by  map[string]detection
	def detection
}

type detection struct {
	lang model.Language
	conf float64
	ok   bool
}

func (d *scriptedDetector) detect(text string) (model.Language, float64, bool) {
	if x, ok := d.by[text]; ok {
		return x.lang, x.conf, x.ok
	}
	return d.def.lang, d.def.conf, d.def.ok
}

func spanishEverywhere() *scriptedDetector {
	return &scriptedDetector{def: detection{"es", 0.97, true}}
}

// newCorrectService builds a service whose settings have the correction ON.
func newCorrectService(t *testing.T, fc *fakeCorrector, det *scriptedDetector) *Service {
	t.Helper()
	st, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st.Get().CorrectSourceText = true
	svc := NewService(engine.NewRegistry(), nil, st, nil)
	svc.detect = det.detect
	svc.corrector = fc
	svc.SetLangPrefs(langpref.New())
	return svc
}

func correct(svc *Service, text string, from model.Language) model.Correction {
	return svc.CorrectSource(model.CorrectionRequest{Text: text, From: from})
}

func assertNoCorrection(t *testing.T, label string, got model.Correction, text string, want model.CorrectionStatus) {
	t.Helper()
	if got.Corrected {
		t.Errorf("%s: Corrected = true, want false", label)
	}
	if got.Status != want {
		t.Errorf("%s: Status = %q, want %q", label, got.Status, want)
	}
	if got.Text != text {
		t.Errorf("%s: Text = %q, want the text as it came %q", label, got.Text, text)
	}
	if len(got.Changes) != 0 {
		t.Errorf("%s: Changes = %+v, want none", label, got.Changes)
	}
}

func TestCorrectSourceCorrectsGrammarKeepingTheVariant(t *testing.T) {
	fc := &fakeCorrector{out: fixedMX}
	svc := newCorrectService(t, fc, spanishEverywhere())
	got := correct(svc, badMX, model.ESMX)
	if !got.Corrected || got.Status != model.CorrectionCorrected {
		t.Fatalf("got %+v, want a correction", got)
	}
	if got.Text != fixedMX || got.Original != badMX {
		t.Errorf("Text/Original = %q / %q", got.Text, got.Original)
	}
	if got.Language != model.ESMX {
		t.Errorf("Language = %q, want es-MX", got.Language)
	}
	// The variant reaches the model: Mexican Spanish stays Mexican Spanish.
	if !strings.Contains(fc.req.Instructions, "Mexican Spanish") {
		t.Errorf("the instructions do not name Mexican Spanish:\n%s", fc.req.Instructions)
	}
	if fc.req.Instructions != CorrectionInstructions(model.ESMX) {
		t.Error("the model got instructions other than CorrectionInstructions(es-MX)")
	}
	if fc.req.Text != badMX {
		t.Errorf("the model got %q, want the text", fc.req.Text)
	}
	// The change list is computed here from the two texts.
	want := diffWords(badMX, fixedMX)
	if len(want) == 0 || !slices.Equal(got.Changes, want) {
		t.Errorf("Changes = %+v, want %+v", got.Changes, want)
	}
}

func TestCorrectSourceReplacesMixedInEnglish(t *testing.T) {
	fc := &fakeCorrector{out: mixedFixed}
	// The mixed sentence is not confidently Spanish: detection must not move the language off the pin.
	det := &scriptedDetector{def: detection{"es", 0.97, true}, by: map[string]detection{mixedMX: {"es", 0.55, true}}}
	svc := newCorrectService(t, fc, det)
	got := correct(svc, mixedMX, model.ESMX)
	if !got.Corrected || got.Text != mixedFixed {
		t.Fatalf("got %+v", got)
	}
	if got.Language != model.ESMX {
		t.Errorf("Language = %q, want es-MX", got.Language)
	}
	var sawFollowUp bool
	for _, c := range got.Changes {
		if strings.Contains(c.Before, "follow up") && strings.Contains(c.After, "seguimiento") {
			sawFollowUp = true
		}
	}
	if !sawFollowUp {
		t.Errorf("Changes = %+v, want follow up -> seguimiento", got.Changes)
	}
}

func TestCorrectSourceAlreadyCorrectTextIsUnchanged(t *testing.T) {
	for label, out := range map[string]string{
		"identical":        goodMX,
		"whitespace only":  "  " + strings.ReplaceAll(goodMX, " ", "  ") + "\n",
		"trailing newline": goodMX + "\n",
	} {
		fc := &fakeCorrector{out: out}
		svc := newCorrectService(t, fc, spanishEverywhere())
		got := correct(svc, goodMX, model.ESMX)
		assertNoCorrection(t, label, got, goodMX, model.CorrectionUnchanged)
		if fc.callCount() != 1 {
			t.Errorf("%s: the model was called %d times, want 1", label, fc.callCount())
		}
	}
}

func TestCorrectSourceSettingOffDoesNothing(t *testing.T) {
	fc := &fakeCorrector{out: fixedMX}
	svc := newCorrectService(t, fc, spanishEverywhere())
	svc.settings.Get().CorrectSourceText = false
	assertNoCorrection(t, "off", correct(svc, badMX, model.ESMX), badMX, model.CorrectionOff)
	if fc.callCount() != 0 {
		t.Errorf("the model was called %d times with the setting off", fc.callCount())
	}
	// No settings service at all reads as off too (the default is off).
	svc.settings = nil
	assertNoCorrection(t, "no settings", correct(svc, badMX, model.ESMX), badMX, model.CorrectionOff)
	if fc.callCount() != 0 {
		t.Errorf("the model was called %d times without settings", fc.callCount())
	}
}

func TestCorrectSourceAutoSourceIsNeverCorrected(t *testing.T) {
	for _, from := range []model.Language{model.Auto, "", "AUTO"} {
		fc := &fakeCorrector{out: fixedMX}
		svc := newCorrectService(t, fc, spanishEverywhere())
		assertNoCorrection(t, "auto", correct(svc, badMX, from), badMX, model.CorrectionAutoSource)
		if fc.callCount() != 0 {
			t.Errorf("from %q: the model was called", from)
		}
	}
}

func TestCorrectSourceShortTextIsNeverCorrected(t *testing.T) {
	// 7 code points (multi-byte ones count once) and blanks: nothing. 8: corrected.
	for _, text := range []string{"", "   ", "hola", "ñandúes", "  ñandúes  ", "\n\nhola\n"} {
		fc := &fakeCorrector{out: "hola, amigo"}
		svc := newCorrectService(t, fc, spanishEverywhere())
		assertNoCorrection(t, "short "+text, correct(svc, text, model.ESMX), text, model.CorrectionTooShort)
		if fc.callCount() != 0 {
			t.Errorf("%q: the model was called", text)
		}
	}
	fc := &fakeCorrector{out: "ñandúes!"}
	svc := newCorrectService(t, fc, spanishEverywhere())
	if got := correct(svc, "ñandues!", model.ESMX); !got.Corrected {
		t.Errorf("an 8 code point text was not corrected: %+v", got)
	}
}

func TestCorrectSourceLongTextIsNeverCorrected(t *testing.T) {
	fc := &fakeCorrector{out: fixedMX}
	svc := newCorrectService(t, fc, spanishEverywhere())
	long := strings.Repeat("Ellos no sabe donde esta. ", 200) // well over the cap
	assertNoCorrection(t, "long", correct(svc, long, model.ESMX), long, model.CorrectionTooLong)
	if fc.callCount() != 0 {
		t.Error("the model was called for a text over the cap")
	}
}

func TestCorrectSourceUnavailableProviderIsNeverTried(t *testing.T) {
	for _, st := range []engine.CorrectionStatus{
		engine.CorrectionModelNotReady, engine.CorrectionAppleIntelligenceOff, engine.CorrectionUnsupportedHardware,
		engine.CorrectionUnsupportedLanguage, engine.CorrectionUnsupportedPlatform, engine.CorrectionUnavailable,
	} {
		fc := &fakeCorrector{status: st, out: fixedMX}
		svc := newCorrectService(t, fc, spanishEverywhere())
		got := correct(svc, badMX, model.ESMX)
		assertNoCorrection(t, string(st), got, badMX, model.CorrectionUnavailable)
		if got.Reason != string(st) {
			t.Errorf("%s: Reason = %q, want the provider's status", st, got.Reason)
		}
		if fc.callCount() != 0 {
			t.Errorf("%s: an unavailable provider was still asked to correct", st)
		}
		// The language the text is in is what availability is asked for.
		if !slices.Contains(fc.langs, model.ESMX) {
			t.Errorf("%s: availability was asked for %v, want es-MX", st, fc.langs)
		}
	}
	// No provider at all is unavailable, not a crash.
	svc := newCorrectService(t, &fakeCorrector{}, spanishEverywhere())
	svc.corrector = nil
	assertNoCorrection(t, "nil provider", correct(svc, badMX, model.ESMX), badMX, model.CorrectionUnavailable)
}

func TestCorrectSourceGuardsTheOutput(t *testing.T) {
	spanglishEN := "I need to do the follow up with the client before the deadline"
	det := &scriptedDetector{def: detection{"es", 0.97, true}, by: map[string]detection{
		spanglishEN: {"en", 0.99, true},
		mixedMX:     {"es", 0.55, true},
	}}
	cases := []struct {
		name  string
		text  string
		out   string
		err   error
		want  model.CorrectionStatus
		guard string
	}{
		{"empty output", badMX, "", nil, model.CorrectionRejected, "empty"},
		{"blank output", badMX, "  \n ", nil, model.CorrectionRejected, "empty"},
		{"provider refusal", badMX, "", engine.ErrCorrectionRefused, model.CorrectionFailed, ""},
		{"provider error", badMX, "", errors.New("boom"), model.CorrectionFailed, ""},
		{"refusal in the text", badMX, "Lo siento, no puedo ayudar con eso.", nil, model.CorrectionRejected, "refusal"},
		{"english refusal", badMX, "I'm sorry, but I can't help with that request.", nil, model.CorrectionRejected, "refusal"},
		{"far too long", badMX, badMX + " " + strings.Repeat("Además explico lo que cambié y por qué lo cambié. ", 6), nil, model.CorrectionRejected, "length"},
		{"far too short", badMX, "Ellos no", nil, model.CorrectionRejected, "length"},
		{"another language", mixedMX, spanglishEN, nil, model.CorrectionRejected, "language"},
	}
	for _, c := range cases {
		fc := &fakeCorrector{out: c.out, err: c.err}
		svc := newCorrectService(t, fc, det)
		got := correct(svc, c.text, model.ESMX)
		assertNoCorrection(t, c.name, got, c.text, c.want)
		// The guard that caught it is the one that should have: a second guard catching it by
		// accident would leave the first one untested.
		if got.Reason != c.guard {
			t.Errorf("%s: Reason = %q, want %q", c.name, got.Reason, c.guard)
		}
		if fc.callCount() != 1 {
			t.Errorf("%s: model calls = %d, want 1", c.name, fc.callCount())
		}
	}
	// A refusal that the INPUT itself starts with is not a refusal of ours.
	fc := &fakeCorrector{out: "Lo siento, no puedo ir mañana a la fiesta"}
	svc := newCorrectService(t, fc, spanishEverywhere())
	in := "Lo siento, no puedo ir mañana a la fiesta"
	assertNoCorrection(t, "input starts like a refusal", correct(svc, in, model.ESMX), in, model.CorrectionUnchanged)
}

// A correction that only drops the trailing punctuation the text ended with is the model being
// pedantic about a fragment, not a fix: the text goes on as it came.
func TestCorrectSourceDroppingTrailingPunctuationIsRejected(t *testing.T) {
	for _, c := range []struct{ in, out string }{
		{"Mientras tanto,", "Mientras tanto"},
		{"Mientras tanto ,", "Mientras tanto"},
		{"Nos vemos mañana!", "Nos vemos mañana"},
	} {
		svc := newCorrectService(t, &fakeCorrector{out: c.out}, spanishEverywhere())
		got := correct(svc, c.in, model.ESMX)
		assertNoCorrection(t, c.in, got, c.in, model.CorrectionRejected)
		if got.Reason != "punctuation" {
			t.Errorf("%q: Reason = %q, want punctuation", c.in, got.Reason)
		}
	}
	// Adding or changing punctuation, or fixing a word as well, is still a correction.
	svc := newCorrectService(t, &fakeCorrector{out: "Mientras tanto, voy."}, spanishEverywhere())
	if got := correct(svc, "Mientras tanto, voi,", model.ESMX); !got.Corrected {
		t.Errorf("a real fix alongside dropped punctuation was rejected: %+v", got)
	}
}

func TestCorrectSourceTimeoutTranslatesTheOriginal(t *testing.T) {
	fc := &fakeCorrector{block: true}
	svc := newCorrectService(t, fc, spanishEverywhere())
	svc.correctTimeout = 40 * time.Millisecond
	start := time.Now()
	got := correct(svc, badMX, model.ESMX)
	assertNoCorrection(t, "timeout", got, badMX, model.CorrectionFailed)
	if time.Since(start) > 3*time.Second {
		t.Errorf("the call took %v, the timeout did not bound it", time.Since(start))
	}
}

func TestCorrectSourceLanguageFollowsTheText(t *testing.T) {
	const english = "I would like to go to the market tomorrow morning"
	// A pinned es-MX under English text: the text is corrected AS ENGLISH, never turned into Spanish.
	fc := &fakeCorrector{out: english}
	det := &scriptedDetector{def: detection{"en", 0.99, true}}
	svc := newCorrectService(t, fc, det)
	got := correct(svc, english, model.ESMX)
	if got.Language != model.EN || !strings.Contains(fc.req.Instructions, "English") || strings.Contains(fc.req.Instructions, "Mexican") {
		t.Errorf("English text under an es-MX pin: language %q, instructions:\n%s", got.Language, fc.req.Instructions)
	}
	// A bare detection of the pinned language keeps the pinned variant.
	fc = &fakeCorrector{out: fixedMX}
	svc = newCorrectService(t, fc, spanishEverywhere())
	if got := correct(svc, badMX, model.ESMX); got.Language != model.ESMX {
		t.Errorf("bare es under an es-MX pin: language %q, want es-MX", got.Language)
	}
	// Spanish text under an English pin: corrected as Spanish, in a variant the dropdowns offer.
	fc = &fakeCorrector{out: fixedMX}
	svc = newCorrectService(t, fc, spanishEverywhere())
	if got := correct(svc, badMX, model.EN); got.Language != model.ESMX {
		t.Errorf("Spanish under an en pin: language %q, want es-MX", got.Language)
	}
	// No local detector (off macOS) and no hint: the pin is the language.
	fc = &fakeCorrector{out: fixedMX}
	svc = newCorrectService(t, fc, &scriptedDetector{})
	if got := correct(svc, badMX, model.PTBR); got.Language != model.PTBR || !strings.Contains(fc.req.Instructions, "Brazilian Portuguese") {
		t.Errorf("no detector: language %q", got.Language)
	}
}

func TestCorrectSourceNeverPersistsOrLearns(t *testing.T) {
	dir := t.TempDir()
	st, err := settings.NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	st.Get().CorrectSourceText = true
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	prefs := langpref.New()
	svc := NewService(engine.NewRegistry(), nil, st, nil)
	svc.detect = spanishEverywhere().detect
	svc.corrector = &fakeCorrector{out: fixedMX}
	svc.SetLangPrefs(prefs)
	fromBefore, toBefore := st.Get().DefaultFrom, st.Get().DefaultTo
	if got := correct(svc, badMX, model.ESMX); !got.Corrected {
		t.Fatalf("no correction: %+v", got)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Error("a correction rewrote settings.json")
	}
	if st.Get().DefaultFrom != fromBefore || st.Get().DefaultTo != toBefore {
		t.Error("a correction changed the default language pair")
	}
	if got := prefs.Qualify("es"); got != "es" {
		t.Errorf("a correction taught the variant store: es -> %q", got)
	}
}

func TestCorrectSourceLogsLengthNeverText(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	for _, out := range []string{fixedMX, "", badMX} {
		svc := newCorrectService(t, &fakeCorrector{out: out}, spanishEverywhere())
		correct(svc, badMX, model.ESMX)
	}
	logged := buf.String()
	if !strings.Contains(logged, "text_len") {
		t.Fatalf("nothing about the correction was logged (a length is expected):\n%s", logged)
	}
	for _, secret := range []string{"biblioteca", "ciudad", "Ellos", "dónde"} {
		if strings.Contains(logged, secret) {
			t.Errorf("the log carries the user's text (%q):\n%s", secret, logged)
		}
	}
}

func TestCorrectionAvailabilityDoesNotDependOnTheSetting(t *testing.T) {
	fc := &fakeCorrector{}
	svc := newCorrectService(t, fc, spanishEverywhere())
	svc.settings.Get().CorrectSourceText = false
	if got := svc.CorrectionAvailability(); !got.Available || got.Reason != "" {
		t.Errorf("available provider: %+v", got)
	}
	fc.status = engine.CorrectionAppleIntelligenceOff
	if got := svc.CorrectionAvailability(); got.Available || got.Reason != "apple_intelligence_off" {
		t.Errorf("Apple Intelligence off: %+v", got)
	}
	svc.corrector = nil
	if got := svc.CorrectionAvailability(); got.Available || got.Reason != string(engine.CorrectionUnavailable) {
		t.Errorf("no provider: %+v", got)
	}
}

// Issue #44 / #80 stay intact: normal translation never presents X to X as a translation, and the
// correction is its own mode, never on the translation path. Translating an es-MX text to es-MX
// with the correction switched ON still shows the source text as an identity result, asks no
// engine, and never calls the corrector.
func TestNormalTranslationSameLanguageStillIdentityWithCorrectionOn(t *testing.T) {
	svc, g, _ := newIdentityService(t, gtxOpts{result: "engine-text"})
	fc := &fakeCorrector{out: "Hola mundo!"}
	st, err := settings.NewService(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	st.Get().CorrectSourceText = true
	svc.settings = st
	svc.corrector = fc
	res, err := idTranslate(svc, model.ESMX, model.ESMX)
	assertIdentity(t, "es-MX/es-MX with the correction on", res, err)
	if n := g.count(); n != 0 {
		t.Errorf("the engine got %d requests, want 0", n)
	}
	if fc.callCount() != 0 {
		t.Errorf("translating called the corrector %d times: the correction is its own mode", fc.callCount())
	}
	// And the correction of the same text is not a translation result: it produces no TranslateResult.
	svc.detect = spanishEverywhere().detect
	if got := correct(svc, idText+" amigo", model.ESMX); got.Status == "" {
		t.Errorf("no status: %+v", got)
	}
}
