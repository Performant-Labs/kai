# Handoff-T-rework2: PR #100 re-review pin round 2 (R1 unpushed-tag resume, R2 stale changelog marker)

**Date:** 2026-10-20
**Branch:** issue-78-… (worktree `.worktrees/0078-issue-78`, HEAD 7dd871a)
**Findings source:** PR-Agent **re-review** of PR #100 — 2 new ACTIVE defects (delivered with
this dispatch; not yet journaled as their own decisions.md entry at T-start — this handoff and
the T entry below record them from the dispatch text):
- **R1** `scripts/release.sh:570-580` (`ph_tag`) — resume gap on an unpushed tag: the phase
  probes only the REMOTE tag; with the annotated tag already existing LOCALLY at the merge
  commit and absent from the remote, re-running attempts a second `git tag -a`, which fails
  ("tag already exists"), and the run stops instead of pushing the existing local tag.
- **R2** `scripts/release.sh:396-402` (`ph_changelog`) — gate bypass via a stale marker: the
  none-marker check greps the WHOLE file (`:399`), so an old `<!-- known-issues: none … -->`
  under a RELEASED version section satisfies the gate while `[Unreleased]` has no Known Issues
  bullets and no fresh marker.

**Files touched:** `scripts/release.test.sh` only (+62/−2; `git status` confirms nothing else,
production code untouched, `release-ask-ai.test.sh` hash-identical).

## Tests authored

Both cases extend the EXISTING harness (`scripts/release.test.sh`: throwaway repo + local bare
origin + fake-bin PATH steering, fake `git` wrapping real git) — integration tier, cheapest
sufficient: both defects are resume/gate behavior of the operator script itself, observable
only by driving `release.sh` end-to-end against fakes. No duplication: `chgstop` pins the
no-marker stop; `chgstrict` pins the coverage stop; nothing pinned the unpushed-local-tag
resume or the stale-marker bypass.

| Case | Finding | Pins (behavior, not implementation) |
|---|---|---|
| `tagpush` | R1 | Setup: default posture completes once (tag created + pushed, draft exists). Harness then deletes ONLY the remote tag (a plain `$RG push --delete`, outside the fake's log) and removes `state.json` (R6 re-derivation). Re-run with `--publish` must: exit 0; push the EXISTING local tag exactly once (`^push( -q)? origin v0\.5\.1$` total 2 — setup 1 + resume 1); attempt `git tag -a` ZERO further times (total stays 1); mark `.phases.tag.done` true; reach `.phases.closeout.done` true. |
| `stalemarker` | R2 | CHANGELOG whose `[Unreleased]` has the skip line + a coverage entry but NO `### Known Issues` bullets and NO marker, while `<!-- known-issues: none 2026-09-01 -->` sits under the released `## [0.4.0]` section. `rrun v0.5.1` must take the stop-for-owner path: exit non-zero; the stop message names the gap (`no curated Known Issues`); never consults the AI (keep-pin); `.phases.changelog.done` stays false; nothing past the gate runs (`bump.log` 0 lines). |

**One existing case repaired (test-owned, disclosed):** `chgstop`'s positive resume appended its
`<!-- known-issues: none … -->` at EOF — i.e. UNDER the released `[0.4.0]` section, not inside
`[Unreleased]` — and only unblocked the phase because of the whole-file grep R2 removes. The
resume now rewrites the CHANGELOG with the fresh marker (dated `$(date +%Y-%m-%d)`, so it can
never rot) INSIDE `[Unreleased]`, and the verdict is renamed to "a none-marker inside
[Unreleased] unblocks the phase". Green before the fix (whole-file grep still finds it) and
green after (scoped check finds it) — RED-neutral for the existing suite, contract-correct
afterwards. This is the task's "keep/verify the existing positive case" half.

## RED confirmation

Run: `bash scripts/release.test.sh` → **exit 1, 190 ok / 9 FAIL** (baseline before edits
re-verified first: 188 ok / 0 FAIL, and `bash scripts/release-ask-ai.test.sh` → PASS 24/24,
re-verified after edits too). The 9 FAILs are exactly the new behavior pins; zero pre-existing
verdicts changed:

```
FAIL PR100-R1: an unpushed local tag at the merge commit is pushed, not re-created (exit 1, wanted 0)
FAIL PR100-R1: the EXISTING local tag is pushed exactly once (git.log =~ /^push( -q)? origin v0\.5\.1$/ matched 1, wanted 2)
FAIL PR100-R1: no second 'git tag -a' is attempted on resume (git.log =~ /^tag -a v0\.5\.1/ matched 2, wanted 1)
FAIL PR100-R1: tag marked done by the push (jq '.phases.tag.done' gave 'false', wanted 'true')
FAIL PR100-R1: the resumed run runs to completion (jq '.phases.closeout.done' gave 'false', wanted 'true')
FAIL PR100-R2: a stale none-marker under a RELEASED section does not bypass the gate (exit 0, wanted non-zero)
FAIL PR100-R2: the stop still names the uncurated Known Issues (wanted /no curated Known Issues/)
FAIL PR100-R2: changelog is NOT marked done by a stale marker (jq '.phases.changelog.done' gave 'true', wanted 'false')
FAIL PR100-R2: nothing past the gate runs (no bump) (bump.log has 1 lines, wanted 0)
```

New-ok rows in the same run: `PR100-R1: tagpush setup: default posture completes first` and
`PR100-R2: the stop-for-owner never consults the AI` (keep-pin, passes both sides), plus the
repaired `ok R10p2: a none-marker inside [Unreleased] unblocks the phase`.

**Right-reason proof (R1):** the harness `git` fake execs the real git, so the failed re-tag is
real git's own refusal, not a fake artifact — verified directly:

```
$ git tag -a v0.5.1 -m v0.5.1   # second attempt, tag exists
fatal: tag 'v0.5.1' already exists
exit=128
```

routed by the code under test (`sed -n '578p' scripts/release.sh`):

```
  git tag -a "$tag" -m "$tag" "$msha" || do_stop tag "git tag -a $tag failed"
```

i.e. resume → `tag -a` attempted (matched 2) → real git refuses → `do_stop` → exit 1,
`tag.done`/`closeout.done` false, no push (matched 1). Not an import/setup error: the setup
verdict one row above is ok.

**Right-reason proof (R2):** exit 0 + `.phases.changelog.done == true` + `bump.log == 1` means
the stale marker under `[0.4.0]` satisfied `:399`'s whole-file grep and the run sailed through
the gate to the draft — precisely the bypass, not a fixture error (the no-marker twin `chgstop`
still stops, and the repaired in-`[Unreleased]` marker still proceeds, in the same run).

## Case → finding map

| Harness case | PR-Agent finding | Defect site |
|---|---|---|
| `tagpush` (5 FAIL) | R1 resume gap on unpushed tag | `scripts/release.sh:570-580` (`ph_tag`) |
| `stalemarker` (4 FAIL) | R2 changelog gate bypass | `scripts/release.sh:396-402` (`ph_changelog`, grep at `:399`) |
| `chgstop` repair (still ok) | R2 positive half | same — marker must count only inside `[Unreleased]` |

## Ready for F

RED is valid: 9 new-finding pins fail for the right reasons, all 188 pre-existing verdicts
pass, and production code is untouched. F implements against these pins (expected fix shapes:
`ph_tag` checks for a LOCAL tag at `$msha` before `git tag -a` and pushes it when found;
`ph_changelog` scopes the `known-issues: none` grep to the `[Unreleased]` slice it already
extracts at `:397`).
