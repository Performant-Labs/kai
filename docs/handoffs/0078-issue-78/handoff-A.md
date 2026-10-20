# Handoff-A: Phase 3 — Up-front plan review (issue #78: static release workflow)

**Date:** 2026-10-19
**Branch:** `issue-0078-issue-78` (worktree = `master` @ `15d323a`)
**Brief reviewed:** `docs/handoffs/0078-issue-78/brief.md` · **Reuse map:** `docs/handoffs/0078-issue-78/survey.md` §"Reuse & Analogous-Feature map" · **Wireframe:** N/A (uiSurface false, declared) · **Outside brief review:** `docs/handoffs/0078-issue-78/dual-review-brief.md` (0 BLOCK / 6 WARN / 7 NV / 2 NIT)
**Verdict:** PASS

## Summary

The plan extends the architecture the Reuse map names: `release.sh` is a new orchestrator that
**wraps** `release-preflight.sh`, `changelog-check.sh`, `release-bump.sh`, `release-gates.sh`,
`release-local.sh`, `release-notes.sh` (via `release-local.sh`), `install-release.sh` and `gh`,
and creates no parallel path — the map names `release-local.sh`'s step-function shape as the
analogue to extend **by wrapping**, and the brief follows it. Every "settled" claim I could check
against source holds: `release-local.sh --yes` satisfies the draft ask, the #74 merged-tip rule
matches `release-preflight.sh` verbatim, `test.unit.command` carries the new harnesses, and the
`changelog-skip` token is real. All postconditions in my canonical phase list are observable
from outside the wrapped scripts — **zero wrapped-script modifications are required**. The six
WARNs and the brief's "For A to settle" list are settled below with binding rulings; ten
amendments to the brief are listed explicitly and journaled in `decisions.md`. No block findings:
the ambiguities the outside review flagged are exactly the ones this handoff exists to pin.

## Findings — Block

None.

## Findings — Warn

Each Warn below is *settled* by a ruling in the next section; none blocks the plan.

| # | Severity | Plan element | Finding (with evidence) | Settled by |
|---|---|---|---|---|
| W-A1 | warn | brief.md:42-45 `--dry-run` / default / `--publish` postures | Three postures are named but their endpoints are not pinned, and the issue's "dry-run = everything up to the draft, nothing public" reads two ways. `release-local.sh` itself has exactly two modes — `sed -n '146,149p' scripts/release-local.sh`: `if [[ $dry -eq 1 ]]; then printf '\nDry run done: no release created. Files are in %s\n' "$out"; exit 0; fi` — and the draft ask is `sed -n '152,156p'`: `if [[ $yes -ne 1 ]]; then [[ -t 0 ]] \|\| die "no terminal to ask on: pass --yes to create the draft"` — so the workflow has one consistent mapping, but the brief must state it (Ruling R8, amendment A1). | R8/A1 |
| W-A2 | warn | brief.md:69-73 item 9 — "fake wrapped scripts on `PATH`" | The repo convention for sibling calls is `"$here/..."`: `sed -n '30,30p' scripts/release-local.sh`: `here="$(cd "$(dirname "$0")" && pwd)"` and `sed -n '97,97p'`: `export EXCLUDE_CHECK="Build, verify, package, draft"; if [[ $dry -eq 1 ]]; then "$here/release-gates.sh" ...`. If `release.sh` follows that convention, a fake on `PATH` **cannot intercept** and the harness in AC4/AC5 runs real network/builds. The brief implies PATH dispatch but never states it as a constraint. | R7 |
| W-A3 | warn | brief.md:59-60 item 5 — mechanism `opencode run` | Verified present and non-interactive (`opencode v2.0.29`; `opencode run --help` shows `[<message...>]` positional and `--format choice (default, json)`), **but** a live probe in this checkout returned a structured failure: `timeout 90 opencode run --standalone --format json "Reply with the single word: ok"` → stdout `{"type":"error","timestamp":1792454525968,"sessionID":"","error":{"type":"unknown","message":"UnexpectedStatus: 500"}}`, `EXIT=1`. The mechanism fails *detectably*, which is what the contract needs — but consultation availability on the release Mac is proven only in the post-merge rehearsal, and the tests must fake the binary. | R9 |
| W-A4 | warn | Phase plan vs. "no questions" | Two phases need human input the brief doesn't place: changelog-entry authorship and Known-Issues curation. `docs/releasing.md:286-288` forbids machine-written entries: "Do not write an entry from a title alone. 'Implements #NN' pipeline PRs say nothing about what changed; read the issue's acceptance section and the diff" — so an AI consultation must never author changelog prose. These phases need an explicit stop-for-owner gate, not silence and not AI. | R10 (phases 2, 5) |
| W-A5 | warn | brief.md:53-60 item 5 vs AC3 (brief.md:82-83) | Consultation-count ambiguity (outside review W-3) and mutation authority (outside W-6) both live here; tests will pin the wrong predicate unless the counting rule and the executor-performs-mutations principle are canonical. | R4, R5 |
| W-A6 | warn | brief.md:16-17 state file | No corruption/partial-write/concurrency story (outside review W-5). | R6 |

## Findings — Nit

| # | Severity | Location | Finding | Settled by |
|---|---|---|---|---|
| N-A1 | nit | brief.md:80 AC2 "killing it at any phase" | Should read "at any phase boundary" — postcondition-first cannot resume a half-completed side effect inside a wrapped script; a mid-phase kill re-runs that phase from its postcondition probe. | A8 |
| N-A2 | nit | brief.md:47 item 3 "codesign key-access prompt allowed once" | OS-level prompt, not a workflow prompt; docs must say so once (AC7's "said once, in one place"). | A9 |
| N-A3 | nit | Phase 6 (release PR) | The release PR itself will appear uncovered in the **next** release's `changelog-check` unless a skip line is added — chicken-and-egg (the PR number doesn't exist when its own diff is written). Precedent: the skip lines were added later, e.g. `CHANGELOG.md:18`: `<!-- changelog-skip: release PR plumbing, no user-facing change: #45 -->`. Ruling: documented behavior, not code — phase 2's stop message prints the exact skip line to add next release. No code change. | R10 |
| N-A4 | nit | Phase "closeout" / item 8 | `gh issue comment` prompts interactively without `--body`: `gh issue comment --help`: "Without the body text supplied through flags, the command will interactively prompt for the comment text." Every checklist-issue write must pass `--body`/`--body-file` (and `gh issue create --title --body-file`). | R11 |

## Verification of the brief's "settled" claims (with evidence)

- **Item 2 — `--yes` satisfies the draft ask.** `sed -n '34,35p' scripts/release-local.sh`: `--dry-run) dry=1 ;; --yes) yes=1 ;;`; with `yes=1` the ask block at 152-156 is skipped entirely and the draft is created at 157-158. **TRUE.**
- **Item 4 — the #74 rule.** `sed -n '213,219p' scripts/release-preflight.sh`: `rb="$(git ls-remote --heads origin "refs/heads/release/$version" ...)"; if [[ -n "$rb" ]]; then if git merge-base --is-ancestor "$rb" origin/master 2>/dev/null; then leftover="release/$version"; else taken+=("branch release/$version (not merged into master)")` — merged tip ⇒ leftover (WARN + delete hint at 228-231); unmerged ⇒ FAIL. **TRUE** (also closes NV-7).
- **Item 9 / NV-5 — `test.unit.command` can carry the harnesses.** `.aftersight/pipeline.config.json:7-12` is a single `sh -c` string: `"(cd pkg/swiftbridge/scripts && bash ./build.sh) && go test -vet=off ./internal/... ./pkg/... -count=1 && pnpm --dir frontend test"` with `timeoutMs: 900000`; appending `&& bash scripts/release.test.sh && bash scripts/release-ask-ai.test.sh` changes no other assumption (configShapeVersion untouched; the new harnesses are fake-script, no-network, seconds-fast). **TRUE.**
- **NV-1 — wrapped scripts are non-interactive and postconditions observable.** `release-local.sh` flags parsed at 32-43 (accepts `--yes`, `--dry-run`, `--out DIR`); draft-with-4-assets observable via `gh release view vX.Y.Z --json isDraft,assets` (drafts are listable: `release-gates.sh:61` queries `repos/$repo/releases` and `select(.tag_name == ...)` catches drafts "included", per its header comment lines 12-13). Packaged-draft-staged observable on disk under `bin/release/vX.Y.Z/` (release-local.sh:111,141-144). No new flags needed anywhere. **TRUE.**
- **NV-2 — `opencode run`.** Exists, v2.0.29, single-shot, `--format json`; live call currently 500s in this checkout context (see W-A3) — acceptable **because** the contract treats non-zero/unparseable as no-answer → stop path, and tests fake the binary. **TRUE with Ruling R9.**
- **NV-3 — `changelog-skip` token.** Mechanism: `sed -n '70,70p' scripts/changelog-check.sh`: `cited="$(printf '%s\n' "$entries" | { grep -o '#[0-9]\+' \|\| true; } ...)"` — every `#N` inside `[Unreleased]` (outside Known Issues), *including numbers inside skip comments*, counts as cited; documented at `changelog-check.sh:15-16` and `docs/releasing.md:284`. Precedent verbatim: `CHANGELOG.md:9`: `<!-- changelog-skip: release tooling, no user-facing change: #74 -->` — the brief's AC7 line copies this shape with `#78`. **TRUE.**
- **NV-4 — harness shape generalizes.** `release-preflight.test.sh:19-31` defines `mkrepo/run/has/hasnt/exit_is`; 116-132 a fake `gh` with env-steered canned answers; 158-170 a fake `git` wrapping the real one (`if [[ "$1" == ls-remote ]]; then ... exec "$real_git" "$@"`) — that wrapper pattern is how `release.test.sh` intercepts `push`/`fetch`/`ls-remote` while keeping real git for everything else; env-steered fakes (`FAKE_RELEASE_LOCAL=fail-once` style) give kill/fail-at-phase + resume. **TRUE.**
- **NV-6 — `gh` issue semantics.** `gh issue create` (`--title`, `--body-file`), `gh issue comment <N> --body`, `gh issue close <N>` all exist (help output read). Interactive-prompt trap noted as N-A4. **TRUE.**
- **Repo gates respected.** PRs to `Performant-Labs/kai` with `--repo … --base master` (AGENTS.md ground rules — encoded in Ruling R10 phase 6); tag truth, never delete/overwrite a release, package once (docs/releasing.md) — encoded in the phase list and hard stops; `.aftersight/` diff = `pipeline.config.json` test-command amendment only; no upstream involvement.

## Canonical rulings (binding for T and F)

**R1 — PR-Agent evaluation rule (W-1).** The gate on the release PR passes **iff**, for the
current PR head: (a) every check-run on that head is `completed` with conclusion
`success|skipped|neutral` (same predicate as `release-gates.sh:49`), and (b) the latest PR-Agent
review on the PR is complete and current — parse the hidden machine block in the PR-Agent
comment: it exists verbatim in this repo (PR #88), starts `<!-- pr-agent-review-state:v1` and
carries JSON with `findings[].state`, `last_run.complete`, `last_run.head_sha`, plus the HTML
score line `<td>🏅&nbsp;<strong>Score</strong>: 90</td>` — and (c) **score ≥ 90** and (d)
**zero findings with `state != "RESOLVED"`**. The hard-stop predicate is the negation:
`score < 90` **OR** any unresolved finding **OR** review incomplete/stale for the head (that
last one is *wait*, not stop: poll). Boundary: **90 passes** (`score < 90` stops) — repo
precedent: PR #88 merged with Score exactly 90 and all findings RESOLVED. Failing either (c) or
(d) alone is a hard stop (releasing.md:50 states the pass condition as a conjunction). This
replaces both brief wordings ("PR-Agent < 90 … with unresolved findings" and "> 90").

**R2 — AC1 verifiable via the fake-script harness (W-2).** AC1 is satisfied **in this run** by
`release.test.sh`: `mkrepo` throwaway repo, fake `gh`/`git`/wrapped scripts (env-steered) on
`PATH`, run `release.sh v0.5.1 --dry-run </dev/null`, assert: exit 0, no prompt consumption,
`bin/release/v0.5.1/` holds `RELEASE_NOTES.md` + the 4 staged asset names + `Kai.app` (fake
`release-local.sh --dry-run` creates them), `state.json` records phases done through `local`,
and stdout names exactly the remaining owner steps (create draft / smoke / publish). The live
`--dry-run` on a real clean master is the owner's post-merge rehearsal, recorded on the issue.

**R3 — Consultation counting (W-3).** Consultations are **one per unknown-failure event**, never
batched: the first unknown failure of a phase triggers **exactly one** consultation (AC3's
meaning); a second is permitted **only** if the first returned an allowlisted action, the
executor executed it, and the phase then failed again; a third failure in the same phase never
consults — straight to stop. Counter = append-only `consultations[]` in `state.json`, filtered
by phase; cap 2/phase. Tests pin all three cases.

**R4 — Executor performs mutations; the AI session is advisory-only (W-6).** Nothing the AI
produces is ever executed or piped anywhere; the executor maps each allowlisted action to a
**static bash code path it owns**: `retry` = re-run the phase command; `wait` = poll the phase
postcondition for a bounded window (default 10 min), then re-evaluate; `run-handler <name>` =
name must exist in the static known-blocker table (dispatch refused otherwise); `propose-fix-pr`
= executor creates a **new branch off `origin/master`** (never `release/vX.Y.Z`), applies the
proposed change only if it applies cleanly and touches no tag/release/`.aftersight` path, opens
a **draft PR**, then **stops** (the release resumes only after the owner merges it — this action
never unblocks by itself); `stop` = the stop path. Additionally: the executor checks the
hard-stop list **before** consulting — a hard-stop condition never consults, whatever its
surface error.

**R5 — Hard stops and their pinned tests (W-4).** Drop "verbatim" from AC6. The five hard stops
(brief item 6) each get one distinct stop test, plus one shared refusal test looping all five
with a canned AI answer `{action:"retry"}` (assert: no retry executed, stop path taken):
(1) identity missing — fake `codesign-app.sh --check` fails; output names `release-cert.sh`;
(2) tag or release already exists — fake `release-gates.sh`/`gh release view` reports existing
(draft and published variants); (3) CI red on the release commit — fake check-runs `failure`;
(4) PR-Agent — fake review-state JSON with score 88 (and separately score 95 + one `OPEN`
finding); (5) published-tag safety — negative assertion: the fake `git` records every arg; assert
no `tag -d`, no `push --force`, no non-fast-forward `refs/tags/*` write is ever issued by any
handler or action (the leftover-branch handler deletes only `refs/heads/release/*`).

**R6 — `state.json` write, lock, and posture discipline (W-5).** Location
`bin/release/vX.Y.Z/state.json` (already git-ignored: `sed -n '2,2p' .gitignore` = `bin`; jq is
a required tool — `release-local.sh:69`). **Atomic write:** jq → `state.json.tmp.$$` in the same
directory → `mv` (same-dir rename). **Lock:** `mkdir bin/release/vX.Y.Z/.lock` (atomic); a live
owner-PID inside refuses a second invocation ("another release.sh is running (PID N)"); a dead
PID's lock is removed and retaken. **Corrupt/absent state:** absent = fine, phases re-derive
from postconditions (state is a cache, never the source of truth); unparseable = stop, printing
the path and "delete it to rebuild from postconditions". **Posture escalation only:** postures
order `dry-run` ⊂ `default` ⊂ `publish`; a re-run at a *lower* posture than already recorded
(e.g. `--dry-run` after the draft exists) is refused; higher is fine. `--smoke`/`--quit-kai` are
per-invocation modifiers (the smoke *result* is recorded in state).

**R7 — Invocation dispatch (enables the whole harness).** `release.sh` invokes every wrapped
script by **bare name** (`release-preflight.sh vX.Y.Z`), and ensures resolvability itself:
prepend `$(dirname "$0")` to `PATH` **only for names not already resolvable** (`command -v`).
Production needs no setup; tests win by putting their fakes dir first on `PATH`. Do **not**
copy the `"$here/..."` sibling-call convention of the leaf scripts — it is un-fakeable and would
void AC4/AC5. (If F prefers an explicit override, an env var along the `KAI_RELEASE_REPO` /
`KAI_INSTALL_BUNDLE_ID` precedent is acceptable; the binding constraint is bare-name dispatch.)

**R8 — Postures (AMENDS brief item 2).** Exactly three, mapping onto `release-local.sh`'s own
two: `--dry-run` = the full phase order through `local`, calling `release-local.sh vX.Y.Z
--dry-run` — **no draft is created**; nothing is "public" in the release sense (branch/PR/merge/
tag still happen — they *are* the release; only the GitHub Release object is gated). Default =
same, calling `release-local.sh vX.Y.Z --yes` — **draft created** (non-public), run stops after
the draft (+ smoke gate if `--smoke`), printing the single `--publish` resume command.
`--publish` = continues: publish, verify-published, closeout. `--yes` is always passed in
default/publish postures (`release-local.sh:153` dies without a tty otherwise); publishing
authority is the flag, never a terminal question.

**R9 — `release-ask-ai.sh` mechanism (settles the survey's recommendation).** Invocation:
`timeout 300 opencode run --format json "<fixed prompt>"` (prompt = phase, command, last 200 log
lines, that phase's checklist text; message passed as a single argv). Parse the assistant text
(the last text part) out of the JSON with jq; **any** non-zero exit, timeout, or unparseable
output = no-answer ⇒ the stop path (fail-closed, exactly as an absent binary). Tests fake the
binary on `PATH` (canned JSON; also the invalid-JSON and exit-1 cases) — RED/GREEN never needs
a live model. Live availability is proven only in the owner's post-merge rehearsal. The
read-only guarantee is structural (R4), not prompt-based.

**R10 — Canonical phase list.** The table below is *the* phase list; resume = postcondition
first. Failure classes: **[H]** hard stop (no AI), **[K]** known blocker, static handler,
**[W]** wait (poll, known), **[S]** stop-for-owner (human input; no AI), **[A]** unknown → AI
consultation (R3/R4 rules).

| # | Phase | Wraps / does | Postcondition (resume probe) | Failure class |
|---|---|---|---|---|
| 0 | `needs` | Static needs-the-owner list, printed before anything: identity probe `KAI_SIGN_IDENTITY="Kai Release" codesign-app.sh --check` (the probe `release-local.sh:77` uses); backup marker file exists (`bin/release/kai-release-key.backed-up`, owner-created once, documented); `gh api repos/$repo --jq .permissions.push == true`; Kai not running, or `--quit-kai` given (then quit now, gracefully). Opens the checklist issue: `gh issue create --title "Release checklist: vX.Y.Z" --body-file docs/release-checklist.md`. | All probes pass now + issue exists (`gh issue list --search` by exact title, any state) | [H] identity missing, no push rights, Kai running without `--quit-kai` |
| 1 | `preflight` | Wraps `release-preflight.sh vX.Y.Z`; WARN rows → one marker-tagged checklist comment | Re-run exits 0 (pure check, idempotent) | [H] version taken (tag/release); [S] unmerged `release/vX.Y.Z` = release in progress |
| 2 | `changelog` | Wraps `changelog-check.sh --strict`; gathers known-issue candidates (`gh issue list --label bug --state open`, `--label known-issue --state open` + standing line) into the checklist issue; stops until `[Unreleased]` carries a Known Issues subsection (≥1 bullet, or the owner adds the printed `<!-- known-issues: none YYYY-MM-DD -->` marker) | `--strict` exits 0 AND Known-Issues curated | [S] uncovered PRs, uncurated list — changelog prose is human-authored (releasing.md:286-288); the stop message prints the entries/skip lines to add |
| 3 | `branch` | `git checkout -b release/vX.Y.Z <releaseCommit>` + push (release commit frozen in state at phase 1) | `ls-remote` shows `refs/heads/release/vX.Y.Z` at the release commit, **or** group-skip: branch tip merged into master ⇒ skip 3-6 and delete the leftover branch (the #74 rule) | [S] branch exists unmerged |
| 4 | `bump` | Wraps `release-bump.sh X.Y.Z` — **plain version, no `v`** (`release-bump.sh:18`) | Re-run idempotent; `git diff` shows only the 4 version lines (checklist step 11) | [A] |
| 5 | `restructure` | Mechanical move: `[Unreleased]` (Known Issues verbatim, phase 2 curated it) → `## [X.Y.Z] - YYYY-MM-DD`; fresh empty `## [Unreleased]` above | `release-check-version.sh X.Y.Z .` exits 0 AND `^## \[Unreleased\]` present (it already checks cfg+plist+CHANGELOG: `release-check-version.sh:9-11`) | [A] |
| 6 | `release-pr` | Commit, push, `gh pr create --repo Performant-Labs/kai --base master` (AGENTS.md: fork default targets the parent — never omit these flags) | `gh pr list --head release/vX.Y.Z` non-empty (or merged ⇒ group-skip) | [A] |
| 7 | `review` | Wait: CI green on the exact PR head + PR-Agent gate per **R1** | All head checks success ∧ review-state complete@head ∧ score ≥ 90 ∧ 0 unresolved | [W] checks/review incomplete; [H] CI red; [H] score<90 or unresolved findings (owner edits the PR; new head re-evaluates) |
| 8 | `merge` | `gh pr merge --merge --delete-branch`; wait CI green on the merge commit | PR `MERGED` ∧ merge-sha check-runs all success | [S] conflict; [H] red merge CI |
| 9 | `tag` | `git tag -a vX.Y.Z -m "vX.Y.Z"` on the merge commit + push | `ls-remote refs/tags/vX.Y.Z` == merge commit | [H] tag exists at any commit — never move; recovery by named owner action only |
| 10 | `local` | Wraps `release-local.sh vX.Y.Z --yes` (default/publish) or `--dry-run` (dry-run posture, R8) | default/publish: `gh release view vX.Y.Z --json isDraft,assets` → `isDraft` ∧ 4 assets; dry-run: `bin/release/vX.Y.Z/` holds notes + zip + updater-zip + dmg + `SHA256SUMS` + `Kai.app`. Note: re-running before anything is published may rebuild/repackage — allowed; "package once" binds after publish | [K] Kai running → handler `quit-kai`, dispatchable only with `--quit-kai`; [K] bindings drift (red `push.yml` bindings gate on the PR, or tree-check fail) → handler `regen-bindings` (`make bindings` + commit to the release PR — #73); [H] identity missing (`release-cert.sh` hint); [H] release/tag exists; [H] CI red; [A] else |
| 11 | `smoke` | Without `--smoke`: record `smoke: skipped` in state + checklist comment. With: print matrix instructions (docs/release-checklist.md §"Manual smoke matrix"), notify, comment, **exit 0** in `awaiting-owner`; resume with `--smoke-done` records `smoke: done (owner-asserted <date>)`. No terminal prompts; the matrix is human on the real Mac | state/checklist record ∈ {skipped, done} | owner gate (not a failure) |
| 12 | `publish` | `--publish` posture only: `gh release edit vX.Y.Z --repo $repo --draft=false` (the canonical command `release-local.sh:163` prints) | `isDraft == false` ∧ 4 assets | [H] anything unexpected — never delete/overwrite |
| 13 | `verify-published` | Checklist step 28 as code, in a clean temp dir: `gh release download`; `shasum -a 256 -c SHA256SUMS`; `cmp` zip vs updater zip; `ditto -x -k` + `release-verify.sh`; mount dmg + `diff -r`; `install-release.sh vX.Y.Z --dest "$(mktemp -d)" --keep-permissions` | The whole sequence exits 0 (pure verification, idempotent) | [H] any mismatch — published artifact wrong; owner decides |
| 14 | `closeout` | Link the checklist from the release, evidence comment, `gh issue close <N>` | Issue state `closed` | [S] |

**R11 — Checklist-issue writes are idempotent.** Every comment `release.sh` posts begins with a
stable marker `<!-- release.sh: vX.Y.Z <phase> <kind> -->`; search-before-post; always
`--body`/`--body-file` (N-A4). Tests assert no duplicate comments across kill/resume.

**R12 — Wrapped scripts: zero modifications.** Every postcondition above is observable through
existing flags/`gh`/disk. If F nevertheless needs one, brief item 1's minimal-extension rule
governs (a flag/query, never duplicated logic) and AC5's "nothing wrapped broke" re-runs the
existing `scripts/*.test.sh` once to prove it.

## Amendments to the brief (journaled in decisions.md)

- **A1 (item 2, AC1):** three postures pinned per R8; `--dry-run` still runs branch/PR/merge/tag
  (they are the release) and stops before draft creation; AC1's verification is the fake-script
  harness per R2 (live run = post-merge rehearsal).
- **A2 (item 5):** consultation counting per R3; executor-performs-mutations per R4; hard-stop
  check precedes consultation.
- **A3 (item 6, AC6):** "verbatim" dropped; the five pinned stop tests + shared AI-refusal loop
  + no-tag-mutation negative assertion per R5.
- **A4 (state file):** schema, atomic write, lock, escalation-only posture per R6.
- **A5 (new):** bare-name invocation dispatch per R7 (the enabling constraint for the harness).
- **A6 (item 3):** backup marker = `bin/release/kai-release-key.backed-up`, owner-created once,
  documented in `docs/releasing.md`; `--quit-kai` quits immediately at phase 0 when given.
- **A7 (new):** `release-ask-ai.sh` = `timeout 300 opencode run --format json`, fail-closed per
  R9; harness fakes the binary.
- **A8 (AC2):** "killing it at any phase **boundary**"; a mid-phase kill re-runs that phase from
  its postcondition probe.
- **A9 (item 3 / AC7):** one docs sentence: the macOS codesign key-access prompt is OS-level,
  not a workflow prompt.
- **A10 (new):** phase 2's stop message prints the exact `<!-- changelog-skip: … #N -->` line
  for the current release PR, to be added next release (N-A3, the #45/#33 precedent).

## Notes for O

No re-review required: this is a PASS with rulings T and F implement against directly. One
operational note, not a plan defect: the live `opencode run` call currently 500s in this
checkout context (W-A3) — fail-closed handles it, and the rehearsal will prove the real path;
if it still fails there, the answer is configuration on the release Mac, not a code change.

## Patterns referenced

1. `scripts/release-local.sh` — the wrap target: step functions, die-on-failure, gates before
   side effects, `--dry-run`/`--yes`, machine-check block (lines 27-86, 146-158).
2. `scripts/release-preflight.sh:209-231` — the #74 leftover-branch rule, verbatim.
3. `scripts/release-preflight.test.sh:19-31, 113-170` — the harness analogue (`mkrepo/run/has`,
   fake `gh`, fake-`git`-wrapping-real-`git`, no network).
4. `docs/releasing.md` §"Step by step" (349-385) and `docs/release-checklist.md` — the 29 steps
   the phase list encodes; tag truth (12-13), PR-Agent rule (48-53), package once (168-172).
5. `CHANGELOG.md:9-10` + `scripts/changelog-check.sh:70` — the changelog-skip token and its
   mechanism; `.aftersight/pipeline.config.json:7-12` — the test-command amendment point.
