# CLAUDE.md — kai

Claude Code entry point for this repo. The project instructions are in [`AGENTS.md`](AGENTS.md); read it first (project docs, ground rules, worktree-per-run). It is written for OpenCode, but its repo rules apply to Claude too.

## Test command

The authoritative suite is `testSuites.unit.script` in `.aftersight/project.json` (the Claude pipeline reads the same line from `testCommands`). Run this exact command for RED and GREEN. This is not an npm project at the root, so do not run `npm test`.

```
bash scripts/test-unit.sh
```

It runs the Swift bridge build, then `go test -vet=off ./internal/... ./pkg/... -count=1`, then `pnpm --dir frontend test`, then the release-script tests (`scripts/release.test.sh`, `scripts/release-ask-ai.test.sh`).

Environment note: the frontend tests need no `NODE_OPTIONS` on any Node version. Node 25 and later define their own `localStorage`, and `frontend/vitest.setup.ts` replaces it with jsdom's for the tests.

## Coding pipeline (Aftersight)

This project runs its stories with the Aftersight Claude pipeline (`claude-pipeline-v0.4.0`) through the loader in
your client configuration; nothing of it is in this repository except the seeds in `.aftersight/` (`project.json`,
`claude.json`, and `opencode.json` for the OpenCode pipeline), which `aftersight config push` sends to the server.
Before you set it up or run a story, read the Aftersight agent runbook (`docs/runbooks/pipeline-for-agents.md` in the
Aftersight repository, which is private: read it with your `gh` login) and follow it. `AGENTS.md` has this project's
pipeline rules, and they apply to Claude too.

- Server: `https://localhost:3443` (project `kai`). The operator sets `AFTERSIGHT_API_TOKEN`; you never do.
- Run a story only when the operator names the issue and the rigor.
- Never print or store a token or key; never edit gate settings or credentials, force, or move a tag.
- Check `Closes` or `Refs` in the body of a pipeline pull request.
- A failing check is a stop: report the row and message, and ask.

## Worktrees and PRs

Work in a per-issue worktree under `.worktrees/` (branch `issue-NNNN-<slug>`), never in the primary checkout. Issues and PRs go to `Performant-Labs/kai` (pass `--repo Performant-Labs/kai --base master`), never to upstream `dtapps/kai`. The old private repository is archived.
