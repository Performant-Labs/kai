package translate

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/events"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #84 (T, RED): the per-engine chunk loop behind translateWithEngine (callEngineChunked),
// driven through the public entry points with fake engines. No real engine and no network.
//
// Names these tests pin, in package translate, beyond the splitter's (chunk_test.go):
//
//	Service.budgetOf func(name string) (engine.Budget, bool) // defaults to engine.InputBudget
//
// Documents are built from chunkPara: each paragraph is ONE 60-rune sentence with no clause
// marks, and the fake budget's Max is 100 runes. So a paragraph always fits Max, two never do,
// and the split is one chunk per paragraph whatever target the chunk-size rule picks for an
// engine with no latency data (a sentence that fits Max is never hard split).

const chunkTestMax = 100

// chunkPara is paragraph i: one 60-rune ASCII sentence tagged "Part NN" so a fake engine knows
// which chunk it was handed. "zorblax" is in every paragraph, for the history query.
func chunkPara(i int) string {
	s := fmt.Sprintf("Part %02d zorblax %s", i, wordRun(12))
	return s[:59] + "."
}

// chunkDoc is n paragraphs (1..n) separated by blank lines.
func chunkDoc(n int) string {
	ps := make([]string, n)
	for i := range ps {
		ps[i] = chunkPara(i + 1)
	}
	return strings.Join(ps, "\n\n")
}

// bracketed is what the default fake engine returns for text; bracketedDoc is the reassembled
// translation of paragraphs from..to of a chunkDoc, joined with its blank-line separators.
func bracketed(text string) string { return "[" + text + "]" }

func bracketedDoc(from, to int) string {
	ps := []string{}
	for i := from; i <= to; i++ {
		ps = append(ps, bracketed(chunkPara(i)))
	}
	return strings.Join(ps, "\n\n")
}

// paraIndex reads the "Part NN" tag of the first paragraph in text; -1 when there is none.
func paraIndex(text string) int {
	at := strings.Index(text, "Part ")
	if at < 0 {
		return -1
	}
	var n int
	if _, err := fmt.Sscanf(text[at:], "Part %02d", &n); err != nil {
		return -1
	}
	return n
}

// ---- scripted fake engine ----------------------------------------------------------------

type scriptEngine struct {
	name   string
	behave func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error)

	mu          sync.Mutex
	calls       []model.TranslateRequest
	inflight    int
	maxInflight int
	started     map[int]bool
	returned    map[int]bool
	sawCancel   map[int]bool
}

func newScript(name string, behave func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error)) *scriptEngine {
	return &scriptEngine{name: name, behave: behave,
		started: map[int]bool{}, returned: map[int]bool{}, sawCancel: map[int]bool{}}
}

func (e *scriptEngine) Name() string { return e.name }

func (e *scriptEngine) Translate(ctx context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
	idx := paraIndex(req.Text)
	e.mu.Lock()
	e.calls = append(e.calls, req)
	e.inflight++
	if e.inflight > e.maxInflight {
		e.maxInflight = e.inflight
	}
	e.started[idx] = true
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.inflight--
		e.returned[idx] = true
		e.mu.Unlock()
	}()
	if e.behave != nil {
		return e.behave(ctx, e, idx, req)
	}
	return &model.TranslateResult{Result: bracketed(req.Text)}, nil
}

func (e *scriptEngine) callList() []model.TranslateRequest {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]model.TranslateRequest(nil), e.calls...)
}

func (e *scriptEngine) is(m map[int]bool, idx int) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return m[idx]
}

func (e *scriptEngine) hasStarted(idx int) bool  { return e.is(e.started, idx) }
func (e *scriptEngine) hasReturned(idx int) bool { return e.is(e.returned, idx) }
func (e *scriptEngine) cancelSeen(idx int) bool  { return e.is(e.sawCancel, idx) }

func (e *scriptEngine) peakInflight() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.maxInflight
}

// blockOnCtx holds a chunk until its ctx ends, recording that it saw the cancel. It gives up
// after 5 s so a missing cancel is a failed assertion, not a hung test.
func blockOnCtx(ctx context.Context, e *scriptEngine, idx int) (*model.TranslateResult, error) {
	select {
	case <-ctx.Done():
		e.mu.Lock()
		e.sawCancel[idx] = true
		e.mu.Unlock()
		return nil, fmt.Errorf("%s", ctx.Err().Error())
	case <-time.After(5 * time.Second):
		return nil, errors.New("chunk was never cancelled")
	}
}

// waitFor polls cond for up to d without failing; it reports whether cond came to hold.
func waitFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
	return true
}

// ---- service helpers ----------------------------------------------------------------------

func setBudgets(svc *Service, m map[string]engine.Budget) {
	svc.budgetOf = func(name string) (engine.Budget, bool) {
		b, ok := m[name]
		return b, ok
	}
}

func chunkedService(t *testing.T, engs ...*scriptEngine) (*Service, *recEmitter) {
	t.Helper()
	trs := make([]engine.Translator, len(engs))
	budgets := map[string]engine.Budget{}
	for i, e := range engs {
		trs[i] = e
		budgets[e.name] = budgetMax(t, engine.UnitRunes, chunkTestMax)
	}
	svc, em, _ := newCancelService(t, trs...)
	setBudgets(svc, budgets)
	return svc, em
}

func seedCacheText(svc *Service, session, text string) {
	svc.screenshotCacheMu.Lock()
	svc.screenshotCache[session] = ocrCache{text: text, imageURL: "data:image/png;base64,AAAA"}
	svc.screenshotCacheMu.Unlock()
}

func useLocale(t *testing.T, loc string) {
	t.Helper()
	prev := i18n.GetLocale()
	i18n.SetLocale(loc)
	t.Cleanup(func() { i18n.SetLocale(prev) })
}

// terminal waits for the one terminal result of engine eng in request id, then for the registry
// to settle, and checks there is exactly one.
func terminal(t *testing.T, svc *Service, em *recEmitter, id, eng string) model.TranslateResult {
	t.Helper()
	waitUntil(t, "terminal result of "+eng, func() bool { return len(em.resultsFor(id, eng)) >= 1 })
	waitUntil(t, "registry empty", func() bool { return svc.activeRequestCount() == 0 })
	rs := em.resultsFor(id, eng)
	if len(rs) != 1 {
		t.Fatalf("%d terminal results for %s, want exactly 1: %+v", len(rs), eng, rs)
	}
	return rs[0]
}

func chunkEvents(em *recEmitter, id, eng string) []events.TranslateProgressPayload {
	var out []events.TranslateProgressPayload
	for _, p := range em.progress() {
		if p.Phase == events.ProgressPhaseChunk && p.RequestID == id && p.Engine == eng {
			out = append(out, p)
		}
	}
	return out
}

func startedID(t *testing.T, em *recEmitter, eng string) string {
	t.Helper()
	var id string
	waitUntil(t, "started event of "+eng, func() bool {
		for _, p := range em.progress() {
			if p.Phase == events.ProgressPhaseStarted && p.Engine == eng {
				id = p.RequestID
				return true
			}
		}
		return false
	})
	return id
}

// ---- chunking and reassembly ------------------------------------------------------------

// Text over the engine's Max is sent as one call per chunk and reassembled in source order with
// the recorded separators; the assembled result holds the whole input as Text, no Phonetic or
// Dict, and it is saved to history once.
func TestChunkedTranslateMultiSplitsAndReassemblesInOrder(t *testing.T) {
	eng := newScript("eng", func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error) {
		return &model.TranslateResult{Result: bracketed(req.Text), Phonetic: "ph", Dict: []model.DictItem{{Word: "w"}}}, nil
	})
	svc, em := chunkedService(t, eng)
	hist := svc.history
	text := chunkDoc(3)
	if _, err := svc.TranslateMulti(model.TranslateRequest{Text: text, From: model.ES, To: model.EN, RequestID: "r-ck"}); err != nil {
		t.Fatal(err)
	}
	got := terminal(t, svc, em, "r-ck", "eng")

	calls := eng.callList()
	if len(calls) != 3 {
		t.Fatalf("engine called %d times, want 3 (one per chunk)", len(calls))
	}
	seen := map[string]bool{}
	for _, c := range calls {
		seen[strings.TrimSpace(c.Text)] = true
		// #161: a pinned request over the 20-code-point floor sends chunk 1 as auto; this engine
		// reports no detection, so every later chunk falls back to the pin.
		wantFrom := model.ES
		if paraIndex(c.Text) == 1 {
			wantFrom = model.Auto
		}
		if c.To != model.EN || c.From != wantFrom {
			t.Errorf("chunk request %q has From=%q To=%q, want %s→en", c.Text, c.From, c.To, wantFrom)
		}
	}
	for i := 1; i <= 3; i++ {
		if !seen[chunkPara(i)] {
			t.Errorf("paragraph %d was never sent as its own chunk; calls: %+v", i, calls)
		}
	}
	if got.Error != "" || got.Cancelled {
		t.Fatalf("payload = %+v, want a success", got)
	}
	if want := bracketedDoc(1, 3); got.Result != want {
		t.Errorf("Result =\n%q\nwant\n%q", got.Result, want)
	}
	if got.Text != text {
		t.Errorf("Text = %q, want the whole input", got.Text)
	}
	if got.Phonetic != "" || len(got.Dict) != 0 {
		t.Errorf("multi-chunk result carries Phonetic=%q Dict=%v, want both empty", got.Phonetic, got.Dict)
	}
	if got.From != model.ES || got.To != model.EN {
		t.Errorf("From/To = %q/%q, want es/en", got.From, got.To)
	}
	if n := historyRows(t, hist, "zorblax"); n != 1 {
		t.Errorf("history rows = %d, want exactly 1 for the whole request", n)
	}
}

// The single entry point chunks too (it has no request id, so it reports no progress).
func TestChunkedTranslateSingleEntryPoint(t *testing.T) {
	eng := newScript("eng", nil)
	svc, _ := chunkedService(t, eng)
	res, err := svc.Translate(model.TranslateRequest{Text: chunkDoc(3), From: model.ES, To: model.EN, EngineName: "eng"})
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if n := len(eng.callList()); n != 3 {
		t.Errorf("engine called %d times, want 3", n)
	}
	if want := bracketedDoc(1, 3); res.Result != want {
		t.Errorf("Result = %q, want %q", res.Result, want)
	}
	if n := historyRows(t, svc.history, "zorblax"); n != 1 {
		t.Errorf("history rows = %d, want 1", n)
	}
}

// Text within Max makes exactly one call, byte for byte the request as today, and the payload is
// identical to the one an engine without a budget row (never chunked) produces.
func TestChunkedSingleChunkIsByteIdenticalToUnchunked(t *testing.T) {
	text := " " + chunkPara(1) + " " + chunkPara(2)[:37] + "\n" // exactly 100 runes = Max
	if n := len([]rune(text)); n != chunkTestMax {
		t.Fatalf("setup: text has %d runes, want %d", n, chunkTestMax)
	}
	run := func(withBudget bool) (model.TranslateResult, []model.TranslateRequest) {
		eng := newScript("eng", func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error) {
			return &model.TranslateResult{Result: "R", Phonetic: "ph", Dict: []model.DictItem{{Word: "w", Pos: "n.", Explain: "x"}}}, nil
		})
		svc, em := chunkedService(t, eng)
		if !withBudget {
			setBudgets(svc, nil)
		}
		if _, err := svc.TranslateMulti(model.TranslateRequest{Text: text, From: model.ES, To: model.EN, RequestID: "r-1"}); err != nil {
			t.Fatal(err)
		}
		got := terminal(t, svc, em, "r-1", "eng")
		got.RequestID = ""
		return got, eng.callList()
	}
	plain, _ := run(false)
	chunked, calls := run(true)
	if len(calls) != 1 {
		t.Fatalf("engine called %d times for text within Max, want exactly 1", len(calls))
	}
	if calls[0].Text != text {
		t.Errorf("the one call carried %q, want the request text unchanged %q", calls[0].Text, text)
	}
	if !reflect.DeepEqual(plain, chunked) {
		t.Errorf("single-chunk payload differs from today's:\n got %+v\nwant %+v", chunked, plain)
	}
}

// An engine with no budget row is sent the whole text in one call; the lookup never panics.
func TestChunkedEngineWithoutBudgetRowIsSentWhole(t *testing.T) {
	eng := newScript("eng", nil)
	svc, em := chunkedService(t, eng)
	setBudgets(svc, nil)
	text := chunkDoc(30)
	if _, err := svc.TranslateMulti(model.TranslateRequest{Text: text, From: model.ES, To: model.EN, RequestID: "r-nr"}); err != nil {
		t.Fatal(err)
	}
	got := terminal(t, svc, em, "r-nr", "eng")
	if calls := eng.callList(); len(calls) != 1 || calls[0].Text != text {
		t.Fatalf("calls = %d, want one call with the whole text", len(calls))
	}
	if got.Result != bracketed(text) {
		t.Errorf("Result = %q, want the one call's answer", got.Result)
	}
}

// The service's budget source defaults to the real table.
func TestServiceBudgetDefaultsToInputBudgetTable(t *testing.T) {
	svc := NewService(engine.NewRegistry(), nil, nil, nil)
	if svc.budgetOf == nil {
		t.Fatal("budgetOf is nil; want engine.InputBudget by default")
	}
	for _, name := range []string{"apple", "google", "deepl", "nope"} {
		gotB, gotOK := svc.budgetOf(name)
		wantB, wantOK := engine.InputBudget(name)
		if gotB != wantB || gotOK != wantOK {
			t.Errorf("budgetOf(%q) = %+v,%v, want %+v,%v", name, gotB, gotOK, wantB, wantOK)
		}
	}
}

// Two engines with different budgets chunk the same text differently in the same request.
func TestChunkedEnginesChunkIndependently(t *testing.T) {
	wide, narrow := newScript("wide", nil), newScript("narrow", nil)
	svc, em := chunkedService(t, wide, narrow)
	setBudgets(svc, map[string]engine.Budget{
		"wide":   budgetMax(t, engine.UnitRunes, 10000),
		"narrow": budgetMax(t, engine.UnitRunes, chunkTestMax),
	})
	text := chunkDoc(3)
	if _, err := svc.TranslateMulti(model.TranslateRequest{Text: text, From: model.ES, To: model.EN, RequestID: "r-2e"}); err != nil {
		t.Fatal(err)
	}
	gw := terminal(t, svc, em, "r-2e", "wide")
	gn := terminal(t, svc, em, "r-2e", "narrow")
	if n := len(wide.callList()); n != 1 {
		t.Errorf("wide engine called %d times, want 1 (text fits its Max)", n)
	}
	if n := len(narrow.callList()); n != 3 {
		t.Errorf("narrow engine called %d times, want 3", n)
	}
	if gw.Result != bracketed(text) {
		t.Errorf("wide Result = %q", gw.Result)
	}
	if want := bracketedDoc(1, 3); gn.Result != want {
		t.Errorf("narrow Result = %q, want %q", gn.Result, want)
	}
}

// ---- progress ---------------------------------------------------------------------------

func assertChunkProgress(t *testing.T, got []events.TranslateProgressPayload, total int) {
	t.Helper()
	if len(got) != total {
		t.Fatalf("%d chunk progress events, want %d (one per part): %+v", len(got), total, got)
	}
	for i, p := range got {
		if p.Done != i+1 || p.Total != total {
			t.Errorf("event %d = done %d / total %d, want %d / %d", i, p.Done, p.Total, i+1, total)
		}
		if p.StartedAtMs <= 0 {
			t.Errorf("event %d StartedAtMs = %d, want the engine's start", i, p.StartedAtMs)
		}
	}
}

// reportProgress fires once per completed chunk, done 1..total, before the terminal result.
func TestChunkedProgressOncePerPart(t *testing.T) {
	eng := newScript("eng", nil)
	svc, em := chunkedService(t, eng)
	if _, err := svc.TranslateMulti(model.TranslateRequest{Text: chunkDoc(3), From: model.ES, To: model.EN, RequestID: "r-pg"}); err != nil {
		t.Fatal(err)
	}
	terminal(t, svc, em, "r-pg", "eng")
	assertChunkProgress(t, chunkEvents(em, "r-pg", "eng"), 3)
	lastChunk, result := -1, -1
	for i, e := range em.all() {
		switch v := e.data.(type) {
		case events.TranslateProgressPayload:
			if v.Phase == events.ProgressPhaseChunk && v.RequestID == "r-pg" {
				lastChunk = i
			}
		case model.TranslateResult:
			if v.RequestID == "r-pg" {
				result = i
			}
		}
	}
	if lastChunk > result {
		t.Errorf("a chunk progress event (index %d) follows the terminal result (index %d)", lastChunk, result)
	}
}

// A hard-split sentence is one part in Done/Total, however many calls its pieces take.
func TestChunkedProgressCountsAHardSplitSentenceOnce(t *testing.T) {
	eng := newScript("eng", nil)
	svc, em := chunkedService(t, eng)
	text := chunkPara(1) + "\n\n" + wordRun(50) // the second paragraph: ~280 runes, no punctuation
	if _, err := svc.TranslateMulti(model.TranslateRequest{Text: text, From: model.ES, To: model.EN, RequestID: "r-hs"}); err != nil {
		t.Fatal(err)
	}
	got := terminal(t, svc, em, "r-hs", "eng")
	if n := len(eng.callList()); n < 4 {
		t.Errorf("engine called %d times, want the long sentence in at least 3 pieces", n)
	}
	assertChunkProgress(t, chunkEvents(em, "r-hs", "eng"), 2)
	if got.Error != "" || !strings.HasPrefix(got.Result, bracketed(chunkPara(1))+"\n\n") {
		t.Errorf("payload = %+v, want paragraph 1's translation, the separator, then the sentence", got)
	}
}

// The screenshot flow reports chunk progress too, under its own request id.
func TestChunkedProgressOnScreenshotRetranslate(t *testing.T) {
	eng := newScript("eng", nil)
	svc, em := chunkedService(t, eng)
	seedCacheText(svc, events.ScreenshotSessionScreenshot, chunkDoc(3))
	if err := svc.ScreenshotRetranslate(events.ScreenshotSessionScreenshot, model.ES, model.EN); err != nil {
		t.Fatalf("ScreenshotRetranslate: %v", err)
	}
	id := startedID(t, em, "eng")
	assertChunkProgress(t, chunkEvents(em, id, "eng"), 3)
	shots := em.screenshots()
	last := shots[len(shots)-1]
	if len(last.Translations) != 1 || last.Translations[0].Result != bracketedDoc(1, 3) {
		t.Errorf("last push translations = %+v, want the reassembled document", last.Translations)
	}
}

// ---- failure ----------------------------------------------------------------------------

// A chunk failure sends no further chunk, cancels the sibling still in flight, leaves Result
// empty, and names how far it got in Error ("part N of M"). The part text survives a long engine
// reason (SanitizeDetail truncation) and the failure keeps the failing chunk's classification.
func TestChunkFailureStopsCancelsSiblingAndNamesPart(t *testing.T) {
	useLocale(t, "en-US")
	var sawSibling bool
	eng := newScript("eng", nil)
	eng.behave = func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error) {
		switch idx {
		case 2:
			sawSibling = waitFor(3*time.Second, func() bool { return e.hasStarted(3) })
			return nil, &engine.HTTPError{Status: 429, Message: "slow down " + strings.Repeat("x", 800)}
		case 3:
			return blockOnCtx(ctx, e, idx)
		default:
			return &model.TranslateResult{Result: bracketed(req.Text)}, nil
		}
	}
	svc, em := chunkedService(t, eng)
	if _, err := svc.TranslateMulti(model.TranslateRequest{Text: chunkDoc(5), From: model.ES, To: model.EN, RequestID: "r-fl"}); err != nil {
		t.Fatal(err)
	}
	got := terminal(t, svc, em, "r-fl", "eng")
	if !sawSibling {
		t.Error("chunk 3 was never in flight beside chunk 2: dispatch is not 2 at a time")
	}
	if got.Result != "" || got.Cancelled {
		t.Errorf("failure payload Result=%q Cancelled=%v, want empty Result and not cancelled", got.Result, got.Cancelled)
	}
	if !strings.Contains(got.Error, "part 1 of 5") {
		t.Errorf("Error = %q, want it to say it stopped at part 1 of 5", got.Error)
	}
	if !strings.Contains(got.Error, "slow down") {
		t.Errorf("Error = %q, want the failing chunk's reason kept", got.Error)
	}
	if got.ErrorKind != model.ErrorKindRateLimit {
		t.Errorf("ErrorKind = %q, want %q (the failing chunk's own classification)", got.ErrorKind, model.ErrorKindRateLimit)
	}
	if eng.hasStarted(4) || eng.hasStarted(5) {
		t.Error("a chunk after the failure was sent")
	}
	if !waitFor(2*time.Second, func() bool { return eng.cancelSeen(3) }) {
		t.Error("chunk 3, in flight when chunk 2 failed, was not cancelled")
	}
}

// N in "part N of M" is the contiguous run of finished parts from part 1: part 3 finished while
// part 2 was still running, then part 2 failed, so N is 1, not 2.
func TestChunkFailurePartIsContiguousFromPartOne(t *testing.T) {
	useLocale(t, "en-US")
	eng := newScript("eng", nil)
	eng.behave = func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error) {
		switch {
		case idx == 2:
			waitFor(3*time.Second, func() bool { return e.hasReturned(3) })
			return nil, errors.New("engine broke")
		case idx >= 4:
			return blockOnCtx(ctx, e, idx)
		default:
			return &model.TranslateResult{Result: bracketed(req.Text)}, nil
		}
	}
	svc, em := chunkedService(t, eng)
	if _, err := svc.TranslateMulti(model.TranslateRequest{Text: chunkDoc(5), From: model.ES, To: model.EN, RequestID: "r-fc"}); err != nil {
		t.Fatal(err)
	}
	got := terminal(t, svc, em, "r-fc", "eng")
	if !eng.hasReturned(3) {
		t.Fatal("part 3 was never sent and finished while part 2 ran: no chunked 2-wide dispatch")
	}
	if got.Result != "" || !strings.Contains(got.Error, "part 1 of 5") {
		t.Errorf("payload Result=%q Error=%q, want empty Result and part 1 of 5", got.Result, got.Error)
	}
	if eng.hasStarted(5) {
		t.Error("chunk 5 was sent after the failure")
	}
}

// The screenshot flow's failure placeholder follows the same contract.
func TestChunkFailureOnScreenshotRetranslate(t *testing.T) {
	useLocale(t, "en-US")
	eng := newScript("eng", func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error) {
		if idx == 2 {
			return nil, errors.New("engine broke")
		}
		return &model.TranslateResult{Result: bracketed(req.Text)}, nil
	})
	svc, em := chunkedService(t, eng)
	seedCacheText(svc, events.ScreenshotSessionScreenshot, chunkDoc(3))
	if err := svc.ScreenshotRetranslate(events.ScreenshotSessionScreenshot, model.ES, model.EN); err != nil {
		t.Fatalf("ScreenshotRetranslate: %v", err)
	}
	shots := em.screenshots()
	tr := shots[len(shots)-1].Translations
	if len(tr) != 1 {
		t.Fatalf("translations = %+v, want one failure placeholder", tr)
	}
	if tr[0].Result != "" || tr[0].Cancelled || !strings.Contains(tr[0].Error, " of 3") || !strings.Contains(tr[0].Error, "part ") {
		t.Errorf("placeholder = %+v, want empty Result and Error naming part N of 3", tr[0])
	}
}

// ---- cancel -----------------------------------------------------------------------------

// A cancel between chunks is a cancel, not a failure: Result carries the reassembled parts that
// finished, with their separator, nothing further is sent, and the engine reports exactly once.
func TestChunkCancelKeepsReassembledPrefix(t *testing.T) {
	eng := newScript("eng", nil)
	eng.behave = func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error) {
		if idx >= 3 {
			return blockOnCtx(ctx, e, idx)
		}
		return &model.TranslateResult{Result: bracketed(req.Text)}, nil
	}
	svc, em := chunkedService(t, eng)
	if _, err := svc.TranslateMulti(model.TranslateRequest{Text: chunkDoc(5), From: model.ES, To: model.EN, RequestID: "r-cx"}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "chunks 3 and 4 in flight", func() bool { return eng.hasStarted(3) && eng.hasStarted(4) })
	if !svc.CancelTranslate("r-cx", "eng") {
		t.Fatal("CancelTranslate found nothing running")
	}
	got := terminal(t, svc, em, "r-cx", "eng")
	if !got.Cancelled || got.Error != "" || got.ErrorKind != "" {
		t.Errorf("payload = %+v, want Cancelled with no Error/ErrorKind", got)
	}
	if res := strings.TrimRightFunc(got.Result, unicode.IsSpace); res != bracketedDoc(1, 2) {
		t.Errorf("Result = %q, want the reassembled prefix %q", got.Result, bracketedDoc(1, 2))
	}
	if eng.hasStarted(5) {
		t.Error("chunk 5 was sent after the cancel")
	}
	if n := historyRows(t, svc.history, "zorblax"); n != 0 {
		t.Errorf("a cancelled request wrote %d history rows, want 0", n)
	}
}

// The prefix is the contiguous run from part 1: part 3 finished while part 2 was still running,
// so after the cancel Result holds part 1 only, with no gap.
func TestChunkCancelPrefixIsContiguousFromPartOne(t *testing.T) {
	eng := newScript("eng", nil)
	eng.behave = func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error) {
		if idx == 2 || idx >= 4 {
			return blockOnCtx(ctx, e, idx)
		}
		return &model.TranslateResult{Result: bracketed(req.Text)}, nil
	}
	svc, em := chunkedService(t, eng)
	if _, err := svc.TranslateMulti(model.TranslateRequest{Text: chunkDoc(5), From: model.ES, To: model.EN, RequestID: "r-oo"}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "part 3 finished while part 2 runs", func() bool { return eng.hasReturned(3) })
	svc.CancelTranslate("r-oo", "")
	got := terminal(t, svc, em, "r-oo", "eng")
	if !got.Cancelled {
		t.Fatalf("payload = %+v, want Cancelled", got)
	}
	if res := strings.TrimRightFunc(got.Result, unicode.IsSpace); res != bracketed(chunkPara(1)) {
		t.Errorf("Result = %q, want part 1 only %q (part 3 is not contiguous)", got.Result, bracketed(chunkPara(1)))
	}
	if eng.hasStarted(5) {
		t.Error("chunk 5 was sent after the cancel")
	}
}

// The screenshot flow's cancelled placeholder carries the prefix too.
func TestChunkCancelPrefixOnScreenshotRetranslate(t *testing.T) {
	eng := newScript("eng", nil)
	eng.behave = func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error) {
		if idx >= 3 {
			return blockOnCtx(ctx, e, idx)
		}
		return &model.TranslateResult{Result: bracketed(req.Text)}, nil
	}
	svc, em := chunkedService(t, eng)
	seedCacheText(svc, events.ScreenshotSessionScreenshot, chunkDoc(4))
	done := make(chan error, 1)
	go func() { done <- svc.ScreenshotRetranslate(events.ScreenshotSessionScreenshot, model.ES, model.EN) }()
	waitUntil(t, "chunks 3 and 4 in flight", func() bool { return eng.hasStarted(3) && eng.hasStarted(4) })
	id := startedID(t, em, "eng")
	svc.CancelTranslate(id, "eng")
	if err := <-done; err != nil {
		t.Fatalf("ScreenshotRetranslate: %v", err)
	}
	shots := em.screenshots()
	tr := shots[len(shots)-1].Translations
	if len(tr) != 1 || !tr[0].Cancelled || tr[0].Error != "" {
		t.Fatalf("translations = %+v, want one cancelled placeholder", tr)
	}
	if res := strings.TrimRightFunc(tr[0].Result, unicode.IsSpace); res != bracketedDoc(1, 2) {
		t.Errorf("placeholder Result = %q, want the prefix %q", tr[0].Result, bracketedDoc(1, 2))
	}
}

// ---- source language ----------------------------------------------------------------------

// On an auto request chunk 1 is sent alone, and its recognized detection is pinned as From for
// every later chunk, even when the engine would detect something else for them.
func TestChunkSourcePinnedFromChunkOne(t *testing.T) {
	release := make(chan struct{})
	eng := newScript("eng", func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error) {
		if idx == 1 {
			<-release
			return &model.TranslateResult{Result: bracketed(req.Text), From: model.ES}, nil
		}
		return &model.TranslateResult{Result: bracketed(req.Text), From: model.FR}, nil
	})
	svc, em := chunkedService(t, eng)
	if _, err := svc.TranslateMulti(model.TranslateRequest{Text: chunkDoc(4), From: model.Auto, To: model.EN, RequestID: "r-pin"}); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "chunk 1 started", func() bool { return eng.hasStarted(1) })
	time.Sleep(30 * time.Millisecond)
	if n := len(eng.callList()); n != 1 {
		t.Errorf("%d calls while chunk 1 was running, want chunk 1 alone on an auto request", n)
	}
	close(release)
	got := terminal(t, svc, em, "r-pin", "eng")
	for _, c := range eng.callList() {
		idx := paraIndex(c.Text)
		switch {
		case idx == 1 && c.From != model.Auto:
			t.Errorf("chunk 1 From = %q, want auto", c.From)
		case idx > 1 && c.From != model.ES:
			t.Errorf("chunk %d From = %q, want es pinned from chunk 1", idx, c.From)
		}
	}
	if got.From != model.ES || got.Error != "" || got.Result != bracketedDoc(1, 4) {
		t.Errorf("payload = %+v, want From es and the reassembled document", got)
	}
}

// A detection the app does not recognize (youdao's direction pair, baidu's native codes) is not
// pinned: every later chunk keeps the request's own From.
func TestChunkUnrecognizedDetectionIsNotPinned(t *testing.T) {
	for _, code := range []model.Language{"zh-CHS2en", "jp", "kor"} {
		t.Run(string(code), func(t *testing.T) {
			eng := newScript("eng", func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error) {
				return &model.TranslateResult{Result: bracketed(req.Text), From: code}, nil
			})
			svc, em := chunkedService(t, eng)
			if _, err := svc.TranslateMulti(model.TranslateRequest{Text: chunkDoc(3), From: model.Auto, To: model.EN, RequestID: "r-yd"}); err != nil {
				t.Fatal(err)
			}
			got := terminal(t, svc, em, "r-yd", "eng")
			calls := eng.callList()
			if len(calls) != 3 {
				t.Fatalf("engine called %d times, want 3", len(calls))
			}
			for _, c := range calls {
				if c.From != model.Auto {
					t.Errorf("chunk %d From = %q, want auto (the unrecognized %q must not be pinned)", paraIndex(c.Text), c.From, code)
				}
			}
			if got.Error != "" {
				t.Errorf("payload = %+v, want a success", got)
			}
		})
	}
}

// An auto request whose chunk 1 detects the target language is an identity result after exactly
// one engine call, whether chunk 1 answered or failed with the detection attached. Chunks 2..N are
// never sent (a same-language chunk would be rejected by engines that refuse the pair).
func TestChunkOneIdentityShortCircuit(t *testing.T) {
	cases := map[string]func() (*model.TranslateResult, error){
		"answered": func() (*model.TranslateResult, error) {
			return &model.TranslateResult{Result: "echo", From: model.EN}, nil
		},
		"failed with detection": func() (*model.TranslateResult, error) {
			return nil, engine.WithDetectedSource(errors.New("cannot translate a language into itself"), model.EN)
		},
	}
	for name, first := range cases {
		t.Run(name, func(t *testing.T) {
			eng := newScript("eng", func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error) {
				if idx == 1 {
					return first()
				}
				return nil, fmt.Errorf("same-language pair: %w", engine.ErrUnsupportedPair)
			})
			svc, em := chunkedService(t, eng)
			text := chunkDoc(3)
			if _, err := svc.TranslateMulti(model.TranslateRequest{Text: text, From: model.Auto, To: model.EN, RequestID: "r-id1"}); err != nil {
				t.Fatal(err)
			}
			got := terminal(t, svc, em, "r-id1", "eng")
			calls := eng.callList()
			if len(calls) != 1 {
				t.Errorf("engine called %d times, want exactly 1 (chunk 1 only)", len(calls))
			}
			if len(calls) > 0 && strings.TrimSpace(calls[0].Text) != chunkPara(1) {
				t.Errorf("the one call carried %q, want chunk 1 alone (the text is over Max)", calls[0].Text)
			}
			if !got.Identity || got.Result != text || got.Error != "" {
				t.Errorf("payload = %+v, want an identity result carrying the whole text", got)
			}
		})
	}
}

// ---- concurrency ------------------------------------------------------------------------

// At most 2 chunks are in flight per engine, and the reassembly is in source order although the
// test releases every pair in reverse.
func TestChunkConcurrencyAtMostTwoAndOrdered(t *testing.T) {
	const n = 6
	release := map[int]chan struct{}{}
	for i := 1; i <= n; i++ {
		release[i] = make(chan struct{})
	}
	eng := newScript("eng", func(ctx context.Context, e *scriptEngine, idx int, req model.TranslateRequest) (*model.TranslateResult, error) {
		select {
		case <-release[idx]:
			return &model.TranslateResult{Result: bracketed(req.Text)}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	svc, em := chunkedService(t, eng)
	if _, err := svc.TranslateMulti(model.TranslateRequest{Text: chunkDoc(n), From: model.ES, To: model.EN, RequestID: "r-cc"}); err != nil {
		t.Fatal(err)
	}
	freed := map[int]bool{}
	waiting := func() []int {
		var out []int
		for i := 1; i <= n; i++ {
			if eng.hasStarted(i) && !freed[i] {
				out = append(out, i)
			}
		}
		return out
	}
	for len(freed) < n {
		// Wait for a chunk to be in flight, then briefly for a second one (a dispatcher that keeps
		// its window on the oldest unfinished chunk has only one to offer here), and release the
		// later of them first, so completions arrive out of source order.
		waitUntil(t, "a chunk in flight", func() bool { return len(waiting()) >= 1 })
		if n-len(freed) >= 2 {
			waitFor(200*time.Millisecond, func() bool { return len(waiting()) >= 2 })
		}
		w := waiting()
		last := w[len(w)-1]
		freed[last] = true
		close(release[last])
	}
	got := terminal(t, svc, em, "r-cc", "eng")
	if p := eng.peakInflight(); p != 2 {
		t.Errorf("peak chunks in flight = %d, want 2", p)
	}
	if got.Result != bracketedDoc(1, n) {
		t.Errorf("Result = %q, want source order %q", got.Result, bracketedDoc(1, n))
	}
}

// A cut after a CJK sentence end has no whitespace in the source (Chinese puts none between
// sentences), so the recorded separator is empty. Joining two English translations with nothing
// between them would run them together ("today.Let's"): a target language that spaces its
// sentences gets one space at such a cut; Japanese, which does not, gets none. Split's own round
// trip is unaffected (chunk_test.go); only the joined translation differs.
func TestChunkedJoinAfterCJKCutSpacesOnlySpacedTargets(t *testing.T) {
	sentence := func(head string) string { return head + strings.Repeat("字", 58) + "。" } // 60 runes
	s1, s2 := sentence("甲"), sentence("乙")
	text := s1 + s2 // one line, 120 runes over Max 100: cut after the first 。, Sep ""
	for _, tc := range []struct {
		to   model.Language
		want string
	}{
		{model.EN, bracketed(s1) + " " + bracketed(s2)},
		{model.JA, bracketed(s1) + bracketed(s2)},
	} {
		t.Run(string(tc.to), func(t *testing.T) {
			eng := newScript("eng", nil)
			svc, em := chunkedService(t, eng)
			id := "r-cjk-" + string(tc.to)
			if _, err := svc.TranslateMulti(model.TranslateRequest{Text: text, From: model.ZH, To: tc.to, RequestID: id}); err != nil {
				t.Fatal(err)
			}
			got := terminal(t, svc, em, id, "eng")
			if n := len(eng.callList()); n != 2 {
				t.Fatalf("engine called %d times, want 2 (one per sentence)", n)
			}
			if got.Error != "" || got.Result != tc.want {
				t.Errorf("zh->%s Result = %q (Error %q), want %q", tc.to, got.Result, got.Error, tc.want)
			}
		})
	}
}
