# Handoff-T-green: Phase 6 - #126 installer clears the Accessibility grant on unchanged signing identity

**Date:** 2026-11-05
**Branch:** issue-0126-issue-126 (.worktrees/0126-issue-126)
**Issue:** #126
**Handoff-F reviewed:** docs/handoffs/0126-issue-126/handoff-F.md
**Handoff-T-red:** docs/handoffs/0126-issue-126/handoff-T-red.md

## GREEN confirmation

`bash scripts/install-release.test.sh` (worktree root, single invocation — not back-to-back, per
F's flake note): **53 ok, 0 FAIL, `PASS`, exit 0.** All three previously-RED assertions are green:

```
ok   the same build again keeps the grant
ok     ... and does not clear it
ok   a certificate-signed build installs
ok   the same certificate on a new build installs
ok     ... and keeps the grant
ok     ... and does not clear it
```

and every green guard from RED stayed green (`a different certificate clears the grant`,
`an ad-hoc-signed old build still clears the grant`, `an old requirement that cannot be read
clears the grant`, `a first install clears the old grant`, `--keep-permissions leaves the grant
alone`).

**Do the tests still fail if the behavior is removed?** Yes — verified by mutation, on *copies*
in a throwaway temp dir (worktree untouched; the harness resolves
`inst="$here/install-release.sh"`, so a relocated script+test pair runs as-is):

- **Mutation 1 — restore the old binary-hash rule (the #126 bug itself):** exactly
  `FAIL ... and keeps the grant (wanted /same signing identity: the Accessibility grant is still valid/)`
  and `FAIL ... and does not clear it (did not want /was cleared/)`; suite `FAILED`. Instructively,
  `the same build again keeps the grant` still *passes* under this mutation (identical binaries ⇒
  identical hash ⇒ keep) — proving the cert-matrix test is the load-bearing one and that it and
  the same-build message test pin different paths, not duplicates.
- **Mutation 2 — invert the fail-safe (unreadable old requirement ⇒ "unchanged"):** exactly
  `FAIL an old requirement that cannot be read clears the grant (wanted /was cleared/)`; suite
  `FAILED`.

Each mutation trips only its own assertion — the suite fails **in isolation, for the right
reason**, and pins behavior (the grant decision), not implementation.

## Tier 1 results

| Check | Command | Expected | Actual | Result |
|---|---|---|---|---|
| Focused suite | `bash scripts/install-release.test.sh` | PASS, exit 0 | 53 ok, `PASS`, exit 0 | PASS |
| Full seed suite | `bash scripts/test-unit.sh` | exit 0 (swiftbridge build, `go test ./internal/... ./pkg/...`, `pnpm --dir frontend test`, `release.test.sh`, `release-ask-ai.test.sh`, `install-release.test.sh`) | ran to its final line under `set -euo pipefail` with the last suite printing `PASS` | PASS |
| Syntax | `bash -n scripts/install-release.sh` | no output | no output | PASS |
| CLI smoke | `scripts/install-release.sh --help` | well-formed, reworded identity line, unchanged shape | correct 22-line header, new wording at the "What it does" line, same line count (diff is 2 lines → 2 lines) | PASS |
| Lint on changed files | (none configured — no shell lint in `test-unit.sh` or CI; CI's actionlint explicitly ignores shellcheck notes) | n/a | n/a; ad-hoc `shellcheck` flags only pre-existing SC2010 (line 93, untouched by this change) | PASS (n/a) |
| Server start / API smoke | n/a — no server in this repo (Go/Wails app; the change is a shell script). The suite *is* the smoke: 20+ real installer invocations against real fake apps/zips/dmg/fake-`gh` | n/a | n/a | N/A |

Cross-check vs F's self-check: F reported 9/10 PASS standalone + `test-unit.sh` exit 0 + `bash -n`
ok + `--help` well-formed. My results match on every command re-run. F's one unreproducible flake
did not recur here (2 green invocations of the suite, spaced; see Advisory notes).

## Tier 2 results

| Check | Method | Result |
|---|---|---|
| Coverage per acceptance criterion | Mapping below — every "Wanted" and automatable "Done when" bullet has a passing test from RED | PASS |
| Test quality (name a behavior / fail in isolation for the right reason / cheapest sufficient tier / no duplication / assert behavior not implementation) | Names are behavior sentences; isolation verified by the two mutations above; tier = existing integration harness (a fake `codesign` could not catch a wrong invocation — this *is* a codesign-orchestration feature); no duplication (mutation 1 passes the same-build test while failing the cert test — distinct paths); proportionate: 6 touched assertions + 3 fixtures in an existing 53-assertion file, nothing speculative added | PASS |
| Type safety | `git diff HEAD --numstat` (excluding handoffs): only `scripts/install-release.sh` (+16/−8), `scripts/install-release.test.sh` (+93/−9), `docs/releasing.md` (+3/−2). No Go/TS source touched — nothing type-bearing changed | PASS (n/a) |
| Error handling | Invalid inputs still gated and tested (wrong bundle id, tampered binary, checksum mismatch, non-archive, missing/empty `--from`/`--dest`); the new error-shaped path — old signature unreadable — is tested (`an old requirement that cannot be read clears the grant`), and `reqof`'s `\|\| true` means a failing `codesign -dr` cannot kill the installer under `set -euo pipefail` (exercised by the stripped-signature fixture) | PASS |
| Data integrity | No DB in scope; filesystem analogue verified: nothing-installed assertions after refusals still pass; step 5 replace logic untouched | PASS |
| API contract | The contract here is CLI output; `has`/`hasnt` pin the brief's verbatim keep message, the untouched reset and `--keep-permissions` messages, and `--help`'s shape is byte-stable apart from the reworded line (verified by running it) | PASS |
| Security | Signature-verify, checksum, and bundle-id gates untouched and still tested; `tccutil reset` still scoped to the overridable test bundle id (tests never touch the real grant); `reqof "$1"` fully quoted, no injection surface; cert fixtures live in a per-run throwaway keychain deleted by the cleanup trap | PASS |
| Migration safety | N/A — no migrations (script-only change) | N/A |
| Playwright / e2e | No `e2e/` workspace exists in this repo — N/A | N/A |

## Acceptance criteria status

| Criterion (brief) | Status | Backed by |
|---|---|---|
| Reset only when the designated requirement actually changed (compare `codesign -dr -` of old and new) | PASS | `the same certificate on a new build installs … and keeps the grant … and does not clear it`; `a different certificate clears the grant`; mutation 1 (hash rule restored) fails the first of these |
| Say so when it keeps the grant (`same signing identity: the Accessibility grant is still valid`), and keep `--keep-permissions` working | PASS | Message asserted in both the same-cert and same-build cases; `--keep-permissions leaves the grant alone` green (and ordered first in step 6, diff-verified) |
| Old ad-hoc-signed app (code-hash requirement) ⇒ keep the reset | PASS | `an ad-hoc-signed old build still clears the grant` (falls out of plain equality, no classifier — as A predicted) |
| Done when: test covers same identity / changed identity / ad-hoc→certificate | PASS | The three cases above, in one matrix, plus A-finding-1's fail-safe (`an old requirement that cannot be read clears the grant`) and first-install reset (pre-existing) |
| Done when: checked on a real update, release build to release build, on a Mac | **Not automatable headless** (declared out of scope in RED; `docs/dev-signing.md:143-147` — TCC cannot be exercised headlessly). Remains a manual check for **O** with two real release builds. Non-blocking. | — |

Also verified beyond the brief's bullets (A's PASS findings, folded into RED): finding 1 fail-safe
default — tested + mutation-2-verified; finding 2's two stale prose lines — both reworded (script
header line confirmed in live `--help` output; `docs/releasing.md:158` confirmed in the diff).

## Blocking issues

None.

## Advisory notes

1. **F's one-off flake not reproduced.** F saw 1 FAILED in 10 standalone runs and hypothesized the
   `hdiutil info` before/after mount-count assertion racing a *previous* run's trap-cleanup detach
   when runs are invoked back-to-back. I ran the suite twice (spaced, once standalone, once inside
   `test-unit.sh`) — both green. F's mitigation advice (don't invoke back-to-back; rerun once)
   is sensible; the mount-count assertion is the place to look if it ever recurs. Not a defect in
   the changed logic (a pure string comparison over local files).
2. **Pre-existing SC2010** (`ls | grep` at `scripts/install-release.sh:93`) — flagged by an ad-hoc
   shellcheck run, untouched by this change and not a repo gate. Housekeeping only.
3. **Manual "Done when" bullet 2** (real TCC grant surviving a real update, real release builds)
   is the one acceptance item no automated tier can cover. Recommend O schedules it at the next
   real release (0.6.x → 0.6.x+1) and confirms the installer prints the keep message.
4. Step 4's old-app predicate widened from `-x "$target/Contents/MacOS/Kai"` to `-d "$target"`:
   safe — a non-app directory at `$target` yields an empty requirement ⇒ changed ⇒ reset (the
   fail-safe), and step 5 `rm -rf`/replace behavior is unchanged.
