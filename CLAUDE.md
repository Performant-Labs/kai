# CLAUDE.md — kai

Claude Code entry point for this repo. The project instructions are in [`AGENTS.md`](AGENTS.md); read it first (project docs, ground rules, worktree-per-run). It is written for OpenCode, but its repo rules apply to Claude too.

## Test command

The authoritative suite is `test.unit.command` in `.opencode/pipeline.config.json`. Run this exact command for RED and GREEN. This is not an npm project at the root, so do not run `npm test`.

```
sh -c "(cd pkg/swiftbridge/scripts && bash ./build.sh) && go test -vet=off ./internal/... ./pkg/... -count=1 && pnpm --dir frontend test"
```

Environment note (verified 2026-09-25 on Node 26.7): the frontend tests fail on master with `localStorage.removeItem is not a function` unless the environment sets `NODE_OPTIONS=--no-experimental-webstorage`. That is a local Node issue, not a code defect; it does not change the command above.

## Worktrees and PRs

Work in a per-issue worktree under `.worktrees/` (branch `issue-NNNN-<slug>`), never in the primary checkout. Issues and PRs go to `Performant-Labs/kai-private`, never to upstream `dtapps/kai`.
