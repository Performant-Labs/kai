// Package translate provides the core orchestration for translation / OCR (pure business
// logic, no dependency on the wails lifecycle).
// It covers: single-engine translation, multi-engine parallel translation, image OCR,
// screenshot OCR, and writing history after a successful translation.
// Engine registration and selection go through engine.Registry; history persistence through
// historystore; user config through settings.Service.
package translate

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"cnb.cool/dtapp/kai/internal/analytics"
	"cnb.cool/dtapp/kai/internal/configstore"
	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/historystore"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/langpref"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/settings"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// Service is the translation / OCR orchestration domain service.
// All dependencies are injected explicitly via NewService — no shared container — making
// individual replacement and testing easy.
type Service struct {
	registry    *engine.Registry
	history     *historystore.Store
	configStore *configstore.Store
	settings    *settings.Service
	app         *application.App
	// langPrefs qualifies engine-detected source languages with the user's learned variant
	// preference (issue #53). Set once at wiring time via SetLangPrefs; nil means no
	// qualification (detected languages are still normalized).
	langPrefs *langpref.Store

	// screenshotCacheMu guards concurrent access to screenshotCache.
	screenshotCacheMu sync.RWMutex
	// screenshotCache caches, per session (screenshot translate window / input translate
	// page), the most recent OCR source text and screenshot, for ScreenshotRetranslate to
	// reuse after a language change (skipping screenshot/OCR and retranslating directly).
	// Per-session separation keeps OCR results from different entry points from clobbering
	// each other.
	screenshotCache map[string]ocrCache

	// requests holds the running translation requests (issue #109): what CancelTranslate reaches
	// and what a newer request of the same session supersedes.
	requests requestRegistry
	// emitter, when set through SetEmitter, receives every app event instead of the wails app.
	// The tests use it to observe what the service emits (issue #109, D8).
	emitter emitter

	// budgetOf looks up an engine's input budget for the chunked translation (issue #84):
	// engine.InputBudget, the one table of limits. It is a field so a test can give its fake
	// engines a budget; an engine without one is sent its text whole.
	budgetOf func(engineName string) (engine.Budget, bool)
	// captureRegion takes the interactive region screenshot ScreenshotTranslate starts from:
	// engine.CaptureRegion. It is a field so a test can drive that flow without screencapture.
	captureRegion func(ctx context.Context) ([]byte, error)
	// detect is the local language detector the automatic source switch (issue #200) reads:
	// engine.DetectLanguage. It is a field so a test can give it a language and a confidence.
	detect func(text string) (lang model.Language, confidence float64, ok bool)
}

// emitter is the one outlet for app events (issue #109, D8). *application.EventManager satisfies
// it as it is, so production needs no adapter; a test hands in a recorder through SetEmitter.
type emitter interface {
	Emit(name string, data ...any) bool
}

// ocrCache is one cached screenshot-OCR unit.
type ocrCache struct {
	text     string
	imageURL string
}

// NewService constructs the translation orchestration service. app may be injected after
// construction via SetApp (app only becomes ready during the startup orchestration).
func NewService(reg *engine.Registry, hist *historystore.Store, st *settings.Service, app *application.App) *Service {
	return &Service{
		registry:        reg,
		history:         hist,
		settings:        st,
		app:             app,
		screenshotCache: make(map[string]ocrCache),
		budgetOf:        engine.InputBudget,
		captureRegion:   engine.CaptureRegion,
		detect:          engine.DetectLanguage,
	}
}

// SetApp injects the app once it is ready (startup orchestration phase).
func (s *Service) SetApp(app *application.App) {
	s.app = app
}

// SetEmitter replaces the outlet for app events (a test seam, issue #109). Nil restores the wails
// app. Like the other setters it is called at wiring time, before the service is used.
func (s *Service) SetEmitter(e emitter) {
	s.emitter = e
}

// emit sends one app event: to the emitter when one was set, else to the wails app, else nowhere
// (the app is injected late, and the tests build the service without one). It is the only place
// the service reaches the event system, so every payload passes the same seam. Payloads are
// values of the type main.go registered for the event.
func (s *Service) emit(name string, data any) {
	switch {
	case s.emitter != nil:
		s.emitter.Emit(name, data)
	case s.app != nil:
		s.app.Event.Emit(name, data)
	}
}

// screenshotWindow fetches the screenshot translate window handle by name (a single choke
// point over GetByName, avoiding business code scattering raw lookups).
// Same style as translateWindow()/settingsWindow() in internal/service/window_wrapper.go.
func (s *Service) screenshotWindow() application.Window {
	if s.app == nil {
		return nil
	}
	win, ok := s.app.Window.GetByName(model.WindowScreenshot)
	if !ok {
		slog.Error(i18n.T("log.window_handle_failed"), slog.String("window", model.WindowScreenshot))
		return nil
	}
	return win
}

// showScreenshotWindow summons the screenshot window (same paradigm as
// window_wrapper.showAndFocus / TriggerInput).
// Implemented independently here because the translate package cannot import the service
// package back (circular dependency).
// Why two consecutive Show() calls: Wails v3 (beta.9), on the first Show() of a Hidden
// window, only synchronously creates the webview impl without actually showing
// (webview_window.go:Show invokes InvokeSync(Run) then returns when impl==nil); only on the
// second Show(), with the impl ready, does it actually show; Focus() then brings it to the
// front.
// By contrast, App.Show()/Hide() are synchronous direct cgo calls (see application.go:994);
// calling them off the main thread trips an AppKit thread assertion → SIGTRAP crash, so
// never call s.app.Show() from a background goroutine.
// The whole sequence is wrapped in an InvokeAsync main-thread closure, avoiding the impl
// failing to build across goroutines.
func showScreenshotWindow(win application.Window) {
	if win == nil {
		slog.Error(i18n.T("log.screenshot_window_nil"))
		return
	}
	// The whole "build impl + show + focus" sequence must run on the main thread:
	// if Show() were called synchronously from a hotkey callback (background goroutine), the
	// Run() that builds the impl on first show nests a dispatch_async + semaphore wait on the
	// main thread, which easily fails to build the impl when the main thread is busy
	// (IsVisible stays false → the window never shows).
	// InvokeAsync dispatches the sequence onto the main thread's event loop; inside the
	// closure, the first Show's InvokeSync(w.Run) builds the impl synchronously right on the
	// main thread, with no cross-goroutine waiting. Safe (no crash) from any caller
	// (hotkey/event).
	application.InvokeAsync(func() {
		slog.Debug(i18n.T("log.screenshot_window_show_enter",
			"Visible", win.IsVisible(), "Focused", win.IsFocused()))

		win.Show()
		slog.Debug(i18n.T("log.screenshot_window_after_show",
			"Visible", win.IsVisible(), "Focused", win.IsFocused()))

		win.Show()
		slog.Debug(i18n.T("log.screenshot_window_after_show",
			"Visible", win.IsVisible(), "Focused", win.IsFocused()))

		win.Focus()
		slog.Debug(i18n.T("log.screenshot_window_after_focus",
			"Visible", win.IsVisible(), "Focused", win.IsFocused()))
	})
}

// SetConfigStore injects the engine config store, used to resolve engine IDs by name when
// writing history.
func (s *Service) SetConfigStore(cs *configstore.Store) {
	s.configStore = cs
}

// SetLangPrefs injects the language-variant preference store (issue #53): from then on every
// result whose source was auto-detected reports the detected language qualified through it (see
// resultFrom). Like SetConfigStore it is setter injection at wiring time; without it detected
// languages are carried but never qualified.
func (s *Service) SetLangPrefs(p *langpref.Store) {
	s.langPrefs = p
}

// Translate performs a single-engine translation: looks up the registered translator by
// engine name, falling back to the default engine on failure;
// on success the result is written to history (failures are only logged, not surfaced).
// It has no request id and no cancel handle (issue #109): the engine runs until it answers, and a
// chunked translation (#84) reports no progress, having no request to report it for. Text over the
// input cap is rejected with an *InputTooLongError before any engine call.
func (s *Service) Translate(req model.TranslateRequest) (*model.TranslateResult, error) {
	if err := checkInputLength(req.Text); err != nil {
		return nil, err
	}
	engineName := req.EngineName
	reg, ok := s.registry.GetTranslator(engineName)
	if !ok {
		return nil, fmt.Errorf("%s: %s", i18n.T("err.translate_engine_not_registered"), engineName)
	}
	res, err := s.translateWithEngine(context.Background(), reg, engineName, req, nil)
	if err != nil {
		return nil, err
	}
	s.saveHistory(res)
	return res, nil
}

// translateWithEngine runs a translation-engine call and assembles the result. It is the single
// per-engine seam shared by the input window (Translate, TranslateMulti) and the screenshot
// flows (translateAllStream), and the owner of the same-language rule (issue #80).
//
// ctx is the engine's cancel-only context (issue #109). It has no deadline: a slow engine runs
// until it answers, and the context ends only when the request registry cancels it, for the
// user's Cancel or because a newer request replaced this one. A cancel is never turned into an
// identity result: callEngine answers a done ctx with context.Cause(ctx) and no detection.
//
// Rule: when the source and the target are the same language, the result is the source text
// itself, flagged Identity, and never a failure. The decision has two branches and a
// fall-through, the same for every engine:
//
//  1. Pinned source, SameAs(From, To): the engine is not called at all. Whitespace-only text is
//     left to the engine, which answers it with its own empty-text error.
//  2. Auto source, or a checked pin the text overrode (see below), and the language the engine
//     reports for the text, D, is a recognized language that is the target's language
//     (D.Covers(To)): identity, whether the engine translated the text anyway, echoed it, or
//     failed because it cannot translate a language into itself. D is what the engine reported,
//     bare and before resultFrom qualifies it through the variant preference: a detection cannot
//     name a dialect, so it must not miss the target for lack of one. On a failure D is the
//     detection the engine attached with engine.WithDetectedSource.
//  3. Anything else: the engine's answer or error, unchanged.
//
// There is no re-run and no second engine call, and the target is never changed: To is always the
// requested one. The identity result carries none of the engine's text (a paraphrase, phonetic or
// dictionary belongs to a translation, and this is not one), and callers do not save it to history
// (saveHistory).
//
// Text over the engine's input budget is translated in chunks (callEngineChunked, issue #84), which
// comes back as one assembled result, so every decision here is still made once per request. On an
// auto request the detection is chunk 1's, the only chunk sent when it already covers the target.
// progress receives each finished part of a chunked translation (nil: nobody listens).
//
// A pinned source is checked against the text (issue #161) when the trimmed text is at least
// minPinCheckRunes code points long: the engine is sent auto instead of the pin, in the same one
// call, so it detects the language as it does for an auto request. req keeps the pin; sent is what
// the engine is given. The detection is then compared with the pin. A different language (not
// SameAs) overrides the pin, and the request is decided like an auto one: identity by branch 2,
// else a correction, where the translation the engine made stands and From is qualified like an
// auto detection. A match (es for an es-MX pin), or no detection the app recognizes, lets the pin
// stand: it is reported as requested, and branch 2 does not apply even when the target is a
// dialect of the pin's language, since a dialect pair (a pt-BR pin into pt-PT) is the engine's to
// translate, as for any pin. A chunked request decides this once, from chunk 1 (pinFallback).
// Shorter text is sent with the pin as given.
//
// A detection counts only when model.ParseLanguage recognizes it: detectedSource also returns an
// engine's own native code (baidu's jp), which names no language to compare, correct to or show.
// DetectedFrom carries the detected language a translation was made from, the same value as From:
// on an auto request with a recognized detection (whatever the text's length) and on a corrected
// pin. It is never set on an identity result, whose own note already explains the text.
func (s *Service) translateWithEngine(ctx context.Context, reg engine.Translator, engineName string, req model.TranslateRequest, progress func(done, total int)) (*model.TranslateResult, error) {
	if strings.TrimSpace(req.Text) != "" && req.From.SameAs(req.To) {
		return s.identityResult(engineName, req, ""), nil
	}

	sent := req
	substitute := !isAutoSource(req.From) && utf8.RuneCountInString(strings.TrimSpace(req.Text)) >= minPinCheckRunes
	// pinFallback is the language chunks 2..N of a checked pin are sent with when chunk 1's
	// detection is no use: the pin itself, never auto. Empty for every other request.
	var pinFallback model.Language
	if substitute {
		sent.From = model.Auto
		pinFallback = req.From
	}

	res, err := s.callEngineChunked(ctx, reg, engineName, sent, progress, pinFallback)
	detected, recognized := detectedSource(res, err)
	if recognized {
		_, recognized = model.ParseLanguage(string(detected))
	}
	// A ctx that ended is a cancel, decided by outcomeOf from context.Cause before anything else: a
	// detection an engine attached to a failure it raced the cancel with is not an identity result.
	// A checked pin the detection confirms stood, and keeps the pinned rule decided at the top.
	// sent, not req: its From is auto on a checked pin too, so From is the qualified detection
	// (resultFrom), never the pin that turned out wrong.
	if isAutoSource(sent.From) && ctx.Err() == nil && recognized && detected.Covers(req.To) &&
		(!substitute || !detected.SameAs(req.From)) {
		return s.identityResult(engineName, sent, detected), nil
	}
	if err != nil {
		return nil, err
	}
	if !substitute {
		// callEngine leaves From as the engine reported it (the decision above needs the bare
		// detection); it is qualified here, once, for the results that are shown as translations.
		res.From = s.resultFrom(req.From, res.From)
		if isAutoSource(req.From) && recognized {
			res.DetectedFrom = res.From
		}
		return res, nil
	}
	if recognized && !detected.SameAs(req.From) {
		// The text is not in the pinned language: it was translated from the detected one, which is
		// reported the way an auto request reports its detection.
		res.From = s.resultFrom(model.Auto, detected)
		res.DetectedFrom = res.From
	} else {
		// The pin was right, or nothing usable was detected: the pin is reported as requested.
		res.From = req.From
	}
	return res, nil
}

// minPinCheckRunes is the shortest text, in code points after trimming, whose pinned source
// language is checked against the language the engine detects (issue #161). Detection is
// unreliable below about 20 characters (Apple's guidance, epic #151), and a wrong correction would
// be worse than the pin, so shorter text is sent with the pin as given. It is the whole threshold:
// no engine reports a detection confidence.
const minPinCheckRunes = 20

// detectedSource is the source language an engine reported for an auto request: the detected
// language a successful result carries (res.From as the engine returned it, still auto when the
// engine detected nothing, which is how the LLM engines answer), or the one a failing engine
// attached to its error (engine.WithDetectedSource). ok is false when there is none. Engines that
// report their own native codes (baidu's jp / kor, youdao's direction pair) yield a detection that
// does not match a recognized target, so the rule cannot fire for them.
func detectedSource(res *model.TranslateResult, err error) (model.Language, bool) {
	if err != nil {
		return engine.DetectedSourceOf(err)
	}
	if isAutoSource(res.From) {
		return "", false
	}
	return res.From, true
}

// identityResult is the result of a request whose source and target are the same language (issue
// #80): the source text as it stands, flagged so the windows can say so. detected is the language
// the engine reported for an auto request, empty for a pinned source; From goes through resultFrom
// like every reported source language (a detection is qualified through the variant preference, a
// pinned source is reported as requested). It is the only place Identity is set: an engine cannot
// declare it, because callEngine builds the engine's result field by field and never copies the
// engine's struct.
func (s *Service) identityResult(engineName string, req model.TranslateRequest, detected model.Language) *model.TranslateResult {
	return &model.TranslateResult{
		Engine:   engineName,
		From:     s.resultFrom(req.From, detected),
		To:       req.To,
		Text:     req.Text,
		Result:   req.Text,
		Identity: true,
	}
}

// callEngine runs one translation-engine call under ctx and assembles the result. It is the only
// place an engine is invoked, so the goroutine, timing log and error wrapping exist once.
//
// It returns as soon as ctx ends, whether or not the engine looks at its ctx: an engine that never
// does is abandoned, keeps running on its own goroutine, and its late result lands in a buffered
// channel nobody reads any more. (The Apple engine does look: it cancels its Swift call when ctx
// ends, so its goroutine ends too, issue #111.) What comes back is then context.Cause(ctx), which
// is how the caller tells a cancel from a failure. A ctx that is done before the engine was
// reached starts nothing.
//
// The returned result's From is what the engine reported, unqualified; translateWithEngine decides
// the same-language rule on it and only then applies resultFrom. A failure keeps the engine's error
// in its chain (%w), so a detection attached with engine.WithDetectedSource is still found.
//
// An engine that panics ends as an ordinary failure, so the request never loses its terminal
// event.
func (s *Service) callEngine(ctx context.Context, reg engine.Translator, engineName string, req model.TranslateRequest) (*model.TranslateResult, error) {
	if ctx.Err() != nil {
		return nil, context.Cause(ctx)
	}
	start := time.Now()
	resCh := make(chan *model.TranslateResult, 1)
	errCh := make(chan error, 1)
	// The engine call runs in a goroutine; results are collected over buffered channels for
	// easy cancellation and concurrency orchestration.
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error(i18n.T("log.translate_engine_panic"),
					slog.String("engine", engineName), slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
				errCh <- fmt.Errorf("%s: %v", i18n.T("err.translate_engine_panic"), r)
			}
		}()
		res, err := reg.Translate(ctx, req)
		if err != nil {
			errCh <- err
			return
		}
		resCh <- &model.TranslateResult{
			Engine:   engineName,
			From:     res.From,
			To:       req.To,
			Text:     req.Text,
			Result:   res.Result,
			Phonetic: res.Phonetic,
			Dict:     res.Dict,
		}
	}()

	select {
	case res := <-resCh:
		slog.Debug(i18n.T("log.translate_engine_cost",
			"Engine", engineName, "Ms", time.Since(start).Milliseconds(), "Ok", true, "Err", ""))
		return res, nil
	case err := <-errCh:
		slog.Debug(i18n.T("log.translate_engine_cost",
			"Engine", engineName, "Ms", time.Since(start).Milliseconds(), "Ok", false,
			"Err", err.Error()))
		return nil, fmt.Errorf("%s(%s): %w", i18n.T("err.translate_failed"), engineName, err)
	case <-ctx.Done():
		cause := context.Cause(ctx)
		slog.Debug(i18n.T("log.translate_engine_cost",
			"Engine", engineName, "Ms", time.Since(start).Milliseconds(), "Ok", false,
			"Err", cause.Error()))
		return nil, cause
	}
}

// resultFrom returns the source language a translation result reports (issue #53).
//
// An explicit request source is reported as requested: it is never replaced by what the engine
// detected and never qualified, so an explicit choice overrides any learned preference. With the
// source on auto, the engine's detected language is carried into the result (the service used to
// drop it and report the request value, "auto") and qualified through the variant preference
// store: es → es-MX once the user has picked es-MX, es-419 folding to es first. An engine that
// reports no detection — empty, or auto echoed back as the LLM engines do — leaves the request
// value in place.
//
// A code the app does not recognize passes through as the engine reported it, so an unknown
// detection (say Italian) is shown rather than hidden behind "auto". Engines that report their
// own native codes instead of a language (baidu's spa / jp / kor / fra, youdao's direction pair)
// are not mapped back here: that reverse mapping belongs with the engine capability registry.
func (s *Service) resultFrom(requested, reported model.Language) model.Language {
	if !isAutoSource(requested) || isAutoSource(reported) {
		return requested
	}
	return s.langPrefs.Qualify(reported)
}

// isAutoSource reports whether a source language asks for auto-detection ("" or auto, any
// case), reading it the way the engines read the request.
func isAutoSource(l model.Language) bool {
	return l == "" || strings.EqualFold(string(l), string(model.Auto))
}

// failurePayload builds the failure entry both fan-outs report for an engine that did not produce
// a translation (issue #96): the sanitized error text and its category (ClassifyEngineError).
// Sanitizing happens here, once, so no caller can forget it; the logs keep the full error.
// From is left empty: the payload doesn't claim a "detected source language", avoiding misuse of
// the auto-detect label. A caller that wants a source on the entry sets it afterwards.
func failurePayload(name string, req model.TranslateRequest, err error) model.TranslateResult {
	return model.TranslateResult{
		Engine:    name,
		To:        req.To,
		Text:      req.Text,
		Error:     SanitizeDetail(err.Error()),
		ErrorKind: ClassifyEngineError(err),
	}
}

// TranslateMulti starts translations on all "enabled translation engines" in parallel,
// streaming each result to the frontend as it lands (EventTranslateResult).
// The registry only contains engines the user enabled in the settings page and that
// registered successfully (OCR engines are excluded from the parallel translation).
// Returns the request's id and the engines started; actual results arrive asynchronously via
// events, tagged with that id, with the frontend aggregating by the engine field.
//
// The request (issue #109) has no time limit. It is named by req.RequestID (the frontend names it
// itself, because a fast engine can emit before this call returns; a backend id is generated only
// when none is sent), and it ends when every engine started has reported once, whether with a
// translation, a failure or a cancel, or when the user cancels it (CancelTranslate). Starting one
// supersedes the translate window's running request: that one emits nothing more.
//
// Text over the input cap is rejected with an *InputTooLongError (issue #84) before the request
// opens: nothing starts, nothing is emitted, and the running request is left as it was.
func (s *Service) TranslateMulti(req model.TranslateRequest) (*model.TranslateMultiResult, error) {
	if err := checkInputLength(req.Text); err != nil {
		return nil, err
	}
	all := s.registry.AllEngines()
	type task struct {
		name string
		reg  engine.Translator
	}
	tasks := make([]task, 0, len(all))
	engines := make([]string, 0, len(all))
	for _, meta := range all {
		// Only parallelize enabled "translation" engines; skip OCR engines.
		if meta.Kind != engine.KindTranslator {
			continue
		}
		reg, ok := s.registry.GetTranslator(meta.Name)
		if !ok {
			continue
		}
		tasks = append(tasks, task{name: meta.Name, reg: reg})
		engines = append(engines, meta.Name)
	}
	if req.RequestID == "" {
		req.RequestID = newRequestID()
	}
	ar := s.requests.open(sessionTranslate, req.RequestID)
	runs := s.requests.start(ar, engines)
	for i, t := range tasks {
		// Each engine gets its own goroutine, never blocking the others; completion is pushed
		// to the frontend via an app-level event.
		go s.translateMultiEngine(ar, runs[i], t.reg, req)
	}
	if len(engines) > 0 {
		analytics.Track(analytics.EventTranslateInput, map[string]any{
			"engine":     strings.Join(engines, ","),
			"src_lang":   string(req.From),
			"dst_lang":   string(req.To),
			"len_bucket": analytics.LenBucket(len(req.Text)),
			"trigger":    "manual",
		})
	}
	return &model.TranslateMultiResult{Count: len(engines), RequestID: req.RequestID, Engines: engines}, nil
}

// translateMultiEngine runs one engine of a TranslateMulti request and reports how it ended: one
// EventTranslateResult, or nothing when the request was superseded.
func (s *Service) translateMultiEngine(ar *activeRequest, run *engineRun, reg engine.Translator, req model.TranslateRequest) {
	defer s.requests.finish(ar, run)
	out := s.runEngine(ar, run, reg, req)
	var payload model.TranslateResult
	switch out.kind {
	case outcomeSuperseded:
		logEngineEnded(run.name, ar, errSuperseded)
		return
	case outcomeCancelled:
		// A cancel is decided before anything failure-shaped runs: no ClassifyEngineError, no
		// analytics.Error, no error log, and no history row.
		logEngineEnded(run.name, ar, errUserCancelled)
		// From is left empty, as for a failure: the payload does not claim a detected source
		// language. Result is what a chunked translation (#84) had finished, empty otherwise.
		payload = model.TranslateResult{Engine: run.name, To: req.To, Text: req.Text, Result: out.prefix, Cancelled: true}
	case outcomeFailed:
		slog.Error(i18n.T("log.translate_multi_engine_failed"), slog.String("engine", run.name), slog.Any("error", out.err))
		analytics.Error("translate_failed", map[string]any{"engine": run.name})
		// issue #42: failures are no longer silently dropped — the same event is pushed
		// with Error/ErrorKind payload, and the frontend shows the per-engine failure
		// reason (categories per ClassifyEngineError).
		// From is left empty: the failure payload doesn't claim a "detected source
		// language", avoiding misuse of the auto-detect label.
		payload = failurePayload(run.name, req, out.err)
		payload.Error = partReached(payload.Error, out)
	default:
		s.saveHistory(out.res)
		payload = *out.res
	}
	payload.RequestID = ar.id
	s.reportOnce(ar, run, func() { s.emit(events.EventTranslateResult, payload) })
}

// outcomeKind says how one engine of a request ended.
type outcomeKind int

const (
	outcomeSuccess    outcomeKind = iota // the engine returned a translation
	outcomeFailed                        // the engine failed on its own
	outcomeCancelled                     // the user cancelled it (errUserCancelled)
	outcomeSuperseded                    // a newer request replaced its request (errSuperseded)
)

// engineOutcome is how one engine of a request ended, decided by outcomeOf.
type engineOutcome struct {
	kind outcomeKind
	res  *model.TranslateResult // outcomeSuccess
	err  error                  // outcomeFailed

	// How far a chunked translation (issue #84) got before it was cancelled or failed, from its
	// *chunkedError: the translated parts from part 1 on, joined (prefix, what a cancelled
	// payload shows), how many parts that is (done), and the parts in all (total). All zero when
	// the engine was called once for the whole text.
	prefix      string
	done, total int
}

// outcomeOf decides how an engine ended from the engine's ctx and what it returned. The ctx's
// cause decides, never the error text: engines lose the error chain on the way out, so a
// cancelled LLM call comes back as plain text. A translation the engine did return stands even if
// a cancel arrived a moment later, but a superseded request drops everything. A chunked
// translation that stopped early says how far it got through its *chunkedError; that never
// changes which outcome it is.
func outcomeOf(ctx context.Context, res *model.TranslateResult, err error) engineOutcome {
	cause := context.Cause(ctx)
	var chunked chunkedError
	if ce, ok := errors.AsType[*chunkedError](err); ok {
		chunked = *ce
	}
	switch {
	case errors.Is(cause, errSuperseded):
		return engineOutcome{kind: outcomeSuperseded}
	case err == nil:
		return engineOutcome{kind: outcomeSuccess, res: res}
	case errors.Is(cause, errUserCancelled):
		return engineOutcome{kind: outcomeCancelled, prefix: chunked.Prefix, done: chunked.Done, total: chunked.Total}
	default:
		return engineOutcome{kind: outcomeFailed, err: err, prefix: chunked.Prefix, done: chunked.Done, total: chunked.Total}
	}
}

// partReached puts how far a failed chunked translation got (issue #84) in front of its failure
// detail: "Stopped after part N of M: <detail>", N the parts translated from part 1 on. The detail
// is already sanitized and cut to length (failurePayload), so a long engine reason cannot push the
// part out of the text. A failure of an engine called once for the whole text is left as it is.
func partReached(detail string, out engineOutcome) string {
	if out.total == 0 {
		return detail
	}
	return i18n.T("err.translate_stopped_after_part", "Done", out.done, "Total", out.total) + ": " + detail
}

// runEngine is the per-engine step both fan-outs share: it announces the engine (the started
// progress event, before any result), runs it under its own cancel-only ctx through the one
// per-engine seam, and says how it ended. The fan-outs build their own payloads from the outcome.
// A chunked translation (#84) reports each finished part as a chunk progress event of the request.
func (s *Service) runEngine(ar *activeRequest, run *engineRun, reg engine.Translator, req model.TranslateRequest) engineOutcome {
	// A request replaced before this engine began has nothing to announce or report.
	if errors.Is(context.Cause(run.ctx), errSuperseded) {
		return engineOutcome{kind: outcomeSuperseded}
	}
	now := time.Now().UnixMilli()
	run.startedAtMs.Store(now)
	ar.emitIfLive(func() {
		s.emit(events.EventTranslateProgress, events.TranslateProgressPayload{
			RequestID:   ar.id,
			Engine:      run.name,
			Phase:       events.ProgressPhaseStarted,
			StartedAtMs: now,
		})
	})
	progress := func(done, total int) { s.reportProgress(ar.id, run.name, done, total) }
	res, err := s.translateWithEngine(run.ctx, reg, run.name, req, progress)
	return outcomeOf(run.ctx, res, err)
}

// reportOnce runs emit, the terminal report of one engine, unless the engine has already reported:
// exactly one terminal event per started engine (issue #109). The engine goroutines are built so
// that this never happens twice; the guard keeps it true if a later change makes it possible. A
// superseded request emits nothing (emitIfLive).
func (s *Service) reportOnce(ar *activeRequest, run *engineRun, emit func()) {
	if !s.requests.claim(run) {
		slog.Warn(i18n.T("log.translate_duplicate_report"), slog.String("engine", run.name), slog.String("request", ar.id))
		return
	}
	ar.emitIfLive(emit)
}

// logEngineEnded records an engine that ended without a result because of its cause: the user's
// Cancel at Info, a newer request replacing it at Debug (routine). Neither is a failure.
func logEngineEnded(engineName string, ar *activeRequest, cause error) {
	level := slog.LevelInfo
	if errors.Is(cause, errSuperseded) {
		level = slog.LevelDebug
	}
	slog.Log(context.Background(), level, i18n.T("log.translate_engine_cancelled"),
		slog.String("engine", engineName), slog.String("request", ar.id), slog.Any("cause", cause))
}

// CancelTranslate cancels the running request requestID, or only its engine engineName when that
// is not empty (issue #109). The others keep running; a cancelled engine reports one terminal
// event with Cancelled set. It reports whether it found something still running, and returns
// false, without any error, for an unknown or finished request or engine. Calling it again is
// harmless.
func (s *Service) CancelTranslate(requestID, engineName string) bool {
	return s.requests.cancel(requestID, engineName)
}

// reportProgress announces that done of total parts of an engine's work are finished (issue
// #109). The chunked translation (#84) calls it as each part finishes, through the callback
// runEngine builds. It emits while the engine is still working and says nothing for an unknown
// request, a finished engine or a cancelled one.
func (s *Service) reportProgress(requestID, engineName string, done, total int) {
	ar, run := s.requests.running(requestID, engineName)
	if ar == nil {
		return
	}
	ar.emitIfLive(func() {
		s.emit(events.EventTranslateProgress, events.TranslateProgressPayload{
			RequestID:   requestID,
			Engine:      engineName,
			Phase:       events.ProgressPhaseChunk,
			StartedAtMs: run.startedAtMs.Load(),
			Done:        done,
			Total:       total,
		})
	})
}

// activeRequestCount is the number of requests still held: 0 once every engine of every request
// has reported. A test hook.
func (s *Service) activeRequestCount() int {
	return s.requests.count()
}

// Ocr runs image OCR: performs OCR directly on the provided image data (the caller has
// already captured/selected the image).
func (s *Service) Ocr(req model.OcrRequest) (*model.OcrResult, error) {
	if len(req.ImageData) == 0 {
		return nil, fmt.Errorf(i18n.T("err.ocr_empty_image"))
	}
	ocr, ok := s.registry.GetOcr(req.Engine)
	if !ok {
		return nil, fmt.Errorf("%s: %s", i18n.T("err.ocr_engine_not_registered"), req.Engine)
	}
	return s.ocrWithEngine(ocr, req)
}

// ScreenshotOCR is screenshot OCR: captures the screen first, then OCRs the captured image,
// pushing the result to the frontend (EventTranslateResult).
func (s *Service) ScreenshotOCR(engineName string) (*model.OcrResult, error) {
	img, err := engine.CaptureScreenshot()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", i18n.T("err.screenshot_failed"), err)
	}
	return s.TriggerOcr(engineName, img)
}

// ScreenshotTranslate is the main screenshot-translate flow (staged streaming):
//  1. capture a region screenshot → system OCR extracts the text
//  2. immediately Emit EventScreenshotOCR (image+text, translations empty); the frontend
//     shows the screenshot and source text right away
//  3. translate per engine, Emitting once more after each finishes (accumulating
//     translations); the frontend appends translation cards incrementally.
//     Engines that fail to translate are appended as a "failure placeholder" too, so nothing
//     is silently lost; a cancelled engine is appended as a cancelled placeholder (issue #109).
//
// session identifies the cache origin (events.ScreenshotSessionScreenshot /
// ScreenshotSessionInput), keeping this round's OCR text and screenshot isolated per entry
// point so different entries don't clobber each other's retranslate cache. It is also the
// request session (issue #109): a run replaces the session's running one, and every push of the
// run carries its request id.
// Returns the full result for callers that need a synchronous reply. A run that a newer capture
// replaced while OCR was running ends quietly with neither a result nor an error.
func (s *Service) ScreenshotTranslate(session string) (*model.ScreenshotResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	slog.Debug(i18n.T("log.screenshot_start"), slog.String("step", "capture_region"))
	img, err := s.captureRegion(ctx)
	if err != nil {
		slog.Error(i18n.T("log.screenshot_capture_region_failed"), slog.Any("error", err))
		return nil, fmt.Errorf("%s: %w", i18n.T("err.ocr_region_capture_failed"), err)
	}
	slog.Debug(i18n.T("log.screenshot_capture_region_done"), slog.Int("image_bytes", len(img)))

	// The requested target of this flow (the screenshot flow has no language bar to read), resolved
	// once so every push below reports the same one.
	to := s.defaultTarget()

	// The region is chosen: this is a new run (issue #109). Opening its request before the first
	// push replaces the session's running one, so nothing of that run can reach the window after
	// this run's first push, and every push below carries this run's id (the progress event is a
	// broadcast without an origin, so the window learns its id from these pushes).
	ar := s.requests.open(session, newRequestID())
	defer s.requests.release(ar)

	// As soon as the screenshot completes (user released the drag, img is in hand), summon
	// the window immediately — no need to wait for OCR and translation (recognition is
	// handled within the page). This path follows the **exact same safe paradigm** as the
	// input translate window's TriggerInput (w.Show();w.Focus() in manager.go:101-102): on
	// the first Show() of a Hidden window, Wails only synchronously creates the webview impl
	// without actually showing (see beta.9 webview_window.go:Show); a second Show() actually
	// displays it once the impl is ready; Focus() then activates the foreground.
	// Note: Focus() internally uses InvokeSync, which dispatches back to the main thread and
	// is safe from non-main-thread callers like hotkey callbacks; whereas App.Show()/Hide()
	// are synchronous direct cgo calls (impl.show() at application.go:994) and calling them
	// off the main thread trips an AppKit thread assertion → SIGTRAP crash (verified with a
	// real stack). So this path must never use s.app.Show() — only window-level
	// Show()/Focus(), fully aligned with TriggerInput.
	showScreenshotWindow(s.screenshotWindow())
	imageURL := "data:image/png;base64," + encodeImage(img)
	s.pushScreenshot(ar, model.ScreenshotResult{
		Image:        imageURL,
		Text:         "",
		Translations: nil,
		To:           to,
	})
	slog.Debug(i18n.T("log.screenshot_pushed_image"), slog.String("step", "image_pushed"), slog.Int("image_len", len(imageURL)))

	ocrName := s.registry.DefaultOCREngineName()
	if ocrName == "" {
		slog.Error(i18n.T("log.screenshot_no_ocr_engine"))
		return nil, fmt.Errorf(i18n.T("err.no_ocr"))
	}
	slog.Debug(i18n.T("log.screenshot_ocr_start"), slog.String("ocr_engine", ocrName))
	ocrRes, err := s.TriggerOcr(ocrName, img)
	if err != nil {
		slog.Error(i18n.T("log.screenshot_ocr_failed"), slog.String("ocr_engine", ocrName), slog.Any("error", err))
		analytics.Error("ocr_failed", map[string]any{"ocr_provider": ocrName})
		// OCR failure (including timeout) must be delivered to the frontend, otherwise the
		// page keeps spinning on "recognizing text…".
		s.pushScreenshot(ar, model.ScreenshotResult{
			Image:        imageURL,
			Text:         "",
			Translations: nil,
			To:           to,
			Error:        err.Error(),
		})
		return nil, err
	}
	text := ocrRes.Text
	slog.Debug(i18n.T("log.screenshot_ocr_done"), slog.Int("text_len", len(text)))
	if text == "" {
		slog.Warn(i18n.T("log.screenshot_ocr_empty"))
		return nil, fmt.Errorf(i18n.T("err.ocr_no_text"))
	}
	// Over the input cap (issue #84). The request was opened before OCR, when the length of the
	// text was not known yet, so the rejection reaches the window the way an OCR failure does. The
	// text is not cached: a retranslate could only be rejected again.
	if err := checkInputLength(text); err != nil {
		s.pushScreenshot(ar, model.ScreenshotResult{
			Image:        imageURL,
			Text:         "",
			Translations: nil,
			To:           to,
			Error:        err.Error(),
		})
		return nil, err
	}
	// A newer capture replaced this run while OCR was running: it neither caches its text (that
	// would clobber the newer run's retranslate cache) nor translates.
	if ar.isSuperseded() {
		return nil, nil
	}
	// Cache this round's OCR text and screenshot per session for ScreenshotRetranslate to
	// reuse after a language change (isolating different entry points).
	s.screenshotCacheMu.Lock()
	s.screenshotCache[session] = ocrCache{text: text, imageURL: imageURL}
	s.screenshotCacheMu.Unlock()

	from := model.Auto

	imageURL = "data:image/png;base64," + encodeImage(img)

	// Stage one: push the screenshot + source text first so the frontend can display
	// immediately (shown as soon as content is recognized — no waiting for translation).
	s.pushScreenshot(ar, model.ScreenshotResult{Image: imageURL, Text: text, Translations: nil, To: to})

	req := model.TranslateRequest{Text: text, From: from, To: to}
	slog.Debug(i18n.T("log.screenshot_translate_start"), slog.String("target", string(to)), slog.Int("text_len", len(text)))
	translations := s.translateAllStream(ar, req, imageURL, to)

	result := model.ScreenshotResult{
		Image:        imageURL,
		Text:         text,
		Translations: translations,
		To:           to,
		RequestID:    ar.id,
	}
	slog.Debug(i18n.T("log.screenshot_assemble_done"), slog.Int("image_len", len(result.Image)), slog.Int("text_len", len(result.Text)), slog.Int("translations", len(result.Translations)))
	analytics.Track(analytics.EventTranslateScreenshot, map[string]any{
		"engine":       strings.Join(s.enabledTranslatorNames(), ","),
		"ocr_provider": ocrName,
		"ocr_retried":  false,
	})
	return &result, nil
}

// pushScreenshot delivers one push of a screenshot run to the screenshot window, tagged with the
// run's request id, so no push can forget it. A run that a newer one replaced pushes nothing.
func (s *Service) pushScreenshot(ar *activeRequest, res model.ScreenshotResult) {
	res.RequestID = ar.id
	ar.emitIfLive(func() { s.emit(events.EventScreenshotOCR, res) })
}

// defaultTarget resolves the requested target of a flow that has no language bar to read (the
// screenshot flow): the user's saved default_to, else the policy default (settings.DefaultTarget).
// It is the one place that decision lives (issue #44), so no flow carries a hardcoded target of
// its own. The screenshot pushes report its value as the requested target, and every result
// carries that same target (see translateWithEngine).
func (s *Service) defaultTarget() model.Language {
	if s.settings != nil {
		if cfg := s.settings.Get(); cfg != nil && cfg.DefaultTo != "" {
			return model.Language(cfg.DefaultTo)
		}
	}
	return settings.DefaultTarget
}

// enabledTranslatorNames returns the currently enabled, retrievable translation engine names
// (for anonymous analytics reporting). An engine registered as a not-configured stand-in
// (engine.IsNotConfigured, issue #96) is enabled but cannot work, so it is not reported as one.
func (s *Service) enabledTranslatorNames() []string {
	names := make([]string, 0)
	for _, meta := range s.registry.AllEngines() {
		if meta.Kind != engine.KindTranslator {
			continue
		}
		if tr, ok := s.registry.GetTranslator(meta.Name); ok && !engine.IsNotConfigured(tr) {
			names = append(names, meta.Name)
		}
	}
	return names
}

// ScreenshotRetranslate retranslates after a language change: reuses the most recent
// screenshot-OCR text and screenshot of the given session, skipping the screenshot/OCR
// stages, and directly re-invokes each engine with the passed from/to, pushing
// EventScreenshotOCR incrementally.
// Returns the accumulated translation results. Errors when the session has no OCR cache yet
// (no screenshot taken), or with an *InputTooLongError when the cached text is over the input cap
// (issue #84); either way no request is opened.
// Like ScreenshotTranslate it is a request of its session (issue #109): it replaces the running
// one, and its first push, which carries the request id and no translations yet, opens the run
// for the window.
func (s *Service) ScreenshotRetranslate(session string, from, to model.Language) error {
	s.screenshotCacheMu.RLock()
	cache, ok := s.screenshotCache[session]
	s.screenshotCacheMu.RUnlock()
	if !ok || cache.text == "" || cache.imageURL == "" {
		return fmt.Errorf(i18n.T("err.screenshot_no_cache"))
	}
	// Over the input cap (issue #84): rejected like a missing cache, before the request opens.
	if err := checkInputLength(cache.text); err != nil {
		return err
	}
	req := model.TranslateRequest{Text: cache.text, From: from, To: to}
	slog.Debug(i18n.T("log.screenshot_retranslate_start"),
		slog.String("session", session),
		slog.String("from", string(from)), slog.String("to", string(to)),
		slog.Int("text_len", len(req.Text)))
	ar := s.requests.open(session, newRequestID())
	defer s.requests.release(ar)
	s.pushScreenshot(ar, model.ScreenshotResult{Image: cache.imageURL, Text: cache.text, Translations: nil, To: to})
	s.translateAllStream(ar, req, cache.imageURL, to)
	return nil
}

// translateAllStream concurrently invokes all enabled translation engines (each in its own
// goroutine, never blocking the others) as the engines of the request ar (issue #109);
// each completion (success, failure placeholder or cancelled placeholder) Emits EventScreenshotOCR
// once (carrying the accumulated translations), and the frontend dedupes by engine and appends
// incrementally, so translations arrive one by one and a slow engine no longer holds up
// the other engines. An engine that was cancelled reports a placeholder with Cancelled set and no
// error; an engine of a superseded request reports nothing.
// Returns the final accumulated result list (ordered by engine registration order, not
// completion order).
func (s *Service) translateAllStream(ar *activeRequest, req model.TranslateRequest, imageURL string, to model.Language) []model.TranslateResult {
	metas := s.registry.AllEngines()
	type task struct {
		meta engine.EngineMeta
		reg  engine.Translator
	}
	tasks := make([]task, 0, len(metas))
	names := make([]string, 0, len(metas))
	for _, meta := range metas {
		if meta.Kind != engine.KindTranslator {
			continue
		}
		reg, ok := s.registry.GetTranslator(meta.Name)
		if !ok {
			continue
		}
		tasks = append(tasks, task{meta: meta, reg: reg})
		names = append(names, meta.Name)
	}
	runs := s.requests.start(ar, names)

	var mu sync.Mutex
	var wg sync.WaitGroup
	// out keeps slots in engine registration order, written concurrently at their own
	// indices, avoiding append races.
	out := make([]model.TranslateResult, len(tasks))
	for i, t := range tasks {
		wg.Add(1)
		go func(idx int, meta engine.EngineMeta, reg engine.Translator, run *engineRun) {
			defer wg.Done()
			defer s.requests.finish(ar, run)
			slog.Debug(i18n.T("log.screenshot_engine_start"), slog.String("engine", meta.Name))
			outcome := s.runEngine(ar, run, reg, req)
			var item model.TranslateResult
			switch outcome.kind {
			case outcomeSuperseded:
				logEngineEnded(meta.Name, ar, errSuperseded)
				return
			case outcomeCancelled:
				logEngineEnded(meta.Name, ar, errUserCancelled)
				// A cancelled engine is a placeholder card like a failed one, marked Cancelled and
				// carrying no error. Result is what a chunked translation (#84) had finished.
				item = model.TranslateResult{
					Engine:    meta.Name,
					From:      req.From,
					To:        req.To,
					Text:      req.Text,
					Result:    outcome.prefix,
					Cancelled: true,
				}
			case outcomeFailed:
				slog.Warn(i18n.T("log.translate_screenshot_engine_failed"), slog.String("engine", meta.Name), slog.Any("error", outcome.err))
				// Failures also append a placeholder card so the user can see which engine
				// didn't produce a translation, and why (issue #96: the same failurePayload as
				// the translate window's fan-out). The card header prints From for every card,
				// failed ones included, so the placeholder keeps the requested source.
				item = failurePayload(meta.Name, req, outcome.err)
				item.From = req.From
				item.Error = partReached(item.Error, outcome)
			default:
				s.saveHistory(outcome.res)
				item = *outcome.res
			}
			item.RequestID = ar.id
			mu.Lock()
			out[idx] = item
			// Push incrementally after each completion with all results finished so far
			// (unfinished engines' slots are empty; the frontend dedupes by engine and appends,
			// treating an empty Result as a placeholder).
			partial := make([]model.TranslateResult, 0, len(out))
			for _, o := range out {
				if o.Engine != "" {
					partial = append(partial, o)
				}
			}
			// The push is sent through reportOnce, which already holds the run's emit guard, so it
			// emits directly instead of going through pushScreenshot (a nested read lock could
			// deadlock against a superseding writer).
			push := model.ScreenshotResult{
				Image:        imageURL,
				Text:         req.Text,
				Translations: partial,
				To:           to,
				RequestID:    ar.id,
			}
			s.reportOnce(ar, run, func() { s.emit(events.EventScreenshotOCR, push) })
			mu.Unlock()
		}(i, t.meta, t.reg, runs[i])
	}
	wg.Wait()
	// Filter out unfinished empty slots (theoretically all filled after wg.Wait; belt and
	// braces). An engine of a superseded request leaves its slot empty.
	final := make([]model.TranslateResult, 0, len(out))
	for _, o := range out {
		if o.Engine != "" {
			final = append(final, o)
		}
	}
	return final
}

// TriggerOcr runs OCR on the given image, returning the recognition result.
// Note: the OCR result is returned to the caller via the return value (ScreenshotTranslate
// uniformly pushes it to the frontend via EventScreenshotOCR) — it must NOT be emitted here
// via EventTranslateResult: that event's registered type is TranslateResult, and sending an
// OcrResult triggers the ERR "data of type model.OcrResult ... does not match registered
// data type model.TranslateResult".
func (s *Service) TriggerOcr(engineName string, img []byte) (*model.OcrResult, error) {
	ocr, ok := s.registry.GetOcr(engineName)
	if !ok {
		return nil, fmt.Errorf("%s: %s", i18n.T("err.ocr_engine_not_registered"), engineName)
	}
	res, err := s.ocrWithEngine(ocr, model.OcrRequest{ImageData: img, Engine: engineName})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// ocrWithEngine runs a single OCR-engine call and assembles the result.
// req carries Engine/CorrectText/TimeoutSec; the Go-side ctx timeout is set to the Swift
// timeout + headroom, ensuring Swift's internal timeout returns "ocr timeout" first and Go is
// never falsely triggered by a premature ctx.Done().
func (s *Service) ocrWithEngine(ocr engine.OcrEngine, req model.OcrRequest) (*model.OcrResult, error) {
	swiftTimeout := req.TimeoutSec
	if swiftTimeout <= 0 {
		swiftTimeout = 60
	}
	timeout := time.Duration(swiftTimeout+10) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	resCh := make(chan *model.OcrResult, 1)
	errCh := make(chan error, 1)
	go func() {
		res, err := ocr.Recognize(ctx, req)
		if err != nil {
			errCh <- err
			return
		}
		resCh <- res
	}()

	select {
	case res := <-resCh:
		slog.Debug(i18n.T("log.ocr_cost",
			"Engine", req.Engine, "Ms", time.Since(start).Milliseconds(), "Ok", true, "Err", ""))
		return res, nil
	case err := <-errCh:
		slog.Debug(i18n.T("log.ocr_cost",
			"Engine", req.Engine, "Ms", time.Since(start).Milliseconds(), "Ok", false,
			"Err", err.Error()))
		return nil, fmt.Errorf("%s(%s): %w", i18n.T("err.ocr_failed"), req.Engine, err)
	case <-ctx.Done():
		slog.Error(i18n.T("log.screenshot_ocr_timeout"), slog.String("ocr_engine", req.Engine), slog.Any("error", ctx.Err()))
		slog.Debug(i18n.T("log.ocr_cost",
			"Engine", req.Engine, "Ms", time.Since(start).Milliseconds(), "Ok", false,
			"Err", ctx.Err().Error()))
		return nil, fmt.Errorf("%s(%s): %w", i18n.T("err.ocr_timeout"), req.Engine, ctx.Err())
	}
}

// saveHistory writes a successful translation to the history store (failures are only
// logged, not surfaced). An identity result (issue #80) is not saved: history records
// translations, and a copy of the source text is not one. It is the one choke point every caller
// of translateWithEngine goes through, so the rule is stated here and nowhere else.
func (s *Service) saveHistory(res *model.TranslateResult) {
	if s.history == nil || res == nil || res.Identity {
		return
	}
	fromOCR := int64(0)
	if res.FromOCR {
		fromOCR = 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	engineRow, err := s.configStore.GetEngineByName(ctx, res.Engine)
	if err != nil {
		slog.Warn(i18n.T("log.engine_query_id_failed"), slog.String("engine", res.Engine), slog.Any("error", err))
	}
	engineID := int64(0)
	if engineRow != nil {
		engineID = engineRow.ID
	}
	// Dedupe: translations with identical content are not stored twice.
	if dup, _ := s.history.FindByKey(ctx, res.Text, string(res.From), string(res.To), engineID, fromOCR); dup > 0 {
		return
	}
	if _, err := s.history.InsertHistory(ctx, historystore.InsertHistoryParams{
		Text:      res.Text,
		Result:    res.Result,
		FromLang:  string(res.From),
		ToLang:    string(res.To),
		EngineID:  engineID,
		FromOcr:   fromOCR,
		CreatedAt: time.Now().UnixMilli(),
	}); err != nil {
		slog.Error(i18n.T("log.translate_save_history_failed"), slog.Any("error", err))
	}
}

// encodeImage encodes image bytes into a base64 string (without the data URL prefix).
func encodeImage(b []byte) string {
	return base64.StdEncoding.EncodeToString(b)
}
