# Handoff-F: Phase 5 - Implement (rework round 6)

**Date:** 2026-10-20
**Branch / worktree:** issue-0078-issue-78 (`.worktrees/0078-issue-78`)
**Contract:** handoff-T-rework6.md (the `stateloss` pin + the two realism-flipped scenarios, AC2/R6 `nostate` and PR100-R1 `tagpush`)

## What I changed

Exactly one production file, one hunk: `scripts/release.sh`, `ph_changelog` — a
postcondition probe before the Known Issues gate. No other file was touched by me
(`git status` also shows T's `scripts/release.test.sh`, `decisions.md` and
`handoff-T-rework6.md`, which are T's round-6 writes, unmodified by F).

The probe derives done-ness from repo state exactly as the handoff's degree of freedom
blesses: it delegates to `release-check-version.sh "$ver" .`, which verifies the whole
changelog/restructure postcondition of THIS release — the CHANGELOG's `## [X.Y.Z]`
section (the thing this run's `restructure_apply` writes) plus the four bumped version
lines (every `build/config.yml` `version:` and both `build/darwin/Info.plist` CFBundle
keys). When it holds, `mark_done changelog` and return — the gate (both
`changelog-check.sh --strict` and the Known Issues curation) is skipped, so a
state-loss re-run after a completed release is a no-op continuation through phase 2,
not a spurious stop on the now-empty `[Unreleased]`. This is the same probe idiom
`ph_restructure` already uses (release.sh:484 pre-edit), so the two phases now re-derive
done-ness the same way.

Why the gate is not weakened: on a genuine pre-release tree (no `## [X.Y.Z]` section,
version files still at the old version) the probe FAILS and the gate runs exactly as
before — pinned green by `chgstop` (uncurated Known Issues stop), `chgstrict`
(uncovered PRs stop) and `PR100-R2` `stalemarker` (stale none-marker does not bypass),
all of which pass unchanged. `ph_restructure` needed no change: the sibling audit
verdict for it is "the probe is complete in production; the gap was harness realism,
fixed this round [by T]". The audit's two residues (Note A: state-blind `gh issue list`
fake; Note B: mid-release state loss on an unmerged branch) were explicitly listed as
not pinned this round; I did not act on them.

### Fix verbatim

Before (`sed -n '393,398p' scripts/release.sh` at head 15f1f2c):

```bash
ph_changelog() {
  printf '\n== phase 2: changelog (coverage + Known Issues)\n'
  if phase_done changelog; then say "  .. changelog already done"; return 0; fi
  local log="$statedir/logs/changelog.log"
  mkdir -p "$statedir/logs"
```

After (`sed -n '393,411p' scripts/release.sh` in this worktree):

```bash
ph_changelog() {
  printf '\n== phase 2: changelog (coverage + Known Issues)\n'
  if phase_done changelog; then say "  .. changelog already done"; return 0; fi
  # Postcondition probe (state-is-a-cache, R6; PR #100 R6): with state absent after a
  # completed release, restructure_apply has already moved [Unreleased]'s curated content
  # under `## [X.Y.Z] - date`, so the Known Issues gate below would see an EMPTY
  # [Unreleased] and spuriously stop. release-check-version.sh verifies this release's
  # whole changelog postcondition (the `## [X.Y.Z]` section plus the bumped version
  # files — the same probe ph_restructure delegates to); when it holds, the changelog
  # work for THIS version is done and the gate is skipped. A genuinely uncurated
  # [Unreleased] (no released section, unbumped files) still fails the probe and stops.
  if run_tool release-check-version.sh "$ver" . >/dev/null 2>&1; then
    say "  .. CHANGELOG already carries a [$ver] section and the version files say $ver (postcondition holds)"
    mark_done changelog
    return 0
  fi
  local log="$statedir/logs/changelog.log"
  mkdir -p "$statedir/logs"
```

Rationale, one line: the gate's job is to curate THIS release's `[Unreleased]`; once
that content lives under `## [X.Y.Z]` and the version files are bumped, the work is
done by definition, and re-running the gate against the emptied `[Unreleased]` is the
documented no-op case ("re-running a finished phase is a no-op", release.sh header).

## Self-check

Tier-1 commands run in the worktree (authoritative GREEN remains the plugin's T-green
run of `test.unit.command`):

- `bash -n scripts/release.sh` → clean.
- `bash scripts/release.test.sh` → **PASS**, 292 ok / 0 FAIL (round-6 RED baseline was
  278 ok / 14 FAIL). All 14 RED rows green: the 7 `PR100-R6` stateloss rows (no-op
  continuation exit 0, no spurious Known-Issues stop, no changelog-phase stop, gate not
  re-run at 1 invocation, publish exactly once, `changelog.done` re-derived true,
  `closeout.done` true) plus the 7 realism flips (AC2/R6 `nostate` x3, PR100-R1
  `tagpush` x4). The 9 stateloss premise rows and the 5 already-holding contract rows
  stay green. Gate-strength scenarios unchanged and green: `R10p2` chgstop/chgstrict,
  `PR100-R2` stalemarker (5 rows, including "changelog is NOT marked done by a stale
  marker").
- `bash scripts/release-ask-ai.test.sh` → **PASS**, 24 ok / 0 FAIL.
- Full authoritative suite (`.aftersight/pipeline.config.json` `test.unit.command`):
  `sh -c '(cd pkg/swiftbridge/scripts && bash ./build.sh) && go test -vet=off
  ./internal/... ./pkg/... -count=1 && pnpm --dir frontend test && bash
  scripts/release.test.sh && bash scripts/release-ask-ai.test.sh'` → **exit 0**, 336
  ok rows, 0 FAIL rows, both scripts end in PASS.

## Tests I think are wrong (if any)

None. Every RED row failed for the pinned single cause (the spurious changelog stop
after state loss) and the fix greens all of them without editing any test.

## Ready for T(green)

Implemented against the RED contract: `ph_changelog` now derives its done-state from
repo state (postcondition-first), the gate stands for the genuine pre-release case, and
the sibling audit required no other production change. The plugin should now run
`test.unit.command`: GREEN means the loop advances; RED means I rework.
