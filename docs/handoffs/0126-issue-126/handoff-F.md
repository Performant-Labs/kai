# Handoff-F: Phase 5 - Implement

**Date:** 2026-11-05
**Branch / worktree:** issue-0126-issue-126 (.worktrees/0126-issue-126)
**Contract:** handoff-T-red.md (tests + the behaviors they pin)

## What I changed

**`scripts/install-release.sh`** — the whole fix; one decision site, one message, one prose line:

- New helper `reqof` (after `die`): prints an app's designated requirement via `codesign -dr - "$1" 2>&1 | sed -n 's/^#* *designated => //p' | head -1 || true` — the exact parsing technique from `sign-check.sh:29`, per A's note. The trailing `|| true` (mirroring the file's existing `PlistBuddy ... || true` idiom) means an unreadable signature yields an **empty** string instead of killing the script under `set -euo pipefail`.
- Step 4 rewritten: `changed` now defaults to 1 and flips to 0 only when an old install exists **and** both requirements are non-empty **and** they are string-equal. This is the fail-safe A finding 1 pinned: no old app ⇒ changed (first install, as before); old requirement unreadable (stripped/corrupt — the `nosig` fixture) ⇒ empty ⇒ changed ⇒ reset; and the brief's ad-hoc case falls out of plain equality (a `cdhash H"…"` requirement never equals a certificate requirement), as A predicted — no classifier added. The old `shasum` binary-hash comparison is replaced, not kept alongside (A: "subsumed").
- Step 6: keep-branch message is now the brief's verbatim `same signing identity: the Accessibility grant is still valid` (satisfies RED tests 2 and 6, including the updated same-build assertion — identical binaries ⇒ identical requirements ⇒ keep). The `--keep-permissions` message, the reset message, and the `tccutil reset … || true` are untouched.
- Header comment (inside `--help`'s `sed -n '2,23p'` range, A finding 2): reworded in place to "only if the signing identity (designated requirement) changed" — same line count, so the help output's shape is unchanged (verified by running `--help`).

**`docs/releasing.md`** (A finding 2's second line): the installer description now reads "clears the stale Accessibility grant **only if the signing identity (designated requirement) changed**".

Nothing else: the reset branch, quarantine logic, `gh` path, and all argument handling are untouched. No test file was modified.

## Self-check

- `bash scripts/install-release.test.sh` — ran it **10 times**: 9× PASS with all 53 `ok`, 1× FAILED (see the flake note below). All three previously-RED assertions green in every passing run: `the same build again keeps the grant` (+ `… and does not clear it`), `the same certificate on a new build installs` (+ `… and keeps the grant`, `… and does not clear it`). The green guards (`a different certificate clears the grant`, `an ad-hoc-signed old build still clears the grant`, `an old requirement that cannot be read clears the grant`, first-install reset, `--keep-permissions`) stayed green.
- `bash scripts/test-unit.sh` (the pipeline's seed command, which includes this file at line 10) — **exit 0**: swiftbridge build, Go tests, frontend tests, and all three script suites pass.
- `bash -n scripts/install-release.sh` — syntax ok. `scripts/install-release.sh --help` — well-formed, new wording, unchanged shape.

Note: the authoritative GREEN is the plugin's T-green run, not this self-check.

**Flake observation for T (harness, not a wrong test):** my 2nd local run printed `FAILED`, but I had piped it through `tail -3` and lost the failing line; 8 consecutive reruns (3–10) plus the full-suite run were entirely green, so I could not reproduce or identify it. The one-off is consistent with the harness's environment-coupled fixtures — most plausibly the `hdiutil info | grep -c image-path` before/after comparison racing with the *previous* run's trap-cleanup detach when runs are invoked back-to-back (my runs were), or a transient in the throwaway-keychain/`codesign` machinery. The changed production logic is a pure string comparison over local files with no environmental coupling. If T-green hits a one-off red, a rerun (or not invoking back-to-back) is the first thing to try; if it recurs, the mount-count assertion is where I'd look.

## Tests I think are wrong

None. The suite's assertions match the brief and A's plan review exactly; the fail-safe (finding 1) and prose lines (finding 2) were folded in as specified.

## Ready for T(green)

Implemented against the RED contract: designated-requirement comparison replaces the binary-hash rule, fail-safe defaults unchanged, brief's keep message, `--keep-permissions` first, both stale prose lines reworded. The plugin should now run the suite: GREEN means the loop advances; RED means I rework.
