# Brief — issue #91: a short capture the detector names at the lower bar switches again (#91)

Run `0091-issue-91` · worktree `.worktrees/0091-issue-91` · branch `issue-0091-issue-91` ·
base `master` of `Performant-Labs/kai`.
Survey: `docs/handoffs/0091-issue-91/survey.md` (defect trace, reuse map, measurements).
Journal: `docs/handoffs/0091-issue-91/decisions.md`.

**Review-rigor dial: `second-opinion`.** Brief gate and diff gate each take one outside-model
review (`dual-review.sh`, glm-5.3 via Z.ai per the run's `.env`); round cap 2. **uiSurface:
false — Phase D and Phase U are N/A, declared.**

## Objective

On macOS 27's NaturalLanguage, a two-word capture like "Nos vemos" scores 0.565 as es — under
the general bar (0.8) but at/above the #54 lower bar (0.5) — and the automatic source switch
refuses it as `too_short`, so Alt-A on such a capture leaves the wrong pinned source (the exact
#8 complaint, back for the [0.5, 0.8) band). Make the smallest correct change so a **short
capture the local detector names at the lower bar switches like a confident one**, with every
other rule (dialect handling, same-language refusal, hint floor, #54 for long texts) unchanged.

## Non-goals (explicit)

- No change to `switchMinConfidence` (0.8), `switchMinConfidenceTarget` (0.5), or
  `switchMinRunes` (12) — no new constants; the measured numbers stay.
- No change to the engine, the Swift bridge, or how confidence is obtained.
- No frontend behavior change (a comment in `frontend/src/utils/sourceSwitch.ts` that describes
  the floor rule stays true — comment-only touch allowed).
- No upstream (`dtapps/kai`) involvement; no CI workflow changes.

## Settled by this brief (from the issue + survey + measurement)

1. **Design — extend the #54 promotion to short captures, before the floor.** In
   `planSourceSwitch`, the #54 promotion block moves ABOVE the code-point floor check and its
   qualifying condition becomes `rec.Covers(req.To) || d.runes < switchMinRunes` (recognized
   language required, exactly as today). Effects, each deliberate:
   - short (<12 cp) + local detection ≥ 0.5 naming a recognized language → **switches**
     (the #91 ask; "Nos vemos" = es @ 0.565 measured 2026-10-21 on this build);
   - short + in the TARGET language at ≥ 0.5 → switches (the #54 rationale — identity result
     vs. cheap undo — is *stronger* for two words; previously the floor refused these before
     the promotion could fire — a consistency gap this closes, disclosed here);
   - long + ≥ 0.5 covering the target → switches (#54, unchanged);
   - long + [0.5, 0.8) not covering the target → still `below_confidence` (unchanged);
   - short + < 0.5, or a hint (no local detection) → still `too_short` (floor unchanged);
   - unrecognized language at any confidence → never switches (unchanged).
   Rejected: (a) *drop the #92 pin only* — accepts the #8 regression for two-word captures,
   makes Alt-A "sometimes" fail again, decides nothing; (b) *lower the general bar* — switches
   long ambiguous texts #54 deliberately protects; (c) *lower/raise the rune floor* — length is
   the wrong axis, the confidence already carries the evidence (#8's own lesson).
2. **Boundary semantics**: exactly 0.5 on a short capture switches (`>=`, matching #54's
   existing boundary test); 0.49 does not. Exactly 12 code points at [0.5, 0.8) not covering
   the target → `below_confidence` (12 is not short).
3. **Test-first, existing fixtures** (T authors; F never edits tests):
   - unit (`source_switch_test.go`): the #91 arm as a boundary table in the #54 pattern —
     short/0.565/not-target → switch with the full pair (es-MX -> en incl. variant selection);
     short/0.5 → switch; short/0.49 → `too_short`; long/0.565/not-target → `below_confidence`;
     short/0.6/target → switch (#54-for-short); short unrecognized/0.6 → `too_short`; hint
     short → `too_short` (regression-guard the unchanged floor). The existing
     `TestPlanSourceSwitchFloorGuardsAnswersWithoutConfidence` case "short low-confidence local"
     (0.5 @ 11 cp) flips from `too_short` to switch — that pin was the old decision; update it
     and its comment to cite #91.
   - darwin (`source_switch_real_darwin_test.go`): replace the #92 pin
     (`TestRealDetectorDoesNotSwitchAnUnsureTwoWordCapture`) with the #91 expectation — "Nos
     vemos" under an en pin switches es-MX -> en on the real detector; header cites the
     re-measurement (0.565, 2026-10-21, this macOS 27 build) and keeps the "finding, not a
     flake" doctrine for future drift.
4. **Log contract (#16) unchanged**: the decision line's fields and vocabulary are the wire
   contract; a switched short capture simply logs `reason=switched` with its own confidence.
   No new keys.
5. **CHANGELOG**: one `[Unreleased]` line for the user-visible behavior (#91), and the
   `sourceSwitch.ts` comment stays true (comment-only).
6. **What a unit test cannot show** (must be said plainly in the PR): the fakes pin the rule;
   real NaturalLanguage behavior — that "Nos vemos" really scores 0.565 ≥ 0.5 on macOS 27 and
   that the bridge path end-to-end switches — is proven only by the darwin suite on a Mac with
   the bridge loaded (run green in this run on the release Mac), never in CI (Linux runners);
   and the real signed app doing a live Alt-A capture is owner-verified, not claimed.

## Acceptance criteria

- [ ] AC1: A short capture (<12 code points) with a local detection naming a recognized
      language at confidence ≥ `switchMinConfidenceTarget` switches exactly like a confident
      one — variant selection (es → es-MX), target replacement (`To = req.From`), and the
      same-language / Auto-source / setting-off refusals all apply unchanged.
- [ ] AC2: Boundaries: 0.5 switches; 0.49 → `too_short`; 12 code points at [0.5, 0.8) not
      covering the target → `below_confidence`; ≥ 0.8 at any length unchanged.
- [ ] AC3: The floor still guards hints and sub-0.5 local answers: short hint never switches;
      short local < 0.5 → `too_short`; unrecognized language never switches.
- [ ] AC4: #54 for long texts unchanged (the existing table test passes untouched except where
      it pins the flipped short case).
- [ ] AC5: The darwin #92 pin is replaced by the #91 expectation and the suite is GREEN on the
      real detector on this Mac (run in t-green; CI cannot run it).
- [ ] AC6: The #16 decision line is shape-identical (one line per decision, same fields); the
      switched short capture logs `reason=switched`.
- [ ] AC7: `CHANGELOG.md` has the #91 `[Unreleased]` line; `sourceSwitch.ts` comment remains
      true (comment-only if touched).
- [ ] AC8: No file outside `internal/translate/source_switch.go`,
      `internal/translate/source_switch_test.go`,
      `internal/translate/source_switch_real_darwin_test.go`, `CHANGELOG.md`,
      `frontend/src/utils/sourceSwitch.ts`, this run's `docs/handoffs/0091-issue-91/` is
      modified.

## Test command

The plugin's `test.unit.command` (builds the Swift bridge, Go suite incl. darwin tags on this
Mac, frontend with its own NODE_OPTIONS handling, release harnesses). RED = the new/flipped
cases fail at master; GREEN = all pass with the implementation.
