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
  - PR-Agent score above 90, every finding fixed or dismissed with a reason
  - Link the specific run: `___`
- [ ] 3. **No real, ready work left unmerged**
  - `gh pr list --repo Performant-Labs/kai-private --state open` (ignore dependabot unless one is wanted)
- [ ] 4. **In-app updater channel** (release-blocking until decided; see `docs/releasing.md`)
  - The updater is hardcoded to upstream `dtapps/kai` (`main.go` ~line 642) and can offer
    upstream's build over this one.
  - Tracked in [#178](https://github.com/Performant-Labs/kai-private/issues/178). State of the fix: `___` ("fixed in <PR>", or "not fixed: release notes tell users to decline the update prompt")

## Known issues

Gathered here, before the CHANGELOG is written, so nothing is missed.

- [ ] 5. **Skim open `bug` issues**
  - `gh issue list --repo Performant-Labs/kai-private --label bug --state open`
  - Also anything plainly a defect without the label
- [ ] 6. **One line per real, still-open issue**, with a link. This exact list goes into the
  CHANGELOG's Known Issues in step 10, verbatim: `___`

## Version and changelog

Steps 7-11 happen on ONE branch, `release/vX.Y.Z`, in one PR.

- [ ] 7. **Decide the bump** (PATCH / MINOR / MAJOR) from `CHANGELOG.md`'s `## [Unreleased]`
- [ ] 8. **Branch `release/vX.Y.Z`** off the release commit
- [ ] 9. **`scripts/release-bump.sh X.Y.Z`**, then confirm `git diff` shows only 4 changed lines
  (2 in `build/config.yml`, 2 in `build/darwin/Info.plist`)
- [ ] 10. **Restructure `CHANGELOG.md`**
  - `[Unreleased]` becomes `## [X.Y.Z] - YYYY-MM-DD`, organized per `docs/releasing.md`
  - Known Issues = the list from step 6, verbatim
  - Signing and updater notes from `docs/releasing.md` included in the release notes
  - Fresh empty `## [Unreleased]` above it
- [ ] 11. **Open the PR, merge it, confirm CI is green on the merge commit**

## Tag and build

Platform: macOS Apple Silicon only. Build on a real Apple Silicon Mac, never cross-compile.

- [ ] 12. **Tag the merge commit**: `git tag -a vX.Y.Z -m "vX.Y.Z"`, then `git push origin vX.Y.Z`
- [ ] 13. **Build from that tag in a clean checkout**
  - `git clone --branch vX.Y.Z --depth 1 git@github.com:Performant-Labs/kai-private.git kai-release && cd kai-release`
  - `make install` (first time on that clone), then `make darwin-package VERSION=X.Y.Z`
  - `VERSION=` is mandatory: without it the build reports a commit SHA or `vX.Y.Z`, not `X.Y.Z`
- [ ] 14. **Quit any running Kai, then `scripts/release-verify.sh bin/Kai.app X.Y.Z`**
  - Must print `PASS`: version, signature, no embedded personal path, launches and stays running
  - Output: `___`

## Manual smoke matrix (on the same `bin/Kai.app`)

CI and the verify script cannot see these. Install the built app to `/Applications` first
(quit Kai, replace the bundle, relaunch), then re-grant Accessibility if hotkeys copy nothing.

- [ ] 15. **App stays up.** Launch from `/Applications`, leave it 60 seconds. No crash, no stray window
- [ ] 16. **Alt+A on selected text in a native app** (Notes or TextEdit) opens the translate window
  with that text, not a stale clipboard value
- [ ] 17. **Translate** a short and a multi-line text with the default engine. Result appears, no error
- [ ] 18. **Windows.** Resize the translate window, quit, relaunch: size restored, window comes to the front.
  Settings window opens and stays fixed-size
- [ ] 19. **Alt+S screenshot translate** opens region select and returns a result
- [ ] 20. **Known-gap check.** Anything in step 6's list that this build was meant to fix: try it
- Result / anything that failed and was overridden (which, why): `___`

## Package and publish (confirm before doing: public, outward-facing)

- [ ] 21. **Package**
  - `ditto -c -k --keepParent bin/Kai.app Kai-X.Y.Z-darwin-arm64.zip` (not plain `zip`: it can break the signature)
  - `shasum -a 256 Kai-X.Y.Z-darwin-arm64.zip > SHA256SUMS`
- [ ] 22. **Extract the release notes**: the CHANGELOG's `## [X.Y.Z]` section into a standalone
  file, no new content. Read it once as a stranger would
- [ ] 23. **Explicit go-ahead obtained** to publish
- [ ] 24. **Create the release**
  - `gh release create vX.Y.Z Kai-X.Y.Z-darwin-arm64.zip SHA256SUMS --notes-file <notes> --repo Performant-Labs/kai-private`
- [ ] 25. **Review the published Release page**: asset present, notes render, known issues visible

## Post-release

Verify the published artifact, not the local build that produced it.

- [ ] 26. **Download and re-verify**
  - Clean directory: `gh release download vX.Y.Z --repo Performant-Labs/kai-private`
  - `shasum -a 256 -c SHA256SUMS`
  - `ditto -x -k Kai-X.Y.Z-darwin-arm64.zip out` then `scripts/release-verify.sh out/Kai.app X.Y.Z`
- [ ] 27. **Close out**: link this checklist from the GitHub Release, tick every box, close the issue
