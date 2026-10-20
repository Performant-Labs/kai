# Handoff-T-rework: run 0078-issue-78, PR #100 rework pin round 7 (the PERFECT round)

**Date:** 2026-10-20
**Branch:** issue-0078-release-workflow (worktree `.worktrees/0078-issue-78`)
**Scope:** harness rework of BOTH test files (`scripts/release.test.sh`,
`scripts/release-ask-ai.test.sh`): new production RED where production must change (A,
B1, B2), hygiene pins that are GREEN today because production already holds or no real
binary is reached (B3, C, D-guards, E), and the fix for the real-opencode escape (C3,
in the harness itself). No production file was touched by T: `git diff --name-only`
shows exactly `scripts/release.test.sh`, `scripts/release-ask-ai.test.sh`,
`docs/handoffs/0078-issue-78/decisions.md` (this handoff and the journal entry added on
top).

## Suite state (the RED contract for F)

| suite | ok | FAIL | FAIL rows are |
|---|---|---|---|
| `bash scripts/release.test.sh` | 377 | **11** | drystop (4) + drystopdone (3) + handlercounts (4) — all production defects |
| `bash scripts/release-ask-ai.test.sh` | 33 | **3** | escapere (3) — the production defect |

Same results under `/bin/bash` (3.2.57): 377/11 and 33/3 (see E). Baseline before this
round was 292/0 and 24/0; row arithmetic proves **zero existing-row flips**: release.test.sh
added 96 rows (292+96 = 388 = 377 ok + 11 FAIL), ask-ai added 12 (24+12 = 36 = 33+3).
Nothing was weakened: no pre-existing assertion was edited, deleted or loosened; the only
edits inside existing fakes are ADDITIVE steering (markers that no old scenario sets).

## A. "drystop" — the --dry-run + --smoke posture contradiction must be refused up front (RED)

Smoke is an owner gate at phase 11; the dry-run posture exits at the boundary after
phase 10, so `--dry-run --smoke` and `--dry-run --smoke-done` are contradictions that
production today **silently ignores** — the run completes the dry run and exits 0.
Verbatim current behavior (`rrun v0.5.1 --dry-run --smoke`, instrumented scratch copy of
the harness, since deleted):

```
== phase 10: local build, verify, package (--dry-run, no draft)
Dry-run posture complete for v0.5.1: every phase through the local build ran; NO draft release was created.
  2. smoke:   run the manual smoke matrix (docs/release-checklist.md) on .../bin/release/v0.5.1/Kai.app; record with --smoke/--smoke-done
```

…exit 0, `.smoke` never leaves `"pending"`, `--smoke` honored nowhere. The pinned
contract (scenario `drystop`): refuse up front — non-zero exit, message naming both
flags and why (the smoke matrix needs the default posture; smoke recording happens after
the draft), BEFORE any phase runs (`gh.log` empty, `preflight.log` empty, no VCS op in
`git.log`). RED rows (11 total across the two scenarios; the message-shape rows
`names --dry-run/--smoke/says why` pass today only because the completion banner
mentions the words — the load-bearing RED is `exit_nonzero` + the three no-phase rows):

```
FAIL R7/drystop: --dry-run --smoke is refused up front (exit 0, wanted non-zero)
FAIL R7/drystop: no gh call happens (refusal precedes phases) (gh.log has 19 lines, wanted 0)
FAIL R7/drystop: no wrapped tool runs (preflight) (preflight.log has 1 lines, wanted 0)
FAIL R7/drystop: no VCS op happens (no checkout/push/commit) (did not want /checkout|^push|^commit|^tag / in git.log)
FAIL R7/drystop: --dry-run --smoke-done is refused up front (exit 0, wanted non-zero)
FAIL R7/drystop: no gh call happens (gh.log has 19 lines, wanted 0)
FAIL R7/drystop: no wrapped tool runs (preflight) (preflight.log has 1 lines, wanted 0)
```

Companions (green, guarding against over-refusal — the not-over-reach half of the pin):
`--dry-run` ALONE still works (pinned by `ac1life`, rows 609-633, untouched) and
`--smoke` ALONE still enters awaiting-owner (pinned by `smoke`, rows 676-683, untouched)
— cited, not duplicated. New green guards: `drystopub` pins the existing
`--dry-run --publish` refusal (release.sh:46) so the new gate matches that shape, and
`dryquit` proves `--quit-kai` still ACTS under `--dry-run` (Kai running at phase 0 is
quit, the dry run completes) — a capability flag is posture-independent, never ignored.

**F's degree of freedom:** extend the existing combination-validation block at
release.sh:45-46 (before `root`/lock/state) with the two refusals; exit 2 fits the
block's precedent; the message must name both flags and the rationale.

## B1. "handlercounts" — per-handler blocker retry counters (RED)

`phase_run` keeps ONE shared `handlers` counter for both static handler tables
(release.sh:243-273): quit-kai twice consumes the budget, so the FIRST bindings event
then trips `handlers=3 > 2` and stops with "bindings drift persists after regeneration"
though the regen handler has never run. Verbatim current behavior (scenario
`handlercounts`, seq `kai kai bindings bindings` steered at phase 4 via the new
`bump.seq` per-invocation steering — phase_run's dispatch is generic across phases, the
`premergedrift` precedent):

```
== phase 4: version bump
  .. known blocker: Kai is running — quitting it (--quit-kai)
  .. known blocker: Kai is running — quitting it (--quit-kai)
FAIL: the tree is not the tag: frontend/bindings differs (run make bindings and commit the result, #73)
STOP: v0.5.1, phase "bump": bindings drift persists after regeneration (#73)
```

The pinned contract: the cap is PER HANDLER — quit-kai x2 + regen-bindings x2 all fit
in one phase run (exit 0, two quits, two regens, bump runs 5x = 4 blocked + 1 clean,
draft reached). RED rows:

```
FAIL R7/handlercounts: quit-kai x2 + regen-bindings x2 all fit one phase run (exit 1, wanted 0)
FAIL R7/handlercounts: the regen handler dispatched exactly twice (make.log =~ /^bindings$/ matched 0, wanted 2)
FAIL R7/handlercounts: bump ran 5 times (4 blocked + 1 clean) (bump.count is '3', wanted '5')
FAIL R7/handlercounts: the run still reaches the draft (missing handlercounts.sd/ghstate/release-v0.5.1.json)
```

Green guards for the refactor (caps stand PER handler, not in total):
`handlercapkai` (a THIRD quit stops with "still running after two quit attempts",
exactly two quits dispatched) and `handlercapregen` (persistent drift allows exactly two
regens then stops with "persists after regeneration", `make bindings` count 2). The
AI-consultation cap is unchanged and stays pinned by the untouched `aicap` scenario
(2 consultations per phase, third failure stops) — F must not touch `consult_count`
semantics while splitting the handler counters.

## B2. "escapere" — the ask-ai checklist grep must treat the command description as a FIXED string (RED)

`release-ask-ai.sh:47-49` interpolates the command description's first word into an
ERE: `grep -iE "$phase|$first_word"`. Both defect modes reproduced live before writing
the pin: `grep -iE 'bump|a(b'` exits 2 ("parentheses not balanced") and the `|| true`
swallows it → EMPTY context; `grep -iE 'bump|[xyz]'` matches `plain exhibits` via the
character-class reading. The seam: `$here` derives from `$0`, so a SYMLINKED copy of
the real script (the code under test is the real, unmodified file) reads a fixture
checklist. Cases `R7/escapere` (in `release-ask-ai.test.sh`):

```
FAIL R7/escapere: a line matching only via the character class is OUT (did not want /merely exhibits/ in oc.args)
FAIL R7/escapere: the literal a(b checklist line is included (wanted /literal paren token a\(b/ in oc.args)
FAIL R7/escapere: the phase line is still matched (no silent empty) (wanted /13\. bump: run release-bump/ in oc.args)
```

Pinned contract: a line is selected only when it LITERALLY contains the description
string (or matches the phase name, which may stay a regex); a metachar description
yields an answer (exit 0, both xruns) and never silently empties the context. The
scenario is not vacuously red: 4 of its 7 rows pass today (answer produced, the literal
`[xyz]` line included — it matches under either semantics, the phase line for the
bracket case). **F's degree of freedom:** any fixed-string mechanics (e.g. a union of
`grep -iE "$phase"` and `grep -iF -- "$first_word"`, or awk `index()`), as long as a
bad-regex description can neither mis-select nor kill the whole context.

## B3. "verifytrapall" — every ph_verify stop path trap-cleans (GREEN pins)

Production already sets `VERIFY_TMP="$vdir"` before the first stop (release.sh:732-733)
and adds the installer `--dest` before its run; the EXIT trap cleans both on every
stop. What was missing was COVERAGE: only success (`verifyok`), installer failure
(`verifystop`) and the forced kill (`verifyterm`) were driven. Eight new legs (`vleg`,
each: steer → resume `--publish` → stop names its cause AND phase verify-published →
`vleft` + `tmpleft` nothing survives under the steered TMPDIR), all GREEN:

| leg | steer | stop path driven |
|---|---|---|
| `vtdownload` | `dl-fail` | gh release download fails |
| `vtassets` | `dl-partial` | incomplete assets (only the zip arrives) |
| `vtsums` | `dl-badsums` | SHA256SUMS verification fails |
| `vtupmismatch` | `dl-upmismatch` | updater zip differs but sums cover the differing bytes (so `shasum -c` passes, `cmp` fails — the real check order) |
| `vtditto` | `dl-notzip` | checksum-valid non-zip → real `ditto -x -k` fails |
| `vtverify` | `verify-fail` | release-verify.sh fails |
| `vtmount` | `mount-fail` | hdiutil attach fails (no mount point) |
| `vtdmg` | `dmg-diff` | the dmg's Kai.app differs from the zip's (detach-then-stop path) |

The installer-`--dest` half stays pinned by `verifystop`/`verifyok` (it exists only on
the last two stops; macOS's no-template `mktemp` ignores `$TMPDIR`, so it is observed
via the recorded `--dest` path, not steered — unchanged from round 3).

## C. Harness hygiene

### C1 — recording stubs, fail-by-default (GREEN: no scenario reaches them today)

- **NEW fail-by-default stubs** in `$FB` for `opencode`, `curl`, `codesign`: record argv
  to `$FAKE_STATE_DIR/unexpected.log`, print `UNEXPECTED REAL <tool>` on stderr, exit 127.
  No scenario steers them — ANY reach is a bug and fails the phase plus the finale.
- **Hardened fake `git`** (the existing pattern, hardened): local object operations pass
  through to the real git; any network subverb (`push|fetch|pull|ls-remote|clone|submodule`)
  is refused with 127 `UNEXPECTED REAL git (network <subverb>)` unless steered — the argv
  names, or the resolved `origin` is, the scenario's local bare repo (`FAKE_ORIGIN`,
  exported by `mkscn`), or `FAKE_GIT_ALLOW_NET=1`. The refusal fires BEFORE the exec, so
  the real network is never touched even when tripped.
- The already-steered fakes keep their pattern (they never fall through to a real
  binary; unhandled gh shapes exit 1 "unhandled"): `gh`, `hdiutil`, `osascript`, `pgrep`,
  `pkill`, `make`. `release-ask-ai.test.sh` gains the same `curl`/`codesign`
  fail-by-default stubs in `$T2` (inert there — nothing dispatches them).
- **Finale `norealbins`:** scans every `$tmp/*.sd/unexpected.log`; any trip in any
  scenario is a FAIL row naming the scenario and argv. GREEN today.

### C2 — `stubtrip` self-test (GREEN)

Proves the stubs trip: steer nothing, probe each stub directly — `opencode`, `curl`,
`codesign` (127 + `UNEXPECTED REAL <tool>`), the hardened `git` against a scratch repo
whose `origin` is `https://example.invalid/x.git` (127 + `UNEXPECTED REAL git`), local
git object ops still pass through (exit 0), the trips were recorded — then `rm -rf` the
probe dir ("then reset"), so the deliberate trips never feed `norealbins`. A scenario
that forgot to steer would fail its own rows exactly the way the probes assert.

### C3 — the real-opencode escape: found, explained, fixed

**Path it escaped through:** `scripts/release-ask-ai.test.sh` case **A08** ("an absent
binary is no-answer") ran with `PATH="$T2:$PATH"` on the assumption that `opencode` is
absent from the inherited PATH. False on any machine that runs this pipeline — opencode
IS the pipeline (here: `/opt/homebrew/bin/opencode`, verified with
`PATH="$T2:$PATH" command -v opencode`). So inside A08 the real script's
`command -v opencode` SUCCEEDED, execution reached `timeout 300 opencode run --format
json "$msg"`, and the fake `timeout`'s `shift; exec "$@"` **executed the REAL opencode**
— a real `opencode run` (network, potentially a live agent session) fired from inside
the test. This is also why A08's exit happened to be non-zero (real opencode without a
TTY/session errors) — the case "passed" for the wrong reason while leaking a process.

**Fix (harness-side, mine):** the absent-binary case now runs against a HERMETIC bin
(`H2`): `PATH` is EXACTLY one dir containing the fake `timeout`, `$BASH` (the script's
`#!/usr/bin/env bash` shebang must resolve), and symlinks to the coreutils
release-ask-ai.sh needs (jq awk tail head grep mktemp sed dirname cat) resolved from the
running shell. Absence is now REAL — no inherited binary is reachable at all. A08's
assertions are unchanged (still fail-closed + "no answer") plus two new rows: no
`300 opencode …` dispatch was recorded, and no stub tripped. In `release.test.sh` the
same class is closed structurally by the `opencode` fail-by-default stub.

## D. Class sweeps

### D1 — command-before-trailing-cleanup (failure propagation)

Audit of every phase/apply function in release.sh for the R4b masked-exit class:

| function | shape | verdict |
|---|---|---|
| `release_pr_apply` | ops then `rm -f "$bf"` | fixed in R4b; pinned `maskedexit*` — unchanged |
| `restructure_apply` | `awk >tmp \|\| {rm;return 1}` then **bare `mv`** then the postcondition re-check | an `mv` failure is not propagated directly but self-surfaces: the trailing `release-check-version.sh` re-check still fails (the CHANGELOG was not moved) → phase_run loop. Residual: the `CHANGELOG.md.rs.$$` tmp leaks in `$root` on that path. ADVISORY for F (guard or trap the tmp); not pinned — an `mv` failure cannot be steered in a throwaway dir we own without breaking every other git op |
| `handler_regen_bindings` | every op `\|\| return 1` | closed (R4b round) |
| `ph_needs` gh issue create | `gh … \| tail \| sed` under `set -o pipefail`; empty ISSUE → do_stop | propagates; unpinned corner (gh create failing) noted, low value |
| `st_update` | swallows jq failures (`else rm -f tmp`) | by design (state is a cache; absent state re-derives) — listed, not a defect |
| `issue_comment` / `notify` | `\|\| true` | by design (comments/notifications are niceties, never gates) |
| `ph_verify` | every op `\|\| do_stop`; end-of-phase `rm -rf` now trap-backed | pinned by B3 legs + verifyok/verifystop/verifyterm |

### D2 — flag combinations that a gate can silently skip (the grid)

| combination | today | pin |
|---|---|---|
| `--dry-run --smoke` | silently ignored (defect A) | **RED** `drystop` |
| `--dry-run --smoke-done` | silently ignored (same class) | **RED** `drystopdone` |
| `--dry-run --publish` | refused (release.sh:46) | GREEN guard `drystopub` (new pin) |
| `--dry-run --quit-kai` | acts (quit at phase 0; dry run completes) | GREEN guard `dryquit` (new pin) |
| `--publish --smoke` | acts (awaiting-owner exit 0 before publish) | covered by `smoke`/`smokeresume` semantics |
| `--smoke-done --publish` | acts (records, resumes) | `smokeresume` (existing) |
| `--smoke --smoke-done` (no dry-run) | acts with precedence (smoke-done wins, `--smoke` half dropped) | documented, NOT pinned: out of the task's listed grid; the combination necessarily cannot honor both flags and it is not a silent no-op of the whole combination. Flagged for F/O if a loud refusal is preferred |

### D3 — state-loss resume for every phase group (state.json absent)

The completed-release absent-state walk (`stateloss`, round 6) exercises EVERY group in
one pass; round 7 adds the four per-group rows that were missing (all GREEN guards):
branch group (`skipping phases 3-6` via the merged PR), review group (`settled by its
merge`), merge group (`pr merge` count stays 1 — no re-merge), local group
(`release-local.sh` count stays 1 — the 4-asset postcondition is recognized). Already
covered per group: changelog/bump/restructure/release-pr/publish rows in `stateloss`
itself, the tag group standalone with state absent (`tagpush`), the branch group
standalone (`leftover`, `mergedpr`). Carried, still open BY DECISION (round 6 notes A/B):
the `gh issue list` state-blindness residue (cosmetic duplicate checklist issue on a
closeout-time state-loss re-run) and mid-release state loss with an OPEN unmerged PR
(a loud stop with hand resolution — a refuse, not an ignore).

## E. bash 3.2 compatibility

- **Both harness files verified under real 3.2:** `/bin/bash scripts/release.test.sh` →
  377 ok / 11 FAIL (identical to the bash-5 run); `/bin/bash scripts/release-ask-ai.test.sh`
  → 33 ok / 3 FAIL (identical). `/bin/bash` here is 3.2.57; `bash` is 5.3.9.
- **New case `b32`:** production's shebang is `#!/usr/bin/env bash` (stock macOS → 3.2),
  so one whole scenario runs with the interpreter pinned: `/bin/bash "$release" v0.5.1`
  — the full flow (arg validation, lock, state, every phase function, the recovery loop,
  the traps) completes GREEN under 3.2 today.
- **Production findings for F: NONE.** Method: `/bin/bash -n` on all 17 non-test
  production scripts (parse-clean); a hazard-construct grep across them for
  `mapfile`/`readarray`, `declare -A`/`local -A`, `${var,,}`/`${var^^}` (either fold),
  `|&`, `&>`, negative array indices, `read -s`, `printf %q`, `${var@Q}`, `coproc`,
  `lastpipe`, and ANY array usage (production uses none — so no empty-array-expansion
  under `set -u` is possible); plus the empirical b32 run. Near-misses checked and found
  safe: `[[ "$tag" =~ ^v[0-9]+…$ ]]` (literal ERE, 3.2-safe), `$'\n'` ANSI-C quoting
  (3.0+), and the harness's own `${r_env[@]+"${r_env[@]}"}` guard idiom (the 3.2-safe
  empty-array form — kept in every new array use I added).

## Notes for F (advisories, non-blocking)

1. `restructure_apply`'s bare `mv` (D1 above): consider propagating it (`mv … || return 1`)
   so the tmp cannot leak — cosmetic, no pin.
2. The dry-run boundary banner's advice line ("record with --smoke/--smoke-done") stays
   accurate after the drystop fix: it advertises the NEXT run (default posture), where
   the flags are valid.
3. If you prefer `--smoke --smoke-done` (without --dry-run) to refuse loudly rather than
   take smoke-done precedence (D2 last row), say so in the F handoff and T will pin it;
   the current behavior already "acts", so it is not RED.

## Ready for F

RED is valid: 14 rows fail on feature assertions across three production defects —
(drystop/drystopdone) the silently-ignored posture contradiction, (handlercounts) the
shared blocker counter, (escapere) the ERE-interpolated command description. Every
premise row in those scenarios passes (the runs demonstrably complete/stop today for the
pinned reason), the guard rows prove the pins are not vacuous, and 94 new hygiene/guard
rows are green. Production must change in exactly: `scripts/release.sh` (arg validation
+ per-handler counters) and `scripts/release-ask-ai.sh` (fixed-string checklist match).
