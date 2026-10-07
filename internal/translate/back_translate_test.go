package translate

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #56: the displayed result translated back into the source language, to check it in the
// user's own language. The request is an ordinary single-engine translation whose text is the
// result and whose pair is swapped by the caller; what BackTranslate adds is that it is a request
// of its own (cancellable, replaced by a newer one), writes no history, and never raises an error:
// a failed back-translation must not disturb the main result.

func backReq(eng, id string) model.TranslateRequest {
	return model.TranslateRequest{Text: "I need the bullet points", From: model.EN, To: model.ESMX, EngineName: eng, RequestID: id}
}

func TestBackTranslateReturnsTheEnginesAnswerForTheGivenPair(t *testing.T) {
	var mu sync.Mutex
	var seen model.TranslateRequest
	eng := &fakeEngine{name: "probe", run: func(_ context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		mu.Lock()
		seen = req
		mu.Unlock()
		return &model.TranslateResult{Result: "  Necesito los puntos clave  ", From: model.EN, To: req.To}, nil
	}}
	svc, _, _ := newCancelService(t, eng)
	got := svc.BackTranslate(backReq("probe", "b1"))
	if got.Status != model.BackTranslateOK || got.Result != "Necesito los puntos clave" || got.Engine != "probe" || got.RequestID != "b1" {
		t.Fatalf("got %+v, want ok with the trimmed answer from probe", got)
	}
	mu.Lock()
	defer mu.Unlock()
	// A pinned source is checked against a text of 20+ code points (#161): the engine is then sent
	// auto and detects it itself, so only the text and the target are pinned here.
	if seen.Text != "I need the bullet points" || seen.To != model.ESMX || (seen.From != model.EN && seen.From != model.Auto) {
		t.Errorf("the engine saw %+v, want the text, the target es-MX and the source en (or auto after the #161 check)", seen)
	}
}

func TestBackTranslateWritesNoHistory(t *testing.T) {
	eng := &fakeEngine{name: "probe", run: func(_ context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		return &model.TranslateResult{Result: "Necesito los puntos clave", From: req.From, To: req.To, Text: req.Text}, nil
	}}
	svc, _, hist := newCancelService(t, eng)
	if got := svc.BackTranslate(backReq("probe", "b1")); got.Status != model.BackTranslateOK {
		t.Fatalf("got %+v, want ok", got)
	}
	if n := historyRows(t, hist, "bullet"); n != 0 {
		t.Errorf("%d history rows after a back-translation, want 0", n)
	}
	// The ordinary single-engine call does write one: the test would not see the difference otherwise.
	if _, err := svc.Translate(backReq("probe", "")); err != nil {
		t.Fatal(err)
	}
	if n := historyRows(t, hist, "bullet"); n != 1 {
		t.Fatalf("control: Translate wrote %d rows, want 1", n)
	}
}

func TestBackTranslateNeverRaisesAnError(t *testing.T) {
	boom := &fakeEngine{name: "boom", run: func(context.Context, model.TranslateRequest) (*model.TranslateResult, error) {
		return nil, errors.New("provider said: I need the bullet points")
	}}
	empty := &fakeEngine{name: "empty", run: func(context.Context, model.TranslateRequest) (*model.TranslateResult, error) {
		return &model.TranslateResult{Result: "  "}, nil
	}}
	svc, _, _ := newCancelService(t, boom, empty)
	if got := svc.BackTranslate(backReq("boom", "b1")); got.Status != model.BackTranslateFailed || got.Result != "" {
		t.Errorf("engine error: got %+v, want failed and no text", got)
	}
	if got := svc.BackTranslate(backReq("empty", "b2")); got.Status != model.BackTranslateSkipped || got.Result != "" {
		t.Errorf("empty answer: got %+v, want skipped", got)
	}
	if got := svc.BackTranslate(backReq("nope", "b3")); got.Status != model.BackTranslateFailed {
		t.Errorf("unknown engine: got %+v, want failed", got)
	}
	long := backReq("empty", "b4")
	long.Text = strings.Repeat("a", 1<<20)
	if got := svc.BackTranslate(long); got.Status != model.BackTranslateFailed {
		t.Errorf("text over the input cap: got %+v, want failed", got)
	}
}

func TestBackTranslateSameLanguageIsSkipped(t *testing.T) {
	called := false
	eng := &fakeEngine{name: "probe", run: func(context.Context, model.TranslateRequest) (*model.TranslateResult, error) {
		called = true
		return okResult("x"), nil
	}}
	svc, _, _ := newCancelService(t, eng)
	req := backReq("probe", "b1")
	req.From, req.To = model.EN, model.EN
	if got := svc.BackTranslate(req); got.Status != model.BackTranslateSkipped || got.Result != "" {
		t.Fatalf("got %+v, want skipped (an identity result is not a back-translation)", got)
	}
	if called {
		t.Error("the engine was called for the same language on both sides")
	}
}

func TestBackTranslateCanBeCancelledAndIsReplacedByANewerOne(t *testing.T) {
	started := make(chan string, 4)
	eng := &fakeEngine{name: "slow", run: func(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		started <- req.RequestID
		<-ctx.Done()
		return nil, context.Cause(ctx)
	}}
	svc, _, _ := newCancelService(t, eng)

	// The user's Cancel.
	done := make(chan model.BackTranslateResult, 1)
	go func() { done <- svc.BackTranslate(backReq("slow", "c1")) }()
	<-started
	if !svc.CancelTranslate("c1", "") {
		t.Fatal("the back-translation was not cancellable")
	}
	select {
	case got := <-done:
		if got.Status != model.BackTranslateCancelled || got.Result != "" || got.RequestID != "c1" {
			t.Fatalf("got %+v, want cancelled", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the cancel did not reach the engine")
	}

	// A newer back-translation replaces the running one: the old one ends cancelled, silently.
	first := make(chan model.BackTranslateResult, 1)
	go func() { first <- svc.BackTranslate(backReq("slow", "r1")) }()
	<-started
	second := make(chan model.BackTranslateResult, 1)
	go func() { second <- svc.BackTranslate(backReq("slow", "r2")) }()
	select {
	case got := <-first:
		if got.Status != model.BackTranslateCancelled {
			t.Fatalf("replaced one: got %+v, want cancelled", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the newer request did not replace the older one")
	}
	<-started
	svc.CancelTranslate("r2", "")
	<-second
	if n := svc.activeRequestCount(); n != 0 {
		t.Errorf("%d requests still registered", n)
	}
}

// Over the wire, with the real google engine against a loopback server: the second call really
// carries the result as its text, with the swapped pair, and the answer really comes back.
func TestBackTranslateOverHTTPCarriesTheResultAndSwappedPair(t *testing.T) {
	var mu sync.Mutex
	var q, sl, tl string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		q, sl, tl = r.URL.Query().Get("q"), r.URL.Query().Get("sl"), r.URL.Query().Get("tl")
		mu.Unlock()
		_, _ = w.Write([]byte(`[[["Necesito los puntos clave","I need the bullet points",null,null,1]],null,"en"]`))
	}))
	defer srv.Close()
	reg := engine.NewRegistry()
	reg.RegisterTranslator(engine.NewGoogle(srv.URL, srv.Client()))
	svc := NewService(reg, nil, nil, nil)

	got := svc.BackTranslate(model.TranslateRequest{Text: "I need the bullet points", From: model.EN, To: model.ES, EngineName: "google", RequestID: "h1"})
	if got.Status != model.BackTranslateOK || got.Result != "Necesito los puntos clave" || got.Engine != "google" {
		t.Fatalf("got %+v, want ok from google", got)
	}
	mu.Lock()
	defer mu.Unlock()
	// The source goes out as auto for a text of 20+ code points (#161), and the target is es.
	if q != "I need the bullet points" || (!strings.HasPrefix(sl, "en") && sl != "auto") || !strings.HasPrefix(tl, "es") {
		t.Errorf("the server saw q=%q sl=%q tl=%q, want the result text, en (or auto) -> es", q, sl, tl)
	}
}
