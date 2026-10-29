# Handoff-T-red: Phase 4 — RED suite for issue #78 (static release workflow)

**Date:** 2026-10-19
**Branch:** `issue-0078-issue-78` (worktree `.worktrees/0078-issue-78`)
**Brief / handoff-A reviewed:** `docs/handoffs/0078-issue-78/brief.md` (AC1–AC7 as amended by A1–A10), `docs/handoffs/0078-issue-78/handoff-A.md` (rulings R1–R12 — the binding contract)
**Harness shape copied:** `scripts/release-preflight.test.sh` (`mkrepo`/`run`/`has`/`hasnt`/`exit_is`, fake `gh`, fake-`git`-wrapping-real-`git`, no network)

## A precondition

Confirmed: A returned **PASS** on the plan (`handoff-A.md`, 0 block / 6 warn settled / 4 nit) with rulings R1–R12 and amendments A1–A10. F has written no code; `scripts/release.sh` and `scripts/release-ask-ai.sh` do not exist (asserted by gate cases C01/A01).

## Tests authored

Two harnesses, both macOS-bash-3.2-safe (`set -uo pipefail`, no bash-4 constructs), no network, per-case `ok`/`FAIL` lines, `exit 1` if any case fails. Both are already wired into `test.unit.command` (`bash scripts/release.test.sh && bash scripts/release-ask-ai.test.sh` — operator amendment, decisions.md).

### `scripts/release.test.sh` — 36 case groups, 167 assertions (30 scenarios + C01 + 3 AC7 greps + 2 R5(5) sweeps)

Environment: throwaway git repo per scenario with a **local bare origin** (so real `git checkout -b`/`commit`/`push`/`tag`/`ls-remote` run for real, zero network), a **recording fake `git`** (logs every argv, then execs `/usr/bin/git`), a **stateful fake `gh`** (issues / PRs / releases / check-runs / PR-Agent review-state, env-steered by `FAKE_CI`, `FAKE_AGENT`), and env-steered fakes for every wrapped script (`release-preflight.sh`, `changelog-check.sh`, `release-bump.sh`, `release-local.sh`, `release-notes.sh`, `release-check-version.sh`, `release-tree-check.sh`, `release-verify.sh`, `release-package.sh`, `release-gates.sh`, `sign-check.sh`, `release-cert.sh`, `codesign-app.sh`, `install-release.sh`, `release-ask-ai.sh`) plus `pgrep`/`pkill`/`osascript`/`make`/`hdiutil` — all resolved by bare name because they sit **first on `PATH`** (R7's enabling constraint). Phase-13 fixtures are a real zip (real `ditto`/`shasum`/`cmp`/`diff` verify it) + a fake dmg only the fake `hdiutil` opens.

| Cases (scenario) | Pin | AC / ruling |
|---|---|---|
| C01 | `scripts/release.sh` exists+executable (the RED gate) | — |
| ac1life (3 runs) | `v0.5.1 --dry-run </dev/null` exit 0; no `[y/N]`; remaining steps draft/smoke/publish named; `release-preflight.sh v0.5.1`, `release-bump.sh 0.5.1` (**plain version**), `release-local.sh v0.5.1 --dry-run` recorded verbatim; notes + 4 assets + `Kai.app` under `bin/release/v0.5.1/`; state `.posture=="dry-run"`, `.phases.local.done==true`; checklist issue opened by exact title; known-issue candidates commented; preflight WARN → marker comment; branch **and** tag still pushed (dry-run is the release); PR to `--repo Performant-Labs/kai --base master`; merge `--merge --delete-branch`. Then `--publish` resumes at the boundary (escalation), publish = `release edit v0.5.1 … --draft=false`, installer runs, issue closed, `.posture=="publish"`, closeout done. Then full-success re-run is a no-op (no 2nd issue/PR/publish). No duplicate comments; every comment starts `<!-- release.sh: …` | AC1, AC2; R2, R6, R7, R8, R10 p0/p1/p2/p4/p6/p8/p12/p13/p14, A1, R11 |
| default (3 runs) | Score **exactly 90** + all RESOLVED + complete@head **passes** review; `release-local.sh v0.5.1 --yes`; draft `isDraft==true`; `.smoke=="skipped"` + smoke phase done + skip comment; publish NOT done; `--publish` resume completes (`isDraft==false`, publish done); then `--dry-run` re-run **refused** (posture escalation-only) | AC4; R1 (boundary), R8, R10 p11/p12, R6 |
| smoke (2 runs) | `--smoke` exits 0 awaiting owner, `.smoke != "skipped"`, smoke-phase marker comment; `--publish --smoke-done` records `.smoke` `done (owner-asserted …` and completes | AC4; R10 p11 |
| retry | unknown failure at bump + AI `retry`: exit 0, bump ran exactly 2×, **exactly 1** consultation (ask-ai invoked once, `consultations|length==1`), phase named in the consultation | AC3; R3, R4 |
| aitwo | fail-fail-pass + retry: 3 bump runs, exactly 2 consultations (second allowed only after first answer executed and failed again) | AC3; R3 |
| aicap | fail×3 + retry: stops, **cap 2** consultations (ask-ai never called a 3rd time), resume command printed | AC3; R3 |
| aibad | AI answers `reboot-the-mac` (out of allowlist): stop path, bump NOT re-run (1 invocation), refusal reported | AC3; R4 |
| ainone | AI produces no answer: stop (fail-closed), nothing retried, consultation attempted once | AC3; R9 |
| nostate | default run; `state.json` deleted; `--publish` re-derives from postconditions (no 2nd PR, one publish, closeout done) | AC2; R6 |
| kai / kaiquit | Kai running at phase 10 without `--quit-kai`: stop, 0 consultations, no blind retry, `--quit-kai` named; with `--quit-kai`: static handler quits (osascript/pkill recorded) and re-runs local (2 invocations), reaches draft, 0 consultations | AC3 blocker 1; R10 p10 [K] |
| leftover | merged `release/v0.5.1` on origin + MERGED PR: group-skip (bump invoked **0**×), branch deleted via `gh api -X DELETE …git/refs/heads/release/v0.5.1`, reaches draft, 0 consultations | AC3 blocker 2; R10 p3, #74 rule |
| bindings | bindings-drift failure at local: `make bindings` runs, result committed (`frontend/bindings` in git args), local re-runs and succeeds, 0 consultations | AC3 blocker 3; R10 p10 [K], #73 |
| identity | `codesign-app.sh --check` fails: stop naming `release-cert.sh`, 0 consultations, preflight never runs | AC3 blocker 4; R5 stop 1, R10 p0 [H] |
| chgstop | uncurated Known Issues: stop-for-owner naming the gap + printing the `changelog-skip` line (A10), 0 consultations; `<!-- known-issues: none YYYY-MM-DD -->` unblocks | R10 p2 [S], A10 |
| chgstrict | `changelog-check.sh --strict` uncovered: stop naming `#501` + skip line | R10 p2 [S] |
| locklive / lockdead | live-PID lock refused (`another release.sh is running`, `PID N` shown); dead-PID lock removed and retaken | R6 |
| corrupt | garbage `state.json`: stop, path printed, `delete it to rebuild from postconditions` | R6 |
| hsdraft / hspub | existing draft / published release: hard stop, 0 consultations, **no `release delete`, no `release edit`** ever | AC6; R5 stop 2 |
| hsci | red check-runs on the release commit: hard stop, 0 consultations, nothing built | AC6; R5 stop 3, R10 p7 |
| hsagent88 / hsagent95o | PR-Agent score 88 (below boundary); score 95 + one OPEN finding: both hard stops, 0 consultations, review gate named | AC6; R5 stop 4, R1 |
| nopush | no `gh` push rights at phase 0: hard stop, 0 consultations | R10 p0 [H] |
| R5-loop ×5 | the five hard-stop steerings each re-run with a canned AI answer `{"action":"retry"}`: stop stands, **ask-ai never invoked** (hard-stop check precedes consultation), phase command not re-run | AC6; R5, R4 |
| AC7 greps | `docs/releasing.md` + `docs/release-checklist.md` reference `scripts/release.sh`; CHANGELOG `[Unreleased]` carries `<!-- changelog-skip: release tooling, no user-facing change: #78 -->` | AC7 |
| R5(5) sweep | across **every** `git` invocation recorded by the whole harness: no `tag -d`/`tag --delete`, no `-f`/`--force`/`+refs/tags` push, ever | AC6; R5 stop 5 |

### `scripts/release-ask-ai.test.sh` — 9 case groups, 25 assertions

Fakes `timeout` and `opencode` on `PATH` (no model, no network). 250-line log fixture pins the "last 200 lines" prompt rule.

| Cases | Pin | Ruling |
|---|---|---|
| A01 | `release-ask-ai.sh` exists+executable (RED gate) | — |
| A02 | good envelope → exit 0, the **last text part** printed (earlier parts dropped) | R9 |
| A03 | argv = `run`, `--format`, `json`, **one** message arg (argc==4); prompt carries phase, failing command, log tail `LINE250` and not `LINE001` | R9, brief item 5 |
| A04 | wrapped in `timeout 300 opencode run --format json` | R9 |
| A05 | unparseable output → no-answer, non-zero | R9 |
| A06 | opencode exit 1 (the live `UnexpectedStatus: 500` envelope) → no-answer | R9, W-A3 |
| A07 | timeout kill (exit 124) → no-answer | R9 |
| A08 | opencode **absent** from PATH → no-answer (fail-closed, not a hang) | R9 |
| A09 | envelope with no text part → no-answer | R9 |

## RED confirmation

Baseline (without the harnesses) was verified GREEN by the operator (decisions.md, 2026-10-19: Go 20/20 packages, frontend 870/870, pristine command). RED below comes **only** from the two harnesses — the full-suite `&&` chain proves it in one run: it *reached* the harnesses, so swiftbridge+Go+frontend were green in the same invocation.

`bash scripts/release.test.sh` → **exit 1** (112 FAIL / 55 ok — every ok is a negative/count-zero assertion with failing positive siblings):

```text
FAIL C01 scripts/release.sh exists and is executable (feature absent: F implements it)
FAIL AC1: v0.5.1 --dry-run with no input exits 0 (exit 127, wanted 0)
FAIL AC1/R10p4: bump wrapped with the PLAIN version (no v) (wanted /^0\.5\.1$/ in bump.log)
FAIL AC1/R8: local wrapped as 'v0.5.1 --dry-run' (wanted /^v0\.5\.1 --dry-run$/ in local.log)
FAIL AC1/R6: state.json records the posture (jq '.posture' gave '', wanted 'dry-run')
…
FAIL AC3/R3: consultations capped at 2 per phase (askai.log has 0 lines, wanted 2)
…
FAIL R6: the stop says how to rebuild (wanted /delete it to rebuild from postconditions/)
…
FAIL AC7: docs/releasing.md points at scripts/release.sh as the operator (wanted /scripts/release\.sh/ in releasing.md)
FAIL AC7: CHANGELOG [Unreleased] carries the #78 skip line (wanted /changelog-skip: release tooling, no user-facing change: #78/ in CHANGELOG.md)
ok   AC6/R5-5: no tag is ever deleted
ok   AC6/R5-5: no force push is ever issued
FAILED
```

`bash scripts/release-ask-ai.test.sh` → **exit 1** (20 FAIL / 5 ok):

```text
FAIL A01 scripts/release-ask-ai.sh exists and is executable (feature absent: F implements it)
FAIL A02/R9: a good envelope exits 0 (exit 127, wanted 0)
FAIL A03/R9: the message is a single argv (run --format json + 1 arg) (oc.argc is '', wanted '4')
FAIL A03/R9: the prompt carries the log TAIL (wanted /LINE250/ in oc.args)
FAIL A04/R9: the call is wrapped in timeout (wanted /^300 / in timeout.log)
…
FAIL A09/R9: the no-answer condition is reported (wanted /[Nn]o answer/)
FAILED
```

Full suite command (`test.unit.command` verbatim) → **exit 1**, upstream green, harness last:

```text
… (swiftbridge build) …
ok  <75 Go package ok-lines>            # go test ./internal/... ./pkg/...
Test Files  79 passed (79)              # pnpm --dir frontend test
     Tests  870 passed (870)
<release.test.sh output begins — the 112 FAIL lines above>
FAILED                                   # suite exit 1; release-ask-ai.test.sh not reached
```

Every failure fails for the **right reason**: exit 127 (script absent), empty fake logs, absent `state.json`/draft files, and the three AC7 greps against not-yet-updated docs/CHANGELOG. No import/setup errors, no harness crashes, no hangs.

## Ready for F

**Confirmed RED is valid. F may implement `scripts/release.sh` and `scripts/release-ask-ai.sh` against these tests.** Wrapped scripts need zero modifications (R12); if F nevertheless touches one, the existing `scripts/*.test.sh` must be re-run green (AC5 — T-green will).

## Assumed — invented interface details, BINDING for F

1. **CLI**: `release.sh vX.Y.Z [--dry-run|--publish] [--smoke] [--smoke-done] [--quit-kai]`. Exit 0 on success **and** on awaiting-owner stops (draft stop, smoke await); non-zero on refusals/stops.
2. **state.json schema** (location per R6): `{ "posture": "dry-run"|"default"|"publish", "phases": { "<phase>": { "done": true, "commit": "<sha>" } }, "consultations": [ … ], "smoke": "skipped" | "done (owner-asserted YYYY-MM-DD)" }` — phase keys exactly R10's names (`needs, preflight, changelog, branch, bump, restructure, release-pr, review, merge, tag, local, smoke, publish, verify-published, closeout`); `consultations[]` append-only, ≥ `{phase}` per entry.
3. **Lock**: `bin/release/vX.Y.Z/.lock/` containing a `pid` file with the owner PID; live PID → message containing `another release.sh is running` and `(PID N)`; dead PID → remove and retake.
4. **Corrupt state**: stop message prints the path and `delete it to rebuild from postconditions`.
5. **release-ask-ai.sh CLI**: `release-ask-ai.sh <phase> <command> <logfile>` (argv order); success = last text part printed on stdout, exit 0; every no-answer case = a line containing `no answer` + non-zero exit.
6. **opencode envelope shape** (the fake's contract): JSON object with `parts[]`; text parts carry `.type=="text"` and `.text`; the answer is the **last** text part. (Real `opencode run --format json` output need only be a superset in irrelevant fields.)
7. **Marker comments** (R11): every checklist comment body starts `<!-- release.sh: vX.Y.Z <phase> <kind> -->`; the smoke-await and smoke-skip comments carry phase `smoke`.
8. **Leftover-branch deletion** goes through `gh api -X DELETE repos/<repo>/git/refs/heads/release/vX.Y.Z` (preflight's own hint, `release-preflight.sh:230`).
9. **Kai quit** via `osascript` (a quit AppleScript) or `pkill -x Kai` — either accepted; both are faked.
10. **Fake gh canned formats** (the parsing contract): `issue list`/`pr list` print TSV (`N\tTITLE\tSTATE`, `N\tSTATE`) or JSON `{"number","title","state"}` when `--json` is passed; check-runs print `name\tstatus\tconclusion` TSV; PR-Agent state is fetched via `gh pr view <N> --json …comments…` **or** `gh api repos/<repo>/pulls/<N>/comments` — comment body contains the `<!-- pr-agent-review-state:v1` JSON block and the `<td>🏅 … Score</td>` line; `release view` returns `{"isDraft", "assets"}`.
11. **Failure signatures** the known-blocker/hard-stop tables must recognize (the fakes emit these verbatim, mirroring the real scripts' wording): `Kai is running` (+ `pgrep -x Kai` exit 0); `frontend/bindings differs` + `make bindings`; `a release for vX.Y.Z already exists` (draft and published variants); `codesign-app.sh --check` exit 1; uncurated = `[Unreleased]` lacks a `### Known Issues` bullet **and** lacks `<!-- known-issues: none YYYY-MM-DD -->`; uncovered = `changelog-check.sh --strict` non-zero with `NOT covered`.
12. **Posture refusal** message contains `posture` or `dry-run`; **stop** messages print a `resume` command; out-of-allowlist refusal says `refus`/`allowlist`.
13. **No origin-URL check of its own**: the harness origin is a local bare repo path — release.sh must leave origin validation to the wrapped `release-preflight.sh` (R10 p1, R12) and honor `KAI_RELEASE_REPO` like `release-preflight.sh:15`.
14. **Phase-13 primitives**: `gh release download vX.Y.Z` yields the 4 files in cwd (or `--dir D`); `hdiutil attach` prints `<dev>\tApple_HFS\t<mountdir>`; `install-release.sh vX.Y.Z --dest <dir> --keep-permissions` prints its three result lines. R10's phase-13 sequence runs against exactly these.
15. **Consultation wiring**: the executor invokes `release-ask-ai.sh` (bare name) with the phase as the first argument; consultations are counted per phase (R3).

## Hedged

- **[W] wait/poll class not exercised** (pending CI, stale review head, AI `wait` action): steering it would make the harness wait real minutes (R4's default 10-min window). Covered indirectly: every steerable wait precondition has a green-or-stop sibling. Residual risk for T-green: an unbounded poll loop in F would surface as a harness/suite timeout.
- **"exactly the remaining owner steps"** (R2) pinned as presence of the draft/smoke/publish concepts, not exact wording; smoke-await's intermediate state value pinned only as ≠ `skipped`.
- AC5's second half (pre-existing `scripts/*.test.sh` stay green) is T-green's re-run after F's diff; AC1's live rehearsal remains post-merge, owner-driven (R2).

## Decided

- Harness = fakebin-first `PATH` over a **local bare origin** with a recording fake `git` pass-through: real git semantics for branch/commit/push/tag/ls-remote with zero network, plus the verbatim argv record R5(5)'s negative sweep needs (extends `release-preflight.test.sh`'s fake-git pattern per NV-4).
- Gate cases C01/A01 make feature-absence the first named failure; every scenario also has positive assertions so no case can pass vacuously at RED or GREEN.
- The full-suite run doubles as the baseline proof: the `&&` chain reaching the harnesses demonstrates swiftbridge+Go+frontend green in the same invocation that fails on the harnesses.
