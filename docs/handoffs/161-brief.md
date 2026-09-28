# Brief: #161 source-lang-correct

Repo: Performant-Labs/kai-private (never upstream dtapps/kai). Issue: #161. Rigor: **in-session** (see "Rigor" below for why this isn't raised). UI surface: **yes** — a wireframe is required (see "UI surface determination"). Kind: two related but distinct fixes, one backend behaviour change (`internal/translate/service.go`) plus one frontend correctness/UX gap (`TranslateWindow.svelte`, `detectedLang.ts`), landing together because the issue's own comment says the second makes the first safe to ship.

Worktree: `.worktrees/0161-source-lang-correct`, branch `issue-161-source-lang-correct`, base master `096643a` (includes #84's chunking/chunk-1 identity short-circuit, #159).

## Problem

Two problems, deliberately scoped as one issue by the principal (comment on #161, filed against #11's shipped design):

**(a) A pinned source language that is wrong produces a silent no-op.** Hand-tested 2026-09-28: `default_from` pinned to `es-MX`, English text pasted and translated, comes back byte-identical to the input — no error, no warning, reads as a successful translation. Root cause: when the source is pinned (not Auto), `translateWithEngine` (`internal/translate/service.go:251`) sends the engine's request with `From: es-MX` unchanged and never asks whether the text actually is Spanish. At least Apple's engine, given a source it doesn't recognize as different, just echoes the input back rather than erroring. DeepL's own product behaviour is the model to match: when the pinned source doesn't match the pasted text, DeepL corrects the source dropdown to what it actually detects, visibly, as part of translating — the user is never left staring at an unexplained echo.

**(b) The Auto option has no stable landmark once it's relabeled.** Already shipped in #11: the Auto entry's label is replaced wholesale by `detectedSourceLabel` (`frontend/src/utils/detectedLang.ts:34`) with e.g. `"Spanish (Mexico) (detected)"` — the literal word "Auto" never appears. A user scanning the dropdown has no fixed anchor; they must already know the top entry, whose label just happens to look like a real language name plus a small suffix, is the auto-detect choice — worse once a pinned entry with the identical base name (plain "Spanish (Mexico)") sits a few rows below it. The issue's own comment says this "needs solving as part of this issue's design, not left as-is," and matters *more* once (a) ships: a user who wants to escape a bad auto-correction back to Auto needs to be able to find it.

## What the code does today (base `096643a`, read 2026-09-28)

### Backend: `internal/translate/service.go`
- `translateWithEngine` (:251) is the single per-engine seam. Same-language short-circuit first (`req.From.SameAs(req.To)` → `identityResult`, no engine call). Otherwise `callEngineChunked` runs the engine (chunked per #84 when the text is over budget), then: **only when `isAutoSource(req.From)`** (:259) does it look at `detectedSource(res, err)` and, if the detection `Covers(req.To)`, turn the whole thing into an identity result. A **pinned** source never reaches this check at all — the branch is gated on `isAutoSource`, so a pinned mismatch is invisible to this function by construction.
- `detectedSource(res, err)` (:279) is engine-agnostic: it reads `res.From` (or, on failure, `engine.DetectedSourceOf(err)`) whether or not the request was pinned. **Correction (fourth review, B1 residual): its `ok` return is `false` only for the auto-echo case (an engine that just hands back the auto code with nothing detected).** An engine's own untranslated native code — baidu/youdao's own signature codes such as "jp" — comes back `ok=true`; `detectedSource` alone does not exclude it, and `Covers`/`SameAs` don't normalize an unrecognized code either. This is why `translateWithEngine`'s pseudocode below adds its own separate `model.ParseLanguage` recognition gate (`recognized`) on top of `detectedSource`'s `ok` — the two checks together, not `detectedSource` alone, are what makes a detection trustworthy enough to act on.
- `resultFrom(requested, reported)` (:389): `if !isAutoSource(requested) || isAutoSource(reported) { return requested }` — a pinned request's `From` is *always* reported back as requested, never replaced by what the engine detected. This is the qualification layer (issue #53's variant preference) and must stay true for the ordinary case; (a)'s correction has to bypass it deliberately and say so (see "Confidence-threshold decision").
- `isAutoSource(l)` (:398): `l == "" || EqualFold(l, "auto")`.
- `#84`'s `callEngineChunked` (`service_chunk.go:66`, merged today as 096643a) already runs this exact "detect on the first chunk, decide, then commit `from` for the rest of the request" pattern for **auto** sources only (:104-128): chunk 1 is sent alone, `detectedSource` is checked against `req.To` for the identity short-circuit, and separately (when it doesn't match) a recognized detection (`model.ParseLanguage`) is pinned as `from` for chunks 2..N so the source is decided once per request, never per chunk. This is the precedent to extend, not duplicate: a pinned-source correction must be decided from chunk 1 the same way, once, for the same reason (a per-chunk re-decision would let a long paste flip languages mid-translation).
- `model.TranslateResult` (`internal/model/model.go:259`) has no field for "the source was corrected" — `Identity` (:268) is the nearest precedent (`// set by the translate service only, never by an engine`).
- `model.Language` (`internal/model/model.go`): `SameAs` (:113, dialect-aware "same source/target" comparison), `Covers` (:136, detection-vs-target), `ParseLanguage` (:174, raw code → recognized code), `Base` (dialect → base alias). No confidence field exists anywhere on `TranslateResult`, and no engine reports a numeric detection confidence — OCR's `Conf float64` (`model.go:314`, `// Recognition confidence`) is a different, unrelated field on a different result type. **There is no confidence signal to threshold on beyond text length.**

### Frontend: `frontend/src/components/TranslateWindow.svelte`, `frontend/src/utils/detectedLang.ts`
- `fromLang` (:161, `$state<TranslateLang>(TRANSLATE_LANG.Auto)`) is the source select's bound value; `TRANSLATE_LANG.Auto` is the literal auto code.
- `detectedFrom` (:536, `$derived`) reads `activeResult?.from` — whatever the backend reported for the currently-displayed engine's result.
- `fromOptionLabel(value)` (:537-546) calls `detectedSourceLabel(value, TRANSLATE_LANG.Auto, detectedFrom, langName, t('translate.detected'))`; falls back to `langName(value)` when it returns `null`.
- `detectedSourceLabel` (`detectedLang.ts:23-34`): returns `null` unless `fromLang === autoCode`; returns `null` if nothing detected (`detectedFrom === '' || detectedFrom === autoCode`); otherwise returns `nameOf(detectedFrom) + suffix` — a **full replacement** of the option's text, e.g. `"Spanish (Mexico) (detected)"`. This is exactly what #11's own follow-up comment flags: nothing in that string says "Auto".
- The option list itself (:899-901, source select): `{#each languages as l}<option value={l.value}>{fromOptionLabel(l.value)}</option>{/each}`. `languages` (:160, loaded by `loadLanguages()`/`GetLanguages`) is the full list including the Auto entry (`TRANSLATE_LANG.Auto` is one of its values, per `GetLanguages`/`fallbackLanguages` at :717-722 which maps `ALL_TRANSLATE_LANGS`, and `ALL_TRANSLATE_LANGS` includes Auto — confirmed by `langName(value)` being called for it in the fallback branch same as every other entry). So the Auto option is rendered by the exact same `<option>` templated with `fromOptionLabel`, which is the single relabeling chokepoint — no separate "Auto row" markup exists to redesign around; the fix is entirely in what `fromOptionLabel`/`detectedSourceLabel` return for the Auto value.
- `onLangPicked(ev)` (:706-709): fired **only** by the two selects' own `onchange` (:896, :932). Calls `learnLangVariant(value)` (→ `learnFromSelection` → the backend's `LearnLangVariant`/`langpref.Store`, issue #53) then `persistLangs()` (writes `default_from`/`default_to` to the settings file). `swap()` (:768-777) and `loadDefaults()` (:679-687) assign `fromLang`/`toLang` directly and deliberately never call `onLangPicked` — the existing, load-bearing rule is "assigning the state directly never teaches; only the select's own user-driven `onchange` teaches." **This issue's correction never assigns `fromLang` at all** (see "Visible-cue mechanism" below — the dropdown never changes), so it does not need to follow or imitate this convention; there is nothing to bypass.
- The toast mechanism exists (`TranslateWindow.svelte:842-847` — `let toast = $state('')`, `showToast(msg)`, a 1.6s timer, rendered at :1363-1364), used today only by `copy()`. **This issue does not use it** (see "Visible-cue mechanism" below — a persistent note, not a toast).
- `#53`'s variant-learning path (`learnLangVariant`/`persistLangs`) is the mechanism (a) must NOT feed: an engine-detected correction is not a user's deliberate pick, and teaching it back would let one auto-corrected guess silently become the new default pin for every future translation — the opposite of what a correction should do.

## Prior brief format/rigor conventions (`docs/handoffs/145-brief.md`, merged as c3a7d13)

#145 (also `TranslateWindow.svelte`, in-session, UI surface: yes) is the closest prior art for both format and for a UI surface on this exact window: it hand-tested the file at a real base commit, cited exact line numbers for every existing mechanism it reused (`showToast`, `persistLangs`, `onLangPicked`'s "assign directly never teaches" rule, the `#118` coalesce timer), stated an explicit Reuse map table, an explicit "what to delete" section, a `Decided rules` section citing its own reasoning (D1-D4) rather than leaving them open, and required `docs/handoffs/145/wireframe.html` — low-fi HTML, light+dark, en-US+zh-CN, a zoom control (150% start, `-`/`+`/`0` keys) matching `docs/handoffs/118/wireframe.html`'s convention — approved by the principal before T. This brief follows the same structure and the same wireframe convention (see "UI surface determination").

No prior wireframe covers the language bar itself (#95/#96's wireframes, per `git log --all --oneline -- "docs/handoffs/*wireframe*"`, cover the screenshot-preview zoom control and the #96 failure-card UI, not the language selects) — #161's wireframe is new ground, not a reskin of an existing one.

## Issue #11 — what this extends

#11 ("Auto-detect source language with visible '(detected)' feedback," closed, in-session, UI surface: true) shipped: "Source control defaults to 'Detect language'; once a result arrives it reads e.g. 'English (detected)'. Pinning a concrete source language clears the marker." That is exactly `detectedSourceLabel`'s current behaviour, described above — it never anticipated a *pinned* source ever needing feedback of its own, because #11 predates (a) entirely. #161(a) extends the same "surface what the engine actually saw" principle from the Auto case to the pinned case; #161(b) is a *bug fix* in #11's own shipped design (the missing landmark), not new scope.

## Acceptance criteria

### (a) Auto-correct a pinned source language that doesn't match the actual text

1. **When the source is pinned (not Auto) and the request's text is at least 20 code points long (see "Confidence-threshold decision"), the engine call is dispatched with an internally-substituted `From: auto` instead of the pin** — see "Where a pinned request's detection comes from" for the full mechanism and why this is the only option that actually works for Apple, the engine the bug was reported on. The engine's own detection and its translation come back together, from the one call, exactly as an ordinary Auto request already works today.
2. The engine's detection is then compared against the **original pin** (`SameAs`/`Covers`, backend-decided — see WARN 4's fix below): if it matches, nothing changed from the user's point of view — the pin was right, the translation is what it always would have been, no note. If it differs, that's the correction: `DetectedFrom` is set to what was actually detected, the translation the engine already produced (using auto) is used as-is — **no second engine call, no re-run**.
3. The decision (substitute-as-auto, then compare) is made once per request, from the first chunk only when chunked (mirrors #84's existing auto-source pattern in `callEngineChunked`, :104-128) — never re-decided chunk to chunk. Chunks 2..N use whichever language chunk 1's comparison settled on (the pin, if it matched; the detection, if it didn't) — same as #84 already does for a genuine Auto request.
4. **`DetectedFrom` is a backend decision, not a frontend string-diff** (fixes WARN 4), and the SAME rule applies on both paths this issue touches (fixes B2 residual, fourth review — the two paths must not disagree): on the substituted-pinned path, the backend sets it only when its own `SameAs`/`Covers` comparison says the detected language actually differs from the pin — dialect-aware, so `es` vs `es-MX` is correctly treated as the same language, not a false correction. On the genuine-Auto path (no pin to compare against), it is set unconditionally whenever a recognized detection exists and the result isn't identity — no floor, no comparison, since there is nothing to have gotten wrong. **On both paths the field holds the same shape of value: the qualified `From` (`s.resultFrom(model.Auto, detected)`), never the bare unqualified detection** — so the note reads "Spanish (Mexico)", not "es", either way. The frontend renders the note whenever the field is non-empty; it does no comparison of its own.
5. Text under the 20-code-point floor is sent with the pin exactly as today — no internal auto substitution, no correction, no note. This is unchanged from before.
6. The corrected source is **not** taught back to the variant-preference store (`learnLangVariant`/`persistLangs`) and does **not** overwrite the user's saved `default_from` pin. The next translation (of different text) still starts from the user's original pin. This is a per-request override, not a re-pin.
7. `TranslateMulti` (parallel fan-out, `internal/translate/service.go:432`) and the single-engine `Translate` path both go through `translateWithEngine`, so both get the correction for free — no separate wiring per caller. `ScreenshotTranslate`'s own capture flow always sends `from := model.Auto` and is genuinely untouched. **`ScreenshotRetranslate`, however, DOES reach `translateWithEngine` (via `translateAllStream`) and CAN carry a pinned mismatch** — accepted in writing (fixes WARN 7): the correction still happens correctly on the backend, but `ScreenshotCard`/`TranslateCard` renders no note today, so a screenshot re-translate silently benefits from the fix with no visible explanation. Out of scope to fix `TranslateCard`'s rendering in this issue; flag it as a known follow-up in the PR body.

### (b) Auto landmark redesign

1. The Auto entry in the source dropdown always reads "Detected" (the new dedicated key, see "Wording" below — not the old literal word "Auto"), in every state: no result yet, a result with a detection, a result with no detection, and — new with (a) — after a pinned entry elsewhere in the list was just auto-corrected. The entry's underlying VALUE stays `TRANSLATE_LANG.Auto`; only its displayed text changes.
2. A concrete pinned language's own entry (e.g. plain "Spanish (Mexico)") is never relabeled or suffixed by the detection feedback; only the Auto entry carries the "(detected: …)" information. (Unchanged from #11 — stated explicitly because (a) makes it load-bearing: the user must be able to tell "my pin was silently corrected this once" apart from "I am looking at the fixed Auto entry.")
3. `fromOptionLabel` (now trivial, see Reuse map) is the single relabeling chokepoint — `detectedSourceLabel`/`detectedLang.ts` are deleted (fixes WARN 6), not extended; no second markup path for the Auto `<option>`.

## Where a pinned request's detection comes from (principal decision, 2026-09-28, resolves the Phase-3 BLOCK)

**The first version of this brief was wrong about this, and the architecture review caught it before any code was written.** It assumed `detectedSource(res, err)` would return something useful on a *pinned* call — it doesn't, for most engines. A pinned request tells the engine exactly what language to assume; engines that were never asked to detect anything don't detect anything: Apple's bridge only runs its own detector when the source is left empty (`apple_translate.swift:253`); DeepL and Tencent just echo the pin back (`deepl.go:114-117`, `tencent.go:180-183`); the three LLM engines return the requested `From` unchanged. So the original plan could never fire for Apple — the exact engine the bug was reported on. Worse, even if a detection existed, this codebase has a hard rule against a second engine call to re-translate with a corrected language (`translateWithEngine`'s own comment: "no re-run and no second engine call," `service.go:242`), and #84's single-call-per-budget shape doesn't leave room for one anyway.

**The fix, decided by the principal: when a pinned request's text is at least 20 code points long, dispatch the engine call with `From` internally substituted to `auto`, not the pin.** This makes the engine do exactly what it already does for a genuine Auto request. The second architecture review found the first version of this section underspecified two real things — the identity/covers-target sub-case (the reported bug's actual shape: pin `es-MX`, English text, target `en`), and exactly where the original pin lives through the call — both are named precisely below, against the real functions.

**Named seam: `translateWithEngine` builds a second, local request value for dispatch; `req` itself is never mutated.**

```
func (s *Service) translateWithEngine(ctx, reg, engineName, req, progress) (*model.TranslateResult, error) {
    if text non-empty && req.From.SameAs(req.To):
        return s.identityResult(engineName, req, ""), nil          // unchanged, existing top-of-function check

    sent := req                                                     // local copy; req is untouched throughout
    substitute := !isAutoSource(req.From) && codePointLen(strings.TrimSpace(req.Text)) >= 20
                                                                     // fixes W3 (fourth review): trimmed, matching
                                                                     // "Confidence-threshold decision"'s own stated
                                                                     // rule ("the request's trimmed text") — the
                                                                     // gate previously measured untrimmed length,
                                                                     // which disagreed with the prose next to it.
    if substitute:
        sent.From = model.Auto

    // pinFallback: what callEngineChunked's chunk-1 logic falls back to for chunks 2..N when
    // detection is unusable. req.From when substitute (fall back to the ORIGINAL pin, not
    // auto — a substituted-pinned request with no usable detection should behave as if it had
    // simply been sent with the pin, not keep re-detecting every later chunk). "" when not
    // substituting (today's unchanged behavior: chunks 2..N stay auto and re-detect).
    pinFallback := ""
    if substitute:
        pinFallback = req.From

    res, err := s.callEngineChunked(ctx, reg, engineName, sent, progress, pinFallback)

    // A detection only counts if model.ParseLanguage recognizes it (fixes B1, second review's
    // false-positive bug): detectedSource's ok==true only means "the engine reported SOME From",
    // which includes an engine's own untranslated native code (Baidu's "jp", "kor" — baidu.go:120).
    // SameAs("jp","ja") is false (canonical() does not normalize an unrecognized code), so without
    // this gate, EVERY Baidu request pinned to Japanese would have been flagged as a false
    // "correction" to "jp". A recognized-but-unparseable detection is treated exactly like no
    // detection at all: fall through to the unusable-detection branch below.
    detected, dok := detectedSource(res, err)
    recognized := dok
    if dok {
        _, recognized = model.ParseLanguage(string(detected))
    }

    // Gated on sent.From, not req.From (unchanged from before): this is what makes the
    // identity/covers-target case fire for a substituted pinned request too — sent.From is auto
    // whenever substitute is true, exactly like a genuine auto request, so this same existing
    // check now also catches "the pin was wrong and the text is actually the target language"
    // (the reported bug), not just "the caller asked for auto and it happened to match the
    // target."
    if isAutoSource(sent.From) && ctx.Err() == nil && recognized && detected.Covers(req.To):
        return s.identityResult(engineName, sent, detected), nil
        // sent (NOT req — fixes B2, second review's bug): sent.Text/sent.To are identical to
        // req's (only .From ever differs between them), so nothing about the identity result's
        // content changes — but identityResult computes From as resultFrom(passed.From, detected),
        // and resultFrom's PINNED branch ignores its second argument entirely and just returns the
        // pin verbatim. Passing req here would have reported the WRONG, uncorrected pin (es-MX) as
        // From even on a successful identity correction. Passing sent (whose .From is auto when
        // substitute is true) takes resultFrom's AUTO branch instead, which correctly qualifies
        // the real detected language. DetectedFrom is NOT set on this path (unchanged) — Identity
        // already explains it.

    if err != nil:
        return nil, err

    // DetectedFrom is set whenever the result is not identity AND a recognized detection exists —
    // on BOTH paths, substituted-pinned and genuine Auto (fixes B3, second review's bug: the prior
    // version left it empty for a genuine Auto result, contradicting the Visible-cue section and
    // the approved wireframe's state 2(b), which both require the note on a plain Auto
    // translation too). The 20-code-point floor does NOT apply here — the floor exists to gate
    // overriding a user's deliberate pin, a real behavior change; showing what Auto already
    // detected is purely informational and carries none of that risk. (Principal decision, made
    // here per the second review's recommendation, not left open — see "Open decisions" below.)
    if !substitute:
        res.From = s.resultFrom(req.From, res.From)                // UNCHANGED path, byte-for-byte as today
        if isAutoSource(req.From) && recognized:
            res.DetectedFrom = res.From                             // mirrors the already-qualified From;
                                                                     // same value, just exposed on the new field
        return res, nil

    // substitute == true, and this was not an identity result: decide whether it was a real
    // correction, the pin turned out to be right, or the detection was unusable/unrecognized.
    if recognized && !detected.SameAs(req.From):
        res.From = s.resultFrom(model.Auto, detected)               // reuses resultFrom's EXISTING auto branch
                                                                     // (s.langPrefs.Qualify(detected)) — the
                                                                     // corrected language is qualified through
                                                                     // #53 exactly like a genuine auto result is;
                                                                     // no new qualification logic is written.
        res.DetectedFrom = res.From                                 // fixes B2 (fourth review): store the QUALIFIED
                                                                     // value, same shape as the non-substituted
                                                                     // branch above (:143) — both branches now put
                                                                     // the identical kind of value on DetectedFrom
                                                                     // (never the bare unqualified `detected`), so
                                                                     // the note reads "Spanish (Mexico)" in both
                                                                     // state 2(b) and state 2(c), never a bare code.
    else:
        res.From = req.From                                         // detection matched the pin, was unusable,
                                                                     // or was an unrecognized native code:
                                                                     // report the pin, unchanged from what the
                                                                     // caller asked for. DetectedFrom stays
                                                                     // empty; no note.
    return res, nil
}
```

**`callEngineChunked` gains one new parameter, `pinFallback model.Language`** (`service_chunk.go:66`'s signature), threaded through from the call above. Its existing chunk-1 block (`:103-128`) is otherwise untouched except for one precision fix (W1, second review): where it currently does `if l, ok := model.ParseLanguage(string(a.res.From)); ok { from = l }` unconditionally, change it so a substituted-pinned request whose chunk-1 detection *matches the pin* keeps the pin's full dialect specificity for chunks 2..N, rather than downgrading to the bare parsed code:

```
if l, ok := model.ParseLanguage(string(a.res.From)); ok {
    if pinFallback != "" && l.SameAs(pinFallback) {
        from = pinFallback   // preserve the pin's dialect (es-MX), not the bare match (es)
    } else {
        from = l             // a genuine correction (or a genuine auto request): use what was detected
    }
} else if pinFallback != "" {
    from = pinFallback        // unrecognized detection, substituted case: fall back to the pin
}
// else (unrecognized, pinFallback == ""): from stays the auto value, unchanged — a genuine auto
// request's existing per-chunk-re-detect behavior on an unrecognized chunk-1 detection is untouched.
```

Only a substituted pinned call ever passes a non-empty `pinFallback`, so a genuine auto request's behavior — including this new middle branch, which can never fire for it — is bit-for-bit unchanged.

**Why the short (non-chunked) and chunked paths don't diverge, despite looking like two code paths (this was the review's other concern):** `callEngineChunked`'s own single-call shortcut (`budget.Fits(req.Text)`, the common case for ordinary-length text) never runs its internal chunk-1 detection logic at all — it just calls `callEngine` once and returns. The identity/covers-target catch for that common case is `translateWithEngine`'s own post-call check above (now gated on `sent.From`), which already ran before this section existed, for genuine auto requests, and now also correctly covers the substituted case. `callEngineChunked`'s internal chunk-1 identity check only matters for genuinely long (actually-chunked) text, as a chunking-specific early exit — it converges on the exact same `s.identityResult(...)` call in `translateWithEngine` once it returns. One identity construction, one place, for every request shape.

**The tradeoff, stated plainly since it's a real behavior change, not just a bug fix:** every pinned request over the 20-code-point floor now runs through the engine as if it were Auto internally, and only reports back as "pinned" if the result agrees with what was asked. This is different from today's contract, where a pin is sent to the engine exactly as given, always, with no verification. The principal accepted this tradeoff explicitly in exchange for a fix that actually works on Apple.

## Confidence-threshold decision (concrete rule)

There is **no per-detection numeric confidence available from any engine** for translation (grepped `internal/model`, `internal/engine`: nothing beyond OCR's unrelated `Conf` field on `model.OcrResult`). The rule the issue asks for ("minimum text length + minimum detection confidence if the engine reports one") therefore reduces, honestly, to a **length gate alone**, stated as such rather than implying a confidence signal that doesn't exist:

- **Rule: a correction only fires when the request's trimmed text is at least 20 Unicode code points long**, citing #119/#151's epic context directly: "Apple's own guidance covers only the short end (language-detection reliability under ~20 characters)" (#151 epic body) — the same ~20-character floor `internal/engine/input_budget.go:124`'s comment already references for a different reason (youdao's signature input is cut to 20 characters). Below the floor, the pin is honored as given, exactly as today (no correction, no cue) — a short fragment ("ok", "sí", a name) is exactly the case where a detector is least trustworthy and a wrong "correction" would be worse than today's silent echo.
- **The gate is on the whole request's text, not per-chunk**: for a chunked request (#84), the 20-code-point check is against the full `req.Text` before the chunk-1 detection is even attempted — mirrors the existing chunk-1-decides-for-the-request shape, and avoids a large paste whose *first* chunk happens to be short skipping the correction it should get.
- **The detection itself still has to be "confident" in the one sense available**: `detectedSource`'s `ok` return excludes the auto-echo case (an engine handing back nothing usable); a separate `model.ParseLanguage` recognition gate in `translateWithEngine` (the pseudocode's `recognized` variable) excludes the other low-signal case, an engine's own unrecognized native code (baidu/youdao's "jp"/"kor"), which `detectedSource` alone reports as `ok=true`. Together, these two existing/reused checks are the whole of "is this detection usable" — no new confidence math is invented on top of them; the 20-character floor is the whole of the separate "is this pin worth overriding" threshold.
- This is a decided rule, not left open for T/F to invent independently — the wireframe (below) does not need to re-litigate the number, only show the cue's behavior at/around it if useful for principal sign-off.

## Auto-correction must not get taught back into `learnLangVariant`/`persistLangs`

**Superseded by the "dropdown never changes" decision below — kept here only to state the final rule plainly.** The correction never touches `fromLang`, `default_from`, `learnLangVariant`, `persistLangs`, or `onLangPicked`, on any code path, for any reason. The frontend's only new state is the result-pane note, read from `activeResult?.detected_from`; nothing about the dropdown's bound value is ever assigned by this feature. `swap()`/`loadDefaults()`'s existing "assign directly never teaches" convention is not something this feature needs to imitate, because it never assigns `fromLang` at all — there is nothing to bypass.

Backend-side, `resultFrom` (:389) is **not** the thing that performs the correction, and is not modified — see "Where a pinned request's detection comes from" for the actual mechanism (a request-shape change decided before the engine call, via `sent.From` substitution) and exactly how the corrected `From` is qualified (`s.resultFrom(model.Auto, detected)`, reusing the existing auto branch unchanged).

## Wording: the dropdown's Auto option is relabeled "Detected" (principal, 2026-09-28)

**A NEW i18n key, not a change to the shared `lang.auto` string.** `lang.auto` ("Auto" / "自动", `en-US.ts:295` / `zh-CN.ts:287`) is used in two places: the translate window's source-language dropdown (the one this brief is about) AND, unrelated, `GeneralTab.svelte:79` — the app's own interface-language setting ("follow system"), where "Auto" is correct and must stay "Auto". Renaming the shared key would silently break that settings screen. **Add a new key instead** (e.g. `translate.sourceAuto`, en-US "Detected", zh-CN a short equivalent such as "检测" or "自动检测" — F/D's call), used only by the translate window's FROM-dropdown option; `GeneralTab.svelte` keeps using `lang.auto` unchanged.

This is the literal, permanent, unconditional label for that one dropdown entry now — it is not the "Auto — detected: X" relabeling this brief already rejected above, and it is not conditional on detection state: the entry always reads "Detected" (or its zh-CN equivalent), whether or not a translation has run yet. Every wireframe frame and every place elsewhere in this brief that says "bare Auto" now means "bare Detected" for this one dropdown — `GeneralTab.svelte`'s language selector is unaffected and keeps reading "Auto".

## Visible-cue mechanism (revised: principal override, 2026-09-28 — the dropdown never changes, ever)

**The FROM dropdown's displayed value never changes programmatically, in either direction covered by this issue.** Not for Auto's own live detection, and not for a pinned-language correction. This overrides both the original brief's toast-plus-possible-reassignment plan and #11's already-shipped Auto-row relabeling (`detectedSourceLabel` returning the detected language name in place of "Auto") — the principal's own words: *"I think changing 'Auto' to 'Auto — detected: Spanish (Mexico)' is a mistake... I don't want the dropdown to change from Auto."* This is a real, deliberate walk-back of part of #11's shipped behavior, not just a decision about #161's new addition — say so plainly in the PR body.

**What replaces both the toast and the relabeled dropdown: a persistent, muted note near the result, not a transient toast.** When the language actually used for a request (chunk 1's `detectedSource`, filtered through the `model.ParseLanguage` recognition gate — fixed wording, fourth review: the 20-code-point floor gates only whether a PIN gets overridden at all, it does not gate whether the note appears once a recognized detection exists) differs from what the FROM dropdown displays — whether the dropdown says "Auto" (informational only, nothing to correct) or shows a specific pinned language that didn't match (the actual bug this issue exists to fix) — render a small, lightly-colored line near the result pane: **"Translated from auto-detected Spanish (Mexico)"** (exact wording the principal gave; i18n picks the zh-CN equivalent). This note:
- Is **persistent for as long as that result is showing**, not a 1.6 s toast — it's informational, not an event notification, so it should not disappear on its own. Style it like the existing muted small-text notes already in this pane (the phonetic/identity note convention, `u-muted text-[11px]`, `TranslateWindow.svelte` around the identity-note rendering) rather than the toast mechanism.
- Appears **whenever the used language differs from the dropdown's display**, including the plain Auto case (this is what makes Auto's own detection visible now that the dropdown itself no longer says so) — so this single mechanism replaces #11's relabeled dropdown AND this issue's original toast plan in one stroke.
- **Never appears, and the dropdown never moves, when the pin was already correct** — unchanged from the original brief's intent.
- Still does **not** get taught back into `learnLangVariant`/`persistLangs` — nothing about a note changes that; the pin (`fromLang`, `default_from`) is genuinely never touched by this feature at all now, not even transiently, which actually simplifies that part of the brief (no "assign `fromLang` directly, never teach" bypass logic is needed any more, because `fromLang` is never assigned by this feature — delete that reuse-map row).

**The original visible-cue toast plan (`showToast`/`translate.sourceCorrected`) and the Auto-landmark relabeling proposal (`translate.autoDetected`, the three rejected alternatives) are both superseded by this section — do not build either.** The Reuse map below is corrected to match.

## Auto-landmark problem: resolved as a side effect, not a separate redesign

The issue's original complaint (comment on #161, "Auto is nowhere for the user to see and select" once relabeled) is now moot: `detectedSourceLabel` is reverted to never relabel the Auto row's detection-dependent text at all — it always reads the fixed "Detected" label (the new dedicated key; see "Wording" above), never a relabeling toward a language name. The detected-language feedback #11 was trying to surface moves entirely to the new persistent result-pane note above. **This means #11's own shipped `detectedSourceLabel` change is reverted as part of this PR** — but `detectedFrom` itself (the existing `$derived` block, `TranslateWindow.svelte:536`, reading `activeResult?.from`) is **NOT** repurposed or touched: it still feeds `swapPair`/`swapLanguages` (:556-567) exactly as it does today, and breaking that is not an option. The new result-pane note reads a **separate, new** derived value off `activeResult?.detected_from` (the new backend field, lowercase JSON key) — a sibling to `detectedFrom`, not a replacement for it. `fromOptionLabel` (which today calls `detectedSourceLabel(..., detectedFrom, ...)`) stops calling `detectedSourceLabel` at all (deleted, see Reuse map) and returns the fixed `translate.sourceAuto` string for Auto instead — `detectedFrom` simply has one fewer caller, it is not deleted or changed.

## UI surface determination

**This needs a wireframe. Say so explicitly, and require it before T** — this is not a case where the existing rendering already covers it (unlike, say, a pure backend bugfix). One new visual element is being introduced to a window the principal actively hand-tests (per #145's precedent on this exact file): the persistent muted result-pane note. This is exactly the kind of "does this read right in the actual pane at real widths, light and dark, both locales, without colliding with the identity/phonetic notes already there" call #145's wireframe existed to settle for this same file. (The dropdown itself is now the easy part: confirming it never changes needs no new visual design, just a frame proving the rule.)

**Wireframe requirements** (`docs/handoffs/161/wireframe.html`, low-fi HTML, light+dark, en-US+zh-CN, zoom control 150% start with `-`/`+`/`0` keys — same convention as `docs/handoffs/118/wireframe.html` and `145/wireframe.html`), to draw:

1. The source dropdown open, showing that the Auto row always reads "Detected" (the new dedicated key) regardless of detection state — confirm visually it never carries a language-name suffix, next to a plain pinned entry ("Spanish (Mexico)") for comparison, so the wireframe makes the "dropdown never changes" rule legible, not just described.
2. The new persistent, muted result-pane note in its three states: (a) not shown at all (pinned source was correct, or nothing translated yet), (b) shown under an Auto-mode result ("Translated from auto-detected Spanish (Mexico)"), (c) shown under a pinned-but-wrong-source result (same wording, dropdown still showing the original pin above it, unmoved) — at both window widths, both themes, both locales, styled consistent with the pane's existing muted small-text notes (not the toast mechanism).
3. A frame showing the note does not visually collide with the identity note (#80) or the phonetic/dictionary rows already in that pane, since it's a new line in the same area.

## Reuse map (extend, do not duplicate)

| Need | Existing object | Change |
|---|---|---|
| Substitute `From: auto` for a pinned request that's long enough | new local logic in `translateWithEngine` (`service.go:251`) | `sent := req`, `sent.From = model.Auto` when `!isAutoSource(req.From) && codePointLen(req.Text) >= 20`; `req` itself is never mutated — see "Where a pinned request's detection comes from" for the exact algorithm |
| Decide "does this result count as a detection I can trust" | `detectedSource(res, err)` (`service.go:279`) + a new `model.ParseLanguage` recognition gate at the call site | `detectedSource` itself is reused unchanged; it now also runs on a substituted-pinned request because `sent.From` is auto. Its own `ok` only excludes the auto-echo case, not an engine's unrecognized native code, so `translateWithEngine` layers `recognized := ok && ParseLanguage(detected) succeeds` on top before trusting any detection (fixes B1, both second and fourth reviews) |
| Decide the correction once per request, from chunk 1 (chunked case) / one call (common case) | `callEngineChunked`'s existing auto-source chunk-1 decision (`service_chunk.go:104-128`) + `translateWithEngine`'s own post-call check (`service.go:259-263`) | `callEngineChunked` gains one new parameter, `pinFallback model.Language`, consulted in TWO branches (fixes W1 residual, fourth review — the prior wording of this row named only one): a RECOGNIZED chunk-1 detection that matches the pin, where it preserves the pin's dialect specificity instead of downgrading to the bare parsed code (`service_chunk.go`'s new `if pinFallback != "" \&\& l.SameAs(pinFallback)` branch), and an UNRECOGNIZED chunk-1 detection, where it's the fallback itself (the existing `else` branch). `translateWithEngine`'s existing post-call identity check is re-gated on `sent.From` instead of `req.From` — this is the whole fix, not a new decision point |
| Compare a language against the target/pin for "is this actually different" | `Language.SameAs` / `Language.Covers` (`model.go:113`, `:136`) | reused unchanged, backend-side only — no frontend string comparison anywhere |
| Recognize a raw detected code | `model.ParseLanguage` (`model.go:174`) | reused unchanged, same as #84's chunk-1 pin |
| Carry the actually-used source language to the frontend, for the result-pane note | new field on `model.TranslateResult`, sibling to `Identity` (`model.go:268`, `// set by the translate service only, never by an engine`) | add `DetectedFrom model.Language \`json:"detected_from,omitempty"\`` — one unified contract (fixes B2 residual, fourth review: the prior wording of this row disagreed with (a)4 and with the pseudocode's own comments). Set ONLY by the backend, to the same **qualified** value stored in `From` (`s.resultFrom(model.Auto, detected)`), never the bare unqualified detection: unconditionally on the non-substituted (genuine Auto) path whenever a recognized detection exists (no 20-code-point floor — that floor gates overriding a pin, not displaying what Auto already saw), and ONLY on a genuine mismatch (`recognized \&\& !detected.SameAs(req.From)`) on the substituted-pinned path. Empty on the identity path, empty on a matched/unusable/unrecognized-detection substituted path. The frontend renders the note whenever this field is non-empty — it does not compare anything itself. |
| Qualify the corrected language the same way a genuine auto result is qualified | `resultFrom` (`service.go:389`) | **not modified.** Called as `s.resultFrom(model.Auto, detected)` on the corrected path, reusing its existing auto branch (`s.langPrefs.Qualify(reported)`) unchanged — this is what answers "does the correction go through #53's Qualify": yes, via this exact existing call. |
| Show the persistent detected-language note | the pane's existing muted small-text note styling (the identity/phonetic note convention already rendered in this pane) | new note line, reusing existing CSS tokens (`u-muted text-[11px]` or equivalent already in use), sourced from `activeResult.detected_from`; not the toast mechanism — `showToast`/`u-toast` is not used by this feature at all |
| Relabel the Auto option | `fromOptionLabel` (`TranslateWindow.svelte:537-546`) | **`detectedSourceLabel` and `detectedLang.ts` are deleted, not extended** (fixes WARN 6): once the Auto label is the fixed constant `translate.sourceAuto`, `detectedSourceLabel`'s whole parameterized signature (fromLang/autoCode/detectedFrom/nameOf/suffix) is dead. `fromOptionLabel` becomes a two-line function: `value === TRANSLATE_LANG.Auto ? t('translate.sourceAuto') : langName(value)`. `detectedLang.test.ts` (if present) is deleted with it. |
| Keep the existing `detectedFrom` derived value | `TranslateWindow.svelte:536`, `$derived(activeResult?.from)`, feeds `swapPair`/`swapLanguages` (:556-567) | **unchanged, and NOT the same thing as the new backend field** (fixes WARN 5) — `detectedFrom` (frontend, lowercase, existing) is whatever `From` the active result reports, used for swap; `DetectedFrom`/`detected_from` (backend, new) is the correction flag this issue adds. Don't conflate or delete the existing one while adding the new one. |
| i18n catalog | `en-US.ts` / `zh-CN.ts` / `keys.ts` catalog check | add `translate.sourceAuto` ("Detected" / "自动检测") and `translate.translatedFromDetected` (the result-pane note). Do NOT add `translate.autoDetected`/`translate.sourceCorrected` (superseded). **`translate.detected` is removed from both catalogs** (fixes WARN 3) once `variants.e2e.test.ts:34-49`'s `describe('detected label with the real dictionaries', ...)` block — built entirely on the now-deleted `detectedSourceLabel` — is deleted outright, not updated; that block was its only remaining user. |

## Scope limits (explicit)

- **No change to which engines are enabled** or to engine registration/config (`internal/engine`, `internal/configstore`).
- **No change to #84's chunker** (`chunk.go`, `Split`, `chunkTarget`) — this reuses `callEngineChunked`'s existing chunk-1 decision point, it does not touch chunk sizing or the reassembly (`join`/`sepAfter`).
- **No change to the request registry** (#109, `requests.go`/`activeRequest`/`engineRun`) — a correction is decided synchronously inside one engine's existing call path, not a new kind of request or a new cancel surface.
- **No second/parallel detection call, ever.** The substitution (sending `auto` instead of the pin) IS how a real detection is obtained, in the same single call `translateWithEngine` already makes — not a second call. An engine that comes back with no usable detection (echoes `auto`, or an unrecognized native code) gets no correction — accepted limitation, not a follow-up TODO to silently work around later.
- **No numeric confidence.** The 20-code-point floor is the entire threshold; do not invent a confidence score or heuristic beyond it.
- **`ScreenshotTranslate`'s capture flow is untouched** — it always sends `from = model.Auto` (:790); a pinned mismatch cannot occur there. **`ScreenshotRetranslate` IS in scope and DOES get the fix** (accepted in writing, see acceptance criterion (a)7): it reaches `translateWithEngine` via `translateAllStream` with an explicit `from`/`to` (:865), so the same substitution/correction logic applies to it automatically, with no separate wiring. What's explicitly out of scope is fixing `TranslateCard`'s rendering to show the note there too — the correction happens correctly, it's just invisible on that surface today. Flag that gap as a known follow-up in the PR body, don't build it here.
- **No change to `learnLangVariant`/`langpref.Store`/`persistLangs` themselves** — the correction must route around them, not modify their contracts.
- **No change to the swap button's logic** (`swap()`, `swapLanguages`) at all — the correction never assigns `fromLang`, so there is no interaction with `swap()`'s convention to preserve; `swapPair`'s derivation and its existing `detectedFrom` input are both untouched (see WARN 5's fix in the Reuse map).

## Tests to author (T, before F writes code — in-session pipeline, no dual-review gate)

Backend (Go, `internal/translate`, real HTTP loopback fixtures per this repo's no-mocks convention, matching #11's own T (a)):
- **Real mismatch:** a pinned request ≥20 code points, loopback fixture detects a different recognized language than the pin: `DetectedFrom` set to it, `From` reports it (qualified via `resultFrom(model.Auto, detected)`), the translation used is the one the substituted-auto call produced — assert exactly ONE HTTP call was made to the fixture (proves no re-run).
- **Detection matches the pin:** fixture detects the same language as the pin, including a dialect match (pin `es-MX`, detection `es`, via `SameAs` — not a string-equality check): `DetectedFrom` empty, `From` reports the original pin unchanged.
- **Detection covers the target (the reported bug's exact shape):** pin `es-MX`, fixture's substituted-auto call detects `en`, target is `en` — result is an **identity** result (`Identity: true`, via `s.identityResult`), `From` equals `s.resultFrom(model.Auto, detected)`'s qualified value for `en` (i.e. the SAME `Qualify("en")` call a genuine auto identity result already goes through — assert this explicitly, fixes W2, fourth review), `DetectedFrom` stays empty (no redundant correction note alongside the identity note), and the result text is the source text. Exactly one HTTP call made.
- **No usable detection on the substituted call, two distinct cases (fourth review: both must be tested, they hit different gates):** (i) the fixture echoes `auto` back — `detectedSource`'s own `ok=false` path; (ii) the fixture returns an unrecognized native code (e.g. baidu's own "jp") — `detectedSource` returns `ok=true` here, so this case instead fails the separate `model.ParseLanguage` recognition gate (`recognized=false`). Both converge on the same outcome: falls back to the pin, `From` reports the pin, `DetectedFrom` empty, no correction. Exercised here on the substituted-pinned branch specifically (case (i) is already tested on the genuine-auto branch).
- **Text under 20 code points:** no substitution happens at all — assert the fixture receives `From` equal to the pin, not `auto` (proves the gate runs before dispatch, not after).
- **Chunked pinned request** (text over budget): the correction (or its absence, or the identity result) is decided once from chunk 1 via the new `pinFallback` parameter on `callEngineChunked`; every later chunk uses the decided language, not a per-chunk re-decision, and — specifically — when chunk 1's detection is unrecognized, chunks 2..N fall back to the ORIGINAL PIN (via `pinFallback`), not to re-detecting as auto (which is what a genuine, non-substituted auto request still correctly does — add a test proving THAT existing behavior is unaffected too, since `pinFallback == ""` for it). **Also test the dialect-preservation branch specifically (fixes W2, fourth review):** pin `es-MX`, chunk 1's fixture detects the bare `es` (a RECOGNIZED match via `SameAs`, not an unrecognized code) — assert chunks 2..N receive `es-MX`, not the downgraded bare `es`, proving `callEngineChunked`'s new dialect-preserving branch (not just its unrecognized-detection fallback) actually fires.
- **`resultFrom` itself is untouched:** existing tests for the ordinary (non-substituted) pinned path stay green unmodified (regression guard by construction, not a new test) — and a new test confirms `resultFrom(model.Auto, detected)` is what qualifies a corrected language, i.e. the SAME code path a genuine auto result's qualification already uses, not new qualification logic.
- `TranslateMulti`/`translateMultiEngine` and the single-engine `Translate` path both surface the corrected/identity result the same way, since both flow through the same modified `translateWithEngine`.

Frontend (vitest, pure + source-contract, matching #145's two-tier convention):
- `fromOptionLabel`: returns `t('translate.sourceAuto')` for the Auto value in every state (no detection, a detection, after a correction — it never varies), `langName(value)` for every other value, a plain pinned entry's label is never suffixed. `detectedLang.ts` and `detectedLang.test.ts` are **deleted outright** (fixes WARN 6) — do not write new tests against them; `variants.e2e.test.ts`'s `describe('detected label with the real dictionaries', ...)` block (`:34-49`, built entirely on `detectedSourceLabel`) is **deleted, not updated** (fixes WARN 3's precision note) — its two `it` blocks test behavior that no longer exists.
- `translate.detected` is removed from both `en-US.ts` and `zh-CN.ts` (fixes WARN 3) once `variants.e2e.test.ts`'s describe block is gone and `fromOptionLabel` no longer references it — confirm no other call site needs it first (the only other reference found was that now-deleted test).
- Three existing code comments cite `detectedLang.ts` as a "pure function, no bindings" precedent for an unrelated reason — `translateSession.ts:21`, `langLearn.ts:5`, `swapLangs.ts:19` (fixes WARN 3) — repoint them at `swapLangs.ts`/`targetCapability.ts` instead, so they don't cite a deleted file.
- The existing `detectedFrom` derived value (`:536`) and its test coverage for `swapPair` are unaffected — a regression test that `swapPair`'s behavior is unchanged confirms WARN 5 held.
- The new result-pane note: a pure/source-contract test that it renders from `activeResult?.detected_from` when non-empty, renders nothing when empty, and is a sibling element before the result `<textarea>` (not inside its value).
- Source-contract tests (reading `TranslateWindow.svelte`'s source, like `sourceUndo.test.ts`/`swapWindow.test.ts`): the code path that renders the result-pane note calls neither `learnLangVariant` nor `persistLangs` nor `LearnLangVariant`, and never assigns `fromLang`/`default_from` — an equivalent assertion to `swapWindow.test.ts:49`'s `expect(swap()).not.toContain('learnFromSelection')`. Also assert it does NOT call `showToast`.
- i18n catalog: `translate.sourceAuto` and `translate.translatedFromDetected` exist in both `en-US.ts` and `zh-CN.ts`; `translate.detected` is gone from both; `keys.test.ts` stays green.
- `ui-walkthrough` (U phase, real browser/headless preview per this repo's pipeline): pin a source language, translate text that doesn't match it, confirm (a) the dropdown still reads the original pin, unchanged, (b) the result-pane note appears with the actually-used language, (c) the result is a real translation (not an echo), and (d) the Auto entry in the dropdown reads "Detected" both before and after, in every state tested.

## Rigor

**in-session.** Not raised to `second-opinion` despite touching both the backend detection seam and a hand-tested user-facing window, because: the backend change is a bounded, well-precedented extension of an existing, already-tested pattern (#84's chunk-1 detection decision, extended from the auto branch to the pinned branch with one new gate) rather than new architecture; the frontend change is a label/cue change to a simplified mechanism (`fromOptionLabel` now trivial, no toast) with a strict "never teach the variant store" contract this codebase already contract-tests for an analogous case (`swap()`). Both #11 and #145 — the two closest precedents on this exact file/area — shipped at `in-session` themselves. Every design question this brief raised (the dropdown-never-changes rule, the wording, the Auto-note floor) has been settled by the principal or by architecture review, not left open into T/F — if T's RED phase turns up something genuinely new, raise rigor then rather than pre-guessing it now.

## Open decisions left in this brief (for the D-phase wireframe / principal sign-off)

1. **Settled by the principal, 2026-09-28 (no longer open): the dropdown never changes, for either Auto or a pinned correction.** A persistent muted result-pane note carries the detected-language information instead — see "Visible-cue mechanism" above. This also reverts #11's shipped Auto-row relabeling.
2. Exact wording/punctuation of the result-pane note. The principal's own wording, "Translated from auto-detected Spanish (Mexico)," is the brief's proposal; F/D may tighten it, but keep the shape (states the actual source, names it as auto-detected, lightly colored/muted).
3. **Settled here, per the second architecture review's recommendation (no longer open): the 20-code-point floor does not apply to showing the note on a genuine Auto result.** The floor exists to gate overriding a deliberate pin — a real behavior change with a real cost if wrong. Displaying what Auto already detected carries none of that risk; it's the same information the engine already used, just not previously surfaced now that #11's relabeling is reverted. So a short Auto-mode translation (under 20 code points) still shows the note if a recognized detection came back, even though a short PINNED mismatch still gets no correction and no note.
4. `TranslateCard`'s missing note (the fix applies to `ScreenshotRetranslate`, per acceptance criterion (a)7, but nothing renders it there) is a known, accepted gap for this PR to flag in its body — not pre-filed as a separate issue, and not something F should spend time deciding; just say it plainly.
