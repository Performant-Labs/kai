//go:build enginelimits && darwin

package enginelimits

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

// Apple input-limit probe (issue #83, revised by #111 and #119). Opt-in: build tag enginelimits
// (darwin) AND KAI_ENGINE_PROBE=1. See doc.go for the exact command and the knobs. The probe goes
// through the same code the app uses (engine.NewApple() behind the engine.Translator interface),
// never through swiftbridge.KaiTranslate directly, so what it measures includes TrimSpace, the
// language-code registry, the 64 KiB output buffer and the JSON decode.
//
// The bridge has no wait of its own (#111), so a request is never cut off and the time a request
// takes is not a limit. What a run can fail on is what the engine itself reports: an error (a
// framework failure, or the output buffer, which shows up as a JSON parse failure), a paragraph
// number missing from the output, or an output implausibly small for its input.
//
// A session runs a fixed plan (buildPlan in plan.go), cheapest first so the time cap cuts the least
// valuable data: phase 1, a ladder of sizes per pair, each size repeated, for the spread and the
// latency curve; phase 2, single runs that bracket the zh-Hans>en failure; phase 3, the optional
// ja>en ladder. The planning, statistics and bookkeeping are the pure helpers in plan.go; this file
// makes the Apple calls, reads the machine load and logs.

const gateEnv = "KAI_ENGINE_PROBE"

const (
	minOutPct    = 25 // a pass needs output runes >= 25% of input runes (coarse partial-result guard)
	minTailRunes = 40 // the final paragraph never ends up a stub shorter than this

	// The probe's ceiling on a one-sentence request (the pre-check, the queue drain, the cancel
	// checks); pointCeiling, in plan.go, is the one on a measured run. The bridge has no limit of its
	// own, so a framework that never answers (no main run loop, a hung service) would hang the run.
	// Hitting either ceiling stops the probe with the likely cause and is never recorded as a limit.
	// It goes through the engine's cancel path, which is exercised the same way a user's Cancel is.
	precheckCeiling = 2 * time.Minute

	// A session waits up to quietWait, checking every quietEvery, for a quiet machine.
	quietWait  = 5 * time.Minute
	quietEvery = 30 * time.Second

	// Phase 3 starts only when the session so far took less than this.
	jaLatestStart = 60 * time.Minute

	// The status line written when the budget stops the run (the planned phases are in plan.go).
	phaseBudgetExhausted = "budget_exhausted"
)

// reservedDigits may not appear in a probe sentence. Digits are the paragraph numbers, and a numeral
// in the source (a full-width digit, or a CJK numeral) can come back as an ASCII digit and pass for
// a paragraph number that is missing.
const reservedDigits = "0123456789０１２３４５６７８９〇零一二两三四五六七八九十百千万亿億兆"

// prereqHelp names everything a probe run needs. A run without them fails in ways that look like
// engine limits (or, since the bridge has no wait, never ends), so every prerequisite failure stops
// the probe with this text instead.
const prereqHelp = "The probe needs: (1) external linking with cgo (CGO_ENABLED=1 go test ... -ldflags=-linkmode=external); " +
	"(2) the main-thread TestMain in this file, which parks the main OS thread in dispatch_main(); " +
	"(3) the Swift bridge built first ((cd pkg/swiftbridge/scripts && bash ./build.sh)); " +
	"(4) English and Chinese (Simplified) installed under System Settings > General > Language & Region > Translation Languages, " +
	"and for the probe Spanish too (Japanese is optional)."

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

// script is one language pair plus the fixed prose its input is built from.
type script struct {
	name       string         // the sentence pool, the log's search= key: latin | cjk | ja
	from, to   model.Language // requests use the Kai-side codes; the engine registry maps them
	fromCode   string         // framework codes: the pair label, and the log
	toCode     string
	sentences  []string // no reserved digit anywhere: the digits in the input are the paragraph numbers
	join       string   // between the sentences of one paragraph
	perPara    int      // sentences per paragraph
	terminator rune     // ends every paragraph
	warm       string   // one sentence for the pre-check, the queue drain and the cancel checks
}

// pair is the script's name in the plan, the PROBE lines and the status file (pairENZH, ...).
func (s script) pair() string { return s.fromCode + ">" + s.toCode }

// withTarget returns the same prose, translated into another language.
func (s script) withTarget(to model.Language, toCode string) script {
	s.to, s.toCode = to, toCode
	return s
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

// latinES is the English prose translated into Spanish: Latin to Latin, a target that is not CJK
// (#119's extra pair). Only the target differs from latin.
var latinES = latin.withTarget(model.ES, "es")

// japanese is the optional pair (phase 3): the same prose in Japanese, translated into English. It
// runs only when Japanese is installed; the probe never installs a language.
var japanese = script{
	name: "ja", from: model.JA, to: model.EN, fromCode: "ja", toCode: "en",
	join: "", perPara: 4, terminator: '。',
	warm: "今日は天気が良いので、市場まで歩いて行きます。",
	sentences: []string{
		"委員会は年次報告書を審査し、反対意見のないまま翌期の予算を承認しました。",
		"毎朝、パン職人は日の出前に店を開け、焼きたてのパンを窓際の木の棚に並べます。",
		"研究者たちは川のいくつかの地点で水温を測り、その結果を共有のノートに記録しました。",
		"注意深い読者は、旅人たちが港を離れるとすぐに物語の調子が変わることに気づくでしょう。",
		"新しい図書館には静かな自習室があり、各国の新聞を無料で読めるうえ、入り口の近くには小さなカフェもあります。",
		"技術者たちは大雨と強風の中で橋を試験し、構造が終始安定していたことに満足しました。",
		"長い冬の間、村人たちは集会所に集まって物語を語り合い、網を繕い、春の種まきの計画を立てました。",
		"彼女は、今回のソフトウェア更新によって電池の持ちが良くなり、報告された問題がいくつか修正され、設定メニューが簡単になると説明しました。",
		"博物館では、農家の人々が北の畑を耕しているときに見つけた絵画や硬貨、道具が展示されます。",
		"休暇に出かける前に、ドアに鍵をかけ、電気を消し、鍵を隣人に預けるのを忘れないでください。",
		"道は狭くて急でしたが、運転手たちは辛抱強く、配達のトラックを安全に通らせました。",
		"先生は生徒たちに、好きな季節について説明し、なぜその季節が楽しく穏やかな気持ちにさせてくれるのかを話すように頼みました。",
	},
}

// scripts is every pair the probe can run.
var scripts = []script{latin, cjk, latinES, japanese}

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

// outcome is the record of one probe run.
type outcome struct {
	pt                       planPoint
	runes, bytes, paragraphs int
	kind                     string
	err, class               string // error text and translate.ClassifyEngineError kind; empty for non-errors
	cause                    error  // the error itself, for bufferBound
	missing                  int    // first missing paragraph number, for kind truncated
	latency                  time.Duration
	outRunes, outBytes       int
	load1                    float64 // the 1-minute load average when the run started
	noisy                    bool    // isNoisy(load1): listed, left out of the statistics
}

func (o outcome) ok() bool { return o.kind == kindPass }

// prober drives the Apple engine through the engine.Translator interface.
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

// try runs one planned point, which starts at the 1-minute load average load1, and logs it. The
// PROBE point line keeps every key of the #111 lines (search= is the sentence pool), so those stay
// comparable, and adds the pair, phase, repeat, load and noisy flag. runes counts runes and
// utf8_bytes counts bytes.
func (p *prober) try(s script, pt planPoint, load1 float64) outcome {
	p.t.Helper()
	text, paragraphs := s.buildInput(pt.runes)
	o := outcome{pt: pt, runes: utf8.RuneCountInString(text), bytes: len(text), paragraphs: paragraphs, load1: load1, noisy: isNoisy(load1)}
	if o.runes != pt.runes {
		p.t.Fatalf("probe bug: buildInput(%d) produced %d runes", pt.runes, o.runes)
	}
	res, latency, err := p.call(s, text, pointCeiling)
	o.latency = latency
	switch {
	case err != nil:
		o.kind, o.err, o.class, o.cause = kindError, err.Error(), translate.ClassifyEngineError(err), err
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
	p.t.Logf("PROBE point search=%s pair=%s phase=%s repeat=%d n=%d runes=%d utf8_bytes=%d paragraphs=%d kind=%s latency_ms=%d out_runes=%d out_bytes=%d missing_marker=%d class=%s error=%q load1=%.2f noisy=%t",
		s.name, pt.pair, pt.phase, pt.repeat, pt.runes, o.runes, o.bytes, o.paragraphs, o.kind, o.latency.Milliseconds(), o.outRunes, o.outBytes, o.missing, dash(o.class), o.err, o.load1, o.noisy)
	return o
}

// bridgeLanguages loads the bridge and lists the installed translation languages; a failure stops
// the run with the cause.
func (p *prober) bridgeLanguages() []string {
	t := p.t
	t.Helper()
	if err := swiftbridge.Init(""); err != nil || !swiftbridge.Available() {
		t.Fatalf("probe prerequisite failed: the Swift bridge did not load (init error: %v). %s", err, prereqHelp)
	}
	langs, err := engine.AvailableLanguages()
	if err != nil {
		t.Fatalf("probe prerequisite failed: listing the installed languages failed: %v. %s", err, prereqHelp)
	}
	return langs
}

// ready stops the run with the cause unless the prerequisites of every required script hold: both
// its languages installed, and a one-sentence request of its pair answered. An optional script
// runs too when they hold for it; otherwise it is logged and skipped (never installed). ready
// returns the latency of each pair's first call (it includes prepareTranslation), by pair, and the
// scripts that can run: the required ones, then the optional ones that are ready.
func (p *prober) ready(required []script, optional ...script) (map[string]time.Duration, []script) {
	t := p.t
	t.Helper()
	langs := p.bridgeLanguages()
	all := append(slices.Clone(required), optional...)
	var codes, missing []string
	for i, s := range all {
		for _, c := range []string{s.fromCode, s.toCode} {
			if !slices.Contains(codes, c) {
				codes = append(codes, c)
			}
			if i < len(required) && !hasLang(langs, c) && !slices.Contains(missing, c) {
				missing = append(missing, c)
			}
		}
	}
	fields := make([]string, len(codes))
	for i, c := range codes {
		fields[i] = fmt.Sprintf("%s=%t", strings.ToLower(strings.ReplaceAll(c, "-", "_")), hasLang(langs, c))
	}
	t.Logf("PROBE precheck languages=%d %s", len(langs), strings.Join(fields, " "))
	if len(missing) > 0 {
		t.Fatalf("probe prerequisite failed: the installed languages %v lack %v. %s", langs, missing, prereqHelp)
	}
	first := map[string]time.Duration{}
	var run []script
	for i, s := range all {
		isOptional := i >= len(required)
		if isOptional && (!hasLang(langs, s.fromCode) || !hasLang(langs, s.toCode)) {
			t.Logf("PROBE precheck pair=%s installed=false skipped=true", s.pair())
			continue
		}
		res, d, err := p.call(s, s.warm, precheckCeiling)
		if err != nil || res.Result == "" {
			if !isOptional {
				t.Fatalf("probe prerequisite failed: a one-sentence %s translation failed after %s (error: %v). %s", s.pair(), d, err, prereqHelp)
			}
			t.Logf("PROBE precheck pair=%s first_call_ms=%d ok=false skipped=true error=%q", s.pair(), d.Milliseconds(), errText(err))
			continue
		}
		first[s.pair()] = d
		run = append(run, s)
		t.Logf("PROBE precheck pair=%s first_call_ms=%d ok=true", s.pair(), d.Milliseconds())
	}
	return first, run
}

// precheck is the cancel checks' prerequisite check: the bridge, English and Chinese
// (Simplified), and a first call of each of their two pairs. It returns each first call's latency
// by pair.
func (p *prober) precheck() map[string]time.Duration {
	p.t.Helper()
	first, _ := p.ready([]script{latin, cjk})
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

func errText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// bridgeStamp identifies the bridge dylib the run loaded (dev builds load the one in the source
// tree), so a log can be tied to the build.sh run that produced it. build.sh is deterministic, so
// two sessions on the same bridge show the same sha256.
func bridgeStamp() string {
	p, err := filepath.Abs("../../../pkg/swiftbridge/libkai_bridge.dylib")
	if err != nil {
		return "unknown"
	}
	st, err := os.Stat(p)
	if err != nil {
		return "unknown"
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "unknown"
	}
	return fmt.Sprintf("size=%d mtime=%s sha256=%x", st.Size(), st.ModTime().UTC().Format(time.RFC3339), sha256.Sum256(b))
}

func logHost(t *testing.T, cfg probeConfig) {
	t.Helper()
	ver, _ := unix.Sysctl("kern.osproductversion")
	build, _ := unix.Sysctl("kern.osversion")
	chip, _ := unix.Sysctl("machdep.cpu.brand_string")
	mem, _ := unix.SysctlUint64("hw.memsize")
	t.Logf("PROBE host macos=%s build=%s chip=%q cpus=%d mem_gb=%d go=%s goarch=%s bridge=%q",
		ver, build, chip, runtime.NumCPU(), mem>>30, runtime.Version(), runtime.GOARCH, bridgeStamp())
	t.Logf("PROBE config repeats=%d budget_min=%d max_runes=%d skip_boundary=%t step_floor=%d step_pct=%d min_out_pct=%d point_ceiling_s=%d precheck_ceiling_s=%d out_buf_payload_max=%d status=%q",
		cfg.repeats, int(cfg.budget.Minutes()), cfg.maxRunes, cfg.skipBoundary, stepFloor, stepPct, minOutPct,
		int(pointCeiling.Seconds()), int(precheckCeiling.Seconds()), appleOutBufPayloadMax, cfg.statusPath)
}

// loadAvg returns the 1-, 5- and 15-minute load averages: sysctl vm.loadavg, a struct loadavg of
// three fixed-point values and the scale they are fixed to (what `sysctl -n vm.loadavg` prints).
func loadAvg(t *testing.T) [3]float64 {
	t.Helper()
	b, err := unix.SysctlRaw("vm.loadavg")
	if err != nil || len(b) < 24 || binary.NativeEndian.Uint64(b[16:24]) == 0 {
		t.Fatalf("probe stopped: reading vm.loadavg failed (%d bytes, error %v)", len(b), err)
	}
	scale := float64(binary.NativeEndian.Uint64(b[16:24]))
	var l [3]float64
	for i := range l {
		l[i] = float64(binary.NativeEndian.Uint32(b[4*i:])) / scale
	}
	return l
}

// waitQuiet records the load when the session starts. A session starts only on a quiet machine
// (quietToStart): it waits up to quietWait for one and otherwise stops with the cause, so a loaded
// run is never labelled quiet. A run bounded to smokeMaxRunes or fewer by KAI_ENGINE_PROBE_MAX_RUNES
// is a check of the harness (the smoke run), not a session: it records the load and goes on. A larger
// bound is a capped real session and waits like any other.
func waitQuiet(t *testing.T, cfg probeConfig) {
	t.Helper()
	deadline := time.Now().Add(quietWait)
	for {
		l := loadAvg(t)
		if quiet := quietToStart(l[0]); quiet || isSmokeRun(cfg) {
			t.Logf("PROBE load at=start load1=%.2f load5=%.2f load15=%.2f quiet=%t", l[0], l[1], l[2], quiet)
			return
		}
		if !time.Now().Before(deadline) {
			t.Fatalf("probe stopped: the 1-minute load average is %.2f, above %.0f, after waiting %s. A session starts only when the Mac is not saturated (load 10 or below on this 10-core M1 Max); rerun it when the load drops.",
				l[0], quietLoad, quietWait)
		}
		t.Logf("PROBE load at=wait load1=%.2f load5=%.2f load15=%.2f quiet=false", l[0], l[1], l[2])
		time.Sleep(quietEvery)
	}
}

// realClock is the wall clock, the probeClock settle runs on in a session.
type realClock struct{}

func (realClock) Now() time.Time        { return time.Now() }
func (realClock) Sleep(d time.Duration) { time.Sleep(d) }

// session is one planned run of the probe: its settings, the clock its budget runs on, and every
// run so far.
type session struct {
	p         *prober
	cfg       probeConfig
	started   time.Time                // the budget runs from here, the start of the test
	scripts   map[string]script        // the scripts ready to run, by pair
	first     map[string]time.Duration // the pre-check's first call, by pair
	runs      []outcome                // every measured run, in order
	msPerRune map[string][]float64     // per pair, every run so far: what projects the next point
	partial   bool                     // the budget or a stop ended the run before its plan did

	jaDecided bool // phase 3 was started or skipped

	// Phase 2's bracket: the largest passing and the smallest failing size so far (0: none yet).
	boundaryStarted, confirming bool
	pass, fail                  int
}

func (r *session) elapsed() time.Duration { return time.Since(r.started) }
func (r *session) elapsedS() int          { return int(r.elapsed().Seconds()) }
func (r *session) budgetLeftS() int       { return max(0, int((r.cfg.budget - r.elapsed()).Seconds())) }

// run works through the plan in order. Phase 2 is planned as its first point only: each boundary
// result decides the next boundary point, which goes to the head of the queue.
func (r *session) run(plan []planPoint) {
	t := r.p.t
	queue := slices.Clone(plan)
	for len(queue) > 0 {
		pt := queue[0]
		if pt.pair == pairJAEN && !r.jaDecided {
			r.jaDecided = true
			if el := r.elapsed(); el >= jaLatestStart {
				t.Logf("PROBE phase3 skipped=true pair=%s elapsed_s=%d reason=%q", pt.pair, int(el.Seconds()),
					"phase 3 starts only when the session so far took under "+jaLatestStart.String())
				return // phase 3 is the last in the plan
			}
		}
		s, ok := r.scripts[pt.pair]
		if !ok {
			t.Fatalf("probe bug: the plan has pair %s, which has no ready script", pt.pair)
		}
		if !r.admit(pt) {
			r.exhausted(queue)
			return
		}
		o := r.measure(s, pt)
		queue = queue[1:]
		if pt.phase == phaseBoundary {
			queue = append(r.boundaryNext(o), queue...)
		}
		r.status(o, queue)
	}
}

// admit lets the framework settle after the previous run (settle: a pause, or a queue drain after
// an error; a measured run is never cancelled) and reports whether the budget still has room for
// pt: a point starts only if the elapsed time plus 1.25 times its projection fits the cap.
func (r *session) admit(pt planPoint) bool {
	if r.elapsed() >= r.cfg.budget {
		return false
	}
	if n := len(r.runs); n > 0 {
		prev := r.runs[n-1]
		began := time.Now()
		drain := settle(prev.kind, realClock{}, r.warm(r.scripts[prev.pt.pair]))
		r.p.t.Logf("PROBE settle after=%s queue_drain=%s waited_ms=%d", prev.kind, drain, time.Since(began).Milliseconds())
	}
	return shouldStart(r.elapsed(), r.cfg.budget, projectPoint(pt.runes, r.msPerRune[pt.pair]))
}

// warm is the one-sentence request settle drains the framework's queue with.
func (r *session) warm(s script) func() (time.Duration, error) {
	return func() (time.Duration, error) {
		res, d, err := r.p.call(s, s.warm, precheckCeiling)
		if err == nil && res.Result == "" {
			err = errors.New("empty result")
		}
		r.p.t.Logf("PROBE warm pair=%s latency_ms=%d error=%q", s.pair(), d.Milliseconds(), errText(err))
		return d, err
	}
}

// measure reads the load, runs pt and records it.
func (r *session) measure(s script, pt planPoint) outcome {
	o := r.p.try(s, pt, loadAvg(r.p.t)[0])
	r.runs = append(r.runs, o)
	r.msPerRune[pt.pair] = append(r.msPerRune[pt.pair], float64(o.latency.Milliseconds())/float64(o.runes))
	return o
}

// sizes returns the verdict of every run of pair, in the given phases (every phase when none).
func (r *session) sizes(pair string, phases ...string) []sizeResult {
	var out []sizeResult
	for _, o := range r.runs {
		if o.pt.pair == pair && (len(phases) == 0 || slices.Contains(phases, o.pt.phase)) {
			out = append(out, sizeResult{runes: o.pt.runes, pass: o.ok(), noisy: o.noisy})
		}
	}
	return out
}

// boundaryNext takes one phase 2 result and returns what phase 2 runs next: the next bisection
// point (nextBoundary, starting from the pair's ladder), nothing once the bracket is done, or, when
// the finished bracket lies inside the #111 one, its two points once more (repeat 2, when the budget
// allows; a boundary point is never used for the spread). The failure is decided by output bytes,
// not by load, so the bracket counts noisy runs like any other; the statistics still leave them out.
func (r *session) boundaryNext(o outcome) []planPoint {
	t := r.p.t
	if r.confirming {
		return nil // a confirmation run does not move the bracket
	}
	if !r.boundaryStarted {
		r.boundaryStarted = true
		seed := r.sizes(o.pt.pair, phaseLadder)
		for i := range seed {
			seed[i].noisy = false
		}
		r.pass, _ = maxPassing(seed, capRunes)
	}
	if o.ok() {
		r.pass = max(r.pass, o.pt.runes)
	} else if r.fail == 0 || o.pt.runes < r.fail {
		r.fail = o.pt.runes
	}
	next, done := nextBoundary(r.pass, r.fail)
	if !done {
		if r.cfg.maxRunes > 0 && next > r.cfg.maxRunes {
			t.Logf("PROBE boundary pair=%s done=false stopped=max_runes next_runes=%d pass_runes=%d fail_runes=%s", o.pt.pair, next, r.pass, sizeOrNone(r.fail))
			return nil
		}
		return []planPoint{{phase: phaseBoundary, pair: o.pt.pair, runes: next, repeat: 1}}
	}
	t.Logf("PROBE boundary pair=%s done=true pass_runes=%d fail_runes=%s", o.pt.pair, r.pass, sizeOrNone(r.fail))
	if r.fail != 0 && r.pass >= prior111Pass && r.fail <= prior111Fail {
		r.confirming = true
		return []planPoint{
			{phase: phaseBoundary, pair: o.pt.pair, runes: r.pass, repeat: 2},
			{phase: phaseBoundary, pair: o.pt.pair, runes: r.fail, repeat: 2},
		}
	}
	return nil
}

func sizeOrNone(n int) string {
	if n == 0 {
		return "none"
	}
	return strconv.Itoa(n)
}

// label names one planned point: "ladder en>zh-Hans 1600 #2".
func label(pt planPoint) string {
	return fmt.Sprintf("%s %s %d #%d", pt.phase, pt.pair, pt.runes, pt.repeat)
}

// describe names planned points compactly, the repeats of one size together: "ladder en>es 1600
// x3, boundary zh-Hans>en 12800 x1".
func describe(points []planPoint) string {
	var parts []string
	for i := 0; i < len(points); {
		j := i + 1
		for j < len(points) && points[j].phase == points[i].phase && points[j].pair == points[i].pair && points[j].runes == points[i].runes {
			j++
		}
		parts = append(parts, fmt.Sprintf("%s %s %d x%d", points[i].phase, points[i].pair, points[i].runes, j-i))
		i = j
	}
	return strings.Join(parts, ", ")
}

// status appends run o's line to the progress file; next is the head of the queue, or done.
func (r *session) status(o outcome, queue []planPoint) {
	next := phaseDone
	if len(queue) > 0 {
		next = label(queue[0])
	}
	r.write(statusLine{
		Time: time.Now(), ElapsedS: r.elapsedS(), Phase: o.pt.phase, Pair: o.pt.pair,
		Runes: o.runes, UTF8Bytes: o.bytes, Repeat: o.pt.repeat, LatencyMs: o.latency.Milliseconds(),
		OutBytes: o.outBytes, Kind: o.kind, Load1: o.load1, Next: next, BudgetLeftS: r.budgetLeftS(),
	})
}

// write appends l to the progress file. A file that cannot be written is logged, not fatal: the
// PROBE lines hold every run anyway.
func (r *session) write(l statusLine) {
	if err := appendStatus(r.cfg.statusPath, l); err != nil {
		r.p.t.Logf("PROBE status_error path=%q error=%q", r.cfg.statusPath, err.Error())
	}
}

// exhausted stops the run at the budget: it logs what the cap left out and marks the session
// partial.
func (r *session) exhausted(queue []planPoint) {
	r.partial = true
	remaining := describe(queue)
	r.p.t.Logf("PROBE budget_exhausted elapsed_s=%d remaining=%q", r.elapsedS(), remaining)
	r.write(statusLine{Time: time.Now(), ElapsedS: r.elapsedS(), Phase: phaseBudgetExhausted, Load1: loadAvg(r.p.t)[0], Next: remaining, BudgetLeftS: r.budgetLeftS()})
}

// finish reports the session and ends the progress file: a summary per size, a result per pair,
// the buffer evidence of every failed run, the load at the end, and the session's limit last. It
// runs deferred, so a run the probe stops early still reports what it measured (as partial).
func (r *session) finish() {
	t := r.p.t
	if t.Failed() {
		r.partial = true
	}
	r.summaries()
	limit, basis := r.results()
	l := loadAvg(t)
	t.Logf("PROBE load at=end load1=%.2f load5=%.2f load15=%.2f", l[0], l[1], l[2])
	t.Logf("PROBE apple_limit_runes=%d basis=%s capped=%t partial=%t elapsed_s=%d", limit, basis, limit == capRunes, r.partial, r.elapsedS())
	r.write(statusLine{Time: time.Now(), ElapsedS: r.elapsedS(), Phase: phaseDone, Load1: l[0], Next: "-", BudgetLeftS: r.budgetLeftS()})
}

// summaries logs a PROBE summary per size of each pair and phase, in the order they ran: every
// latency and output size, min, median and max over the runs that are not noisy (summarize), and a
// verdict by the maximum rule (maxPassing: one failed run that is not noisy fails the size).
func (r *session) summaries() {
	type key struct {
		phase, pair string
		runes       int
	}
	var order []key
	groups := map[key][]outcome{}
	for _, o := range r.runs {
		k := key{o.pt.phase, o.pt.pair, o.pt.runes}
		if _, seen := groups[k]; !seen {
			order = append(order, k)
		}
		groups[k] = append(groups[k], o)
	}
	for _, k := range order {
		runs := groups[k]
		samples, sizes := make([]sample, len(runs)), make([]sizeResult, len(runs))
		lat, out := make([]string, len(runs)), make([]string, len(runs))
		passed, noisy := 0, 0
		for i, o := range runs {
			samples[i] = sample{latency: o.latency, noisy: o.noisy}
			sizes[i] = sizeResult{runes: k.runes, pass: o.ok(), noisy: o.noisy}
			lat[i], out[i] = strconv.FormatInt(o.latency.Milliseconds(), 10), strconv.Itoa(o.outBytes)
			if o.ok() {
				passed++
			}
			if o.noisy {
				noisy++
			}
		}
		st := summarize(samples)
		verdict := "pass"
		if best, _ := maxPassing(sizes, capRunes); best != k.runes {
			verdict = "fail"
			if noisy == len(runs) {
				verdict = "no_quiet_run"
			}
		}
		r.p.t.Logf("PROBE summary phase=%s pair=%s runes=%d utf8_bytes=%d runs=%d passed=%d noisy=%d latencies_ms=%s min_ms=%d median_ms=%d max_ms=%d median_ms_per_rune=%.2f out_bytes=%s verdict=%s",
			k.phase, k.pair, k.runes, runs[0].bytes, len(runs), passed, noisy, strings.Join(lat, ","),
			st.min.Milliseconds(), st.median.Milliseconds(), st.max.Milliseconds(),
			float64(st.median)/float64(time.Millisecond)/float64(k.runes), strings.Join(out, ","), verdict)
	}
}

// results logs a PROBE result per pair (the largest size at which every run that is not noisy
// passed, and the smallest size with a failed one) and a PROBE failure per failed run, with the
// output bytes projected from the pair's passing runs against the buffer (bufferBound). It returns
// the session's limit: the smaller of the en>zh-Hans and zh-Hans>en maxima, or an extra pair's
// maximum when that pair failed below it.
func (r *session) results() (limit int, basis string) {
	t := r.p.t
	var pairs []string
	for _, o := range r.runs {
		if !slices.Contains(pairs, o.pt.pair) {
			pairs = append(pairs, o.pt.pair)
		}
	}
	maxOf, failed := map[string]int{}, map[string]bool{}
	for _, pair := range pairs {
		best, capped := maxPassing(r.sizes(pair), capRunes)
		maxOf[pair] = best
		var fail *outcome
		var atMax []sample
		for i := range r.runs {
			o := &r.runs[i]
			if o.pt.pair != pair {
				continue
			}
			if o.pt.runes == best {
				atMax = append(atMax, sample{latency: o.latency, noisy: o.noisy})
			}
			if !o.noisy && !o.ok() && (fail == nil || o.pt.runes < fail.pt.runes) {
				fail = o
			}
		}
		failed[pair] = fail != nil
		minFail, failKind, failClass, failLatency, failErr := "none", "-", "-", int64(0), ""
		if fail != nil {
			minFail, failKind, failClass = strconv.Itoa(fail.pt.runes), fail.kind, dash(fail.class)
			failLatency, failErr = fail.latency.Milliseconds(), fail.err
		}
		s := r.scripts[pair]
		t.Logf("PROBE result search=%s pair=%s from=%s to=%s max_pass_runes=%d min_fail_runes=%s capped=%t latency_at_max_ms=%d first_call_ms=%d fail_kind=%s fail_class=%s fail_latency_ms=%d fail_error=%q",
			s.name, pair, s.fromCode, s.toCode, best, minFail, capped, summarize(atMax).median.Milliseconds(), r.first[pair].Milliseconds(), failKind, failClass, failLatency, failErr)
	}
	for _, o := range r.runs {
		if o.ok() {
			continue
		}
		var perRune []float64
		for _, q := range r.runs {
			if q.pt.pair == o.pt.pair && q.ok() {
				perRune = append(perRune, float64(q.outBytes)/float64(q.runes))
			}
		}
		projected := projectOutBytes(o.runes, perRune)
		t.Logf("PROBE failure pair=%s phase=%s repeat=%d runes=%d utf8_bytes=%d kind=%s noisy=%t out_bytes=%d projected_out_bytes=%d out_buf_payload_max=%d buffer_bound=%t error=%q",
			o.pt.pair, o.pt.phase, o.pt.repeat, o.runes, o.bytes, o.kind, o.noisy, o.outBytes, projected, appleOutBufPayloadMax, bufferBound(o.outBytes, projected, o.cause), o.err)
	}
	limit, basis = maxOf[pairENZH], pairENZH
	switch zh := maxOf[pairZHEN]; {
	case zh < limit:
		limit, basis = zh, pairZHEN
	case zh == limit:
		basis = pairENZH + "+" + pairZHEN
	}
	for _, extra := range []string{pairENES, pairJAEN} {
		if failed[extra] && maxOf[extra] < limit {
			limit, basis = maxOf[extra], extra
		}
	}
	return limit, basis
}

func TestProbeApple(t *testing.T) {
	if os.Getenv(gateEnv) != "1" {
		t.Skip("set " + gateEnv + "=1 to run the Apple input-limit probe (the full command is in doc.go)")
	}
	started := time.Now()
	cfg, err := probeConfigFromEnv(os.Getenv)
	if err != nil {
		t.Fatalf("probe settings: %v", err)
	}
	if _, err := buildPlan(cfg); err != nil {
		t.Fatalf("probe plan: %v", err) // before any Apple call: a bad knob costs nothing
	}
	for _, s := range scripts {
		for _, sentence := range append([]string{s.warm}, s.sentences...) {
			if strings.ContainsAny(sentence, reservedDigits) {
				t.Fatalf("probe bug: %s sentence %q contains a digit or a numeral; digits are reserved for the paragraph numbers", s.name, sentence)
			}
		}
	}
	logHost(t, cfg)
	waitQuiet(t, cfg)
	p := &prober{t: t, tr: engine.NewApple()}
	first, ready := p.ready([]script{latin, cjk, latinES}, japanese)
	cfg.includeJA = slices.ContainsFunc(ready, func(s script) bool { return s.pair() == pairJAEN })
	plan, err := buildPlan(cfg)
	if err != nil {
		t.Fatalf("probe plan: %v", err)
	}
	t.Logf("PROBE plan points=%d include_ja=%t plan=%q", len(plan), cfg.includeJA, describe(plan))
	r := &session{p: p, cfg: cfg, started: started, scripts: map[string]script{}, first: first, msPerRune: map[string][]float64{}}
	for _, s := range ready {
		r.scripts[s.pair()] = s
	}
	defer r.finish()
	r.run(plan)
}
