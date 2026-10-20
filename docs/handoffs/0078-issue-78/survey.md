# Survey — issue #78: a static release workflow; the AI is consulted only when a phase is blocked

Run `0078-issue-78`. Everything below was read in the worktree at `issue-0078-issue-78` (= `master`
at `15d323a` when provisioned). Verbatim excerpts carry file:line.

## The story in one paragraph

The v0.5.0 release (checklist #68, closed) was driven by an AI agent walking
`docs/release-checklist.md` (29 steps) and deciding as it went — it re-asked settled questions
(publish ×3, smoke ×2), stopped on things a script should handle (Kai running, leftover
`release/vX.Y.Z` branch), and found late what static checks find early (bindings drift, missing
signing key). Issue #78 inverts the shape: **a static workflow** (`scripts/release.sh vX.Y.Z
[--publish] [--smoke]`) runs every phase in a fixed order with fixed rules and no questions; an AI
is consulted only when a phase is blocked, with a bounded allowlisted job. The owner keeps exactly
the decisions config settles once (`--publish`, `--smoke`, recovery of tags by named action only).

## Corpus read

- `docs/releasing.md` (389 lines) — the release rules: version truth (tag), bump decision from
  `[Unreleased]` headings, CI-green requirement, PR-Agent > 90 on the release PR, signing
  (`Kai Release` self-signed cert, `release-cert.sh`, never ad-hoc without asking), packaging
  (4 assets, package once, never rebuild), updater constraints, changelog coverage, notes preface.
- `docs/release-checklist.md` (118 lines) — the 29-step template the workflow encodes; the
  checklist issue this workflow must open/report-on/close.
- `scripts/release-local.sh` (165 lines) — steps 11–16 in one command: machine check, tag/gates,
  fresh detached worktree of the tag, version/changelog check, notes, `make darwin-package`,
  tree-check, stable-signature check, verify, package, **draft** release (asks, or `--yes`;
  `--dry-run` creates nothing). Dies on first problem; never deletes/overwrites a release.
- `scripts/release-preflight.sh` (268 lines) + `.test.sh` — machine/commit checks; with #74/#75
  landed: a running Kai is a WARN (`release-preflight.sh:12` "A running Kai is only a WARN here"),
  a merged `release/vX.Y.Z` is a WARN with delete hint, unmerged still FAILs.
- `scripts/release-bump.sh` — edits exactly 4 lines (`build/config.yml` versions,
  `build/darwin/Info.plist` CFBundleShortVersionString/CFBundleVersion).
- `scripts/release-gates.sh` — tag exists / on origin/master / every CI check passed on that
  commit / no release (draft included) exists for the tag. `EXCLUDE_CHECK` env support.
- `scripts/changelog-check.sh` (+test), `release-notes.sh` (+test), `release-check-version.sh`,
  `release-tree-check.sh`, `release-verify.sh` (+test), `release-package.sh` (+test),
  `sign-check.sh` (+test), `codesign-app.sh` (+test), `install-release.sh` (+test).
- `.github/workflows/`: `ci.yml` (actionlint + gitleaks only), `push.yml` (Linux build+test,
  **bindings-drift gate** at `push.yml:149-157`), `release-kai.yml` (hosted runner; cannot build
  today — SDK too old; fails after gates, creates nothing), `release.yml` (upstream's, unused).
- `.gitignore:2` = `bin` → `bin/release/vX.Y.Z/state.json` is untracked local state, as the issue
  specifies.
- Issues: #74 (closed), #75 (closed) — the two pre-flight fixes the issue cites as done; #73
  (closed) bindings-drift CI; #31 (open) automated draft-from-tag — this story supersedes its
  agent-driven shape; #68 (closed) the 0.5.0 checklist this replaces as operator; #87 (closed)
  moved the pipeline to opencode-pipeline-v0.2.0 "so #78 can run through it at second-opinion".
- Prior run docs: `docs/handoffs/81/`, `96/` (handoff naming + journal format precedents).

## What exists vs. what is new

Exists and is reused as-is (wrapped, not modified unless a postcondition needs exposing):

| Concern | Existing asset |
| --- | --- |
| Steps 1–5 (machine, commit, CI, known issues, changelog coverage) | `release-preflight.sh`, `changelog-check.sh` |
| Steps 9–13 (bump decision inputs, branch, bump, changelog restructure, PR) | `release-bump.sh`, `gh`, `docs/releasing.md` §"Step by step" |
| Steps 14–16 + 23–26 (tag, build, verify, package, draft) | `release-local.sh` (incl. `--dry-run`, `--yes`) |
| Tag/CI/release-exists gates | `release-gates.sh` |
| Step 17 smoke matrix | `docs/release-checklist.md` §"Manual smoke matrix" (human; `--smoke` gates its inclusion) |
| Steps 27–29 + installer verification | `install-release.sh`, `release-verify.sh`, `gh release` |

New (no precedent in the repo):

1. `scripts/release.sh` — the phase state machine: fixed order, postcondition-first resume,
   `bin/release/vX.Y.Z/state.json`, `--dry-run` (default posture: everything up to the draft,
   nothing public), `--publish`, `--smoke`, `--quit-kai`, needs-the-owner pre-list, hard stops.
2. The known-blocker table with static handlers (Kai running, leftover `release/vX.Y.Z` branch,
   bindings drift, missing signing identity — the four that cost 0.5.0 time).
3. `scripts/release-ask-ai.sh` — one bounded consultation: fixed prompt (phase, command, last 200
   log lines, that phase's checklist text), read-only tools, JSON answer
   `{diagnosis, action, argument, reason}` validated against the allowlist `retry | wait |
   run-handler <name> | propose-fix-pr | stop`; ≤ 2 consultations per phase; out-of-allowlist →
   refused (treated as no answer → stop path).
4. Stop reporting: macOS notification + checklist-issue comment (phase, log tail, diagnosis,
   resume command) + the resume ergonomics.
5. The changelog-restructure and release-PR phases as code (today they are prose steps 9–13;
   `release-bump.sh` covers only the 4-line edit). **Note:** opening the release PR needs a token
   that can start CI; `GITHUB_TOKEN` cannot (`docs/releasing.md:215-217`) — the workflow runs on
   the release Mac with the owner's `gh`, which can.

## Reuse & Analogous-Feature map (required)

- **Closest analogue, orchestrator shape:** `scripts/release-local.sh` itself — step functions,
  `die`-on-failure, gates before side effects, `--dry-run`/`--yes` flags, machine check block.
  `release.sh` **extends by wrapping**: it calls `release-local.sh` for the whole build/verify/
  package/draft span rather than growing a second copy of that logic. Recommendation (default
  extend): do not fork any of its internals into `release.sh`; if a postcondition is not
  observable from outside (e.g. "draft exists with 4 assets"), add a small flag/query to the
  existing script instead of duplicating the step.
- **Test harness analogue:** `scripts/release-preflight.test.sh:19-31` (`mkrepo`/`run`/`has`/
  `hasnt`/`exit_is`, throwaway git repos, `--skip-github`, no network). New harnesses
  (`release.test.sh`, `release-ask-ai.test.sh`) copy that harness shape and stub the wrapped
  scripts with fakes on `PATH` (the issue's "tests with fake scripts").
- **Output convention analogue:** `release-preflight.sh:31-35` `ok/warn/fail/hint` printers and
  "runs every check even after a failure" — the blocker table and phase reports keep this voice.
- **State file:** no precedent; new `bin/release/vX.Y.Z/state.json` (jq is already a required
  tool, `release-local.sh:69`). Untracked by `.gitignore:2`.
- **AI invocation:** no precedent in `scripts/`. Recommended: `opencode run` (the runtime already
  on this Mac; the pipeline itself runs through it) with a JSON-only contract; absence of the
  binary = consultation unavailable → unknown-failure path (stop + report). Settle in A.

## Test wiring finding (drives one brief decision)

`scripts/*.test.sh` are run by **nothing**: not `make check` (`Makefile:202`), not `push.yml`
(Linux suite: bindings, sqlc, i18n, `make check`), not `ci.yml` (actionlint+gitleaks), and not the
pipeline's authoritative `test.unit.command` (`.aftersight/pipeline.config.json`):
`swiftbridge build && go test ./internal/... ./pkg/... && pnpm --dir frontend test`. They are
release-Mac harnesses by design (`release-preflight.test.sh:2-5`). Consequence: a sibling
`release.test.sh` alone makes T's RED **invisible to the plugin**. Brief decision: amend
`test.unit.command` to append the two new harnesses (this Mac satisfies their assumptions; the
t-red/t-green phases run here). Wiring them into GitHub CI is out of scope for this story.

## Risks / open questions for A

1. **Postcondition granularity** — every phase must be checkable from disk/network state alone
   (resume = re-run). The phase list in the issue mixes repo state (branch, PR merged, tag) with
   local state (draft created). A settles the canonical phase list + each postcondition.
2. **The PR-Agent > 90 gate** is human-readable output, not an API number; how the workflow waits
   for and evaluates it needs a precise rule (or it stays owner-listed).
3. **`release-ask-ai.sh` mechanism** (see map) and its read-only enforcement in practice.
4. **`--smoke` semantics** — the matrix is human-on-real-Mac; presumably pause-and-confirm +
   record. A settles; the checklist issue records "skipped" when unset.
5. **Throwaway-tag rehearsal format** (post-merge, owner-driven; this run delivers the harness).
6. **No migrations analogy** — N/A here (no DB); ignore the pipeline's migration language.
