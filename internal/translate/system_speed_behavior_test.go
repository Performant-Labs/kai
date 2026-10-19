package translate

import (
	"context"
	"strings"
	"testing"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #10 behavior tests. They drive the real service with fake engines and fail when the
// behavior is removed; the source-text guards in service_json_shape_test.go only pin wording.

func noticeEngine(name string, noticeFor func(text string) string) *fakeEngine {
	return &fakeEngine{name: name, run: func(_ context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		return &model.TranslateResult{Result: bracketed(req.Text), From: model.EN, To: req.To, Notice: noticeFor(req.Text)}, nil
	}}
}

func TestEngineNoticeReachesTheTranslateResult(t *testing.T) {
	eng := noticeEngine("probe", func(string) string { return "pack missing, used the slow mode" })
	svc, _, _ := newCancelService(t, eng)
	got, err := svc.Translate(model.TranslateRequest{Text: "hello there my friend", From: model.EN, To: model.ESMX, EngineName: "probe", RequestID: "n1"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if got.Notice != "pack missing, used the slow mode" {
		t.Errorf("Notice = %q, want the engine's notice carried into the result", got.Notice)
	}
}

func TestNoNoticeStaysEmpty(t *testing.T) {
	eng := noticeEngine("probe", func(string) string { return "" })
	svc, _, _ := newCancelService(t, eng)
	got, err := svc.Translate(model.TranslateRequest{Text: "hello there my friend", From: model.EN, To: model.ESMX, EngineName: "probe", RequestID: "n2"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if got.Notice != "" {
		t.Errorf("Notice = %q, want none when the engine reported none", got.Notice)
	}
}

// A chunked translation of one document: the notices of its parts must not stack up (16 parts
// would show 16 copies); the assembled result carries exactly one of them.
func TestChunkedResultCarriesOneEngineNotice(t *testing.T) {
	eng := noticeEngine("probe", func(text string) string {
		switch {
		case strings.Contains(text, "Part 02"):
			return "n-02"
		case strings.Contains(text, "Part 03"):
			return "n-03"
		}
		return ""
	})
	svc, _, _ := newCancelService(t, eng)
	setBudgets(svc, map[string]engine.Budget{"probe": budgetMax(t, engine.UnitRunes, chunkTestMax)})
	got, err := svc.Translate(model.TranslateRequest{Text: chunkDoc(3), From: model.EN, To: model.ESMX, EngineName: "probe", RequestID: "n3"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if got.Notice != "n-02" && got.Notice != "n-03" {
		t.Errorf("Notice = %q, want exactly one of the parts' notices (n-02 or n-03)", got.Notice)
	}
}

func startedEvents(em *recEmitter, id string) []events.TranslateProgressPayload {
	var out []events.TranslateProgressPayload
	for _, p := range em.progress() {
		if p.RequestID == id && p.Phase == events.ProgressPhaseStarted {
			out = append(out, p)
		}
	}
	return out
}

func TestStartedEventCarriesTheSystemEnginesEstimateAndSuggestion(t *testing.T) {
	long := strings.Repeat("word ", 8000) // 40,000 characters: over the 2-minute line at the default mode
	short := "a short sentence to translate"

	cases := []struct {
		name, eng, text, id string
		wantEstimate        bool
		wantSuggest         bool
	}{
		{"system engine, long text", "apple", long, "s-long", true, true},
		{"system engine, short text", "apple", short, "s-short", true, false},
		{"another engine, long text", "probe", long, "o-long", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			eng := &fakeEngine{name: c.eng, run: func(_ context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
				return &model.TranslateResult{Result: "x", From: model.EN, To: req.To}, nil
			}}
			svc, em, _ := newCancelService(t, eng)
			if _, err := svc.TranslateMulti(model.TranslateRequest{Text: c.text, From: model.EN, To: model.ESMX, RequestID: c.id}); err != nil {
				t.Fatalf("TranslateMulti: %v", err)
			}
			waitUntil(t, "result", func() bool { return len(em.resultsFor(c.id, c.eng)) == 1 })
			evs := startedEvents(em, c.id)
			if len(evs) != 1 {
				t.Fatalf("got %d started events, want 1", len(evs))
			}
			if (evs[0].EstimatedMs > 0) != c.wantEstimate {
				t.Errorf("EstimatedMs = %d, want an estimate: %v", evs[0].EstimatedMs, c.wantEstimate)
			}
			if evs[0].SuggestFast != c.wantSuggest {
				t.Errorf("SuggestFast = %v, want %v", evs[0].SuggestFast, c.wantSuggest)
			}
		})
	}
}
