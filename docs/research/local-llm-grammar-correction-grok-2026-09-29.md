# Local models for grammar and language correction on macOS 27: Grok's research from X

Kept for issue #208 (correct the source text's grammar and word choice, and replace mixed-in English).

**Provenance.** This is Grok's answer, saved unchanged from its shared conversation:
https://grok.com/share/c2hhcmQtMi1jb3B5_64b360d8-7442-45cc-8aa7-48f55f4f124b
(created 2026-09-29 22:27 UTC, 4:27 PM MDT). The prompt is reproduced first so the answer can be read against what was asked.

**Reliability.** Nothing in the answer below has been verified. Grok reports what it found in posts on X and in web pages; the people and figures are as it cited them, and none of the X posts were opened when this was saved. Treat the links as leads and check any claim before relying on it. Its own summary is that X says a lot about runtimes and shipping and almost nothing about Mexican-Spanish grammar quality or Spanish/English mixing, so no small model has any quality evidence for our task yet.

Where we used it: the summary in #208 and the build order there (Phase 1 on Apple's on-device model through the existing Swift bridge; a downloadable local model only if Phase 1 falls short).

---

## The prompt that was given to Grok

I'm building a macOS 27 menu-bar app (a translator written in Go with a Svelte UI, Apple Silicon only) and I want to add a small on-device language model for one narrow job. Please search X (posts, threads, replies, developer accounts) for how people are actually doing this on macOS 27 right now, and report back what you find, with links.
WHAT WE WANT TO DO
Take a piece of text and correct it in its own language, then let the user translate it onward. Two cases:
1. The text is entirely in one language, here Mexican Spanish, but the grammar or word choice is wrong. Fix the errors and keep the regional variant. Text that is already correct must come back unchanged.
2. The text mixes two languages, for example Spanish with English words inside it. Replace the English with the natural Spanish equivalent and leave the rest alone.
The model can be small; it does not need to be a big general chatbot. A "Models" screen is the UX target, like MacWhisper's: the user picks a model from a short list, it downloads on demand with progress and a checksum, one is set as the default, and models can be deleted. All inference is local so no text leaves the Mac.
WHAT I NEED YOU TO FIND ON X
1. Runtime: what are people using to run small text models inside a native Mac app on macOS 27? Compare Apple's Foundation Models framework (on-device model, where it is available and what hardware it needs), MLX and mlx-swift, llama.cpp with Metal, and Core ML conversions. What do people report about speed, memory use, and startup time?
2. Models: which small models (roughly 0.5B to 4B parameters) do people report as good at grammar correction and at Spanish, especially Latin American or Mexican Spanish, and at mixed-language (code-switched) text? Give model names, sizes, quantisation, and any reported failure modes such as over-rewriting or changing the meaning.
3. Shipping: how do developers of Mac apps that download models at runtime handle it? Model storage location, download and resume, checksums, licences (for example Gemma, Llama, Qwen terms), code signing and notarization of embedded inference binaries, and sandbox issues. Name the apps and developers doing it well (MacWhisper is one example) and what they say about the pitfalls.
4. Embedding from Go or another non-Swift host: has anyone embedded llama.cpp or MLX in a Go, Rust or Electron app on macOS, and what broke (threading rules for AppKit calls, cgo, app size)?
5. Anything specific to macOS 27 that changed for this (new frameworks, entitlements, permission prompts, Neural Engine access).
HOW TO ANSWER
- Give me a ranked list of the most useful posts and threads, each with the link, the author, the date, and a one-line summary of what they claim.
- Separate firsthand reports (someone shipped it or measured it) from opinions and speculation. Label each finding accordingly.
- Quote numbers only when the post gives them (tokens per second, memory, download size), and say which Mac chip they were measured on.
- If a claim looks like marketing or is a single unverified post, say so.
- If you cannot find good evidence for a question, say that plainly rather than guessing.
- End with a short recommendation: which runtime and which two or three candidate models you would test first for the grammar and code-switching task, and why, based only on what you found.

---

## Grok's answer (verbatim)

I have the OS and framework picture. Next I’ll pull developer-first reports on runtimes, small models for Spanish/grammar, and how Mac apps actually ship downloads.X is loud about **runtimes and shipping a rewrite UI**, and almost silent about **Mexican Spanish grammar / code-switching quality**. That split is the useful result. macOS 27 Golden Gate shipped 14 Sep 2026 (27.0.1 on 28 Sep); Apple Silicon only.

Below: ranked posts first, then findings by your five questions. Firsthand = someone shipped or measured. Opinion = analysis without a shipping artifact. Marketing = vendor or single unverified claim.

---

## Ranked posts and threads

1. **Mohammed Farmaan (@zxcodes) — 22 Sep 2026**  
   https://x.com/zxcodes/status/2102223540544327892  
   **Firsthand.** Shipped **Fixyy**, a menu-bar “fix / rewrite selection in place” app on Apple’s on-device `SystemLanguageModel`. No API key, nothing leaves the Mac. Requires macOS 26+, Apple Silicon, Apple Intelligence. OSS. This is the closest existing product to your job.

2. **Daisuke Majima / MLBoy (@JackdeS11) — 8 Sep 2026**  
   https://x.com/JackdeS11/status/2097196950470918527  
   **Firsthand measurement.** Core AI vs MLX on the same models, Mac + iPhone. Follow-up numbers live in his later posts and the write-up dated 5 Sep 2026. Best public speed/memory table I found.

3. **Daisuke Majima (@JackdeS11) — 17 Aug 2026**  
   https://x.com/JackdeS11/status/2089431820719063428  
   **Firsthand.** Qwen3.8-27B on Core AI, int4 + speculative decoding: **15.9 → 33.4 tok/s on a Mac Studio (M4 Max)**. Recipe + Swift engine linked. Larger than your band, but it is a real Core AI shipping path.

4. **poserDAD (@jmariwyatt) — 23 Sep 2026**  
   https://x.com/jmariwyatt/status/2102799424279970298  
   **Firsthand.** Shipped **Foundation-Chat** (SwiftUI + AppKit) wrapping macOS 27’s on-device Foundation Model / hidden `fm` CLI. Claims fully on-device, no backend. Plans to add more local models.

5. **El Pid (@pidster) — 26 Sep 2026**  
   https://x.com/pidster/status/2103939403303600131  
   **Firsthand experiment.** On-device AI experiment using macOS 27 Foundation Models: https://github.com/pidster/wisp

6. **Prasenjit Sarkar (@stretchcloud) — 21 Sep 2026**  
   https://x.com/stretchcloud/status/2101944528592584830  
   **Informed opinion, not a shipping report.** Best single post on what `fm` / `fm serve` actually changes: the on-device model is described as ~**3B**, already installed, and `fm serve` exposes an OpenAI-compatible localhost endpoint. Useful if your Go host does not want to speak Swift.

7. **Nick Hirras (@NickHirras) — 17 Sep 2026**  
   https://x.com/NickHirras/status/2100634910813765735  
   **Firsthand (his own app).** Bundled llama.cpp instead of renting APIs: persistent server, **warm completion ~650 ms, 61 tok/s generating on Metal**. Chip not named. Single-app report, not a benchmark suite.

8. **David Hendrickson (@TeksEdge) — 25 Sep 2026**  
   https://x.com/TeksEdge/status/2103303476574847060  
   **Firsthand-ish measurement (HF + his harness).** On an **M2 Max**, packed GGUF via Transformers/ggml Metal vs llama.cpp: Qwen3.5-4B Q4 **70.4 vs 71.8 tok/s**; Qwen3.8-27B Q4 **15.9 vs 13.4**; Qwen3.5-35B-A3B IQ4 **60.2 vs 61.3**. He notes conditions are not perfectly identical. Does not kill MLX.

9. **Jesús Espino (@jespinog) — 9–10 Aug 2026**  
   https://x.com/jespinog/status/2086403948080939139  
   https://x.com/jespinog/status/2086738492655042591  
   **Firsthand technical report.** **yzma** runs llama.cpp from Go with **`CGO_ENABLED=0`**: runtime bind via purego + libffi, downloads the right llama.cpp binary and GGUF for the machine. This is the only serious Go-on-Mac embedding story I found on X.

10. **takasago (@sago35tk) — 25 Sep 2026**  
    https://x.com/sago35tk/status/2103480336449445980  
    **Firsthand smoke test.** yzma playground ran **Qwen3.5-2B-UD-Q2_K_XL.gguf** without drama (WASM/WebGPU demo, not a native menu-bar app).

11. **Jordi Bruin (@jordibruin) — 23–24 Sep 2026**  
    https://x.com/jordibruin/status/2102785719865643244  
    https://x.com/jordibruin/status/2103134385369174092  
    **Firsthand (MacWhisper author).** 15.2 adds NVIDIA Nemotron 3 Diarization via Argmax; **that model is bundled, not downloaded**. Separate reply: other Whisper models are the “whisper c++ ones.” Closest living “Models screen” app, but he is not posting checksum/sandbox internals.

12. **Abhi Ram Salammagari (@abhiram304) — 6 Jul 2026**  
    https://x.com/abhiram304/status/2073997173835063759  
    **Firsthand.** Pomvox (local Mac dictation): native Swift app, FluidInference on ANE for STT, **mlx-swift for cleanup**. Two-engine repo; Swift port vector-checked against a frozen Python spec.

13. **Michael Doise (@mikedoise) — 6 Apr 2026**  
    https://x.com/mikedoise/status/2041203467935531190  
    **Firsthand.** Perspective Intelligence on the App Store: Gemma 4 E2B on-device via a custom MLX-Swift build with KV-cache work so long chats do not crash. Model still marked experimental.

14. **dnu (@DnuLkjkjh) — 30 Apr–1 May 2026**  
    https://x.com/DnuLkjkjh/status/2049966666621374687  
    **Firsthand (iOS, still relevant).** Shipping on-device LLM was not the hard part; **getting MLX-Swift to build cleanly was**. Thread on what broke.

15. **Awni Hannun (@awnihannun) — 9 Jun 2026**  
    https://x.com/awnihannun/status/2064199840658256166  
    **Official-adjacent.** WWDC MLX videos, including **MLX Swift**. Not a shipping-app report.

16. **Masao Ohkushi (@masao94) — 26 Sep 2026**  
    https://x.com/masao94/status/2103853323476783274  
    **Firsthand integration.** `fm serve` as OpenAI-compatible local API, called from a Slack Socket Mode agent with no external LLM server.

17. **KI-Spot.de (@KI_Spot_de) / Ahmed Ibrahim Hamdy — 27–28 Sep 2026**  
    https://x.com/KI_Spot_de/status/2104617165982929378  
    https://x.com/AhmedHamdy29189/status/2104346353287725541  
    **Firsthand CLI use, thin on numbers.** `sudo fm license` then `fm chat`. One claims ~**1 second** replies, offline; “too small for knowledge questions.” Chip not named. Treat the 1s figure as anecdotal.

18. **Rapid-MLX (@rapidmlx) — 1 Sep 2026**  
    https://x.com/rapidmlx/status/2094631703071621435  
    **Vendor measurement.** Desktop Mac app + CLI. GLM-5.3-Flash 4-bit: median **30 tok/s** over 512 generated tokens on **M3 Ultra**, **165 GB** active on 192 GB+ machines. Wrong size class for you; useful only as “Mac app that downloads models” existence proof. Marketing tone.

19. **Nativ (@Nativ_AI) — 29 Sep 2026**  
    https://x.com/Nativ_AI/status/2105001913028583662  
    **Vendor.** Local Apple-Silicon app with a Discover → Download models screen. Image model, not text correction. Existence proof for the UX you want, not evidence it is well engineered.

20. **Javier Larez (@CarlosLarez) — 16 Sep 2026**  
    https://x.com/CarlosLarez/status/2100280923195355274  
    **Firsthand user report.** Apple Intelligence “now works in Spanish” after iOS/macOS 27 launch issues. No quality detail, no Mexican-variant test.

---

## 1. Runtime on macOS 27

### Apple Foundation Models (on-device)

**What changed in 27 (Apple + WWDC, not X folklore):** Swift API to the same on-device model as Apple Intelligence; sessions can now be backed by that model, PCC, Claude/Gemini, or any provider that implements the Language Model protocol. Apple open-sourced **CoreAILanguageModel** and **MLXLanguageModel** adapters. New `fm` CLI and `fm serve` (OpenAI-compatible localhost). Multimodal prompts, Dynamic Profiles, Evaluations framework. Core AI is a separate OS framework for *your* models: load, specialize, AOT-compile, run on-device.

**Hardware (press + developer writeups, not a single X thread):** on-device AFM runs on Apple Silicon Macs with Apple Intelligence enabled. Ars describes **AFM 3 Core** on every Apple Intelligence device (iPhone 15 Pro through M5 Ultra) and **AFM 3 Core Advanced** needing **M3 or newer with 12 GB+ RAM**. Your “Apple Silicon only” app matches the OS: macOS 27 dropped Intel.

**Spanish:** Apple lists Spanish among Foundation Models languages. Users report Apple Intelligence Spanish lighting up around 16 Sep 2026. Nobody on X reported Mexican-Spanish dialect fidelity for the on-device model.

**Speed / memory / startup on X:** almost no measured tok/s for AFM itself. Anecdotes: `fm chat` “about 1 second,” “too small for knowledge questions.” One analysis post calls the on-device model **~3B**. I did not find a Mac-chip-tagged AFM tok/s number.

**Fit for you:** Fixyy already does menu-bar fix/rewrite with `SystemLanguageModel`. That is the lowest-friction path if you can call Swift (or `fm serve` from Go). You do **not** get a MacWhisper-style model picker for Apple’s weights; the model is the system one.

### Core AI vs MLX vs llama.cpp Metal vs Core ML

Best numbers are Majima’s, **M4 Max / macOS 27 beta**, same-harness, greedy decode. Treat as one careful experimenter, not a vendor bake-off.

| Setup | Number | Chip |
|---|---|---|
| Qwen3-0.6B 4-bit, Core AI GPU (macOS 26 export) | **1,121 tok/s** | M4 Max |
| Same model, Core AI re-export on 27β | **~500 tok/s** | M4 Max |
| Same model, MLX | **455 tok/s** | M4 Max |
| Qwen3-8B, Core AI vs MLX | **94 vs 90 tok/s** | M4 Max |
| gpt-oss-20b MoE | MLX **100.2** vs Core AI **78.1** tok/s | M4 Max |
| Gemma 4 E2B 4-bit, MLX vs Core AI | **177.8 vs 53 tok/s** | M4 Max |
| Qwen3.5-4B Q4, Transformers-GGUF vs llama.cpp | **70.4 vs 71.8 tok/s** | M2 Max |
| llama.cpp bundled in a shipping app | **~61 tok/s**, warm **~650 ms** | Metal, chip unnamed |
| Qwen3.8-27B Core AI int4 + specdec | **15.9 → 33.4 tok/s** | Mac Studio M4 Max |

**Memory (Majima, selected):** Qwen3-0.6B on iPhone 17 Pro — Core AI GPU **196 MB**, MLX **489 MB**, llama.cpp mmap for Gemma 4 E2B **191 MB** vs MLX PTQ **3,010 MB**. On Mac, mmap/llama.cpp still wins footprint; MLX often loads hotter.

**Startup:** Core AI first generation after a fresh install can be slow (iPhone Qwen3-0.6B cold **76.5 tok/s**, then **193**). Core AI `.aimodel` export is **not a pure function** — 26 vs 27β export cut Qwen3-0.6B from 1,121 to ~500 tok/s. If you ship Core AI artifacts, pin the export OS.

**Opinions that match the numbers:** small dense (≤1B) Core AI GPU can win; mid 4B–12B Core AI ≈ MLX; MoE and some hybrids MLX or llama.cpp win; llama.cpp still best “one GGUF, small RSS, ship a binary.”

**Core ML conversions:** barely discussed on X in 2026 except as a low-memory, slower path (stateful INT4 Qwen3-0.6B **184 MB / 39 tok/s** on iPhone in Majima’s table). Not what people are shipping for text LLMs right now.

---

## 2. Models (0.5B–4B), Spanish, grammar, code-switch

**I could not find good X evidence for your actual task.** No thread measured “leave correct Mexican Spanish alone” or “replace embedded English only.” Claims below are adjacent, not a bake-off.

What *does* show up:

- **Apple on-device AFM (~3B, per one analysis post).** Already on the machine. Spanish is a supported language. Fixyy uses it for generic fix/rewrite. Failure modes for over-rewriting: **not reported**.
- **Qwen3 / Qwen3.5 0.6B–4B.** Heavily used in Core AI / MLX / GGUF benches. Qwen3.5-4B Q4 is the size that keeps showing up as “small but usable” (~70 tok/s on M2 Max). One research-adjacent post: a **4B** model “grabs the correct endpoint but spills arguments in English on a Spanish prompt”; SFT mostly fixes it. That is tool-calling, not grammar repair, but it is the only Spanish/English mix failure mode I found.
- **Gemma 4 E2B / 4B.** Shipped on-device in Perspective Intelligence (MLX-Swift) and in at least one Steam app that claims “improved multilingual support.” Speed: ~178 tok/s 4-bit MLX on M4 Max; much slower on Core AI because of per-layer embeddings (TTFT ~**5 s** on a 19-token prompt in one measurement). License is Gemma terms, not Apache.
- **Qwen3.5-2B GGUF** — yzma playground “just worked.” No quality report.
- **MiniCPM5-2B** (2.52B, Q4_K_M **1.56 GB**, Apache-2.0). Hyped on Artificial Analysis; Hendrickson’s own CPU run called it fast and **weak** on factual/coding evals. Not tested for Spanish on X.
- **decider-0.8B (Qwen3.5-0.8B → Core AI)** — Majima, decision model (options in, answer+probability out), not a rewriter.

**Over-rewriting / meaning change:** no measured failure modes on X for this task. Do not invent them.

**Latin American / Mexican Spanish:** no firsthand model ranking. Apple Intelligence Spanish working ≠ Mexican variant preserved.

---

## 3. Shipping: downloads, storage, checksums, licences, signing, sandbox

X is thin here. People show the UX. They do not write the ops post.

**What exists:**

- **MacWhisper (Jordi Bruin)** — the UX you named. New diarization model is **bundled**. Older Whisper weights are the usual downloadable ggml/whisper.cpp set. No post about checksums, resume, or `Application Support` paths.
- **Fixyy / Foundation-Chat / wisp** — no extra weights; they call the system model. That sidesteps download, license display, and sandbox file quotas.
- **Nativ, Rapid-MLX, LM Studio-class apps** — Discover → Download screens exist. No checksum or license-flow writeup on X.
- **yzma** — downloads llama.cpp dylibs for the detected GPU and fetches GGUFs. Closest “runtime download” story, aimed at Go CLI/apps, not notarized menu-bar apps.

**Not found on X (say so plainly):**

- Where Mac apps store LLM weights (`~/Library/Application Support/...` vs cache vs group container)
- Resume + SHA256/BLAKE3 practices for GGUF/MLX
- How people present Gemma / Llama / Qwen license acceptance in-app
- Code-signing / notarization failures for embedded `libllama.dylib` / Metal metallib
- App Sandbox `network.client` + large-file + JIT/executable-memory issues for inference binaries
- Hardened Runtime exceptions people actually shipped

**Licences mentioned in passing, not as shipping advice:** Gemma terms (Gemma 4), Apache-2.0 (MiniCPM5, Mapika deciders), Qwen Research License called out as non-commercial on an image model. Nobody posted a lawyered in-app flow.

If you need MacWhisper-like ops, you will be copying patterns from their app binary / docs, not from X.

---

## 4. Embedding from Go / Rust / Electron

**Go + llama.cpp is the only well-documented path on X.**

- **yzma** (hybridgroup): llama.cpp from Go **without cgo**, runtime-loaded dylib, hardware-specific binary download, GGUF helper. Articles + GopherCon 2026 workshop. Someone ran Qwen3.5-2B GGUF on it.
- Older cgo wrappers exist in the ecosystem; X in 2026 is pointing at yzma specifically to *avoid* cgo, C compilers, and fat cross-compile pain.
- **purego** is mentioned as the “call C without cgo” primitive yzma sits on.

**Not found:**

- A shipping **Go + Svelte menu-bar** LLM app
- AppKit main-thread rules when the inferencer is in a Go process (who owns the status item, who calls `NSPasteboard`)
- Electron + llama.cpp/MLX breakage reports from 2026
- mlx-swift or Core AI used from Go (those APIs are Swift; the realistic bridge is a small Swift helper, XPC, or `fm serve` on localhost)

**Practical implication from what *is* there:** if the host stays Go, people are using **llama.cpp as a dylib + localhost or in-process FFI**, not MLX-Swift. If you want Foundation Models / Core AI / mlx-swift, you add a Swift sidekick. `fm serve` is the hack that lets Go keep an OpenAI client and never link an inference library.

App size: nobody posted a before/after MB number for embedding llama.cpp in a Mac app.

---

## 5. macOS 27-specific

Confirmed from Apple + X, not rumor:

- **Foundation Models** opened to third-party and local models via a protocol; MLX and Core AI adapters are first-party-shaped.
- **Core AI** is new in this generation as the “bring your own model” OS framework (AOT, memory-safe Swift, hardware specialization).
- **`fm` / `fm serve` / `fm chat`** preinstalled. Some posts say `sudo fm license` first.
- **Apple Silicon only.** Last release with full Rosetta 2.
- Spanish for Apple Intelligence / Siri AI rolling through 27.0 → 27.2 betas.
- **No X thread** about a new Neural Engine entitlement, a new TCC prompt for local LLMs, or a sandbox change unique to inference. If those exist, developers are not posting about hitting them yet (the OS is two weeks old).

---

## Recommendation (only from the above)

**Runtime to test first:** Apple **SystemLanguageModel** via Foundation Models, with Go talking to **`fm serve`** or a 50-line Swift XPC helper. That is what Fixyy already ships for “fix this selection.” Zero download, zero license screen, Spanish is a supported language, nothing leaves the Mac. Use tight instructions / guided generation so correct text is echoed. Measure over-rewrite yourself; X will not tell you.

**If you still want a MacWhisper-style Models screen**, do not start with MLX-Swift from Go. Test:

1. **llama.cpp Metal + yzma** — only Go-native path with recent firsthand posts; GGUF, mmap, small RSS, delete = unlink a file; checksum the GGUF you host.
2. **mlx-swift or Core AI in a Swift sidecar** — better if you later want Foundation Models protocol / Dynamic Profiles. Core AI AOT is fast on small dense models but export is OS-fragile; MLX is the less surprising 4B path on Mac.

**Three weights to put on that list first:**

1. **System AFM** (default, already present).  
2. **Qwen3.5-4B Q4 GGUF** (or Qwen3-4B if 3.5 packs are awkward) — only small model with repeated Mac tok/s numbers and a documented English-on-Spanish-prompt failure that SFT fixed.  
3. **Gemma 4 E2B or 4B 4-bit MLX/GGUF** — actually shipped in a Mac/iOS app; multilingual claims exist; watch TTFT on Core AI and Gemma’s license text in the UI.

Skip MiniCPM5-2B until you have an eval set; the one local run that bothered to measure called it thin. Skip 27B-class models for this job.

Build a 30-prompt eval before you pick a runtime: 10 correct Mexican Spanish (must be unchanged), 10 broken Mexican Spanish, 10 Spanglish. X does not have that eval. You will.

---

## Web pages Grok's search also returned

Listed as returned by its search, in order, without duplicates. Grok's answer cites X posts; these are the general web sources it looked at alongside them.

- [What’s New - macOS - Apple Developer](https://developer.apple.com/macos/whats-new/)
- [WWDC26: What’s new in the Foundation Models framework | Apple](https://www.youtube.com/watch?v=Xrv8m_EHCbg)
- [Apple's Foundation Models Framework: What Actually Runs on the iPhone, and What Doesn't - DEV Community](https://dev.to/iniyarajan86/apples-foundation-models-framework-what-actually-runs-on-the-iphone-and-what-doesnt-j4f)
- [macOS 27 Golden Gate: The Ars Technica review - Ars Technica](https://arstechnica.com/gadgets/2026/09/macos-27-golden-gate-the-ars-technica-review/)
- [Apple's on-device Foundation Model, now on the AnyRouter API](https://anyrouter.dev/blog/apple-foundation-models-on-device)
- [Bring an LLM provider to the Foundation Models framework - WWDC26 - Videos - Apple Developer](https://developer.apple.com/videos/play/wwdc2026/339/?time=1179)
- [MacOS Golden Gate](https://en.wikipedia.org/api/rest_v1/page/html/MacOS_27)
- [Apple demonstrates cross-platform Siri upgrades in macOS 27 Golden Gate at WWDC — update brings Liquid Glass improvements and unifies AI strategy | Tom's Hardware](https://www.tomshardware.com/software/macos/apple-demonstrates-cross-platform-siri-upgrades-in-macos-27-golden-gate-at-wwdc-update-brings-liquid-glass-improvements-and-unifies-ai-strategy)
- [WWDC26: AI & Machine Learning](https://www.youtube.com/watch?v=Ljdcg9t1mUw)
- [WWDC25: Meet the Foundation Models framework | Apple](https://www.youtube.com/watch?v=mJMvFyBvZEk)
- [Managed OS for macOS](https://www.iru.com/updates/2026/09/28/managed-os-for-macos-b2a8eb6e-2e68-4545-a754-d873bd0ff7e2)
- [Apple Releases macOS Golden Gate 27.0.1 With Bug Fixes - MacRumors](https://www.macrumors.com/2026/09/28/apple-releases-macos-27-0-1/)
- [Apple confirms macOS 27 Golden Gate launch date: September 14 - 9to5Mac](https://9to5mac.com/2026/09/09/apple-confirms-macos-27-golden-gate-launch-date-september-14/)
- [macOS 27 will arrive on September 14 along with other OS updates](https://appleworld.today/2026/09/macos-27-will-arrive-on-september-14-along-with-other-os-updates/)
- [Apple Core AI vs MLX: which is faster on iPhone and Mac for the same model?](https://rockyshikoku.medium.com/apple-core-ai-vs-mlx-which-is-faster-on-iphone-and-mac-for-the-same-model-8faf3c8784ff)
