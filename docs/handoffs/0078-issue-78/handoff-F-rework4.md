# Handoff-F-rework4: Phase 5 — the 2 PR-Agent round-4 findings (R4a, R4b)

**Date:** 2026-10-20
**Branch / worktree:** issue-0078-issue-78 (`.worktrees/0078-issue-78`)
**Contract:** handoff-T-rework4.md (cases `posttagdrift`, `premergedrift`, `maskedexit`, `maskedexitcap`, `maskedexitpush`)
**Production scope:** `scripts/release.sh` only. No test file touched (`git status`: `M scripts/release.sh`, `M scripts/release.test.sh` = T's own RED edit, `M docs/handoffs/0078-issue-78/decisions.md` = T's, plus this handoff).

## Baseline (RED confirmed before any edit)

`bash scripts/release.test.sh` → exit 1, **223 ok / 25 assertion FAILs / FAILED** — the exact 25
from handoff-T-rework4 (re-verified line by line against its verbatim list; the grep count of 26
includes the `FAILED` summary line). `bash scripts/release-ask-ai.test.sh` → PASS.

## Fix 1 — R4a "Post-tag source mutation" (finding `3fe580bc5f88`)

`handler_regen_bindings` is now **phase-aware**: it takes `<phase> <log>` and, when the tag
phase is done **or** the PR is merged (`phase_done tag || phase_done merge` — the state the
dispatch can't see on its own), it `do_stop`s naming the tag-truth rule and the owner decision
BEFORE touching the tree. The guard lives in the handler, so both dispatch sites are covered:
the static known-blocker table (release.sh:267) and the AI's `run-handler regen-bindings` path
(release.sh:312) — post-tag, neither can mutate the tagged tree. Pre-merge the #73 behavior is
untouched (`premergedrift` pins it). The commit inside the handler also lost its `|| true`
(see Fix 2's class sweep): it now runs only when the regen actually staged something, and a
real failure (e.g. a rejecting hook) returns 1 instead of being swallowed.

`sed -n '212,231p' scripts/release.sh` — before:

```bash
handler_regen_bindings() { # <log> — make bindings + commit (#73)
  local log="$1"
  say "  .. known blocker: bindings drift — regenerating (make bindings) and committing (#73)"
  make bindings >>"$log" 2>&1 || return 1
  git add frontend/bindings >>"$log" 2>&1 || return 1
  git commit -q -m "Release $tag: regenerate frontend/bindings (#73)" >>"$log" 2>&1 || true
  return 0
}
```

after:

```bash
handler_regen_bindings() { # <phase> <log> — make bindings + commit (#73). PRE-merge only.
  # PR #100 R4a: once the tag is pushed (or the PR merged) the git tag is the source of
  # truth (docs/releasing.md) — regenerating and committing frontend/bindings then would
  # move the tree past the published tag, so the handler refuses instead: a hard stop for
  # the owner, no mutation of the tagged tree, no commit to release/*, no consultation.
  local phase="$1" log="$2"
  if phase_done tag || phase_done merge; then
    do_stop "$phase" "bindings drift, but $tag is already tagged/merged: the git tag $tag is the source of truth (docs/releasing.md) — regenerating frontend/bindings and committing it now would build from a tree that is not the tagged commit; an owner decision is required (fix forward and cut the next tag); the workflow never mutates the tagged tree (#73 refuses post-tag)" "$log"
  fi
  say "  .. known blocker: bindings drift — regenerating (make bindings) and committing (#73)"
  make bindings >>"$log" 2>&1 || return 1
  git add frontend/bindings >>"$log" 2>&1 || return 1
  # Commit only when the regen staged something; a real commit failure (e.g. a rejecting
  # hook) propagates to phase_run (PR #100 R4b class) instead of being swallowed.
  git diff --cached --quiet >>"$log" 2>&1 \
    || git commit -q -m "Release $tag: regenerate frontend/bindings (#73)" >>"$log" 2>&1 || return 1
  return 0
}
```

The two call sites, before → after (verbatim, the whole change is the added `"$phase"`):

```bash
-      handler_regen_bindings "$log" || do_stop "$phase" "make bindings / commit failed (#73)" "$log"
+      handler_regen_bindings "$phase" "$log" || do_stop "$phase" "make bindings / commit failed (#73)" "$log"
-            handler_regen_bindings "$log" || do_stop "$phase" "run-handler regen-bindings failed (#73)" "$log"
+            handler_regen_bindings "$phase" "$log" || do_stop "$phase" "run-handler regen-bindings failed (#73)" "$log"
```

Why `phase_done tag || phase_done merge` is the right predicate: every consultable phase that
can surface the drift signature is `bump`/`restructure`/`release-pr` (pre-merge by phase order)
or `local` (phase 10, after `tag` marks done at phase 9); and the #74 group-skip paths mark
`bump restructure release-pr` done when the PR was merged, so those phases never re-run in a
merged resume — `local` is the only post-merge surface, and `tag` is done by then. The
`merge` arm covers a merged-but-not-yet-tagged resume. It is state-only: no extra gh probe.

## Fix 2 — R4b "Masked exit status" (finding `5f2708832375`)

`release_pr_apply` now fails fast: `git add`, the commit (hook rejection included) and the push
each `|| return 1`, and `gh pr create`'s status is captured into `rc` and returned **after**
the `rm` cleanup. Every failure reaches `phase_run`, so the R3/R4 consult-and-retry loop runs
(1 consultation on the first failure, retry on `retry`, the cap-2 stop when it persists).
Nothing runs after a failed operation (the contract explicitly does not pin whether `gh pr
create` runs after a failed push — it does not).

`sed -n '489,505p' scripts/release.sh` — before:

```bash
release_pr_apply() {
  git add -A
  git diff --cached --quiet || git commit -q -m "Release $tag: version bump and changelog restructure"
  git push -q origin "release/$tag"
  local bf; bf="$(mktemp "${TMPDIR:-/tmp}/release-pr.XXXXXX")"
  printf 'Release %s: version bump and changelog restructure.\nManaged by scripts/release.sh (checklist issue #%s).\n' "$tag" "${ISSUE:-}" >"$bf"
  gh pr create --repo "$repo" --base master --head "release/$tag" --title "Release $tag" --body-file "$bf" >/dev/null
  rm -f "$bf"
}
```

after:

```bash
release_pr_apply() {
  # PR #100 R4b: every operation's exit status must reach phase_run — a rejecting commit
  # hook, a refused push, a failing gh pr create — so the R3/R4 consult-and-retry loop can
  # run; the closing rm must never mask them. Fail fast: nothing runs after a failure.
  git add -A || return 1
  git diff --cached --quiet || git commit -q -m "Release $tag: version bump and changelog restructure" || return 1
  git push -q origin "release/$tag" || return 1
  local bf rc
  bf="$(mktemp "${TMPDIR:-/tmp}/release-pr.XXXXXX")"
  printf 'Release %s: version bump and changelog restructure.\nManaged by scripts/release.sh (checklist issue #%s).\n' "$tag" "${ISSUE:-}" >"$bf"
  gh pr create --repo "$repo" --base master --head "release/$tag" --title "Release $tag" --body-file "$bf" >/dev/null
  rc=$?
  rm -f "$bf"
  return "$rc"
}
```

## The sweep — every helper whose last command is cleanup (rm/echo/say), and every `*_apply` / run wrapper

The class, not the instance: every function in `scripts/release.sh` (and the wrapped
`release-*.sh` tools release.sh invokes) was inspected for an operations status masked by a
trailing cleanup. Verdicts:

| Site (file:line at head 4960559) | Last command | Operations whose status could be masked | Verdict |
|---|---|---|---|
| `release_pr_apply` (release.sh:479) | `rm -f "$bf"` | `git add`, `git commit` (hook), `git push`, `gh pr create` | **FIXED** — per-op `\|\| return 1`; create's rc captured, returned after the rm (Fix 2) |
| `handler_regen_bindings` (release.sh:212) | `return 0` | `git commit` swallowed by `\|\| true` (hook rejection invisible) | **FIXED** — commit only when the regen staged something; a real failure returns 1 to the dispatch's `do_stop` (Fix 1's edit, R4b class) |
| `restructure_apply` (release.sh:453) | `run_tool release-check-version.sh … >/dev/null` | awk failure | already correct — awk failure does `{ rm -f "$tmp"; return 1; }` (capture, cleanup, return); the last command IS the operation. No change |
| `issue_comment` (release.sh:135) | `rm -f "$f"` | `gh issue comment` | reviewed, no change — the `\|\| true` is deliberate (R11: marker comments are best-effort; a comment failure must never block `do_stop`'s exit or a phase), and every caller ignores the status by design |
| `notify` (release.sh:130) | `osascript … \|\| true` | notification | reviewed, no change — best-effort by design, no operation to propagate |
| `cleanup` (release.sh:62, EXIT trap) | lock `rm -rf` | trap cleanup | n/a — an EXIT trap preserves the script's exit status; nothing is masked |
| `run_tool` (release.sh:125) | the wrapped invocation (both branches) | — | already correct — the tool's status is the function's status |
| `find_pr` / `merge_sha_of` (release.sh:160/164) | `\|\| true` | gh probes | n/a — callers test output emptiness and route to `do_stop`; the probe's status is deliberately not the signal |
| `phase_run` (release.sh:225) | loop | the phase command, `release-ask-ai.sh` | already correct — `rc=$?` each iteration; `arc=$?` for the AI call |
| `ph_verify` (release.sh:677) | `rm -rf "$vdir" "$idest"` after guarded ops | download/shasum/cmp/ditto/verify/install | already correct — every operation is `\|\| do_stop`-guarded BEFORE the cleanup rm; the trailing rm follows only guarded success; `hdiutil detach … \|\| true` is unmount cleanup, deliberately best-effort |
| `handler_quit_kai` (release.sh:205) | `return 0` | osascript/pkill | n/a — best-effort quit by design; the dispatch's `pgrep` re-check + handlers cap 2 is the real gate |
| `scripts/release-ask-ai.sh` (whole script) | explicit `exit 0` / `no_answer` → `exit 1` | timeout/opencode/jq | already correct — fail-closed with explicit exits; temp file trap-cleaned (`trap 'rm -f "$out"' EXIT`), which cannot mask the exit status |
| other `scripts/release-*.sh` tools | — | — | inspected: no helper functions with a cleanup-last shape (only `release-ask-ai.sh` defines one, covered above) |

Net: exactly two masked sites existed in the class (`release_pr_apply` itself and the
handler's swallowed commit); both fixed. Everything else either propagates already, is a
deliberate best-effort by documented design, or is a probe whose emptiness is the signal.

## Self-check (Tier-1 commands, run in this worktree)

```text
$ bash -n scripts/release.sh                                   → clean
$ bash scripts/release.test.sh                                 → exit 0, 248 ok / 0 FAIL / PASS
$ bash scripts/release-ask-ai.test.sh                          → exit 0,  24 ok / 0 FAIL / PASS
$ sh -c "(cd pkg/swiftbridge/scripts && bash ./build.sh) \
    && go test -vet=off ./internal/... ./pkg/... -count=1 \
    && pnpm --dir frontend test \
    && bash scripts/release.test.sh \
    && bash scripts/release-ask-ai.test.sh"                    → SUITE_EXIT=0
```

(the last is `test.unit.command` from `.aftersight/pipeline.config.json`, verbatim; inside it:
swift bridge built, every Go package `ok` (e.g. `ok  cnb.cool/dtapp/kai/internal/translate
13.040s`), frontend `Test Files 79 passed (79)` / `Tests 870 passed (870)`, both shell suites
PASS.)

### RED → GREEN accounting

`release.test.sh`: 223 ok + 25 FAIL (RED) → **248 ok + 0 FAIL** (GREEN). 223 + 25 = 248: every
round-4 assertion flipped green and no pre-existing assertion regressed. The five new scenarios
behave as pinned: `posttagdrift` stops at phase local (make.log 0, no regen commit, 0
consultations, no draft); `premergedrift` keeps the #73 regen+commit+retry (bump.count 2, tag
done, draft reached); `maskedexit` consults once, retries, recovers to exit 0;
`maskedexitcap` runs 3 apply attempts to the cap-2 consultation stop; `maskedexitpush` retries
the push 4 argv (1 branch + 3 apply), stops at the cap with release-pr not done and no draft.

Note: the authoritative GREEN is the plugin's T-green run of the same suite, not this
self-check.

## Tests I think are wrong

None. Both findings' pinned observables were implementable exactly as written; no test was
edited (and none could be).

## Ready for T(green)

Implemented against the RED contract: `scripts/release.sh` only, 25 insertions / 9 deletions.
The plugin should now run the suite.
