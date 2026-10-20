# Handoff-F: Phase 5 - Implement (rework 7, the PERFECT round)

**Date:** 2026-10-20
**Branch / worktree:** issue-0078-issue-78 (`.worktrees/0078-issue-78`)
**Contract:** handoff-T-rework7.md (14 RED rows, three production defects, + the owner's sweep list)

## What I changed

Production: `scripts/release.sh` (defects A + B1, plus one D1 advisory), `scripts/release-ask-ai.sh`
(defect B2), `scripts/merge-go-i18n.sh` + `scripts/merge-frontend-i18n.sh` (sweep: mktemp without
trap), `docs/releasing.md` (behavior docs). `docs/release-checklist.md`: re-read end to end, **no
stale sentence found, no change** (details under "Docs"). No test file was touched:
`git diff --name-only` over `*.test.sh` shows only T's round-7 edits.

### A. drystop / drystopdone — `--dry-run` + smoke flags refused up front (RED rows 1-7)

`scripts/release.sh`, extending the existing combination-validation block (was release.sh:45-46,
before `root`/lock/state — so the refusal precedes every phase, gh call, wrapped tool and VCS op;
exit 2 matches the block's precedent):

```bash
# before (line 46, the whole combination validation):
[[ $want_dry -eq 0 || $want_pub -eq 0 ]] || { echo "give --dry-run or --publish, not both" >&2; exit 2; }

# after (same two lines kept, then):
if [[ $want_dry -eq 1 && $smoke -eq 1 ]]; then
  echo "--dry-run cannot be combined with --smoke: the smoke matrix belongs to the default posture (run it on the packaged build after the draft release is created; recording with --smoke/--smoke-done happens there too)" >&2
  exit 2
fi
if [[ $want_dry -eq 1 && $smoke_done -eq 1 ]]; then
  echo "--dry-run cannot be combined with --smoke-done: recording the smoke matrix happens after the draft release exists, and a dry run never creates one — re-run in the default posture and record it there" >&2
  exit 2
fi
```

Each message names both flags and the rationale (matrix / default posture / draft). The header
comment's Postures paragraph (the `-h` help text) gained the same fact in three lines. The
companions hold unchanged: `drystopub` (dry+publish refusal, release.sh:52) and `dryquit`
(--quit-kai acts under --dry-run at phase 0) are green without edits; `--dry-run` alone (ac1life)
and `--smoke` alone (smoke) untouched. Live check outside the harness (scratch dir, real script):
all three refusals print and exit 2 before any output phase.

### B1. handlercounts — per-handler blocker retry counters (RED rows 8-11)

`scripts/release.sh` `phase_run`: the one shared counter became two, one per static handler table.
The AI-consultation bookkeeping (`consult_count` / `record_consult`, 2 per phase) is untouched —
`aicap` stays green on the same code.

```bash
# before:
local handlers=0 rc answer arc action diagnosis base arg n why
...
        handlers=$((handlers + 1))
        [[ $handlers -le 2 ]] || do_stop "$phase" "Kai is still running after two quit attempts" "$log"
...
      handlers=$((handlers + 1))
      [[ $handlers -le 2 ]] || do_stop "$phase" "bindings drift persists after regeneration (#73)" "$log"

# after:
local quit_tries=0 regen_tries=0 rc answer arc action diagnosis base arg n why
...
        quit_tries=$((quit_tries + 1))
        [[ $quit_tries -le 2 ]] || do_stop "$phase" "Kai is still running after two quit attempts" "$log"
...
      regen_tries=$((regen_tries + 1))
      [[ $regen_tries -le 2 ]] || do_stop "$phase" "bindings drift persists after regeneration (#73)" "$log"
```

The caps themselves are unchanged (2 per handler): `handlercapkai` (third quit stops, exactly two
dispatches) and `handlercapregen` (two regens then the persists-stop) green; `handlercounts`
(quit x2 + regen x2, bump 5x, draft reached) green. AI-dispatched `run-handler` calls remain
uncounted by these counters, as before (the consultation cap bounds them).

### B2. escapere — the command description is a FIXED string (RED rows 12-14)

`scripts/release-ask-ai.sh:59` — the description half moved from the ERE to `grep -iF`; the phase
half keeps regex semantics (the phase name is release.sh's own vocabulary):

```bash
# before:
  checklist="$(grep -iE "$phase|$first_word" "$cl" 2>/dev/null | head -15 || true)"

# after:
  checklist="$({ grep -iE -- "$phase" "$cl" 2>/dev/null || true; grep -iF -- "$first_word" "$cl" 2>/dev/null || true; } \
    | awk '!seen[$0]++' | head -15)"
```

Why this shape: `grep -F` has no pattern-error mode, so a metachar description can neither
mis-select (`[xyz]` as a class) nor kill the context (`a(b` → grep exit 2 swallowed by `|| true`
into an empty context) — the exit-2 class is gone for the arbitrary half, not hidden. The two
`|| true`s mean "no match" (exit 1) is a normal empty half under `pipefail`; a line selected by
both halves prints once (awk dedup). The in-file comment carries the defect modes verbatim.

### Sweep fix 1 (D1 advisory, taken) — `restructure_apply`'s bare `mv`

```bash
# before:                                        # after:
  mv "$tmp" "$root/CHANGELOG.md"                   mv "$tmp" "$root/CHANGELOG.md" || { rm -f "$tmp"; return 1; }
```

An `mv` failure now propagates to `phase_run` (the trailing `release-check-version.sh` re-check
would have caught it, but the `CHANGELOG.md.rs.$$` tmp leaked in `$root` on that path).

### Sweep fix 2 (mktemp class) — `merge-go-i18n.sh` / `merge-frontend-i18n.sh`

Both had `local tmp_file=$(mktemp)` … `rm -f "$tmp_file"` with no trap: under their `set -e`, a
failing `jq` aborts mid-function and the tmp leaks. Fix (identical in both):

```bash
  # tmp_file is a GLOBAL on purpose: the EXIT trap runs after this function's locals are
  # unwound when set -e aborts mid-merge (a failing jq), so it must see the path there.
  tmp_file=$(mktemp)
  trap 'rm -f "$tmp_file"' EXIT   # fires even when set -e aborts mid-merge (jq failure)
```

Dropping `local` is load-bearing, verified by a live probe before shipping it: an EXIT trap set
while a *local* holds the path sees it **empty** after `set -e` unwinds the function
(`trap sees local: [] global: [/var/.../tmp.XXX]`) — a trap referencing the local would `rm -f ""`
and leak anyway. Probe after the fix (scratch copy, `jq` parse-error steer): abort exit, tmp count
before == after (no leak). Both real scripts re-run: exit 0, byte-identical output
(`git status` on `internal/i18n/locales` + `frontend/src/locales` empty). **Scope note:** these
two files sit outside the pair T named ("production must change in exactly release.sh +
release-ask-ai.sh"); they are the owner's sweep bullet "any mktemp without trap cleanup" answered
literally, no test drives them, and the evidence above is direct runs — flagging the scope
extension here for S.

## Self-check (the plugin's T-green run is the authoritative verdict)

| suite | before (RED reproduced) | after |
|---|---|---|
| `bash scripts/release.test.sh` | 377 ok / 11 FAIL | **388 ok / 0 FAIL — PASS** |
| `/bin/bash scripts/release.test.sh` (3.2.57) | 377 ok / 11 FAIL (T's run) | **388 ok / 0 FAIL — PASS** |
| `bash scripts/release-ask-ai.test.sh` | 33 ok / 3 FAIL | **36 ok / 0 FAIL — PASS** |
| `/bin/bash scripts/release-ask-ai.test.sh` | 33 ok / 3 FAIL (T's run) | **36 ok / 0 FAIL — PASS** |

Row arithmetic: exactly the 14 RED rows flipped; no other row changed (388 = 377+11, 36 = 33+3;
both files print PASS and exit 0). Full authoritative suite (`test.unit.command` in
`.aftersight/pipeline.config.json`): **exit 0** — swiftbridge build ok, `go test ./internal/...
./pkg/...` all ok, frontend 79 files / 870 tests passed, both release suites PASS.

## The owner's sweep table

| # | class | sites audited | verdict / evidence |
|---|---|---|---|
| 1 | exit status masked by a trailing cleanup | every phase/apply function in release.sh (T's D1 map re-walked) + the wrapped scripts' tails | `release_pr_apply` fixed in R4b (rc captured, then `rm`; pinned `maskedexit*`). `restructure_apply` bare `mv` **fixed this round** (above). `handler_regen_bindings` every op `\|\| return 1`. `ph_needs`' `gh issue create \| tail \| sed` under pipefail: empty ISSUE → do_stop. `st_update` swallows jq failure by design (state is a cache; absent re-derives). `issue_comment`/`notify` `\|\| true` by design (niceties, never gates). `ph_verify` every stop `\|\| do_stop` + trap-backed dirs (pinned by B3's 8 legs + verifyok/verifystop/verifyterm). Wrapped scripts end with their real status (early `die`/`exit 1`, success `exit 0`) — no trailing-cleanup mask found. **No open site.** |
| 2 | gh query blind to merged/closed state or a deleted head branch | `find_pr`, `merge_sha_of`, the merge re-query, `gh issue list` calls | `find_pr` uses `--state all` (release.sh:182; pinned mergedvisreview / mergedvismerge / leftover / mergedpr). Re-query guarded for empty (`:605`, `set -- $info` crash guard). `merge_sha_of` reads `mergeCommit` via `pr view` — works for a merged PR whose head branch is deleted. **Carried, by decision** (round-6 notes A/B, restated in T-rework7 D3): `gh issue list --search` sees only open issues in real gh, so a closeout-time state-loss re-run can open a cosmetic duplicate checklist issue — left as the owner decided. |
| 3 | local use of a remote-only object (fetch first) | every sha consumer in release.sh | `ph_tag`: `cat-file -e` → `git fetch -q origin` → `cat-file -e` again → stop (`:638-641`; pinned tagresume + mergefetch + both `fetched_before_tag` rows). `ph_branch` fetches before `merge-base --is-ancestor` (`:463`). `ci_green_on` reads check-runs via `gh api` (remote), needs no local object. **No open site.** |
| 4 | resume marking a phase done without its dependents | every `mark_done` call | The branch group-skip marks its whole group at once (`mark_done branch bump restructure release-pr`, `:468`/`:478`); `review` settles on the MERGED PR and `merge` adopts it — both re-derive from `find_pr --state all`; `ph_changelog`'s postcondition probe implies restructure's, which re-derives via the same probe. Pinned per group by the stateloss D rows + mergedvisreview / mergedvismerge / tagpush. **No orphan found.** |
| 5 | gate silently skippable by a flag combination | the D2 grid | Complete after this round: dry+smoke **refuse** (new), dry+smoke-done **refuse** (new), dry+publish refuse (drystopub), dry+quit-kai acts (dryquit), publish+smoke acts (awaiting-owner exits 0 before publish), publish+smoke-done acts (smokeresume), smoke+smoke-done takes smoke-done (acts). **Every combination now acts or refuses.** Position on T's open question (`--smoke --smoke-done` without --dry-run): **keep the precedence** — `--smoke-done` is the strictly stronger owner assertion (it resolves exactly the awaiting-owner state `--smoke` would write), so the combined intent is unambiguous and a refusal would block a resolvable request. Now documented (releasing.md + the script header); T pins it or leaves it documented. |
| 6 | mktemp without trap cleanup | all 11 mktemp sites in production scripts | Trap-covered already: release-ask-ai.sh (`trap 'rm -f "$out"' EXIT`), release-cert.sh, install-release.sh, release-local.sh, release-package.sh, release-verify.sh (each `trap cleanup EXIT`), release.sh ph_verify (`VERIFY_TMP` + EXIT + TERM traps, pinned), release.sh `issue_comment` (no exit path between mktemp and its unconditional `rm -f`; `gh … \|\| true` cannot abort), release_pr_apply (rc captured, then `rm`). **Fixed this round:** merge-go-i18n.sh + merge-frontend-i18n.sh (above, probe-verified). **No open site.** |
| 7 | tree mutated after the annotated ref exists | every git-write path | `handler_regen_bindings` refuses when `phase_done tag \|\| phase_done merge` (`:239`; pinned posttagdrift: no regen, no commit, no consult). The only other committer, `release_pr_apply`, runs at phase 6 (pre-tag) and every resume path sees the MERGED PR first (`find_pr --state all`), so it never re-runs post-tag. `ph_local` builds in a fresh detached worktree under mktemp (release-local.sh:100-103), never the working tree. **No open site.** |
| 8 | set -u / pipefail pitfalls; bash 3.2 | release.sh + release-ask-ai.sh (changed files), plus T's all-17-scripts scan | Production uses **no arrays at all**, so no empty-array expansion under `set -u` is possible; hazard grep re-run this round on both changed files: zero hits (mapfile/readarray/`declare -A`/`${var,,}`/`\|&`/`&>`/`read -s`/`printf %q`/coproc/lastpipe). `VERIFY_TMP` is a plain string iterated unquoted — empty expands to zero iterations in 3.2 too. `bash -n` and `/bin/bash -n` clean on all four changed scripts. Empirical: the b32 scenario (whole flow under /bin/bash 3.2.57) green in both suite runs, and both full harnesses PASS under /bin/bash. New code uses only `[[ ]]`, `case`, grep/awk, `trap` — 3.2-safe. |

## Docs

- `docs/releasing.md`, two edits: (1) the operator paragraph now states the known blockers have
  static handlers "each capped at two dispatches per handler per phase" (the behavior B1 pins);
  (2) the Postures paragraph states the whole flag grid in one sentence — **the dry-run/smoke
  refusal documented once here**, plus the `--smoke --smoke-done` precedence and `--quit-kai`'s
  posture-independence. The `release-local.sh --dry-run` examples elsewhere in the file are that
  script's own flag and stay accurate.
- `docs/release-checklist.md`: re-read end to end against the final behavior — **zero stale
  sentences**; its smoke sentence ("recorded by the workflow when you re-run with
  `--smoke` / `--smoke-done`") stays true (the recording re-run is the default posture, and the
  checklist already defers to releasing.md, where the refusal lives once). No change.
- AC7 pins re-verified green: both docs still name `scripts/release.sh`; the CHANGELOG `#78`
  skip line untouched.

## Tier-1 commands (run in the worktree; the plugin's T-green run of `test.unit.command` is the authoritative GREEN)

- `bash -n scripts/release.sh && /bin/bash -n scripts/release.sh` (and the same on
  `release-ask-ai.sh`, `merge-go-i18n.sh`, `merge-frontend-i18n.sh`) → clean.
- `bash scripts/release.test.sh` → **PASS**, 388 ok / 0 FAIL (round-7 RED baseline 377/11).
  All 14 RED rows green: drystop 7 (refusal exit, flags named, rationale, no gh call, no
  wrapped tool, no VCS op, for both --smoke and --smoke-done), handlercounts 4 (exit 0 with
  quit x2 + regen x2 in one phase, both dispatch counts 2, bump 5x, draft reached), escapere
  3 (char-class line OUT, literal `a(b` line IN, phase line still present). Guards green and
  untouched: drystopub, dryquit, handlercapkai, handlercapregen, aicap, the 8 verifytrapall
  legs, b32, norealbins.
- `bash scripts/release-ask-ai.test.sh` → **PASS**, 36 ok / 0 FAIL (baseline 33/3).
- Both suites re-run under `/bin/bash` (3.2.57) → identical PASS (388 + 36).
- Full authoritative suite `test.unit.command` → **exit 0** (swiftbridge build,
  `go test -vet=off ./internal/... ./pkg/... -count=1` all ok, `pnpm --dir frontend test`
  79 files / 870 tests, both release suites PASS).
- Live refusals outside the harness (scratch dir, real script): `--dry-run --smoke`,
  `--dry-run --smoke-done`, `--dry-run --publish` each print their message and exit 2 before
  any phase output.

## Incident note (transparency for S)

The round's commit is an `--amend` of the first attempt: after the full-suite green run, the
two `merge-*-i18n.sh` edits silently reverted to their original bytes in the worktree between
my last content-verification (`git diff` showed both hunks) and `git add -A` — no hook, no
watcher and no command in my transcript writes those files, and the cause was not identified.
Caught by the commit stat (8 files instead of 10), re-applied from the verified diff, staged
explicitly, amended, and re-verified in BOTH the worktree and `git show HEAD:` (grep + stat).
Nothing the authoritative suite reads changed after the green run (`release.sh` last modified
09:57, `release-ask-ai.sh` before the run; the merge scripts are not in any suite's path), so
the suite evidence stands; the fast ask-ai suite was re-run post-amend as a smoke check.

## Tests I think are wrong (if any)

None. All 14 RED rows were implementable exactly as pinned; every guard row
(drystopub, dryquit, handlercapkai, handlercapregen, aicap) passed without touching them.

## Ready for T(green)

Implemented against the RED contract; both harnesses and the full authoritative suite pass
locally (388 + 36 ok, suite exit 0). The plugin should now run the suite: GREEN means the loop
advances; RED means I rework.
