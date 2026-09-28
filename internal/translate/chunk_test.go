package translate

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"cnb.cool/dtapp/kai/internal/engine"
)

// Issue #84 (T, RED): the pure splitter and the per-engine chunk-size rule.
//
// Names these tests pin (package translate; docs/handoffs/84-brief.md, "The splitter" and
// "Chunk-size decision rule"):
//
//	type Chunk struct {
//		Text string // the source text sent to the engine as one call
//		Sep  string // the separator that follows Text in the input, restored verbatim on reassembly
//		Part int    // the logical part this chunk belongs to: 0, 1, 2, ... in order. The pieces of
//		            // one hard-split sentence share a Part (it counts once in Done/Total and is
//		            // shown only once every piece has translated).
//	}
//	func Split(text string, budget engine.Budget, targetSize int) []Chunk
//	func chunkTarget(engineName string, budget engine.Budget, text string) int
//
// Invariant for every input: the concatenation of Text+Sep over the chunks is the input, byte for
// byte.

// budgetMax returns a budget in unit whose Max() is exactly max (Limit is found, not assumed, so
// the test does not depend on the margin arithmetic).
func budgetMax(t *testing.T, unit engine.BudgetUnit, max int) engine.Budget {
	t.Helper()
	for limit := max; limit < max*2+10; limit++ {
		b := engine.Budget{Unit: unit, Limit: limit, Source: engine.SourceProvisional, FollowUp: 84}
		if b.Max() == max {
			return b
		}
	}
	t.Fatalf("no Limit gives Max()=%d", max)
	return engine.Budget{}
}

func joinChunks(cs []Chunk) string {
	var b strings.Builder
	for _, c := range cs {
		b.WriteString(c.Text)
		b.WriteString(c.Sep)
	}
	return b.String()
}

// checkChunks asserts the invariants every Split result must hold: exact round trip, every chunk
// within Max in the budget's own unit, no chunk that would send the engine empty text, valid UTF-8
// on both sides of every cut, and Part numbering contiguous from 0.
func checkChunks(t *testing.T, input string, b engine.Budget, cs []Chunk) {
	t.Helper()
	if got := joinChunks(cs); got != input {
		t.Fatalf("round trip broken:\n got %q\nwant %q", got, input)
	}
	for i, c := range cs {
		if n := b.Measure(c.Text); n > b.Max() {
			t.Errorf("chunk %d measures %d %s, over Max %d: %q", i, n, b.Unit, b.Max(), c.Text)
		}
		if strings.TrimSpace(input) != "" && strings.TrimSpace(c.Text) == "" {
			t.Errorf("chunk %d has no text to translate (Text=%q Sep=%q)", i, c.Text, c.Sep)
		}
		if !utf8.ValidString(c.Text) || !utf8.ValidString(c.Sep) {
			t.Errorf("chunk %d cuts inside a character: Text=%q Sep=%q", i, c.Text, c.Sep)
		}
		if strings.TrimSpace(c.Sep) != "" {
			t.Errorf("chunk %d Sep %q is not whitespace: a separator carries no text to translate", i, c.Sep)
		}
	}
	for i, c := range cs {
		switch {
		case i == 0 && c.Part != 0:
			t.Errorf("first chunk Part = %d, want 0", c.Part)
		case i > 0 && c.Part != cs[i-1].Part && c.Part != cs[i-1].Part+1:
			t.Errorf("chunk %d Part = %d after %d: parts must be contiguous and in order", i, c.Part, cs[i-1].Part)
		}
	}
}

// boundary is the whitespace around the cut between chunk i and chunk i+1.
func boundary(cs []Chunk, i int) string {
	a := cs[i].Text
	trail := a[len(strings.TrimRightFunc(a, unicode.IsSpace)):]
	b := cs[i+1].Text
	lead := b[:len(b)-len(strings.TrimLeftFunc(b, unicode.IsSpace))]
	return trail + cs[i].Sep + lead
}

// words is a sentence of n Latin words from a small vocabulary, no punctuation.
var vocab = []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta"}

func wordRun(n int) string {
	w := make([]string, n)
	for i := range w {
		w[i] = vocab[i%len(vocab)]
	}
	return strings.Join(w, " ")
}

// ---- round trip --------------------------------------------------------------------------

func TestSplitRoundTripAcrossBoundaryKinds(t *testing.T) {
	cjk := strings.Repeat("今天天气很好。我们去公园吧！你想去吗？", 12)
	cases := map[string]string{
		"paragraphs":        strings.Repeat("First sentence here. Second one follows.\n\n", 12),
		"triple blank":      "Head paragraph is here.\n\n\nNext paragraph after three newlines.\n\n\n\n" + wordRun(30) + ".",
		"crlf":              strings.Repeat("Line one of the text.\r\nLine two of it.\r\n\r\n", 10),
		"lines":             strings.Repeat("A line that ends here.\n", 20),
		"sentences":         strings.Repeat("This is a sentence. Is it? It is! ", 15),
		"cjk sentences":     cjk,
		"clauses":           strings.Repeat("one clause here, another clause there; and a third: ", 10) + "end.",
		"hard split word":   strings.Repeat("x", 530),
		"hard split words":  wordRun(120),
		"lead/trail spaces": "\n\n   " + strings.Repeat("Body sentence goes on. ", 20) + "  \n\n",
		"tabs and spaces":   strings.Repeat("Cell one.\t\tCell two.   \n", 15),
	}
	b := budgetMax(t, engine.UnitRunes, 100)
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			cs := Split(in, b, 50)
			if len(cs) < 2 {
				t.Fatalf("input of %d runes over Max 100 gave %d chunk(s), want a split", utf8.RuneCountInString(in), len(cs))
			}
			checkChunks(t, in, b, cs)
		})
	}
}

// Every chunk fits Max in the budget's own unit, for all three units.
func TestSplitRespectsEveryBudgetUnit(t *testing.T) {
	in := strings.Repeat("Mixed text with Latin words. 中文句子在这里。日本語の文もあります。\n\n", 20)
	for _, unit := range []engine.BudgetUnit{engine.UnitRunes, engine.UnitUTF8Bytes, engine.UnitQueryEscapedBytes} {
		t.Run(string(unit), func(t *testing.T) {
			b := budgetMax(t, unit, 120)
			cs := Split(in, b, 80)
			if len(cs) < 2 {
				t.Fatalf("got %d chunk(s) for %d %s over Max 120", len(cs), b.Measure(in), unit)
			}
			checkChunks(t, in, b, cs)
		})
	}
}

// ---- boundary preference -----------------------------------------------------------------

// Paragraphs that each fit the target are never cut inside: every cut is at a blank line.
func TestSplitPrefersParagraphBreaks(t *testing.T) {
	para := "Short line one. Another sentence.\nSecond line here, with a clause."
	in := strings.Repeat(para+"\n\n", 8) + para
	b := budgetMax(t, engine.UnitRunes, 200)
	cs := Split(in, b, 100)
	checkChunks(t, in, b, cs)
	if len(cs) < 2 {
		t.Fatalf("got %d chunk(s), want several", len(cs))
	}
	for i := 0; i < len(cs)-1; i++ {
		if bd := boundary(cs, i); !strings.Contains(bd, "\n\n") {
			t.Errorf("cut %d is at %q, not a paragraph break, although every paragraph fits the target", i, bd)
		}
	}
}

// Inside one paragraph, lines that each fit the target are never cut inside.
func TestSplitPrefersLineBreaksOverSentences(t *testing.T) {
	line := "First sentence. Second sentence. Third one"
	in := strings.Repeat(line+"\n", 12) + line
	b := budgetMax(t, engine.UnitRunes, 200)
	cs := Split(in, b, 100)
	checkChunks(t, in, b, cs)
	for i := 0; i < len(cs)-1; i++ {
		if bd := boundary(cs, i); !strings.Contains(bd, "\n") {
			t.Errorf("cut %d is at %q, not a line break, although every line fits the target", i, bd)
		}
	}
}

// Inside one line, cuts fall after a sentence terminator, English and CJK alike.
func TestSplitCutsAtSentenceEnds(t *testing.T) {
	cases := map[string]string{
		"english":  strings.Repeat("The cat sat down. Did it purr? It did! ", 12),
		"chinese":  strings.Repeat("今天天气很好。我们去公园吧！你想去吗？", 10),
		"japanese": strings.Repeat("これはテストです。本当ですか？はい！", 10),
	}
	b := budgetMax(t, engine.UnitRunes, 60)
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			cs := Split(in, b, 40)
			checkChunks(t, in, b, cs)
			if len(cs) < 2 {
				t.Fatalf("got %d chunk(s), want a split", len(cs))
			}
			for i := 0; i < len(cs)-1; i++ {
				end := strings.TrimRightFunc(cs[i].Text, unicode.IsSpace)
				r, _ := utf8.DecodeLastRuneInString(end)
				if !strings.ContainsRune(".?!。！？", r) {
					t.Errorf("chunk %d ends with %q, not a sentence terminator: %q", i, r, cs[i].Text)
				}
			}
		})
	}
}

// A sentence longer than the target but within Max is not split at all: a hard split (below
// sentence level) is only for a sentence that itself exceeds the budget.
func TestSplitDoesNotBreakASentenceThatFitsMax(t *testing.T) {
	sentence := wordRun(14) + "." // ~80 runes, one sentence, no clause marks
	in := sentence + "\n\n" + sentence + "\n\n" + sentence
	b := budgetMax(t, engine.UnitRunes, 100)
	cs := Split(in, b, 30)
	checkChunks(t, in, b, cs)
	if len(cs) != 3 {
		t.Fatalf("got %d chunks, want 3 (one per sentence; each fits Max 100 though over target 30): %+v", len(cs), cs)
	}
	for i, c := range cs {
		if strings.TrimSpace(c.Text) != sentence {
			t.Errorf("chunk %d = %q, want the whole sentence", i, c.Text)
		}
		if c.Part != i {
			t.Errorf("chunk %d Part = %d, want %d", i, c.Part, i)
		}
	}
}

// ---- over-budget sentence ----------------------------------------------------------------

// A sentence over Max splits at clauses first, and its pieces share one Part.
func TestSplitLongSentenceAtClausesAsOnePart(t *testing.T) {
	cases := map[string]string{
		"english": strings.Repeat("the first clause runs on, ", 8) + "and it ends here.",
		"chinese": strings.Repeat("这是一个很长的分句，", 12) + "最后结束。",
	}
	b := budgetMax(t, engine.UnitRunes, 50)
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			cs := Split(in, b, 30)
			checkChunks(t, in, b, cs)
			if len(cs) < 2 {
				t.Fatalf("got %d chunk(s), want the over-budget sentence split", len(cs))
			}
			for i := 0; i < len(cs)-1; i++ {
				end := strings.TrimRightFunc(cs[i].Text, unicode.IsSpace)
				r, _ := utf8.DecodeLastRuneInString(end)
				if !strings.ContainsRune(",;:，；、：", r) {
					t.Errorf("piece %d ends with %q, not at a clause: %q", i, r, cs[i].Text)
				}
			}
			for i, c := range cs {
				if c.Part != 0 {
					t.Errorf("piece %d Part = %d, want 0: the pieces of one sentence are one part", i, c.Part)
				}
			}
		})
	}
}

// A sentence with no clause marks is hard split between words, not inside one.
func TestSplitHardSplitsBetweenWords(t *testing.T) {
	in := wordRun(80) + "."
	b := budgetMax(t, engine.UnitRunes, 50)
	cs := Split(in, b, 30)
	checkChunks(t, in, b, cs)
	if len(cs) < 2 {
		t.Fatalf("got %d chunk(s), want a hard split", len(cs))
	}
	known := map[string]bool{}
	for _, w := range vocab {
		known[w] = true
		known[w+"."] = true
	}
	for i, c := range cs {
		for _, f := range strings.Fields(c.Text) {
			if !known[f] {
				t.Errorf("piece %d holds %q, a cut word: %q", i, f, c.Text)
			}
		}
		if c.Part != 0 {
			t.Errorf("piece %d Part = %d, want 0 (one sentence)", i, c.Part)
		}
	}
}

// A hard-split sentence between two ordinary paragraphs is one part among three.
func TestSplitHardSplitSentenceIsOnePartAmongOthers(t *testing.T) {
	head := wordRun(30)[:94] + "." // 95 runes: cannot share a chunk with any piece
	long := wordRun(60)            // ~330 runes, no punctuation
	tail := wordRun(30)[:94] + "."
	in := head + "\n\n" + long + "\n\n" + tail
	b := budgetMax(t, engine.UnitRunes, 100)
	cs := Split(in, b, 60)
	checkChunks(t, in, b, cs)
	parts := map[int]bool{}
	longPart := -1
	for i, c := range cs {
		parts[c.Part] = true
		if strings.Contains(c.Text, "gamma delta") && !strings.Contains(c.Text, ".") {
			if longPart == -1 {
				longPart = c.Part
			} else if c.Part != longPart {
				t.Errorf("piece %d of the long sentence has Part %d, want %d like its other pieces", i, c.Part, longPart)
			}
		}
	}
	if len(parts) != 3 {
		t.Errorf("got %d distinct parts, want 3 (head, the long sentence, tail): %+v", len(parts), cs)
	}
	if len(cs) <= 3 {
		t.Errorf("got %d chunks, want the long sentence in several pieces", len(cs))
	}
}

// ---- graphemes -------------------------------------------------------------------------

func isExtender(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) ||
		r == 0x200D || // zero width joiner
		(r >= 0xFE00 && r <= 0xFE0F) || // variation selectors
		(r >= 0x1F3FB && r <= 0x1F3FF) // skin tone modifiers
}

func isRegionalIndicator(r rune) bool { return r >= 0x1F1E6 && r <= 0x1F1FF }

// No cut falls inside an emoji ZWJ sequence, a combining-mark sequence, a skin-tone emoji, a flag,
// or a multi-byte character, whatever the unit.
func TestSplitNeverCutsAGrapheme(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		unit  engine.BudgetUnit
		max   int
		group int // runes per grapheme, when every grapheme in the input has the same size
	}{
		{"zwj family, runes", strings.Repeat("👨‍👩‍👧‍👦", 20), engine.UnitRunes, 20, 7},
		{"zwj family, bytes", strings.Repeat("👨‍👩‍👧‍👦", 20), engine.UnitUTF8Bytes, 60, 7},
		{"combining acute", strings.Repeat("é", 60), engine.UnitRunes, 9, 2},
		{"skin tone", strings.Repeat("👍🏽", 40), engine.UnitRunes, 9, 2},
		{"flags", strings.Repeat("🇯🇵🇫🇷", 20), engine.UnitRunes, 9, 2},
		{"four-byte, query escaped", strings.Repeat("😀", 60), engine.UnitQueryEscapedBytes, 50, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := budgetMax(t, tc.unit, tc.max)
			cs := Split(tc.in, b, tc.max/2+1)
			checkChunks(t, tc.in, b, cs)
			if len(cs) < 2 {
				t.Fatalf("got %d chunk(s), want a split", len(cs))
			}
			for i, c := range cs {
				first, _ := utf8.DecodeRuneInString(c.Text)
				last, _ := utf8.DecodeLastRuneInString(c.Text)
				if isExtender(first) {
					t.Errorf("chunk %d starts with %U, inside a grapheme", i, first)
				}
				if last == 0x200D {
					t.Errorf("chunk %d ends with a zero width joiner", i)
				}
				ri := 0
				for _, r := range c.Text {
					if isRegionalIndicator(r) {
						ri++
					}
				}
				if ri%2 != 0 {
					t.Errorf("chunk %d splits a flag (%d regional indicators)", i, ri)
				}
				if n := utf8.RuneCountInString(c.Text); n%tc.group != 0 {
					t.Errorf("chunk %d has %d runes, not whole graphemes of %d", i, n, tc.group)
				}
			}
		})
	}
}

// ---- edges and target -----------------------------------------------------------------

func TestSplitEmptyAndWhitespaceOnly(t *testing.T) {
	b := budgetMax(t, engine.UnitRunes, 100)
	for _, in := range []string{"", "   ", "\n\n\t \n"} {
		cs := Split(in, b, 50)
		if len(cs) > 1 {
			t.Errorf("Split(%q) gave %d chunks, want at most 1", in, len(cs))
		}
		if got := joinChunks(cs); got != in {
			t.Errorf("Split(%q) round trip = %q", in, got)
		}
	}
}

// Chunks aim near the target, not near Max: with many short sentences, every chunk but the last
// is between half the target and the target plus one sentence, and none crosses Max.
func TestSplitAimsNearTarget(t *testing.T) {
	sentence := "Ten runes." // 10 runes
	in := strings.TrimSpace(strings.Repeat(sentence+" ", 200))
	b := budgetMax(t, engine.UnitRunes, 400)
	const target = 100
	cs := Split(in, b, target)
	checkChunks(t, in, b, cs)
	if len(cs) < 15 {
		t.Fatalf("got %d chunks for %d runes at target %d, want about %d (chunks sized to the target, not Max)",
			len(cs), utf8.RuneCountInString(in), target, utf8.RuneCountInString(in)/target)
	}
	for i, c := range cs[:len(cs)-1] {
		n := b.Measure(strings.TrimSpace(c.Text))
		if n < target/2 || n > target+11 {
			t.Errorf("chunk %d measures %d, want near target %d", i, n, target)
		}
	}
}

// ---- chunk-size rule -------------------------------------------------------------------

// No engine's chunk target is Max itself: it is always strictly under the ceiling and positive.
func TestChunkTargetIsBelowMaxForEveryEngine(t *testing.T) {
	latin := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 2000)
	cjk := strings.Repeat("今天天气很好。", 5000)
	n := 0
	for _, m := range engine.KnownEngines() {
		if m.Kind != engine.KindTranslator {
			continue
		}
		b, ok := engine.InputBudget(m.Name)
		if !ok {
			continue
		}
		n++
		for label, text := range map[string]string{"latin": latin, "cjk": cjk} {
			got := chunkTarget(m.Name, b, text)
			if got <= 0 || got >= b.Max() {
				t.Errorf("chunkTarget(%s, %s) = %d, want 0 < target < Max %d", m.Name, label, got, b.Max())
			}
		}
	}
	if n == 0 {
		t.Fatal("no translator budgets found")
	}
	fake := budgetMax(t, engine.UnitRunes, 1000)
	if got := chunkTarget("some-new-engine", fake, latin); got <= 0 || got >= 1000 {
		t.Errorf("chunkTarget for an engine with no latency data = %d, want 0 < target < Max 1000", got)
	}
}

// Apple's target comes from the measured latency (docs/engine-limits.md, #119): about 3,500 runes
// for Latin-script text, about 1,200 for CJK/Japanese, chosen from the text's own script.
func TestChunkTargetAppleByScript(t *testing.T) {
	b, ok := engine.InputBudget("apple")
	if !ok {
		t.Fatal("apple has no budget row")
	}
	cases := []struct {
		name     string
		text     string
		min, max int
	}{
		{"latin", strings.Repeat("The quick brown fox jumps over the lazy dog. ", 2000), 3000, 4000},
		{"spanish", strings.Repeat("El rápido zorro marrón salta sobre el perro. ", 2000), 3000, 4000},
		{"chinese", strings.Repeat("今天天气很好。", 5000), 1000, 1500},
		{"japanese", strings.Repeat("これはテストの文章です。", 3000), 1000, 1500},
	}
	for _, tc := range cases {
		if got := chunkTarget("apple", b, tc.text); got < tc.min || got > tc.max {
			t.Errorf("chunkTarget(apple, %s) = %d, want %d..%d", tc.name, got, tc.min, tc.max)
		}
	}
}
