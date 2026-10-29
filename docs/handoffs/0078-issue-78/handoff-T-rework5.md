# Handoff-T-rework: run 0078-issue-78, PR #100 rework pin round 5

**Date:** 2026-10-20
**Branch:** issue-0078-release-workflow (worktree `.worktrees/0078-issue-78`)
**Scope:** harness-only rework of `scripts/release.test.sh` (no production change) making the
fake gh realistic enough to turn PR-Agent's 2 ACTIVE defects RED.
**Files written:** `scripts/release.test.sh` (modified), this handoff. `git status` shows
exactly ` M scripts/release.test.sh`; `git diff scripts/release.sh` is empty (production
untouched).

## The two defects being pinned (verbatim, `scripts/release.sh`)

find_pr asks for no state — real `gh pr list` defaults to OPEN only
(`sed -n '160,163p' scripts/release.sh`):

```
find_pr() { # "<number> <state> <headSha>" for release/$tag, or nothing
  gh pr list --repo "$repo" --head "release/$tag" --json number,state,headRefOid 2>/dev/null \
    | jq -r -s 'flatten | .[0] | if . then "\(.number) \(.state) \(.headRefOid // "")" else empty end' 2>/dev/null || true
}
```

ph_merge re-queries after merging (`sed -n '554,566p'`) and ph_tag annotates whatever sha
`merge_sha_of` reported (`sed -n '608,609p'`) without ensuring the merge commit is local:

```
    info="$(find_pr)"
    set -- $info
    prst="$2"
    [[ "$prst" == "MERGED" ]] || do_stop merge "PR #$prn did not end up MERGED (state: $prst)"
  ...
  git tag -a "$tag" -m "$tag" "$msha" || do_stop tag "git tag -a $tag failed"
```

## Harness changes (all in `scripts/release.test.sh`)

1. **Fake `gh pr list` is state-realistic** (release.test.sh:367-396): defaults to OPEN,
   honors `--state all|merged|open|closed` (and `--state=x`) exactly like the CLI. Nothing
   else about the output shape changed.
2. **Fake `gh pr merge` merges server-side** (release.test.sh:397-421): after the state
   flip it creates a REAL merge commit with git plumbing in the ORIGIN bare repo
   (`commit-tree "$base^{tree}" -p base -p head` + `update-ref refs/heads/master`) and
   records its oid on pr-file line 4. The caller's clone does not get the object — only a
   `git fetch` installs it, which is precisely GitHub's shape.
3. **Fake `gh pr view` returns the requested fields with real shapes** (release.test.sh:422-447):
   a MERGED PR carries `"mergeCommit":{"oid":...}`, an unmerged one `"mergeCommit":null`
   (real gh never reports a mergeCommit for an open PR). The old fake returned only
   `{state,headRefOid}`, so production's `jq '.mergeCommit.oid // .headRefOid'` always fell
   back to the PR head — an sha the clone already had. That fallback was the permissiveness.
4. **`seed_merged_pr` / `seed_state` helpers** (release.test.sh:517-577) seed the same
   GitHub shape harness-side: MERGED PR, branch gone from origin, merge commit on
   origin/master only, plus a state.json as an interrupted run leaves it.
5. **Setup adjustments, exactly two** — `leftover` and `mergedpr` seeded 3-line MERGED PRs
   (no mergeCommit). Real GitHub ALWAYS reports a mergeCommit for a MERGED PR, so both
   setups now call `seed_merged_pr 101`. No assertion was touched, deleted or loosened in
   either case (or anywhere else in the file).
6. **New pin section** (release.test.sh:1156-1257) with four scenarios and four helpers
   (`peel_is`, `unresolved_is`, `fetched_before_tag`, plus a distinct-merge-commit premise).

**Deviation note (env-steering):** the task asked for fake-git env-steering so a configured
merge commit "becomes resolvable only after fetch records it". Implemented via real objects
instead: the merge commit is real, exists only in the real bare origin, so pre-fetch
resolution fails by real git itself (`git tag -a` → `fatal: bad object type.`, exit ≠ 0)
and post-fetch resolution succeeds — a synthetic rev-parse gate would either duplicate real
git or force the fake to forge objects after "fetch" (less real). The fake git is unchanged;
its existing argv log (`git.log`) is what `fetched_before_tag` consumes. A probe also
showed `git rev-parse --verify <full-sha>` "resolves" shas whose object is absent — so the
harness premise uses `cat-file -e`, and F is warned: a presence check via plain rev-parse
is wrong (it resolves absent full shas).

## BEFORE / AFTER

- BEFORE: `bash scripts/release.test.sh` → **PASS**, 248 ok / 0 FAIL (exit 0).
- AFTER: `bash scripts/release.test.sh` → **FAILED** (exit 1), the RED pin state:
  271 assertions = **156 existing still-PASS + 92 existing flipped to FAIL + 19 new-pin FAIL
  + 4 new ok** (2 no-re-merge guards, 2 harness premises). All counts machine-derived by
  sorting and comparing the two runs' ok/FAIL lines.
- `bash scripts/release-ask-ai.test.sh` (the other suite file touching release tooling)
  still PASSes; `bash -n scripts/release.test.sh` clean.

## Which existing cases flipped, and why each flip is realism (not weakening)

Every flipped assertion belongs to a scenario whose run now stops at a merged-visibility or
tag-boundary that real gh/git would actually enforce. Two manifestations, both R5a:

- **Fresh flow (the post-merge re-query):** after `gh pr merge`, the PR is MERGED, the
  state-blind `find_pr` returns nothing, and — because the script runs `set -u` —
  `prst="$2"` at release.sh:563 aborts the whole script with
  `release.sh: line 563: $2: unbound variable` (exit 1, no STOP banner, no resume line — a
  crash, not a clean stop; an uglier shape of the same defect, exposed not introduced).
- **Resume flow:** the first `find_pr` of the phase returns nothing → clean
  `do_stop ... "no release PR found for release/v0.5.1"`.

| Scenario (assertion group) | Where the run now ends | Realism argument |
|---|---|---|
| ac1life (AC1 + AC2) | crash :563 (run 1); `merge: no release PR found` (resume) | real `gh pr list` (default open) does not show the just-merged PR |
| default (AC4) | crash :563; resume stops at merge | same |
| smoke (AC4) | crash :563 | same |
| retry, aitwo (AC3) | crash :563 after the consult/retry recovery | same; the AI path itself is untouched (bump.count/askai counts still pass) |
| nostate (AC2/R6) | crash :563; the re-derivation re-creates branch+PR (spurious `pr create` ×2) then stops at merge | the merged PR is invisible, so re-derivation cannot adopt it |
| kai, kaiquit (AC3) | crash :563 before the local phase (`local.log` 0 lines) | same; `--quit-kai` in `$out` still passes via ph_needs' banner — a pre-existing weak match, unchanged |
| leftover (AC3/#74) | group-skip still happens (ls-remote path, bump.log 0 passes); stops at `review: no release PR found` | the MERGED PR is invisible to ph_review's find_pr |
| **mergedpr (PR100-F1)** | ph_branch no longer detects the MERGED PR → bump re-runs (wanted 0), spurious branch+PR, crash :563 | **the R5a ph_branch pin**: the old fake made the default query see MERGED PRs |
| posttagdrift, premergedrift (PR100-R4a) | crash :563 (posttagdrift's premise `tag.done` false — the tag phase is never reached) | same |
| chgstop resume (R10p2) | crash :563 | same (the changelog stop/refusal assertions before it still pass) |
| lockdead (R6) | crash :563 | same |
| hsdraft, hspub, loopexists | crash :563; the `exit_nonzero` assertions pass vacuously, the discriminating `already exists` assertions flip | the run no longer reaches phase 10 under real merged-visibility |
| verifyok, verifystop, verifyterm (PR100-F2) | setup crashes :563; downstream temp-hygiene assertions pass vacuously / `--dest` helper fails on absence | the setup never reaches verify |
| tagpush (PR100-R1) | setup crashes :563 (no tag ever created; the harness tag-delete then no-ops) | same |
| smokeresume (PR100-R3) | setup `--smoke` crashes :563; awaiting-owner is never recorded, so the R3 refusal assertions lose their premise | same |
| maskedexit (PR100-R4b-a) | crash :563 after the create-retry recovery (recovery counts still pass) | same |

**Unflipped (still fully green):** aicap, aibad, ainone, identity, chgstop 1st run,
chgstrict, stalemarker, locklive, corrupt, hsci, hsagent88, hsagent95o, nopush, loopidentity,
loopci, loopagent88, loopagent95o, maskedexitcap, maskedexitpush, AC7 docs, R5(5) negative
sweep — every scenario that stops (or asserts) before ph_merge. That is the expected
boundary: the realism change cannot affect a run that never asks about a merged PR.

No assertion was weakened, deleted or re-worded anywhere in the file; the only setup edits
(leftover, mergedpr) made the seeds MORE like real GitHub (a MERGED PR always has a
mergeCommit).

## New pins — all RED, verbatim

`mergedvisreview` (ph_review resume, seeded MERGED PR + branch deleted, state stops before
review; run ends `STOP: v0.5.1, phase "review": no release PR found for release/v0.5.1`):

```
FAIL PR100-R5a: ph_review resume settles on the MERGED PR (exit 1, wanted 0)
FAIL PR100-R5a: the gate is settled by the recorded merge (wanted /already merged/)
FAIL PR100-R5a: review marked done by the settle (jq '.phases.review.done' gave 'false', wanted 'true')
FAIL PR100-R5a: merge marked done from the merged PR (jq '.phases.merge.done' gave 'false', wanted 'true')
FAIL PR100-R5a: mergeSha is the merge commit, not the PR head (jq '.mergeSha' gave 'null', wanted '2536f25...')
ok   PR100-R5a: a merged PR is never merged again      (guard; holds pre- and post-fix)
```

`mergedvismerge` (ph_merge resume, state stops before merge; run ends
`STOP: v0.5.1, phase "merge": no release PR found for release/v0.5.1`):

```
FAIL PR100-R5a: ph_merge resume adopts the MERGED PR (exit 1, wanted 0)
FAIL PR100-R5a: merge marked done by adoption (jq '.phases.merge.done' gave 'false', wanted 'true')
FAIL PR100-R5a: mergeSha is the merge commit (jq '.mergeSha' gave 'null', wanted 'f691501...')
FAIL PR100-R5b: the tag lands on that merge commit (v0.5.1^{} is '', wanted 'f691501')
FAIL PR100-R5b: the merge commit is fetched before tagging (fetch/pull at line none, tag -a at line none of git.log)
ok   PR100-R5a: no second merge of an already-merged PR (guard)
```

`tagresume` (ph_merge done + mergeSha recorded, no tag anywhere; run ends
`fatal: bad object type.` / `STOP: v0.5.1, phase "tag": git tag -a v0.5.1 failed` — real
git refusing to annotate a commit the clone never fetched):

```
ok   PR100-R5b: premise: the merge commit is NOT local before the run
FAIL PR100-R5b: ph_tag fetches the merge commit and completes (exit 1, wanted 0)
FAIL PR100-R5b: the annotated tag is created at the merge commit (v0.5.1^{} is '', wanted '5e565a0')
FAIL PR100-R5b: tag marked done (jq '.phases.tag.done' gave 'false', wanted 'true')
FAIL PR100-R5b: the fetch precedes git tag -a in git.log (fetch/pull at line none, tag -a at line 5 of git.log)
```

`mergefetch` (live full flow: the workflow's own `gh pr merge` lands the merge commit in
origin only; run crashes at :563):

```
FAIL PR100-R5b: the workflow fetches its own merge commit and completes (exit 1, wanted 0)
ok   PR100-R5b: premise: the fake merge produced a distinct merge commit
FAIL PR100-R5b: the tag marks the MERGE commit the workflow created (v0.5.1^{} is '', wanted '7366b52')
FAIL PR100-R5b: state records that merge commit as mergeSha (jq '.mergeSha' gave 'null', wanted '7366b52...')
FAIL PR100-R5b: the fetch precedes the annotation in the live flow (fetch/pull at line none, ...)
FAIL PR100-R5b: the run still reaches the draft (missing mergefetch.sd/ghstate/release-v0.5.1.json)
```

The ph_branch half of (a) is the existing `mergedpr` scenario, now RED via the realistic
seed (see table). 19 new-pin assertions fail in total.

## The contract F implements against (behavior, not implementation)

1. `find_pr` must ask gh for the state it needs to see merged PRs (e.g. `--state all`) so
   ph_branch's group-skip, ph_review's settled gate, ph_merge's re-query/resume and ph_tag's
   derivation all find the MERGED PR. Note the re-query crash at release.sh:563 (`set -u` +
   `prst="$2"` on an empty re-query) — any fix that leaves the re-query empty still crashes.
2. ph_merge/ph_tag must ensure the merge commit gh reports is locally present (fetch, or
   verify presence with `git cat-file -e` — NOT plain `rev-parse --verify`, which resolves
   absent full shas — and fetch when absent) before `git tag -a`/`git push`. The tag must
   land on the merge commit, not the PR head (`peel_is` asserts the peel target).

## Counts

- **new-FAIL: 19** (all new pins RED for the named reasons)
- **existing-PASS: 156** (of 248 before; the other 92 flipped, each attributed above)
- flipped existing: 92 · new ok guards/premises: 4 · suite: 271 assertions, exit 1 (RED).

## Blocking issues

None for this round's deliverable (the RED pin state is the deliverable). For F: both
defects above, after which this suite must return to 271/271 green.
