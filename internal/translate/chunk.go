package translate

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"cnb.cool/dtapp/kai/internal/engine"
)

// The splitter behind chunked translation (issue #84). It is pure: no engine, no ctx, no I/O. The
// chunk loop (callEngineChunked) calls it for text over an engine's Budget.Max() and sends each
// chunk as its own call.

// Chunk is one piece of a split source text.
//
// The concatenation of Text+Sep over all the chunks of a Split is the input, byte for byte. No cut
// falls inside a user-perceived character, and no chunk's Text is whitespace alone; only the first
// chunk's Text can start with whitespace (the input's own leading whitespace).
type Chunk struct {
	// Text is the source text of one engine call; the chunk loop sends it without the whitespace
	// around it and puts that whitespace back around the translation.
	Text string
	// Sep is the whitespace that follows Text in the input (empty where the text has none, as
	// between two CJK sentences), restored verbatim when the translations are joined. The last
	// chunk's Sep is the input's trailing whitespace.
	Sep string
	// Part is the logical part this chunk belongs to: 0, 1, 2, ... in order. The pieces of one
	// sentence too long for the budget share a Part: it counts once in the progress Done/Total and
	// is shown only once every piece has translated.
	Part int
}

// Split cuts text into chunks for one engine: each chunk measures at most budget.Max() in the
// budget's own unit, and chunks aim at targetSize (chunkTarget), which is the aim, not the ceiling.
//
// Cuts are made at the strongest boundary that brings the chunks within reach of the target, in
// this order: a blank line, a line break, the end of a sentence (. ! ? and the CJK 。！？), the end
// of a clause (, ; : and the CJK ，；、：), a space between words, and only as the last resort
// between two user-perceived characters (never inside an emoji sequence, a flag or a base
// character with its combining marks). Consecutive pieces are packed into one chunk while they
// fit the target. A paragraph or a line over the target is cut further; a sentence is cut only
// when it is over Max itself, and then its pieces make up one Part.
//
// The input's leading whitespace rides on the first chunk without counting toward its size (it is
// never sent). Empty text gives no chunks, and whitespace-only text one chunk holding all of it.
func Split(text string, budget engine.Budget, targetSize int) []Chunk {
	body := strings.TrimLeftFunc(text, unicode.IsSpace)
	if body == "" {
		if text == "" {
			return nil
		}
		return []Chunk{{Text: text}}
	}
	sp := &splitter{
		text:    text,
		budget:  budget,
		ceiling: budget.Max(),
		target:  max(1, min(targetSize, budget.Max())),
	}
	start := len(text) - len(body)
	sp.pack(sp.units(levelParagraph, start, len(text)), levelParagraph)
	return sp.out
}

// level is how finely the splitter cuts: each level cuts the units of the one above it.
type level int

const (
	levelParagraph level = iota // at blank lines
	levelLine                   // at line breaks
	levelSentence               // after sentence ends
	levelClause                 // after clause marks
	levelWord                   // at spaces
	levelGrapheme               // between user-perceived characters
)

// span is one unit of the text: text[start:end], followed by its separator text[end:sepEnd].
type span struct{ start, end, sepEnd int }

// splitter holds one Split in progress.
type splitter struct {
	text    string
	budget  engine.Budget
	ceiling int // budget.Max(): no chunk measures more
	target  int // what chunks aim at, 1..ceiling

	out     []Chunk
	parts   int  // parts numbered so far
	holding bool // inside a sentence over the ceiling: every chunk gets part held
	held    int
}

func (sp *splitter) measure(start, end int) int { return sp.budget.Measure(sp.text[start:end]) }

// units cuts text[start:end] at level lv. The last unit's separator is the range's trailing
// whitespace (empty below the paragraph level, whose units carry no trailing whitespace).
func (sp *splitter) units(lv level, start, end int) []span {
	switch lv {
	case levelParagraph:
		return spaceCuts(sp.text, start, end, func(run string) bool { return lineBreaks(run) >= 2 })
	case levelLine:
		return spaceCuts(sp.text, start, end, func(run string) bool { return lineBreaks(run) >= 1 })
	case levelSentence:
		return markCuts(sp.text, start, end, sentenceMark, true)
	case levelClause:
		return markCuts(sp.text, start, end, clauseMark, false)
	case levelWord:
		return spaceCuts(sp.text, start, end, func(string) bool { return true })
	default:
		return graphemeCuts(sp.text, start, end)
	}
}

// tooBig reports whether a unit of level lv measuring n must be cut further. A paragraph or a line
// is cut to reach the target; anything from a sentence down is cut only when it is over the
// ceiling, so a sentence that fits Max is sent whole even when it is over the target.
func (sp *splitter) tooBig(lv level, n int) bool {
	switch lv {
	case levelParagraph, levelLine:
		return n > sp.target
	case levelGrapheme:
		return false
	default:
		return n > sp.ceiling
	}
}

// pack emits the units of one level as chunks, packing consecutive units while they fit the
// target, and cutting a unit that is too big at the next level down.
func (sp *splitter) pack(us []span, lv level) {
	open, size := -1, 0 // the first unit of the chunk being filled (-1: none) and its size
	for i, u := range us {
		n := sp.measure(u.start, u.end)
		if sp.tooBig(lv, n) {
			if open >= 0 {
				sp.emit(us[open].start, us[i-1].end, us[i-1].sepEnd)
				open = -1
			}
			sp.descend(u, lv)
			continue
		}
		if open >= 0 {
			if grown := size + sp.measure(us[i-1].end, us[i-1].sepEnd) + n; grown <= sp.target {
				size = grown
				continue
			}
			sp.emit(us[open].start, us[i-1].end, us[i-1].sepEnd)
		}
		open, size = i, n
	}
	if open >= 0 {
		last := us[len(us)-1]
		sp.emit(us[open].start, last.end, last.sepEnd)
	}
}

// descend cuts one unit of level lv at the next level down; its own separator follows its last
// piece. The pieces of a sentence over the ceiling are one part.
func (sp *splitter) descend(u span, lv level) {
	sub := sp.units(lv+1, u.start, u.end)
	sub[len(sub)-1].sepEnd = u.sepEnd
	if lv == levelSentence {
		sp.held, sp.holding = sp.parts, true
		sp.parts++
		defer func() { sp.holding = false }()
	}
	sp.pack(sub, lv+1)
}

// emit appends the chunk text[start:end] with its separator text[end:sepEnd].
func (sp *splitter) emit(start, end, sepEnd int) {
	if len(sp.out) == 0 {
		start = 0 // the input's leading whitespace rides on the first chunk
	}
	part := sp.held
	if !sp.holding {
		part = sp.parts
		sp.parts++
	}
	sp.out = append(sp.out, Chunk{Text: sp.text[start:end], Sep: sp.text[end:sepEnd], Part: part})
}

// spaceCuts cuts text[start:end] at the whitespace runs cut accepts; a run it rejects stays inside
// its unit. A run that reaches end is the last unit's separator. start must not be whitespace.
func spaceCuts(text string, start, end int, cut func(run string) bool) []span {
	var out []span
	unit := start
	for i := start; i < end; {
		r, n := utf8.DecodeRuneInString(text[i:end])
		if !unicode.IsSpace(r) {
			i += n
			continue
		}
		j := i + n
		for j < end {
			r, n := utf8.DecodeRuneInString(text[j:end])
			if !unicode.IsSpace(r) {
				break
			}
			j += n
		}
		if j == end || cut(text[i:j]) {
			out = append(out, span{unit, i, j})
			unit = j
		}
		i = j
	}
	if unit < end {
		out = append(out, span{unit, end, end})
	}
	return out
}

// lineBreaks counts the line breaks in a whitespace run: CRLF counts once, and a paragraph
// separator (U+2029) counts as a blank line.
func lineBreaks(run string) int {
	n := 0
	for i, r := range run {
		switch r {
		case '\n', '\v', '\f', '\u0085', '\u2028':
			n++
		case '\r':
			if !strings.HasPrefix(run[i+1:], "\n") {
				n++
			}
		case '\u2029':
			n += 2
		}
	}
	return n
}

// markKind is how a punctuation mark ends a sentence or a clause.
type markKind int

const (
	notMark    markKind = iota
	narrowMark          // Latin-style: ends only where whitespace follows (3.14, 10:30 stay whole)
	wideMark            // CJK-style: ends where it stands, with or without whitespace after it
)

func sentenceMark(r rune) markKind {
	switch r {
	case '.', '!', '?', '…', '؟', '۔', '।':
		return narrowMark
	case '。', '！', '？', '．', '｡':
		return wideMark
	}
	return notMark
}

func clauseMark(r rune) markKind {
	switch r {
	case ',', ';', ':', '،':
		return narrowMark
	case '，', '；', '、', '：', '､':
		return wideMark
	}
	return notMark
}

// isCloser reports a closing quote or bracket, which stays with the mark before it ("Yes." she
// said / 「はい。」).
func isCloser(r rune) bool {
	return unicode.In(r, unicode.Pe, unicode.Pf) || r == '"' || r == '\'' || r == '＂' || r == '＇'
}

// markCuts cuts text[start:end] after the marks mark recognizes; the whitespace that follows a
// mark is the separator. With sentences set, a Latin-style mark followed by a lowercase letter is
// not a sentence end ("e.g. the"). start must not be whitespace.
func markCuts(text string, start, end int, mark func(rune) markKind, sentences bool) []span {
	var out []span
	unit := start
	for i := start; i < end; {
		r, n := utf8.DecodeRuneInString(text[i:end])
		i += n
		kind := mark(r)
		if kind == notMark {
			continue
		}
		// Further marks and closing quotes or brackets stay with this one ("?!", "。」").
		for i < end {
			r, n := utf8.DecodeRuneInString(text[i:end])
			if k := mark(r); k != notMark {
				kind = max(kind, k)
			} else if !isCloser(r) {
				break
			}
			i += n
		}
		if i == end {
			break
		}
		j := i
		for j < end {
			r, n := utf8.DecodeRuneInString(text[j:end])
			if !unicode.IsSpace(r) {
				break
			}
			j += n
		}
		if j == end { // whitespace to the end: the last unit's separator
			return append(out, span{unit, i, end})
		}
		next, _ := utf8.DecodeRuneInString(text[j:end])
		switch {
		case j == i && (kind == narrowMark || isGraphemeExtend(next)):
			continue
		case j > i && sentences && kind == narrowMark && unicode.IsLower(next):
			continue
		}
		out = append(out, span{unit, i, j})
		unit, i = j, j
	}
	return append(out, span{unit, end, end})
}

// graphemeCuts cuts text[start:end] between user-perceived characters.
func graphemeCuts(text string, start, end int) []span {
	var out []span
	for i := start; i < end; {
		n := graphemeLen(text[i:end])
		out = append(out, span{i, i + n, i + n})
		i += n
	}
	return out
}

// graphemeLen is the length in bytes of the user-perceived character s starts with: a base
// character and every combining mark, variation selector, skin tone and tag after it, an emoji
// sequence joined with zero width joiners, a regional-indicator pair (a flag), a Hangul syllable
// spelled in jamo, or CRLF. It is the extended grapheme cluster of Unicode's UAX #29, simplified
// to what a cut must never break.
func graphemeLen(s string) int {
	r, n := utf8.DecodeRuneInString(s)
	if r == '\r' && strings.HasPrefix(s[n:], "\n") {
		return n + 1
	}
	flag := regionalIndicator(r) // a regional indicator waiting for its pair
	prev := r
	for n < len(s) {
		next, m := utf8.DecodeRuneInString(s[n:])
		switch {
		case isGraphemeExtend(next), prev == '\u200D':
			flag = false
		case flag && regionalIndicator(next):
			flag = false
		default:
			return n
		}
		prev = next
		n += m
	}
	return n
}

// isGraphemeExtend reports a character that belongs to the one before it.
func isGraphemeExtend(r rune) bool {
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Mc) ||
		r == '\u200C' || r == '\u200D' || // zero width non-joiner and joiner
		(r >= 0x1F3FB && r <= 0x1F3FF) || // emoji skin tone modifiers
		(r >= 0xE0020 && r <= 0xE007F) || // tags (subdivision flags)
		(r >= 0x1160 && r <= 0x11FF) || (r >= 0xD7B0 && r <= 0xD7FF) // Hangul vowel and final jamo
}

// regionalIndicator reports one of the 26 letters two of which spell a flag.
func regionalIndicator(r rune) bool { return r >= 0x1F1E6 && r <= 0x1F1FF }

// isCJK reports a Chinese, Japanese or Korean character, CJK punctuation included: the scripts
// written without spaces between sentences, and the ones Apple translates slowest per rune.
func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) ||
		(r >= 0x3000 && r <= 0x30FF) || // CJK symbols and punctuation, kana (incl. ー)
		(r >= 0xFF00 && r <= 0xFFEF) // full-width and half-width forms
}

// The chunk-size rule (issue #84; docs/engine-limits.md). Budget.Max() is the ceiling no chunk may
// cross; the size a chunk aims at is smaller, because a chunk near Max is minutes of silence on a
// slow engine and, on Apple, a cancel leaves a drain tail that grows with the chunk (#157). The
// figures below are code constants, not a Budget field: the table in input_budget.go holds limits
// only. Quality does not enter the rule: #152/#153 found no quality loss with input size up to
// Apple's measured limit (docs/quality-limits.md), so chunks are not shrunk "to be safe".

// chunkTargetPercent is the share of Max a chunk aims at on an engine with no measured latency:
// every engine but Apple (their rows are documented or provisional, with no latency curve yet).
// Half of Max keeps a chunk well under a ceiling that may itself be off, and at most doubles the
// calls against sending Max; the follow-ups that measure each engine (#87 google, #88 deepl,
// #89 baidu, #90 tencent, #91 youdao, #92 openai, #93 anthropic, #94 gemini) replace it with a
// latency-based figure, as #119 did for Apple. Provisional.
const chunkTargetPercent = 50

// Apple's targets, in runes (the unit of its row), from the latency #119 measured
// (docs/engine-limits.md, "Probe runs (#119)", chunk size for #84): about 8-9 ms per rune for
// Latin-script source text and 22-31 ms per rune for Chinese or Japanese source text, so about
// 30 s of visible progress per chunk is roughly 3,500 Latin runes or 1,200 CJK runes.
const (
	appleLatinChunkRunes = 3500
	appleCJKChunkRunes   = 1200
)

// chunkTarget is the size, in the budget's unit, the splitter aims each chunk of text at on
// engineName: always under budget.Max(), never Max itself. Apple's target comes from its measured
// latency and the text's own script (the source language may still be auto when chunks are
// sized); every other engine aims at chunkTargetPercent of its Max.
func chunkTarget(engineName string, budget engine.Budget, text string) int {
	target := budget.Max() * chunkTargetPercent / 100
	if engineName == "apple" && budget.Unit == engine.UnitRunes {
		target = min(target, appleChunkRunes(text))
	}
	return max(target, 1)
}

// appleChunkRunes applies the ~30 s rule to the text's own mix of scripts: a CJK letter costs
// 1/1,200 of a chunk's work and any other letter 1/3,500, so text all in one script gets that
// script's figure and mixed text lands between the two. Text without letters is taken as Latin.
func appleChunkRunes(text string) int {
	letters, cjk := 0, 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			letters++
			if isCJK(r) {
				cjk++
			}
		}
	}
	if letters == 0 {
		return appleLatinChunkRunes
	}
	return appleLatinChunkRunes * appleCJKChunkRunes * letters /
		(appleCJKChunkRunes*(letters-cjk) + appleLatinChunkRunes*cjk)
}
