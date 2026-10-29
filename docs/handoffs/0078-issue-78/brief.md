# Brief — issue #78: a static release workflow runs the whole release; the AI is consulted only when a phase is blocked

Run `0078-issue-78` · worktree `.worktrees/0078-issue-78` · branch `issue-0078-issue-78` ·
base `master` of `Performant-Labs/kai`.
Survey: `docs/handoffs/0078-issue-78/survey.md` (corpus, reuse map, test-wiring finding).
Journal: `docs/handoffs/0078-issue-78/decisions.md`.

**Review-rigor dial: `second-opinion`.** Brief gate and diff gate each take one outside-model
review (`dual-review.sh`, default model, repo exclusions per AGENTS.md); round cap 2.

## Objective

Replace the agent-driven 29-step release checklist with a **static release workflow**,
`scripts/release.sh vX.Y.Z [--publish] [--smoke] [--quit-kai] [--dry-run]`, run on the release
Mac: every phase in a fixed order with fixed rules and **no questions**; state in
`bin/release/vX.Y.Z/state.json` (finished phases + the commit each ran on; postcondition-first
resume; re-running a finished phase is a no-op). An AI is consulted **only** when a phase is
blocked, via `scripts/release-ask-ai.sh`, with a bounded allowlisted job; anything it cannot
resolve stops the run and tells the owner exactly what is needed. Governing rules that the
workflow encodes (do not restate them in code comments — link): `docs/releasing.md`,
`docs/release-checklist.md`.

## Non-goals (explicit)

- No change to what a release **is**: same tag rules, same 4 assets, same signing, same
  changelog structure. The workflow orchestrates; it does not redefine.
- No hosted-runner release path (`release-kai.yml` stays as is; SDK gap unchanged).
- No wiring of `scripts/*.test.sh` into GitHub CI (separate decision; survey explains).
- No upstream (`dtapps/kai`) involvement of any kind.
- The **live rehearsal release** (throwaway tag, real repo) is post-merge, owner-driven; this run
  delivers everything it exercises, plus the fake-script tests that stand in for it here.

## Settled by this brief (from the issue + survey)

1. **Wrap, never fork** (reuse map): `release.sh` calls `release-preflight.sh`,
   `changelog-check.sh`, `release-bump.sh`, `release-gates.sh`, `release-local.sh`,
   `release-notes.sh`, `install-release.sh`, `gh` — one script one job. If a postcondition is not
   observable from outside a wrapped script, extend that script minimally (a flag/query), never
   duplicate its logic.
2. **Flags settle once, in config/flags, never questions**: `--publish` (default posture stops at
   the verified draft; publish + post-release verification only with it), `--smoke` (matrix runs
   only with it; otherwise recorded as skipped in the checklist issue), `--quit-kai` (Kai quit
   only when given), `--dry-run` (to the draft, nothing public). No interactive prompts on the
   happy path (the wrapped `release-local.sh` draft-ask is satisfied with `--yes` — publishing
   authority comes from `--publish`, not from a terminal question).
3. **Needs-the-owner list, printed before anything starts** (static checks): signing identity in
   the keychain **and a backup marker exists**, Kai closed (or `--quit-kai`), `gh` push rights,
   the codesign key-access prompt allowed once. Late surprises are a defect.
4. **Known-blocker table, static handlers, no AI**: at least the four from 0.5.0 — Kai running
   (quit via `--quit-kai` only), leftover merged `release/vX.Y.Z` (delete-and-continue, the #74
   rule: merged tip = leftover; unmerged = release in progress, stop), bindings drift (regenerate
   + open a PR — the #73 gate), missing signing identity (stop; `release-cert.sh` hint).
5. **Unknown failure → exactly one AI consultation per round, ≤ 2 per phase**:
   `release-ask-ai.sh` sends a fixed prompt (phase, command, last 200 log lines, that phase's
   checklist text) with read-only tools and must answer JSON
   `{diagnosis, action, argument, reason}`; `action` ∈ `retry` | `wait` | `run-handler <name>` |
   `propose-fix-pr` | `stop`. The executor **validates against the allowlist and runs only
   allowlisted actions**; anything else (invalid JSON, out-of-allowlist, missing AI) is refused
   and takes the stop path. Mechanism: recommended `opencode run` (A settles; absent binary ⇒
   consultation unavailable ⇒ stop path).
6. **Hard stops no AI can override**: signing identity missing; tag or release (draft included)
   already exists; CI red on the release commit; PR-Agent < 90 on the release PR with unresolved
   findings; anything that would delete or move a **published** tag.
7. **Stop = loud**: macOS notification + comment on the release checklist issue (phase, log tail,
   AI diagnosis if any, single resume command) + non-zero exit.
8. **Checklist issue lifecycle**: the workflow opens `Release checklist: vX.Y.Z` from
   `docs/release-checklist.md` early, reports smoke-skipped/blocked phases into it, and closes it
   with evidence when done — replacing the hand-ticked copy.
9. **Test wiring** (survey finding): new harnesses `scripts/release.test.sh` and
   `scripts/release-ask-ai.test.sh` follow the `release-preflight.test.sh` harness shape
   (`mkrepo`/`run`/`has`, throwaway repos, no network, fake wrapped scripts on `PATH`);
   `.aftersight/pipeline.config.json`'s `test.unit.command` is amended to run them so RED/GREEN
   are observable. This is the only `.aftersight/` change in the diff.

## Acceptance criteria

- [ ] AC1 `scripts/release.sh v0.5.1 --dry-run` on a clean `master` reaches a verified, packaged
  draft posture with **no input**, printing exactly what (if anything) is left for the owner.
- [ ] AC2 Killing it at any phase and re-running continues where it stopped (postcondition-first);
  running it again after full success changes nothing (no-op, exit 0).
- [ ] AC3 Each known blocker (all four), forced in a test, is handled **without** the AI; an
  unknown failure triggers **exactly one** consultation; an out-of-allowlist answer is refused
  (executor never runs it) and leads to the stop path.
- [ ] AC4 `--publish`/`--smoke` behave as settled above (draft-default; smoke skipped is recorded
  in the checklist issue); rehearsed end-to-end via the fake-script harness (live rehearsal is
  post-merge, owner-driven — recorded on the issue, a precondition for trusting the workflow).
- [ ] AC5 Both new harnesses pass from a clean worktree and are part of
  `test.unit.command`; the pre-existing `scripts/*.test.sh` stay green (nothing wrapped broke).
- [ ] AC6 The workflow never prompts on the happy path, never deletes/overwrites a release or
  published tag, and encodes the hard-stop list verbatim (tests pin at least: existing release ⇒
  stop; red CI ⇒ stop; each hard stop refuses even with an AI answer saying retry).
- [ ] AC7 Docs tell the same story: `docs/releasing.md` and `docs/release-checklist.md` point at
  `scripts/release.sh` as the operator, with the manual remainder (smoke, publish go-ahead) said
  once, in one place. CHANGELOG: `<!-- changelog-skip: release tooling, no user-facing change:
  #78 -->` (the #74/#75 precedent).

## Phase plan (this run)

D **N/A** (no UI surface — declared, not silent) → A (up-front plan review) → T-red → F →
T-green → A-dup → U **N/A** → S → merge. Rigor gates: brief (this file) and diff, each one
outside-model review under the repo's exclusion set.

## Handoffs (this run)

`docs/handoffs/0078-issue-78/handoff-{A,T-red,F,T-green,A-dup,S}.md` + `decisions.md` (append per
phase). Verbatim-evidence rule (AGENTS.md) applies to every claim that characterizes behavior.

## For A to settle (bounded)

Survey "Risks/open questions": canonical phase list + postconditions; PR-Agent > 90 evaluation
rule; `release-ask-ai.sh` invocation mechanism; `--smoke` pause/confirm semantics; state-file
schema. A may amend this brief; amendments land as decisions.md entries, then A re-reviews.
