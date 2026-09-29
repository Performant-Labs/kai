package translate

import (
	"strings"
	"testing"
	"unicode/utf8"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/langpref"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/settings"
)

// Issue #200: text arrives in another language than the pinned source shows. PlanSourceSwitch is
// the ONE place that decides what the source and target become, for every way text arrives (the
// hotkey fill, the tray, a paste, Translate on typed text). These tests pin the decision itself:
// the frontend only asks and applies.

const spanish = "Hola, necesito que me ayudes con este documento hoy" // well over 20 code points

type fakeDetector struct {
	lang  model.Language
	conf  float64
	ok    bool
	calls int
	seen  string
}

func (f *fakeDetector) detect(text string) (model.Language, float64, bool) {
	f.calls++
	f.seen = text
	return f.lang, f.conf, f.ok
}

func newSwitchService(t *testing.T, det *fakeDetector, prefs *langpref.Store) *Service {
	t.Helper()
	svc := NewService(engine.NewRegistry(), nil, nil, nil)
	svc.detect = det.detect
	svc.SetLangPrefs(prefs)
	return svc
}

func spanishDetector() *fakeDetector { return &fakeDetector{lang: "es", conf: 0.97, ok: true} }

func TestPlanSourceSwitchMismatchSwitchesAndSwapsTarget(t *testing.T) {
	prefs := langpref.New()
	prefs.Learn(model.ESMX)
	svc := newSwitchService(t, spanishDetector(), prefs)
	got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.FR})
	if !got.Switched {
		t.Fatal("Spanish text under an English pin did not switch")
	}
	// The dialect the user works in is kept (es-MX learned), and the old target is REPLACED by the
	// old source: not fr, not left alone.
	if got.From != model.ESMX || got.To != model.EN {
		t.Fatalf("pair = %s -> %s, want es-MX -> en", got.From, got.To)
	}
}

func TestPlanSourceSwitchBareDetectionLandsOnSelectableVariant(t *testing.T) {
	svc := newSwitchService(t, spanishDetector(), langpref.New())
	got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.FR})
	if !got.Switched || got.From != model.ESMX || got.To != model.EN {
		t.Fatalf("got %+v, want es-MX -> en (bare es is not a dropdown option)", got)
	}
}

func TestPlanSourceSwitchMatchChangesNothing(t *testing.T) {
	det := &fakeDetector{lang: "en", conf: 0.99, ok: true}
	svc := newSwitchService(t, det, langpref.New())
	got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: strings.Repeat("the quick brown fox ", 3), From: model.EN, To: model.FR})
	if got.Switched {
		t.Fatalf("text in the pinned language switched: %+v", got)
	}
}

func TestPlanSourceSwitchDetectedEqualsTargetIsTheSameSwap(t *testing.T) {
	svc := newSwitchService(t, spanishDetector(), langpref.New())
	got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.ESMX})
	if !got.Switched || got.From != model.ESMX || got.To != model.EN {
		t.Fatalf("got %+v, want es-MX -> en", got)
	}
	// The target's own dialect wins over a bare detection: a pt-PT target stays pt-PT (not the
	// selectable default pt-BR) when Portuguese arrives.
	pt := newSwitchService(t, &fakeDetector{lang: "pt", conf: 0.95, ok: true}, langpref.New())
	got = pt.PlanSourceSwitch(model.SourceSwitchRequest{Text: "Olá, preciso que você me ajude com este documento", From: model.EN, To: model.PTPT})
	if !got.Switched || got.From != model.PTPT || got.To != model.EN {
		t.Fatalf("got %+v, want pt-PT -> en", got)
	}
}

func TestPlanSourceSwitchKeepsDialectMatching(t *testing.T) {
	// A bare detection cannot name a dialect: it covers a pin of any of its variants, so an es-MX
	// pin is not "corrected" by a Spanish detection, and pt-BR / pt-PT pins are not corrected by a
	// Portuguese one.
	svc := newSwitchService(t, spanishDetector(), langpref.New())
	if got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish, From: model.ESMX, To: model.EN}); got.Switched {
		t.Errorf("es-MX pin switched on a Spanish detection: %+v", got)
	}
	svc2 := newSwitchService(t, &fakeDetector{lang: "pt", conf: 0.95, ok: true}, langpref.New())
	for _, pin := range []model.Language{model.PTBR, model.PTPT} {
		got := svc2.PlanSourceSwitch(model.SourceSwitchRequest{Text: "Olá, preciso que você me ajude com este documento", From: pin, To: model.EN})
		if got.Switched {
			t.Errorf("%s pin switched on a Portuguese detection: %+v", pin, got)
		}
	}
}

func TestPlanSourceSwitchShortTextNeverSwitches(t *testing.T) {
	svc := newSwitchService(t, spanishDetector(), langpref.New())
	nineteen := "Hola, como estas ok" // 19 code points
	if n := utf8.RuneCountInString(nineteen); n != 19 {
		t.Fatalf("fixture is %d code points, want 19", n)
	}
	twenty := nineteen + "?"
	for _, tc := range []struct {
		name string
		text string
		want bool
	}{
		{"19", nineteen, false},
		{"20", twenty, true},
		{"19 padded with whitespace", "   " + nineteen + "\n\n", false},
		{"20 padded with whitespace", "\t" + twenty + "  ", true},
	} {
		got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: tc.text, From: model.EN, To: model.FR})
		if got.Switched != tc.want {
			t.Errorf("%s: Switched = %v, want %v", tc.name, got.Switched, tc.want)
		}
	}
}

func TestPlanSourceSwitchCountsCodePointsNotBytes(t *testing.T) {
	det := &fakeDetector{lang: "zh-Hans", conf: 0.99, ok: true}
	svc := newSwitchService(t, det, langpref.New())
	// 10 Chinese characters are 30 bytes but 10 code points: too short.
	if got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: "今天天气很好我们去玩", From: model.EN, To: model.FR}); got.Switched {
		t.Errorf("10 code points (30 bytes) switched: %+v", got)
	}
	got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: strings.Repeat("今天天气很好", 4), From: model.EN, To: model.FR})
	if !got.Switched || !got.From.SameAs(model.ZH) || got.To != model.EN {
		t.Errorf("24 Chinese code points: got %+v, want zh -> en", got)
	}
}

func TestPlanSourceSwitchLowConfidenceNeverSwitches(t *testing.T) {
	for _, conf := range []float64{0, 0.3, 0.79} {
		det := &fakeDetector{lang: "es", conf: conf, ok: true}
		svc := newSwitchService(t, det, langpref.New())
		if got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.FR}); got.Switched {
			t.Errorf("confidence %.2f switched: %+v", conf, got)
		}
	}
	det := &fakeDetector{lang: "es", conf: 0.8, ok: true}
	svc := newSwitchService(t, det, langpref.New())
	if got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.FR}); !got.Switched {
		t.Error("confidence 0.80 (the threshold) did not switch")
	}
}

func TestPlanSourceSwitchNoUsableDetectionNeverSwitches(t *testing.T) {
	for name, det := range map[string]*fakeDetector{
		"detector unavailable":  {ok: false},
		"empty language":        {lang: "", conf: 1, ok: true},
		"auto":                  {lang: model.Auto, conf: 1, ok: true},
		"unrecognized language": {lang: "xx", conf: 1, ok: true},
	} {
		svc := newSwitchService(t, det, langpref.New())
		if got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.FR}); got.Switched {
			t.Errorf("%s switched: %+v", name, got)
		}
	}
}

func TestPlanSourceSwitchAutoSourceNeverSwitches(t *testing.T) {
	det := spanishDetector()
	svc := newSwitchService(t, det, langpref.New())
	for _, from := range []model.Language{model.Auto, "", "AUTO"} {
		if got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish, From: from, To: model.EN}); got.Switched {
			t.Errorf("source %q switched: %+v", from, got)
		}
	}
	if det.calls != 0 {
		t.Errorf("the detector ran %d times for an Auto source, want 0", det.calls)
	}
}

func TestPlanSourceSwitchSettingOffNeverSwitches(t *testing.T) {
	dir := t.TempDir()
	st, err := settings.NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	det := spanishDetector()
	svc := NewService(engine.NewRegistry(), nil, st, nil)
	svc.detect = det.detect
	req := model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.FR}
	if got := svc.PlanSourceSwitch(req); !got.Switched {
		t.Fatalf("default settings (setting on) did not switch: %+v", got)
	}
	st.Get().AutoSwitchSource = false
	det.calls = 0
	if got := svc.PlanSourceSwitch(req); got.Switched {
		t.Fatalf("setting off still switched: %+v", got)
	}
	if det.calls != 0 {
		t.Errorf("the detector ran %d times with the setting off, want 0", det.calls)
	}
}

func TestPlanSourceSwitchEngineHintIsOnlyTheFallback(t *testing.T) {
	// The #162 fallback: a result already carries the engine's detection. It is used only when the
	// local detector has none (no bridge, or nothing recognized); the engine reports no confidence,
	// so it must never override the local detector's verdict, low confidence included. The length
	// floor applies either way.
	req := model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.FR, Detected: model.ESMX}

	unavailable := newSwitchService(t, &fakeDetector{ok: false}, langpref.New())
	got := unavailable.PlanSourceSwitch(req)
	if !got.Switched || got.From != model.ESMX || got.To != model.EN {
		t.Fatalf("detector unavailable: got %+v, want es-MX -> en from the hint", got)
	}

	unsure := newSwitchService(t, &fakeDetector{lang: "es", conf: 0.4, ok: true}, langpref.New())
	if got := unsure.PlanSourceSwitch(req); got.Switched {
		t.Errorf("a low-confidence local detection was overridden by the engine hint: %+v", got)
	}

	if short := unavailable.PlanSourceSwitch(model.SourceSwitchRequest{Text: "Hola amigo", From: model.EN, To: model.FR, Detected: model.ESMX}); short.Switched {
		t.Errorf("short text switched on a hint: %+v", short)
	}
	if junk := unavailable.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.FR, Detected: "xx"}); junk.Switched {
		t.Errorf("an unrecognized hint switched: %+v", junk)
	}
}

func TestPlanSourceSwitchDetectorSeesBoundedText(t *testing.T) {
	det := spanishDetector()
	svc := newSwitchService(t, det, langpref.New())
	huge := strings.Repeat("é", 200_000)
	svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: huge, From: model.EN, To: model.FR})
	if n := utf8.RuneCountInString(det.seen); n == 0 || n > 4000 {
		t.Errorf("detector was given %d code points, want between 1 and 4000", n)
	}
}

func TestPlanSourceSwitchNeverLearnsOrPersists(t *testing.T) {
	// The switch is not a choice the user made: it must not teach the variant store (only a select's
	// own pick does) and must not touch the persisted default pair.
	dir := t.TempDir()
	st, err := settings.NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	wantFrom, wantTo := st.Get().DefaultFrom, st.Get().DefaultTo
	prefs := langpref.New()
	svc := NewService(engine.NewRegistry(), nil, st, nil)
	svc.detect = spanishDetector().detect
	svc.SetLangPrefs(prefs)
	got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.ESMX})
	if !got.Switched {
		t.Fatal("expected a switch")
	}
	if q := prefs.Qualify(model.Language("es")); q != "es" {
		t.Errorf("the variant store learned %q from an automatic switch, want none", q)
	}
	if st.Get().DefaultFrom != wantFrom || st.Get().DefaultTo != wantTo {
		t.Errorf("default pair changed to %s -> %s", st.Get().DefaultFrom, st.Get().DefaultTo)
	}
}
