# Release checklist (template)

**This is the reusable template. It carries no version number.** Copy this file's content into a
new GitHub issue titled `Release checklist: vX.Y.Z` each time a release is cut, and fill in that
copy. Never check boxes here. The reasoning behind each step is in
[`docs/releasing.md`](releasing.md).

## Pre-flight

- [ ] 1. **Identify the release commit** (usually `master`'s tip)
  - Commit SHA: `___`
- [ ] 2. **Confirm CI is green on that exact commit**
  - `ci / go / build-lint-test`, `ci / frontend / typecheck`, secret scan, actionlint
  - PR-Agent is not part of this: it runs on PRs and shows as skipped on `master`. Its score is checked on the release PR in step 12
  - Link the specific run: `___`
- [ ] 3. **No real, ready work left unmerged**
  - `gh pr list --repo Performant-Labs/kai-private --state open`
  - Dependabot bumps: merge the ones wanted, and record why the rest wait (a major bump such as a new Wails or Vitest version is a decision, not noise): `___`
- [ ] 4. **In-app updater channel** (release-blocking until decided; see `docs/releasing.md`)
  - The updater is hardcoded to upstream `dtapps/kai` (`main.go` ~line 642) and can offer
    upstream's build over this one.
  - Tracked in [#178](https://github.com/Performant-Labs/kai-private/issues/178). State of the fix: `___` ("fixed in <PR>", or "not fixed: release notes tell users to decline the update prompt")

## Known issues and changelog coverage

Gathered here, before the CHANGELOG is written, so nothing is missed.

- [ ] 5. **Build the candidate list** (the tracker is not reliable on its own)
  - `gh issue list --repo Performant-Labs/kai-private --label bug --state open`
  - `gh issue list --repo Performant-Labs/kai-private --label known-issue --state open`
  - Then skim the whole open list: real defects have been filed with no label
  - **Open does not mean live.** For each candidate, confirm it still happens on the build you are releasing, or write it as "unverified" and say why
- [ ] 6. **One line per real, still-open issue**, with a link. This exact list, plus the two standing lines from `docs/releasing.md` (updater #178, Accessibility re-grant), goes into the CHANGELOG's Known Issues in step 11, verbatim: `___`
- [ ] 7. **Changelog coverage: `scripts/changelog-check.sh`**
  - Every PR it lists gets an entry under `[Unreleased]` citing its number or issue, or a reasoned entry in the `<!-- changelog-skip: ... -->` comment
  - Read the issue and the diff for "Implements #NN" PRs; do not write an entry from a title alone
  - Done when `scripts/changelog-check.sh --strict` prints `ok`. Output before: `___`

## Version and changelog

Steps 8-12 happen on ONE branch, `release/vX.Y.Z`, in one PR.

- [ ] 8. **Decide the bump** (PATCH / MINOR / MAJOR) from `CHANGELOG.md`'s `## [Unreleased]`
  - The first release has no earlier release tag: it is `0.1.0`, and its content is everything in `[Unreleased]`. Ignore the `backup/...` tag on the remote
- [ ] 9. **Branch `release/vX.Y.Z`** off the release commit
- [ ] 10. **`scripts/release-bump.sh X.Y.Z`**, then confirm `git diff` shows only 4 changed lines
  (2 in `build/config.yml`, 2 in `build/darwin/Info.plist`)
- [ ] 11. **Restructure `CHANGELOG.md`**
  - `[Unreleased]` becomes `## [X.Y.Z] - YYYY-MM-DD`, organized per `docs/releasing.md`
  - Known Issues = the list from step 6, verbatim, standing lines included (release notes are extracted from this section, so this is how the signing and updater warnings reach them)
  - Fresh empty `## [Unreleased]` above it
- [ ] 12. **Open the PR, get PR-Agent above 90, merge it, confirm CI is green on the merge commit**
  - Every PR-Agent finding read and fixed or dismissed with a reason

## Tag and build

Platform: macOS Apple Silicon only. Build on a real Apple Silicon Mac, never cross-compile.

- [ ] 13. **Tag the merge commit**: `git tag -a vX.Y.Z -m "vX.Y.Z"`, then `git push origin vX.Y.Z`
  - Annotated only: Holler signs its tags, but no signing key is configured for this repo
- [ ] 14. **Build from that tag in a clean checkout, then check the tree**
  - `git clone --branch vX.Y.Z --depth 1 git@github.com:Performant-Labs/kai-private.git kai-release && cd kai-release`
  - `make install` (first time on that clone), then `make darwin-package VERSION=X.Y.Z`
  - `VERSION=` is mandatory: without it the build reports a commit SHA or `vX.Y.Z`, not `X.Y.Z`
  - `scripts/release-tree-check.sh vX.Y.Z` must print `ok`. Building always rewrites `build/darwin/icons.icns` and creates `build/darwin/Assets.car`; anything else changed means this is not the tag
- [ ] 15. **Quit any running Kai, then `scripts/release-verify.sh bin/Kai.app X.Y.Z`**
  - Must print `PASS`: version, signature, no embedded personal path, launches and stays running
  - It launches the app: the Dock icon bounces and Kai briefly takes focus
  - Output: `___`

## Manual smoke matrix (on the same `bin/Kai.app`)

CI and the verify script cannot see these. Install the built app to `/Applications` first
(quit Kai, replace the bundle, relaunch), then re-grant Accessibility if hotkeys copy nothing.

- [ ] 16. **App stays up.** Launch from `/Applications`, leave it 60 seconds. No crash, no stray window
- [ ] 17. **Alt+A on selected text in a native app** (Notes or TextEdit) opens the translate window
  with that text, not a stale clipboard value
- [ ] 18. **Translate** a short and a multi-line text with the default engine. Result appears, no error
- [ ] 19. **Windows.** Resize the translate window, quit, relaunch: size restored, window comes to the front.
  Settings window opens and stays fixed-size. Switch to another app and back: the Settings window is still there (#154)
- [ ] 20. **Alt+S screenshot translate** opens region select and returns a result
- [ ] 21. **Known-gap check.** Anything in step 6's list that this build was meant to fix: try it
- Result / anything that failed and was overridden (which, why): `___`

## Package and publish (confirm before doing: public, outward-facing)

- [ ] 22. **Package**
  - `ditto -c -k --keepParent bin/Kai.app Kai-X.Y.Z-darwin-arm64.zip` (not plain `zip`: it can break the signature)
  - `shasum -a 256 Kai-X.Y.Z-darwin-arm64.zip > SHA256SUMS`
- [ ] 23. **Extract the release notes**: the CHANGELOG's `## [X.Y.Z]` section into a standalone
  file. Extraction only, no new content (the standing lines are already in its Known Issues).
  Read it once as a stranger would
- [ ] 24. **Explicit go-ahead obtained** to publish
- [ ] 25. **Create the release**
  - `gh release create vX.Y.Z Kai-X.Y.Z-darwin-arm64.zip SHA256SUMS --notes-file <notes> --repo Performant-Labs/kai-private`
- [ ] 26. **Review the published Release page**: asset present, notes render, known issues visible

## Post-release

Verify the published artifact, not the local build that produced it.

- [ ] 27. **Download and re-verify**
  - Clean directory: `gh release download vX.Y.Z --repo Performant-Labs/kai-private`
  - `shasum -a 256 -c SHA256SUMS`
  - `ditto -x -k Kai-X.Y.Z-darwin-arm64.zip out` then `scripts/release-verify.sh out/Kai.app X.Y.Z`
- [ ] 28. **Close out**: link this checklist from the GitHub Release, tick every box, close the issue
