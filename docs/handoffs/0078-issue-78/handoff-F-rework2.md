# Handoff-F-rework2: PR #100 re-review fixes round 2 (R1 unpushed-tag resume, R2 stale marker)

**Date:** 2026-10-20
**Branch / worktree:** issue-0078-issue-78 (`.worktrees/0078-issue-78`, HEAD 7dd871a + this change)
**Contract:** handoff-T-rework2.md (tagpush + stalemarker cases in `scripts/release.test.sh`)

## What I changed

One file: `scripts/release.sh` (+15/−1). Two fixes, one per PR-Agent finding; no test file
touched (denied), nothing else in the tree moved.

### Fix 1 — R1: `ph_tag` resume on a local annotated tag the remote lacks (`scripts/release.sh:580-591`)

**Behavior:** when the remote lacks the tag but an earlier run left it LOCALLY at the merge
commit (its push after `git tag -a` was interrupted), the resume pushes the EXISTING local tag
and marks the phase done — it never attempts a second `git tag -a` (which real git refuses with
"tag already exists"). A local tag at the WRONG commit stays a hard stop, mirroring the
existing remote-tag rule (a tag is never moved; R5). Satisfies the 5 `PR100-R1` pins.

**Before** (`sed -n '578p'` of the pre-edit file — the whole local-tag gap was the absence of
any branch between the remote probe and the re-tag):

```
  git tag -a "$tag" -m "$tag" "$msha" || do_stop tag "git tag -a $tag failed"
```

**After** (`sed -n '580,592p' scripts/release.sh`):

```
  # The remote lacks the tag, but an earlier run may have left it LOCALLY at the merge
  # commit (its push after `git tag -a` was interrupted). Push that existing tag — never
  # re-create it (`git tag -a` fails "already exists") and never move one at a wrong
  # commit (R5; PR #100 R1).
  local ltag; ltag="$(git rev-parse -q --verify "$tag^{}" 2>/dev/null || true)"
  if [[ -n "$ltag" ]]; then
    [[ "$ltag" == "$msha" ]] || do_stop tag "hard stop: local $tag already exists at ${ltag:0:7}, not the merge commit ${msha:0:7} — a tag is never moved; resolve it by hand"
    git push -q origin "$tag" || do_stop tag "git push of $tag failed"
    say "  ok  $tag already created locally at ${msha:0:7}; pushed it"
    mark_done tag
    return 0
  fi
  git tag -a "$tag" -m "$tag" "$msha" || do_stop tag "git tag -a $tag failed"
```

Rationale: the probe peels the annotated tag to a commit (`"$tag^{}"`, the same peel the
remote probe at `:572-573` uses), so the equality check compares commit-to-commit; the branch
sits after the remote probe (remote-wrong-commit and remote-at-merge-commit keep their existing
outcomes) and before `git tag -a` (create only when the tag exists nowhere).

### Fix 2 — R2: `ph_changelog` none-marker check scoped to `[Unreleased]` (`scripts/release.sh:399-401`)

**Behavior:** the `<!-- known-issues: none … -->` check now greps the `[Unreleased]` slice the
phase already extracts at `:398` (same scoping idiom as `scripts/changelog-check.sh:61-67`),
not the whole file — a stale marker under a released version section no longer satisfies the
stop-for-owner gate. Satisfies the 4 `PR100-R2` pins and keeps the repaired in-`[Unreleased]`
positive case (`R10p2`) green.

**Before** (`sed -n '399p'` of the pre-edit file):

```
  if grep -q 'known-issues: none' "$root/CHANGELOG.md"; then none=1; else none=0; fi
```

**After** (`sed -n '399,401p' scripts/release.sh`):

```
  # The none-marker counts only inside [Unreleased] (PR #100 R2): a stale marker under a
  # released version section says nothing about THIS release and must not satisfy the gate.
  if printf '%s\n' "$unreleased" | grep -q 'known-issues: none'; then none=1; else none=0; fi
```

Rationale: `$unreleased` is the exact section the adjacent bullet check (`:398`) already
consumes, so marker and bullet now read the same slice — one-line change, no new extraction.

## Self-check

All run in the worktree against the current tree (my footprint: `git status` shows only
`scripts/release.sh` modified by me; `scripts/release.test.sh`, `decisions.md` and
`handoff-T-rework2.md` are T's, untouched).

RED re-verified first, then both fixes, then everything:

| Command (Tier-1) | Result |
|---|---|
| `bash scripts/release.test.sh` (before edits) | exit 1 — **190 ok / 9 FAIL**, exactly T's 9 `PR100-R1`/`R2` pins |
| `bash scripts/release.test.sh` (after) | exit 0 — **199 ok / 0 FAIL**, `PASS` |
| `bash scripts/release-ask-ai.test.sh` (after) | exit 0 — **24 ok / 0 FAIL**, `PASS` (24/24, T's baseline) |
| Full suite = `test.unit.command` from `.aftersight/pipeline.config.json`: `sh -c "(cd pkg/swiftbridge/scripts && bash ./build.sh) && go test -vet=off ./internal/... ./pkg/... -count=1 && pnpm --dir frontend test && bash scripts/release.test.sh && bash scripts/release-ask-ai.test.sh"` | **exit 0** — swiftbridge dylib/lib built; every Go package `ok` (`cnb.cool/dtapp/kai/...`, 0 FAIL); frontend `Test Files 79 passed (79)`; both release harnesses `PASS` |

All 11 `PR100-R` rows in the green run are `ok` (5 R1 pins, 4 R2 pins, plus the tagpush setup
row and the R2 keep-pin), including:

```
ok   PR100-R1: an unpushed local tag at the merge commit is pushed, not re-created
ok   PR100-R1: the EXISTING local tag is pushed exactly once
ok   PR100-R1: no second 'git tag -a' is attempted on resume
ok   PR100-R1: tag marked done by the push
ok   PR100-R1: the resumed run runs to completion
ok   PR100-R2: a stale none-marker under a RELEASED section does not bypass the gate
ok   PR100-R2: the stop still names the uncurated Known Issues
ok   PR100-R2: changelog is NOT marked done by a stale marker
ok   PR100-R2: nothing past the gate runs (no bump)
```

Note: the authoritative GREEN is the plugin's T-green run of the suite, not this self-check.

## Tests I think are wrong

None.

## Ready for T(green)

Implemented against the RED contract (`handoff-T-rework2.md`). The plugin should now run the
suite: GREEN means the loop advances; RED means I rework.
