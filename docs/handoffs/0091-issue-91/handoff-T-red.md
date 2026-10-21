Verdict: RED

# Handoff-T-red: Phase 4 — issue #91 (a short capture the detector names at the lower bar switches)

**Date:** 2026-10-21
**Branch:** `issue-0091-issue-91` (run `0091-issue-91`, worktree `.worktrees/0091-issue-91`, base `ab5ea85`)
**Brief / wireframe reviewed:** `docs/handoffs/0091-issue-91/brief.md` (§Settled 1–3, AC1–AC8) ·
`docs/handoffs/0091-issue-91/survey.md` (§Reuse & Analogous-Feature map) · wireframe N/A
(uiSurface: false, declared)

## A precondition

Confirmed: A returned **PASS** on the plan (`docs/handoffs/0091-issue-91/handoff-A.md`, 0 block /
2 warn / 2 nit). Warn 1 (the unlisted `same_language` reason cell) is pinned as a table row below.
Warn 2 (three comments that become false) and nits 3–4 (keep `d.runes`; reword `confidentLocal`)
are F's production-edit duties, restated under "Not pinned by tests" so they are not lost.

## Tests authored

Test-only edits, exactly two files; no production code, no skips, no `-skip` flags.

### Unit — `internal/translate/source_switch_test.go`

**New: `TestPlanSourceSwitchShortCaptureAtTheLowerBarSwitches`** — the #91 boundary table in the
#54 table pattern (`TestPlanSourceSwitchTextInTheTargetLanguageNeedsLessConfidence`,
source_switch_test.go:495-516). Tier: unit, through the `svc.detect` fake seam — the cheapest tier
that can pin the rule; real-detector behavior is the darwin suite's job. Fixtures: `"Nos vemos"`
(9 cp, asserted) and `"Hola, amigos"` (12 cp, asserted) — the floor's two sides.

| Row (all short = 9 cp unless said) | Pins | Backs |
|---|---|---|
| es @ **0.565**, not target → **switch es-MX -> en** (variant selection + target replacement asserted) | the #91 ask, the full pair | AC1 |
| exactly **0.5** → switch | the `>=` boundary, matching #54's own boundary row | AC2 |
| **0.49** → `too_short` | the floor keeps sub-0.5 answers | AC2, AC3 |
| **12 cp** @ 0.565, not target → `below_confidence` | 12 is not short; #54's long-text cell | AC2, AC4 |
| **0.6, target** (To=es-MX) → switch es-MX -> en | #54-for-short, the disclosed consistency arm | AC1 |
| **unrecognized** (xx) @ 0.6 → `too_short` | an unrecognized language never promotes | AC3 |
| **hint** (detector down, hint es-MX) → `too_short` | the floor is a hint's only guard | AC3 |
| **0.6, detection covers the pin** (From=es-MX) → no switch, reason `same_language` | **A's warn-1 row**: the logged reason changes (`too_short` -> `same_language`), the answer stays no-switch | AC1 refusals, AC6 vocabulary |

**Flipped: `TestPlanSourceSwitchFloorGuardsAnswersWithoutConfidence`** — the "short
low-confidence local" case (0.5 @ 11 cp, EN->FR) now wants the switch (`es-MX -> en`) instead of
`too_short`; its comment cites #91. That test's other cases (hint 11/12 cp ± padding, 12-cp
`below_confidence`, blank text) are untouched and must keep passing — they are the AC3/AC4 guards.
The test's function comment was updated in the same edit so it stays true post-#91 (the floor
guards what the lower bar does not accept: sub-0.5 or unrecognized — not "under the confidence
bar").

### Darwin — `internal/translate/source_switch_real_darwin_test.go`

**Replaced: the #92 pin** `TestRealDetectorDoesNotSwitchAnUnsureTwoWordCapture` →
`TestRealDetectorSwitchesATwoWordCaptureAtTheLowerBar`: "Nos vemos" under an en pin must switch
**es-MX -> en on the real detector** through the loaded bridge. Tier: real-detector execution
gate — no lower tier can prove NaturalLanguage's confidence or the bridge path (CI's Linux runners
can never run it; AC5 is observed in t-green on this Mac). The test's header cites the
re-measurement (es @ 0.565, 2026-10-21, this macOS 27 build; #92 measured the same 0.565 on
2026-10-19) and keeps the finding-not-a-flake doctrine, with the drift threshold now 0.5. The
file header's #8 paragraph was corrected in the same edit (comment-only): it claimed both
sub-floor samples are "pinned by the floor exemption for confident local detections" — false for
"Nos vemos" once #91 lands. **No other darwin test was touched or weakened.**

## RED confirmation

Run from the worktree root (scoped; the plugin's full `test.unit.command` is the gate's, not run
here):

```
go test -vet=off -run 'TestPlanSourceSwitch|TestRealDetector' ./internal/translate/ -count=1
```

Result: **FAIL, exit 1** — exactly the three intended test functions fail; every failure is the
feature assertion (the floor's `too_short` where #91 wants a switch, or warn-1's `same_language`),
no compile or setup errors; every other test in the run passes. Verbatim failing output (the
decision line above each unit/darwin failure is part of the run's output and is the evidence the
right cell was hit):

```text
2026/10/21 06:04:32 INFO [dynamic-bridge] Swift bridge loaded path=/Users/andreangelantoni/kai-pipeline-run-C/.worktrees/0091-issue-91/pkg/swiftbridge/libkai_bridge.dylib
…
2026/10/21 06:04:32 INFO [source switch] decision reason=too_short detected=es confidence=0.564932107925415 runes=9 by=local from=en to=fr result_from="" result_to="" version=dev commit=unknown
--- FAIL: TestRealDetectorSwitchesATwoWordCaptureAtTheLowerBar (0.00s)
    source_switch_real_darwin_test.go:81: Nos vemos: got {Switched:false From: To: Reason:too_short}, want es-MX -> en (the #91 lower bar for a short capture)
…
2026/10/21 06:04:32 INFO [source switch] decision reason=too_short detected=es confidence=0.5 runes=11 by=local from=en to=fr result_from="" result_to="" version=dev commit=unknown
--- FAIL: TestPlanSourceSwitchFloorGuardsAnswersWithoutConfidence (0.00s)
    source_switch_test.go:165: short lower-bar local: got {Switched:false From: To: Reason:too_short}, want es-MX -> en (#91)
…
--- FAIL: TestPlanSourceSwitchShortCaptureAtTheLowerBarSwitches (0.00s)
    source_switch_test.go:587: the #91 capture: 0.565, not the target: Switched = false (reason "too_short"), want true
    source_switch_test.go:587: exactly at the lower bar: Switched = false (reason "too_short"), want true
    source_switch_test.go:587: short in the target language (#54 for short): Switched = false (reason "too_short"), want true
    source_switch_test.go:591: short detection covers the pin: Reason = "too_short", want "same_language"
FAIL
FAIL	cnb.cool/dtapp/kai/internal/translate	0.960s
FAIL
```

Why each failure is the RIGHT reason:

- **Table (:587/:591):** the four failing rows are precisely the #91 behavior cells — three
  switches and warn-1's reason change — each refusing with today's `too_short`. The four guard
  rows (0.49, 12 cp, unrecognized, hint) raised **no** error at this baseline, by design: they pin
  unchanged cells and will catch F over-widening (dropping the floor, promoting sub-0.5, letting
  12 cp count as short, letting a hint ride the promotion).
- **Flip (:165):** fails on the floor's refusal of the exact brief-named cell (log line:
  `confidence=0.5 runes=11 by=local`), the old #92-era pin this run deliberately reverses.
- **Darwin (:81):** a real run, not a skip — the bridge loaded, and this run's own decision line
  reads `detected=es confidence=0.564932107925415 runes=9 by=local … reason=too_short`: the
  re-measured 0.565 ≥ 0.5, live under the en pin, refused by current code. A skip would have been
  a coverage hole and an invalid RED; it is not what happened.

## Not pinned by tests (for F / S, so nothing is lost)

- A's warn-2 comment-only edits are **required of F**, not optional: `source_switch.go:39-45`,
  `source_switch.go:117-122`, `frontend/src/utils/sourceSwitch.ts:5-7` all lie if untouched.
- A's nits: keep `d.runes` (`det.runes` is always 0 — `detectForSwitch` never sets it); reword
  `confidentLocal`'s name/comment in the same edit.
- AC5's "GREEN on this Mac" is observed in t-green (darwin suite), AC7 (CHANGELOG line, TS comment
  truth) and AC8 (scope) are review-time checks, not unit-pinnable here.

## Ready for F

**RED is valid. F may implement against these tests.** Expected change surface:
`internal/translate/source_switch.go` only — move the #54 promotion block above the floor check
and add the qualifying arm `|| d.runes < switchMinRunes` (recognized language still required).
The two test files are staged by explicit path; the handoffs are left untracked per run
convention. No production code was touched.
