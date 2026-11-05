# Handoff-A: Phase 3 - #126 installer clears the Accessibility grant on unchanged signing identity  (up-front plan review)

**Date:** 2026-11-05
**Branch:** issue-0126-issue-126
**Brief reviewed:** docs/handoffs/0126-issue-126/brief.md   **Reuse map:** docs/handoffs/0126-issue-126/survey.md (issue-as-survey per ADR 0027 — no separate Reuse map; the brief itself names `scripts/install-release.sh` as the object)   **Wireframe:** N/A (no UI surface)
**Verdict:** PASS

## Summary
PASS. The plan extends the right object at the right layer: `scripts/install-release.sh` step 4 (the `changed` decision, lines 123-129, currently a binary `shasum` comparison that flips on every build) plus step 6's messages (lines 141-150) is the only Accessibility-grant-reset site in the repo — the sole executable `tccutil` is `install-release.sh:147`; all Go/TS Accessibility references (hotkey manager, swiftbridge, settings UI) only detect or request the permission, and the in-app updater has no permission logic. Comparing designated requirements instead of binary hashes has direct precedent in `scripts/sign-check.sh:27-32`, and the change stays inside the scripts/ standalone-script-with-paired-test cadence (`install-release.test.sh`, wired via `test-unit.sh:10`). Two warn-level notes: the fail-safe when the old app's requirement is unreadable is unspecified, and two prose descriptions of the old decision rule (script header — which is inside the `--help` output — plus `docs/releasing.md`) go stale with the fix.

## Findings

| # | Severity | Plan element | Drift dimension | Finding | Suggested fix |
|---|---|---|---|---|---|
| 1 | warn | "compare `codesign -dr -` of the old and the new app" | cross-cutting concerns | The brief does not specify the fail-safe when the OLD app's designated requirement cannot be read (corrupt/stripped signature, or `codesign -dr` failing on an old install). Two adjacent defaults must stay as they are: `changed=1` when no old app exists (`install-release.sh:124` — first install resets; pinned by the test at `install-release.test.sh:64`) and `|| true` on the reset (`:147`). | Pin the fail-safe in the brief: unreadable old requirement ⇒ treat as changed ⇒ reset (today's behavior). Same when there is no old app. Do not invert the default. |
| 2 | warn | behavior change in the installer | pattern consistency | Two prose descriptions of the old decision rule go stale: the script's header comment ("only if the app's code actually changed, resets the now-invalid Accessibility grant", `install-release.sh:17-20` — inside the `-h|--help` output range `sed -n '2,23p'`, so user-visible) and `docs/releasing.md:157-158` ("clears the stale Accessibility grant **only if the app's code changed**"). No automated gate pins that wording (checked `release.sh`'s standing-lines usage — advisory only), so nothing breaks, but both would contradict the script. | Include the header-comment line and the one `docs/releasing.md` line in the change's blast radius, reworded to "only if the signing identity (designated requirement) changed". |

Notes (not findings):

- The brief's ad-hoc case ("old app's requirement is a code hash ⇒ keep the reset") falls out of a plain string comparison of the two requirements — an ad-hoc `cdhash H"..."` requirement never equals a certificate requirement — so no separate classifier is needed. If F wants explicit ad-hoc detection anyway, the established test is `sign-check.sh:32` (`*cdhash*` and not `certificate leaf`/`anchor ...`).
- The step-4 binary-hash comparison (`shasum` of the old/new executable, `:126-128`) is subsumed by requirement comparison (identical binaries ⇒ identical requirements). Replacing rather than adding a second check is the cleaner shape; either is acceptable.
- Requirement parsing should reuse the known-good technique from `sign-check.sh:29` (`codesign -dr - 2>&1 | sed -n 's/^#* *designated => //p' | head -1`). Duplicating that two-line extraction inside `install-release.sh` is consistent with the scripts/ cadence — every script there is standalone with a paired `.test.sh`, and there is no shared shell library; extracting one for this would be over-abstraction relative to neighbors.
- Message cadence matches: the brief's "same signing identity: the Accessibility grant is still valid" mirrors the existing one-liner at `install-release.sh:145`; the existing reset message at `:148` ("This build has a new signature...") becomes accurate after the fix. The `--keep-permissions` branch (`:142-143`) is untouched by the plan.
- Test placement is the existing `scripts/install-release.test.sh` harness (`mkapp` ad-hoc-signs fake apps; `run`/`has`/`hasnt`/`exit_is`/`truth` helpers; fake `gh` on PATH). The brief's three cases map onto it: ad-hoc fixtures are `mkapp`'s default, and a headless throwaway self-signed-cert fixture for the same-identity/changed-identity cases is documented as verified feasible in `docs/dev-signing.md:136-141`. Fixture design is T's; noted only because the surface exists.
- The brief's "Done when" second bullet (check on a real update that macOS keeps the grant) matches `docs/dev-signing.md:143-147`, which states TCC cannot be exercised headlessly — consistent, not drift.
- No repo-specific gate applies: no migrations, no DB harness, no Fastify root hooks, no workspace crossings — the repo is a Go/Wails app and the change is confined to `scripts/install-release.sh` + `scripts/install-release.test.sh` (+ the two prose lines above).

## Notes for O
None required (PASS). Optionally fold findings 1-2 into the brief as explicit bullet points so F and T pick them up without reading this handoff.

## Patterns referenced
- `scripts/install-release.sh` — the object to extend (step 4 decision, step 6 messages, header comment, fail-safe defaults)
- `scripts/sign-check.sh:27-44` — established designated-requirement parsing and ad-hoc/Developer-ID/stable-certificate classification
- `scripts/install-release.test.sh` — the paired-test cadence and fake-app/fake-gh harness the new cases must live in
- `docs/dev-signing.md:113-147` — documented semantics of designated requirements across rebuilds, incl. headless verification of cert-signed fixtures and the TCC-can't-be-tested-headlessly caveat
- `docs/releasing.md:150-159` — prose description of the installer's reset rule (finding 2)
