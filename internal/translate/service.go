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
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

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
	}
}

// SetApp injects the app once it is ready (startup orchestration phase).
func (s *Service) SetApp(app *application.App) {
	s.app = app
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
func (s *Service) Translate(req model.TranslateRequest) (*model.TranslateResult, error) {
	engineName := req.EngineName
	reg, ok := s.registry.GetTranslator(engineName)
	if !ok {
		return nil, fmt.Errorf("%s: %s", i18n.T("err.translate_engine_not_registered"), engineName)
	}
	res, err := s.translateWithEngine(reg, engineName, req)
	if err != nil {
		return nil, err
	}
	s.saveHistory(res)
	return res, nil
}

// translateWithEngine runs a single translation-engine call and assembles the result.
func (s *Service) translateWithEngine(reg engine.Translator, engineName string, req model.TranslateRequest) (*model.TranslateResult, error) {
	timeout := 30 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	resCh := make(chan *model.TranslateResult, 1)
	errCh := make(chan error, 1)
	// The engine call runs in a goroutine; results are collected over buffered channels for
	// easy timeout and concurrency orchestration.
	go func() {
		res, err := reg.Translate(ctx, req)
		if err != nil {
			errCh <- err
			return
		}
		resCh <- &model.TranslateResult{
			Engine:   engineName,
			From:     s.resultFrom(req.From, res.From),
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
		slog.Debug(i18n.T("log.translate_engine_cost",
			"Engine", engineName, "Ms", time.Since(start).Milliseconds(), "Ok", false,
			"Err", ctx.Err().Error()))
		return nil, fmt.Errorf("%s(%s): %w", i18n.T("err.translate_timeout"), engineName, ctx.Err())
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

// TranslateMulti starts translations on all "enabled translation engines" in parallel,
// streaming each result to the frontend as it lands (EventTranslateResult).
// The registry only contains engines the user enabled in the settings page and that
// registered successfully (OCR engines are excluded from the parallel translation).
// Returns the number of engines started; actual results arrive asynchronously via events,
// with the frontend aggregating by the engine field.
func (s *Service) TranslateMulti(req model.TranslateRequest) (*model.TranslateMultiResult, error) {
	all := s.registry.AllEngines()
	started := 0
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
		engines = append(engines, meta.Name)
		started++
		// Each engine gets its own goroutine, never blocking the others; completion is pushed
		// to the frontend via an app-level event.
		go func(reg engine.Translator, name string) {
			res, err := s.translateWithEngine(reg, name, req)
			if err != nil {
				slog.Error(i18n.T("log.translate_multi_engine_failed"), slog.String("engine", name), slog.Any("error", err))
				analytics.Error("translate_failed", map[string]any{"engine": name})
				// issue #42: failures are no longer silently dropped — the same event is pushed
				// with Error/ErrorKind payload, and the frontend shows the per-engine failure
				// reason (categories per ClassifyEngineError).
				// From is left empty: the failure payload doesn't claim a "detected source
				// language", avoiding misuse of the auto-detect label.
				if s.app != nil {
					s.app.Event.Emit(events.EventTranslateResult, model.TranslateResult{
						Engine:    name,
						To:        req.To,
						Text:      req.Text,
						Error:     err.Error(),
						ErrorKind: ClassifyEngineError(err.Error()),
					})
				}
				return
			}
			s.saveHistory(res)
			if s.app != nil {
				s.app.Event.Emit(events.EventTranslateResult, *res)
			}
		}(reg, meta.Name)
	}
	if started > 0 {
		analytics.Track(analytics.EventTranslateInput, map[string]any{
			"engine":     strings.Join(engines, ","),
			"src_lang":   string(req.From),
			"dst_lang":   string(req.To),
			"len_bucket": analytics.LenBucket(len(req.Text)),
			"trigger":    "manual",
		})
	}
	return &model.TranslateMultiResult{Count: started}, nil
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
//     is silently lost.
//
// session identifies the cache origin (events.ScreenshotSessionScreenshot /
// ScreenshotSessionInput), keeping this round's OCR text and screenshot isolated per entry
// point so different entries don't clobber each other's retranslate cache.
// Returns the full result for callers that need a synchronous reply.
func (s *Service) ScreenshotTranslate(session string) (*model.ScreenshotResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	slog.Debug(i18n.T("log.screenshot_start"), slog.String("step", "capture_region"))
	img, err := engine.CaptureRegion(ctx)
	if err != nil {
		slog.Error(i18n.T("log.screenshot_capture_region_failed"), slog.Any("error", err))
		return nil, fmt.Errorf("%s: %w", i18n.T("err.ocr_region_capture_failed"), err)
	}
	slog.Debug(i18n.T("log.screenshot_capture_region_done"), slog.Int("image_bytes", len(img)))

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
	if s.app != nil {
		s.app.Event.Emit(events.EventScreenshotOCR, model.ScreenshotResult{
			Image:        imageURL,
			Text:         "",
			Translations: nil,
			To:           model.ZH,
		})
	}
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
		if s.app != nil {
			s.app.Event.Emit(events.EventScreenshotOCR, model.ScreenshotResult{
				Image:        imageURL,
				Text:         "",
				Translations: nil,
				To:           model.ZH,
				Error:        err.Error(),
			})
		}
		return nil, err
	}
	text := ocrRes.Text
	slog.Debug(i18n.T("log.screenshot_ocr_done"), slog.Int("text_len", len(text)))
	if text == "" {
		slog.Warn(i18n.T("log.screenshot_ocr_empty"))
		return nil, fmt.Errorf(i18n.T("err.ocr_no_text"))
	}
	// Cache this round's OCR text and screenshot per session for ScreenshotRetranslate to
	// reuse after a language change (isolating different entry points).
	s.screenshotCacheMu.Lock()
	s.screenshotCache[session] = ocrCache{text: text, imageURL: imageURL}
	s.screenshotCacheMu.Unlock()

	to := model.ZH
	if s.settings != nil && s.settings.Get() != nil && s.settings.Get().DefaultTo != "" {
		to = model.Language(s.settings.Get().DefaultTo)
	}
	from := model.Auto

	imageURL = "data:image/png;base64," + encodeImage(img)

	// Stage one: push the screenshot + source text first so the frontend can display
	// immediately (shown as soon as content is recognized — no waiting for translation).
	first := model.ScreenshotResult{Image: imageURL, Text: text, Translations: nil, To: to}
	if s.app != nil {
		s.app.Event.Emit(events.EventScreenshotOCR, first)
	}

	req := model.TranslateRequest{Text: text, From: from, To: to}
	slog.Debug(i18n.T("log.screenshot_translate_start"), slog.String("target", string(to)), slog.Int("text_len", len(text)))
	translations := s.translateAllStream(req, imageURL, to)

	result := model.ScreenshotResult{
		Image:        imageURL,
		Text:         text,
		Translations: translations,
		To:           to,
	}
	slog.Debug(i18n.T("log.screenshot_assemble_done"), slog.Int("image_len", len(result.Image)), slog.Int("text_len", len(result.Text)), slog.Int("translations", len(result.Translations)))
	analytics.Track(analytics.EventTranslateScreenshot, map[string]any{
		"engine":       strings.Join(s.enabledTranslatorNames(), ","),
		"ocr_provider": ocrName,
		"ocr_retried":  false,
	})
	return &result, nil
}

// enabledTranslatorNames returns the currently enabled, retrievable translation engine names
// (for anonymous analytics reporting).
func (s *Service) enabledTranslatorNames() []string {
	names := make([]string, 0)
	for _, meta := range s.registry.AllEngines() {
		if meta.Kind != engine.KindTranslator {
			continue
		}
		if _, ok := s.registry.GetTranslator(meta.Name); ok {
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
// (no screenshot taken).
func (s *Service) ScreenshotRetranslate(session string, from, to model.Language) error {
	s.screenshotCacheMu.RLock()
	cache, ok := s.screenshotCache[session]
	s.screenshotCacheMu.RUnlock()
	if !ok || cache.text == "" || cache.imageURL == "" {
		return fmt.Errorf(i18n.T("err.screenshot_no_cache"))
	}
	req := model.TranslateRequest{Text: cache.text, From: from, To: to}
	slog.Debug(i18n.T("log.screenshot_retranslate_start"),
		slog.String("session", session),
		slog.String("from", string(from)), slog.String("to", string(to)),
		slog.Int("text_len", len(req.Text)))
	s.translateAllStream(req, cache.imageURL, to)
	return nil
}

// translateAllStream concurrently invokes all enabled translation engines (each in its own
// goroutine, never blocking the others);
// each completion (success or failure placeholder) Emits EventScreenshotOCR once (carrying
// the accumulated translations), and the frontend dedupes by engine and appends
// incrementally, so translations arrive one by one and a google timeout no longer holds up
// deepl and the other engines.
// Returns the final accumulated result list (ordered by engine registration order, not
// completion order).
func (s *Service) translateAllStream(req model.TranslateRequest, imageURL string, to model.Language) []model.TranslateResult {
	metas := s.registry.AllEngines()
	type task struct {
		meta engine.EngineMeta
		reg  engine.Translator
	}
	tasks := make([]task, 0, len(metas))
	for _, meta := range metas {
		if meta.Kind != engine.KindTranslator {
			continue
		}
		reg, ok := s.registry.GetTranslator(meta.Name)
		if !ok {
			continue
		}
		tasks = append(tasks, task{meta: meta, reg: reg})
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	// out keeps slots in engine registration order, written concurrently at their own
	// indices, avoiding append races.
	out := make([]model.TranslateResult, len(tasks))
	for i, t := range tasks {
		wg.Add(1)
		go func(idx int, meta engine.EngineMeta, reg engine.Translator) {
			defer wg.Done()
			slog.Debug(i18n.T("log.screenshot_engine_start"), slog.String("engine", meta.Name))
			res, err := s.translateWithEngine(reg, meta.Name, req)
			var item model.TranslateResult
			if err != nil {
				slog.Warn(i18n.T("log.translate_screenshot_engine_failed"), slog.String("engine", meta.Name), slog.Any("error", err))
				// Failures also append a placeholder card so the user can see which engine
				// didn't produce a translation.
				item = model.TranslateResult{
					Engine: meta.Name,
					From:   req.From,
					To:     req.To,
					Text:   req.Text,
					Result: "",
				}
			} else {
				s.saveHistory(res)
				item = *res
			}
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
			if s.app != nil {
				s.app.Event.Emit(events.EventScreenshotOCR, model.ScreenshotResult{
					Image:        imageURL,
					Text:         req.Text,
					Translations: partial,
					To:           to,
				})
			}
			mu.Unlock()
		}(i, t.meta, t.reg)
	}
	wg.Wait()
	// Filter out unfinished empty slots (theoretically all filled after wg.Wait; belt and
	// braces).
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
// logged, not surfaced).
func (s *Service) saveHistory(res *model.TranslateResult) {
	if s.history == nil || res == nil {
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
