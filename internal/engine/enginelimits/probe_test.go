//go:build enginelimits && darwin

package enginelimits

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"cnb.cool/dtapp/kai/internal/engine"
	"cnb.cool/dtapp/kai/internal/model"
	"cnb.cool/dtapp/kai/internal/translate"
	"cnb.cool/dtapp/kai/pkg/swiftbridge"
	"github.com/ebitengine/purego"
	"golang.org/x/sys/unix"
)

// Apple input-limit probe (issue #83, revised by #111). Opt-in: build tag enginelimits (darwin) AND
// KAI_ENGINE_PROBE=1. See doc.go for the exact command. The probe goes through the same code the
// app uses (engine.NewApple() behind the engine.Translator interface), never through
// swiftbridge.KaiTranslate directly, so what it measures includes TrimSpace, the language-code
// registry, the 64 KiB output buffer and the JSON decode.
//
// The bridge no longer has a wait of its own (#111), so a request is never cut off and the time a
// request takes is not a limit. What a point can fail on is what the engine itself reports: an
// error (a framework failure, or the 64 KiB output buffer, which shows up as a JSON parse failure),
// a paragraph number missing from the output, or an output implausibly small for its input.

const gateEnv = "KAI_ENGINE_PROBE"

// maxRunesEnv overrides the upper bound of the search (in runes), for a shorter run.
const maxRunesEnv = "KAI_ENGINE_PROBE_MAX_RUNES"

// The search runs from lowRunes up to the upper bound (defaultHighRunes, or maxRunesEnv). 20,000 is
// the epic's target document; time now grows linearly with size (about 9 ms per Latin rune and 26
// per CJK rune on the dev host), so the doubling search costs minutes, and a bound beyond what the
// epic needs would only make the run longer.
//
// The bisection stops at 5% of the low end, not the 1% of the first probe (#83). With no wait to
// cut a point short, every point costs its full translation time even when it fails (about 10
// minutes for 16,000 CJK runes), a CJK limit near 18,000 needs several bisection points, and the
// budget applies its own 20% margin, so a bracket of 5% is well inside it.
const (
	lowRunes         = 100    // lower bound of the search
	defaultHighRunes = 20_000 // upper bound unless maxRunesEnv says otherwise
	stepFloor        = 50     // the search stops when hi-lo <= max(stepFloor, stepPct% of lo)
	stepPct          = 5

	minOutPct    = 25 // a pass needs output runes >= 25% of input runes (coarse partial-result guard)
	minTailRunes = 40 // the final paragraph never ends up a stub shorter than this

	// The probe's own ceilings, not the product's: the bridge has no limit of its own, so a
	// framework that never answers (no main run loop, a hung service) would hang the run. Hitting
	// one stops the probe with the likely cause and is never recorded as a limit. It goes through
	// the engine's cancel path, which is exercised the same way a user's Cancel is.
	pointCeiling    = 20 * time.Minute // one search point
	precheckCeiling = 2 * time.Minute  // one one-sentence request
)

// highRunes is the upper bound of the search, set once by TestProbeApple.
var highRunes = defaultHighRunes

// prereqHelp names everything a probe run needs. A run without them fails in ways that look like
// engine limits (or, since the bridge has no wait, never ends), so every prerequisite failure stops
// the probe with this text instead.
const prereqHelp = "The probe needs: (1) external linking with cgo (CGO_ENABLED=1 go test ... -ldflags=-linkmode=external); " +
	"(2) the main-thread TestMain in this file, which parks the main OS thread in dispatch_main(); " +
	"(3) the Swift bridge built first ((cd pkg/swiftbridge/scripts && bash ./build.sh)); " +
	"(4) English and Chinese (Simplified) installed under System Settings > General > Language & Region > Translation Languages."

// init runs on the main OS thread. Locking it here makes the generated test main, and so
// TestMain, run on that thread, which is the thread dispatch_main must be called on.
func init() { runtime.LockOSThread() }

// TestMain parks the main OS thread in dispatch_main() so the main dispatch queue is serviced.
// Translation.framework needs that to answer: without it the installed-language list comes back
// empty after 30 s and a translation never returns (before #111 the bridge's 20 s wait gave up on
// it with an empty result). The tests run on another goroutine. It is harmless when the probe is
// skipped: m.Run() returns at once and the process exits.
func TestMain(m *testing.M) {
	// The engine logs the raw (up to 64 KB) payload on a parse failure; that is noise here. The
	// probe records the returned error text instead.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))
	lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	var dispatchMain func()
	purego.RegisterLibFunc(&dispatchMain, lib, "dispatch_main")
	go func() { os.Exit(m.Run()) }()
	dispatchMain() // never returns
}

// script is one search: a language pair plus the fixed prose its input is built from.
type script struct {
	name       string         // log key: latin | cjk
	from, to   model.Language // requests use the Kai-side codes; the engine registry maps them
	fromCode   string         // framework codes, for the log only
	toCode     string
	sentences  []string // no digit anywhere: the digits in the input are the paragraph numbers
	join       string   // between the sentences of one paragraph
	perPara    int      // sentences per paragraph
	terminator rune     // ends every paragraph
	warm       string   // one sentence for the pre-check (and the cancel checks in cancel_test.go)
}

var latin = script{
	name: "latin", from: model.EN, to: model.ZH, fromCode: "en", toCode: "zh-Hans",
	join: " ", perPara: 2, terminator: '.',
	warm: "The weather is nice today, so we will walk to the market.",
	sentences: []string{
		"The committee reviewed the annual report and approved the budget for the coming quarter without any objections.",
		"Every morning the baker opens the shop before sunrise and arranges fresh bread on the wooden shelves near the window.",
		"Researchers measured the temperature of the river at several locations and recorded the results in a shared notebook.",
		"A careful reader will notice that the story changes its tone as soon as the travelers leave the harbor behind them.",
		"The new library offers quiet study rooms, free access to newspapers from many countries, and a small cafe on the ground floor.",
		"Engineers tested the bridge under heavy rain and strong wind, and they were pleased that the structure held steady throughout.",
		"During the long winter the villagers gathered in the hall to share stories, mend their nets, and plan the spring planting.",
		"She explained that the software update would improve battery life, fix several reported problems, and simplify the settings menu.",
		"The museum will display paintings, coins, and tools that were discovered by farmers while they were plowing the northern fields.",
		"Please remember to lock the door, turn off the lights, and leave the keys with the neighbor before you go on vacation.",
		"Although the road was narrow and steep, the drivers were patient and allowed the delivery truck to pass safely.",
		"The teacher asked the students to describe their favorite season and to explain why it makes them feel happy and calm.",
	},
}

var cjk = script{
	name: "cjk", from: model.ZH, to: model.EN, fromCode: "zh-Hans", toCode: "en",
	join: "", perPara: 4, terminator: '。',
	warm: "今天天气很好，所以我们打算步行去市场。",
	sentences: []string{
		"委员会审查了年度报告，并且在没有任何反对意见的情况下批准了下个季度的预算。",
		"每天清晨，面包师在日出之前打开店门，把新鲜出炉的面包整齐地摆放在靠窗的木架上。",
		"研究人员在河流的多个地点测量了水温，并把结果记录在共享的笔记本里。",
		"细心的读者会发现，当旅行者离开港口之后，故事的语气就发生了变化。",
		"新图书馆提供安静的自习室、免费阅读各国报纸的服务，以及位于底层的小咖啡馆。",
		"工程师们在大雨和强风中测试了这座桥梁，令他们满意的是整个结构始终保持稳定。",
		"漫长的冬天里，村民们聚集在大厅里分享故事、修补渔网，并商量春天的播种计划。",
		"她解释说，这次软件更新将延长电池寿命，修复若干已报告的问题，并简化设置菜单。",
		"博物馆将展出农民在北面田地耕作时发现的绘画、硬币和工具。",
		"出门度假之前，请记得锁好房门、关掉电灯，并把钥匙交给邻居保管。",
		"虽然道路狭窄而陡峭，但司机们都很有耐心，让送货卡车安全地通过了。",
		"老师请学生们描述自己最喜欢的季节，并解释为什么这个季节让他们感到快乐和平静。",
	},
}

// paragraph returns perPara consecutive sentences from the cyclic pool, starting at cursor.
func (s script) paragraph(cursor int) []rune {
	parts := make([]string, s.perPara)
	for i := range parts {
		parts[i] = s.sentences[(cursor+i)%len(s.sentences)]
	}
	return []rune(strings.Join(parts, s.join))
}

// stream returns exactly n runes of the cyclic sentence stream starting at cursor.
func (s script) stream(cursor, n int) []rune {
	out := make([]rune, 0, n+len(s.sentences[0]))
	for i := cursor; len(out) < n; i++ {
		if i > cursor {
			out = append(out, []rune(s.join)...)
		}
		out = append(out, []rune(s.sentences[i%len(s.sentences)])...)
	}
	return out[:n]
}

// buildInput returns the probe text for n runes and its paragraph count. The text is numbered
// paragraphs ("17. ...") separated by a blank line, exactly n runes long, deterministic for a given
// n, and ends on the script's sentence terminator. The final paragraph absorbs the remainder, so
// the text never ends in a stub. It has no leading or trailing space, so the engine's TrimSpace
// leaves the length alone.
func (s script) buildInput(n int) (string, int) {
	var out []rune
	cursor := 0
	for para := 1; ; para++ {
		head := fmt.Sprintf("%d. ", para)
		if para > 1 {
			head = "\n\n" + head
		}
		body := s.paragraph(cursor)
		left := n - len(out) - utf8.RuneCountInString(head)
		if left-len(body) < minTailRunes {
			// This is the last paragraph: fill exactly the runes that remain.
			body = s.stream(cursor, left)
			body[len(body)-1] = s.terminator
			out = append(out, []rune(head)...)
			out = append(out, body...)
			return string(out), para
		}
		out = append(out, []rune(head)...)
		out = append(out, body...)
		cursor += s.perPara
	}
}

// firstMissingMarker returns the first paragraph number 1..paragraphs that does not appear, in
// order, among the digit runs of out; 0 means every number survived. Extra digit runs are
// tolerated (the input has none, but a translation may add some).
func firstMissingMarker(out string, paragraphs int) int {
	want := 1
	cur, in := 0, false
	flush := func() {
		if in && want <= paragraphs && cur == want {
			want++
		}
		cur, in = 0, false
	}
	for _, r := range out {
		if r >= '0' && r <= '9' {
			// Clamp so an absurdly long digit run cannot wrap around into a false match.
			cur, in = min(cur*10+int(r-'0'), 1<<30), true
			continue
		}
		flush()
	}
	flush()
	if want > paragraphs {
		return 0
	}
	return want
}

const (
	kindPass      = "pass"
	kindError     = "error"     // Translate returned an error
	kindTruncated = "truncated" // no error, but a paragraph number is missing from the output
	kindShort     = "short"     // no error and every number present, but the output is implausibly small
)

// outcome is the record of one probe point.
type outcome struct {
	n, runes, bytes, paragraphs int
	kind                        string
	err, class                  string // error text and translate.ClassifyEngineError kind; empty for non-errors
	missing                     int    // first missing paragraph number, for kind truncated
	latency                     time.Duration
	outRunes, outBytes          int
}

func (o outcome) ok() bool { return o.kind == kindPass }

// prober drives searches through the engine.Translator interface.
type prober struct {
	t  *testing.T
	tr engine.Translator
}

// call translates text with the script's pair and times the call, under the probe's own ceiling.
// Reaching the ceiling stops the run: the engine has no limit of its own, so a call that long means
// the framework is not answering, and that is a broken prerequisite, not a size limit.
func (p *prober) call(s script, text string, ceiling time.Duration) (res *model.TranslateResult, latency time.Duration, err error) {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), ceiling)
	defer cancel()
	start := time.Now()
	res, err = p.tr.Translate(ctx, model.TranslateRequest{Text: text, From: s.from, To: s.to})
	latency = time.Since(start)
	if errors.Is(err, context.DeadlineExceeded) {
		p.t.Fatalf("probe stopped: a %d-byte %s>%s request had no answer after %s. The engine imposes no limit of its own, so the framework is not answering: most likely the main run loop is not running (external linking and the TestMain in this file), or the Translation service is hung. %s",
			len(text), s.fromCode, s.toCode, ceiling, prereqHelp)
	}
	return res, latency, err
}

// try runs one probe point of n runes and logs it.
func (p *prober) try(s script, n int) outcome {
	p.t.Helper()
	text, paragraphs := s.buildInput(n)
	o := outcome{n: n, runes: utf8.RuneCountInString(text), bytes: len(text), paragraphs: paragraphs}
	if o.runes != n {
		p.t.Fatalf("probe bug: buildInput(%d) produced %d runes", n, o.runes)
	}
	res, latency, err := p.call(s, text, pointCeiling)
	o.latency = latency
	switch {
	case err != nil:
		o.kind, o.err, o.class = kindError, err.Error(), translate.ClassifyEngineError(err)
	default:
		o.outRunes, o.outBytes = utf8.RuneCountInString(res.Result), len(res.Result)
		if o.missing = firstMissingMarker(res.Result, paragraphs); o.missing != 0 {
			o.kind = kindTruncated
		} else if o.outRunes*100 < o.runes*minOutPct {
			o.kind = kindShort
		} else {
			o.kind = kindPass
		}
	}
	p.t.Logf("PROBE point search=%s n=%d runes=%d utf8_bytes=%d paragraphs=%d kind=%s latency_ms=%d out_runes=%d out_bytes=%d missing_marker=%d class=%s error=%q",
		s.name, n, o.runes, o.bytes, o.paragraphs, o.kind, o.latency.Milliseconds(), o.outRunes, o.outBytes, o.missing, dash(o.class), o.err)
	return o
}

// result is one finished search.
type result struct {
	s       script
	maxPass outcome // largest passing point below minFail
	minFail outcome // smallest failing point; zero when capped
	capped  bool    // highRunes itself passed
	first   time.Duration
}

// search brackets the limit: it doubles from lowRunes until a point fails (or highRunes passes),
// then bisects the last passing and first failing sizes down to max(stepFloor, stepPct% of the low
// end) runes. Doubling from below, rather than testing highRunes first, keeps the cost of the
// failing points small: the first failing size is at most twice the limit, and a point costs its
// full translation time even when it fails (the engine has no early exit). It assumes failure is
// monotonic in size; every point is logged, so a non-monotonic result is visible.
func (p *prober) search(s script, first time.Duration) result {
	p.t.Helper()
	r := result{s: s, first: first}
	r.maxPass = p.try(s, lowRunes)
	if !r.maxPass.ok() {
		p.t.Fatalf("search %s: the lower bound of %d runes already fails (kind=%s, error=%q); the probe cannot bracket a limit", s.name, lowRunes, r.maxPass.kind, r.maxPass.err)
	}
	for r.maxPass.n < highRunes {
		next := p.try(s, min(r.maxPass.n*2, highRunes))
		if !next.ok() {
			r.minFail = next
			break
		}
		r.maxPass = next
	}
	if r.minFail.n == 0 {
		r.capped = true
		return r
	}
	for r.minFail.n-r.maxPass.n > max(stepFloor, r.maxPass.n*stepPct/100) {
		mid := p.try(s, r.maxPass.n+(r.minFail.n-r.maxPass.n)/2)
		if mid.ok() {
			r.maxPass = mid
		} else {
			r.minFail = mid
		}
	}
	return r
}

func (r result) log(t *testing.T) {
	t.Helper()
	minFail, failKind, failClass, failLatency, failErr := "none", "-", "-", int64(0), ""
	if !r.capped {
		minFail, failKind, failClass = strconv.Itoa(r.minFail.n), r.minFail.kind, dash(r.minFail.class)
		failLatency, failErr = r.minFail.latency.Milliseconds(), r.minFail.err
	}
	t.Logf("PROBE result search=%s from=%s to=%s max_pass_runes=%d min_fail_runes=%s capped=%t latency_at_max_ms=%d first_call_ms=%d fail_kind=%s fail_class=%s fail_latency_ms=%d fail_error=%q",
		r.s.name, r.s.fromCode, r.s.toCode, r.maxPass.n, minFail, r.capped, r.maxPass.latency.Milliseconds(), r.first.Milliseconds(), failKind, failClass, failLatency, failErr)
}

// precheck stops the probe with the cause unless the prerequisites hold, and returns the latency
// of the first call of each pair (that call includes prepareTranslation).
func (p *prober) precheck() map[string]time.Duration {
	t := p.t
	t.Helper()
	if err := swiftbridge.Init(""); err != nil || !swiftbridge.Available() {
		t.Fatalf("probe prerequisite failed: the Swift bridge did not load (init error: %v). %s", err, prereqHelp)
	}
	langs, err := engine.AvailableLanguages()
	if err != nil {
		t.Fatalf("probe prerequisite failed: listing the installed languages failed: %v. %s", err, prereqHelp)
	}
	hasEN, hasZH := hasLang(langs, "en"), hasLang(langs, "zh-Hans")
	t.Logf("PROBE precheck languages=%d en=%t zh_hans=%t", len(langs), hasEN, hasZH)
	if !hasEN || !hasZH {
		t.Fatalf("probe prerequisite failed: installed languages %v must include English (en) and Chinese Simplified (zh-Hans). %s", langs, prereqHelp)
	}
	first := map[string]time.Duration{}
	for _, s := range []script{latin, cjk} {
		res, d, err := p.call(s, s.warm, precheckCeiling)
		if err != nil || res.Result == "" {
			t.Fatalf("probe prerequisite failed: a one-sentence %s>%s translation failed after %s (error: %v). %s", s.fromCode, s.toCode, d, err, prereqHelp)
		}
		first[s.name] = d
		t.Logf("PROBE precheck pair=%s>%s first_call_ms=%d ok=true", s.fromCode, s.toCode, d.Milliseconds())
	}
	return first
}

// hasLang reports whether an installed language identifier is code or a longer BCP-47 form of it
// (the framework lists full identifiers such as en-Latn-US and zh-Hans-CN).
func hasLang(installed []string, code string) bool {
	for _, l := range installed {
		if l == code || strings.HasPrefix(l, code+"-") {
			return true
		}
	}
	return false
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// bridgeStamp identifies the bridge dylib the run loaded (dev builds load the one in the source
// tree), so a log can be tied to the build.sh run that produced it.
func bridgeStamp() string {
	p, err := filepath.Abs("../../../pkg/swiftbridge/libkai_bridge.dylib")
	if err != nil {
		return "unknown"
	}
	st, err := os.Stat(p)
	if err != nil {
		return "unknown"
	}
	return fmt.Sprintf("size=%d mtime=%s", st.Size(), st.ModTime().UTC().Format(time.RFC3339))
}

func logHost(t *testing.T) {
	t.Helper()
	ver, _ := unix.Sysctl("kern.osproductversion")
	build, _ := unix.Sysctl("kern.osversion")
	chip, _ := unix.Sysctl("machdep.cpu.brand_string")
	mem, _ := unix.SysctlUint64("hw.memsize")
	t.Logf("PROBE host macos=%s build=%s chip=%q cpus=%d mem_gb=%d go=%s goarch=%s bridge=%q",
		ver, build, chip, runtime.NumCPU(), mem>>30, runtime.Version(), runtime.GOARCH, bridgeStamp())
	t.Logf("PROBE config low=%d high=%d step_floor=%d step_pct=%d min_out_pct=%d point_ceiling_s=%d precheck_ceiling_s=%d",
		lowRunes, highRunes, stepFloor, stepPct, minOutPct, int(pointCeiling.Seconds()), int(precheckCeiling.Seconds()))
}

func TestProbeApple(t *testing.T) {
	if os.Getenv(gateEnv) != "1" {
		t.Skip("set " + gateEnv + "=1 to run the Apple input-limit probe (the full command is in doc.go)")
	}
	started := time.Now()
	if v := os.Getenv(maxRunesEnv); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 2*lowRunes {
			t.Fatalf("%s=%q: want a whole number of at least %d runes", maxRunesEnv, v, 2*lowRunes)
		}
		highRunes = n
	}
	for _, s := range []script{latin, cjk} {
		for _, sentence := range append([]string{s.warm}, s.sentences...) {
			if strings.ContainsAny(sentence, "0123456789") {
				t.Fatalf("probe bug: %s sentence %q contains a digit; digits are reserved for the paragraph numbers", s.name, sentence)
			}
		}
	}
	logHost(t)
	p := &prober{t: t, tr: engine.NewApple()}
	first := p.precheck()

	lat := p.search(latin, first[latin.name])
	lat.log(t)
	cj := p.search(cjk, first[cjk.name])
	cj.log(t)

	limit, basis := lat.maxPass.n, lat.s.name
	if cj.maxPass.n < limit {
		limit, basis = cj.maxPass.n, cj.s.name
	}
	if lat.maxPass.n == cj.maxPass.n {
		basis = "both"
	}
	t.Logf("PROBE apple_limit_runes=%d basis=%s capped=%t elapsed_s=%d", limit, basis, lat.capped && cj.capped, int(time.Since(started).Seconds()))
}
