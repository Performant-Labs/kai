# Handoff-T-red: Phase 4 - #126 installer clears the Accessibility grant on unchanged signing identity

**Date:** 2026-11-05
**Branch:** issue-0126-issue-126
**Brief / wireframe reviewed:** docs/handoffs/0126-issue-126/brief.md (wireframe: N/A — no UI surface); A's plan review docs/handoffs/0126-issue-126/handoff-A.md (PASS)

## A precondition
Confirmed: A returned PASS on the plan (Phase 3). Findings 1 (fail-safe default) and 2 (stale prose in the script header / `docs/releasing.md`) are folded into this suite and the notes below.

## Tests authored

All tests live in the existing paired harness `scripts/install-release.test.sh` (wired into the suite via `scripts/test-unit.sh:10`). Tier: integration — the harness drives the real installer script against real fake apps, real `codesign`, real zips; this is the cheapest tier that can exercise the decision at all, and it matches the file's established cadence. No new file was needed.

Fixture additions (mine, test-only):

- **Throwaway-certificate fixture**: a miniature `scripts/release-cert.sh` — two self-signed code-signing certificates ("Kai Test A"/"Kai Test B") made with `/usr/bin/openssl` and imported into a throwaway keychain created per run and deleted by the cleanup trap. Nothing touches the login keychain. Verified feasible headless before authoring (see Evidence); this matches A's note citing `docs/dev-signing.md:136-141`.
- **`mkapp` extension**: optional 4th argument = certificate name → the fake app is certificate-signed instead of ad-hoc. All existing call sites unchanged.
- **Three cert-signed zips**: 1.0.0 and 2.0.0 signed by cert A (same identity, different code), 2.0.0 signed by cert B (different identity). Verified on this Mac: same cert + different binaries ⇒ **identical** designated requirement (`identifier "test.kai.installer" and certificate root = H"…"`); different certs ⇒ different requirement; ad-hoc ⇒ `cdhash H"…"` requirement.

Tests (assertion = the behavior pinned):

| # | Test | Criterion / behavior pinned | Status before F |
|---|------|------------------------------|-----------------|
| 1 | `a certificate-signed build installs` | A cert-signed (untrusted, self-signed) release build must pass the installer's `codesign --verify --deep --strict` gate and install — the whole matrix below presupposes it | ok (green) |
| 2 | `the same certificate on a new build installs` / `… and keeps the grant` / `… and does not clear it` | **Brief "Wanted" 1 + "Done when" case "same identity"**: same designated requirement on changed code ⇒ grant kept, with the brief's verbatim message `same signing identity: the Accessibility grant is still valid`, and no reset | **RED** (core bug #126: today the binary-hash rule clears it) |
| 3 | `a different certificate clears the grant` | **"Done when" case "changed identity"**: cert A → cert B ⇒ requirement changed ⇒ reset | ok (green guard) |
| 4 | `an ad-hoc-signed old build still clears the grant` | **"Done when" case "ad-hoc to certificate"**: old ad-hoc (`cdhash` requirement) → new cert-signed ⇒ reset — "that grant really is invalid for the new build" | ok (green guard) |
| 5 | `an old requirement that cannot be read clears the grant` | **A finding 1 (fail-safe)**: old install's signature unreadable (fixture: `codesign --remove-signature`) ⇒ treated as changed ⇒ reset, like a first install. Do not invert this default | ok (green guard) |
| 6 | `the same build again keeps the grant` (updated assertion) | **Brief "Wanted" 2 message contract**: identical build ⇒ the keep message is now the brief's `same signing identity: …` wording (old `same build as the one that was installed` message is superseded — identical binaries ⇒ identical requirements, so A's "subsumed" note applies) | **RED** (message does not exist yet) |

Existing assertions that already pin the rest of the wanted behavior and were deliberately left untouched: first install with no old app ⇒ reset (`a first install clears the old grant`); `--keep-permissions` never touches the grant (`--keep-permissions leaves the grant alone`, exercised on a would-clear transition); ad-hoc→ad-hoc different build ⇒ reset (`… and clears the stale grant`). Not re-pinned — that would duplicate.

Out of scope for automated tests, by design: the brief's "Done when" second bullet (a real macOS TCC grant surviving a real update) — TCC cannot be exercised headlessly (`docs/dev-signing.md:143-147`); that check belongs to the operator with two real release builds. A finding 2's prose updates (script header inside `--help`, `docs/releasing.md:157-158`) are F's blast radius, not a test surface (no automated gate pins that wording).

## RED confirmation

Run command, twice (deterministic), from the worktree root:

```
bash scripts/install-release.test.sh
```

Exact failing output (identical on both runs; suite exits 1):

```
FAIL the same build again keeps the grant (wanted /same signing identity: the Accessibility grant is still valid/)
ok     ... and does not clear it
ok   a new build replaces the old one
ok     ... and clears the stale grant
ok   --keep-permissions leaves the grant alone
ok   a certificate-signed build installs
ok   the same certificate on a new build installs
FAIL   ... and keeps the grant (wanted /same signing identity: the Accessibility grant is still valid/)
FAIL   ... and does not clear it (did not want /was cleared/)
ok   a different certificate clears the grant
ok   an ad-hoc-signed old build still clears the grant
ok   an old requirement that cannot be read clears the grant
FAILED
```

Every other assertion in the file (48 ok) passes, including all pre-existing ones — the fixture changes broke nothing. Each failure is the feature assertion itself (the identity-based keep decision and the brief's keep message), not an import/setup error: the cert fixtures demonstrably build, install, and verify (`a certificate-signed build installs` ok) in the same run.

## Ready for F
Confirmed RED is valid; F may implement against these tests. Contract in one line: replace the step-4 binary-hash comparison in `scripts/install-release.sh` with a comparison of the old and new designated requirements (`codesign -dr -`, parsing as in `sign-check.sh:29`), defaulting to changed/reset when there is no old app or its requirement is unreadable, keeping `--keep-permissions` first, and printing `same signing identity: the Accessibility grant is still valid` when the grant is kept (the reset message and `--keep-permissions` message stay as they are). Remember A finding 2's two stale prose lines (script header comment, `docs/releasing.md`) in the same change.
