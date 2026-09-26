# Engine input limits

Where each translation engine breaks on large input, and the input budget the chunker (#84) works to. Issue #83, child of epic #86. The bridge's fixed 20 s wait on apple was removed for #111, so the apple row is provisional again until #119 measures it.

The budget lives in code: `internal/engine/input_budget.go` (`InputBudget`, `Budget.Max`). This page is its written record and is checked against it. `TestEngineLimitsDocListsEveryTranslator` fails when a translator has no row here, or when a row's Verified, Follow-up or link cell disagrees with the table in code.

**Status.** No row is measured now. The first apple measurement (#83) mostly measured the bridge's own 20 s wait, which #111 removed, so apple is a provisional figure like the others: `Verified` is `no` and the follow-up issue in the last column (#119 for apple) measures it.

## Reading the table

- **Unit** is what the engine limits, and what the budget counts. `runes` is `utf8.RuneCountInString(text)`. `utf8 bytes` is `len(text)`. `query-escaped bytes` is `len(url.QueryEscape(text))`, which is what a URL or form value costs on the wire (a CJK character is 9).
- **Latin max** and **CJK max** are the largest inputs seen to pass, in runes: English to Chinese (Latin) and Chinese to English (CJK). In a measured row they come from a probe run on a recorded host; the provisional apple row's come from the #111 throwaway checks (see "Apple, provisional"). `>= N` means N runes was the largest size seen to pass, not a ceiling.
- **Budget (80%)** is what a chunker may send: the limit times `BudgetMarginPercent` (80), rounded down, in the row's unit. For a measured apple row the limit is the smaller of the Latin and CJK maximums. The provisional apple row does not follow that rule: its limit (3200) is the largest Latin size that passed in the #111 throwaway checks, and its CJK maximum (`>= 1000`) is only a lower bound, from one check at 1000 runes, so it does not set the limit. The smaller of the two would give a budget of 800, not 2560. #119 measures both searches, and from then on the rule applies to the row.
- **Verified**: `yes` means measured by a probe run on a recorded host (none are, now). `no` means a documented or provisional figure that has not been checked against the live service.

## Limits per engine

| Engine | Unit | Latin max | CJK max | Failure past the limit | Latency at max | Documented limit | Budget (80%) | Verified | Date | Follow-up |
|---|---|---|---|---|---|---|---|---|---|---|
| apple | runes | >= 3200 | >= 1000 | none seen | Latin 31.8 s at 3200, CJK 36.3 s at 1000 (throwaway checks, no wait) | none found in Apple's Translation documentation (checked 2026-09-26); Kai's bridge has no wait of its own since #111 and a 65,535-byte output buffer (`apple_darwin.go`) | 2560 runes | no | - | #119 |
| google | query-escaped bytes | unmeasured | unmeasured | unmeasured | unmeasured | undocumented for the gtx endpoint Kai calls; nearest published figure is Cloud Translation's recommended maximum of 5K characters (code points) per request ([quotas](https://docs.cloud.google.com/translate/quotas)) | 4000 query-escaped bytes | no | - | #87 |
| deepl | query-escaped bytes | unmeasured | unmeasured | unmeasured | unmeasured | request size limit of 128 KiB for the whole request ([API reference](https://developers.deepl.com/api-reference/translate/request-translation)) | 104857 query-escaped bytes | no | - | #88 |
| openai | runes | unmeasured | unmeasured | unmeasured | unmeasured | no input limit binds, the output cap does; Kai sets none, so the model default applies; 8192 output tokens is the assumed basis ([API reference](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)) | 3276 runes | no | - | #92 |
| anthropic | runes | unmeasured | unmeasured | unmeasured | unmeasured | no input limit binds, the output cap does; Kai sets `MaxTokens` to 8192 in `anthropic.go` ([Messages API](https://platform.claude.com/docs/en/api/messages)) | 3276 runes | no | - | #93 |
| gemini | runes | unmeasured | unmeasured | unmeasured | unmeasured | no input limit binds, the output cap does; Kai sets `MaxOutputTokens` to 8192 in `gemini.go` ([API reference](https://ai.google.dev/api/generate-content)) | 3276 runes | no | - | #94 |
| baidu | utf8 bytes | unmeasured | unmeasured | unmeasured | unmeasured | 6000 bytes per request, about 2000 Chinese characters ([API doc](https://fanyi-api.baidu.com/doc/21)) | 4800 utf8 bytes | no | - | #89 |
| tencent | runes | unmeasured | unmeasured | unmeasured | unmeasured | text length below 2000, unit not stated, read as characters ([SDK doc comment](https://pkg.go.dev/github.com/tencentyun/tencentcloud-sdk-go/tencentcloud/tmt/v20180321)) | 1600 runes | no | - | #90 |
| youdao | runes | unmeasured | unmeasured | unmeasured | unmeasured | 5000 characters per query ([API doc](https://ai.youdao.com/DOCSIRMA/html/trans/api/wbfy/index.html)) | 4000 runes | no | - | #91 |

## Apple, provisional

- **Time does not bind any more.** Since #111 the bridge waits for Apple with no timer (`job.sema.wait()` in `pkg/swiftbridge/internal/swift/apple_translate.swift`); a request runs to the end or until the user cancels it. The old 20 s wait produced the first numbers (2250 and 650 runes), so they were the wait, not the framework. The provisional 3200 is the largest size seen to succeed in throwaway checks with no wait, not a ceiling.
- **The 64 KiB output buffer is expected to bind** for text that grows when translated (Chinese to English grows about 3.5 times in bytes); #119 measures where. #106 owns the buffer.
- **Cancel frees Kai, not the framework.** A cancelled call returns at once, but Apple keeps working on the abandoned text and answers the next request only when it is done (see "Cancel checks"). Nothing in Kai can shorten that, so the chunker (#84) should keep Apple chunks well below `Max()`: a few thousand Latin runes or about a thousand CJK runes is roughly half a minute of work on an M1 Max.

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

## How to re-run the apple probe (#119)

1. Build the bridge: `(cd pkg/swiftbridge/scripts && bash ./build.sh)`.
2. Install English and Chinese (Simplified) under System Settings > General > Language & Region > Translation Languages.
3. Keep the Mac awake and otherwise idle (`caffeinate -is`). Every point costs its full translation time, so other work makes the run longer and noisier.
4. From the repository root: `KAI_ENGINE_PROBE=1 CGO_ENABLED=1 go test -tags enginelimits -ldflags=-linkmode=external -run TestProbeApple -timeout 3h -count=1 -v ./internal/engine/enginelimits/`. `KAI_ENGINE_PROBE_MAX_RUNES=1000` gives a short smoke run. A first data point (52 minutes, not verified) is in `docs/handoffs/111/handoff-F.md`, "Data for #119".
5. Put the result in the apple row of `internal/engine/input_budget.go` and of this page (Latin and CJK maximums, latency, host, date; `Verified` yes, `SourceMeasured`, follow-up gone). The limit is the smaller of the two maximums, the `apple_limit_runes` line the probe prints last. Then take the provisional-row wording out of "Reading the table" here and out of the last paragraph of `internal/engine/enginelimits/doc.go`; no test checks that prose.
6. After any change to the bridge's cancel path, run the cancel checks (command above, about two minutes).

The probe and the cancel checks are opt-in on purpose: they need the build tag `enginelimits` and `KAI_ENGINE_PROBE=1`, and they are not part of CI, the Makefile, the Taskfile or the pipeline's test command. They need external linking and a main-thread run loop (the `TestMain` in the probe parks the main OS thread in `dispatch_main`); without them a translation never returns. The probe checks the prerequisites first, puts a ceiling of its own on every call, and stops with the cause instead of reporting a limit.
