# Handoff-T-rework3: PR #100 round 3 — pinning the ACTIVE "Smoke gate bypass" RED

**Date:** 2026-10-20
**Branch:** issue-0078-issue-78 (worktree `.worktrees/0078-issue-78`)
**Scope:** `scripts/release.test.sh` only (+ this handoff, + the decisions entry). No production code touched.

## The finding (verbatim, PR #100 PR-Agent, finding_id `68ce73cf4590`, state ACTIVE)

> **Smoke gate bypass**
>
> If a run with `--smoke` leaves state `.smoke` as `awaiting-owner` and exits before marking phase `smoke` done, a later resume with `--publish` but without `--smoke-done` falls through the no-flag branch and writes `.smoke = "skipped"` before continuing to publish. This silently skips the manual smoke matrix, allowing a publish without the owner's smoke record. The resume should refuse unless `--smoke-done` is supplied when the recorded state is `awaiting-owner`.

Anchored at `scripts/release.sh:646-649`. Verbatim excerpt (`sed -n '624,650p' scripts/release.sh`, the whole phase for context; the defect is the final fall-through, last 5 lines):

```bash
ph_smoke() {
  printf '\n== phase 11: smoke\n'
  if phase_done smoke; then say "  .. smoke already recorded ($(jq -r '.smoke // ""' "$state" 2>/dev/null))"; return 0; fi
  local v
  if [[ $smoke_done -eq 1 ]]; then
    v="done (owner-asserted $(date +%Y-%m-%d))"
    st_update '.smoke = $v' --arg v "$v"
    issue_comment smoke done "smoke matrix: $v — recorded by --smoke-done"
    mark_done smoke
    say "  ok  smoke recorded: $v"
    return 0
  fi
  if [[ $smoke -eq 1 ]]; then
    st_update '.smoke = $v' --arg v "awaiting-owner"
    issue_comment smoke awaiting-owner "smoke matrix awaiting the owner: run docs/release-checklist.md §Manual smoke matrix on $statedir/Kai.app, then resume with --smoke-done"
    say ""
    say "Smoke matrix awaiting the owner (docs/release-checklist.md, Manual smoke matrix)."
    say "Run it on $statedir/Kai.app, then record it:"
    say "resume: scripts/release.sh $tag --smoke-done --publish"
    notify "smoke matrix awaiting the owner"
    exit 0
  fi
  st_update '.smoke = $v' --arg v "skipped"
  issue_comment smoke skipped "smoke: skipped (--smoke not given this run; recorded per the checklist)"
  mark_done smoke
  say "  .. smoke recorded skipped (no --smoke)"
}
```

The `--smoke` branch records `awaiting-owner` and `exit 0`s **without** `mark_done smoke`, so the
phase re-enters on resume; with `smoke_done=0` and `smoke=0` it falls through to the `skipped`
write, marks the phase done, and (under `--publish`) publishes.

## The case: `smokeresume` (scripts/release.test.sh, after the `stalemarker` block)

Setup: `mkscn smokeresume; rrun v0.5.1 --smoke` — a `--smoke` run stopped awaiting the owner
(exit 0, `.smoke == "awaiting-owner"`, `.phases.smoke.done == false`; all three setup assertions
PASS today, pinning the finding's premise). Then, in order (the no-flag variant must come first:
a `--publish` run would escalate the recorded posture and refuse the later default-posture run
for an unrelated reason):

1. **Variant 1 — re-run with NO flags** (`rrun v0.5.1`): must refuse (exit non-zero), the refusal
   must name `--smoke-done`, `.smoke` must stay `awaiting-owner`, the phase must stay not-done,
   and no `smoke: skipped` checklist comment may be posted.
2. **Variant 2 — the finding's exact shape** (`rrun v0.5.1 --publish`, no `--smoke-done`): must
   refuse naming `--smoke-done`, never consult the AI (owner gate, not a failure), keep
   `awaiting-owner` / phase not-done, and publish nothing: `.phases.publish.done` false, zero
   `release edit` in `gh.log`, the draft's `isDraft` still true.
3. **Valid transition preserved** (`rrun v0.5.1 --publish --smoke-done`): exits 0, records
   `.smoke` = `done (owner-asserted …)`, reaches closeout — the refusals must leave the state
   machine recoverable exactly the documented way.

The other valid transitions stay pinned by the untouched existing cases: absent+no-flag →
`skipped` (scenario `default`), absent+`--smoke` → `awaiting-owner` exit 0 (scenario `smoke`).

## Finding → assertion map

| Finding clause | Assertions (label → pinned behavior) |
|---|---|
| "falls through the no-flag branch and writes `.smoke = \"skipped\"`" | `no-flag: awaiting-owner is not downgraded to skipped`; `no smoke-skipped comment is posted`; `no-flag: the smoke phase stays not-done` |
| "a later resume with `--publish` but without `--smoke-done`" (same fall-through under publish) | `publish-resume: awaiting-owner still stands`; `publish-resume: the smoke phase stays not-done` |
| "before continuing to publish … allowing a publish without the owner's smoke record" | `a --publish resume without --smoke-done is refused` (exit non-zero); `publish never runs without the owner's smoke record` (`.phases.publish.done` false); `no release edit …` (gh.log count 0); `the draft stays a draft` (`.isDraft` true) |
| "The resume should refuse unless `--smoke-done` is supplied" | both `exit_nonzero` refusals + `the refusal tells the owner to pass --smoke-done` / `that refusal also names --smoke-done`; `the owner-gate refusal never consults the AI` (stop-for-owner class, matching every other stop in the harness) |
| "exits before marking phase `smoke` done" (premise + recovery) | the 3 setup assertions + `--smoke-done finally resumes and completes` / `the owner assertion is recorded after the refusals` / `the run reaches closeout` |

Expected fix shape for F (contract, not implementation mandate): in `ph_smoke`, before the
no-flag fall-through, a recorded `.smoke == "awaiting-owner"` with `smoke_done=0` is a
stop-for-owner naming `--smoke-done` (a `do_stop` satisfies every assertion: non-zero exit,
message, checklist comment, no state write, no AI). An `--smoke`-re-run (awaiting → awaiting)
stays legal — nothing in this case pins it either way; only the silent **skipped** downgrade and
the publish-through are the defect.

## RED confirmation (verbatim)

Baseline before the edit (same tree, `15b6c8f`): `bash scripts/release.test.sh` → exit 0,
**199 ok / 0 FAIL / PASS** (matches the journal's round-2 count).

After the edit: `bash scripts/release.test.sh` → **exit 1, 205 ok / 13 FAIL / FAILED**.
Every existing assertion still passes (205 − 6 new passing lines = the baseline 199). All 13
FAILs are the new case — verbatim:

```text
FAIL PR100-R3: a no-flag re-run is refused, not downgraded (exit 0, wanted non-zero)
FAIL PR100-R3: the refusal tells the owner to pass --smoke-done (wanted /--smoke-done/)
FAIL PR100-R3: no-flag: awaiting-owner is not downgraded to skipped (jq '.smoke' gave 'skipped', wanted 'awaiting-owner')
FAIL PR100-R3: no-flag: the smoke phase stays not-done (jq '.phases.smoke.done' gave 'true', wanted 'false')
FAIL PR100-R3: no-flag: no smoke-skipped comment is posted (did not want /smoke: skipped/ in comments.log)
FAIL PR100-R3: a --publish resume without --smoke-done is refused (exit 0, wanted non-zero)
FAIL PR100-R3: that refusal also names --smoke-done (wanted /--smoke-done/)
FAIL PR100-R3: publish-resume: awaiting-owner still stands (jq '.smoke' gave 'skipped', wanted 'awaiting-owner')
FAIL PR100-R3: publish-resume: the smoke phase stays not-done (jq '.phases.smoke.done' gave 'true', wanted 'false')
FAIL PR100-R3: publish never runs without the owner's smoke record (jq '.phases.publish.done' gave 'true', wanted 'false')
FAIL PR100-R3: no release edit without the owner's smoke record (gh.log =~ /release edit/ matched 1, wanted 0)
FAIL PR100-R3: the draft stays a draft (jq '.isDraft' gave 'false', wanted 'true')
FAIL PR100-R3: the owner assertion is recorded after the refusals (jq '.smoke | startswith("done (owner-asserted")' gave 'false', wanted 'true')
```

Right-reason check: the setup lines PASS (the `--smoke` stop, `awaiting-owner` recorded, phase
not done — no setup or import error; the scenario reaches phase 11 and exercises exactly the
defect path), and every FAIL names the defect's own observables: `exit 0` where a refusal is
required, `'skipped'` where `awaiting-owner` must stand, `publish.done true` / `release edit`
matched / `isDraft false` where publish must not run, and the owner assertion lost after the
downgrade. Not one FAIL is a typo, missing import or harness artifact.

## Quality / proportionality

One scenario, 19 assertions, cheapest sufficient tier (the existing shell harness — same tier as
every other release.sh contract), no duplication of the existing `smoke`/`default` scenarios
(those keep the valid transitions; this case pins only the forbidden one and the recovery).
Production code untouched: `git diff --stat` = `scripts/release.test.sh | 38 insertions(+)`
(plus this handoff and the decisions entry).

## Ready for F

RED is valid. F may implement the refusal in `ph_smoke` against these tests.
