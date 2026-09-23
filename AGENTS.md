# AGENTS.md — kai (OpenCode)

Agent instructions for OpenCode in this repo. (Claude Code reads `CLAUDE.md` instead; this file
is the OpenCode equivalent and is what the `/pipeline` command and the `pipeline_preflight`
plugin drive.)

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
- Issues and PRs go to `performant-labs/kai`, never to the upstream `dtapps/kai`.

## OpenCode coding pipeline

Implementation follows the OpenCode coding-pipeline plugin (`.opencode/plugins/`, installed from
the Playbook's `workflow/opencode/`), test-first, with a per-story review-rigor dial
(`direct` / `in-session` / `second-opinion` / `panel`).

**The plugin owns the process, not the model.** Phase order (pre-flight → survey-brief → design →
architecture-review → t-red → implement → t-green → a-dup → ui-walkthrough → spec-audit → merge),
RED/GREEN ordering, rigor gates, and the named stops are code in the plugin. You (the model) may not
skip, reorder, or waive a phase. On any disagreement between your judgment and the plugin's report,
the plugin wins.

### Pre-flight is a hard gate

`/pipeline` (or the `pipeline_preflight` tool) runs pre-flight. It returns one structured payload
with **every** check result — do not run the checks yourself.

- **Every check runs.** One failing row never aborts the rest; the payload lists all of them.
- **Pre-flight never runs the product suite** (`test.unit.command` in `.opencode/pipeline.config.json`) and never fires a commit hook.
  It checks wiring and reachability only. If the gate model is a self-hosted endpoint and it is
  down, pre-flight **fails closed** — there is no silent fallback to another provider.
- A `PRE-FLIGHT: FAIL` payload is a **hard stop**: do not begin survey-brief until each failing
  row is resolved or waived by a human.

### Configuration

Everything project-specific lives in `.opencode/pipeline.config.json` (test commands, toolchains,
path globs, handoffs dir, worktree naming). Role models are **not** committed here: they come from
the OpenCode configuration of the environment that runs the agents; pre-flight reports the resolved
role→model mapping verbatim and never reconciles it against Claude's model tiers. The role agents in `.opencode/agents/` are **generated** — edit
`docs/agent-overlays/<role>.md` and re-run `node ~/Projects/playbook/workflow/opencode/install.mjs
--project . --sync-agents`, never the generated files.

### Worktree-per-run

Every pipeline run — even a single-shot `rigor: direct` change — uses an isolated git worktree,
never the primary checkout's branch. The plugin provisions it during pre-flight:
`.worktrees/NNNN-<slug>` on branch `issue-NNNN-<slug>` (`NNNN` = the zero-padded issue number),
with the port and any throwaway database name derived deterministically from the issue number.
Prune orphaned worktrees (merged branch, closed story) before starting a new one — pre-flight
flags orphans.

### UI-gated phases

The `design` and `ui-walkthrough` phases apply only when the story has a UI surface
(`uiSurface: true` in pre-flight). When there is no UI surface they are **skipped and declared** —
never silently omitted.

## Verification

The authoritative suite is `test.unit.command` in `.opencode/pipeline.config.json`. It runs in the
**t-red** and **t-green** phases (the plugin reads its exit code), never in pre-flight.
