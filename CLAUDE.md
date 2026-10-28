# CLAUDE.md — kai

Claude Code entry point for this repo. The project instructions are in [`AGENTS.md`](AGENTS.md); read it first (project docs, ground rules, worktree-per-run). It is written for OpenCode, but its repo rules apply to Claude too.

## Test command

The authoritative suite is `test.unit.command` in `.aftersight/pipeline.config.json`. Run this exact command for RED and GREEN. This is not an npm project at the root, so do not run `npm test`.

```
sh -c "(cd pkg/swiftbridge/scripts && bash ./build.sh) && go test -vet=off ./internal/... ./pkg/... -count=1 && pnpm --dir frontend test"
```

Environment note: the frontend tests need no `NODE_OPTIONS` on any Node version. Node 25 and later define their own `localStorage`, and `frontend/vitest.setup.ts` replaces it with jsdom's for the tests.

## Worktrees and PRs

Work in a per-issue worktree under `.worktrees/` (branch `issue-NNNN-<slug>`), never in the primary checkout. Issues and PRs go to `Performant-Labs/kai` (pass `--repo Performant-Labs/kai --base master`), never to upstream `dtapps/kai`. The old private repository is archived.
