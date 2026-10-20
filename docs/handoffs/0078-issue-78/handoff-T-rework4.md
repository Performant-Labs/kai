# Handoff-T-rework4: PR #100 round 4 — pinning the 2 ACTIVE findings RED

**Date:** 2026-10-20
**Branch:** issue-0078-issue-78 (worktree `.worktrees/0078-issue-78`)
**Scope:** `scripts/release.test.sh` only (+ this handoff, + the decisions entry). No production
code touched (`git diff --stat` = `scripts/release.test.sh | 107 insertions(+), 9 deletions(-)`).

## The findings (verbatim, PR #100 PR-Agent, both state ACTIVE at head 4960559)

**R4a — "Post-tag source mutation"**, finding_id `3fe580bc5f88`, `scripts/release.sh:560-621`:

> **Post-tag source mutation**
>
> The tag is pushed in phase 9 before the local phase, but the bindings-drift handler can run during phase 10 and `handler_regen_bindings` commits regenerated `frontend/bindings` to the still-checked-out release branch. The local build then runs from a tree that differs from the already-pushed `vX.Y.Z` tag, so the published artifact would not correspond to the tagged commit, conflicting with the documented rule that the git tag is the source of truth. The fix commit is also left on an already-merged branch and is not part of the tag. The diff does not show `release-local.sh` re-checking-out the tag; if it does, the regeneration is discarded instead.

**R4b — "Masked exit status"**, finding_id `5f2708832375`, `scripts/release.sh:479-487`:

> **Masked exit status**
>
> `release_pr_apply` always returns the status of its last command, `rm -f "$bf"`, so failures in `git add`, `git commit`, `git push`, or `gh pr create` are invisible to `phase_run`. A transient `gh pr create` failure, or a commit rejected by a hook, leaves `phase_run` thinking release-pr succeeded: it neither consults the AI nor retries, and the later `find_pr` check either stops with a generic "no PR found" or accepts a PR that does not contain the release changes. This bypasses the R3/R4 consultation-and-retry contract from handoff-A.

## Verbatim excerpts of the defect code

`sed -n '479,487p' scripts/release.sh` (R4b — every failure is overwritten by the final `rm`):

```bash
release_pr_apply() {
  git add -A
  git diff --cached --quiet || git commit -q -m "Release $tag: version bump and changelog restructure"
  git push -q origin "release/$tag"
  local bf; bf="$(mktemp "${TMPDIR:-/tmp}/release-pr.XXXXXX")"
  printf 'Release %s: version bump and changelog restructure.\nManaged by scripts/release.sh (checklist issue #%s).\n' "$tag" "${ISSUE:-}" >"$bf"
  gh pr create --repo "$repo" --base master --head "release/$tag" --title "Release $tag" --body-file "$bf" >/dev/null
  rm -f "$bf"
}
```

`sed -n '212,219p' scripts/release.sh` (R4a — the committing handler) and `sed -n '254,259p'`
(its dispatch, blind to which side of the tag the phase is on):

```bash
handler_regen_bindings() { # <log> — make bindings + commit (#73)
  local log="$1"
  say "  .. known blocker: bindings drift — regenerating (make bindings) and committing (#73)"
  make bindings >>"$log" 2>&1 || return 1
  git add frontend/bindings >>"$log" 2>&1 || return 1
  git commit -q -m "Release $tag: regenerate frontend/bindings (#73)" >>"$log" 2>&1 || true
  return 0
}
```

```bash
    if grep -q 'frontend/bindings' "$log"; then
      handlers=$((handlers + 1))
      [[ $handlers -le 2 ]] || do_stop "$phase" "bindings drift persists after regeneration (#73)" "$log"
      handler_regen_bindings "$log" || do_stop "$phase" "make bindings / commit failed (#73)" "$log"
      continue
    fi
```

The tag-truth rule the handler violates (docs/releasing.md:40 and :276, verbatim): "**The git
tag `vX.Y.Z` is the source of truth.**" / "Anything **else** changed after the build means the
app was not built from the tag as committed."

## Case 1: `posttagdrift` (+ `premergedrift`, the safe side)

**Supersedes the round-1 `bindings` scenario.** The removed block
(`scripts/release.test.sh`, former lines 648-657) steered the exact defect shape —
`echo bindings >"$SD/local-fail"` firing at phase 10, one run, the tag pushed at phase 9 just
before — and pinned the condemned behavior as correct (`exit_is 0`, "the regenerated bindings
are committed", `local re-ran … 2`, draft reached). The finding (first_seen 2026-10-20T12:24,
after round 3) declares that same shape a defect, so the old contract is unsatisfiable
alongside the new one and is replaced, not kept. Every other existing assertion is untouched.

- **`posttagdrift`** (the RED): `mkscn posttagdrift; echo bindings >"$SD/local-fail";
  r_env=(FAKE_AI=retry); rrun v0.5.1`. The drift surfaces at phase 10 with `.phases.tag.done`
  true (asserted as the premise) and the tag already pushed. The pin: hard stop (exit
  non-zero); no regeneration attempted (`make.log` count 0 — a regen would dirty or mutate the
  tagged tree); nothing committed to `release/*` (no `regenerate frontend/bindings` argv in the
  fake git's log — it records all argv); zero consultations even with an AI standing by that
  would say `retry`; no blind retry (`local.log` 1); the stop names the tag as the truth and
  the bindings drift; the stop is visible on the checklist (`STOP at phase local`); local not
  marked done; nothing drafted past the stop (no `release create`).
- **`premergedrift`** (the safe side, passes today and must keep passing): drift steered
  pre-merge at phase 4 via a new `bump-drift` marker (the fake `release-bump.sh` prints the
  drift signature; the fake `make` clears it, like `local-fail`). Pin: the #73 handler still
  regenerates and commits to the release branch, never consults, the phase retries
  (`bump.count` 2), and the run proceeds to the draft with the eventual tag including the regen.
  Per the dispatch's "read the handler first and pin the CORRECT behavior": the current design
  does NOT stop for re-review here — R10p10's handler contract is regen+commit+re-run without
  the AI, and the pipeline's re-review is the phase-7 gate on the PR that carries the commit.
  No tag exists yet, so the tag-truth rule is not engaged. Pinning an explicit stop here would
  have contradicted the ruled handler contract, so the commit+retry+proceed shape is pinned
  instead — this also keeps F from over-correcting by deleting the handler outright.

## Case 2: `maskedexit` / `maskedexitcap` / `maskedexitpush`

Two new env-steered knobs in the fakes (no behavior change when unset — verified: the other 34
scenarios are untouched and pass):

- fake `gh` `pr create`: `FAKE_PRCREATE_FAILS=N` fails the first N invocations (counter knob,
  mirroring `FAKE_BUMP_FAILS`), logging each attempt to `prcreate.count`.
- fake `git`: `FAKE_PUSH_FAIL_AT=N` fails the Nth and every later `push … release/vX.Y.Z`. The
  steer is by occurrence because ph_branch (phase 3) and release_pr_apply (phase 6) issue the
  identical argv; ph_branch's push is #1, the apply's is #2+.

The three variants pin the unknown-failure contract (R3/R4) for both commands the finding names:

1. **`maskedexit`** (transient create failure, `FAKE_PRCREATE_FAILS=1 FAKE_AI=retry`): the
   failure must reach `phase_run` — exactly ONE consultation, recorded in state for phase
   `release-pr`, the apply re-run (create attempted twice), the run recovers to exit 0 with
   release-pr legitimately done and the draft reached. Today: the `rm` masks the failure, no
   consult, no retry, and the run dies later on the generic "no PR found" stop (exit 1).
2. **`maskedexitcap`** (persistent create failure, `FAKE_PRCREATE_FAILS=99`): three apply
   attempts through the consult loop, consultations capped at 2, and the stop is the
   consultation-cap stop ("still failing after 2 consultations"), not the generic no-PR stop.
3. **`maskedexitpush`** (failing push, `FAKE_PUSH_FAIL_AT=2`): same contract on the push path —
   the release-branch push retried through the loop (4 argv: 1 branch + 3 apply), 2
   consultations, the consultation-cap stop, release-pr NOT marked done, no draft created
   through a masked failure, the stop visible on the checklist. Today this is the finding's
   worst shape: full **silent success** (exit 0, PR accepted, release-pr done, draft created)
   despite the push failing.

## Finding → assertion map

| Finding clause | Assertions (label → pinned behavior) |
|---|---|
| R4a "commits regenerated frontend/bindings to the still-checked-out release branch" (post-tag) | `posttagdrift`: `nothing is committed to the tagged release branch` (no `regenerate frontend/bindings` argv); `no regeneration is attempted post-tag` (make.log 0) |
| R4a "a tree that differs from the already-pushed vX.Y.Z tag / the tag is the source of truth" | `post-tag bindings drift is a hard stop, not a regen+commit` (exit non-zero); `the stop names the tag as the truth (an owner decision)` (/truth/); `the stop names the bindings drift`; premise `the tag phase was already done when the drift fired` |
| R4a "the published artifact would not correspond to the tagged commit" | `local ran once, no blind retry`; `local is not marked done`; `nothing is drafted past a post-tag drift stop` (no `release create`); `the stop is visible on the checklist issue` |
| R4a safe side (pre-merge regen is the #73 rule, no tag exists yet) | `premergedrift` ×7: exit 0, make runs, commit to the release branch, no consult, bump re-ran (2), tag done, draft reached |
| R4b "failures in … `git push`, or `gh pr create` are invisible to `phase_run`" | `maskedexit`: `consulted the AI exactly once`, `create attempted twice`, `recorded for phase release-pr`, `names its phase`; `maskedexitpush`: `the release-branch push is retried through the loop (1 branch + 3 apply)`, `consults the AI (capped at 2)` |
| R4b "it neither consults the AI nor retries" / "bypasses the R3/R4 contract" | `maskedexitcap`: `three apply attempts through the consult loop`, `consultations capped at 2 per phase`, `the stop is the consultation-cap stop, not a generic no-PR stop`; both persistent variants' `still failing after 2 consultations` |
| R4b "leaves phase_run thinking release-pr succeeded" / silent success | `maskedexitpush`: `not masked into success` (exit non-zero — today exit 0), `release-pr is NOT marked done on a masked push failure` (today true), `no draft is created through a masked failure` (today created), `the stop is visible on the checklist issue` |
| R4b recovery is real, not just refusal | `maskedexit`: `not masked into phase success` (exit 0 via the retry), `release-pr then legitimately done`, `the recovered run still reaches the draft` |

## RED confirmation (verbatim)

Baseline before the edit (same tree, `4960559`): `bash scripts/release.test.sh` → exit 0,
**218 ok / 0 FAIL / PASS**. Sibling suite `bash scripts/release-ask-ai.test.sh` → PASS (and
still PASS after the edit; `bash -n scripts/release.test.sh` clean).

After the edit: `bash scripts/release.test.sh` → **exit 1, 223 ok / 25 assertion FAILs /
FAILED**. All 211 pre-existing assertions that remain (218 baseline − the 7 superseded
`bindings` lines) still pass; every FAIL is one of the new round-4 assertions. The 25, verbatim:

```text
FAIL PR100-R4a: post-tag bindings drift is a hard stop, not a regen+commit (exit 0, wanted non-zero)
FAIL PR100-R4a: no regeneration is attempted post-tag (make.log has 1 lines, wanted 0)
FAIL PR100-R4a: nothing is committed to the tagged release branch (did not want /regenerate frontend/bindings/ in git.log)
FAIL PR100-R4a: local ran once, no blind retry (local.log has 2 lines, wanted 1)
FAIL PR100-R4a: the stop names the tag as the truth (an owner decision) (wanted /truth/)
FAIL PR100-R4a: the stop is visible on the checklist issue (wanted /STOP at phase local/ in comments.log)
FAIL PR100-R4a: local is not marked done (jq '.phases.local.done' gave 'true', wanted 'false')
FAIL PR100-R4a: nothing is drafted past a post-tag drift stop (gh.log =~ /release create/ matched 1, wanted 0)
FAIL PR100-R4b: a failed gh pr create is not masked into phase success (exit 1, wanted 0)
FAIL PR100-R4b: the failure consulted the AI exactly once (askai.log has 0 lines, wanted 1)
FAIL PR100-R4b: the apply re-ran after the consult (create attempted twice) (prcreate.count is '1', wanted '2')
FAIL PR100-R4b: the consultation is recorded for phase release-pr (jq '[.consultations[]? | select(.phase == "release-pr")] | length' gave '0', wanted '1')
FAIL PR100-R4b: the consultation names its phase (wanted /release-pr/ in askai.log)
FAIL PR100-R4b: release-pr then legitimately done (jq '.phases["release-pr"].done' gave 'false', wanted 'true')
FAIL PR100-R4b: the recovered run still reaches the draft (missing maskedexit.sd/ghstate/release-v0.5.1.json)
FAIL PR100-R4b: three apply attempts through the consult loop (prcreate.count is '1', wanted '3')
FAIL PR100-R4b: consultations capped at 2 per phase (askai.log has 0 lines, wanted 2)
FAIL PR100-R4b: the stop is the consultation-cap stop, not a generic no-PR stop (wanted /still failing after 2 consultations/)
FAIL PR100-R4b: a failing git push in release_pr_apply is not masked into success (exit 0, wanted non-zero)
FAIL PR100-R4b: the release-branch push is retried through the loop (1 branch + 3 apply) (git.log =~ /^push( -q)? origin release/v0\.5\.1$/ matched 2, wanted 4)
FAIL PR100-R4b: the push failure consults the AI (capped at 2) (askai.log has 0 lines, wanted 2)
FAIL PR100-R4b: the failure surfaces as the consultation stop (wanted /still failing after 2 consultations/)
FAIL PR100-R4b: release-pr is NOT marked done on a masked push failure (jq '.phases["release-pr"].done' gave 'true', wanted 'false')
FAIL PR100-R4b: no draft is created through a masked failure (gh.log =~ /release create/ matched 1, wanted 0)
FAIL PR100-R4b: the stop is visible on the checklist issue (wanted /STOP at phase release-pr/ in comments.log)
```

Right-reason check: 12 of the 36 new assertions PASS today, and each is a premise or a
deliberate keep-green pin — `posttagdrift`'s premise (`tag done` when the drift fires), its
`askai.log 0` (today's handler also never consults) and the word `bindings` in today's output;
`premergedrift` ×7 (the safe side works today and must survive the fix); `maskedexitcap`'s
exit (today already non-zero via the wrong, generic stop). Every FAIL names the defect's own
observables: `exit 0` where a stop/success-with-recovery is required, the `regenerate
frontend/bindings` commit argv that must not exist, `make.log 1`/`local.log 2` where the
regen+retry must not run, `askai.log 0` where exactly 1 or 2 consultations are the contract,
`prcreate.count 1` where 2-3 attempts are, `release-pr done true` / `release create` matched
where the masked failure must stop first, and `wanted /truth/` where the stop must name the
tag-truth rule. Not one FAIL is a typo, missing import or harness artifact; the scenarios
reach the exact defect paths (phase 10 post-tag; phase 6 release-pr).

## Quality / proportionality

Five scenarios, 36 assertions (25 RED / 11 keep-green), all at the cheapest sufficient tier
(the existing shell harness — same tier as every other release.sh contract). No duplication:
the cap-2 machinery itself stays pinned once (`aicap`, bump); these cases pin only that
release-pr's apply failures REACH it, for both commands the finding names. The pre-merge
companion exists only to keep the #73 handler alive where it is correct (a ceiling on the fix,
not extra coverage). Harness steering added: two env knobs + one marker, inert when unset.

## Expected fix shape for F (contract, not implementation mandate)

- **R4a:** the bindings-drift dispatch (release.sh:254-259) or the handler learns which side
  of the tag it is on (e.g. `phase_done tag`, or the remote tag existing) and post-tag becomes
  a `do_stop` whose reason names the tag as the truth / an owner decision and the bindings
  drift; pre-merge behavior is unchanged (`premergedrift` pins it). Any conforming shape passes.
- **R4b:** every command in `release_pr_apply` (add/commit/push/create) must propagate its
  failure to `phase_run` (early `return 1` or an aggregate status — both satisfy the pins;
  nothing pins whether `gh pr create` runs after a failed push). The existing consultable-phase
  machinery then does the rest; no new failure class is introduced.

## Ready for F

RED is valid. F may implement both fixes against these tests.
