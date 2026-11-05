# Speeding up Apple's Translation framework: Grok's research from X

Kept for issue #10 (long text on the System engine takes about ten minutes).

**Provenance.** This is Grok's answer, pasted unchanged into the session on 2026-09-30 (a share link was not provided). The prompt is reproduced first so the answer can be read against what was asked.

**Reliability.** Grok reports what it found in posts on X and in web pages. Apart from the checks in the last section of this file, nothing in its answer has been verified, and none of the X posts or pages were opened when this was saved. Treat the people, tools and figures as leads. Its one strong claim, that the speed depends on a strategy switch added in macOS 26.4, was checked on this Mac the same day; see "Checked on this Mac" at the end.

Where we used it: the build order and the checklist in #10.

---

## The prompt that was given to Grok

You are researching how to make Apple's Translation framework (the on-device translator behind Apple's Translate app and the `Translation` Swift framework) translate long text faster on macOS. Search X (Twitter) and the web for what developers and users have actually found, not marketing claims. I want first-hand reports, measurements, and workarounds.

Context: I maintain a macOS menu-bar translation app written in Go and Svelte, with a Swift bridge that calls `TranslationSession(installedSource:target:)` and `session.translate(_:)`. macOS 27 is the target, Apple silicon. Long text is split into parts of about 3,500 characters and translated one part at a time.

What I measured on an Apple M1 Max (idle Mac), English to Spanish (Mexico):
- Speed is about 4 to 6 ms per character for a single call, roughly 6 seconds for 1,500 characters, about 19 seconds for 3,400 characters of real prose. In the app, under heavier load, I saw about 11 ms per character.
- Sending 2 or 4 requests at once, on the same language pair or on different pairs, gives no speedup (0.93x to 1.15x). Apple serializes them.
- The batch calls `translations(from:)` and `translate(batch:)` give no speedup either: 24.1 s for four 1,500-character texts in one batch, against 23.2 s for four separate calls. The streamed form only returns each part as it finishes.
- Output text from the batch call was identical to the separate calls.

Questions to answer, with links to the posts or pages that support each answer:
1. Is there any known way to make TranslationSession faster: a hidden option, a different initializer, a different API, a configuration, a lower-latency mode, a smaller model, or an Apple Intelligence route (Foundation Models, SystemLanguageModel) that translates faster than Translation.framework?
2. Does translation speed change with input size in a way that favours smaller parts or larger parts? Does a very long single call run faster per character than many short ones, or slower?
3. What slows it down: the Mac's load, power mode (Low Power Mode), thermal state, running on battery, the Neural Engine being busy, memory pressure, the language pair, or first-call preparation? What has anyone measured?
4. Do other apps get faster results (DeepL-style apps that use Apple's engine, Apple's own Translate app, Safari page translation, Live Translation)? Are they using a different API, a private framework, or a different model? Anything about `_LTTranslator`, `LanguageTranslation`, `TranslationUIProvider`, or the system translation extension?
5. Is there a way to run several translations truly in parallel, such as separate processes, separate app bundles, XPC services, or separate sessions created from different threads? Has anyone measured that, and what happened?
6. Reports from macOS 26 and the macOS 27 betas about translation speed getting better or worse, or about new APIs, WWDC sessions, or release notes about translation performance.
7. Alternatives that keep the text on the Mac but run faster, for example small on-device models (MLX, Core ML, llama.cpp, Apple's Foundation Models) used for translation, with reported speed and quality against Apple's Translation framework.

How to report:
- For each question, say what people found, quote the number or claim in your own words, and give the link. Mark each item as measured, anecdotal, or speculation.
- Say plainly when you found nothing, and do not fill gaps with guesses.
- Finish with a ranked list of the three ideas most likely to give a real speedup for long text, each with the evidence behind it and how I could test it in under an hour.

---

## Grok's answer

The numbers you measured line up with Apple Intelligence “high fidelity” translation, not the old language-pack NMT. That is the main finding.

---

## 1. Is there a known way to make `TranslationSession` faster?

**Yes. There is a public strategy switch, added in iOS/macOS 26.4.** It is the only documented speed control.

Apple documents two strategies:

- `TranslationSession.Strategy.lowLatency` — “fast translations using traditional models,” less power, language packs must be downloaded. Apple points this at real-time audio.
- `TranslationSession.Strategy.highFidelity` — “more fluent translations using Apple Intelligence.” Models are already on disk when Apple Intelligence is on. “May take longer.” Extra languages. Falls back to traditional models if AI is off.

You set it on the session or the configuration:

```swift
TranslationSession(
  installedSource: source,
  target: target,
  preferredStrategy: .lowLatency
)
```

or `TranslationSession.Configuration(source:target:preferredStrategy:)`. Same value exists on `LanguageAvailability(preferredStrategy:)`.

**Measured.** The `trn` CLI (macOS 26.4+, wraps Translation.framework) timed the same README, 4,818 characters, English → Japanese:

| Quality | Wall time | ≈ ms/char |
|---|---|---|
| high | 39.777 s | 8.3 |
| low | 3.461 s | 0.72 |

About **11.5×**. A 1,006-character incident report: high 6.39 s vs low 0.68 s (**9.4×**). A Japanese notice: high 16.2 s vs low 0.97 s (**16.8×**). Single-run, one machine, debug binary.

Your idle M1 Max numbers (4–6 ms/char, 6 s for 1,500, 19 s for 3,400) sit on the **high** curve, not the low one.

**Anecdotal / SDK behavior.** Jay (@SlowbrewedS) reported that apps *built with the 26.4+ SDK* make `LanguageAvailability()` prefer Apple Intelligence models first. Older SDKs stay on traditional packs. This is compile-SDK, not deployment target. If your Swift bridge is built with a current Xcode, omitting `preferredStrategy` is a plausible way to land on highFidelity whenever Apple Intelligence is on. Fraze independently saw sessions translate pairs that had no language pack installed unless they forced `lowLatency` — consistent with the highFidelity path using the already-downloaded AI models.

**Nothing found** for: a hidden “turbo” flag, a smaller traditional model toggle, a lower-precision knob, or a SystemLanguageModel / Foundation Models route that Apple documents as a faster *drop-in* for Translation.framework. Foundation Models is a separate ~3B generative model with a 4,096-token window, not an MT engine.

`TranslationUIProvider` is the default-translation-app extension (XPC UI sheet). It does not expose a faster engine.

---

## 2. Does speed change with input size in a way that favors smaller or larger parts?

**Sparse first-hand data. No published curve of ms/char vs chunk size for Translation.framework.**

What exists:

- **Your own measurements (measured):** 1,500 chars ≈ 6 s (~4 ms/char) vs 3,400 chars ≈ 19 s (~5.6 ms/char) on one pair. Per-character cost is similar; the longer call is not cheaper. Batch of four 1,500-char texts ≈ four separate calls. So neither “one giant call” nor the official batch API bought throughput.
- **`trn` (measured, different pair/machine):** they default to **512-character** buffers, split on newlines. They say smaller buffers start sooner; larger ones keep more context. They did not publish a size-vs-ms/char table. Long-text tests with the 512-char default produced chunk-boundary artifacts (`rain.She`, broken fragments) in both quality modes.
- **Anecdotal.** A Japanese subtitle app batches Translation.framework in groups of **10 segments** because “throwing a large number of requests at once makes the framework unstable.” That is stability, not speed.
- WWDC24 said batch APIs are “best and most efficient” for many strings of the *same* language. That is Apple’s claim about API shape (one session, one language), not a measured throughput gain for long prose. Your batch test already falsifies a wall-clock win for long chunks.

**Nothing found** that a single multi-thousand-character call is faster per character than sentence- or paragraph-sized calls, or the reverse, except your data (roughly flat / slightly worse as size grows).

---

## 3. What slows it down?

**Documented by Apple, not measured by third parties for this API:**

- Model choice: highFidelity “may take longer”; lowLatency “faster … use less power.”
- First use of a *traditional* pair: language-pack download. `prepareTranslation()` only triggers that download; it is not a warmup for inference.
- First-call model load: `trn` notes timing “varies … whether the translation model is already loaded.” No numbers.

**Measured by you, not contradicted elsewhere:** concurrent same-process requests do not scale (0.93–1.15×). Under “heavier load” you saw ~11 ms/char vs 4–6 idle. That is the only load number I found.

**Anecdotal / adjacent:**

- `translation-rs` (Rust bindings): Translation.framework **finishes work on the main queue**. A blocked main run loop yields `MainRunLoopNotRunning` after 10 s; calls time out at 60 s. Tokio on the main thread starves it. This is a latency/hang risk, not a throughput knob.
- Apple Community (iOS 17 Translate app): “more than 10 seconds per word” — old cloud-path complaint, not the current on-device API.
- Forum note: requesting a new translation before the previous one returns produced “Refusing new translation request because text session has already been cancelled.” Serial session, not parallel workers.

**Nothing found** (no measurements) for: Low Power Mode, battery vs wall, thermal state, ANE busy with another model, memory pressure, or language-pair speed tables for Translation.framework. MacGeneration’s +15% “traduction automatique” figure is Geekbench AI on an M1 going from macOS 26.5.1 → 27 beta 2, not this framework.

---

## 4. Do other apps get faster results? Private APIs?

**Public apps that wrap the same framework do not claim a faster private engine.**

- Apple’s own Translate / Live Translation / SpeechAnalyzer path: Fraze explicitly requests **`lowLatency`** because that is the strategy Apple documents for real-time audio. They treat highFidelity as slower.
- `trn` and Arthur-Ficial/`translate` (DeepL-compatible local HTTP server): same Translation.framework. `trn` is fast only when `--quality low`.
- Sentence-scale claim from a 2026 video-app guide: “50–200 ms per sentence on modern Apple silicon.” That is marketing-adjacent, not a long-prose benchmark, and matches lowLatency sentence work, not your 6–19 s chunks.
- Safari page translation (2024 reverse-engineering on Michael Tsai’s blog): soft-links **private** `TranslationUIServices`, classes `LTUISourceMeta`, `LTUITranslationViewController`. No speed numbers, and no evidence it is a different MT model than the shared on-device packs WWDC said are “shared with all apps … including the Translate app.”
- `_LTTranslator`, `LanguageTranslation`: **no first-hand posts or measurements found** in this search. `TranslationUIProvider` is the third-party default-app UI extension, not a faster backend.

MakeUseOf notes Siri and Safari can still use **cloud** translation even when the Translate app is set to on-device. That is a different path, not available as a public faster on-device API.

---

## 5. Can several translations run truly in parallel (processes, bundles, XPC, threads)?

**No published measurement of multi-process Translation.framework showing a speedup.**

What people actually reported:

- **Same process, multiple tasks / batch API:** you measured ~1×. Matches a single serialized system service.
- **`trn --concurrency 4`:** the CLI *issues* up to four chunk requests and preserves output order. There is no published proof those four run on four Neural Engine contexts. Given your 2- and 4-way test, treat this as request pipelining, not parallel compute.
- **Threads:** `translation-rs` says the framework still completes on the **main queue**. Extra threads do not give extra translators.
- **SwiftUI session lifetime:** storing a session after its view dies is fatal; overlapping requests on one session get cancelled. That pushes people toward one live session, not a pool.
- **XPC / separate app bundles / two processes:** **nothing found** that anyone measured this for Translation.framework. I am not filling that gap.

---

## 6. macOS 26 / 27: speed, new APIs, WWDC, release notes

**macOS 26.4 / iOS 26.4 (measured in docs + first-hand notes):**

- New `TranslationSession.Strategy` (`lowLatency` / `highFidelity`).
- New `init(..., preferredStrategy:)`, `Configuration.preferredStrategy`, `LanguageAvailability(preferredStrategy:)`.
- AttributedString translate overload.
- SDK-default shift toward Apple Intelligence models (Jay’s report).

**macOS 27 / WWDC26:**

- No WWDC session or release-note line found that says “Translation.framework is faster.”
- macOS 27 release notes talk general responsiveness, Spotlight, AirDrop — not this API.
- MacGeneration: Geekbench AI **machine-translation subscore +15%** on M1, 26.5.1 → 27 beta 2, Neural Engine path. Adjacent hardware/runtime, not a framework changelog.
- Foundation Models in 27: new AFM 3 Core, LanguageModel protocol, MLX/CoreAI backends, 4k context. That is generative AI, not a documented Translation.framework replacement.
- Live Translation shipped with Tahoe (macOS 26) as an Apple Intelligence feature in Messages/FaceTime/Phone, not as a faster public text API.

X was thin. The useful posts are Vistar on 26.4 Strategy + AttributedString, and Jay on the SDK default flip.

---

## 7. On-device alternatives that stay on the Mac

| Approach | Speed reports | Quality vs Apple Translation | Notes |
|---|---|---|---|
| Translation.framework **lowLatency** | **Measured:** ~0.7 ms/char on `trn` EN↔JA; ~10× vs high | `trn` found low “sufficient” on short EN/JA, worse names/roles; high safer on long notices | Same API you already call |
| Translation.framework **highFidelity** / default on 26.4+ SDK | **Measured:** 6–8 ms/char (`trn` + your M1 Max) | Apple: “more fluent” | Looks like what you have now |
| SystemLanguageModel / Foundation Models as a translator | **Anecdotal:** Pre-Babel Lens on M4 Air “quite fast,” DeepL-like two-pane. No ms/char vs Translation.framework | Generative, 4096-token window; not an MT model | Separate framework; needs Apple Intelligence on |
| Dedicated small MT LLMs (HY-MT 1.8B, TranslateGemma via MLX) | **Measured (other apps):** TranslateGemma long text ~45 chars/s on M2 Max (≈22 ms/char) — *slower* than your current highFidelity and much slower than lowLatency. HY-MT used to break a 3–5 s speech-translation latency wall, not for 3k-char documents | Task-tuned; variable | Stay on-device; you already have the silicon for this |
| Firefox/Mozilla WASM MT | Old bug about model *download* time on Mac vs Linux, not inference vs Apple | Different engine | Not Apple’s models |

No head-to-head “Foundation Models vs Translation.framework on the same 3,400-character EN→es-MX passage” was published in what I searched.

---

## Ranked: three ideas most likely to give a real speedup, testable in under an hour

### 1. Force `preferredStrategy: .lowLatency` (highest confidence)

**Why.** Your 4–6 ms/char matches `trn`’s highFidelity curve. Apple and `trn` both say lowLatency is the fast traditional model. The 26.4+ SDK default is a plausible reason you never opted into it.

**Test (~15 min).** In the Swift bridge only:

```swift
let session = TranslationSession(
  installedSource: source,   // e.g. en
  target: target,            // e.g. es-MX
  preferredStrategy: .lowLatency
)
// print(session.preferredStrategy)
```

Confirm language packs are installed (`LanguageAvailability(preferredStrategy: .lowLatency)` → `.installed`). Time the same 1,500 and 3,400 character fixtures you already have, idle M1 Max, three runs after a warmup call. Also time `.highFidelity` on the same text so you have a paired ratio, not just a hope.

**Pass:** wall clock drops on the order of 5–10× and output is still usable for your product.  
**Fail:** times unchanged → you were already on traditional models; look at (2) and load.

### 2. Shrink chunks to sentence/paragraph (~200–800 characters), keep one session, do not expect parallel speedup

**Why.** Official batch and 2–4 concurrent calls did not help you. `trn` still splits at 512 characters for streaming start time and to limit damage when a chunk goes wrong. A subtitle app caps batches at 10 for stability. Per-character cost in your data is not better at 3,400 than at 1,500.

**Test (~20 min).** Same session, `.lowLatency` if (1) worked. Split the 3,400-char fixture at sentence boundaries into ~400, ~800, ~1500, and one full blob. Sequential `translate(_:)`. Record time-to-first-chunk and total time. Inspect joins.

**Pass:** time-to-first-chunk falls and total time does not get worse; quality at joins is acceptable.  
**Do not** spend the hour on a second process or XPC pool until this and (1) are measured — nobody has shown that extra processes bypass the serializer.

### 3. Confirm you are not paying a main-queue / first-load tax; only then try a tiny MT model

**Why.** `translation-rs` documents completion on the main queue and 60 s timeouts. Menu-bar + Go + Svelte + Swift can stall that queue. First call after process start includes model load (`trn`). A 1.8B MT model is *unlikely* to beat lowLatency on bulk prose given published MLX numbers, but it is the fallback if lowLatency quality is unacceptable and highFidelity stays at 4–6 ms/char.

**Test (~25 min).**  
(a) Warmup: one 200-char translate, then time the 3,400-char run. Compare to cold start.  
(b) Log whether `translate` is being waited on from a thread that also pumps UI.  
(c) Only if (1) fails on quality: run `trn --quality low` and `--quality high` on your EN→es-MX fixture (install via Homebrew) as an independent clock, then optionally time HY-MT 1.8B or TranslateGemma on the same text if you already have MLX set up.

**Pass for (a)/(b):** cold start explains a chunk of the 11 ms/char “under load” figure, or the menu-bar thread is blocking.  
**Pass for (c):** only if lowLatency quality is a product problem; do not expect it to beat lowLatency on speed.

---

If you only change one line this week, change the initializer to `preferredStrategy: .lowLatency` and re-run the 3,400-character EN→es-MX fixture. Everything else in the public record is either that switch, or people discovering the same system-wide queue you already measured.

---

## Checked on this Mac (2026-09-30)

A standalone Swift test on the same M1 Max that produced the earlier numbers, English to Spanish (Mexico), 1,260 characters of real prose, `TranslationSession(installedSource:target:preferredStrategy:)`:

- **`.highFidelity`:** 10.4 s and 10.3 s in two runs, 8.2 to 8.3 ms per character. The default (no strategy given) is this path here, which agrees with Grok's reading that our numbers sit on the "high" curve.
- **`.lowLatency`, before the packs were installed:** failed with `TranslationError.Cause.notInstalled`. `LanguageAvailability(preferredStrategy: .lowLatency)` reported `supported`, not `installed`, for every pair; `.highFidelity` reported all of them `installed`.
- **`.lowLatency`, after downloading the English and Spanish (Spain) packs in System Settings:** en to es-MX, es and es-ES report `installed` (the Spain pack also serves Mexican Spanish). Two runs each on the same 1,260 characters:

| Strategy | Time | Per character |
|---|---|---|
| `.highFidelity` | 10.5 s, 10.3 s, 10.6 s, 10.5 s | 8.2 to 8.4 ms |
| `.lowLatency` | 1.0 s, 0.8 s, 1.0 s, 1.0 s | 0.6 to 0.8 ms |

About 12 times faster, which agrees with Grok's `trn` figures (9 to 17 times).

- **Quality, same passage:** the fast mode is understandable but rougher. It dropped the subject in "Is proud to be the number one contributor" (wrote "Se enorgullece de ser el colaborador número uno de Drupal"), used the singular "Bienvenido" for a room, wrote "Lo hemos estado" for "we have been", and mixed "puedes" with "ustedes". The slow mode wrote "Estoy orgulloso de ser el principal colaborador de Drupal" and stayed in one register ("ustedes", "han visto"). One passage in one pair is a sample, not a verdict.

So the switch is real and the speedup is about 12 times, at some cost in quality. That is why it is a user setting in #10 and not a silent change. Kai's bridge does not set a strategy today (`pkg/swiftbridge/internal/swift/apple_translate.swift`).
