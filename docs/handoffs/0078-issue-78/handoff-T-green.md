# Handoff-T-green: Phase 6 — GREEN + Tier 2 (issue #78: static release workflow)

**Date:** 2026-10-20
**Branch:** `issue-0078-issue-78` (worktree `.worktrees/0078-issue-78`, HEAD `3c6522d`)
**Issue:** #78
**Handoff-F reviewed:** `docs/handoffs/0078-issue-78/handoff-F.md` (incl. the post-GATE addendum)
**Handoff-T-red:** `docs/handoffs/0078-issue-78/handoff-T-red.md`

**Verdict:** PASS

## GREEN confirmation

The authoritative suite (`test.unit.command` from `.aftersight/pipeline.config.json`, verbatim, run
by me from the worktree root, 911-line log captured) — **exit 0**:

```
sh -c "(cd pkg/swiftbridge/scripts && bash ./build.sh) && go test -vet=off ./internal/... ./pkg/... -count=1 && pnpm --dir frontend test && bash scripts/release.test.sh && bash scripts/release-ask-ai.test.sh"
SUITE_EXIT_CODE=0
```

Verbatim summary lines from that run:

```text
>> done: pkg/swiftbridge/libkai_bridge.dylib            # swiftbridge build (log line 42)
>> done: pkg/swiftbridge/libkai_bridge.a                #   (log line 43)
ok  	cnb.cool/dtapp/kai/internal/buildinfo	0.298s     # first of 20 Go package ok-lines
ok  	cnb.cool/dtapp/kai/internal/configstore	0.889s
 Test Files  79 passed (79)                             # pnpm --dir frontend test
      Tests  870 passed (870)
ok   AC6/R5-5: no tag is ever deleted                    # release.test.sh: 166 ok / 0 FAIL
ok   AC6/R5-5: no force push is ever issued
PASS                                                    # release.test.sh trailer (log line 886)
ok   A08/R9: an absent binary is no-answer (fail-closed, not a hang)
ok   A09/R9: an envelope with no text part is no-answer
ok   A09/R9: the no-answer condition is reported
PASS                                                    # release-ask-ai.test.sh trailer (log line 911, final)
```

Totals: 210 `ok` lines = 20 Go packages + 166 (`release.test.sh`) + 24 (`release-ask-ai.test.sh`);
**0 `FAIL` lines anywhere in the log**. This matches the plugin's recorded t-green verdict
(GREEN, exit 0) and F's and S's reported numbers exactly — no discrepancies.

Both harnesses standalone (step 2 of the dispatch), independent invocations:

```text
bash scripts/release.test.sh        → exit 0, ok=166 FAIL=0, trailer PASS
bash scripts/release-ask-ai.test.sh → exit 0, ok=24  FAIL=0, trailer PASS
```

Assertion-count continuity with RED (same files, zero edits): release.test.sh emitted 167 lines at
RED (112 FAIL + 55 ok = 166 checks + FAILED trailer) and 166 checks + PASS trailer at GREEN;
release-ask-ai.test.sh emitted 25 lines at RED (20 FAIL + 5 ok) and 24 checks + PASS at GREEN.
The check inventory is unchanged — every RED check exists and now passes.

**Pin check (behavior, not implementation).** RED→GREEN on untouched harness bytes is itself the
pin: the identical 190 assertions failed against absent behavior (exit 127, empty fake logs, absent
state/docs) and pass against F's implementation — nothing about F's particular code structure
satisfies them by construction. The suite's negative/count-zero assertions (R5(5) sweeps over every
recorded `git` argv; "ask-ai never invoked" across five steered hard stops; "0 consultations" on
every blocker) also assert the *absence* of behavior, so a hollow implementation cannot pass them.
I did not mutate production code to re-demonstrate a failure — editing implementation is outside T's
write scope; the RED record is the documented baseline for the same bytes.

## No-test-edits proof (dispatch step 3)

The branch carries RED and F squashed into one feature commit (`b44533c`, 2026-10-19 — the RED
authorship day; F was dispatched after it, journal "Phase F — implement"). Proof the harnesses are
byte-identical to what T wrote, modulo nothing:

```text
git log --oneline -- scripts/release.test.sh          → b44533c (only commit, ever)
git log --oneline -- scripts/release-ask-ai.test.sh   → b44533c (only commit, ever)
git log --oneline b44533c..HEAD -- scripts/           → (empty: no later commit touches scripts/)

git rev-parse b44533c:scripts/release.test.sh           = 11b7eea3e65243d952f5c1984b28ea2ecbee77a8
git rev-parse HEAD:scripts/release.test.sh              = 11b7eea3e65243d952f5c1984b28ea2ecbee77a8
git hash-object scripts/release.test.sh   (worktree)    = 11b7eea3e65243d952f5c1984b28ea2ecbee77a8
git rev-parse b44533c:scripts/release-ask-ai.test.sh    = 0541e3b05d37c6fc6cb12cc6cefd2052445b2339
git rev-parse HEAD:scripts/release-ask-ai.test.sh       = 0541e3b05d37c6fc6cb12cc6cefd2052445b2339
git hash-object scripts/release-ask-ai.test.sh (worktree)= 0541e3b05d37c6fc6cb12cc6cefd2052445b2339
git status --porcelain                                  → (clean)
```

Cross-check against the gate record: the harness sections of `.stage/diff.patch` (the diff the
dual-review round 2 PASS'd) are sha256-identical to the corresponding sections of a fresh
`git diff origin/master..HEAD` — `release.test.sh` `cdac2f62f1a316a6…` both sides,
`release-ask-ai.test.sh` `0b4bdd381de640f4…` both sides. Conclusion: **zero test edits since RED
authorship**; F's claim ("T's files, untouched") holds.

## Assumed-list conformity (dispatch step 4) — all 15 honored, one line each

| # | Assumed (contract for F) | Status — evidence (harness green + source anchor) |
|---|---|---|
| 1 | CLI `vX.Y.Z [--dry-run\|--publish] [--smoke] [--smoke-done] [--quit-kai]`, exit 0 on success + awaiting-owner stops | **Honored** — usage `release.sh:8`, flag parse `:34-41` (`--dry-run/--publish/--smoke/--smoke-done/--quit-kai`, unknown → exit 2); ac1life/smoke/default scenarios green |
| 2 | `state.json` schema: posture / 15 phase keys / append-only `consultations[]` ≥ `{phase}` / smoke value | **Honored** — `phase_done`/`mark_done` on `.phases[$p].done/.commit`; 15 keys init `done:false` (F handoff); `consult_count` filters `.consultations[]? by .phase` (`:94`); scenarios assert postures, `local.done`, `smoke` values |
| 3 | Lock `bin/release/vX.Y.Z/.lock/` with pid file; live → "another release.sh is running (PID N)"; dead → retake | **Honored** — `lockdir="$statedir/.lock"` `:56`, message `:70`; locklive/lockdead green |
| 4 | Corrupt state: path + `delete it to rebuild from postconditions` | **Honored** — `release.sh:102` verbatim; corrupt green |
| 5 | `release-ask-ai.sh <phase> <command> <logfile>`; last text part, exit 0; no-answer line + non-zero | **Honored** — invoked `release-ask-ai.sh "$phase" "$desc" "$log"` (`release.sh:266`); `release-ask-ai: no answer ($1)` (`ask-ai:31`); A02–A09 green |
| 6 | opencode envelope: `parts[]`, text parts, **last** text part wins | **Honored** — `jq -r '.parts \| map(select(.type == "text")) \| last \| .text // empty'` (`ask-ai:66`); A02/A09 green |
| 7 | Marker `<!-- release.sh: vX.Y.Z <phase> <kind> -->` on every checklist comment; smoke phases carry `smoke` | **Honored** — `printf '<!-- release.sh: %s %s %s -->'` (`release.sh:136`); no-duplicate-comment asserts green |
| 8 | Leftover-branch deletion via `gh api -X DELETE …git/refs/heads/release/vX.Y.Z` | **Honored** — `release.sh:411` exact; leftover green (bump 0×, 0 consultations) |
| 9 | Kai quit via osascript AppleScript or `pkill -x Kai` | **Honored** — `tell application "Kai" to quit` `:203`, `pkill -x Kai` fallback `:205`; kai/kaiquit green |
| 10 | Fake-gh canned formats (TSV/JSON lists, check-runs TSV, PR-Agent state via comments, `release view` isDraft/assets) | **Honored** — dual JSON/TSV parsing (F handoff); default/review/leftover/hsdraft scenarios green |
| 11 | Failure signatures (Kai running, bindings drift, release exists, codesign check, uncurated/ uncovered changelog) | **Honored** — known-blocker + hard-stop tables; kai/bindings/leftover/identity/chgstop/chgstrict/hsdraft/hspub green, all 0-consultation |
| 12 | Posture refusal says posture/dry-run; stops print a resume command; allowlist refusal says refus/allowlist | **Honored** — posture escalation-only `:105-111`; `do_stop` carries resume command; aibad refusal green |
| 13 | No origin-URL check of its own; `KAI_RELEASE_REPO` honored | **Honored** — `repo="${KAI_RELEASE_REPO:-Performant-Labs/kai}"` `:25`; `grep origin.*url\|remote get-url` → 0 matches |
| 14 | Phase-13 primitives (`gh release download`, `hdiutil attach` → mountdir, `install-release.sh --dest --keep-permissions`) | **Honored** — `:659`, `:673` (awk `$3` mountdir), `:680`; verify-published done in ac1life resume |
| 15 | Consultation wiring: bare-name dispatch, phase as first arg, per-phase cap from state | **Honored** — `run_tool` bare-name `:121-124`; phase-first `:266`; cap from `state.consultations[]` (`:94`, `:262-265`); retry/aitwo/aicap/aibad/ainone green (1/2/cap-2/refused/fail-closed) |

F's disclosed deviations (handoff-F §Deviations 2–6) concern **ruling letter, not the Assumed
contract**: R11 posting mechanics, the [W] wait class, `propose-fix-pr` executor half, merge-commit
fallback, checklist-link placement. None touches a harness Assumed detail; each is recorded as a
known issue below.

## Tier 1 results

| Check | Command / method | Expected | Actual | |
|---|---|---|---|---|
| Build | suite leg 1 (swiftbridge `build.sh`) | dylib + .a built | `>> done: libkai_bridge.dylib` / `.a` | PASS |
| Go tests | suite leg 2 (`go test -vet=off ./internal/... ./pkg/... -count=1`) | 20/20 ok | 20 package ok-lines, 0 FAIL | PASS |
| Frontend tests | suite leg 3 (`pnpm --dir frontend test`) | 79 files / 870 tests | 79 passed (79), 870 passed (870) | PASS |
| New harnesses (in-suite) | suite legs 4–5 | exit 0 | 166+24 ok, 0 FAIL, both `PASS` | PASS |
| New harnesses (standalone) | separate invocations | exit 0 | release 166/0, ask-ai 24/0, both `PASS` | PASS |
| Server starts / API smoke | — | — | N/A — Wails desktop app, no server/API; the suite is the smoke | N/A |
| Testcontainer leftovers | `docker ps --filter label=org.testcontainers=true` | none | none (no containers) | PASS |
| Worktree cleanliness | `git status --porcelain` | clean (nothing uncommitted beyond handoffs) | clean | PASS |

Cross-check vs F/S: F reported 166/24 ok and full-suite exit 0; S independently re-ran and reported
the same; the plugin recorded GREEN exit 0. My run agrees with all three — no discrepancies.

## Tier 2 results

| Check | Method | Result |
|---|---|---|
| Coverage per AC | RED case table → GREEN run (below) | PASS — every AC has ≥1 green backing case |
| Test quality / proportionality | 190 assertions over 744+72 lines of orchestrator+client; a-dup check 5 (no assertion overlap between harnesses); assertions read exit codes/stdout/fake argv logs/state files — the only source greps are AC7's docs greps, which the spec itself requires | PASS |
| Fails-in-isolation / right reason | RED record (exit 127, empty logs, absent state) on identical bytes | PASS |
| Type safety | N/A (bash; no typecheck configured — `pluginTypecheck: null`, sanctioned) | N/A |
| Error handling | fail-closed paths green: `ainone`, `aibad`, `corrupt`, `locklive`, A05–A09; `do_stop` prints log tail + resume command | PASS |
| Data integrity | atomic `st_update` (jq→tmp→same-dir `mv`), mkdir lock with PID liveness, corrupt-state stop; idempotent no-op re-run green | PASS |
| API contract (gh/opencode shapes) | A10 canned formats + A02–A09 envelope cases green | PASS |
| Security | tag-format validation `^v[0-9]+\.[0-9]+\.[0-9]+$` (`:45`); no credentials/secrets in either script; AI answers allowlisted before execution | PASS |
| Migration safety | N/A — no DB/schema migrations (state.json is a documented cache, R6: delete-to-rebuild) | N/A |
| Playwright / e2e | no `e2e/` workspace exists (Wails desktop app) — nothing to run, no skipped surface to flag | N/A |
| R12 (zero wrapped scripts modified) | `git diff --name-status origin/master..HEAD -- scripts/` → only the four `A` files (release.sh, release-ask-ai.sh + two harnesses) | PASS |

## Acceptance criteria status

| AC | Status | Backed by (all green in my run) |
|---|---|---|
| AC1 dry-run reaches packaged posture, no prompts, remaining steps printed | PASS | ac1life (exit 0 under `</dev/null`, no `[y/N]`, draft/smoke/publish named, local/dry-run wraps verbatim, disk artifacts, posture, branch+tag pushed, PR target, merge flags). Live half = post-merge rehearsal per R2/A1 (hedged at RED) |
| AC2 kill/resume postcondition-first; re-run no-op | PASS | ac1life 3-run arc, nostate (state deleted → re-derived), corrupt/locklive/lockdead |
| AC3 blockers handled without AI; consultation protocol | PASS | kai/kaiquit, leftover, bindings, identity (0 consultations each); retry/aitwo/aicap (1/2/cap-2), aibad (refused), ainone (fail-closed) |
| AC4 postures + smoke | PASS | default (score exactly 90 passes, draft-default, smoke skipped recorded), smoke/smoke-done, posture escalation-only refusal |
| AC5 harnesses in suite, pass clean; pre-existing harnesses stay green | PASS | Suite legs 4–5 (exit 0); standalone runs; 9/10 pre-existing harnesses green with the tenth failing identically on master (see known issues) |
| AC6 no prompts; never delete/overwrite release or tag; hard stops refuse even when AI says retry | PASS | hsdraft/hspub/hsci/hsagent88/hsagent95o, R5-loop ×5 (stop stands, ask-ai never invoked), R5(5) argv sweeps |
| AC7 docs tell the story; CHANGELOG skip line | PASS | the three AC7 greps (releasing.md, release-checklist.md, CHANGELOG) green in-suite |

## Blocking issues

None.

## Known issues carried (dispatch step 5 — for the record, non-blocking)

From the diff gate (`dual-review-diff.md`, round 1 BLOCK → round 2 **PASS**; residue = the
post-merge hardening list, decisions.md "Diff gate (2026-10-20)"):

1. Six WARN + three NIT, all cosmetic-to-minor, none touching an AC: blocker-attempt counter shared
   across blocker kinds (`release.sh:243-252`; mixed Kai+bindings in one phase would share the cap
   of 2); `--dry-run --smoke` combination silently ignored; one unescaped ERE in
   `release-ask-ai.sh:49`; `$vdir` cleanup on stop paths in `ph_verify`; `sed -n '2,/^set -uo/p'`
   help-slice fragility (`release.sh:39`); `set -- $info` positional reuse.

From the anti-duplication gate (`handoff-A-dup.md`, **PASS** — disclosed ruling-letter deviations):

2. **W-1** R4 `propose-fix-pr` executor half de-scoped to record-and-stop (answer schema carries no
   patch to apply); O to amend R4's text or schedule follow-up.
3. **W-2** R10 [W] wait class implemented as stop-or-one-bounded-60s-pause, not a poll loop (T
   hedged this class as unexercised at RED); resume command is the documented recovery.
4. **W-3** R11 marker+state posting instead of search-before-post; cosmetic residual: after a
   deliberate state delete, a posted-but-not-done phase may re-post its comment.
5. **N-1** CI pass predicate stated in two places (`release.sh:173` vs `release-gates.sh:49`) —
   plan-directed (R1(a) + R12); **N-2** stale #31 future tense in `releasing.md:236-245` old
   section (post-merge docs pass); **N-3** pipeline-config nulls disclosure; **N-4** = the shared
   counter of item 1.

Environmental / pre-existing:

6. `changelog-check.test.sh` fails exactly one case (`no --since and no tag: old PR too`) —
   **identically on master** (verified by S both sides): pre-existing live-PR drift in a wrapped
   script this diff never touched (R12). Not in `test.unit.command`; carried as AC5's documented
   exception.
7. Post-merge rehearsal list (owner-driven, R2): live `--dry-run` on clean master; `opencode run`
   availability on the release Mac (currently 500s, W-A3); GNU `timeout` presence (macOS ships
   none — brew coreutils) or every consultation is honestly fail-closed; real-gh shapes beyond the
   fakes'; the #74 group-skip and bindings handler against real conditions.

## Advisory notes

- The squashed single feature commit (`b44533c`) carrying both harnesses and implementation is the
  one weakener of per-phase provenance; the blob-hash + gate-diff cross-check above is the
  compensating evidence. Future runs that can commit RED separately should.
- Suite runtime note for the operator: the full chain (swiftbridge + Go + frontend + harnesses) is
  several minutes; both harnesses are seconds standalone — prefer them for F-iteration loops.
