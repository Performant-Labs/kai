package translate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/langpref"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #161 (T, RED): a pinned source language that does not match the text is corrected by the
// backend, from ONE engine call. When the pin is at least 20 code points of trimmed text long,
// translateWithEngine dispatches the call with From substituted to auto, compares the engine's
// detection against the ORIGINAL pin (SameAs / Covers, behind a ParseLanguage recognition gate),
// and reports:
//
//   - a real mismatch: From and DetectedFrom both hold s.resultFrom(model.Auto, detected), the
//     qualified detection, and the translation is the one the substituted call produced;
//   - a detection that covers the target: an identity result, From qualified the same way,
//     DetectedFrom empty (the identity note already explains it);
//   - a match, or no usable detection: From is the pin, DetectedFrom empty.
//
// A genuine auto request also reports its recognized detection on DetectedFrom (no length floor).
//
// DetectedFrom is read through the JSON contract the frontend consumes ("detected_from",
// omitempty), so this file compiles before the field exists and fails on the assertion instead.
//
// Single-call cases go through the real google engine against a loopback gtx server that records
// every request's sl (source) and tl (target), so "substituted to auto" and "exactly one call" are
// facts observed on the wire. Chunked cases use the scripted engine from service_chunk_test.go,
// as the #84 chunk tests do.

// longEN is English text well over the 20-code-point floor.
const longEN = "The quick brown fox jumps over the lazy dog."

// slGtx is a loopback gtx endpoint that records the sl and tl of every request and answers with a
// fixed translation and a fixed detected-language slot (raw JSON: `"en"`, `"auto"`, `null`).
type slGtx struct {
	mu       sync.Mutex
	sls, tls []string
	detected string
	result   string
}

func (g *slGtx) calls() (sls, tls []string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.sls...), append([]string(nil), g.tls...)
}

// newSLService wires a real Service (history + config store under t.TempDir()) to the real google
// engine over a loopback slGtx. prefs, when non-nil, is installed as the variant preference store.
func newSLService(t *testing.T, detectedJSON, result string, prefs *langpref.Store) (*Service, *slGtx) {
	t.Helper()
	g := &slGtx{detected: detectedJSON, result: result}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q, _ := url.ParseQuery(r.URL.RawQuery)
		g.mu.Lock()
		g.sls = append(g.sls, q.Get("sl"))
		g.tls = append(g.tls, q.Get("tl"))
		g.mu.Unlock()
		_, _ = w.Write([]byte(`[[["` + g.result + `","src",null,null,1]],null,` + g.detected + `]`))
	}))
	t.Cleanup(srv.Close)
	svc, _ := newIdentityServiceWith(t, engine.NewGoogle(srv.URL, srv.Client()))
	if prefs != nil {
		svc.SetLangPrefs(prefs)
	}
	return svc, g
}

// detectedFromOf reads the result's "detected_from" through its JSON encoding ("" when absent).
func detectedFromOf(t *testing.T, res model.TranslateResult) string {
	t.Helper()
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	v, ok := m["detected_from"]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// hasDetectedFromKey reports whether the JSON encoding carries the key at all (omitempty).
func hasDetectedFromKey(t *testing.T, res model.TranslateResult) bool {
	t.Helper()
	b, _ := json.Marshal(res)
	return strings.Contains(string(b), `"detected_from"`)
}

// oneAutoCall asserts the fixture saw exactly one request, sent with sl=auto.
func oneAutoCall(t *testing.T, g *slGtx) {
	t.Helper()
	sls, _ := g.calls()
	if len(sls) != 1 {
		t.Fatalf("engine called %d times, want exactly 1 (no re-run, no second call); sl=%q", len(sls), sls)
	}
	if sls[0] != "auto" {
		t.Errorf("engine was sent sl=%q, want auto (a pinned request over the floor is dispatched as auto)", sls[0])
	}
}

func translatePinned(t *testing.T, svc *Service, text string, from, to model.Language) model.TranslateResult {
	t.Helper()
	res, err := svc.Translate(model.TranslateRequest{Text: text, From: from, To: to, EngineName: "google"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	return *res
}

// ---- real mismatch ------------------------------------------------------------------------

// The reported shape, with a target that is not the detected language: pin es-MX, English text,
// target zh. One call, sent as auto; the translation the engine produced is used as-is; From and
// DetectedFrom both report the qualified detection. The second case stores a variant preference
// (pt → pt-BR) so the qualified value (pt-BR) and the bare detection (pt) actually differ.
func TestPinnedMismatchIsCorrectedFromOneCall(t *testing.T) {
	ptPrefs := langpref.New()
	ptPrefs.Learn(model.PTBR)
	cases := []struct {
		name     string
		pin, to  model.Language
		detected string
		prefs    *langpref.Store
		text     string
		want     model.Language // the qualified detection
	}{
		{"es-MX pin, English text", model.ESMX, model.ZH, "en", nil, longEN, model.EN},
		{"fr pin, Portuguese text, pt-BR preference", model.FR, model.EN, "pt", ptPrefs, "O rato roeu a roupa do rei de Roma.", model.PTBR},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, g := newSLService(t, `"`+tc.detected+`"`, "TRANSLATED", tc.prefs)
			res := translatePinned(t, svc, tc.text, tc.pin, tc.to)
			oneAutoCall(t, g)
			if want := svc.resultFrom(model.Auto, model.Language(tc.detected)); want != tc.want {
				t.Fatalf("setup: resultFrom(auto, %q) = %q, want %q", tc.detected, want, tc.want)
			}
			if res.From != tc.want {
				t.Errorf("From = %q, want %q (the qualified detection, not the pin %q)", res.From, tc.want, tc.pin)
			}
			if got := detectedFromOf(t, res); got != string(tc.want) {
				t.Errorf("detected_from = %q, want %q (qualified, same value as From)", got, tc.want)
			}
			if got := detectedFromOf(t, res); got != string(res.From) {
				t.Errorf("detected_from = %q differs from From %q; both must hold the qualified detection", got, res.From)
			}
			if res.Result != "TRANSLATED" || res.Identity || res.To != tc.to {
				t.Errorf("result = %+v, want the substituted call's translation, not identity, To %q", res, tc.to)
			}
		})
	}
}

// The substituted-pinned path and the genuine-auto path store the identical qualified shape on
// detected_from for the same detection (never bare on one and qualified on the other).
func TestDetectedFromSameShapeOnBothPaths(t *testing.T) {
	prefs := func() *langpref.Store { s := langpref.New(); s.Learn(model.ESMX); return s }
	const esText = "¿Dónde está la biblioteca más cercana, por favor?"

	pinSvc, _ := newSLService(t, `"es"`, "TRANSLATED", prefs())
	pinned := translatePinned(t, pinSvc, esText, model.FR, model.EN)

	autoSvc, _ := newSLService(t, `"es"`, "TRANSLATED", prefs())
	auto := translatePinned(t, autoSvc, esText, model.Auto, model.EN)

	if got := detectedFromOf(t, pinned); got != string(model.ESMX) {
		t.Errorf("substituted-pinned detected_from = %q, want es-MX (qualified)", got)
	}
	if got := detectedFromOf(t, auto); got != string(model.ESMX) {
		t.Errorf("genuine-auto detected_from = %q, want es-MX (qualified)", got)
	}
	if detectedFromOf(t, pinned) != detectedFromOf(t, auto) || pinned.From != auto.From {
		t.Errorf("pinned {From %q, detected_from %q} vs auto {From %q, detected_from %q}: the two paths must agree",
			pinned.From, detectedFromOf(t, pinned), auto.From, detectedFromOf(t, auto))
	}
}

// ---- detection matches the pin ------------------------------------------------------------

// The pin was right: the call still goes out as auto (one call), but From reports the pin
// unchanged and there is no correction. The dialect case (pin es-MX, detection bare es) is a match
// by SameAs, not a correction to es.
func TestPinnedDetectionMatchingPinIsNotACorrection(t *testing.T) {
	cases := []struct {
		name     string
		pin      model.Language
		detected string
		text     string
	}{
		{"exact", model.FR, "fr", "Le renard brun rapide saute par-dessus le chien."},
		{"dialect via SameAs", model.ESMX, "es", "¿Dónde está la biblioteca más cercana, por favor?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// A preference that would qualify a bare es to es-419-ish forms is irrelevant here; use
			// none, so a wrong implementation reporting the bare detection shows up as "es".
			svc, g := newSLService(t, `"`+tc.detected+`"`, "TRANSLATED", langpref.New())
			res := translatePinned(t, svc, tc.text, tc.pin, model.EN)
			oneAutoCall(t, g)
			if res.From != tc.pin {
				t.Errorf("From = %q, want the pin %q unchanged", res.From, tc.pin)
			}
			if hasDetectedFromKey(t, res) {
				t.Errorf("detected_from present (%q), want it absent: the pin matched", detectedFromOf(t, res))
			}
			if res.Result != "TRANSLATED" || res.Identity {
				t.Errorf("result = %+v, want the engine's translation, not identity", res)
			}
		})
	}
}

// ---- detection covers the target (the reported bug) ---------------------------------------

// Pin es-MX, English text, target en: the substituted call detects en, which covers the target,
// so the result is an identity result built by s.identityResult, From qualified through
// resultFrom(model.Auto, detected) exactly as a genuine auto identity result is (never the wrong
// pin), detected_from empty. The second case stores es → es-MX so the qualification is visible.
func TestPinnedDetectionCoveringTargetIsIdentity(t *testing.T) {
	esPrefs := langpref.New()
	esPrefs.Learn(model.ESMX)
	cases := []struct {
		name     string
		pin, to  model.Language
		detected model.Language
		prefs    *langpref.Store
		text     string
		want     model.Language
	}{
		{"reported bug: es-MX pin, English text, target en", model.ESMX, model.EN, model.EN, nil, longEN, model.EN},
		{"fr pin, Spanish text, target es, es-MX preference", model.FR, model.ES, model.ES, esPrefs,
			"¿Dónde está la biblioteca más cercana, por favor?", model.ESMX},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, g := newSLService(t, `"`+string(tc.detected)+`"`, "ENGINE ECHO", tc.prefs)
			res := translatePinned(t, svc, tc.text, tc.pin, tc.to)
			oneAutoCall(t, g)
			if !res.Identity {
				t.Fatalf("Identity = false, want an identity result (detection %q covers target %q); result %+v", tc.detected, tc.to, res)
			}
			if res.Result != tc.text || res.Text != tc.text {
				t.Errorf("Result = %q, want the source text %q (the engine's echo is not used)", res.Result, tc.text)
			}
			want := svc.resultFrom(model.Auto, tc.detected)
			if want != tc.want {
				t.Fatalf("setup: resultFrom(auto, %q) = %q, want %q", tc.detected, want, tc.want)
			}
			if res.From != want {
				t.Errorf("From = %q, want %q = resultFrom(auto, %q) (not the wrong pin %q)", res.From, want, tc.detected, tc.pin)
			}
			if hasDetectedFromKey(t, res) {
				t.Errorf("detected_from = %q on an identity result, want absent (the identity note explains it)", detectedFromOf(t, res))
			}
			if res.To != tc.to {
				t.Errorf("To = %q, want the requested %q", res.To, tc.to)
			}
		})
	}
}

// ---- no usable detection on the substituted call -------------------------------------------

// Two distinct gates, one outcome. (i) The engine echoes auto: detectedSource's own ok=false.
// (ii) The engine returns its own unrecognized native code (baidu's "jp"): detectedSource says
// ok=true, and the separate model.ParseLanguage recognition gate rejects it — without that gate a
// Japanese pin would be falsely "corrected" to "jp" (SameAs("jp","ja") is false). Both fall back
// to the pin: From is the pin, detected_from absent, the engine's translation used.
func TestPinnedNoUsableDetectionFallsBackToPin(t *testing.T) {
	cases := []struct {
		name, detectedJSON string
	}{
		{"auto echoed (detectedSource ok=false)", `"auto"`},
		{"unrecognized native code jp (ParseLanguage gate)", `"jp"`},
		{"no detection slot", `null`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, g := newSLService(t, tc.detectedJSON, "TRANSLATED", langpref.New())
			res := translatePinned(t, svc, "東京の天気は今日とても良いです。散歩に行きましょう。", model.JA, model.EN)
			oneAutoCall(t, g)
			if res.From != model.JA {
				t.Errorf("From = %q, want the pin ja", res.From)
			}
			if hasDetectedFromKey(t, res) {
				t.Errorf("detected_from = %q, want absent (no usable detection, no correction)", detectedFromOf(t, res))
			}
			if res.Identity || res.Result != "TRANSLATED" {
				t.Errorf("result = %+v, want the engine's translation", res)
			}
		})
	}
}

// ---- the 20-code-point floor, on the TRIMMED text ------------------------------------------

// Below the floor the pin is sent as given, exactly as today: the wire sees the pin, not auto,
// and nothing is corrected even though the engine reports another language. The padded case is
// ~11 code points of text inside whitespace that takes it past 20 untrimmed: the gate measures
// trimmed text. The CJK case is 19 code points (57 bytes): the gate counts code points, not bytes.
func TestPinnedShortTextIsNotSubstituted(t *testing.T) {
	cases := []struct{ name, text string }{
		{"short", "Hello there"},
		{"whitespace-padded short", "        Hello there        \n\n  "},
		{"19 code points, 57 bytes", strings.Repeat("天", 19)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, g := newSLService(t, `"en"`, "TRANSLATED", langpref.New())
			res := translatePinned(t, svc, tc.text, model.ESMX, model.ZH)
			sls, _ := g.calls()
			if len(sls) != 1 || sls[0] == "auto" || sls[0] == "" {
				t.Fatalf("sl sent = %q, want exactly one call carrying the pin (not auto)", sls)
			}
			if res.From != model.ESMX {
				t.Errorf("From = %q, want the pin es-MX (no correction below the floor)", res.From)
			}
			if hasDetectedFromKey(t, res) {
				t.Errorf("detected_from = %q, want absent below the floor", detectedFromOf(t, res))
			}
		})
	}
}

// Exactly 20 trimmed code points is at the floor, so it is substituted (the gate is >= 20). The
// text is 20 CJK code points inside whitespace.
func TestPinnedTextAtFloorIsSubstituted(t *testing.T) {
	svc, g := newSLService(t, `"zh"`, "TRANSLATED", langpref.New())
	res := translatePinned(t, svc, "  "+strings.Repeat("天", 20)+"  ", model.JA, model.EN)
	oneAutoCall(t, g)
	if res.From != model.ZH || detectedFromOf(t, res) != string(model.ZH) {
		t.Errorf("From = %q, detected_from = %q, want zh / zh (a correction at the floor)", res.From, detectedFromOf(t, res))
	}
}

// ---- genuine auto: detected_from with no floor --------------------------------------------

// A genuine auto request reports its recognized detection on detected_from, qualified, whatever
// the text's length (the floor gates overriding a pin, not showing what auto saw). Nothing
// detected, or an identity result, leaves it absent.
func TestAutoResultCarriesDetectedFromWithoutFloor(t *testing.T) {
	prefs := langpref.New()
	prefs.Learn(model.ESMX)
	svc, g := newSLService(t, `"es"`, "Hello", prefs)
	res := translatePinned(t, svc, "Hola", model.Auto, model.EN)
	if sls, _ := g.calls(); len(sls) != 1 || sls[0] != "auto" {
		t.Fatalf("sl = %q, want one auto call", sls)
	}
	if res.From != model.ESMX {
		t.Errorf("From = %q, want es-MX (unchanged #53 qualification)", res.From)
	}
	if got := detectedFromOf(t, res); got != string(model.ESMX) {
		t.Errorf("detected_from = %q, want es-MX (= From; no 20-code-point floor on auto)", got)
	}

	t.Run("nothing detected", func(t *testing.T) {
		svc, _ := newSLService(t, `null`, "Hello", nil)
		res := translatePinned(t, svc, "Hola", model.Auto, model.EN)
		if res.From != model.Auto || hasDetectedFromKey(t, res) {
			t.Errorf("From = %q, detected_from = %q, want auto and absent", res.From, detectedFromOf(t, res))
		}
	})
	t.Run("unrecognized native code", func(t *testing.T) {
		svc, _ := newSLService(t, `"jp"`, "Hello", nil)
		res := translatePinned(t, svc, "こんにちは", model.Auto, model.EN)
		if hasDetectedFromKey(t, res) {
			t.Errorf("detected_from = %q, want absent (jp is not a recognized language)", detectedFromOf(t, res))
		}
	})
	t.Run("identity", func(t *testing.T) {
		svc, _ := newSLService(t, `"en"`, "Hello", nil)
		res := translatePinned(t, svc, "Hello", model.Auto, model.EN)
		if !res.Identity || hasDetectedFromKey(t, res) {
			t.Errorf("Identity = %v, detected_from = %q, want identity and absent", res.Identity, detectedFromOf(t, res))
		}
	})
}

// ---- resultFrom is untouched -------------------------------------------------------------

// resultFrom keeps both of its existing branches: a pinned request value is returned verbatim
// whatever the engine reported, and an auto one is qualified. The correction is made by calling
// the auto branch (resultFrom(model.Auto, detected)), not by changing resultFrom.
func TestResultFromBranchesUnchanged(t *testing.T) {
	prefs := langpref.New()
	prefs.Learn(model.ESMX)
	svc := NewService(engine.NewRegistry(), nil, nil, nil)
	svc.SetLangPrefs(prefs)
	if got := svc.resultFrom(model.FR, model.ES); got != model.FR {
		t.Errorf("resultFrom(fr, es) = %q, want fr (a pinned value is reported as requested)", got)
	}
	if got := svc.resultFrom(model.Auto, model.ES); got != model.ESMX {
		t.Errorf("resultFrom(auto, es) = %q, want es-MX (qualified)", got)
	}
	if got := svc.resultFrom(model.Auto, model.Auto); got != model.Auto {
		t.Errorf("resultFrom(auto, auto) = %q, want auto", got)
	}
}

// ---- both entry points --------------------------------------------------------------------

// TranslateMulti's streamed payload carries the same correction as Translate: both go through
// translateWithEngine.
func TestTranslateMultiSurfacesCorrectionAndIdentity(t *testing.T) {
	run := func(t *testing.T, detected string, to model.Language, id string) model.TranslateResult {
		g := &slGtx{detected: `"` + detected + `"`, result: "TRANSLATED"}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			q, _ := url.ParseQuery(r.URL.RawQuery)
			g.mu.Lock()
			g.sls = append(g.sls, q.Get("sl"))
			g.mu.Unlock()
			_, _ = w.Write([]byte(`[[["` + g.result + `","src",null,null,1]],null,` + g.detected + `]`))
		}))
		t.Cleanup(srv.Close)
		svc, em, _ := newCancelService(t, engine.NewGoogle(srv.URL, srv.Client()))
		if _, err := svc.TranslateMulti(model.TranslateRequest{Text: longEN, From: model.ESMX, To: to, RequestID: id}); err != nil {
			t.Fatal(err)
		}
		waitUntil(t, "result of google", func() bool { return len(em.resultsFor(id, "google")) >= 1 })
		oneAutoCall(t, g)
		return em.resultsFor(id, "google")[0]
	}
	t.Run("correction", func(t *testing.T) {
		got := run(t, "en", model.ZH, "r-161-c")
		if got.From != model.EN || detectedFromOf(t, got) != string(model.EN) || got.Result != "TRANSLATED" {
			t.Errorf("payload = %+v (detected_from %q), want From en, detected_from en, the engine's translation", got, detectedFromOf(t, got))
		}
	})
	t.Run("identity", func(t *testing.T) {
		got := run(t, "en", model.EN, "r-161-i")
		if !got.Identity || got.From != model.EN || hasDetectedFromKey(t, got) || got.Result != longEN {
			t.Errorf("payload = %+v, want an identity result, From en, no detected_from", got)
		}
	})
}

// ---- chunked pinned requests (#84's chunk-1 decision, extended by pinFallback) -------------

// chunkedPinned runs a chunked Translate over chunkDoc(n) with the scripted engine answering
// chunk 1 with first as its reported From and every later chunk with later.
func chunkedPinned(t *testing.T, n int, pin, to model.Language, first, later model.Language, prefs *langpref.Store) (model.TranslateResult, []model.TranslateRequest, error) {
	t.Helper()
	eng := newScript("eng", func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error) {
		if idx == 1 {
			return &model.TranslateResult{Result: bracketed(req.Text), From: first}, nil
		}
		return &model.TranslateResult{Result: bracketed(req.Text), From: later}, nil
	})
	svc, _ := chunkedService(t, eng)
	if prefs != nil {
		svc.SetLangPrefs(prefs)
	}
	res, err := svc.Translate(model.TranslateRequest{Text: chunkDoc(n), From: pin, To: to, EngineName: "eng"})
	var out model.TranslateResult
	if res != nil {
		out = *res
	}
	return out, eng.callList(), err
}

// chunkFroms maps each call's chunk number to the From it was sent with.
func chunkFroms(calls []model.TranslateRequest) map[int]model.Language {
	m := map[int]model.Language{}
	for _, c := range calls {
		m[paraIndex(c.Text)] = c.From
	}
	return m
}

// A chunked pinned request is decided once, from chunk 1 sent alone as auto: a recognized
// mismatch (en for an es-MX pin) is sent as From for every later chunk, even though the engine
// would report something else for them, and the result reports the correction.
func TestChunkedPinnedMismatchDecidedFromChunkOne(t *testing.T) {
	release := make(chan struct{})
	eng := newScript("eng", func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error) {
		if idx == 1 {
			<-release
			return &model.TranslateResult{Result: bracketed(req.Text), From: model.EN}, nil
		}
		return &model.TranslateResult{Result: bracketed(req.Text), From: model.FR}, nil
	})
	svc, _ := chunkedService(t, eng)
	done := make(chan struct{})
	var res *model.TranslateResult
	var err error
	go func() {
		defer close(done)
		res, err = svc.Translate(model.TranslateRequest{Text: chunkDoc(4), From: model.ESMX, To: model.ZH, EngineName: "eng"})
	}()
	waitUntil(t, "chunk 1 started", func() bool { return eng.hasStarted(1) })
	time.Sleep(30 * time.Millisecond)
	if n := len(eng.callList()); n != 1 {
		t.Errorf("%d calls while chunk 1 was running, want chunk 1 alone on a substituted pinned request", n)
	}
	close(release)
	<-done
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	froms := chunkFroms(eng.callList())
	if len(froms) != 4 {
		t.Fatalf("calls = %v, want 4 chunks", froms)
	}
	for idx, f := range froms {
		switch {
		case idx == 1 && f != model.Auto:
			t.Errorf("chunk 1 From = %q, want auto (substituted)", f)
		case idx > 1 && f != model.EN:
			t.Errorf("chunk %d From = %q, want en (chunk 1's detection, decided once)", idx, f)
		}
	}
	if res.From != model.EN || detectedFromOf(t, *res) != string(model.EN) {
		t.Errorf("From = %q, detected_from = %q, want en / en", res.From, detectedFromOf(t, *res))
	}
	if res.Result != bracketedDoc(1, 4) || res.Identity {
		t.Errorf("Result = %q, want the reassembled translation", res.Result)
	}
}

// Dialect preservation: pin es-MX, chunk 1 detects the bare es — a RECOGNIZED match by SameAs.
// Chunks 2..N must be sent with the pin's full es-MX, not downgraded to the bare es, and the
// result reports the pin with no correction.
func TestChunkedPinnedDialectMatchKeepsPinForLaterChunks(t *testing.T) {
	res, calls, err := chunkedPinned(t, 3, model.ESMX, model.EN, model.ES, model.ES, nil)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	froms := chunkFroms(calls)
	if froms[1] != model.Auto {
		t.Errorf("chunk 1 From = %q, want auto (substituted)", froms[1])
	}
	for idx := 2; idx <= 3; idx++ {
		if froms[idx] != model.ESMX {
			t.Errorf("chunk %d From = %q, want es-MX (the pin's dialect preserved, not the bare es)", idx, froms[idx])
		}
	}
	if res.From != model.ESMX || hasDetectedFromKey(t, res) {
		t.Errorf("From = %q, detected_from = %q, want the pin es-MX and absent", res.From, detectedFromOf(t, res))
	}
}

// No usable chunk-1 detection on a substituted pinned request: chunks 2..N fall back to the
// ORIGINAL PIN (pinFallback), not to re-detecting as auto and not to the unrecognized code.
func TestChunkedPinnedUnusableDetectionFallsBackToPin(t *testing.T) {
	for _, first := range []model.Language{"jp", "", model.Auto} {
		t.Run("chunk 1 reports "+string(first), func(t *testing.T) {
			res, calls, err := chunkedPinned(t, 3, model.JA, model.EN, first, model.EN, nil)
			if err != nil {
				t.Fatalf("Translate: %v", err)
			}
			froms := chunkFroms(calls)
			if froms[1] != model.Auto {
				t.Errorf("chunk 1 From = %q, want auto (substituted)", froms[1])
			}
			for idx := 2; idx <= 3; idx++ {
				if froms[idx] != model.JA {
					t.Errorf("chunk %d From = %q, want ja (fall back to the pin)", idx, froms[idx])
				}
			}
			if res.From != model.JA || hasDetectedFromKey(t, res) {
				t.Errorf("From = %q, detected_from = %q, want the pin ja and absent", res.From, detectedFromOf(t, res))
			}
		})
	}
}

// A chunked pinned request whose chunk 1 detects the target language is an identity result after
// exactly one call, From qualified via resultFrom(auto, detected).
func TestChunkedPinnedDetectionCoveringTargetIsIdentity(t *testing.T) {
	res, calls, err := chunkedPinned(t, 3, model.ESMX, model.EN, model.EN, model.EN, nil)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if len(calls) != 1 {
		t.Errorf("engine called %d times, want exactly 1 (chunk 1 only)", len(calls))
	}
	if !res.Identity || res.Result != chunkDoc(3) || res.From != model.EN || hasDetectedFromKey(t, res) {
		t.Errorf("result = %+v, want identity over the whole text, From en, no detected_from", res)
	}
}

// Regression guard: a genuine auto request whose chunk 1 reports nothing usable keeps re-detecting
// (chunks 2..N stay auto), as before #161 — pinFallback is empty for it. The unrecognized-code
// half of this is TestChunkUnrecognizedDetectionIsNotPinned; this adds the no-detection half.
func TestChunkedGenuineAutoWithoutDetectionKeepsAuto(t *testing.T) {
	_, calls, err := chunkedPinned(t, 3, model.Auto, model.EN, "", "", nil)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	for idx, f := range chunkFroms(calls) {
		if f != model.Auto {
			t.Errorf("chunk %d From = %q, want auto (a genuine auto request re-detects per chunk)", idx, f)
		}
	}
}
