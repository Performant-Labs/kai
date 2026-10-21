Verdict: PASS

# Handoff-A: Phase 3 — up-front plan review (issue #91: short capture at the lower bar switches again)

**Date:** 2026-10-21
**Branch:** `issue-0091-issue-91` (run `0091-issue-91`)
**Brief reviewed:** `docs/handoffs/0091-issue-91/brief.md` · **Reuse map:** `docs/handoffs/0091-issue-91/survey.md` §Reuse & Analogous-Feature map · **Wireframe:** N/A (uiSurface: false, declared)
**Verdict:** PASS

## Summary

PASS. The settled design is the smallest change that fixes the #91 defect: the #54 promotion
block (source_switch.go:129-133) moves above the code-point floor (:123-126) and gains exactly
one qualifying arm, `|| d.runes < switchMinRunes`. I traced every input cell of the decision
function against the plan; the two intended behavior changes (the #91 arm and the disclosed
short-target-language consistency arm) are exactly as the brief describes, everything else —
hints, sub-0.5, unrecognized, ≥12 cp, confident, blank/links, Auto-source, setting-off — is
unchanged, and all downstream guards (same-language :139/:149, variant selection :147, target
replacement :152) are untouched. The plan extends the analogous object the reuse map names
(the #54 promotion) rather than building a parallel path, and the three rejected alternatives
are correctly rejected on grounds the existing tests already pin. Two warns: one behavior cell
the effects list does not enumerate (a reason change, not a switch change — see finding 1), and
three comments that become false statements if F leaves them (finding 2). Neither is a block.

## Findings

| # | Severity | Plan element | Dimension | Finding | Suggested fix |
|---|----------|--------------|-----------|---------|---------------|
| 1 | warn | Brief §Settled 1 effects list ("Effects, each deliberate:") | cross-cutting / disclosure completeness | The effects list misses one changed cell: short + local [0.5,0.8) + recognized detection that **covers the PIN** (`req.From`). Today the floor fires first and logs `too_short`; after the move the promotion clears `reason`, the floor is skipped, and `detected.Covers(req.From)` (:139) refuses it as `same_language`. Still no switch — AC1's "same-language refusals apply unchanged" holds for the outcome but not for the logged reason — and the #16 vocabulary is unchanged (both are existing `SwitchReason*` values, model.go:391-399). A disclosure gap, not a correctness one. | Add one row to T's boundary table (short / 0.6 / detected = the pin → no switch, reason `same_language`) and, at O's discretion, one clause in the brief's effects list. No brief amendment required to proceed. |
| 2 | warn | Brief §Settled 5 / AC7 (comment accuracy) | pattern consistency (docs-in-code) | Three comments describe the current floor semantics and become **false** after the change: (a) `source_switch.go:39-45` — "a local detection under switchMinConfidence (the #54 lower bar included) is not one either"; (b) `source_switch.go:117-122` — "a local detection under the confidence bar (the #54 promotion **below** included)" (also "below" becomes "above" after the move); (c) `frontend/src/utils/sourceSwitch.ts:5-7` — "the code-point floor — which guards only answers without a confident local detection, #8". AC7 makes the TS comment optional ("if touched"); it is not optional — untouched it lies. The two Go comments ride AC8's file scope but the brief never names them. | Treat all three as required comment-only edits (already inside AC8's file list + AC7); name them in F's instructions so none is missed. |
| 3 | nit | the moved block's variable choice | naming | The brief's formula `d.runes < switchMinRunes` is exactly right and must stay that way: `d.runes` is set at :103 from the trimmed text; the `switchDetail` returned by `detectForSwitch` never sets `runes` (see :173-183), so `det.runes` is always 0 and would make every text qualify as "short". Guard note for F, no plan change. | Keep `d.runes`; F must not "simplify" it to `det.runes`. |
| 4 | nit | `confidentLocal := reason == "" && d.by == "local"` (:123) | naming | After the move, a promoted 0.565 short detection reaches this predicate with `reason == ""`, so `confidentLocal` becomes true of detections that are not confident — the name drifts from its #8 meaning. Behavior is the designed one; the name/comment refresh belongs in the same edit (in scope per survey's scope guard: "the promotion + floor order + comments"). | Rename or reword in the implementation (e.g. fold the promotion's outcome into the floor's guard comment); do not leave the #8 comment above it as-is. |

## Why the design is the smallest correct change (verified)

The seams, verbatim. The floor and the promotion today, in order:

```text
source_switch.go:123-126
	confidentLocal := reason == "" && d.by == "local"
	if !confidentLocal && d.runes < switchMinRunes {
		return no(model.SwitchReasonTooShort)
	}
source_switch.go:129-133
	if reason == model.SwitchReasonBelowConfidence && det.confidence >= switchMinConfidenceTarget {
		if rec, known := recognized(det.lang); known && rec.Covers(req.To) {
			detected, reason = rec, ""
		}
	}
```

- **The move is required, not stylistic.** Widening the promotion's condition *without* moving
  it above the floor is a no-op: for a short non-confident detection the floor returns
  `too_short` at :125 before the promotion at :129 is reached. The survey's reuse map states
  this ("moves ABOVE the floor check so a short capture it accepts is not first refused
  `too_short`") and it is correct.
- **The reorder is behavior-neutral for long text.** At ≥12 code points the floor predicate
  (`d.runes < switchMinRunes`, :124) is false, so promotion-before-floor and
  floor-before-promotion take the same path. #54's long-text behavior is untouched (AC4).
- **The mechanism is the #8 exemption's own shape.** A promoted capture arrives at the floor
  with `reason == ""`, `by == "local"` — exactly what a confident detection does today — so the
  floor's existing guard admits it. No new exemption path is carved; the reuse map's "reuse
  as-is" for `confidentLocal` is accurate.
- **Hints can never ride the promotion.** `detectForSwitch` returns `below_confidence` only
  from the local-detector branch (:176-178); the hint branch returns `""` or `no_detection`
  (:184-191). The promotion's `reason == model.SwitchReasonBelowConfidence` gate therefore
  excludes hints structurally — AC3's hint guarantee holds without any new code.
- **Unrecognized languages stay out.** The promotion requires `recognized(det.lang)` (:130);
  an `xx` detection at [0.5,0.8) arrives as `below_confidence` with `known == false` and is not
  promoted, then the floor (short) or the `reason != ""` return (:134-136) refuses it — the
  brief's "short unrecognized/0.6 → `too_short`" is the correct expectation.
- **Boundary semantics check out against the code.** Exactly 0.5 switches via the existing
  `>=` (:129); 0.5 is binary-exact so no float hazard; the #54 boundary-table precedent is
  `{"detected = target, exactly at the lower bar", switchMinConfidenceTarget, model.ESMX, true}`
  (source_switch_test.go:504) — the brief's "matching #54's existing boundary test" is
  accurate. Exactly 12 code points: `12 < 12` is false, so the floor passes and an unpromoted
  [0.5,0.8) not-target detection returns `below_confidence` — and the existing
  "long low-confidence local" case (source_switch_test.go:166-169, exactly 12 cp @ 0.5,
  not-target) already pins that cell and does **not** flip.
- **Exactly one unit case flips, as the brief claims.** I traced every unit test that combines
  short text with a local detection: the only case in [0.5,0.8) under 12 cp is
  source_switch_test.go:162-165 — `&fakeDetector{lang: "es", conf: 0.5, ok: true}` on
  `"Hola, amigo"` (11 cp), From=EN To=FR — which the promotion (`0.5 >= 0.5`, recognized es,
  `11 < 12`) now switches to es-MX -> en. The brief names precisely this case and its flip.
  (`TestPlanSourceSwitchLowConfidenceNeverSwitches` :203-216, `...EngineHintIsOnlyTheFallback`
  :281-284 at 0.4, `...LowerBarLeavesTheOtherRulesAlone` :528-539 all use long text or <0.5 —
  unchanged.)
- **The darwin pin must be replaced, not amended.** `TestRealDetectorDoesNotSwitchAnUnsureTwoWordCapture`
  (source_switch_real_darwin_test.go:72-78) asserts `got.Switched || got.Reason != model.SwitchReasonTooShort`
  fails — under the plan "Nos vemos" (es @ 0.565, 9 cp, re-measured 2026-10-21) switches. The
  brief's replacement + re-measured header is the only consistent move, and the #92 comment's
  own doctrine ("a change to the floor or the bar for such captures is a decision, not an
  accident", :70-71) is satisfied by this run being exactly that decision.
- **#16 log contract is untouched.** The decision line (:79-83) reads `plan.Reason` and
  `d` fields; a promoted short switch logs `reason=switched`, `detected=es`,
  `confidence=0.565`, `runes=9`, `by=local` — same keys, same one-line-per-decision shape, no
  new constants. AC6 matches the code.
- **Rejected alternatives are correctly rejected.** (b) *lower the general bar* would flip
  `{"detected is not the target, 0.74 stays below the general bar", 0.74, model.FR, false}`
  (source_switch_test.go:507) — a protection #54 deliberately added. (c) *move the floor*
  would unguard the hint rows pinned at source_switch_test.go:141-159 (a hint carries no
  confidence; the floor is its only guard). (a) *drop the #92 pin only* leaves the measured
  defect in place. All three rejections are grounded in existing pinned behavior, not taste.

## Scope check

No scope creep: the touch list (AC8) is identical to the survey's scope guard (survey.md:59-67),
confined to the one file the header itself names as "the ONE place that decides it"
(source_switch.go:19) plus its two test surfaces, CHANGELOG, and a comment-only TS touch. No
new constant, no engine/swiftbridge change, no frontend behavior change, no CI change. No
repo-specific gate applies (no migrations, no workspace crossings, no root-scope hooks).

## Notes for O

None required for PASS. Optional, at O's discretion: fold finding 1's one-line disclosure into
the brief's effects list so the "each deliberate" enumeration is complete before T authors the
table (T should add the pinning row either way).

## Patterns referenced

1. `internal/translate/source_switch.go` — the decision function under review (:95-153), its
   constants (:39-62), `detectForSwitch` (:168-192), and the #16 decision line (:71-85).
2. `internal/translate/source_switch_test.go` — the #54 boundary table (:495-516) and the
   floor test whose one case flips (:130-176).
3. `internal/translate/source_switch_real_darwin_test.go` — the #92 pin to be replaced
   (:68-78) and the suite's measurement doctrine (:14-24).
4. `internal/model/model.go` — `SwitchReason*` vocabulary (:391-399), `Covers`/`SameAs`
   (:114-151), `SelectableOr` (:199-207).
5. `frontend/src/utils/sourceSwitch.ts` — the frontend half's floor-rule comment (:1-12).
