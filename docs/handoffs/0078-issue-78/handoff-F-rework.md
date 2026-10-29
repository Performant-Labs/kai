# Handoff-F-rework: Phase 5 (rework) — implement PR #100's two PR-Agent findings

**Date:** 2026-10-20
**Branch / worktree:** issue-0078-issue-78 (`.worktrees/0078-issue-78`)
**Contract:** `handoff-T-rework.md` (RED: 8 FAIL pins across `mergedpr` + `verifyok`/`verifystop`/`verifyterm`)
**Provenance note:** the implementation below was applied in F's previous session, which was
killed after verification but before this handoff was written. This session changed **no code
and no test** (`git diff` at handoff time = the rework itself + `decisions.md`'s T-rework entry
+ one unrelated stray, disclosed below); everything here was re-verified from scratch this
session, including fresh suite runs.

## What I changed (production: `scripts/release.sh` only)

### F1 — `ph_branch` MERGED path must group-skip phases 3–6 (resume-state gap)

Pinned by scenario `mergedpr` (2 FAIL pins: bump re-run + dirty tree). The MERGED-PR corner
(state absent, PR #101 MERGED, its remote branch deleted by `gh pr merge --delete-branch`)
marked only `branch` done, so resume re-ran `release-bump.sh` and left uncommitted version
edits. Fix: mirror the #74 leftover-branch group-skip exactly.

`scripts/release.sh:425-429` — before (at HEAD `04874b9`; `scripts/release.sh` is identical
at `5a6f19e` — the commits between touched only docs):

```bash
    if [[ "$2" == "MERGED" ]]; then
      say "  .. release PR #$1 already merged (its branch was deleted by the merge)"
      mark_done branch
      return 0
```

after (working tree):

```bash
    if [[ "$2" == "MERGED" ]]; then
      say "  .. release PR #$1 already merged (its branch was deleted by the merge): skipping phases 3-6 like the leftover-branch rule (#74)"
      mark_done branch bump restructure release-pr
      return 0
```

Identical `mark_done` list to the leftover path at `:417` (`mark_done branch bump restructure
release-pr`) — one rule, two observers.

### F2 — `ph_verify` temp dirs must be cleaned on every exit (temp-dir leak)

Pinned by `verifyok`/`verifystop`/`verifyterm` (6 FAIL pins: `--dest` leak on success; workdir
+ `--dest` + anything-left leaks on hard stop; workdir + anything-left on a mid-verify SIGTERM).
Before: `rm -rf "$vdir"` existed only on the success line, and the inline
`--dest "$(mktemp -d)"` leaked unconditionally. Fix: a `VERIFY_TMP` accumulator drained by the
global `cleanup()` (already the EXIT trap at `:82`), plus a TERM trap for signal deaths.

1. `scripts/release.sh:61-65` — declaration + drain added to the existing `cleanup()`:

```bash
VERIFY_TMP=""   # phase 13's throwaway dirs (PR #100 F2): trap-cleaned on every exit
cleanup() {
  local d
  for d in $VERIFY_TMP; do [[ -n "$d" ]] && rm -rf "$d"; done
  VERIFY_TMP=""
```

(before: `cleanup()` began directly at the `if [[ "$LOCKED" -eq 1 …` line — no temp handling.)

2. `scripts/release.sh:661-666` — TERM trap + workdir registered the moment it exists:

```bash
  # PR #100 F2: both throwaway dirs below are cleaned on EVERY exit — success, a stop,
  # an unknown failure (the EXIT trap runs cleanup), and a signal mid-verify, where no
  # end-of-function rm could ever run: the TERM trap cleans and exits 143 (128+15).
  trap 'cleanup; exit 143' TERM
  local vdir; vdir="$(mktemp -d "${TMPDIR:-/tmp}/release-verify.XXXXXX")"
  VERIFY_TMP="$vdir"
```

3. `scripts/release.sh:688-692` — the install `--dest` dir captured in a local and registered
   *before* the `run_tool` whose `do_stop` is the exit between the two dirs:

```bash
  local idest; idest="$(mktemp -d)"
  VERIFY_TMP="$vdir $idest"
  run_tool install-release.sh "$tag" --dest "$idest" --keep-permissions \
    || do_stop verify-published "install-release.sh against the published release failed"
```

(before: `run_tool install-release.sh "$tag" --dest "$(mktemp -d)" --keep-permissions \` —
dir created inline, address discarded, unconditional leak.)

4. `scripts/release.sh:694-696` — success path still cleans eagerly, then disarms:

```bash
  rm -rf "$vdir" "$idest"
  VERIFY_TMP=""
  trap - TERM
```

(before: `rm -rf "$vdir"` only — the `--dest` dir was never removed.)

**Exit-path closure for ph_verify:** success → explicit `rm` + disarm; any failure →
`do_stop` → `exit 1` → EXIT trap (`:82 trap cleanup EXIT`) → drain; signal mid-verify → TERM
trap → `cleanup; exit 143`. `ph_verify` is not in `CONSULTABLE`
(`:223 CONSULTABLE=" bump restructure release-pr local "`), so `do_stop` is its only failure
exit (matches T's hedge); the TERM trap intentionally survives into `closeout` after a clean
verify — harmless: `VERIFY_TMP` is empty then and `cleanup` is idempotent (still releases the
lock, as before).

## Self-check (run fresh this session, not trusted from the killed one)

- `bash scripts/release.test.sh` → **exit 0, PASS, 188 `ok` lines / 0 `FAIL`** — all 8 RED
  pins green (7 `mergedpr` assertions incl. `bump is NOT re-run` and `tree stays clean`; 15
  F2 assertions across the three exit classes), baseline 166 unchanged. Tail:

  ```
  ok   PR100-F2: a forced mid-verify kill exits non-zero
  ok   PR100-F2: trap-based cleanup still runs on a forced mid-verify exit
  ok   PR100-F2: nothing at all survives a forced mid-verify exit
  ok   AC7: CHANGELOG [Unreleased] carries the #78 skip line
  ok   AC6/R5-5: no tag is ever deleted
  ok   AC6/R5-5: no force push is ever issued
  PASS
  ```

- `bash scripts/release-ask-ai.test.sh` → **exit 0, PASS, 24 `ok` / 0 `FAIL`** (untouched by
  this rework, hash-identical to RED). Tail:

  ```
  ok   A09/R9: an envelope with no text part is no-answer
  ok   A09/R9: the no-answer condition is reported
  PASS
  ```

Note: the authoritative GREEN is the plugin's T-green run of `test.unit.command`, not this
self-check.

## Class sweep — every `mktemp` in the release scripts, and why it cannot leak

`grep -n mktemp scripts/release.sh scripts/release-ask-ai.sh scripts/release-local.sh`:

| Site | What | Why it cannot leak on any exit path |
|---|---|---|
| `release.sh:665` | `ph_verify` `release-verify.XXXXXX` workdir | `VERIFY_TMP="$vdir"` at `:666`, one line after creation; drained by `cleanup()` via the EXIT trap `:82` on every `do_stop`/unknown-failure exit, by the TERM trap `:664` on signals, and eagerly at `:694` on success. |
| `release.sh:689` | `ph_verify` install `--dest` dir | `VERIFY_TMP="$vdir $idest"` at `:690`, *before* the `run_tool … || do_stop` at `:691` — the one exit that lies between the two dirs is therefore covered; same trap drain + eager `rm` at `:694`. |
| `release.sh:138` | `issue_comment` `release-comment.XXXXXX` body file | No intervening exit path between `mktemp` (`:138`) and `rm -f "$f"` (`:147`): the body write is builtins + a guarded `tail`, and `gh issue comment … || true` (`:145`) cannot fail the script — there is no `errexit` (`:21 set -uo pipefail`), no `do_stop`, no `return` in between (`do_stop` itself calls `issue_comment` and exits only after it returns). Residual: only a signal landing inside that millisecond window — a single small file, not a dir. |
| `release.sh:481` | `release_pr_apply` `release-pr.XXXXXX` body file | Same shape: `mktemp` (`:481`) → `printf` builtin → `gh pr create` → `rm -f "$bf"` (`:484`), no `errexit`, so even a failing `gh pr create` falls through to the `rm`; `phase_run` captures `rc=$?` only *after* the handler returns (`:231 "$@" >"$log" 2>&1`), so every downstream failure class (hard stop, AI consult, cap stop) fires after the file is gone. Same signal-window residual. |
| `release-ask-ai.sh:55` | `release-ask-ai.XXXXXX` | Its own `trap 'rm -f "$out"' EXIT` at `:56`, the line right after `mktemp` — covers every exit of that script (`no_answer` exits, success `exit 0`). |
| `release-local.sh:100` | local-build workdir | Its own `cleanup()` + `trap cleanup EXIT` at `:102-103`, immediately after `mktemp` — covers `die`, the dry-run `exit 0`, the no-terminal/refusal exits, and success. |
| `release-local.sh:164` | `mktemp -d` inside a printed message | Not a code site: it is the literal text of the operator-facing "Next steps" heredoc (instructions for the human's own shell); no dir is created by the script there. |

Every site the PR-Agent class could reach is either trap-covered (its own, or `VERIFY_TMP`) or
has no exit path between create and remove. The two signal-window residuals (`:138`, `:481`)
are single temp *files* whose cleanup window is a handful of builtins wide — out of the RED
contract's scope (which pins `ph_verify`'s dirs) and not worth a global trap churn; noted here
for the record, not fixed.

## Resume-class consistency — every MERGED observer group-skips

`grep -n 'MERGED\|mark_done' scripts/release.sh`, read in context:

| Phase | Lines | MERGED handling |
|---|---|---|
| `ph_branch` | `:425-429` | MERGED PR + deleted branch → `mark_done branch bump restructure release-pr` (**the F1 fix**), mirroring the #74 leftover path at `:414-418`. |
| `ph_review` | `:515-520` | pre-existing: `if [[ "$prst" == "MERGED" ]]` → "the review gate was settled by its merge" → `mark_done review`. |
| `ph_merge` | `:539-555` | pre-existing: MERGED is the accepted pre-state (`:540 if [[ "$prst" != "MERGED" ]]` guards the actual merge), re-checked post-merge (`:546 [[ "$prst" == "MERGED" ]] || do_stop …`), then `mark_done merge` `:555`. |

No resume-side observer of a MERGED release PR re-runs a mutating phase — the class is
consistent after F1.

## Tests I think are wrong

None. All 8 RED pins fail for the right reason against the pre-fix code (T's verbatim FAIL
output matches the defects) and pass for observable behavior (invocation counts, dirty-tree
checks, surviving-directory checks) after the fix.

## Disclosure — one stray working-tree change, hidden by an `assume-unchanged` bit

While this handoff was being written, a **concurrent session** (an `opencode run --continue`
process is live on this machine) modified `internal/service/input_monitoring_request_test.go`
(mtime `Oct 20 04:33:08`, after the rework files; mentioned nowhere in this run's journal or
handoffs). The edit forces `swiftbridge.KaiInputMonitoringRequest = nil` (with a `t.Cleanup`
restore) so the #23 test cannot open System Settings when another test in the package has
loaded the real dylib. Timeline, from my session's own observations:

- ~04:44 — my first `git status` showed it as a plain uncommitted modification (diff quoted
  the force-nil hunk against HEAD).
- ~04:48 — it vanished from `git status` and `git diff HEAD`, **without** the content
  changing (mtime still 04:33:08) and without any commit, stash, or staged change existing.

The cause, verified: `git ls-files -v` reports the file as `h` (lowercase = the index's
**assume-unchanged** bit; it is the only non-`H` entry in the repo), while the blobs still
differ — `git show HEAD:… | shasum` = `a4d6f27f…`, working-tree `shasum` = `6dedd472…`. So
the concurrent session edited the file and then hid it from status via
`git update-index --assume-unchanged`.

Why this matters for the run, not just hygiene: T-green's zero-test-edits provenance and the
diff gate read `git status`/`git diff`, and an assume-unchanged file is invisible to both
while its content diverges from HEAD. I did **not** touch the file, the bit, or the index —
it is a Go test file (T-owned, outside this rework's scope and my permissions), and the other
session may still be working. The operator/human should reconcile it before T-green/merge:
either land the edit properly on whatever branch owns it or revert the content and clear the
bit (`git update-index --no-assume-unchanged
internal/service/input_monitoring_request_test.go`) so the worktree is honestly readable
again.

## Tier-1 commands (re-runnable evidence for any reviewer)

- `bash scripts/release.test.sh` — the rework harness: expect `PASS`, 188 `ok` / 0 `FAIL`, exit 0
- `bash scripts/release-ask-ai.test.sh` — untouched sibling harness: expect `PASS`, 24 `ok` / 0 `FAIL`, exit 0
- `git diff scripts/release.sh scripts/release.test.sh` — the rework diff itself (both fixes + T's cases)
- `grep -n mktemp scripts/release.sh scripts/release-ask-ai.sh scripts/release-local.sh` — the class-sweep site list
- `grep -n 'MERGED\|mark_done' scripts/release.sh` — the resume-class site list
- `git ls-files -v | grep -v '^H'` — the disclosure check (currently: 1 entry, the stray file)

## Ready for T(green)

Implemented against the RED contract (`handoff-T-rework.md`); both harnesses PASS locally
(188/0 and 24/0). The plugin should now run the suite: GREEN means the loop advances (diff
gate as a fresh round 1 on the changed diff, per `decisions.md`), RED means I rework.
