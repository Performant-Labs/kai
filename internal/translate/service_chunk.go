package translate

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"unicode"
	"unicode/utf8"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// Chunked translation (issue #84): the per-engine loop behind translateWithEngine that sends text
// over an engine's budget as several calls and reassembles the answers. The splitter is chunk.go.

// maxChunksInFlight is how many chunks of one engine's request run at once: small and bounded, to
// respect the providers' rate limits. The window is counted from the oldest chunk that has not
// answered, so the answers are joined in source order and at most one waits for an earlier chunk.
const maxChunksInFlight = 2

// errChunkFailed is the cause that ends the chunks still in flight when a sibling failed. It ends
// only the chunk loop's own child ctx: how the engine ended is still decided from the engine's
// ctx (outcomeOf), so this is never taken for the user's Cancel.
var errChunkFailed = errors.New("translation stopped: another part of the text failed")

// chunkedError is the error of a chunked translation that stopped before every chunk translated:
// cancelled (by the user, or by a newer request) or failed. It carries how far the translation got
// through translateWithEngine, which returns it unchanged, to outcomeOf, which copies the facts
// onto the engineOutcome the payloads are built from. Its text is Err's and it unwraps to Err, so
// errors.Is / errors.As (ClassifyEngineError, engine.DetectedSourceOf) see the real cause.
type chunkedError struct {
	Prefix string // the parts translated from part 1 up to the first unfinished one, joined with their separators
	Done   int    // the parts Prefix holds
	Total  int    // the parts the text was split into
	Err    error  // the failing chunk's error, or the cause that ended the engine's ctx
}

func (e *chunkedError) Error() string { return e.Err.Error() }
func (e *chunkedError) Unwrap() error { return e.Err }

// callEngineChunked is callEngine for text of any length, the one call translateWithEngine makes.
// Text within the engine's Budget.Max(), and any text for an engine without a budget row, is
// exactly one callEngine call with the request unchanged, as before chunking existed. Longer text
// is cut by Split, aimed at chunkTarget, and each chunk is one callEngine call:
//
//   - On an auto-source request chunk 1 is sent alone. When the language it detects (bare, from
//     its answer or its error) covers the target, nothing more is sent, and translateWithEngine
//     turns that detection into the identity result for the whole text, as it does unchunked
//     (#80). Otherwise a detection model.ParseLanguage recognizes is pinned as From for every later
//     chunk, so the source language is decided once per request; a code it does not recognize (an
//     engine's native code) is not pinned, and the later chunks keep the request's own From.
//   - At most maxChunksInFlight chunks run at once, under a child of ctx. progress(done, total)
//     reports each part as its last chunk answers; the pieces of a sentence cut below sentence
//     level are one part.
//   - ctx is checked before each chunk is sent: after a cancel nothing more goes out, the chunks
//     in flight end with ctx, and the error is a *chunkedError holding the translated prefix.
//   - A chunk failure sends nothing more and ends the sibling still in flight; the error is a
//     *chunkedError around that chunk's error.
//
// The result has the shape callEngine returns: From as the engine reported it for chunk 1 (bare),
// Text the whole input, Result the answers joined in source order with the recorded separators,
// and no Phonetic or Dict, which do not combine across chunks. A nil progress reports nothing.
func (s *Service) callEngineChunked(ctx context.Context, reg engine.Translator, engineName string, req model.TranslateRequest, progress func(done, total int)) (*model.TranslateResult, error) {
	budget, ok := s.budgetOf(engineName)
	if !ok || budget.Fits(req.Text) {
		return s.callEngine(ctx, reg, engineName, req)
	}
	target := chunkTarget(engineName, budget, req.Text)
	c := newChunkedCall(Split(req.Text, budget, target), req.To)
	if len(c.chunks) < 2 {
		return s.callEngine(ctx, reg, engineName, req)
	}
	slog.Debug(i18n.T("log.translate_chunked",
		"Engine", engineName, "Chunks", len(c.chunks), "Parts", c.parts, "Target", target, "Unit", budget.Unit))

	cctx, stop := context.WithCancelCause(ctx)
	defer stop(nil)
	type answer struct {
		i   int
		res *model.TranslateResult
		err error
	}
	answers := make(chan answer, len(c.chunks))
	from := req.From
	send := func(i int) {
		creq := req
		creq.From = from
		creq.Text = strings.TrimSpace(c.chunks[i].Text)
		go func() {
			res, err := s.callEngine(cctx, reg, engineName, creq)
			answers <- answer{i, res, err}
		}()
	}
	var failed error
	record := func(i int, res *model.TranslateResult) {
		if c.answer(i, res) && failed == nil && progress != nil {
			progress(c.finished, c.parts)
		}
	}

	next := 0
	if isAutoSource(req.From) {
		send(0)
		next = 1
		a := <-answers
		if ctx.Err() == nil {
			if d, ok := detectedSource(a.res, a.err); ok && d.Covers(req.To) {
				if a.err != nil {
					return nil, a.err
				}
				// The text is already in the target language: translateWithEngine builds the identity
				// result from this detection. The answer is the source text itself, so that should a
				// cancel land before that check (an answer then stands as a translation), the
				// request still answers with the whole text, not with chunk 1 alone.
				return &model.TranslateResult{Engine: engineName, From: d, To: req.To, Text: req.Text, Result: req.Text}, nil
			}
		}
		if a.err != nil {
			return nil, c.stopped(ctx, a.err)
		}
		record(0, a.res)
		if l, ok := model.ParseLanguage(string(a.res.From)); ok {
			from = l
		}
	}

	for inflight := 0; ; {
		for failed == nil && ctx.Err() == nil && next < len(c.chunks) && next < c.open+maxChunksInFlight {
			send(next)
			next++
			inflight++
		}
		if inflight == 0 {
			break
		}
		a := <-answers
		inflight--
		switch {
		case a.err == nil:
			record(a.i, a.res)
		case failed == nil && ctx.Err() == nil:
			failed = a.err
			stop(errChunkFailed)
		}
	}
	if ctx.Err() != nil || failed != nil {
		return nil, c.stopped(ctx, failed)
	}
	return &model.TranslateResult{
		Engine: engineName,
		From:   c.from,
		To:     req.To,
		Text:   req.Text,
		Result: c.join(len(c.chunks), true),
	}, nil
}

// chunkedCall is one engine's chunked translation of one request: the chunks, their answers and
// the parts those complete. Only callEngineChunked's goroutine uses it.
type chunkedCall struct {
	chunks   []Chunk
	to       model.Language
	from     model.Language // the source language the engine reported for chunk 1
	answers  []string       // each chunk's translation, trimmed
	answered []bool
	parts    int // the parts in all
	finished int // the parts whose every chunk has answered
	open     int // the first chunk that has not answered
}

func newChunkedCall(chunks []Chunk, to model.Language) *chunkedCall {
	c := &chunkedCall{chunks: chunks, to: to, answers: make([]string, len(chunks)), answered: make([]bool, len(chunks))}
	if len(chunks) > 0 {
		c.parts = chunks[len(chunks)-1].Part + 1
	}
	return c
}

// answer records chunk i's answer and reports whether it completed its part.
func (c *chunkedCall) answer(i int, res *model.TranslateResult) bool {
	if i == 0 {
		c.from = res.From
	}
	c.answers[i] = strings.TrimSpace(res.Result)
	c.answered[i] = true
	for c.open < len(c.chunks) && c.answered[c.open] {
		c.open++
	}
	p := c.chunks[i].Part
	for j := i - 1; j >= 0 && c.chunks[j].Part == p; j-- {
		if !c.answered[j] {
			return false
		}
	}
	for j := i + 1; j < len(c.chunks) && c.chunks[j].Part == p; j++ {
		if !c.answered[j] {
			return false
		}
	}
	c.finished++
	return true
}

// prefix is the run of whole parts from part 1 whose every chunk has answered: the number of
// chunks it spans and of parts it holds. It is what a cancel shows and what a failure names, never
// a later part that finished out of order, which would leave a gap.
func (c *chunkedCall) prefix() (chunks, parts int) {
	if c.open == len(c.chunks) {
		return c.open, c.parts
	}
	end, p := c.open, c.chunks[c.open].Part
	for end > 0 && c.chunks[end-1].Part == p {
		end--
	}
	return end, p
}

// join reassembles the answers of chunks [0, end) in source order: each with the whitespace its
// source chunk had around it, and with the separators between them (sepAfter). With all set, the
// last chunk's own separator, the input's trailing whitespace, follows too.
func (c *chunkedCall) join(end int, all bool) string {
	var b strings.Builder
	for i := 0; i < end; i++ {
		t := c.chunks[i].Text
		b.WriteString(t[:len(t)-len(strings.TrimLeftFunc(t, unicode.IsSpace))])
		b.WriteString(c.answers[i])
		b.WriteString(t[len(strings.TrimRightFunc(t, unicode.IsSpace)):])
		if i < end-1 || all {
			b.WriteString(c.sepAfter(i))
		}
	}
	return b.String()
}

// sepAfter is the separator written after chunk i's answer: the one recorded in the source, except
// where the source had none because its script puts no space between sentences (a cut after 。 in
// Chinese or Japanese) and the target language does. There a space keeps two translated sentences
// from running together ("today.Let's").
func (c *chunkedCall) sepAfter(i int) string {
	sep := c.chunks[i].Sep
	if sep != "" || i == len(c.chunks)-1 || !spacedLanguage(c.to) {
		return sep
	}
	if r, _ := utf8.DecodeLastRuneInString(c.chunks[i].Text); isCJK(r) {
		return " "
	}
	return sep
}

// spacedLanguage reports whether text in l puts spaces between words and sentences: every language
// but Chinese and Japanese.
func spacedLanguage(l model.Language) bool {
	primary := strings.ToLower(string(l))
	if i := strings.IndexAny(primary, "-_"); i >= 0 {
		primary = primary[:i]
	}
	return primary != "zh" && primary != "ja"
}

// stopped is the error of a chunked translation that ends early: the cause that ended ctx after a
// cancel, else the failing chunk's error, with the contiguous prefix translated so far.
func (c *chunkedCall) stopped(ctx context.Context, failed error) *chunkedError {
	err := failed
	if ctx.Err() != nil {
		err = context.Cause(ctx)
	}
	end, done := c.prefix()
	return &chunkedError{Prefix: c.join(end, false), Done: done, Total: c.parts, Err: err}
}
