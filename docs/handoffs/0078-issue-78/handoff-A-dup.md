# Handoff-A-dup: Phase 7 — Anti-duplication gate (issue #78: static release workflow)

**Date:** 2026-10-20
**Branch:** `issue-0078-issue-78` (worktree `.worktrees/0078-issue-78`)
**Diff base:** `fe7ca50` (origin/master)   **Diff head:** `616d7aa`
**Reuse map:** `docs/handoffs/0078-issue-78/survey.md` §"Reuse & Analogous-Feature map"
**Verdict:** PASS

## Summary

F **extended, not forked**. `release.sh` is the wrap-style orchestrator the Reuse map and rulings
R7/R10/R12 prescribed: the whole build/verify/package/draft span is one call to
`release-local.sh` (which itself owns gates/notes/verify/package), the bump is one call to
`release-bump.sh`, and no version-file, gates, notes, packaging or draft-creation logic was
copied — grep of `release.sh` for `config.yml|Info.plist|CFBundle|release create` finds nothing,
and every wrapped script is invoked by bare name through `run_tool`. `release-ask-ai.sh` is the
single AI seam (the only `opencode` invocation in the diff), state is jq + `state.json` exactly
per R6 with no ad-hoc marker system, both harnesses copy the `release-preflight.test.sh` shape
(same `has/hasnt/exit_is` cadence, throwaway repos, env-steered fake `gh`, fake `git` wrapping
the real one) with cleanly distinct scopes, and the docs say each thing once per scope. Zero
wrapped scripts were modified (R12). Three Warns are disclosed ruling-letter deviations (none
creates a parallel path; all de-scope rather than duplicate); four Nits are follow-up notes.

## Findings

| # | Severity | File:line | Finding | Suggested fix |
|---|---|---|---|---|
| W-1 | warn | `scripts/release.sh:306-309` | R4's `propose-fix-pr` executor half is not implemented: the ruling made the executor create a branch off `origin/master`, apply the change "only if it applies cleanly", open a **draft PR**, then stop. The code only posts a checklist comment and stops: `sed -n '306,309p' scripts/release.sh`: `propose-fix-pr)` / `issue_comment "$phase" propose-fix-pr "The AI proposed a fix PR for phase $phase."$'\n'"diagnosis: $diagnosis"$'\n'"Open the fix PR off origin/master (never release/$tag); the release resumes only after it merges."` / `do_stop "$phase" "propose-fix-pr recorded on the checklist issue: open the fix PR off origin/master; resume after it merges" "$log"`. Disclosed (handoff-F §Deviations 4); defensible because the answer schema `{diagnosis,action,argument,reason}` carries no patch to apply — but the ruling and the code now disagree. | O amends R4 to the simpler contract (record + stop) or opens follow-up work for an executor-side draft-PR path; either way journal it. |
| W-2 | warn | `scripts/release.sh:285-289, 517-522` | R10's **[W] wait** class is implemented as stop-or-single-pause, not poll: AI `wait` = one bounded 60 s sleep then re-run (`sed -n '285,289p'`: `wait)` / `say "  .. AI: wait — pausing once (bounded), then re-running the phase"` / `sleep 60` / `continue`), and incomplete CI check-runs / a stale PR-Agent head evaluate as a deterministic stop (`do_stop review "hard stop: CI is not green…"`) rather than a poll loop. Disclosed (handoff-F §Deviations 3); T hedged the class as unsteered (decisions.md:156-158). The safety property (never proceed without green evidence) holds; the ergonomics differ from the ruling's "wait (poll, known)". | Same as W-1: amend R10's [W] semantics or schedule the poll loop; until then the resume command is the documented recovery. |
| W-3 | warn | `scripts/release.sh:131-142` | R11's letter ("search-before-post") is replaced by marker + state-tracked posting: each (phase, kind) comment posts once when its phase first completes, enforced by `phase_done`, not by listing existing issue comments. Marker format matches the ruling verbatim — `sed -n '136,136p'`: `printf '<!-- release.sh: %s %s %s -->\n' "$tag" "$phase" "$kind"` — and the pinned observable (no duplicate comment first-lines across kill/resume) is asserted (`release.test.sh:475-478`). Disclosed (handoff-F §Deviations 2). Residual: if `state.json` is deleted (R6's documented recovery), a phase that posted but was not yet `done` re-derives and may re-post its comment — cosmetic, and state-is-a-cache is R6's own rule. | Accept as-is or add a `gh issue view --json comments` dedupe probe in a follow-up; not worth blocking. |
| N-1 | nit | `scripts/release.sh:173` vs `scripts/release-gates.sh:49` | The CI pass predicate is stated in two places. `sed -n '173,173p' scripts/release.sh`: `bad="$(printf '%s' "$tsv" | awk -F'\t' '$2 != "completed" || $3 !~ /^(success|skipped|neutral)$/ { print $1 " (" $2 " " $3 ")" }')"` vs `sed -n '49,49p' scripts/release-gates.sh`: `elif printf '%s' "$runs" | awk -F'\t' '$2 != "completed" || $3 !~ /^(success|skipped|neutral)$/ { bad = 1 } END { exit !bad }'; then`. This was **plan-directed** (R1(a): "same predicate as `release-gates.sh:49`"; R12: zero wrapped-script modifications), and `release-gates.sh` cannot serve the PR-head/merge-commit probes (it requires the tag to exist — `release-gates.sh:29-32`), so no existing mechanism was bypassed. If the conclusion pass-set ever changes, both sites must change. | Follow-up only, per brief item 1's minimal-extension clause: a `--check-sha` probe on `release-gates.sh`, or extract the predicate; do not do it in this run. |
| N-2 | nit | `docs/releasing.md:236-245` | The pre-existing §"Automated release (draft)" has its own "What stays manual, on purpose:" list scoped to `release-local.sh` (steps 1-10, 13, 17) and says of steps 1-10: "that is a later phase of [#31]" — future tense that this story has now realized (`release.sh` phases 3-9 are exactly steps 1-10). No contradiction: the new §"The operator" (releasing.md:10-36, "said once, here" at :29) is scoped to `release.sh`, the old list to `release-local.sh` as a standalone tool, which it still is. | Post-merge docs pass: re-scope the old list to release-local.sh-the-tool and drop the stale #31 future tense. |
| N-3 | nit | `.aftersight/pipeline.config.json:10-15` (diff) | Beyond the brief's "This is the only `.aftersight/` change in the diff" (the two harness lines in `test.unit.command`), the config also sets `pluginSuite: null` / `pluginTypecheck: null`. Operator-made and journaled twice (decisions.md "T-green rework 1/2"), remediated through the diff gate (round-2 PASS with the handoff-F addendum inventory). Not F drift and not a parallel path — the nulls disable npm-run defaults that cannot exist in kai (no root package.json) — recorded here so the a-dup record is complete. | Nothing; disclosure stands. |
| N-4 | nit | `scripts/release.sh:225,243-252` | The known-blocker attempt counter is shared across blocker kinds: one `handlers` variable caps quit-kai and regen-bindings attempts together (`sed -n '243,244p'`: `handlers=$((handlers + 1))` / `[[ $handlers -le 2 ]] || do_stop "$phase" "Kai is still running after two quit attempts"` and again at 251-252 for bindings). R10's table treats the four blockers independently; a Kai-quit followed by a bindings regen in one phase would hit the shared cap of 2. Already on the diff-gate round-2 post-merge hardening list. | Follow-up: per-kind counters if the mixed-blocker case ever fires in rehearsal. |

No parallel path was found: every concern the Reuse map named an existing asset for is served by
that asset (see the confirmation list below), and every piece of new logic in `release.sh`
(phases 3-9 orchestration, PR/merge/tag probes, PR-Agent gate, postcondition probing, state,
stop reporting) has no prior art in `scripts/` — it is the sanctioned new object (survey "New"
items 1-5; R10's table).

## Reuse confirmation (the dispatch's six checks)

1. **`release.sh` wraps, never reimplements.** Version bump: `sed -n '443,443p'` =
   `phase_run bump "release-bump.sh $ver" run_tool release-bump.sh "$ver"` — and `release.sh`
   contains no version-file edit (`grep -nE 'config\.yml|Info\.plist|CFBundle' scripts/release.sh`
   → none). Gates/verify/package/notes/draft: the whole span is one call — `sed -n '602,602p'` =
   `phase_run local "release-local.sh $tag $mode" run_tool release-local.sh "$tag" "$mode"` — and
   `release-local.sh` still owns that logic itself (`release-gates.sh` at :97, `release-notes.sh`
   at :112, `release-verify.sh` at :138, `release-package.sh` at :141, `gh release create` at
   :157). `release.sh` never calls `release create`, `release delete`, or edits version files;
   its only release write is the R10-p12 canonical publish `gh release edit … --draft=false`
   (`release.sh:646`). Postcondition probes only *read* what the wrapped scripts leave
   (`gh release view … .assets | length` at :595, disk checks at :588).
2. **`release-ask-ai.sh` is the single AI seam.** Its only invocation in `release.sh` is
   `release.sh:266`: `answer="$(run_tool release-ask-ai.sh "$phase" "$desc" "$log" 2>/dev/null)"`;
   `grep -iE 'opencode|anthropic|claude|llm|curl' scripts/release.sh` → none; the only `opencode`
   invocation in the entire diff is `release-ask-ai.sh:60`:
   `if ! timeout 300 opencode run --format json "$msg" >"$out" 2>/dev/null; then`. The harness
   fakes `release-ask-ai.sh` itself (`release.test.sh:189-201`) rather than letting any AI
   reachable from `release.sh`.
3. **State = jq + `state.json` per R6; no second marker system.** All writes go through
   `st_update` (`release.sh:83-87`: jq → `state.json.tmp.$$` → same-dir `mv`); lock is
   `mkdir .lock` + owner PID (:59-78); posture escalation-only (:105-111); corrupt state stops
   with the R6 wording (:100-104). The only marker file is the A6-sanctioned owner-created
   `bin/release/kai-release-key.backed-up` probe (:327) — a needs-check, not a state channel.
   `logs/` holds phase logs, not state.
4. **Harnesses reuse the `release-preflight.test.sh` shape.** Same assertion helpers and ok/FAIL
   cadade (`release-preflight.test.sh:29-31` vs `release.test.sh:18-21` /
   `release-ask-ai.test.sh:18-20`, verbatim-identical `has/hasnt/exit_is` bodies), throwaway git
   repos (`mkrepo` → `mkscn`, `run` → `rrun`), env-steered fake `gh`, and the fake-`git`-
   wrapping-real-`git` pattern (`release.test.sh:58-62`: `printf '%s\n' "\$*" >>"\$FAKE_STATE_DIR/git.log"` /
   `exec $RG "\$@"`); `release.test.sh` even upgrades origin to a local bare repo so real
   push/ls-remote run with zero network. No new test framework, no sourcing, no shared lib — the
   repo's standalone-harness convention, extended additively (`count_is/match_is/val_is/jq_is`).
5. **No test-case duplication between the two harnesses.** Distinct SUTs: `release.test.sh`
   never fakes `timeout`/`opencode` and treats `release-ask-ai.sh` as a fakeable black box;
   `release-ask-ai.test.sh` never runs `release.sh`. No assertion about the consultation
   envelope (argv shape, last-text-part, fail-closed cases A02-A09) appears in
   `release.test.sh`, and no phase-orchestration assertion appears in `release-ask-ai.test.sh`.
   The ~10 shared helper lines are the same boilerplate every standalone `scripts/*.test.sh`
   already carries — convention, not a parallel framework.
6. **Docs say each thing once (per scope).** The operator story lives once in
   `docs/releasing.md:10-36` §"The operator", with the manual remainder pinned by
   `releasing.md:29`: "What stays manual, on purpose — said once, here: the **manual smoke
   matrix** … and the **publish go-ahead**". The checklist header names the two owner steps at
   the point of use and defers (`release-checklist.md:12-16`: "The steps that stay yours: … See
   \"Release notes preface\" and \"The operator\" in docs/releasing.md"), the "Step by step"
   cross-ref (:377) points up rather than restating, and `release.sh`'s help header explicitly
   declines to restate the governing rules (`release.sh:5-6`: "the governing rules live in
   docs/releasing.md and docs/release-checklist.md and are not restated here"). The one
   pre-existing overlap is N-2, scoped to `release-local.sh`, not new.

## Ruling-drift sweep (R1-R12, beyond the findings above)

R1 boundary 90 passes (`release.sh:195`: `"$score" -lt 90` fails ⇒ 90 passes) with
complete@head and zero non-RESOLVED findings (:189-196) — implemented as ruled. R2 satisfied by
the harness scenarios (release.test.sh:435-478). R3 cap-2/one-per-event (:262-265; tests
:514-560). R4 hard-stop-before-consult (:233-238 precede :258+; review/merge phases never
consult) — executor-performs-mutations holds everywhere except W-1's disclosed de-scoping.
R5 all five stops present; tag-safety is structural (`grep -E 'tag (-d|--delete)|--force|\+refs/tags' scripts/release.sh`
→ none; the tag phase refuses to move an existing tag, :566-573; negatively asserted by
`release.test.sh:762-768`). R6 fully implemented (above). R7 bare-name dispatch via
`run_tool` (:121-124, PATH-first with sibling fallback — semantically the ruling's
command-not-found-prepend). R8 postures exact: `--dry-run` exits after `local` (:716-726),
default stops after smoke (:730-736), `--yes` always passed in default/publish (:585-586).
R9 exact (`release-ask-ai.sh:60-69`, single-argv message, last text part, fail-closed on
non-zero/unparseable/no-text/absent binary). R10 all 15 phases in order (:704-740) with
postcondition-first probes (e.g. restructure :464, local :587-601, publish :639-644).
R11 → W-3. R12 **upheld**: `git diff --name-status origin/master..HEAD -- scripts/` shows only
the four new files (`A scripts/release-ask-ai.sh`, `A scripts/release-ask-ai.test.sh`,
`A scripts/release.sh`, `A scripts/release.test.sh`) — zero wrapped scripts modified.

## Notes for F

None required (PASS). The three Warns are already disclosed in handoff-F §Deviations 2-4; this
gate's ask is that O reconcile the ruling text with the implemented semantics (or schedule the
follow-ups) so the next release's auditor reads one story.

## Patterns referenced

1. `scripts/release-local.sh` — the wrap target (gates :97, notes :112, verify :138, package
   :141, draft :157; the `step()` cadence `release.sh`'s phase headers mirror).
2. `scripts/release-preflight.test.sh:19-31` — the harness analogue both new harnesses copy.
3. `scripts/release-gates.sh:29-32,44-57` — why the PR-head CI probe could not reuse it (tag
   required), and the predicate N-1 duplicates with attribution.
4. `docs/handoffs/0078-issue-78/handoff-A.md` §"Canonical rulings" (R1-R12) — the drift
   baseline.
5. `docs/releasing.md:10-36` + `docs/release-checklist.md:9-16` — the say-it-once docs baseline.
