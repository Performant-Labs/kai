# Release checklist (template)

**This is the reusable template. It carries no version number.** Copy this file's content into a
new GitHub issue titled `Release checklist: vX.Y.Z` each time a release is cut, and fill in that
copy. Never check boxes here. The reasoning behind each step is in
[`docs/releasing.md`](releasing.md).

`scripts/release.sh vX.Y.Z` is the operator: it opens the `Release checklist: vX.Y.Z` issue from
this template itself, drives these steps in order with no questions (state in
`bin/release/vX.Y.Z/state.json`, postcondition-first resume), and reports into that issue as it
goes — WARN rows, the smoke record, evidence at closeout — closing it when the release is done.
The steps that stay yours: the **manual smoke matrix** below (recorded by the workflow when you
re-run with `--smoke` / `--smoke-done`) and the **publish go-ahead** (`--publish`). See
"Release notes preface" and "The operator" in [`docs/releasing.md`](releasing.md); the macOS
codesign key-access prompt during the build is OS-level, not a workflow prompt.
`scripts/release.sh vX.Y.Z --dry-run` is safe to run at any time: it runs the read-only checks (needs,
preflight, changelog), prints what the real run would do, and changes nothing.

## Pre-flight

- [ ] 1. **Run `scripts/release-preflight.sh vX.Y.Z`** on the release Mac, in a clean checkout of `master`
  - Machine: Apple Silicon macOS, the required tools, Go at least `go.mod`'s version
  - A running Kai is only a WARN here: quit it before step 15, because `release-local.sh` and `release-verify.sh` refuse while it runs
  - Commit: `origin` is the fork (never upstream), clean tree, `master` equal to `origin/master`, CI green on that exact commit, the version not already taken on `origin` (this clone may also hold `dtapps/kai`'s tags, whose numbers are irrelevant; a stray local tag of that name is reported, fix it with `git tag -d`), `gh` able to push
  - Every FAIL must be fixed before going on. Each WARN is recorded below (Node version, updater, open PRs)
  - Output: `___`

- [ ] 2. **Identify the release commit** (usually `master`'s tip)
  - Commit SHA: `___` (the script prints it)
- [ ] 3. **Confirm CI is green on that exact commit**
  - `ci / go / build-lint-test`, `ci / frontend / typecheck`, secret scan, actionlint
  - PR-Agent is not part of this: it runs on PRs and shows as skipped on `master`. Its score is checked on the release PR in step 13
  - Link the specific run (the script prints it): `___`
- [ ] 4. **No real, ready work left unmerged**
  - `gh pr list --repo Performant-Labs/kai --state open` (the script counts them; the decisions are yours)
  - Dependabot bumps: merge the ones wanted, and record why the rest wait (a major bump such as a new Wails or Vitest version is a decision, not noise): `___`
- [ ] 5. **In-app updater channel** (see `docs/releasing.md`; [#178](https://github.com/Performant-Labs/kai-private/issues/178) (archived repository) is fixed)
  - The updater polls the public `Performant-Labs/kai` (`buildinfo.UpdaterGithubRepo`), not upstream; nothing to decline. The repository is public, so its releases are readable without a login. Until a release exists there and the updater has been seen to find it on a real install ([#196](https://github.com/Performant-Labs/kai-private/issues/196) (archived repository)), say nothing in the release notes that promises auto-update. The `updater-` asset in step 23 is what the updater reads.
  - The script warns if `main.go` ever points at `dtapps/kai` again. Result: `___`

## Known issues and changelog coverage

Gathered here, before the CHANGELOG is written, so nothing is missed.

- [ ] 6. **Build the candidate list** (the tracker is not reliable on its own)
  - `gh issue list --repo Performant-Labs/kai --label bug --state open`
  - `gh issue list --repo Performant-Labs/kai --label known-issue --state open`
  - Then skim the whole open list: real defects have been filed with no label
  - **Open does not mean live.** For each candidate, confirm it still happens on the build you are releasing, or write it as "unverified" and say why
- [ ] 7. **One line per real, still-open issue**, with a link. This exact list, plus the standing line from `docs/releasing.md` (Accessibility re-grant), goes into the CHANGELOG's Known Issues in step 12, verbatim: `___`
- [ ] 8. **Changelog coverage: `scripts/changelog-check.sh`**
  - Every PR it lists gets an entry under `[Unreleased]` citing its number or issue, or a reasoned entry in the `<!-- changelog-skip: ... -->` comment
  - Read the issue and the diff for "Implements #NN" PRs; do not write an entry from a title alone
  - Its range starts at the newest `v*` tag **on `origin`** (the first line, `range: merged after vX.Y.Z (...)`, must name this fork's last release). If the commit is missing, `git fetch origin`
  - Done when `scripts/changelog-check.sh --strict` prints `ok`. Output before: `___`

## Version and changelog

Steps 9-13 happen on ONE branch, `release/vX.Y.Z`, in one PR.

- [ ] 9. **Decide the bump** (PATCH / MINOR / MAJOR) from `CHANGELOG.md`'s `## [Unreleased]`
  - Count from this fork's last release (`gh release list`, or the tags on `origin`), never from `git tag` in a clone that also fetched `dtapps/kai`: upstream has its own `v0.2.x` tags, and a local `v0.2.0` can be upstream's commit, not ours
  - The first release has no earlier release tag: it is `0.1.0`, and its content is everything in `[Unreleased]`. Ignore the `backup/...` tag on the remote
- [ ] 10. **Branch `release/vX.Y.Z`** off the release commit
- [ ] 11. **`scripts/release-bump.sh X.Y.Z`**, then confirm `git diff` shows only 4 changed lines
  (2 in `build/config.yml`, 2 in `build/darwin/Info.plist`)
- [ ] 12. **Restructure `CHANGELOG.md`**
  - `[Unreleased]` becomes `## [X.Y.Z] - YYYY-MM-DD`, organized per `docs/releasing.md`
  - Known Issues = the list from step 7, verbatim, standing lines included (release notes are extracted from this section, so this is how the signing warning reaches them)
  - Fresh empty `## [Unreleased]` above it
- [ ] 13. **Open the PR, get PR-Agent above 90, merge it, confirm CI is green on the merge commit**
  - Merge with `gh pr merge --merge --delete-branch` so `release/vX.Y.Z` does not outlive the PR (a leftover one that is already in master only makes the pre-flight warn)
  - Every PR-Agent finding read and fixed or dismissed with a reason

## Tag and build

Platform: macOS Apple Silicon only. Build on a real Apple Silicon Mac, never cross-compile.

- [ ] 14. **Tag the merge commit**: `git tag -a vX.Y.Z -m "vX.Y.Z"`, then `git push origin vX.Y.Z`
  - Annotated only: Holler signs its tags, but no signing key is configured for this repo
- [ ] 15. **Build from that tag in a clean checkout, then check the tree**
  - `git clone --branch vX.Y.Z --depth 1 git@github.com:Performant-Labs/kai.git kai-release && cd kai-release`
  - `make install` (first time on that clone), then `make darwin-package VERSION=X.Y.Z`
  - `VERSION=` is mandatory: without it the build reports a commit SHA or `vX.Y.Z`, not `X.Y.Z`
  - `scripts/release-tree-check.sh vX.Y.Z` must print `ok`. Building always rewrites `build/darwin/icons.icns` and creates `build/darwin/Assets.car`; anything else changed means this is not the tag
- [ ] 16. **Quit any running Kai or Kai-dev, then `scripts/release-verify.sh bin/Kai.app X.Y.Z`**
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
  - `scripts/release-package.sh bin/Kai.app X.Y.Z` makes `Kai-X.Y.Z-darwin-arm64.zip`, `updater-Kai-X.Y.Z-darwin-arm64.zip` (a byte-identical copy the in-app updater requires by that name), `Kai-X.Y.Z-darwin-arm64.dmg` and `SHA256SUMS` (covering all three), and checks that each archive unpacks to an app identical to the input with a valid signature
  - It packages the verified app and never rebuilds. Package **once**: the disk image is not reproducible, so repackaging after publishing would change its hash
  - Output: `___`
- [ ] 24. **Build the release notes**: the CHANGELOG's `## [X.Y.Z]` section into a standalone
  file, minus the `<!-- changelog-skip -->` comment, with the **Installing** paragraph from
  `docs/releasing.md` ("Release notes preface") in front. Nothing else is added (the Accessibility
  warning is already in its Known Issues). Read it once as a stranger would
- [ ] 25. **Explicit go-ahead obtained** to publish
- [ ] 26. **Create the release**
  - `gh release create vX.Y.Z Kai-X.Y.Z-darwin-arm64.zip updater-Kai-X.Y.Z-darwin-arm64.zip Kai-X.Y.Z-darwin-arm64.dmg SHA256SUMS --notes-file <notes> --repo Performant-Labs/kai --verify-tag`
- [ ] 27. **Review the published Release page**: all four assets present (zip, `updater-` zip, disk image, `SHA256SUMS`), notes render, known issues visible

## Post-release

Verify the published artifact, not the local build that produced it.

- [ ] 28. **Download and re-verify**
  - Clean directory: `gh release download vX.Y.Z --repo Performant-Labs/kai`
  - `shasum -a 256 -c SHA256SUMS` (three lines: zip, `updater-` zip, disk image) and `cmp Kai-X.Y.Z-darwin-arm64.zip updater-Kai-X.Y.Z-darwin-arm64.zip` (identical)
  - `ditto -x -k Kai-X.Y.Z-darwin-arm64.zip out` then `scripts/release-verify.sh out/Kai.app X.Y.Z`
  - Mount the published disk image (`hdiutil attach -readonly -nobrowse Kai-X.Y.Z-darwin-arm64.dmg`) and `diff -r` its `Kai.app` against `out/Kai.app`: identical. Then detach it
  - Run the installer against the real release: `scripts/install-release.sh vX.Y.Z --dest "$(mktemp -d)" --keep-permissions` must print `checksum ok`, `installed Kai X.Y.Z` and `quarantine flag cleared`. This is the check that a person can actually install the release without the macOS block
- [ ] 29. **Close out**: link this checklist from the GitHub Release, tick every box, close the issue
