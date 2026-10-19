package translate

import (
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"cnb.cool/dtapp/kai/internal/buildinfo"
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

// Issue #8 (the Alt-A item): a capture of a word or two of Mexican Spanish sat under the
// code-point floor and was refused before the detector was even asked. A CONFIDENT local
// detection now switches at any length — the detector's own certainty is the guard — so these
// short captures switch.
func TestPlanSourceSwitchShortCaptureWithConfidentDetectionSwitches(t *testing.T) {
	svc := newSwitchService(t, spanishDetector(), langpref.New())
	for name, text := range map[string]string{
		"11 code points": "Buenos días",
		"7 code points":  "Ahorita",
		"padded":         "   Buenos días\n\n",
	} {
		got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: text, From: model.EN, To: model.FR})
		if !got.Switched || got.From != model.ESMX || got.To != model.EN {
			t.Errorf("%s: got %+v, want es-MX -> en (a confident detection has no floor)", name, got)
		}
	}
}

// The floor still guards every answer that carries no confidence of its own (#8): an engine hint
// (no local detector), and a local detection under the confidence bar — including the #54
// promotion. There a length guess is the only guard, as before.
func TestPlanSourceSwitchFloorGuardsAnswersWithoutConfidence(t *testing.T) {
	eleven := "Hola, amigo" // 11 code points
	if n := utf8.RuneCountInString(eleven); n != 11 {
		t.Fatalf("fixture is %d code points, want 11", n)
	}
	twelve := eleven + "s"
	// The hint path (detector unavailable, the engine's detection in req.Detected): the floor
	// decides, exactly as before.
	for _, tc := range []struct {
		name string
		text string
		want bool
	}{
		{"11", eleven, false},
		{"12", twelve, true},
		{"11 padded with whitespace", "   " + eleven + "\n\n", false},
		{"12 padded with whitespace", "\t" + twelve + "  ", true},
	} {
		svc := newSwitchService(t, &fakeDetector{ok: false}, langpref.New())
		got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: tc.text, From: model.EN, To: model.FR, Detected: model.ESMX})
		if got.Switched != tc.want {
			t.Errorf("hint, %s: Switched = %v (reason %q), want %v", tc.name, got.Switched, got.Reason, tc.want)
		}
		if !tc.want && got.Reason != model.SwitchReasonTooShort {
			t.Errorf("hint, %s: Reason = %q, want too_short", tc.name, got.Reason)
		}
	}
	// A local detection under the confidence bar on short text: too_short, not below_confidence
	// (the floor still names the reason there is no switch).
	svc := newSwitchService(t, &fakeDetector{lang: "es", conf: 0.5, ok: true}, langpref.New())
	if got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: eleven, From: model.EN, To: model.FR}); got.Switched || got.Reason != model.SwitchReasonTooShort {
		t.Errorf("short low-confidence local: got %+v, want too_short and no switch", got)
	}
	got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: twelve, From: model.EN, To: model.FR})
	if got.Switched || got.Reason != model.SwitchReasonBelowConfidence {
		t.Errorf("long low-confidence local: got %+v, want below_confidence and no switch", got)
	}
	// Empty text is nothing to detect, whatever a broken detector might claim.
	lying := spanishDetector()
	svc = newSwitchService(t, lying, langpref.New())
	if got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: "   \t ", From: model.EN, To: model.FR}); got.Switched || got.Reason != model.SwitchReasonTooShort || lying.calls != 0 {
		t.Errorf("blank text: got %+v (detector ran %d times), want too_short and no detection", got, lying.calls)
	}
}

// A short English phrase under a Spanish pin must switch, not be "translated" from Spanish to
// English and handed back unchanged.
func TestPlanSourceSwitchShortEnglishUnderSpanishPin(t *testing.T) {
	det := &fakeDetector{lang: "en", conf: 0.99, ok: true}
	svc := newSwitchService(t, det, langpref.New())
	got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: "In the meantime, ", From: model.ESMX, To: model.EN})
	if !got.Switched || got.From != model.EN || got.To != model.ESMX {
		t.Fatalf("got %+v, want en -> es-MX", got)
	}
}

func TestPlanSourceSwitchCountsCodePointsNotBytes(t *testing.T) {
	// The floor — which guards the no-confidence answers (#8) — counts code points, not bytes:
	// 10 Chinese characters are 30 bytes but 10 code points, under it. A confident local detection
	// has no floor at all, so the hint path is where the counting shows.
	svc := newSwitchService(t, &fakeDetector{ok: false}, langpref.New())
	if got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: "今天天气很好我们去玩", From: model.EN, To: model.FR, Detected: model.ZH}); got.Switched {
		t.Errorf("10 code points (30 bytes) switched: %+v", got)
	}
	got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: strings.Repeat("今天天气很好", 4), From: model.EN, To: model.FR, Detected: model.ZH})
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

// Issue #16: every "no switch" used to be silent. The plan now says why (the wire values are the
// contract the frontend logs), and one info line per decision carries the detection, never the text.

func TestPlanSourceSwitchReportsTheReason(t *testing.T) {
	dir := t.TempDir()
	st, err := settings.NewService(dir)
	if err != nil {
		t.Fatal(err)
	}
	mk := func(det *fakeDetector) *Service {
		svc := NewService(engine.NewRegistry(), nil, st, nil)
		svc.detect = det.detect
		return svc
	}
	req := model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.FR}
	for _, tc := range []struct {
		name string
		svc  *Service
		req  model.SourceSwitchRequest
		want string
	}{
		{"switched", mk(spanishDetector()), req, "switched"},
		{"too short", mk(&fakeDetector{ok: false}), model.SourceSwitchRequest{Text: "Hola amigo", From: model.EN, To: model.FR}, "too_short"},
		{"auto source", mk(spanishDetector()), model.SourceSwitchRequest{Text: spanish, From: model.Auto, To: model.EN}, "source_auto"},
		{"same language", mk(spanishDetector()), model.SourceSwitchRequest{Text: spanish, From: model.ESMX, To: model.EN}, "same_language"},
		{"below confidence", mk(&fakeDetector{lang: "es", conf: 0.5, ok: true}), req, "below_confidence"},
		{"no detection", mk(&fakeDetector{ok: false}), req, "no_detection"},
		{"unrecognized detection", mk(&fakeDetector{lang: "xx", conf: 0.99, ok: true}), req, "no_detection"},
	} {
		if got := tc.svc.PlanSourceSwitch(tc.req); got.Reason != tc.want {
			t.Errorf("%s: Reason = %q, want %q (plan %+v)", tc.name, got.Reason, tc.want, got)
		}
	}
	st.Get().AutoSwitchSource = false
	if got := mk(spanishDetector()).PlanSourceSwitch(req); got.Reason != "disabled" || got.Switched {
		t.Errorf("setting off: got %+v, want reason disabled", got)
	}
}

// captureLog routes slog to a buffer for the test and returns it.
func captureLog(t *testing.T) *strings.Builder {
	t.Helper()
	var buf strings.Builder
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// Issue #16: the decision line names the build, so a kai.log pasted after an intermittent miss says
// which version produced it, also when the startup line is in another day's file.
func TestPlanSourceSwitchLogLineNamesTheBuild(t *testing.T) {
	oldV, oldC := buildinfo.Version, buildinfo.GitCommit
	t.Cleanup(func() { buildinfo.Version, buildinfo.GitCommit = oldV, oldC })
	buildinfo.Version, buildinfo.GitCommit = "v7.7.7", "feedbee"
	buf := captureLog(t)
	svc := newSwitchService(t, spanishDetector(), langpref.New())
	svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: "Hola, necesito que me ayudes con este documento hoy", From: model.EN, To: model.FR})
	for _, want := range []string{"version=v7.7.7", "commit=feedbee"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("decision line lacks %q: %s", want, buf.String())
		}
	}
}

func TestPlanSourceSwitchLogsOneInfoLinePerDecisionWithoutContent(t *testing.T) {
	selection := "Mi contrasena secreta es LibelulaAzul y no debe aparecer en ningun registro"
	buf := captureLog(t)
	svc := newSwitchService(t, &fakeDetector{lang: "es", conf: 0.9123, ok: true}, langpref.New())
	svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: selection, From: model.EN, To: model.FR})
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("got %d log lines, want exactly 1:\n%s", len(lines), buf.String())
	}
	line := lines[0]
	n := utf8.RuneCountInString(selection)
	for _, want := range []string{"level=INFO", "reason=switched", "detected=es", "confidence=0.912", "runes=" + strconv.Itoa(n), "by=local"} {
		if !strings.Contains(line, want) {
			t.Errorf("log line lacks %q: %s", want, line)
		}
	}
	for _, leak := range []string{"LibelulaAzul", "contrasena", "Mi contrasena"} {
		if strings.Contains(line, leak) {
			t.Errorf("log line leaks the text (%q): %s", leak, line)
		}
	}
}

// Issue #8 (#16's chosen deliverable): a miss must be diagnosable from the log alone. The decision
// line carries the whole story — the detection, the pair the window shows (from/to), and, when it
// switched, the pair it chose (result_from -> result_to); a miss carries the reason instead and
// names no chosen pair.
func TestPlanSourceSwitchLogNamesTheChosenPair(t *testing.T) {
	buf := captureLog(t)
	svc := newSwitchService(t, spanishDetector(), langpref.New())
	svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.FR})
	for _, want := range []string{"reason=switched", "from=en", "to=fr", "result_from=es-MX", "result_to=en"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("a switch's log line lacks %q: %s", want, buf.String())
		}
	}

	buf = captureLog(t)
	svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish, From: model.ESMX, To: model.EN})
	out := buf.String()
	if !strings.Contains(out, "reason=same_language") {
		t.Errorf("a miss's log line lacks its reason: %s", out)
	}
	if strings.Contains(out, "result_from=es") {
		t.Errorf("a miss names a chosen pair: %s", out)
	}
}

func TestPlanSourceSwitchLogNamesWhoAnsweredAndWhyNot(t *testing.T) {
	for _, tc := range []struct {
		name string
		det  *fakeDetector
		req  model.SourceSwitchRequest
		want []string
	}{
		{"hint answered", &fakeDetector{ok: false}, model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.FR, Detected: model.ESMX}, []string{"by=hint", "detected=es-MX", "reason=switched"}},
		{"below confidence", &fakeDetector{lang: "es", conf: 0.5, ok: true}, model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.FR}, []string{"by=local", "detected=es", "confidence=0.5", "reason=below_confidence"}},
		{"too short", &fakeDetector{ok: false}, model.SourceSwitchRequest{Text: "Hola amigo", From: model.EN, To: model.FR}, []string{"reason=too_short", "runes=10"}},
		{"nothing detected", &fakeDetector{ok: false}, model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.FR}, []string{"by=none", "reason=no_detection"}},
		{"auto source", spanishDetector(), model.SourceSwitchRequest{Text: spanish, From: model.Auto, To: model.EN}, []string{"reason=source_auto"}},
	} {
		buf := captureLog(t)
		newSwitchService(t, tc.det, langpref.New()).PlanSourceSwitch(tc.req)
		for _, w := range tc.want {
			if !strings.Contains(buf.String(), w) {
				t.Errorf("%s: log lacks %q: %s", tc.name, w, buf.String())
			}
		}
	}
}

func TestPlanSourceSwitchIgnoresLinks(t *testing.T) {
	// A link is not text in a language: the detector never sees it and it counts toward no floor.
	det := spanishDetector()
	svc := newSwitchService(t, det, langpref.New())
	link := "https://www.ejemplo.com/noticias/2026/10/articulo-largo?id=12345"
	got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: link, From: model.EN, To: model.FR})
	if got.Switched || got.Reason != "too_short" || det.calls != 0 {
		t.Errorf("a link alone: got %+v (detector ran %d times), want too_short and no detection", got, det.calls)
	}
	got = svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: "Mira esto: " + link + " www.otro.mx/pagina", From: model.EN, To: model.FR})
	// #8: the few words that are left (10 code points) carry the language on their own once the
	// detector is sure — a confident detection has no floor.
	if !got.Switched || strings.Contains(det.seen, "ejemplo.com") {
		t.Errorf("a few words and links: got %+v, detector saw %q; want a switch on the words alone", got, det.seen)
	}
	// Without a local detector the floor still decides what is left after the links are gone.
	hint := newSwitchService(t, &fakeDetector{ok: false}, langpref.New())
	if got := hint.PlanSourceSwitch(model.SourceSwitchRequest{Text: "Mira esto: " + link, From: model.EN, To: model.FR, Detected: model.ESMX}); got.Switched || got.Reason != model.SwitchReasonTooShort {
		t.Errorf("a few words and links on a hint: got %+v, want too_short", got)
	}
	got = svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish + " " + link, From: model.EN, To: model.FR})
	if !got.Switched || strings.Contains(det.seen, "ejemplo.com") {
		t.Errorf("text plus a link: got %+v, detector saw %q; want a switch on the text alone", got, det.seen)
	}
}

// Issue #54: a text in the TARGET language, under a pinned source that is another language, would
// come back as an identity result ("source and target are the same"). There a wrong switch costs
// little and not switching is certainly useless, so the detector's confidence bar is lower than the
// general 0.8; every other case keeps 0.8.
func TestPlanSourceSwitchTextInTheTargetLanguageNeedsLessConfidence(t *testing.T) {
	text := `Verifíca todos los "tool tips"`
	for _, tc := range []struct {
		name string
		conf float64
		to   model.Language
		want bool
	}{
		{"detected = target, 0.74", 0.74, model.ESMX, true},
		{"detected = target, exactly at the lower bar", switchMinConfidenceTarget, model.ESMX, true},
		{"detected = target, just under the lower bar", switchMinConfidenceTarget - 0.01, model.ESMX, false},
		{"detected = target, bare es covers es-MX", 0.6, model.ES, true},
		{"detected is not the target, 0.74 stays below the general bar", 0.74, model.FR, false},
		{"detected is not the target, at the general bar", switchMinConfidence, model.FR, true},
	} {
		svc := newSwitchService(t, &fakeDetector{lang: "es", conf: tc.conf, ok: true}, langpref.New())
		got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: text, From: model.EN, To: tc.to})
		if got.Switched != tc.want {
			t.Errorf("%s: Switched = %v (reason %q), want %v", tc.name, got.Switched, got.Reason, tc.want)
		}
	}
}

func TestPlanSourceSwitchLowerBarSwapsThePair(t *testing.T) {
	svc := newSwitchService(t, &fakeDetector{lang: "es", conf: 0.74, ok: true}, langpref.New())
	got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: `Verifíca todos los "tool tips"`, From: model.EN, To: model.ESMX})
	if !got.Switched || got.From != model.ESMX || got.To != model.EN || got.Reason != model.SwitchReasonSwitched {
		t.Fatalf("got %+v, want es-MX -> en, switched", got)
	}
}

// The lower bar never overrides a text that already is in the pinned language, and never turns a
// below-bar hint or unrecognised language into a switch.
func TestPlanSourceSwitchLowerBarLeavesTheOtherRulesAlone(t *testing.T) {
	// Detected = the pinned source (es-MX), at a low confidence: nothing to switch.
	svc := newSwitchService(t, &fakeDetector{lang: "es", conf: 0.6, ok: true}, langpref.New())
	if got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish, From: model.ESMX, To: model.EN}); got.Switched {
		t.Errorf("text in the pinned language switched: %+v", got)
	}
	// A language the app does not know, even if it is "the target" by code, is not recognised.
	svc = newSwitchService(t, &fakeDetector{lang: "xx", conf: 0.6, ok: true}, langpref.New())
	if got := svc.PlanSourceSwitch(model.SourceSwitchRequest{Text: spanish, From: model.EN, To: model.ESMX}); got.Switched {
		t.Errorf("an unrecognised language switched: %+v", got)
	}
}
