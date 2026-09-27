# Apple translation quality vs. input size

Epic #151: does Apple's Translation.framework's *quality* degrade before it *fails*? #119
(`docs/engine-limits.md`) measured where Apple fails on large input (18,200 runes, bound by the
64 KiB output buffer, #106); it never checked whether a translation that still succeeds is any
good. This page is #152/#153's tooling and, once the real run has happened, its results.

**Status (2026-09-27): measured.** Both sessions have run — #152 (structural + round-trip, no
DeepL) and #153 (with `KAI_ENGINE_PROBE_DEEPL=1`) — and their results are in the "Results" section
below. No degradation was found on any signal, at any size up to 18,200 runes.

## What the probe measures

`TestProbeQuality` (`internal/engine/enginelimits/quality_probe_test.go`) reuses #119's session,
script, prober and script infrastructure (`internal/engine/enginelimits/probe_test.go`) and its
pure planning helpers (`plan.go`). It is opt-in exactly like `TestProbeApple`: the `enginelimits`
build tag (darwin only) and `KAI_ENGINE_PROBE=1`. It never runs in CI, the Makefile, any Taskfile,
or the authoritative test command (`TestProbeNotWiredIntoSuiteOrCI`,
`TestQualityProbeRunGuideTimeoutAndKnobs` in `guard_test.go` check this).

The plan (`buildQualityPlan`) is #151's epic scope: all four of #119's pairs — `en>zh-Hans`,
`zh-Hans>en`, `en>es`, `ja>en` (ja>en only when Japanese is installed) — at the same six sizes
#119 already measured latency at: 400, 1,600, 6,400, 12,800, 16,400 and 18,200 runes. One run per
point, not #119's three: this measures a quality trend across size, not latency variance.

At each point:

1. **Forward translate with Apple** (`prober.call`, the same code path `TestProbeApple` uses:
   `engine.NewApple()` behind the `engine.Translator` interface). Run `checkStructure`
   (`plan.go`) on the result: it extends #119's `firstMissingMarker` paragraph check with three
   more defects, each a known NMT failure mode at scale —
   - **dropped**: a paragraph marker present in the input, absent from the output;
   - **reordered**: markers present, but not in the input's order;
   - **repeated**: a sentence or a paragraph that appears once in the input and more than once in
     the output.

   A `PROBE quality structural` line logs the verdict for every point; when a defect fires, a
   `PROBE quality structural_detail` line follows it with the actual repeated text and an output
   sample, so a human (the operator, reading the log afterward) can judge fluency directly instead
   of trusting the boolean alone.

2. **Round-trip drift**: translate the forward result back to the source language (a second Apple
   call) and score it against the *original* input with `similarity` (`plan.go`) — 1 minus the
   Levenshtein edit distance between the two strings' runes, normalized by the longer one's rune
   count (1.0 identical, 0.0 for two strings with nothing in common at that length; deterministic,
   no LLM judge, free). Logged as `PROBE quality roundtrip`. A knee in the score-vs-size curve
   marks where quality starts to degrade, even though it can't say what specifically went wrong —
   that's what the structural check and the direct read are for.

3. **DeepL reference** (`PROBE quality deepl_divergence`, issue #153): only when
   `KAI_ENGINE_PROBE_DEEPL=1`. Off (the default), #152's checks above cost nothing and make no
   DeepL call. On, the same source text is sent to DeepL (`engine.NewDeepL`, config loaded the
   same way `internal/service/engine_wrapper.go` and `internal/translate/service.go` load an
   engine: `configstore.Open` against the real `~/.kai.dev/data/config.db` — or `~/.kai/data/`
   for a non-dev build — then `GetEngineByName(ctx, "deepl")`), and Apple's forward result is
   scored against DeepL's with the same `similarity` helper. A growing gap as size increases, in
   the same place the Apple-only signals move, is corroborating evidence rather than a single
   probe's noise (#153's own framing) — DeepL is not ground truth here.

Every point also appends a line to the progress file (same `statusLine`/`appendStatus` as #119,
`phase="quality"`), so a long session can be tailed the same way.

## How to re-run

Build the bridge, then from the repository root on a quiet Mac:

```sh
(cd pkg/swiftbridge/scripts && bash ./build.sh)

KAI_ENGINE_PROBE=1 CGO_ENABLED=1 caffeinate -is go test -tags enginelimits -ldflags=-linkmode=external -run TestProbeQuality -timeout 115m -count=1 -v ./internal/engine/enginelimits/
```

Add `KAI_ENGINE_PROBE_DEEPL=1` to also run #153's DeepL reference calls (needs a DeepL API key
configured in Kai's own Settings first; nothing new to wire). **Cost estimate (#153):** 400 +
1,600 + 6,400 + 12,800 + 16,400 + 18,200 = 55,800 source characters per pair, four pairs, one run
each = ~223,200 characters for a full session; DeepL's Pro pricing has historically run near
$25 / 1,000,000 characters, so roughly $5.58 per full session (confirm the account's current rate
before running; #153 explicitly did not verify current pricing against a live source).

The knobs, environment variables read only by `probeConfigFromEnv` (`plan.go`), after the gate:

	KAI_ENGINE_PROBE_BUDGET_MIN     the session's wall-clock cap in minutes (default 90); with a
	                                larger cap, raise the go test -timeout to the cap plus 25 minutes
	KAI_ENGINE_PROBE_MAX_RUNES      drops every planned size above it; 1000 is the smoke run (the
	                                four 400-rune points, forward + round trip only — no DeepL call
	                                is worth making at that size, but the KAI_ENGINE_PROBE_DEEPL gate
	                                still applies), which records the load but does not wait for a
	                                quiet Mac
	KAI_ENGINE_PROBE_STATUS        the progress file (default kai-probe-status.jsonl in the temp
	                                dir, the same default and file #119's probe writes to — point
	                                KAI_ENGINE_PROBE_STATUS at a different path to keep the two runs'
	                                lines apart if both run in the same session)
	KAI_ENGINE_PROBE_DEEPL          1 turns on #153's DeepL reference calls (default 0: off, no
	                                DeepL call, no cost)

`KAI_ENGINE_PROBE_REPEATS` and `KAI_ENGINE_PROBE_SKIP_BOUNDARY` are #119's own knobs
(`TestProbeApple`'s ladder repeats and boundary bisection); the quality probe reads
`probeConfigFromEnv` too but never repeats a point and has no boundary phase, so those two have no
effect on it.

**Budget note.** A quality point costs roughly double a plain probe point's Apple latency (forward
call plus round trip), and #119's own CJK points at 18,200 runes already took about 450 s
one-way. The default 90-minute budget is very likely too short for the full ladder x round trip
across all four pairs — expect this to need multiple sessions, the same way #119 did (see
`docs/engine-limits.md`'s "Apple, measured" section for how #119's two sessions were combined).
Raise `KAI_ENGINE_PROBE_BUDGET_MIN` (and the `-timeout` to match: cap plus 25 minutes) for a
longer single session, or simply re-run and let the plan resume from wherever the log shows it
stopped — `buildQualityPlan` is deterministic and cheapest-first, so a second run covers the next
untested points first.

## Reading the log

- `PROBE quality_plan points=N include_ja=<bool> deepl=<bool>` — the session's plan size and the
  two runtime decisions (Japanese installed, DeepL on).
- `PROBE quality structural pair=... runes=... paragraphs=... dropped=[...] reordered=<bool> repeated_sentences=N repeated_paragraphs=N latency_ms=...` —
  one line per point's structural verdict.
- `PROBE quality structural_detail ...` — only when a defect fired: the actual repeated text and a
  sample of the output, for a human to read.
- `PROBE quality roundtrip pair=... runes=... score=<0..1> fwd_latency_ms=... back_latency_ms=...` —
  the round-trip drift score against the original input.
- `PROBE quality deepl_divergence pair=... runes=... score=<0..1>` — only with
  `KAI_ENGINE_PROBE_DEEPL=1`: Apple's forward result scored against DeepL's.
- `PROBE quality_error pair=... runes=... stage=forward|roundtrip|deepl error=...` — an Apple or
  DeepL call failed at that point; the point's other lines are skipped, the session moves on.
- `PROBE quality_budget_exhausted elapsed_s=... remaining="..."` — the session stopped at the
  time cap; `remaining` lists what it did not get to.
- `PROBE quality_done elapsed_s=... partial=<bool> load1=...` — the session's end.

## Results

Both sessions ran on the same Mac and bridge build as #119: Apple M1 Max, macOS 27.0 (26A428),
10 cores, 32 GB, Go go1.27.1, bridge sha256 `3cf6379b06fdf3226ba29bbe783ce714af3d391e969aa118ed2e3371b5dfc2f2`.

| Session | Date (MDT) | What it ran | Elapsed | Result |
|---|---|---|---|---|
| 1 | 2026-09-27 | `TestProbeQuality`, `KAI_ENGINE_PROBE_DEEPL` off | 7430 s (~2h04m) | PASS, not partial, full 24-point plan |
| 2 | 2026-09-27 | `TestProbeQuality`, `KAI_ENGINE_PROBE_DEEPL=1` | 7508 s (~2h05m) | PASS, not partial, full 24-point plan |

### Structural integrity — 48 real translations checked, zero defects

Every one of the 24 points, run twice (once in each session, 48 real Apple translations total):
`dropped=[]`, `reordered=false`, `repeated_sentences=0`, `repeated_paragraphs=0`. No paragraph was
ever dropped, no marker was ever reordered, and no sentence or paragraph ever repeated beyond what
`selfRepeatFraction`'s margin already tolerates for the probe's own legitimate sentence-pool
cycling at large sizes. **No structural degradation was found at any size, on any pair, up to
18,200 runes.**

### Round-trip drift (#152, Apple only)

Score against the original input after translating there and back (1.0 = identical text
returned; not an accuracy percentage — see "What the probe measures" above for why a perfect
translation still scores below 1.0). None of the four pairs shows a decline as size grows; three
trend flat-to-slightly-improving, one (`ja>en`) is flat after an initial jump from 400 to 1,600:

| Pair | 400 | 1,600 | 6,400 | 12,800 | 16,400 | 18,200 |
|---|---|---|---|---|---|---|
| en>zh-Hans | 0.7698 | 0.7719 | 0.7978 | 0.8092 | 0.8109 | 0.8105 |
| zh-Hans>en | 0.7000 | 0.7231 | 0.7263 | 0.7270 | 0.7270 | 0.7278 |
| en>es | 0.8700 | 0.8797 | 0.8731 | 0.8721 | 0.8736 | 0.8730 |
| ja>en | 0.5637 | 0.6082 | 0.6066 | 0.6073 | 0.6081 | 0.6089 |

`ja>en`'s lower baseline (around 0.61 versus 0.73–0.87 for the other pairs) is consistent with
Japanese and English having looser structural correspondence than the other language pairs here,
not a size effect — it is flat across the whole range, same as the rest.

### DeepL divergence (#153)

Apple's forward result scored against DeepL's own translation of the same source text. 20 of 24
points succeeded; the other four are a real DeepL constraint, not a probe failure (see below).
None of the four pairs shows a decline as size grows:

| Pair | 400 | 1,600 | 6,400 | 12,800 | 16,400 | 18,200 |
|---|---|---|---|---|---|---|
| en>zh-Hans | 0.6349 | 0.6790 | 0.6682 | 0.6644 | 0.6774 | 0.6736 |
| zh-Hans>en | 0.8258 | 0.8458 | 0.8608 | 0.8604 | *(DeepL rejected)* | *(DeepL rejected)* |
| en>es | 0.8147 | 0.8549 | 0.8483 | 0.8442 | 0.8473 | 0.8480 |
| ja>en | 0.8536 | 0.8484 | 0.8512 | 0.8488 | *(DeepL rejected)* | *(DeepL rejected)* |

Divergence scores sit lower than #152's own round-trip scores for the same pairs (`en>zh-Hans`
especially, ~0.65–0.68 versus ~0.77–0.81) — expected, not concerning: agreeing with an
independent translator's own phrasing choices is a harder bar than a translator agreeing with
itself after two passes. Total DeepL spend for the successful points: 154,000 characters (the
20 successful calls only; a rejected call is not billed). The DeepL account's own dashboard is the
final word on cost, since this figure does not include whatever billing-side accounting DeepL
applies to a rejected request.

**Finding: DeepL rejects large CJK-source requests that Apple accepts.** Both CJK-source pairs
(`zh-Hans>en`, `ja>en`) failed at 16,400 and 18,200 runes with `DeepL: API error Payload too
large.` — four times, consistently, not a fluke. This lines up with `docs/engine-limits.md`'s own
documented figure for DeepL: its 128 KiB (131,072-byte) request-body limit, applied to
query-escaped text where a CJK character costs about 9 bytes. 16,400 Chinese or Japanese
characters, query-escaped, comes to roughly 147,600 bytes — past DeepL's limit. Apple has no such
constraint at this size (#119, #152 above): **this is a real difference between the two
translators, not a defect in Apple, in DeepL, or in this probe.** It also means DeepL cannot serve
as a full reference for #84's chunker at Apple's own measured ceiling for CJK source text; a
chunker that wants to compare-and-fall-back between the two engines would need to chunk well below
16,400 runes for a CJK source regardless of which engine sends first.

### Conclusion

Across two independent signals (a deterministic round-trip check and a second translator's
independent output), on four language pairs, at every size from 400 up to Kai's actual production
ceiling of 18,200 runes (#119, buffer-bound by #106): **no evidence that Apple's translation
quality degrades as input size grows.** The structural check — the direct test for the kind of
degenerate output (dropped, reordered, or repeated text) that motivated this epic — found zero
defects in 48 real translations. This does not establish Apple's translations are *accurate* in an
absolute sense (see "What the probe measures": neither signal is an accuracy percentage), only
that whatever quality Apple has at 400 runes, it still has at 18,200 runes.

Open, not settled by this data: whether `en>zh-Hans` and `en>es` (the two pairs where #119 never
found Apple's own failure point — see `docs/engine-limits.md`) would show degradation somewhere
past 18,200 runes. This probe deliberately stopped at Kai's actual usable ceiling, since testing
further would measure something the app cannot send today; if #106 raises the output-buffer limit
in the future, this ladder would be the template for re-checking the new range.
