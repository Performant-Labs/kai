# Release checklist (template)

**This is the reusable template. It carries no version number.** Copy this file's content into a
new GitHub issue titled `Release checklist: vX.Y.Z` each time a release is cut, and fill in that
copy. Never check boxes here. The reasoning behind each step is in
[`docs/releasing.md`](releasing.md).

## Pre-flight

- [ ] 1. **Run `scripts/release-preflight.sh vX.Y.Z`** on the release Mac, in a clean checkout of `master`
  - Machine: Apple Silicon macOS, the required tools, Go at least `go.mod`'s version, Kai not running
  - Commit: `origin` is the fork (never upstream), clean tree, `master` equal to `origin/master`, CI green on that exact commit, the version not already taken, `gh` able to push
  - Every FAIL must be fixed before going on. Each WARN is recorded below (Node version, updater, open PRs)
  - Output: `___`

- [ ] 2. **Identify the release commit** (usually `master`'s tip)
  - Commit SHA: `___` (the script prints it)
- [ ] 3. **Confirm CI is green on that exact commit**
  - `ci / go / build-lint-test`, `ci / frontend / typecheck`, secret scan, actionlint
  - PR-Agent is not part of this: it runs on PRs and shows as skipped on `master`. Its score is checked on the release PR in step 13
  - Link the specific run (the script prints it): `___`
- [ ] 4. **No real, ready work left unmerged**
  - `gh pr list --repo Performant-Labs/kai-private --state open` (the script counts them; the decisions are yours)
  - Dependabot bumps: merge the ones wanted, and record why the rest wait (a major bump such as a new Wails or Vitest version is a decision, not noise): `___`
- [ ] 5. **In-app updater channel** (see `docs/releasing.md`; [#178](https://github.com/Performant-Labs/kai-private/issues/178) is fixed)
  - The updater polls this fork (`buildinfo.UpdaterGithubRepo`), not upstream; nothing to decline. Until releases are public, installed apps get no update and users install by hand; the release notes must not promise auto-update.
  - The script warns if `main.go` ever points at `dtapps/kai` again. Result: `___`

## Known issues and changelog coverage

Gathered here, before the CHANGELOG is written, so nothing is missed.

- [ ] 6. **Build the candidate list** (the tracker is not reliable on its own)
  - `gh issue list --repo Performant-Labs/kai-private --label bug --state open`
  - `gh issue list --repo Performant-Labs/kai-private --label known-issue --state open`
  - Then skim the whole open list: real defects have been filed with no label
  - **Open does not mean live.** For each candidate, confirm it still happens on the build you are releasing, or write it as "unverified" and say why
- [ ] 7. **One line per real, still-open issue**, with a link. This exact list, plus the standing line from `docs/releasing.md` (Accessibility re-grant), goes into the CHANGELOG's Known Issues in step 12, verbatim: `___`
- [ ] 8. **Changelog coverage: `scripts/changelog-check.sh`**
  - Every PR it lists gets an entry under `[Unreleased]` citing its number or issue, or a reasoned entry in the `<!-- changelog-skip: ... -->` comment
  - Read the issue and the diff for "Implements #NN" PRs; do not write an entry from a title alone
  - Done when `scripts/changelog-check.sh --strict` prints `ok`. Output before: `___`

## Version and changelog

Steps 9-13 happen on ONE branch, `release/vX.Y.Z`, in one PR.

- [ ] 9. **Decide the bump** (PATCH / MINOR / MAJOR) from `CHANGELOG.md`'s `## [Unreleased]`
  - The first release has no earlier release tag: it is `0.1.0`, and its content is everything in `[Unreleased]`. Ignore the `backup/...` tag on the remote
- [ ] 10. **Branch `release/vX.Y.Z`** off the release commit
- [ ] 11. **`scripts/release-bump.sh X.Y.Z`**, then confirm `git diff` shows only 4 changed lines
  (2 in `build/config.yml`, 2 in `build/darwin/Info.plist`)
- [ ] 12. **Restructure `CHANGELOG.md`**
  - `[Unreleased]` becomes `## [X.Y.Z] - YYYY-MM-DD`, organized per `docs/releasing.md`
  - Known Issues = the list from step 7, verbatim, standing lines included (release notes are extracted from this section, so this is how the signing warning reaches them)
  - Fresh empty `## [Unreleased]` above it
- [ ] 13. **Open the PR, get PR-Agent above 90, merge it, confirm CI is green on the merge commit**
  - Every PR-Agent finding read and fixed or dismissed with a reason

## Tag and build

Platform: macOS Apple Silicon only. Build on a real Apple Silicon Mac, never cross-compile.

- [ ] 14. **Tag the merge commit**: `git tag -a vX.Y.Z -m "vX.Y.Z"`, then `git push origin vX.Y.Z`
  - Annotated only: Holler signs its tags, but no signing key is configured for this repo
- [ ] 15. **Build from that tag in a clean checkout, then check the tree**
  - `git clone --branch vX.Y.Z --depth 1 git@github.com:Performant-Labs/kai-private.git kai-release && cd kai-release`
  - `make install` (first time on that clone), then `make darwin-package VERSION=X.Y.Z`
  - `VERSION=` is mandatory: without it the build reports a commit SHA or `vX.Y.Z`, not `X.Y.Z`
  - `scripts/release-tree-check.sh vX.Y.Z` must print `ok`. Building always rewrites `build/darwin/icons.icns` and creates `build/darwin/Assets.car`; anything else changed means this is not the tag
- [ ] 16. **Quit any running Kai, then `scripts/release-verify.sh bin/Kai.app X.Y.Z`**
  - Must print `PASS`: version, signature, no embedded personal path, launches and stays running
  - It launches the app: the Dock icon bounces and Kai briefly takes focus
  - Output: `___`

## Manual smoke matrix (on the same `bin/Kai.app`)

CI and the verify script cannot see these. Install the built app to `/Applications` first
(quit Kai, replace the bundle, relaunch), then re-grant Accessibility if hotkeys copy nothing.

- [ ] 17. **App stays up.** Launch from `/Applications`, leave it 60 seconds. No crash, no stray window
- [ ] 18. **Alt+A on selected text in a native app** (Notes or TextEdit) opens the translate window
  with that text, not a stale clipboard value
- [ ] 19. **Translate** a short and a multi-line text with the default engine. Result appears, no error
- [ ] 20. **Windows.** Resize the translate window, quit, relaunch: size restored, window comes to the front.
  Settings window opens and stays fixed-size. Switch to another app and back: the Settings window is still there (#154)
- [ ] 21. **Alt+S screenshot translate** opens region select and returns a result
- [ ] 22. **Known-gap check.** Anything in step 7's list that this build was meant to fix: try it
- Result / anything that failed and was overridden (which, why): `___`

## Package and publish (confirm before doing: public, outward-facing)

- [ ] 23. **Package**
  - `scripts/release-package.sh bin/Kai.app X.Y.Z` makes `Kai-X.Y.Z-darwin-arm64.zip`, `Kai-X.Y.Z-darwin-arm64.dmg` and `SHA256SUMS` (covering both), and checks that each archive unpacks to an app identical to the input with a valid signature
  - It packages the verified app and never rebuilds. Package **once**: the disk image is not reproducible, so repackaging after publishing would change its hash
  - Output: `___`
- [ ] 24. **Build the release notes**: the CHANGELOG's `## [X.Y.Z]` section into a standalone
  file, minus the `<!-- changelog-skip -->` comment, with the **Installing** paragraph from
  `docs/releasing.md` ("Release notes preface") in front. Nothing else is added (the Accessibility
  warning is already in its Known Issues). Read it once as a stranger would
- [ ] 25. **Explicit go-ahead obtained** to publish
- [ ] 26. **Create the release**
  - `gh release create vX.Y.Z Kai-X.Y.Z-darwin-arm64.zip Kai-X.Y.Z-darwin-arm64.dmg SHA256SUMS --notes-file <notes> --repo Performant-Labs/kai-private --verify-tag`
- [ ] 27. **Review the published Release page**: all three assets present (zip, disk image, `SHA256SUMS`), notes render, known issues visible

## Post-release

Verify the published artifact, not the local build that produced it.

- [ ] 28. **Download and re-verify**
  - Clean directory: `gh release download vX.Y.Z --repo Performant-Labs/kai-private`
  - `shasum -a 256 -c SHA256SUMS`
  - `ditto -x -k Kai-X.Y.Z-darwin-arm64.zip out` then `scripts/release-verify.sh out/Kai.app X.Y.Z`
  - Mount the published disk image (`hdiutil attach -readonly -nobrowse Kai-X.Y.Z-darwin-arm64.dmg`) and `diff -r` its `Kai.app` against `out/Kai.app`: identical. Then detach it
  - Run the installer against the real release: `scripts/install-release.sh vX.Y.Z --dest "$(mktemp -d)" --keep-permissions` must print `checksum ok`, `installed Kai X.Y.Z` and `quarantine flag cleared`. This is the check that a person can actually install the release without the macOS block
- [ ] 29. **Close out**: link this checklist from the GitHub Release, tick every box, close the issue
