# Survey — issue #91: sub-floor sample "Nos vemos" no longer switches (NaturalLanguage under the 0.8 bar on macOS 27)

Run `0091-issue-91`. Corpus read for this story: `internal/translate/source_switch.go` (the whole
decision), `internal/translate/source_switch_test.go` (unit fixtures), 
`internal/translate/source_switch_real_darwin_test.go` (real-detector suite), 
`internal/model/model.go` (SwitchReason constants), `internal/translate/service.go:70-111` (the
`detect` seam: `svc.detect = engine.DetectLanguage`, NaturalLanguage through the Swift bridge),
`frontend/src/utils/sourceSwitch.ts` (comment describing the rule), `CHANGELOG.md` ([Unreleased] +
the #8 lines), issue #91 + its lineage #8 / #16 / #46 / #54 / #81 / #92, and the #78 run journal
(dispatch mechanics + gate configuration).

## The defect, precisely

`planSourceSwitch` (source_switch.go:95-153):

1. `detectForSwitch` asks the local detector. "Nos vemos" (9 code points) → es @ 0.565 on this
   macOS 27 build (re-measured 2026-10-21, throwaway probe; #92 measured the same 0.565 on
   2026-10-19). Under `switchMinConfidence` (0.8) it returns reason `below_confidence`.
2. `confidentLocal := reason == "" && d.by == "local"` is false, so the floor check
   (`d.runes < switchMinRunes`, 12) refuses the 9-code-point text as `too_short`.
3. The #54 promotion (source_switch.go:129-133) would accept 0.565 ≥ `switchMinConfidenceTarget`
   (0.5) — but only when the detection covers `req.To` (the TARGET). An en→fr window (the #8
   Alt-A scenario) gets nothing.

So the #8 goal ("a captured word or two the detector is sure of must not be translated from the
wrong pinned language and handed back unchanged") regressed for exactly the two-word captures
NaturalLanguage scores in [0.5, 0.8): the pin stays, the capture is "translated" en→fr. #92 pinned
this measured behavior so the change would be a decision, not an accident — this run is that
decision (#91).

## Reuse & Analogous-Feature map

| Need | Closest existing thing | Extend or new? |
|------|------------------------|----------------|
| A lower confidence bar that still switches | `switchMinConfidenceTarget` (0.5) + the #54 promotion block (source_switch.go:127-133) | **Extend.** The promotion gains one qualifying arm (`|| d.runes < switchMinRunes`) and moves ABOVE the floor check so a short capture it accepts is not first refused `too_short`. |
| Floor exemption for short captures | `confidentLocal` (#8, source_switch.go:123-126) | Reuse as-is; the extended promotion makes a short lower-bar detection arrive at the floor with `reason == ""`, the same way a confident one does. |
| Boundary table tests | `TestPlanSourceSwitchTextInTheTargetLanguageNeedsLessConfidence` (source_switch_test.go:495-516) | Extend the pattern to the short-capture arm (0.5 in / 0.49 out; long-not-target stays `below_confidence`). |
| The real-detector execution gate | `source_switch_real_darwin_test.go` (runs only where the bridge loads; never in CI) | Replace the #92 pin with the #91 expectation + re-measured header. |

**Recommendation: extend, everywhere.** No new file, no new constant, no engine/swiftbridge
change. The 0.5 bar is already shipped and measured (#54: "a misplaced accent alone drops a clear
Spanish sentence to ~0.74"; #92: "Nos vemos" = 0.565), and its rationale transfers: for a word or
two, not switching hands the capture to the engine "translated" from the wrong pinned language —
worthless — while a wrong switch is cheap and undoable (#54's doctrine; the same-language and
variant rules still apply downstream unchanged).

## Test-wiring facts

- `svc.detect` is a field seam (service.go:70-72); unit tests inject `fakeDetector`. The fakes
  pin the RULE; only the darwin suite exercises real NaturalLanguage, and only on a Mac with the
  bridge — CI (Linux) can never run it. The PR must say this plainly.
- The plugin's suite command (`.aftersight/pipeline.config.json` test.unit.command) builds the
  swift bridge then runs `go test ./internal/... ./pkg/...` + frontend + release harnesses — on
  this Mac it includes the darwin tests, so RED and GREEN are both observable here for the real
  detector too.
- Baseline verified today at master `ab5ea85`: darwin real-detector suite 5/5 PASS (including
  the #92 pin); translate package untouched since `93f6601` (test #92).

## Files this story can touch (scope guard)

- `internal/translate/source_switch.go` — the promotion + floor order + comments.
- `internal/translate/source_switch_test.go` — the floor/short-capture tests (#91 arm).
- `internal/translate/source_switch_real_darwin_test.go` — replace the #92 pin, re-measured header.
- `CHANGELOG.md` — one `[Unreleased]` line (#91).
- `frontend/src/utils/sourceSwitch.ts` — comment-only (it describes the floor rule; #81 set the
  precedent for keeping it true).
- Nothing else: no engine, no swiftbridge, no model constants, no UI behavior.
