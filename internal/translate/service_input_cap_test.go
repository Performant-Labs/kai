package translate

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #84 (T, RED): the 100,000-character cap (acceptance criterion 9). Characters are runes:
// 100,000 CJK characters (300,000 UTF-8 bytes) are accepted. Over the cap, the request is rejected
// before any engine call, with a typed error naming the limit and the length.
//
// Names these tests pin, in package translate:
//
//	type InputTooLongError struct{ Limit, Length int }  // *InputTooLongError implements error;
//	                                                     // its message is localized (i18n.T)
//	Service.captureRegion func(ctx context.Context) ([]byte, error) // defaults to engine.CaptureRegion;
//	                                                     // the seam that lets a test drive ScreenshotTranslate
//
// Placement (docs/handoffs/84-brief.md "Additional constraints", handoff-A.md warn 2):
// Translate and TranslateMulti check before requests.open; ScreenshotRetranslate checks the cached
// text before requests.open and returns the error (no push); ScreenshotTranslate opens its request
// before OCR, so it checks right after OCR, before the retranslate cache is written, and delivers
// the rejection through pushScreenshot as the OCR-failure case does.

const inputCap = 100000

func overCap() string { return strings.Repeat("字", inputCap+1) }

func assertCapError(t *testing.T, err error, length int) {
	t.Helper()
	if err == nil {
		t.Fatal("err = nil, want the input rejected over the 100,000-character cap")
	}
	var tl *InputTooLongError
	if !errors.As(err, &tl) {
		t.Fatalf("err = %v (%T), want an *InputTooLongError in its chain", err, err)
	}
	if tl.Limit != inputCap || tl.Length != length {
		t.Errorf("InputTooLongError{Limit: %d, Length: %d}, want {%d, %d}", tl.Limit, tl.Length, inputCap, length)
	}
	msg := err.Error()
	if !strings.Contains(msg, "100001") && !strings.Contains(msg, "100,001") {
		t.Errorf("message %q does not state the current length", msg)
	}
	if strings.HasPrefix(msg, "err.") {
		t.Errorf("message %q is an untranslated i18n key", msg)
	}
}

func TestInputCapRejectsOverLimitOnTranslate(t *testing.T) {
	eng := newScript("eng", nil)
	svc, _ := chunkedService(t, eng)
	_, err := svc.Translate(model.TranslateRequest{Text: overCap(), From: model.ZH, To: model.EN, EngineName: "eng"})
	assertCapError(t, err, inputCap+1)
	if n := len(eng.callList()); n != 0 {
		t.Errorf("engine called %d times, want 0", n)
	}
}

func TestInputCapRejectsOverLimitOnTranslateMulti(t *testing.T) {
	eng := newScript("eng", nil)
	svc, em := chunkedService(t, eng)
	_, err := svc.TranslateMulti(model.TranslateRequest{Text: overCap(), From: model.ZH, To: model.EN, RequestID: "r-cap"})
	assertCapError(t, err, inputCap+1)
	time.Sleep(20 * time.Millisecond) // engines run on their own goroutines; give a wrongly started one time to show
	if n := len(eng.callList()); n != 0 {
		t.Errorf("engine called %d times, want 0", n)
	}
	if n := svc.activeRequestCount(); n != 0 {
		t.Errorf("%d requests open, want none (rejected before requests.open)", n)
	}
	if evs := em.all(); len(evs) != 0 {
		t.Errorf("rejected request emitted %d events, want none: %+v", len(evs), evs)
	}
}

// Exactly 100,000 characters pass the cap (and are then handled per the engine's own budget; this
// engine has no row, so it is one call). They are 300,000 bytes: the cap counts characters.
func TestInputCapAcceptsExactlyTheLimit(t *testing.T) {
	eng := newScript("eng", nil)
	svc, em := chunkedService(t, eng)
	setBudgets(svc, nil)
	text := strings.Repeat("字", inputCap)
	if _, err := svc.TranslateMulti(model.TranslateRequest{Text: text, From: model.ZH, To: model.EN, RequestID: "r-at"}); err != nil {
		t.Fatalf("TranslateMulti at exactly the cap: %v", err)
	}
	got := terminal(t, svc, em, "r-at", "eng")
	if got.Error != "" || got.Result != bracketed(text) {
		t.Errorf("payload Error=%q, want a success at exactly 100,000 characters", got.Error)
	}
	if n := len(eng.callList()); n != 1 {
		t.Errorf("engine called %d times, want 1", n)
	}
}

// ScreenshotRetranslate reads the cached text before it opens a request: over the cap it returns
// the error and opens, pushes and calls nothing.
func TestInputCapOnScreenshotRetranslate(t *testing.T) {
	eng := newScript("eng", nil)
	svc, em := chunkedService(t, eng)
	seedCacheText(svc, events.ScreenshotSessionScreenshot, overCap())
	err := svc.ScreenshotRetranslate(events.ScreenshotSessionScreenshot, model.ZH, model.EN)
	assertCapError(t, err, inputCap+1)
	if n := len(em.screenshots()); n != 0 {
		t.Errorf("%d screenshot pushes, want none", n)
	}
	if n := len(eng.callList()); n != 0 {
		t.Errorf("engine called %d times, want 0", n)
	}
	if n := svc.activeRequestCount(); n != 0 {
		t.Errorf("%d requests open, want none", n)
	}
}

type fakeOCR struct{ text string }

func (f fakeOCR) Name() string { return "fakeocr" }
func (f fakeOCR) Recognize(ctx context.Context, req model.OcrRequest) (*model.OcrResult, error) {
	return &model.OcrResult{Engine: "fakeocr", Text: f.text}, nil
}

// ScreenshotTranslate learns the text only after OCR: over the cap it pushes the rejection as the
// flow's Error (the window stops waiting), returns the typed error, calls no engine, and does not
// cache the over-cap text for a retranslate.
func TestInputCapOnScreenshotTranslateAfterOCR(t *testing.T) {
	eng := newScript("eng", nil)
	svc, em := chunkedService(t, eng)
	svc.registry.RegisterOcr(fakeOCR{text: overCap()})
	svc.captureRegion = func(ctx context.Context) ([]byte, error) { return []byte("png"), nil }
	session := events.ScreenshotSessionScreenshot

	res, err := svc.ScreenshotTranslate(session)
	assertCapError(t, err, inputCap+1)
	if res != nil {
		t.Errorf("result = %+v, want nil on a rejection", res)
	}
	shots := em.screenshots()
	if len(shots) == 0 {
		t.Fatal("no screenshot push; the rejection must reach the window")
	}
	last := shots[len(shots)-1]
	if last.Error == "" || last.RequestID == "" || len(last.Translations) != 0 {
		t.Errorf("last push = {Error:%q RequestID:%q Translations:%d}, want the rejection with the run's id and no translations",
			last.Error, last.RequestID, len(last.Translations))
	}
	if n := len(eng.callList()); n != 0 {
		t.Errorf("engine called %d times, want 0", n)
	}
	svc.screenshotCacheMu.RLock()
	_, cached := svc.screenshotCache[session]
	svc.screenshotCacheMu.RUnlock()
	if cached {
		t.Error("the over-cap OCR text was cached for a retranslate")
	}
	if n := svc.activeRequestCount(); n != 0 {
		t.Errorf("%d requests open after the rejection, want none", n)
	}
}

// The cap message is localized: it differs between en-US and zh-CN.
func TestInputCapErrorIsLocalized(t *testing.T) {
	eng := newScript("eng", nil)
	svc, _ := chunkedService(t, eng)
	msg := func(loc string) string {
		useLocale(t, loc)
		_, err := svc.Translate(model.TranslateRequest{Text: overCap(), From: model.ZH, To: model.EN, EngineName: "eng"})
		assertCapError(t, err, inputCap+1)
		return err.Error()
	}
	en, zh := msg("en-US"), msg("zh-CN")
	if en == zh {
		t.Errorf("cap message is the same in en-US and zh-CN (%q); want it through i18n.T in every locale", en)
	}
}
