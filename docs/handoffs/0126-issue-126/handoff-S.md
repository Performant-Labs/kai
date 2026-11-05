# Handoff-S: Phase 8 - #126 installer clears the Accessibility grant on unchanged signing identity

**Date:** 2026-11-05
**Branch:** issue-0126-issue-126 (.worktrees/0126-issue-126)
**Issue:** #126
**Handoff-T reviewed:** docs/handoffs/0126-issue-126/handoff-T-green.md
**Handoff-A reviewed:** docs/handoffs/0126-issue-126/handoff-A.md, docs/handoffs/0126-issue-126/handoff-A-dup.md
**Handoff-F reviewed:** docs/handoffs/0126-issue-126/handoff-F.md
**Operator-facing report:** N/A (non-visual audit — shell script + paired test; no rendered surface)

## A precondition
Confirmed: A returned PASS on both gates. Up-front plan review (handoff-A.md, `Verdict: PASS`, two warn-level findings later folded in) and the anti-duplication gate (handoff-A-dup.md, `Verdict: PASS`, "No duplication; extension is clean").

## T precondition
Confirmed: handoff-T-green.md reports "Blocking issues: None." Focused suite 53 ok / PASS / exit 0; full `test-unit.sh` ran to completion; mutation testing on copies verified the new tests fail in isolation for the right reason.

## Visual-diff-tool precondition (visual / web projects only)
N/A — the story has no visual surface (a bash installer script and its paired test). Browser and pixel-diff preconditions do not apply.

## Visual diff results (visual / web projects only)
N/A.

## Spec compliance

Spec sources for this run: `docs/handoffs/0126-issue-126/brief.md` (restates issue #126 — gh auth is unavailable in this session, so the GitHub issue body could not be fetched directly; the brief is the run's canonical restatement) plus A's reviewed plan. The aftersight-specific gates in the auditor's standing instructions (docs/adr index, wayfinder scope, CI Proof bar for harness stories) do not exist in this repo — the kai repo has no `docs/adr/`; this is a bug-fix story, not a harness/infra story — so none apply.

| Requirement (brief) | Compliant | Evidence |
|---|---|---|
| Reset the grant only when the designated requirement actually changed (compare `codesign -dr -` of old and new) | YES | `install-release.sh:54` helper `reqof` uses the exact parsing technique from `sign-check.sh:29` (`sed -n 's/^#* *designated => //p' \| head -1`); step 4 (`:128-137`) compares `old_req`/`new_req` strings; the old `shasum` binary-hash rule is **replaced in place**, not kept alongside (A's "subsumed" note). Verified in the diff against base 93542b1. |
| Say so when it keeps the grant (verbatim `same signing identity: the Accessibility grant is still valid`); keep `--keep-permissions` working | YES | `install-release.sh:152` is byte-verbatim from the brief; the `--keep-permissions` branch (`:150-151`) is untouched and ordered first; `--keep-permissions leaves the grant alone` green in T's run. |
| Old ad-hoc-signed app (code-hash requirement) ⇒ keep the reset | YES | Falls out of plain string inequality (a `cdhash H"…"` requirement never equals a certificate requirement) — no classifier added, exactly as A predicted; `an ad-hoc-signed old build still clears the grant` green. |
| Done when: test covers same identity / changed identity / ad-hoc→certificate | YES | The three matrix cases at `install-release.test.sh:150-164`: certA-1.0.0→certA-2.0.0 (same identity, different binaries — the #126 scenario itself), certA→certB (changed identity), ad-hoc→certA. Beyond the brief: A-finding-1 fail-safe (`nosig` fixture, `:167-171`) and first-install reset (pre-existing, `:122`). |
| Done when: checked on a real update, release build to release build, on a Mac | DEFERRED (non-blocking) | Not automatable headless — declared out of scope at RED, confirmed against `docs/dev-signing.md:143-147` ("TCC cannot be exercised headlessly"), carried by T as a manual step for O. See Advisory note 1. |
| A finding 1: fail-safe — unreadable old requirement ⇒ changed ⇒ reset; no old app ⇒ changed; `|| true` on `tccutil` kept | YES | `reqof` captures `2>&1` and appends `|| true` so an unreadable signature yields an empty string under `set -euo pipefail`; step 4 requires **both** requirements non-empty **and** equal for `changed=0`; default `changed=1`; `tccutil … \|\| true` untouched. Mutation 2 (invert fail-safe) trips exactly `an old requirement that cannot be read clears the grant`. |
| A finding 2: reword the two stale prose lines (script header inside `--help`, `docs/releasing.md:157-158`) | YES | Both reworded to "only if the signing identity (designated requirement) changed" in the diff; header stays inside the `sed -n '2,23p'` help range at the same line count (22 lines — re-verified); `docs/releasing.md` line reworded verbatim as prescribed. |

**Spec sanity check (source of truth):** the brief's premise is technically sound per macOS semantics — TCC Accessibility grants are bound to the app's designated requirement, `codesign -dr -` is the standard way to read it, ad-hoc requirements are per-build cdhashes, and the conservative fail-safe direction (unreadable ⇒ reset) is correct. No convention violated; no ADVISORY-HOLD.

## Quality audit

| Area | Result | Notes |
|------|--------|-------|
| API consistency | N/A | No HTTP API; the CLI output contract is consistent: keep/reset/`--keep-permissions` messages follow the existing one-liner cadence, `--help` shape byte-stable apart from the reworded line. |
| Error handling | PASS | Fail-safe default preserved and now *tested* (`nosig` fixture); `reqof`'s `\|\| true` matches the file's existing idiom so a failing `codesign -dr` cannot kill the installer under `set -euo pipefail`; the `[[ … ]] && changed=0` short-circuit keeps the pre-existing safe shape (same construct as before the change, exercised green by the empty-requirement fixture). |
| UI/UX match to spec | N/A | No UI surface. |
| Accessibility | N/A | No UI surface. |
| Architecture gate | PASS | A PASS on both gates; my findings agree with A-dup's extension analysis — no parallel path, decision site extended in place, tests grew inside the existing paired harness. No conflict between my quality findings and A's verdict. |
| Code organization | PASS | One helper (`reqof`) placed after `die` with a comment stating the empty-on-unreadable contract; one decision site; one message; one prose line. Comments explain *why* (grant follows the designated requirement, not the binary hash). |
| Security | PASS | `reqof "$1"` fully quoted, no injection surface; signature-verify/checksum/bundle-id gates untouched and still tested; `tccutil reset` still scoped to the overridable test bundle id; test certs live in a per-run throwaway keychain (`-T /usr/bin/codesign`, partition list scoped to that keychain, deleted by the cleanup trap) — the login keychain is never touched, and the header now says so. Hardcoded `"kai-tests"` password is acceptable for a keychain that lives only for the test's duration. |
| Performance | PASS | Two `codesign -dr` invocations per install — negligible next to the existing download/verify/mount work. Test suite grows by ~seconds (cert generation). |
| Visual regression | N/A | No UI surface. |
| Naming consistency | PASS | `reqof`/`old_req`/`new_req`/`changed` and both messages use the brief's exact terminology ("signing identity (designated requirement)") across script, help, and `docs/releasing.md`. |
| Test quality (test-quality rubric §7) | PASS | See detail below. |

**Test-quality detail.** Per test: all six new/touched assertions name one behavior each, assert user-visible output (`has`/`hasnt` on installer messages — behavior, not implementation), and T's two mutations on copies proved isolation for the right reason (mutation 1 — the #126 bug restored — trips exactly the same-cert keep assertions while the same-build test still passes, proving those two pin *different* paths, not duplicate signals; mutation 2 — fail-safe inverted — trips exactly the `nosig` assertion). Tier: the existing paired-harness integration tier is the cheapest *sufficient* tier — the feature *is* codesign orchestration, so a faked `codesign` could not catch a wrong invocation; no cheaper shell-unit tier exists in this repo. Suite proportionality: a bug-fix story got 6 assertions + 2 fixtures inside the existing 53-assertion file, nothing speculative beyond brief + A's folded findings. Smells: none — no assertion-free or tautological tests, no unreachable outcomes, no mock-shaped assertions, no snapshot padding. No delete/merge findings; the one candidate (`a certificate-signed build installs`, `install-release.test.sh:152`) is diagnostic glue that localizes fixture breakage (throwaway certs rejected by the verify gate) from decision-logic failure — justified, keep. The harness-isolating `mkcert` miniature of `release-cert.sh` is deliberate and reviewed by A-dup (the real script targets the login keychain and would break hermeticity); the duplication is confined to the openssl stanza and matches the recipe documented in `docs/dev-signing.md:136-141`.

## Scope check
Exact match to A's plan and the brief. Diff (base 93542b1 → working tree, handoffs excluded): `scripts/install-release.sh` (+16/−8), `scripts/install-release.test.sh` (+93/−9), `docs/releasing.md` (+3/−2). That is precisely the plan's blast radius: step 4 decision site, step 6 message, `reqof` helper, header comment, the one releasing.md line, tests in the existing harness. No under-delivery. Over-delivery: none — the fail-safe test is A finding 1 (folded into the plan at RED), not scope creep. The reset branch, quarantine logic, `gh` download path, argument handling, and all Go/TS sources are untouched (verified via numstat and the diff). `docs/handoffs/decisions.md` is the pipeline's own log, not F work.

## Verdict

**Verdict:** PASS

All acceptance criteria are met and spec-compliant; quality and security are acceptable; the test suite is verified load-bearing (mutation-tested, isolated, proportionate). The single outstanding "Done when" bullet — confirming on a real Mac that a real release-to-release update keeps the grant — is inherently non-automatable (documented in `docs/dev-signing.md:143-147`), was declared out of automated scope at RED, and is a manual confirmation for O at the next release. It does not block commit. Ready for O to commit.

## Advisory notes
1. **Manual check for O (the deferred Done-when bullet):** at the next real release (0.6.x → 0.6.x+1), run `scripts/install-release.sh` over an existing install and confirm it prints `same signing identity: the Accessibility grant is still valid` and that Privacy & Security still shows Kai granted. This is the only acceptance evidence no automated tier can produce.
2. **F's one-off flake (1 FAILED in 10 back-to-back runs)** did not reproduce in T's spaced runs (2× green, including inside `test-unit.sh`). Most plausible cause is the *pre-existing* mount-count assertion (`install-release.test.sh:56/180`, `hdiutil info | grep -c image-path`) racing a previous run's trap-cleanup detach when invoked back-to-back — unrelated to the changed logic (a pure string comparison). CI invokes suites once, sequentially, so exposure is minimal. If it recurs, that assertion is the place to look.
3. `reqof`'s two-line extraction now exists in both `install-release.sh` and `sign-check.sh` — sanctioned by A for this repo's standalone-script cadence; reconsider a shared helper only if a third consumer appears (A-dup's own note).
4. Pre-existing shellcheck SC2010 (`ls | grep` at `scripts/install-release.sh:93`), untouched by this change and not a repo gate — housekeeping only.
