# Handoff-T-rework: PR #100 post-gate pin (F1 resume-state gap, F2 temp-dir leak)

**Date:** 2026-10-20
**Branch:** issue-0078-issue-78 (run worktree `.worktrees/0078-issue-78`)
**Findings source:** PR-Agent review of PR #100 (score 87, 2 ACTIVE), journaled in
`decisions.md` §"PR #100 review loop (2026-10-20)"; defects at `scripts/release.sh:421-424`
(ph_branch MERGED path) and `scripts/release.sh:657,680,683` (ph_verify).

## What this is

A post-gate rework RED: two defects no harness pins. T extended the EXISTING harnesses (no
restructure) so each defect is pinned by failing cases F implements against. Production code
untouched (`git status`: only `scripts/release.test.sh` modified).

## Tests authored (all in `scripts/release.test.sh`, same tier/conventions: fake gh/git,
## env steering, per-case ok/FAIL, throwaway repos with local bare origins)

### Scenario `mergedpr` — pins finding **F1** (resume-state gap), 7 assertions

Setup mirrors the real corner: `state.json` absent (explicit `rm -f`), release PR #101 in
MERGED state (fake ghstate `pr-101`), its remote branch deleted (never pushed — unlike the
`leftover` case, which keeps the merged tip on origin), local `release/v0.5.1` checked out.

| Assertion | Pins | RED today |
|---|---|---|
| `exit_is … completes 0` | the corner is a group-skip, not a stop | ok (keep-pin) |
| `count_is bump.log 0` | **must NOT re-run release-bump.sh** (fake records invocations) | **FAIL** |
| `jq_is .phases.bump.done true` | group-skip marks bump (mirrors `leftover`'s `mark_done branch bump restructure release-pr`) | ok (keep-pin; today reached the long way) |
| `jq_is .phases.restructure.done true` | same, restructure | ok (keep-pin) |
| `jq_is .phases["release-pr"].done true` | same, release-pr | ok (keep-pin) |
| `git status --porcelain` empty | **tree stays clean — no uncommitted version edits** | **FAIL** |
| `exists release-v0.5.1.json` | still reaches the draft | ok (keep-pin) |

The existing `leftover` (merged-tip-on-remote) case is byte-unchanged and still green — the new
block sits directly after it as "(2b)".

### Scenarios `verifyok` / `verifystop` / `verifyterm` — pin finding **F2** (temp-dir leak), 15 assertions

Belongs in `release.test.sh` (T's call): both leaked dirs are `release.sh`'s own `mktemp`s;
`release-ask-ai.test.sh` tests only the R9 client. TMPDIR is steered per scenario
(`r_env=(TMPDIR="$SD/tmp")`) — release.sh expands it into the `release-verify.XXXXXX` template
itself. The inline `--dest "$(mktemp -d)"` **cannot** be steered (macOS mktemp's no-template
form ignores `$TMPDIR` — verified empirically), so its dir is asserted by the absolute path the
fake `install-release.sh` records in its argv log.

| Scenario | Exit class | Assertions | RED today |
|---|---|---|---|
| `verifyok` | success | exit 0; no `release-verify.*` workdir; `--dest` dir gone; nothing under steered TMPDIR | **FAIL ×1** — `--dest` leaks unconditionally (release.sh:680); workdir is rm'd on success today (keep-pin) |
| `verifystop` | hard stop (`install-fail` marker → fake install-release.sh exits 1 → `do_stop` between the two mktemps) | exit non-zero; stop names `verify-published`; no workdir; `--dest` gone; nothing left | **FAIL ×3** — `rm -rf "$vdir"` only runs on the success line (:683) |
| `verifyterm` | forced mid-verify exit (`verify-term` marker → fake release-verify.sh `kill -TERM $PPID` while the workdir is live) | exit non-zero; workdir cleaned anyway (trap-based); nothing left | **FAIL ×2** — no trap ⇒ SIGTERM death leaks the workdir; an end-of-function rm cannot clean after a kill |

The three exits cover the task's "ANY exit (success, hard stop, unknown-failure)": success,
`do_stop` (the only failure exit ph_verify has — it is not in `CONSULTABLE`), and a forced
signal exit. Fake extensions follow the existing marker conventions (`local-fail`,
`codesign-fail`, …): `install-fail` on the install fake, `verify-term` on a release-verify.sh
override (identical behavior minus the marker); both no-ops in every pre-existing scenario.

## RED confirmation

`bash scripts/release.test.sh` → **exit 1**; **180 ok / 8 FAIL** — baseline before the edit was
**166 ok / 0 FAIL** (re-verified green this session), so all 14 new keep-pins pass and every
FAIL is a new defect pin; zero pre-existing cases changed verdict. `bash
scripts/release-ask-ai.test.sh` untouched (hash identical) and still **24 ok / 0 FAIL, PASS**.

Verbatim failing output (the 8, from the run):

```
FAIL PR100-F1: bump is NOT re-run (group-skip mirrors #74) (bump.log has 1 lines, wanted 0)
FAIL PR100-F1: uncommitted version edits:  M build/config.yml
FAIL PR100-F2: the install-release --dest dir does not survive success (--dest /var/folders/x7/67v942_11v5fkvlldvd0_gq00000gn/T/tmp.jDkXYUEfRF still exists)
FAIL PR100-F2: no release-verify.* workdir survives a hard stop (/var/folders/x7/67v942_11v5fkvlldvd0_gq00000gn/T/tmp.6dH3vMOCN4/verifystop.sd/tmp/release-verify.aJNX4A )
FAIL PR100-F2: the install-release --dest dir does not survive a hard stop (--dest /var/folders/x7/67v942_11v5fkvlldvd0_gq00000gn/T/tmp.xQgIQ1i5zz still exists)
FAIL PR100-F2: nothing at all survives a hard stop (leftovers: release-verify.aJNX4A release-verify.aJNX4A/Kai-0.5.1-darwin-arm64.zip … )
FAIL PR100-F2: trap-based cleanup still runs on a forced mid-verify exit (/var/folders/x7/67v942_11v5fkvlldvd0_gq00000gn/T/tmp.6dH3vMOCN4/verifyterm.sd/tmp/release-verify.G63MCU )
FAIL PR100-F2: nothing at all survives a forced mid-verify exit (leftovers: release-verify.G63MCU release-verify.G63MCU/Kai-0.5.1-darwin-arm64.zip … )
```

Every FAIL names an observable artifact of its defect (a recorded invocation, a dirty
`git status`, a surviving directory) — none is an import/setup error.

## Contract for F (what GREEN means)

1. **F1:** in `ph_branch`'s MERGED-PR path, mark `branch bump restructure release-pr` done
   (the `leftover` group-skip), so bump never re-runs and no version edit lands uncommitted.
2. **F2:** every `mktemp` dir `ph_verify` creates (the `release-verify.*` workdir and the
   install-release `--dest`) is removed on ALL exits, via a trap (or equivalent) that also
   survives a forced mid-verify kill — the `verifyterm` case fails any rm-only-on-exit-paths
   fix.

## File hashes (sha256)

| File | Before | After |
|---|---|---|
| `scripts/release.test.sh` | `63a203524a331570d63a9c0f6f791a39a5f2e126d712fb6d5439e443e4b7c7d1` | `ee2f443383ae05369d81e5c3632acf0f74fecee80c7c476b9a0c755fd17db5e5` |
| `scripts/release-ask-ai.test.sh` | `8c689b9cf54cb88353158f87d415539ed9d4efb49a83ab7e31191578ff7c1a8b` | unchanged |

## Ready for F

RED is valid: each defect has failing cases that fail for the right reason, all pre-existing
cases still pass, production code untouched. F may implement.
