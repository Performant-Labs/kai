# Handoff-A-dup: Phase 7 - #126 installer clears the Accessibility grant on unchanged signing identity  (anti-duplication gate)

**Date:** 2026-11-05
**Branch:** issue-0126-issue-126 (.worktrees/0126-issue-126)
**Diff base:** 93542b1   **Diff head:** working tree (HEAD 93542b1 + staged/unstaged/uncommitted; `docs/handoffs/**` withheld from the patch)
**Reuse map:** docs/handoffs/0126-issue-126/survey.md (issue-as-survey per ADR 0027 — no separate Reuse map; the brief and handoff-A name `scripts/install-release.sh` as the object)
**Verdict:** PASS

## Summary
F extended the analogous object exactly as the plan specified: the decision site in `scripts/install-release.sh` step 4 was **replaced in place** (the `shasum` binary-hash rule is gone, not kept beside the new designated-requirement rule — A's "subsumed" note), step 6's keep-branch got the brief's verbatim message with the `--keep-permissions` and reset branches untouched, and the tests grew inside the existing paired harness `scripts/install-release.test.sh` rather than a new tier. No parallel path exists anywhere in the diff. No duplication; extension is clean.

## Findings
No duplication; extension is clean.

Extension points verified against the plan (handoff-A.md) and neighbors:

- **`reqof` helper (`install-release.sh:54`)** — reuses the exact parsing technique from `sign-check.sh:29` (`codesign -dr - | sed -n 's/^#* *designated => //p' | head -1`), duplicated inline. This is the sanctioned shape: handoff-A explicitly rejected extracting a shared shell library ("every script there is standalone with a paired `.test.sh`"). The one deliberate divergence from `sign-check.sh` — capturing `2>&1` and appending `|| true` so an unreadable signature yields an empty string instead of an error exit — implements A finding 1's fail-safe (empty ⇒ changed ⇒ reset) and mirrors the file's existing `|| true` idiom (`PlistBuddy` at :123, `xattr` at :74/:142, `tccutil` at :155). It is documented in the helper's comment. Not a parallel path.
- **Step 4 (`install-release.sh:128-137`)** — the new rule lives in the same site, same `changed` variable, same default `changed=1`. The ad-hoc case falls out of plain string equality (a `cdhash H"…"` requirement never equals a certificate requirement) exactly as the plan predicted — no second classifier was added alongside `sign-check.sh`'s. The guard change from `[[ -x "$target/Contents/MacOS/Kai" ]]` to `[[ -d "$target" ]]` follows the comparison's unit moving from executable to bundle, and is behavior-preserving in both edge directions (no old app ⇒ changed; corrupt/unsigned old app ⇒ empty requirement ⇒ changed).
- **Step 6 (`install-release.sh:150-158`)** — brief's verbatim `same signing identity: the Accessibility grant is still valid`; the `--keep-permissions` branch, the reset message, and `tccutil … || true` are byte-identical to before.
- **Header comment (`install-release.sh:17-20`)** — reworded in place, same line count, inside the unchanged `sed -n '2,23p'` help range (A finding 2, first item). `docs/releasing.md:157-158` reworded exactly as finding 2's second item prescribed. No other prose went stale.
- **Tests (`install-release.test.sh`)** — extended the existing harness: `mkapp` gained a backward-compatible optional 4th parameter, the new cert fixture and the five new cases use the established `run`/`has`/`hasnt` style, and the file stays wired via `test-unit.sh:10`. The throwaway-keychain `mkcert` fixture is a documented "miniature `scripts/release-cert.sh`": reusing `release-cert.sh` itself was not an option — it targets the *login* keychain, refuses duplicate identities, and has interactive first-use semantics, all of which would break the harness's isolation guarantee ("never touches … your keychain", now also stated in the header). The duplication is confined to the openssl config stanza, is the minimum needed for hermeticity, and matches the headless-verification recipe documented in `docs/dev-signing.md:136-141`. Deliberate and reviewed, not drift.
- **No drive-by edits** — the only other changed path, `docs/handoffs/decisions.md`, is O/T's own pipeline decision log under the withheld `docs/handoffs/**` pattern, not F source work. The reset branch, quarantine logic, `gh` download path, and argument handling are untouched.
- **Repo-specific gates** — none apply: no migrations, no DB harness, no workspace crossings, no Fastify root hooks (the change is confined to `scripts/` + one docs line in a Go/Wails repo).

No rework rounds occurred, so there is no rework-introduced drift to re-flag.

## Notes for F
None required (PASS). For the record, one observation below threshold: `reqof`'s two-line extraction now exists in both `install-release.sh` and `sign-check.sh`. That is the pattern the plan review approved for this repo's standalone-script cadence; if a third consumer ever appears, that would be the moment to reconsider a shared helper — not now.

## Patterns referenced
- `scripts/install-release.sh` — the extended object (step 4 decision, step 6 messages, header comment)
- `scripts/sign-check.sh:27-32` — the designated-requirement parsing technique `reqof` reuses
- `scripts/install-release.test.sh` — the paired-test harness the new cases live in
- `scripts/release-cert.sh:44-63` — the cert-creation stanza the test fixture miniatures (login-keychain target makes direct reuse impossible)
- `docs/handoffs/0126-issue-126/handoff-A.md` — the reviewed plan (findings 1-2, "subsumed" and standalone-script notes) this diff implements
