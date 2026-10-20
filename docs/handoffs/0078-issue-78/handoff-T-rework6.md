# Handoff-T-rework: run 0078-issue-78, PR #100 rework pin round 6

**Date:** 2026-10-20
**Branch:** issue-0078-release-workflow (worktree `.worktrees/0078-issue-78`)
**Scope:** harness-only rework of `scripts/release.test.sh` (no production change): the fake
`release-check-version.sh` becomes realistic so the restructure postcondition path is
genuinely exercised, and a new `stateloss` case pins PR-Agent's 1 ACTIVE defect RED.
**Files written:** `scripts/release.test.sh` (modified), this handoff, `decisions.md` (appended).
`git status` shows exactly ` M scripts/release.test.sh`; `git diff scripts/release.sh
scripts/release-bump.sh scripts/release-check-version.sh` is empty (production untouched).

## The defect being pinned (verbatim)

PR-Agent on PR #100 (head 15f1f2c, score 70), finding `e989fe9ce3ec` ACTIVE,
`scripts/release.sh:411-418` — "Resume after state loss":

> With `state.json` absent after a completed release, `ph_changelog` is not phase_done, so it
> re-runs the changelog gate. After `restructure_apply` has restructured `CHANGELOG.md`, the
> `[Unreleased]` section is empty; the Known Issues gate (lines 411-418) sees no bullet and
> no `known-issues: none` marker inside `[Unreleased]` and calls `do_stop` (or
> `changelog-check.sh --strict` stops first). This contradicts the documented
> postcondition-first/state-is-a-cache resume promise in `docs/releasing.md` and the R6
> state-cache contract, and the harness misses it because the fake `release-check-version.sh`
> always exits 0, so `ph_restructure` never runs `restructure_apply` and the test
> `CHANGELOG.md` stays un-restructured.

Source, verbatim (`sed -n '411,418p' scripts/release.sh`) — the gate that fires on the
already-restructured changelog:

```
  unreleased="$(awk '/^## \[Unreleased\]/{u=1;next} u && /^## \[/{u=0} u{print}' "$root/CHANGELOG.md")"
  if printf '%s\n' "$unreleased" | awk '/^### Known Issues/{k=1;next} /^### /{k=0} k && /^[[:space:]]*- /{f=1} END{exit f?0:1}'; then bullet=1; else bullet=0; fi
  # The none-marker counts only inside [Unreleased] (PR #100 R2): a stale marker under a
  # released version section says nothing about THIS release and must not satisfy the gate.
  if printf '%s\n' "$unreleased" | grep -q 'known-issues: none'; then none=1; else none=0; fi
  if [[ $bullet -eq 0 && $none -eq 0 ]]; then
    do_stop changelog "the [Unreleased] section has no curated Known Issues: add a '### Known Issues' subsection with one line per real, still-open issue (candidates below), or, when there are none, add the marker: <!-- known-issues: none $(date +%Y-%m-%d) -->. Candidates: $cands Next release, also add: <!-- changelog-skip: release tooling, no user-facing change: #<this release PR> -->." "$log"
  fi
```

Live reproduction (scratch driver outside the repo, mirroring `mkscn` + fakebin; CHANGELOG
seeded post-restructure — `## [0.5.1] - 2026-10-20` present, `[Unreleased]` empty — version
files at 0.5.1, `state.json` absent; `release.sh v0.5.1 --publish`):

```
== phase 2: changelog (coverage + Known Issues)
ok: every merged PR in range is covered by [Unreleased] (or skipped)

STOP: v0.5.1, phase "changelog": the [Unreleased] section has no curated Known Issues: add a '### Known Issues' subsection with one line per real, still-open issue (candidates below), or, when there are none, add the marker: <!-- known-issues: none 2026-10-20 -->. Candidates: - #501 Crash when the log fills up (OPEN) [bug] ...
resume: scripts/release.sh v0.5.1
=== exit=1 ===
```

## Harness changes (all in `scripts/release.test.sh`)

1. **The fake `release-check-version.sh` is realistic** (new dedicated fake; removed from the
   generic always-0 loop at the old release.test.sh:224-232): it logs the invocation, then
   `exec`s the REAL `scripts/release-check-version.sh` against the fake tree. The real one
   verifies every `build/config.yml` `version:` line, both `build/darwin/Info.plist`
   CFBundle keys and the CHANGELOG's `## [X.Y.Z]` section, so `ph_restructure`'s postcondition
   probe (`sed -n '484,488p' scripts/release.sh`) now genuinely FAILS pre-restructure and
   genuinely HOLDS post-restructure: `restructure_apply` and its closing re-check run for
   real in every full-flow scenario. No env knob is needed — the fake tree's own state steers
   verify/fail exactly like the real one.
2. **The fake `release-bump.sh` edits both version surfaces** like the real one
   (`sed -n '26,31p' scripts/release-bump.sh`: every config.yml `version:` line + both plist
   CFBundle keys — the four lines): without this, the realistic check fake could never hold
   after a bump and `restructure_apply`'s re-check would fail forever. The old single-surface
   edit was the same class of unrealism as the always-0 check fake.
3. **New case `stateloss`** (release.test.sh, "PR #100 round 6" section): completes a real
   release first (`rrun v0.5.1`, default posture — with the realistic fakes this now genuinely
   restructures), ASSERTS every postcondition premise (CHANGELOG has exactly one
   `## [0.5.1] - date` section, `[Unreleased]` empty above it, config.yml + both plist keys
   at 0.5.1, tag on origin, release PR MERGED, changelog gate ran exactly once), then deletes
   `state.json` and re-runs `--publish`. The contract pinned: exit 0, no
   `phase "changelog"` stop, no "no curated Known Issues", the changelog gate NOT re-run
   (`changelog.log` stays at 1 invocation — postcondition recognized, per release.sh's own
   "re-running a finished phase is a no-op"), bump not re-run, restructure not re-applied
   (still exactly one released section), no second PR / tag / draft / publish, and the rebuilt
   state reaches `changelog.done` and `closeout.done`.

Nothing was weakened: no existing assertion was edited, deleted or loosened; the only edits to
existing scenarios are the two realism upgrades above, which apply uniformly.

## RED confirmation

`bash scripts/release.test.sh` → `FAILED` (exit 1). 278 ok rows, 14 FAIL rows:

- **`stateloss` (new, 7 rows RED — the defect):**

```
FAIL PR100-R6: a stateloss re-run is a no-op continuation, not a stop (exit 1, wanted 0)
FAIL PR100-R6: no spurious Known-Issues stop (did not want /no curated Known Issues/)
FAIL PR100-R6: no stop at the changelog phase at all (did not want /phase .changelog./)
FAIL PR100-R6: the changelog gate is NOT re-run (postcondition recognized) (changelog.log has 2 lines, wanted 1)
FAIL PR100-R6: no second publish (gh.log =~ /release edit/ matched 0, wanted 1)
FAIL PR100-R6: changelog re-derived done from repo state (jq '.phases.changelog.done' gave 'false', wanted 'true')
FAIL PR100-R6: the rebuilt state reaches closeout (jq '.phases.closeout.done' gave 'false', wanted 'true')
```

Right-reason check: all 9 premise rows PASS (the restructure postcondition path was genuinely
exercised in run 1 — only `restructure_apply` writes `## [0.5.1] - date`), the failure is the
re-run's spurious stop at phase changelog (the feature assertion), not an import/setup error,
and the gate-re-run row shows `changelog.log` at 2 invocations (the gate re-ran before the
stop). The 5 contract rows that hold even today (bump not re-run, one released section, no
second PR/tag/draft) also pass, so the scenario is not vacuously red.

- **Two existing scenarios flipped RED (7 rows) — same defect, unmasked by realism; expected
  to re-green with F's fix, reported not hidden:**

```
FAIL AC2/R6: absent state re-derives from postconditions (exit 1, wanted 0)
FAIL AC2/R6: re-derivation publishes once (gh.log =~ /release edit/ matched 0, wanted 1)
FAIL AC2/R6: the rebuilt state reaches closeout (jq '.phases.closeout.done' gave 'false', wanted 'true')
FAIL PR100-R1: an unpushed local tag at the merge commit is pushed, not re-created (exit 1, wanted 0)
FAIL PR100-R1: the EXISTING local tag is pushed exactly once (git.log =~ /^push( -q)? origin v0\.5\.1$/ matched 1, wanted 2)
FAIL PR100-R1: tag marked done by the push (jq '.phases.tag.done' gave 'false', wanted 'true')
FAIL PR100-R1: the resumed run runs to completion (jq '.phases.closeout.done' gave 'false', wanted 'true')
```

  Realism rationale: `nostate` (AC2/R6) and `tagpush` (PR100-R1) are both
  complete-a-release-then-delete-state resumes. Under the always-0 check fake their first
  runs never restructured the CHANGELOG, so `[Unreleased]` kept its Known Issues bullet and
  the re-run's changelog gate passed vacuously — the flips are precisely the finding's
  "the harness misses it" clause in reverse. No assertion in either scenario was touched.

- Everything else: 263 pre-existing rows PASS (baseline before this rework: 270/270 PASS,
  verified). `bash scripts/release-ask-ai.test.sh` still PASSes.

## Sibling postconditions audit (same shape: done-state derivable from repo state, but only checked via state.json)

Walked every phase on the absent-state path (`sed -n '335,767p' scripts/release.sh`):

| phase | derivable? | how it re-derives today | verdict |
|---|---|---|---|
| needs | yes (checklist issue) | `gh issue list --search` re-find/reuse | OK; see note A |
| preflight | n/a (pure check) | re-runs `release-preflight.sh` | OK (idempotent re-check) |
| **changelog** | **yes** (`## [X.Y.Z]` section = restructure postcondition) | **not checked — gate re-runs → spurious stop** | **GAP — pinned RED (`stateloss`)** |
| branch | yes (branch on origin / PR state) | leftover-branch + merged-PR group-skips | OK (pinned `leftover`/`mergedpr`); see note B |
| bump | yes (the 4 version lines) | group-skip when merged; else re-runs `release-bump.sh`, whose edit is a targeted same-value rewrite (idempotent file-wise; the real script edits exactly the 4 lines `release-check-version.sh` verifies) | OK on the completed-release path; see note B |
| restructure | yes (4 lines + CHANGELOG section) | probe at release.sh:484 delegates to `release-check-version.sh`, which includes the CHANGELOG `## [X.Y.Z]` section — the probe is complete in production; the gap was harness realism, fixed this round | OK — now genuinely exercised |
| release-pr | yes | `find_pr` "already exists" | OK |
| review | yes | MERGED settles the gate | OK (pinned `mergedvisreview`) |
| merge | yes | MERGED adoption + `merge_sha_of` | OK (pinned `mergedvismerge`) |
| tag | yes (remote tag at merge sha; local tag) | probe release.sh:607-626 | OK (pinned `tagpush`) |
| local | yes (release w/ 4 assets; dry-run outputs) | probe release.sh:640-654 | OK |
| smoke | **no** — the owner's `--smoke-done` assertion exists only in state.json | re-derives to "skipped" | by design (not derivable); benign after a completed release; listed, not pinned |
| publish | yes (`isDraft` false) | probe | OK |
| verify-published | no (not derivable) | re-runs download/verify | OK (idempotent check; no public mutation) |
| closeout | yes (issue closed) | re-search + re-close | OK-ish; see note A |

**Note A (harness realism residue, same class as the old state-blind pr-list fake):** the
fake `gh issue list --search` returns issues regardless of state; real `gh issue list`
defaults to OPEN. After `closeout` closes the checklist issue, a real stateloss re-run would
not find it and would open a DUPLICATE checklist issue (a cosmetic side effect, not a stop,
and it does not block closeout — the new issue gets closed instead). Not pinned this round:
making the fake state-realistic would couple this round's RED to an untasked behavior.
Flagged for a future round.

**Note B (mid-release state loss, same family, out of this round's scope):** with state.json
absent while the release branch is pushed but the PR is NOT merged, `ph_branch` (release.sh:426-437)
hard-stops "release/$tag exists on origin but is not merged into master — a release is in
progress" — the done-states (branch pushed, PR open) are derivable, but there is no
adopt-the-open-PR path, so the documented recovery for corrupt state ("delete it to rebuild
from postconditions", release.sh:106) dead-ends mid-release. It is a stop with a hand-resolution
(restore state.json or finish by hand), not a silent wrong action, and fixing it is a wider
change than this round's single ACTIVE defect. Listed for a future round/ADR, not pinned.

**No additional RED pin beyond `stateloss`**: the audit found the completed-release siblings
either probed, group-skipped, or idempotent on re-run; the two residues above are documented
with rationale instead (single-cause RED discipline for round 6).

## Ready for F

RED is valid: `stateloss` fails on the feature assertion (the spurious changelog stop after
state loss), every premise holds, and the two flipped scenarios fail for the identical reason.
F may implement against it. The fix's degree of freedom: `ph_changelog` must recognize the
release's changelog/restructure postcondition from repo state (e.g. the `## [X.Y.Z]` section
this run's `restructure_apply` would produce) BEFORE running the Known Issues gate, without
weakening the gate for a genuinely uncurated `[Unreleased]` (`chgstop`/`stalemarker` still
pin those stops). The pinned no-op contract: no gate re-run, no bump/restructure re-run, no
duplicate PR/tag/draft/publish, run completes to closeout.
