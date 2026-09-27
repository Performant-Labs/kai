# Engine input limits

Where each translation engine breaks on large input, and the input budget the chunker (#84) works to. Issue #83, child of epic #86. The bridge's fixed 20 s wait on apple was removed for #111; #119 measured the apple row on a quiet Mac.

The budget lives in code: `internal/engine/input_budget.go` (`InputBudget`, `Budget.Max`). This page is its written record and is checked against it. `TestEngineLimitsDocListsEveryTranslator` fails when a translator has no row here, or when a row's Verified, Follow-up or link cell disagrees with the table in code.

**Status.** apple is measured (#119): `Verified` is `yes`, and it carries no follow-up (`Verified` = `yes` rows never do). Every other row is provisional or documented, not yet checked against the live service: `Verified` is `no` and the follow-up issue in the last column measures it.

## Reading the table

- **Unit** is what the engine limits, and what the budget counts. `runes` is `utf8.RuneCountInString(text)`. `utf8 bytes` is `len(text)`. `query-escaped bytes` is `len(url.QueryEscape(text))`, which is what a URL or form value costs on the wire (a CJK character is 9).
- **Latin max** and **CJK max** are the largest inputs seen to pass, in runes: English to Chinese (Latin) and Chinese to English (CJK). In a measured row they come from a probe run on a recorded host. `>= N` means N runes was the largest size seen to pass, not a ceiling (the apple Latin search hit the probe session's own time cap at 20,000, not a failure).
- **Budget (80%)** is what a chunker may send: the limit times `BudgetMarginPercent` (80), rounded down, in the row's unit. For the measured apple row the limit is the smaller of the Latin and CJK maxima (see "Apple, measured (#119)").
- **Verified**: `yes` means measured by a probe run on a recorded host (apple, since #119). `no` means a documented or provisional figure that has not been checked against the live service.

## Limits per engine

| Engine | Unit | Latin max | CJK max | Failure past the limit | Latency at max | Documented limit | Budget (80%) | Verified | Date | Follow-up |
|---|---|---|---|---|---|---|---|---|---|---|
| apple | runes | >= 20000 | 18200 | the 64 KiB output buffer (#106): CJK fails at 19,100 runes, needing ~67,187 output bytes against the 65,535-byte buffer | CJK 454.4 s median at 18,200 (min 451.3 s, max 457.6 s); Latin 160.9 s at the 20,000-rune cap | none found in Apple's Translation documentation (checked 2026-09-26); Kai's bridge has no wait of its own since #111 and a 65,535-byte output buffer (`apple_darwin.go`), owned by #106 | 14560 runes | yes | 2026-09-27 (MDT), Apple M1 Max, macOS 27.0 (26A428), 10 cores, 32 GB, Go go1.27.1 | - |
| google | query-escaped bytes | unmeasured | unmeasured | unmeasured | unmeasured | undocumented for the gtx endpoint Kai calls; nearest published figure is Cloud Translation's recommended maximum of 5K characters (code points) per request ([quotas](https://docs.cloud.google.com/translate/quotas)) | 4000 query-escaped bytes | no | - | #87 |
| deepl | query-escaped bytes | unmeasured | unmeasured | unmeasured | unmeasured | request size limit of 128 KiB for the whole request ([API reference](https://developers.deepl.com/api-reference/translate/request-translation)) | 104857 query-escaped bytes | no | - | #88 |
| openai | runes | unmeasured | unmeasured | unmeasured | unmeasured | no input limit binds, the output cap does; Kai sets none, so the model default applies; 8192 output tokens is the assumed basis ([API reference](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)) | 3276 runes | no | - | #92 |
| anthropic | runes | unmeasured | unmeasured | unmeasured | unmeasured | no input limit binds, the output cap does; Kai sets `MaxTokens` to 8192 in `anthropic.go` ([Messages API](https://platform.claude.com/docs/en/api/messages)) | 3276 runes | no | - | #93 |
| gemini | runes | unmeasured | unmeasured | unmeasured | unmeasured | no input limit binds, the output cap does; Kai sets `MaxOutputTokens` to 8192 in `gemini.go` ([API reference](https://ai.google.dev/api/generate-content)) | 3276 runes | no | - | #94 |
| baidu | utf8 bytes | unmeasured | unmeasured | unmeasured | unmeasured | 6000 bytes per request, about 2000 Chinese characters ([API doc](https://fanyi-api.baidu.com/doc/21)) | 4800 utf8 bytes | no | - | #89 |
| tencent | runes | unmeasured | unmeasured | unmeasured | unmeasured | text length below 2000, unit not stated, read as characters ([SDK doc comment](https://pkg.go.dev/github.com/tencentyun/tencentcloud-sdk-go/tencentcloud/tmt/v20180321)) | 1600 runes | no | - | #90 |
| youdao | runes | unmeasured | unmeasured | unmeasured | unmeasured | 5000 characters per query ([API doc](https://ai.youdao.com/DOCSIRMA/html/trans/api/wbfy/index.html)) | 4000 runes | no | - | #91 |

## Apple, measured (#119)

- **Time does not bind.** Since #111 the bridge waits for Apple with no timer (`job.sema.wait()` in `pkg/swiftbridge/internal/swift/apple_translate.swift`); a request runs to the end or until the user cancels it. The old 20 s wait produced the first numbers (2250 and 650 runes), so they were the wait, not the framework. Two probe sessions confirm it: English to Chinese never failed at any size tried, up to the session cap of 20,000 runes (about 161 s), and latency grows in proportion to size, not worse.
- **The 64 KiB output buffer binds for Chinese to English**, near 19,100 runes: the failing call projects to about 67,187 output bytes against the 65,535-byte buffer (`apple_darwin.go`). 18,200 passed; #106 owns the buffer, and this limit should be re-measured once it raises. English to Spanish and Japanese to English (the two extra pairs #119 tried) never failed at any size probed, up to 6,400 runes.
- **Cancel frees Kai, not the framework.** A cancelled call returns at once, but Apple keeps working on the abandoned text and answers the next request only when it is done (see "Cancel checks"). Nothing in Kai can shorten that, so the chunker (#84) should keep Apple chunks well below `Max()`, not up near the limit.
- **Quality at large input sizes is unmeasured.** The probe only checks that a call succeeds and times it; it does not check whether the translated text stays accurate as input grows. Kai's bridge sends one whole string per call (`pkg/swiftbridge/internal/swift/apple_translate.swift`), not Apple's batch API (`translate(batch:)` / `translations(from:)`), which translates an array of separate strings and can return results as each one finishes. #84 should chunk by paragraph and consider the batch API, both to avoid sending a single very large string whose quality is untested and to give progressive results, rather than picking a chunk size from latency alone. Epic #151 builds a dedicated quality probe for this (structural integrity, round-trip drift, and an optional DeepL divergence check) — see `docs/quality-limits.md`; as of 2026-09-27 that probe's tooling exists but has not been run yet.

## Notes on the provisional rows

- **google**: the same quotas page also lists hard limits for the paid APIs (Basic: 100K bytes per request; Advanced: 30K code points), none of which is stated for the gtx endpoint. The text travels in the GET URL, and query-escaped bytes are never fewer than code points, so 5000 escaped bytes is stricter than the recommendation (about 555 CJK characters). It is deliberately conservative; #87 replaces it.
- **deepl**: 128 KiB is the whole request body. The other form fields are under 60 bytes, which the 20% margin covers.
- **baidu**: the documentation page renders by JavaScript, so the sentence was read from a search of that page on 2026-09-26 and could not be re-fetched when this page was written. #89 measures it.
- **tencent**: the SDK comment gives no unit; it is read as characters. If #90 finds it counts bytes, the budget is too large for CJK text by up to 3 times.
- **openai, anthropic, gemini**: the input context is far larger than any chunk, so the output cap binds. 4096 runes assumes at most 2 output tokens per character, an assumption and not a measurement, and conservative for Latin text. None of the three engines checks the stop or finish reason, so an output cut at the cap comes back as a successful, silently truncated translation.

## Cancel checks

`internal/engine/enginelimits/cancel_test.go` checks the Apple cancel path against the real framework. The Swift half of the bridge has no other behaviour test in CI (`pkg/swiftbridge/swift_source_test.go` only reads its text), so this is the record that it works. Recorded on 2026-09-26 on an Apple M1 Max, macOS 27.0 (build 26A428), with the bridge whose sha256 is `3cf6379b06fdf3226ba29bbe783ce714af3d391e969aa118ed2e3371b5dfc2f2` (`build.sh` is deterministic), by:

```sh
KAI_ENGINE_PROBE=1 CGO_ENABLED=1 go test -tags enginelimits -ldflags=-linkmode=external -run 'TestAppleCancel|TestAppleNotCutOff' -timeout 30m -count=1 -v ./internal/engine/enginelimits/
```

Raw lines (`PROBE precheck` is the probe's prerequisite check, run at the start of each test):

```text
PROBE precheck languages=47 en=true zh_hans=true
PROBE precheck pair=en>zh-Hans first_call_ms=3009 ok=true
PROBE precheck pair=zh-Hans>en first_call_ms=1618 ok=true
CANCEL a not_cut_off runes=3200 latency_ms=27011 err=<nil>
PROBE precheck languages=47 en=true zh_hans=true
PROBE precheck pair=en>zh-Hans first_call_ms=1651 ok=true
PROBE precheck pair=zh-Hans>en first_call_ms=1564 ok=true
CANCEL b cancel_after_ms=3000 returned_after_ms=3000 lag_ms=0 err=context canceled
CANCEL d next_request_after_cancel latency_ms=24773 err=<nil> (queues behind the abandoned work; not asserted)
PROBE precheck languages=47 en=true zh_hans=true
PROBE precheck pair=en>zh-Hans first_call_ms=1392 ok=true
PROBE precheck pair=zh-Hans>en first_call_ms=1503 ok=true
CANCEL c before_start latency_ms=0 code="cancelled" err=<nil>
CANCEL c same_id_again latency_ms=1417 code="" result_len=51 err=<nil>
PROBE precheck languages=47 en=true zh_hans=true
PROBE precheck pair=en>zh-Hans first_call_ms=1398 ok=true
PROBE precheck pair=zh-Hans>en first_call_ms=1507 ok=true
CANCEL r running_call code="cancelled" latency_ms=501 err=<nil>
CANCEL r zero_id code="" latency_ms=1491 result_len=51 err=<nil>
```

- **A request the old wait cut off completes** (`TestAppleNotCutOff`): 3,200 Latin runes, which failed at exactly 20.0 s under the old wait, translated in 27.0 s through `engine.NewApple()`, with every paragraph number in the output.
- **A cancelled ctx frees the caller at once and reaches the bridge** (`TestAppleCancelReachesSwift`, line `b`): the ctx was cancelled 3 s into that request, `Translate` returned less than a millisecond later with `context.Canceled` itself, not error copy, and the bridge's own call returned the `cancelled` payload instead of running on.
- **The next request is not lost, but it waits** (same test, line `d`): a one-sentence request sent right after took 24.8 s, because Apple was still working on the abandoned text. Nothing in Kai can shorten that. This is why the chunker should keep Apple chunks small (see "Apple, provisional").
- **A cancel that arrives before its call is remembered** (`TestAppleCancelBeforeStart`): `kai_translate_cancel` on a token with no call returned 0, the call that followed with that token returned `cancelled` in under a millisecond and translated nothing, and the same token used again afterwards translated normally (the remembered cancel was consumed).
- **The return values hold** (`TestAppleCancelReturnValues`): cancelling a running call returns 1 and frees it in 1 ms (the cancel came at 500 ms and the call ended at 501 ms), cancelling it again returns 0, and a token of 0 or less can never be cancelled (its call ran to a translation).

Only the lines the test names "asserted" decide a verdict; every latency is logged and none is asserted, because how long the framework takes is the machine's.

## Probe runs (#119)

Both sessions ran on the same Mac and bridge build: Apple M1 Max, macOS 27.0 (26A428), 10 cores, 32 GB, Go go1.27.1.

| Session | Started (MDT) | Load at start | Duration | Scope |
|---|---|---|---|---|
| 1 | 2026-09-26 18:27 | ~8-10 | 72.8 min | full ladder + CJK boundary search |
| 2 | 2026-09-27 08:57 | ~9-14 (settled below 10 before starting) | 32.2 min | ladder only, `KAI_ENGINE_PROBE_SKIP_BOUNDARY=1`, capped at 6,400 runes; picked up `ja>en` because Japanese happened to be installed |

Both sessions used the corrected quiet/noisy thresholds (load 10 to start, 12 or above is noisy; the original 3/5 never let a session start on this Mac's idle load of about 8). 7 of session 1's 36 ladder runs, all mid-size, were flagged noisy and left out of the statistics; none of the sizes that set the limit (18,200 and 19,100) were affected. No size disagreed between the two sessions by more than about 4%, well under the 20% threshold that would have required using the smaller figure.

Per-size medians (min–max), not-noisy runs only:

| Pair | 400 runes | 1,600 runes | 6,400 runes | 20,000 runes |
|---|---|---|---|---|
| en>zh-Hans | 4.7 s (4.6-6.0) | 14.9 s (14.8-16.2) | 53.3-53.5 s | 160.9 s |
| zh-Hans>en | 12.1-12.5 s | 41.4-41.6 s | 162.7-163.7 s | (boundary search took over, see below) |
| en>es | 4.9-5.0 s | 15.0-15.7 s | 55.0-56.3 s | not tried |
| ja>en | 10.9 s (session 2 only) | 35.8 s (session 2 only) | 137.8 s (session 2 only) | not tried |

CJK boundary search (session 1, single runs each): 12,800 runes passed (326.4 s), 16,400 passed (411.5 s), 18,200 passed twice (451.3 s and 457.6 s), 19,100 failed twice (477.3 s and 479.3 s, 0 output bytes, projected 67,187 bytes against the 65,535-byte buffer). The search stopped there: the failure is inside the #111 bracket, so no further bisection was needed.

**Chunk size for #84 (a doc figure, not a new constant).** Median latency is about 8-9 ms per rune for Latin scripts (en>zh-Hans, en>es) and about 22-31 ms per rune for CJK and Japanese source text (zh-Hans>en, ja>en). A chunk that should feel like progress rather than a long silent wait, about 30 s of work on this machine, is roughly 3,500 Latin runes or 1,200 CJK/Japanese-source runes. Neither is a round number the data argues for over the other; #84's brief should cite this paragraph rather than a new constant.

## How to re-run the apple probe (#119)

The probe (`TestProbeApple` in `internal/engine/enginelimits/probe_test.go`) goes through `engine.NewApple()`. Each session runs a fixed plan, cheapest first, so the time cap cuts the least valuable data:

- **Phase 1, ladder** (the spread and the latency curve): `en>zh-Hans` at 400, 1,600, 6,400 and 20,000 runes; `zh-Hans>en` and `en>es` at 400, 1,600 and 6,400. Every size runs `KAI_ENGINE_PROBE_REPEATS` times (default 3; fewer is refused).
- **Phase 2, boundary** (`zh-Hans>en` only, single runs): 12,800 runes, then the #111 sizes (16,400, 18,200, 19,100, 20,000) until one fails, then bisection until the bracket is within 5% of the passing size (at least 50 runes). When the answer lands inside the #111 bracket (18,200 passed, 19,100 failed), its two points run once more if the budget allows.
- **Phase 3, Japanese** (optional): the `ja>en` ladder at 400, 1,600 and 6,400 runes, only when Japanese is installed and the session so far took under 60 minutes. The probe never installs a language.

A session is capped at `KAI_ENGINE_PROBE_BUDGET_MIN` minutes (default 90). A point starts only if the elapsed time plus 1.25 times its projected time fits the cap. The projection is the point's size times the median milliseconds per rune so far for its pair. A point in flight is never cut off. At the cap the probe prints `PROBE budget_exhausted` with what it left out, and the session is partial. Between two runs that completed, the probe pauses 5 s. After an error it waits until a one-sentence request comes back in under 5 s, or until 60 s pass (`queue_drain=timeout`). It never cancels a measured run.

1. Build the bridge: `(cd pkg/swiftbridge/scripts && bash ./build.sh)`. The build is deterministic, and the `PROBE host` line records the dylib's sha256: both sessions must show the same one.
2. Install English, Chinese (Simplified) and Spanish (Japanese is optional) under System Settings > General > Language & Region > Translation Languages. The probe stops with the cause if a required one is missing.
3. Quiet the Mac: nothing else running, and no other kai or pipeline session. The probe records the load average (`vm.loadavg`) at the start, at the start of every run and at the end. A session waits up to five minutes for a 1-minute load average of 10 or below, and stops if the Mac stays busy. A run that starts above 12 is marked `noisy` and left out of the statistics; its line is kept.
4. From the repository root: `KAI_ENGINE_PROBE=1 CGO_ENABLED=1 caffeinate -is go test -tags enginelimits -ldflags=-linkmode=external -run TestProbeApple -timeout 115m -count=1 -v ./internal/engine/enginelimits/ 2>&1 | tee <scratch>/probe-<session>.log`. The timeout is the 90-minute budget, plus the 20-minute ceiling of a point that starts just inside it, plus slack. Record a second session at least an hour after the first one ended, with `KAI_ENGINE_PROBE_SKIP_BOUNDARY=1` (about 30 minutes). A first data point (52 minutes on a loaded machine, not verified) is in `git show 9c575ad^:docs/handoffs/111/handoff-F.md`, section "Data for #119".
5. Put the result in the apple row of `internal/engine/input_budget.go` and of this page (Latin and CJK maximums, latency, host, date; `Verified` yes, `SourceMeasured`, follow-up gone). The limit is the smaller of the two maximums, the `apple_limit_runes` line the probe prints last. Then take the provisional-row wording out of "Reading the table" here and out of the last paragraph of `internal/engine/enginelimits/doc.go`; no test checks that prose.
6. After any change to the bridge's cancel path, run the cancel checks (command above, about two minutes).

The knobs are environment variables, read only by the gated test:

| Variable | Default | Effect |
|---|---|---|
| `KAI_ENGINE_PROBE_REPEATS` | 3 | runs per ladder size; fewer than 3 is refused |
| `KAI_ENGINE_PROBE_BUDGET_MIN` | 90 | the session's cap in minutes; with a larger cap, raise the go test timeout to the cap plus 25 minutes |
| `KAI_ENGINE_PROBE_MAX_RUNES` | no bound | drops every planned size above it; `1000` is the smoke run (the nine 400-rune points, about 3 minutes), which records the load but does not wait for a quiet Mac (any bound above 1,000 is a real capped session and waits like any other) |
| `KAI_ENGINE_PROBE_SKIP_BOUNDARY` | off | `1` leaves out phase 2 |
| `KAI_ENGINE_PROBE_STATUS` | `kai-probe-status.jsonl` in the temp dir | the progress file; the `PROBE config` line prints its path |

**Progress file.** After every run the probe appends one JSON line with these fields: `time` (RFC 3339, with the zone), `elapsed_s`, `phase`, `pair`, `runes`, `utf8_bytes`, `repeat`, `latency_ms`, `out_bytes`, `kind`, `load1`, `next` (the next planned point) and `budget_left_s`. It appends one more line with `phase` `budget_exhausted` when the cap stops the run, and a last one with `phase` `done`. Run `tail -f` on the file to follow a session. The file lives outside the repository, so it is never committed.

**Log lines.** A session opens with:

- `PROBE host`: macOS version and build, chip, cores, memory, Go, bridge sha256.
- `PROBE config`, `PROBE load` (at the start), `PROBE precheck` and `PROBE plan`.

Each run then prints one `PROBE point` line. It carries the #111 keys plus `pair`, `phase`, `repeat`, `load1` and `noisy`; `runes` counts runes and `utf8_bytes` counts bytes. A `PROBE settle` line comes between runs, with a `PROBE warm` line for each drain request after an error. `PROBE boundary` marks the end of phase 2, and `PROBE phase3 skipped=true` a phase 3 left out at the 60-minute mark. At the end:

- a `PROBE summary` per size: every latency, and min, median and max over the runs that are not noisy;
- a `PROBE result` per pair: the largest size at which every run that is not noisy passed;
- a `PROBE failure` per failed run: its output bytes, projected from the pair's passing runs, against the 65,535-byte buffer;
- `PROBE load` (at the end), and `PROBE apple_limit_runes` last.

The probe and the cancel checks are opt-in on purpose: they need the build tag `enginelimits` and `KAI_ENGINE_PROBE=1`, and they are not part of CI, the Makefile, the Taskfile or the pipeline's test command. They need external linking and a main-thread run loop (the `TestMain` in the probe parks the main OS thread in `dispatch_main`); without them a translation never returns. The probe checks the prerequisites first, puts a ceiling of its own on every call, and stops with the cause instead of reporting a limit.
