# AGENTS.md — kai (OpenCode)

Agent instructions for OpenCode in this repo. (Claude Code reads `CLAUDE.md` instead; this file
is the OpenCode equivalent and is what the Aftersight loader's `pipeline_*` tools drive.)

## Project docs

Read before proposing work:

- `README_EN.md` — project overview, layout and setup (the repo's English README).
- `docs/PLAN.md` — the plan behind the app; specs for this repo live in the GitHub issues themselves.
- `Makefile` / `Taskfile.yml` — the build order (Swift bridge → frontend → Go backend; test targets).

Per-story handoffs live in `docs/handoffs/` (run state, briefs, reviews). A decision belongs in
exactly one place: link to it, do not restate it.

## Ground rules

- All work happens in a per-issue worktree under `.worktrees/` (branch `issue-NNNN-<slug>`),
  never in the primary checkout.
- Issues and PRs go to `Performant-Labs/kai` (the public home), never to the upstream
  `dtapps/kai`. Always pass `--repo Performant-Labs/kai --base master` to `gh pr create`:
  this repository began as a fork, and the default target of a fork is its parent. The old
  private repository `Performant-Labs/kai-private` is archived; its issue and PR numbers
  (`#N`) in older comments refer to it.
- Upstream `dtapps/kai` is touched **only** for upstream contributions, and only when an
  issue explicitly asks for one.
- Upstream contribution recipe (only when an issue asks for it): branch off
  `upstream/master` containing **only** the intended change, and **never** `.aftersight/`, `.opencode/`,
  `AGENTS.md`, `opencode.json`, `docs/handoffs/`, or `.github/workflows/ci.yml`; open the PR
  with `gh pr create --repo dtapps/kai --head Performant-Labs:<branch>`.

## Coding pipeline (Aftersight)

This project runs its stories with an Aftersight coding pipeline through the loader in your client
configuration; nothing of it is in this repository beyond the committed seeds (`.aftersight/`).
Before you set it up or run a story, read the runbook and follow it. The repository is private, so
fetch it with your `gh` login:
`gh api repos/Performant-Labs/aftersight/contents/docs/runbooks/pipeline-for-agents.md --jq .content | base64 -d`

- Server: `https://localhost:3443` (project `kai`, tenant `dev`); the operator sets
  `AFTERSIGHT_API_TOKEN`, you never do. Provider keys live in the operator's environment file
  (`~/.aftersight-selfhost/env-kai`, mode 0600) — source it when you start a run.
- Run a story only when the operator names the issue and the rigor (`direct` / `in-session` /
  `second-opinion` / `panel`).
- Never print or store a token or key; never edit gate settings or credentials, force, or move a tag.
- Check `Closes` or `Refs` in the body of a pipeline pull request.
- A failing check is a stop: report the row and message, and ask.

**The release owns the process, not the model.** Phase order (pre-flight → survey-brief → design →
architecture-review → t-red → implement → t-green → a-dup → ui-walkthrough → spec-audit → merge),
RED/GREEN ordering, rigor gates, and the named stops are code in the fetched release. You (the
model) may not skip, reorder, or waive a phase. On any disagreement between your judgment and the
release's report, the release wins.

### Pre-flight is a hard gate

`pipeline_preflight` (a loader tool; call it first, with the issue number and the rigor) returns one
structured payload with **every** check result — do not run the checks yourself.

- **Every check runs.** One failing row never aborts the rest; the payload lists all of them.
- **Pre-flight never runs the product suite** (`bash scripts/test-unit.sh`, named by
  `.aftersight/project.json`) and never fires a commit hook. It checks wiring and reachability
  only. If the gate model is down, pre-flight **fails closed** — no silent fallback.
- A `PRE-FLIGHT: FAIL` payload is a **hard stop**: do not begin survey-brief until each failing
  row is resolved or waived by a human.

### Configuration

The pipeline's project-specific configuration lives in the committed seeds (`.aftersight/project.json`,
`.aftersight/opencode.json`, `.aftersight/claude.json`) and on the server (tenant `dev`); change the
committed files, commit them, then `aftersight config push`. Roles, models and the gate come from the
seeds and the release — there are no in-repo role agents and no `.opencode/` pipeline plugin anymore.
Both seeds pin a release (`opencode-pipeline-v2.2.0`, `claude-pipeline-v0.4.0`) rather than `latest`,
so a release changes only on purpose: move the tag in the seed and push. Stage models run on
`zai/glm-5.3` (OpenCode) and Claude models (Claude); the outside dual-review gate runs on DeepSeek
(`deepseek` binding, key variable `DEEPSEEK_API_KEY`).

Seed rules the releases enforce (`pipeline-upgrading-and-pinning.md` in the Aftersight runbooks):

- A suite states its commands as `script`, a list of shell lines; the key `command` is no longer read.
- `pathSets.tests` and `pathSets.production` must not share a glob; `tests` names test files only.
- A stage's `effort` must be one its model offers. `zai/glm-5.3` offers `low`, `high` and `max` only
  (no `medium`, no `xhigh`); check the `agents.<role>` efforts by hand too.
- On opencode-pipeline 2.2.0 two workarounds are in the seeds, both harmless on 2.2.1 and later:
  `gate.baseBranches` is `master` (the repository's default branch, which 2.2.0 does not detect), and
  `pathSets.handoffs` also names `.worktrees/*/docs/handoffs/**` so the orchestrator may write a run's
  handoffs in its worktree. Drop the second once the seed moves to 2.2.1.

### Dual-review gate policy (flotilla#457 lessons, kai-private#30)

- **Gate diff is narrowed by default.** `DUAL_REVIEW_EXCLUDE="frontend/bindings package-lock.json
  docs"` in the operator's environment file keeps generated artifacts and docs out of the review diff (chat-transport gates
  refuse above `DUAL_REVIEW_MAX_PROMPT_TOKENS` = 32000). Full diff is an explicit opt-in.
- **Round cap: 2.** A BLOCK surviving the round-2 re-review is recorded (findings verbatim) and the
  run **stops**; continuing requires an explicit monitor/principal ruling recorded in
  `docs/handoffs/decisions.md`. Disclosure, not pass.
- **Verbatim evidence.** Any bead comment, brief, or journal that quotes or characterizes source
  behavior must carry a verbatim excerpt (`sed -n 'N,Mp' <file>`) with file:line — never a retyped
  quote. Claims without excerpts are treated as unverified. (Born from the #8 run: a paraphrased
  "isolation probe" re-confirmed its own wrong assumption and cost a full t-green round-trip.)

### Worktree-per-run

Every pipeline run — even a single-shot `direct` change — uses an isolated git worktree, never the
primary checkout's branch. The pipeline provisions it during pre-flight: `.worktrees/NNNN-<slug>`
on branch `issue-NNNN-<slug>` (`NNNN` = the zero-padded issue number), with the port and any
throwaway database name derived deterministically from the issue number. Prune orphaned worktrees
(merged branch, closed story) before starting a new one — pre-flight flags orphans.

### UI-gated phases

The `design` and `ui-walkthrough` phases apply only when the story has a UI surface
(`uiSurface: true`). When there is no UI surface they are **skipped and declared** — never silently
omitted.

## Verification

The authoritative suite is the unit suite named by `.aftersight/project.json`:
`bash scripts/test-unit.sh` (Swift bridge build → Go tests → frontend tests → release-script tests). It runs in the
**t-red** and **t-green** phases (the release reads its exit code), never in pre-flight.
