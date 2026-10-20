# Handoff-F-rework5: run 0078-issue-78, PR #100 rework pin round 5

**Date:** 2026-10-20
**Branch / worktree:** issue-0078-release-workflow (`.worktrees/0078-issue-78`)
**Contract:** handoff-T-rework5.md (the realistic harness: state-realistic `gh pr list`,
server-side `gh pr merge`, real-shape `gh pr view` with `mergeCommit`; 92 flipped + 19 new
RED assertions across 23 scenarios, 4 of them new: `mergedvisreview`, `mergedvismerge`,
`tagresume`, `mergefetch`).

## What I changed

One file: `scripts/release.sh`. Three surgical changes, one per contract item in
handoff-T-rework5 §"The contract F implements against". No test file was touched
(`git status`: ` M scripts/release.sh`, ` M scripts/release.test.sh` (T's RED round),
this handoff).

### 1. `find_pr` asks gh for every state (R5a) — scripts/release.sh:160-168

BEFORE (`sed -n '160,163p'` at the round-5 start):

```
find_pr() { # "<number> <state> <headSha>" for release/$tag, or nothing
  gh pr list --repo "$repo" --head "release/$tag" --json number,state,headRefOid 2>/dev/null \
    | jq -r -s 'flatten | .[0] | if . then "\(.number) \(.state) \(.headRefOid // "")" else empty end' 2>/dev/null || true
}
```

AFTER (`sed -n '160,168p' scripts/release.sh`):

```
find_pr() { # "<number> <state> <headSha>" for release/$tag, or nothing
  # --state all: gh pr list defaults to OPEN only, and every resume path here —
  # ph_branch's group-skip, ph_review's settled gate, ph_merge's adoption and
  # post-merge re-query, ph_tag's derivation — must also SEE an already-MERGED
  # release PR (PR #100 R5a).
  gh pr list --repo "$repo" --head "release/$tag" --state all --json number,state,headRefOid 2>/dev/null \
    | jq -r -s 'flatten | .[0] | if . then "\(.number) \(.state) \(.headRefOid // "")" else empty end' 2>/dev/null || true
}
```

Rationale: real `gh pr list --head X` with no `--state` lists OPEN PRs only, so the merged
release PR was invisible to every consumer. `--state all` (the option T's contract names)
makes the MERGED PR visible to ph_branch's group-skip (`mergedpr`, `leftover`, `nostate`,
`chgstop` resume), ph_review's settled gate (`mergedvisreview`), ph_merge's adoption
(`mergedvismerge`) and re-query (every fresh flow), and ph_tag's derivation. Output shape
unchanged — the same `"N STATE SHA"` triple.

### 2. ph_merge's post-merge re-query is guarded (the :563 `set -u` crash) — scripts/release.sh:564-570

BEFORE (`sed -n '561,563p'` at the round-5 start):

```
    info="$(find_pr)"
    set -- $info
    prst="$2"
```

AFTER (`sed -n '564,570p' scripts/release.sh`):

```
    info="$(find_pr)"
    # The re-query can come back empty (gh hiccup); `set -- $info` on nothing would
    # crash on the unbound $2 under set -u instead of stopping cleanly (PR #100 R5a).
    [[ -n "$info" ]] || do_stop merge "gh no longer reports a PR for release/$tag after merging PR #$prn"
    set -- $info
    prst="$2"
```

Rationale: with `set -uo pipefail`, an empty re-query made `prst="$2"` abort the whole
script (`release.sh: line 563: $2: unbound variable`) — a crash with no STOP banner, no
resume line. The guard converts that into the phase's normal clean stop. With fix 1 in
place the re-query is non-empty in the normal flow (the MERGED PR is now listed), so the
guard is the defensive twin, exactly as the contract demands ("any fix that leaves the
re-query empty still crashes").

### 3. ph_tag ensures the merge commit is LOCAL before annotating (R5b) — scripts/release.sh:594-606

BEFORE (`sed -n '586,587p'` at the round-5 start):

```
  [[ -n "$msha" ]] || do_stop tag "could not determine the merge commit to tag"
  # Peel the annotated tag to compare commits; a tag is never moved or deleted (R5).
```

AFTER (`sed -n '594,606p' scripts/release.sh`):

```
  [[ -n "$msha" ]] || do_stop tag "could not determine the merge commit to tag"
  # The merge commit gh reports is a REMOTE object (GitHub merges server-side); this
  # clone may not have it until a fetch installs it, and `git tag -a $tag $msha` on an
  # unfetched sha fails (PR #100 R5b). Ensure the object is locally present before any
  # tag operation: fetch when cat-file cannot find it — cat-file, NOT rev-parse
  # --verify, which happily resolves a well-formed full sha whose object is absent —
  # then stop if even the fetched clone lacks it. The tag lands on exactly the sha gh
  # reported; it is never moved to a locally-convenient stand-in.
  if ! git cat-file -e "$msha" 2>/dev/null; then
    git fetch -q origin || do_stop tag "git fetch failed (the merge commit ${msha:0:7} is not in this clone)"
    git cat-file -e "$msha" 2>/dev/null \
      || do_stop tag "the merge commit ${msha:0:7} is not in this clone even after fetch — fetch it by hand and resume"
  fi
```

Rationale: the merge commit `merge_sha_of` reports (pr view's `mergeCommit.oid`) is
created server-side and lands on origin only; the clone resolves it only after a fetch.
`git tag -a $tag $msha` on the unfetched sha dies (`fatal: bad object type.` /
`Failed to resolve ... as a valid ref`), which is what `tagresume` pinned. Presence is
checked with `git cat-file -e` per T's explicit warning — plain `rev-parse --verify`
"resolves" any well-formed full sha even when the object is absent. Tag-truth is kept:
the block sits BEFORE the existing remote/local tag comparisons, and every branch of it
either no-ops (object already local) or stops — the annotated tag still lands on exactly
the gh-reported sha (asserted by `peel_is` in `mergedvismerge`, `tagresume`, `mergefetch`),
and the existing wrong-commit hard stops are untouched.

### Sibling sweep (gh-reported SHAs / merged-PR facts consumed elsewhere)

- `scripts/release.sh:427-433` (ph_branch): the ls-remote sha `rb` is consumed by
  `git merge-base --is-ancestor` only AFTER `git fetch -q origin` (line 428) — already
  state-aware; unchanged.
- `scripts/release.sh:568-573` (ph_merge): `merge_sha_of`'s sha feeds `ci_green_on` (a
  gh-api, server-side check) and the state record; the only local-object consumer was
  ph_tag's `git tag -a`, now guarded by fix 3. `find_pr`'s head sha feeds server-side
  gates and state records; it is always a local object (pushed by this clone).
- `scripts/release-local.sh:89-94`: consumes the remote tag sha only as a string
  comparison, after `git fetch -q --tags origin` and resolving the LOCAL tag —
  state-aware; unchanged (and not in this round's scope).
- `scripts/changelog-check.sh:75` (`--state merged`) and
  `scripts/release-preflight.sh:237` (`--state open`): deliberate state choices for
  their own jobs (changelog coverage / open-PR authors); not the R5a defect; unchanged.

## Self-check

Tier-1 commands run in the worktree, in this order:

1. `bash -n scripts/release.sh` → clean.
2. `bash scripts/release.test.sh` → **PASS, exit 0: 271 ok / 0 FAIL** (baseline before
   the fix, same command: FAILED, exit 1 — 160 ok / 112 FAIL, matching T's RED state).
3. `bash scripts/release-ask-ai.test.sh` → **PASS, exit 0** (24 ok / 0 FAIL).
4. Full authoritative suite (`test.unit.command` from `.aftersight/pipeline.config.json`):
   `sh -c "(cd pkg/swiftbridge/scripts && bash ./build.sh) && go test -vet=off ./internal/... ./pkg/... -count=1 && pnpm --dir frontend test && bash scripts/release.test.sh && bash scripts/release-ask-ai.test.sh"`
   → **exit 0**, no FAIL line anywhere in the output.

All four new scenarios green (`mergedvisreview`, `mergedvismerge`, `tagresume`,
`mergefetch` — including their `peel_is`, `fetched_before_tag` and premise guards), and
every still-green scenario stayed green (the AC6/R5-5 negative sweep over all recorded
git invocations included: no tag deletion, no force push). Note: the authoritative GREEN
is the plugin's T-green run of the suite, not this self-check.

## Tests I think are wrong

None. Every flipped assertion read as realism, and the 19 new pins matched the contract
one-for-one; nothing needed weakening or re-reading.

## Ready for T(green)

Implemented against the RED contract: find_pr sees MERGED PRs, the re-query is guarded,
ph_tag fetches and verifies the merge commit locally before annotating. The plugin
should now run the suite: GREEN means the loop advances; RED means I rework.
