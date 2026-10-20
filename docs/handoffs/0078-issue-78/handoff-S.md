# Handoff-S: Phase 8 — Spec audit (issue #78: static release workflow)

**Date:** 2026-10-20
**Branch:** `issue-0078-issue-78` (worktree `.worktrees/0078-issue-78`, HEAD `616d7aa`, diff base `origin/master` = `fe7ca50`)
**Issue:** Performant-Labs/kai#78
**Handoff-T reviewed:** `docs/handoffs/0078-issue-78/handoff-T-red.md` (T-green = plugin record, see below)
**Handoff-A reviewed:** `docs/handoffs/0078-issue-78/handoff-A.md` (R1–R12, A1–A10), `docs/handoffs/0078-issue-78/handoff-A-dup.md`
**Handoff-F reviewed:** `docs/handoffs/0078-issue-78/handoff-F.md` (incl. the post-GATE addendum)
**Gates reviewed:** `dual-review-brief.md` (PASS), `dual-review-diff.md` (round-2 PASS), `decisions.md` (operator rulings)
**Operator-facing report:** N/A — non-visual story (`uiSurface: false` in run state; Phase D/U declared N/A). Markdown handoff only.

## A precondition

Confirmed. A returned **PASS** up-front (handoff-A.md:6 `**Verdict:** PASS`; 0 block / 6 warn, each
settled by a binding ruling R1–R12, amendments A1–A10) and A-dup returned **PASS**
(handoff-A-dup.md:7 `**Verdict:** PASS`). Both dual-review gates PASS on record
(`dual-review-brief.md:28`, `dual-review-diff.md:8`) and in run state (`dualReview.brief.outcome=PASS`,
`dualReview.diff.outcome=PASS`).

## T precondition

Confirmed. The plugin's t-green record shows the suite **GREEN, exit 0** on the exact amended
command (`state.json` → `test.green[0].verdict: "GREEN", code: 0`, command ending
`… && bash scripts/release.test.sh && bash scripts/release-ask-ai.test.sh`). No blocking issues
from T; the T handoff's own contract (192 assertions, Assumed list) is the audit baseline below.

## Browser / visual-diff preconditions

N/A — no UI surface (`uiSurface: false`, declared in run state and decisions.md:14-15; Phase D and
U skipped-and-declared per AGENTS.md). Non-visual audit: markdown handoff only, per charter.

## Spec sanity check (pre-audit)

No `ADVISORY-HOLD` triggers. The one real spec ambiguity (issue: "dry-run = everything up to the
draft, nothing public" vs. A's three-posture mapping) was resolved through the sanctioned
amendment mechanism (A1/R8, journaled decisions.md:93-96), not left contradictory. The amended
spec is internally consistent and testable.

## Evidence I produced myself (Tier 3)

| Command (run by S, in the run worktree) | Result |
|---|---|
| `bash scripts/release.test.sh` | **exit 0** — 166 `ok` / 0 `FAIL`, `PASS` |
| `bash scripts/release-ask-ai.test.sh` | **exit 0** — 24 `ok` / 0 `FAIL`, `PASS` |
| Full `test.unit.command` verbatim (config string) | **exit 0** — swiftbridge `>> done: pkg/swiftbridge/libkai_bridge.dylib`; 210 total `ok` lines (= 20 Go package ok-lines + 166 + 24 harness oks); frontend `Test Files 79 passed (79)` / `Tests 870 passed (870)`; both harness `PASS` trailers |
| All 10 pre-existing `scripts/*.test.sh` | 9 PASS; `changelog-check.test.sh` exit 1 on exactly one case — `FAIL no --since and no tag: old PR too (wanted /#47\b/)` |
| Same harness on master (`835d3b2`, primary checkout) | **identical single FAIL** — pre-existing live-PR drift, not this diff (the wrapped script was not touched: `git diff --name-status origin/master..HEAD -- scripts/` shows only `A scripts/release.sh`, `A scripts/release-ask-ai.sh`, `A scripts/release.test.sh`, `A scripts/release-ask-ai.test.sh`) |
| `grep -nE 'tag (-d\|--delete)\|--force\|+refs/tags\|release delete\|read -r\|\[y/N\]'` over both new scripts | no matches — no tag deletion, no force push, no release delete, no terminal read anywhere |

## Per-AC verdicts

The spec audited against = issue #78's Design/Phases/Acceptance **as amended** by brief.md AC1–AC7,
handoff-A rulings R1–R12 + amendments A1–A10, and decisions.md operator rulings (live rehearsal
post-merge per R2/A1; env-fix split #98/#99 merged as `fe7ca50`, the rebase base — verified:
`git merge-base HEAD origin/master` = `fe7ca50`).

| AC | Verdict | Evidence (mine unless noted) |
|---|---|---|
| **AC1** dry-run, no input, verified/packaged posture, remaining owner steps printed | **PASS** (verification mode = fake-script harness per R2/A1; live half → not-verifiable-here) | `release.test.sh` ac1life: `rrun v0.5.1 --dry-run` with `</dev/null` (harness:415-417) asserts exit 0 (437), no `\[y/N\]` (441), remaining steps draft/smoke/publish (438-440), verbatim wraps `release-local.sh v0.5.1 --dry-run` (444), packaged notes + `Kai.app` + 4 staged assets under `bin/release/v0.5.1/` (445-450), `.posture=="dry-run"` and `.phases.local.done==true` (451-452), branch+tag still pushed (456-457), PR `--repo Performant-Labs/kai --base master` (458-459). Code: dry-run block prints "Remaining owner steps: 1. draft … 2. smoke … 3. publish" and exits 0 (`release.sh:716-726`); `ph_local` picks `--dry-run` mode (`release.sh:585-587`). Green in my run. |
| **AC2** kill/resume postcondition-first; full-success re-run is a no-op exit 0 | **PASS** | ac1life: `--publish` resumes at the boundary (463-469), third run no-op exit 0 with exactly one `issue create` / `pr create` / `release edit` each (470-474), no duplicate comments and every comment marker-tagged (475-478). `nostate`: state deleted → re-derives from postconditions (563-573). Every phase opens with a `phase_done` probe plus postcondition re-derivation (e.g. `release.sh:464` restructure, `release.sh:587-601` local, `release.sh:639-644` publish, `release.sh:406-427` branch/#74). Corrupt state and live/dead lock pinned (harness `corrupt`/`locklive`/`lockdead`; code `release.sh:100-104`, `:67-76`). Green in my run. |
| **AC3** four known blockers handled without AI; unknown failure → exactly one consultation; out-of-allowlist refused | **PASS** | Harness, all green in my run: `kai` (stop, 0 consults, no blind retry; `--quit-kai` quits + re-runs, reaches draft, 0 consults — 575-596); `leftover` (#74 group-skip: `bump.log` **0** invocations, `gh api -X DELETE …refs/heads/release/v0.5.1`, 0 consults — 598-611); `bindings` (`make bindings` + `frontend/bindings` committed, local re-ran, 0 consults — 613-621); `identity` (stop naming `release-cert.sh`, 0 consults, nothing after phase 0 — 623-630). Consultations: `retry` exactly 1 (519), `aitwo` exactly 2 (530), `aicap` capped at 2 + stop (539), `aibad` out-of-allowlist → stop, phase not re-run, refusal reported (544-552), `ainone` fail-closed (555-561). Code: static table before consult (`release.sh:240-255`), cap 2/phase from `state.consultations[]` (`:262-265`), allowlist dispatch with refusal (`:277-313`), `run-handler` names validated (`:301-303`). |
| **AC4** `--publish`/`--smoke` as settled (draft-default, smoke-skipped recorded); rehearsed via fake harness | **PASS** (live rehearsal → not-verifiable-here) | `default`: review passes at Score **exactly 90** + all RESOLVED (486, R1 boundary), `release-local.sh v0.5.1 --yes` (487), draft `isDraft==true` (488), `.smoke=="skipped"` + phase done + skip comment (489-492), publish NOT done (491), `--publish` completes `isDraft==false` (494-497), `--dry-run` re-run refused (498-500). `smoke`: `--smoke` exits 0 awaiting-owner, `.smoke != "skipped"` (504-507); `--smoke-done` records `done (owner-asserted …)` and completes (508-509). Code: `ph_smoke` (`release.sh:606-632`), posture escalation-only (`:105-111`). |
| **AC5** both harnesses pass from a clean worktree, part of `test.unit.command`; pre-existing harnesses stay green | **PASS** (one pre-existing, master-identical failure documented) | My runs: both harnesses exit 0 (166 + 24 ok). Config diff shows `test.unit.command` now ends `… && bash scripts/release.test.sh && bash scripts/release-ask-ai.test.sh` (`.aftersight/pipeline.config.json`, diff `+` line). Full suite command exit 0 in one chain. Pre-existing: 9/10 PASS; `changelog-check.test.sh` fails one case **identically on master** (my runs both sides) — live-PR drift in a script this diff never touched (R12 verified: only 4 new files under `scripts/`). "Nothing wrapped broke" holds. |
| **AC6** never prompts on the happy path; never deletes/overwrites a release or published tag; hard-stop list encoded; hard stops refuse even when the AI says retry | **PASS** | No `read`/prompt pattern in either script (my grep). `hsdraft`/`hspub`: existing draft/published release → hard stop, 0 consults, **no `release delete`, no `release edit`** ever (688-706). `hsci`: red CI → stop, nothing built (709-714). `hsagent88`/`hsagent95o`: score 88 below boundary and 95-with-OPEN-finding both stop, 0 consults (717-729); boundary `"$score" -lt 90` fails ⇒ 90 passes (`release.sh:195`). R5 refusal loop: all five hard-stop steerings re-run with canned `{"action":"retry"}` — stop stands, ask-ai **never invoked** (739-755); code checks hard stops before consulting (`release.sh:233-238` precede `:258+`). R5(5) sweep: across **every** git invocation recorded by the whole harness, no `tag (-d|--delete)`, no `push.*(-f |--force |\+refs/tags)` (762-768); tag phase refuses to move an existing tag (`release.sh:566-573`). |
| **AC7** docs tell the same story; manual remainder said once; CHANGELOG skip line | **PASS** | Harness greps green (758-760): `docs/releasing.md` and `docs/release-checklist.md` reference `scripts/release.sh`; `[Unreleased]` carries `<!-- changelog-skip: release tooling, no user-facing change: #78 -->` (CHANGELOG diff, shape-identical to the #74/#75 precedent lines above it). Read the diff myself: new §"The operator: `scripts/release.sh`" (releasing.md:10-36) with "What stays manual, on purpose — said once, here: the **manual smoke matrix** … and the **publish go-ahead**" (:29-31), the A9 OS-level-prompt sentence (:33-34), the A6 backup marker (`touch bin/release/kai-release-key.backed-up`, :35-36); checklist header names the two owner steps and defers (:12-16); "Step by step" cross-ref (:377-380) points up without restating. |

## Test-quality audit (rubric over T's harnesses)

- **One behavior per test:** yes — each `ok/FAIL` line is a single labeled assertion (AC/ruling in
  its name), verified by reading the harness source (assertions at `release.test.sh:437-478,
  486-500, 504-509, 519-560, 575-630, 633-694, 699-768`).
- **Fails in isolation for the right reason:** proven at RED (handoff-T-red.md:68-112: exit 127,
  empty fake logs, missing state, docs greps) and re-confirmed green by my runs.
- **Cheapest sufficient tier:** a bash orchestrator has no cheaper tier than a fake-script
  integration harness; the AI client gets its own focused argv/envelope harness. No tier
  re-proves another (a-dup check 5: no assertion overlap between the two).
- **Behavior, not implementation:** assertions read exit codes, stdout, fake-tool argv logs,
  `state.json`, and gh state files. The only source greps are AC7's docs greps — which are the
  spec's own requirement, not implementation-coupling.
- **Proportionality:** 190 assertions over a 744-line orchestrator + 72-line client, covering 15
  phases, 4 blockers, 5 hard stops, 3 postures, lock/corrupt state — proportionate to this story.
- **Smells:** none found — no assertion-free or tautological cases (every scenario mixes positive
  and negative assertions, so no case can pass vacuously — T's "Decided" note, borne out in the
  source), no unreachable outcomes, no mock-shaped assertions beyond the deliberately pinned fake
  contract (Assumed 1-15), no snapshot-everything, no padding.
- **Delete/merge findings:** none.

## Quality audit

| Area | Result | Notes |
|------|--------|-------|
| API consistency | PASS | CLI exactly the amended contract `vX.Y.Z [--dry-run|--publish] [--smoke] [--smoke-done] [--quit-kai]` (`release.sh:8,32-45`); phase names exactly R10's 15 (`release.sh:115`). |
| Error handling | PASS | `do_stop` is loud: message + log tail + resume command + macOS notification + marker-tagged checklist comment + exit 1 (`release.sh:143-153`); AI fail-closed on every no-answer shape (`release-ask-ai.sh:30-37,60-69`). |
| UI/UX match to spec | N/A | No UI surface (declared). |
| Accessibility | N/A | No UI surface (declared). |
| Architecture gate | PASS | A PASS + A-dup PASS; wrap-never-fork upheld — my own `git diff --name-status origin/master..HEAD -- scripts/` shows only the four adds (R12: zero wrapped scripts modified). |
| Code organization | PASS | 15 single-purpose phase functions mirroring R10's table; one AI seam; state helpers isolated (`release.sh:80-124`); readable throughout. |
| Security | PASS | The AI's answer is never executed or piped — parsed with jq and dispatched through a fixed `case` allowlist (`release.sh:271-313`); `run-handler` names validated (`:301-303`); quit-kai only with the flag (`:293`); tag/release immutability is structural (my grep: no `tag -d`/`--force`/`+refs/tags`/`release delete`); no secrets; `osascript` notification strings built from the regex-validated tag and fixed literals (`:126-130`). |
| Performance | PASS | No unbounded loops: handler attempts ≤ 2, consultations ≤ 2/phase, AI `wait` bounded at one 60 s pause (`release.sh:285-289`); everything else straight-line. |
| Visual regression | N/A | No UI surface (declared). |
| Naming consistency | PASS | State schema and phase keys byte-match T's Assumed #2; posture names match R6/R8. |
| Test quality (rubric above) | PASS | No delete/merge findings. |

## Scope check

**Neither exceeded nor under-delivered.** The diff (`git diff --stat origin/master..HEAD`) is 20
files: the two new scripts, T's two harnesses, `docs/releasing.md` + `docs/release-checklist.md`
(AC7), one CHANGELOG line, `.aftersight/pipeline.config.json`, and the run's handoff documents.
`frontend/package.json` is correctly absent — the env fix was split out as #98/PR #99 and merged
as `fe7ca50`, which is this branch's merge base (verified: `git merge-base HEAD origin/master` =
`fe7ca50`; `git diff origin/master..HEAD -- frontend/package.json` is empty), exactly as the diff
gate's round-2 PASS records. The `.aftersight` nulls (`pluginSuite`/`pluginTypecheck`) are beyond
the brief's "only change" sentence but are operator-made, journaled twice (decisions.md "T-green
rework 1/2"), disclosed in the handoff-F addendum, and carried through the diff gate — authorized
by the journal. The live rehearsal is de-scoped to post-merge by R2/A1 + the decisions.md Phase 1
entry — the sanctioned amendment chain, not silent under-delivery.

## Ruling on A-dup W-1..W-3 (de-scoping inside a ruling's letter)

Each is **authorized by the amendment chain in substance**; what is outstanding is the journaling
A-dup asked O to do (finding 1 below), not code:

- **W-1 (R4 `propose-fix-pr` executor half → record + stop):** authorized. T's binding contract
  never steered `propose-fix-pr` (no scenario in handoff-T-red's tables); R4's own first principle
  — "Nothing the AI produces is ever executed or piped anywhere" (`release-ask-ai.sh:10-11`
  restates it) — contradicts an executor that applies AI-proposed patches, since the answer schema
  `{diagnosis,action,argument,reason}` carries no patch; the implemented static path
  (`release.sh:306-309`: comment + stop with instructions) satisfies every AC3 pin
  (out-of-allowlist refusal is what the ACs test, and it is green).
- **W-2 (R10 [W] wait → stop-or-single-60s-pause):** authorized. T explicitly hedged the class
  (handoff-T-red.md §Hedged: "[W] wait/poll class not exercised … every steerable wait
  precondition has a green-or-stop sibling"), so the binding RED contract never pinned a poll
  loop; the safety property (never proceed without green evidence) holds — pending CI is a stop
  with a resume command (`release.sh:517-519`), and R6's stop-with-resume is the documented
  recovery. Ergonomics cost is real but lands on the rehearsal watch-list (item 2 below).
- **W-3 (R11 search-before-post → marker + state-tracked posting):** authorized. The pinned
  observable — no duplicate comment first-lines across kill/resume — is asserted
  (`release.test.sh:475-478`, green); the mechanism is an implementation detail, and the residual
  (re-post after a state-file deletion, R6's own documented recovery) is cosmetic.

## Findings

All non-blocking (advisory); none warrants REWORK — every AC is green against real evidence:

1. **A-dup's journaling condition is outstanding.** A-dup PASSed W-1/W-2 conditional on "O amends
   the ruling … or opens follow-up work; either way journal it" (handoff-A-dup.md:28-29), but
   decisions.md has no post-a-dup entry recording the R4/R10[W]/R11 reconciliation (the diff-gate
   round-2 hardening list it records is the outside review's 6 WARN + 3 NIT). Suggested one-liner
   for O at merge: *"Post-a-dup reconciliation: R4 propose-fix-pr = record+stop (executor never
   applies AI-proposed patches), R10 [W] = stop-or-single-bounded-pause (resume is the recovery),
   R11 = marker+state-tracked posting (observable unchanged) — accepted as the implemented
   semantics; poll-loop and executor-side draft-PR stay on the hardening list."*
2. **`handoff-A-dup.md` is untracked** (`git status`: `?? docs/handoffs/0078-issue-78/handoff-A-dup.md`).
   Commit it together with this handoff so the run record is complete on the branch.
3. **Smoke-state edge (rehearsal watch):** after `--smoke` records `awaiting-owner`
   (`release.sh:618-627`), a re-run *without* `--smoke`/`--smoke-done` records `skipped` and marks
   the phase done (`:628-631`). The documented resume path (`--smoke-done`) avoids it, but a real
   owner re-running bare `--publish` mid-matrix would silently convert awaiting→skipped.
4. **Hardening-list concurrence:** I independently confirm the six already-listed post-merge items
   (shared handler counter `release.sh:243-252`; `--dry-run --smoke` silently ignored — smoke only
   evaluates after the dry-run exit at `:716-728`; the unescaped ERE in
   `release-ask-ai.sh:49`; `$vdir` cleanup on stop paths in `ph_verify`; the `sed -n '2,/^set -uo/p'`
   help slice `release.sh:39`; `set -- $info` positional reuse). All cosmetic-to-minor; none
   touches an AC.

## Not verifiable here (the post-merge rehearsal list, R2/A1)

1. **AC1's live half:** a real `scripts/release.sh vX.Y.Z --dry-run` on a clean master, on the
   release Mac. Evidence will come from the owner's rehearsal, recorded on issue #78.
2. **Issue acceptance item 4 / AC4's live half:** a full rehearsal release (throwaway tag) with
   `--publish`/`--smoke` unset/set as documented.
3. **Consultation availability, live:** `opencode run` (500s in this checkout per W-A3/R9) plus
   GNU `timeout` present on the release Mac (brew coreutils; macOS ships none) — otherwise every
   consultation is honestly fail-closed (by design, but then the AI path is unproven).
4. **Real-gh shapes beyond the fakes':** `mergeCommit.oid` on a real merged PR, JSON (not TSV)
   check-runs, the real `pr-agent-review-state:v1` block parse.
5. **Real blocker paths:** #74 group-skip against a genuinely merged leftover branch; the
   bindings-drift handler against real drift (#73).
6. **[W] ergonomics in vivo:** a real release PR whose CI is still running stops (not polls) at
   review; the rehearsal should expect one stop+resume cycle there.

## Verdict

**Verdict:** PASS

All seven acceptance criteria (as amended by A1–A10 and the journaled operator rulings) are
satisfied against evidence I produced myself: both harnesses green in independent runs, the full
amended suite command green end-to-end in one chain, R12's zero-wrapped-modifications verified
against the diff, the four known blockers / consultation counting / allowlist refusal / five hard
stops / tag safety all pinned by tests that pass, and the docs story consistent. The A-dup
de-scopings (W-1..W-3) are authorized by the amendment chain in substance. The live rehearsal
items are de-scoped by the sanctioned R2/A1 ruling and enumerated above for the owner. Ready for
O to commit (folding in findings 1-2, which are O-side documentation actions).
