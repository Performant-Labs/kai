# Handoff-F-rework3: PR #100 round 3 — the smoke-gate refusal (case `smokeresume`)

**Date:** 2026-10-20
**Branch / worktree:** issue-0078-issue-78 (`.worktrees/0078-issue-78`)
**Contract:** handoff-T-rework3.md (finding `68ce73cf4590`, "Smoke gate bypass", ACTIVE; case `smokeresume` in `scripts/release.test.sh`)

## What I changed

One file, one hunk: **`scripts/release.sh`**, `ph_smoke` (+5 lines). Nothing else; no test
file touched (`git diff --stat` = `scripts/release.sh | 5 insertions(+)` + this handoff).

Before (verbatim, `sed -n '636,650p'` at the round-3 baseline — the defect is the
fall-through after the `--smoke` branch):

```bash
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
```

After (verbatim, the whole added guard is the 5 lines between the `--smoke` branch and the
fall-through):

```bash
    notify "smoke matrix awaiting the owner"
    exit 0
  fi
  # PR #100 R3: awaiting-owner is an owner gate — a resume without --smoke-done must refuse;
  # the record is never downgraded to skipped and the run never continues to publish.
  if [[ "$(jq -r '.smoke // ""' "$state" 2>/dev/null)" == "awaiting-owner" ]]; then
    do_stop smoke "the smoke matrix is still awaiting the owner (docs/release-checklist.md §Manual smoke matrix, on $statedir/Kai.app) — resume with --smoke-done to record it"
  fi
  st_update '.smoke = $v' --arg v "skipped"
```

Rationale: `do_stop` is the harness's existing stop-for-owner idiom — it prints the reason
(naming `--smoke-done`) to stderr, notifies, posts a `smoke`/`stop` checklist comment, writes
**no** state, consults **no** AI, and `exit 1`. Placed after the `--smoke` branch and before
the skipped fall-through it fires only when `smoke_done=0 && smoke=0` with `.smoke ==
"awaiting-owner"` recorded — exactly the forbidden transition — and since `ph_smoke` (line
755) runs before the posture gate and `ph_publish` (line 765), the refusal also stops
publish. The state-read idiom (`jq -r '.smoke // ""' "$state" 2>/dev/null`) is the one line
626 of the same function already uses.

Transition table after the fix (the three valid ones unchanged, per the untouched `default`
and `smoke` scenarios and this case's setup/recovery):

| recorded `.smoke` | flags this run | behavior |
|---|---|---|
| absent | none | `skipped`, phase done (unchanged) |
| absent | `--smoke` | `awaiting-owner`, exit 0, phase not done (unchanged) |
| `awaiting-owner` | `--smoke` (re-run) | re-records `awaiting-owner`, exit 0 (legal, per the contract's note) |
| `awaiting-owner` | none / `--publish` (no `--smoke-done`) | **refuse**: `do_stop` naming `--smoke-done`; no state write; publish never reached (the fix) |
| `awaiting-owner` | `--smoke-done` | `done (owner-asserted …)`, phase done, run continues (unchanged) |

## Self-check (green evidence)

Tier-1 commands run in the worktree, verbatim:

- `bash scripts/release.test.sh` → **exit 0 — 218 ok / 0 FAIL / PASS** (baseline 199; the
  round-3 case adds 19: 13 formerly-FAIL defect assertions + 6 setup/recovery already green
  in RED). All 13 round-3 FAILs from handoff-T-rework3 are now `ok`, including
  `a no-flag re-run is refused, not downgraded`, `the refusal tells the owner to pass
  --smoke-done`, `awaiting-owner is not downgraded to skipped`, `publish never runs without
  the owner's smoke record`, `no release edit …`, `the draft stays a draft`,
  `--smoke-done finally resumes and completes`.
- `bash scripts/release-ask-ai.test.sh` → **exit 0 — 24 ok / 0 FAIL / PASS**.
- `bash -n scripts/release.sh` → syntax OK.
- Full authoritative suite (`test.unit.command` in `.aftersight/pipeline.config.json`):
  `sh -c "(cd pkg/swiftbridge/scripts && bash ./build.sh) && go test -vet=off ./internal/... ./pkg/... -count=1 && pnpm --dir frontend test && bash scripts/release.test.sh && bash scripts/release-ask-ai.test.sh"`
  → **SUITE EXIT=0**: swift bridge build ok, go tests ok, frontend **79 files / 870 tests
  passed**, both release suites PASS.

Note: the authoritative RED→GREEN verdict is the plugin's T-green run of the same command,
not this self-check.

## Tests I think are wrong

None. Every assertion in `smokeresume` reads as the finding's own observables, and the
valid-transition cases were already pinned elsewhere and stay green.

## Ready for T(green)

Implemented against the RED contract: the awaiting-owner record is now an owner gate that a
flagless (or `--publish`) resume cannot silently satisfy. The plugin should run the suite:
GREEN means the loop advances; RED means I rework.
