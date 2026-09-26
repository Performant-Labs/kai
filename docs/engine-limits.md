# Engine input limits

Where each translation engine breaks on large input, and the input budget the chunker (#84) works to. Issue #83, child of epic #86.

The budget lives in code: `internal/engine/input_budget.go` (`InputBudget`, `Budget.Max`). This page is its written record and is checked against it. `TestEngineLimitsDocListsEveryTranslator` fails when a translator has no row here, or when a row's Verified, Follow-up or link cell disagrees with the table in code.

**Status.** Only apple is measured (one probe run, recorded below). Every other row is a provisional budget taken from the engine's documented limit: `Verified` is `no` and the follow-up issue in the last column measures it.

## Reading the table

- **Unit** is what the engine limits, and what the budget counts. `runes` is `utf8.RuneCountInString(text)`. `utf8 bytes` is `len(text)`. `query-escaped bytes` is `len(url.QueryEscape(text))`, which is what a URL or form value costs on the wire (a CJK character is 9).
- **Latin max** and **CJK max** are the largest inputs the probe saw the engine accept, in runes: English to Chinese (Latin) and Chinese to English (CJK).
- **Budget (80%)** is what a chunker may send: the limit times `BudgetMarginPercent` (80), rounded down, in the row's unit. For apple the limit is the smaller of the Latin and CJK maximums.
- **Verified**: `yes` means measured by a probe run on the recorded host. `no` means a documented or provisional figure that has not been checked against the live service.

## Limits per engine

| Engine | Unit | Latin max | CJK max | Failure past the limit | Latency at max | Documented limit | Budget (80%) | Verified | Date | Follow-up |
|---|---|---|---|---|---|---|---|---|---|---|
| apple | runes | 2250 | 650 | `[dynamic-bridge] System translation returned empty` after 20.0 s, kind `engine` (the bridge's 20 s wait expiring) | Latin 19.6 s, CJK 17.9 s | none found in Apple's Translation documentation (checked 2026-09-26); Kai's bridge imposes a 20 s wait (`apple_translate.swift`) and a 65,535-byte output buffer (`apple_darwin.go`) | 520 runes | yes | 2026-09-26 | - |
| google | query-escaped bytes | unmeasured | unmeasured | unmeasured | unmeasured | undocumented for the gtx endpoint Kai calls; nearest published figure is Cloud Translation's recommended maximum of 5K characters (code points) per request ([quotas](https://docs.cloud.google.com/translate/quotas)) | 4000 query-escaped bytes | no | - | #87 |
| deepl | query-escaped bytes | unmeasured | unmeasured | unmeasured | unmeasured | request size limit of 128 KiB for the whole request ([API reference](https://developers.deepl.com/api-reference/translate/request-translation)) | 104857 query-escaped bytes | no | - | #88 |
| openai | runes | unmeasured | unmeasured | unmeasured | unmeasured | no input limit binds, the output cap does; Kai sets none, so the model default applies; 8192 output tokens is the assumed basis ([API reference](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)) | 3276 runes | no | - | #92 |
| anthropic | runes | unmeasured | unmeasured | unmeasured | unmeasured | no input limit binds, the output cap does; Kai sets `MaxTokens` to 8192 in `anthropic.go` ([Messages API](https://platform.claude.com/docs/en/api/messages)) | 3276 runes | no | - | #93 |
| gemini | runes | unmeasured | unmeasured | unmeasured | unmeasured | no input limit binds, the output cap does; Kai sets `MaxOutputTokens` to 8192 in `gemini.go` ([API reference](https://ai.google.dev/api/generate-content)) | 3276 runes | no | - | #94 |
| baidu | utf8 bytes | unmeasured | unmeasured | unmeasured | unmeasured | 6000 bytes per request, about 2000 Chinese characters ([API doc](https://fanyi-api.baidu.com/doc/21)) | 4800 utf8 bytes | no | - | #89 |
| tencent | runes | unmeasured | unmeasured | unmeasured | unmeasured | text length below 2000, unit not stated, read as characters ([SDK doc comment](https://pkg.go.dev/github.com/tencentyun/tencentcloud-sdk-go/tencentcloud/tmt/v20180321)) | 1600 runes | no | - | #90 |
| youdao | runes | unmeasured | unmeasured | unmeasured | unmeasured | 5000 characters per query ([API doc](https://ai.youdao.com/DOCSIRMA/html/trans/api/wbfy/index.html)) | 4000 runes | no | - | #91 |

## What the apple numbers say

- **The bridge's 20 s wait binds, not the framework and not the output buffer.** `kai_translate` waits 20 s on a semaphore (`sema.wait(timeout: .now() + 20)` in `pkg/swiftbridge/internal/swift/apple_translate.swift`) and returns `{}` when the wait expires. Every failing point in both searches came back as `returned empty` at 20.00 s (20,001 to 20,006 ms). The largest translation the probe received was 2,286 bytes, against the 65,535 bytes the output buffer holds, so that ceiling is not reachable at these sizes.
- **Two numbers because script changes the time per rune.** Latency grew about linearly with size: roughly 10 ms per rune for Latin text and 25 to 30 ms per rune for CJK text, on top of a fixed cost of about 2 s per call (a warm one-sentence request takes 1.7 to 2.5 s). The apple budget takes the smaller number, so it is conservative for Latin text: a per-script budget would allow chunks about 3.5 times larger there (2250 against 650). The #83 brief (`docs/handoffs/83-brief.md`) fixes one rune budget per engine, so that is not done here.
- **The maximum is one draw near a noisy boundary.** An earlier run of the same probe, with coarser search bounds (lower bound 500, resolution floor 250 runes), bracketed Latin at 1750 pass / 2000 fail and CJK at 500 pass / 750 fail. The recorded run found Latin at 2250 pass / 2300 fail and CJK at 650 pass / 700 fail. The CJK brackets agree; the Latin ones differ by about 20%, and the same 2000-rune Latin input failed at 20.0 s in the first run and passed in 18.3 s in the second. The cause was not investigated (the first run's first call was slower, 6.1 s against 3.6 s, which fits a cold start). Near the limit a call only sometimes finishes inside 20 s, so a recorded maximum is not a guarantee, and the 20% margin is about as wide as the spread seen here.
- **The failure is reported as an empty result.** The Go engine turns the bridge's `{}` into `err.apple_translate_empty` ("System translation returned empty"). `translate.ClassifyEngineError` finds none of its keywords in that text (no "timeout"), so it classifies as `engine`, not `network`. That is recorded, not changed here (failure reasons are #96). After such a failure the abandoned Swift task keeps running, so the probe waits 20 s and then requires a one-sentence request to answer promptly before it starts the next point (the `settle` lines in the log).
- **The number belongs to this host and to the bridge.** It depends on the chip's speed and moves if the wait in `apple_translate.swift` changes. Re-run the probe on a slower host, or after any change to that wait, before trusting it.

## Notes on the provisional rows

- **google**: the same quotas page also lists hard limits for the paid APIs (Basic: 100K bytes per request; Advanced: 30K code points), none of which is stated for the gtx endpoint. The text travels in the GET URL, and query-escaped bytes are never fewer than code points, so 5000 escaped bytes is stricter than the recommendation (about 555 CJK characters). It is deliberately conservative; #87 replaces it.
- **deepl**: 128 KiB is the whole request body. The other form fields are under 60 bytes, which the 20% margin covers.
- **baidu**: the documentation page renders by JavaScript, so the sentence was read from a search of that page on 2026-09-26 and could not be re-fetched when this page was written. #89 measures it.
- **tencent**: the SDK comment gives no unit; it is read as characters. If #90 finds it counts bytes, the budget is too large for CJK text by up to 3 times.
- **openai, anthropic, gemini**: the input context is far larger than any chunk, so the output cap binds. 4096 runes assumes at most 2 output tokens per character, an assumption and not a measurement, and conservative for Latin text. None of the three engines checks the stop or finish reason, so an output cut at the cap comes back as a successful, silently truncated translation. The 30 s request timeout may bind before the cap on a slow model; the budget does not model that.

## Probe run

Recorded on 2026-09-26, started 8:16 AM MDT, 370 s elapsed, by the command below (from the repository root, after building the bridge with `(cd pkg/swiftbridge/scripts && bash ./build.sh)`):

```sh
KAI_ENGINE_PROBE=1 CGO_ENABLED=1 go test -tags enginelimits -ldflags=-linkmode=external -run TestProbeApple -timeout 60m -count=1 -v ./internal/engine/enginelimits/
```

- **Host**: macOS 27.0 (build 26A428), Apple M1 Max (10 cores, 32 GB), Go go1.27.1 darwin/arm64. The bridge dylib was built by `build.sh` at 8:08 AM MDT (14:08 UTC), minutes before the run.
- **Language pairs**: Latin is English to Chinese Simplified (`en` to `zh-Hans`), CJK is Chinese Simplified to English. The source is always explicit, never auto. Requests use Kai's codes (`en`, `zh`), so they go through the same registry mapping the app uses.
- **Path**: `engine.NewApple()` behind the `engine.Translator` interface, the code the app runs (trim, code mapping, 64 KiB output buffer, JSON decode). The probe never calls `swiftbridge.KaiTranslate` directly.
- **Input**: deterministic numbered paragraphs (`17. ` then fixed prose, no other digits anywhere), exactly n runes, the final paragraph absorbing the remainder. The prose is a fixed set of 12 sentences repeated, so real text may translate faster or slower.
- **A point passes** when `Translate` returns no error, every paragraph number appears in the output in order (a missing or misordered number is a silent truncation, kind `truncated`), and the output has at least 25% as many runes as the input.
- **Search**: doubling from 100 runes until a point fails (capped at 100,000, the epic's hard cap), then bisection down to the larger of 50 runes and 1%. The #83 brief specified a lower bound of 500 and a resolution floor of 250 runes; both were tightened after the first run showed a limit of only a few hundred runes (see "Deviations" in `docs/handoffs/83/handoff-F.md`). Failure is assumed monotonic in size; every point is logged, so a non-monotonic result would be visible.
- **Recorded values**: the largest passing size and the smallest failing size per search, the failure's error text and its `ClassifyEngineError` kind, the latency of the largest passing call, and the first call's latency per pair (which includes `prepareTranslation`).

Raw `PROBE` lines from the run, with the test framework's `probe_test.go:NNN:` prefix removed:

```text
PROBE host macos=27.0 build=26A428 chip="Apple M1 Max" cpus=10 mem_gb=32 go=go1.27.1 goarch=arm64 bridge="size=245472 mtime=2026-09-26T14:08:00Z"
PROBE config low=100 high=100000 step_floor=50 step_pct=1 min_out_pct=25 slow_call_ms=15000 settle_wait_ms=20000
PROBE precheck languages=47 en=true zh_hans=true
PROBE precheck pair=en>zh-Hans first_call_ms=3553 ok=true
PROBE precheck pair=zh-Hans>en first_call_ms=1747 ok=true
PROBE point search=latin n=100 runes=100 utf8_bytes=100 paragraphs=1 kind=pass latency_ms=2102 out_runes=26 out_bytes=74 missing_marker=0 class=- error=""
PROBE point search=latin n=200 runes=200 utf8_bytes=200 paragraphs=1 kind=pass latency_ms=2966 out_runes=63 out_bytes=179 missing_marker=0 class=- error=""
PROBE point search=latin n=400 runes=400 utf8_bytes=400 paragraphs=2 kind=pass latency_ms=4751 out_runes=119 out_bytes=343 missing_marker=0 class=- error=""
PROBE point search=latin n=800 runes=800 utf8_bytes=800 paragraphs=4 kind=pass latency_ms=8415 out_runes=241 out_bytes=687 missing_marker=0 class=- error=""
PROBE point search=latin n=1600 runes=1600 utf8_bytes=1600 paragraphs=7 kind=pass latency_ms=16670 out_runes=458 out_bytes=1310 missing_marker=0 class=- error=""
PROBE point search=latin n=3200 runes=3200 utf8_bytes=3200 paragraphs=13 kind=error latency_ms=20001 out_runes=0 out_bytes=0 missing_marker=0 class=engine error="[dynamic-bridge] System translation returned empty"
PROBE settle try=1 latency_ms=2367 ok=true
PROBE point search=latin n=2400 runes=2400 utf8_bytes=2400 paragraphs=10 kind=error latency_ms=20005 out_runes=0 out_bytes=0 missing_marker=0 class=engine error="[dynamic-bridge] System translation returned empty"
PROBE settle try=1 latency_ms=2335 ok=true
PROBE point search=latin n=2000 runes=2000 utf8_bytes=2000 paragraphs=9 kind=pass latency_ms=18290 out_runes=584 out_bytes=1658 missing_marker=0 class=- error=""
PROBE point search=latin n=2200 runes=2200 utf8_bytes=2200 paragraphs=9 kind=pass latency_ms=19337 out_runes=637 out_bytes=1825 missing_marker=0 class=- error=""
PROBE point search=latin n=2300 runes=2300 utf8_bytes=2300 paragraphs=10 kind=error latency_ms=20005 out_runes=0 out_bytes=0 missing_marker=0 class=engine error="[dynamic-bridge] System translation returned empty"
PROBE settle try=1 latency_ms=2329 ok=true
PROBE point search=latin n=2250 runes=2250 utf8_bytes=2250 paragraphs=10 kind=pass latency_ms=19647 out_runes=661 out_bytes=1885 missing_marker=0 class=- error=""
PROBE result search=latin from=en to=zh-Hans max_pass_runes=2250 min_fail_runes=2300 capped=false latency_at_max_ms=19647 first_call_ms=3553 fail_kind=error fail_class=engine fail_latency_ms=20005 fail_error="[dynamic-bridge] System translation returned empty"
PROBE point search=cjk n=100 runes=100 utf8_bytes=294 paragraphs=1 kind=pass latency_ms=3614 out_runes=337 out_bytes=337 missing_marker=0 class=- error=""
PROBE point search=cjk n=200 runes=200 utf8_bytes=584 paragraphs=2 kind=pass latency_ms=6523 out_runes=683 out_bytes=684 missing_marker=0 class=- error=""
PROBE point search=cjk n=400 runes=400 utf8_bytes=1174 paragraphs=3 kind=pass latency_ms=12636 out_runes=1418 out_bytes=1419 missing_marker=0 class=- error=""
PROBE point search=cjk n=800 runes=800 utf8_bytes=2344 paragraphs=6 kind=error latency_ms=20006 out_runes=0 out_bytes=0 missing_marker=0 class=engine error="[dynamic-bridge] System translation returned empty"
PROBE settle try=1 latency_ms=2478 ok=true
PROBE point search=cjk n=600 runes=600 utf8_bytes=1764 paragraphs=4 kind=pass latency_ms=17719 out_runes=2109 out_bytes=2110 missing_marker=0 class=- error=""
PROBE point search=cjk n=700 runes=700 utf8_bytes=2054 paragraphs=5 kind=error latency_ms=20003 out_runes=0 out_bytes=0 missing_marker=0 class=engine error="[dynamic-bridge] System translation returned empty"
PROBE settle try=1 latency_ms=4448 ok=true
PROBE point search=cjk n=650 runes=650 utf8_bytes=1904 paragraphs=5 kind=pass latency_ms=17885 out_runes=2284 out_bytes=2286 missing_marker=0 class=- error=""
PROBE result search=cjk from=zh-Hans to=en max_pass_runes=650 min_fail_runes=700 capped=false latency_at_max_ms=17885 first_call_ms=1747 fail_kind=error fail_class=engine fail_latency_ms=20003 fail_error="[dynamic-bridge] System translation returned empty"
PROBE apple_limit_runes=650 basis=cjk capped=false elapsed_s=370
```

The committed apple limit is the smaller of the two `max_pass_runes` values: `min(2250, 650) = 650` (`apple_limit_runes` in the last line). The budget is 650 * 80 / 100 = 520.

## How to re-run

1. Build the bridge: `(cd pkg/swiftbridge/scripts && bash ./build.sh)`.
2. Install English and Chinese (Simplified) under System Settings > General > Language & Region > Translation Languages.
3. Run the command above from the repository root. It takes about 6 minutes on the recorded host, and longer on a slower one.
4. Compare `apple_limit_runes` with the apple `Limit` in `internal/engine/input_budget.go`. When they differ materially (a slower host, or a changed bridge wait), update that row and this page: the Latin and CJK maximums, the latency, the host and the date.

The probe is opt-in on purpose: it needs the build tag `enginelimits` and `KAI_ENGINE_PROBE=1`, and it is not part of CI, the Makefile, the Taskfile or the pipeline's test command. It needs external linking and a main-thread run loop (the `TestMain` in the probe parks the main OS thread in `dispatch_main`). Without them the framework reports no installed languages, or every call returns `returned empty` after exactly 20 s, which looks like a limit. The probe checks the prerequisites first and stops with the cause instead of reporting a limit.
