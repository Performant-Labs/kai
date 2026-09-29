# Releasing

How to cut a release of Kai from `Performant-Labs/kai-private`. Adapted from Holler's
`docs/releasing.md`; the differences are called out where they matter. Decision record: issue
[#177](https://github.com/Performant-Labs/kai-private/issues/177).

This repo is a private fork of `dtapps/kai`. A release is one macOS Apple Silicon app,
`Kai.app`, installed by hand on the maintainers' Macs. Nothing here publishes to upstream.

## Where the version number lives

**The git tag `vX.Y.Z` is the source of truth.** The tree at that tag must build an app that
reports `X.Y.Z`, so the release branch commits the version into the two files a macOS build reads:

- `build/config.yml`: every `version: "..."` value.
- `build/darwin/Info.plist`: `CFBundleShortVersionString` and `CFBundleVersion`.

`scripts/release-bump.sh X.Y.Z` edits exactly those four lines. Do not use `make build-assets`
for this: it regenerates nine files (Windows, Linux and iOS metadata included) from templates
and drops hand-written content from `Info.plist` (checked when the script was written).

At runtime the version comes from the build flag `-X ...buildinfo.Version`, which
`make darwin-package VERSION=X.Y.Z` sets. **Always pass `VERSION=` for a release build.** The
default is `git describe --tags --always`, which is a bare commit SHA on an untagged commit and
`vX.Y.Z` (with the `v`) on a tagged one. The startup log line `Kai starting... version=...`
shows this value, and `scripts/release-verify.sh` checks it. (No screen in the app displays the
version today.)

Format: SemVer, plain `X.Y.Z` in files, `vX.Y.Z` as the tag. This fork starts its own line at
`0.1.0`. It does not track upstream's numbers (see "The in-app updater" for why that matters).

## Were the tests run? What proves it?

**The release commit is `master` at a commit where every CI check is green** on that exact
commit (`gh pr checks <merging PR>` or the Actions run for the merge commit). Kai's checks:

| Check | What it proves |
| --- | --- |
| `ci / go / build-lint-test` | sqlc, i18n merge, real frontend build, golangci-lint, Go tests |
| `ci / frontend / typecheck` | Svelte / TypeScript typecheck |
| `ci / Secret scan`, `Pre-flight (actionlint)`, `Release conformance` | Repo hygiene |
| `pr-review / pr-agent` | Automated review. The standing bar is a score above 90, with every finding read and fixed or dismissed with a reason |

Do not tag off a commit whose run you have not looked at.

**CI green is necessary, not sufficient.** Kai's worst regression so far passed every automated
check: #163's window-restoration fix crashed the app about one second after every launch, and
was found only by running a real build (#167). CI cannot see native window, hotkey or
permission behavior. So a release also needs:

1. `scripts/release-verify.sh` on the built app. It launches the app and requires it to stay
   running, and checks version, signature and embedded paths.
2. The manual smoke matrix in the checklist template, run on the built app. It covers what only
   a person on a real Mac can see: the hotkey copies the selected text, windows open and keep
   their size, and so on.

A failing check does not automatically block a release; it can be knowingly overridden. The
override must be recorded, not silent: which check, why, in the release checklist issue. "Tests
passed" in release notes must never be covering for "we chose to ship anyway".

## Which platforms does a release target?

| Platform | Built by upstream's `release.yml` | Verified by this fork | Shipped |
| --- | --- | --- | --- |
| macOS arm64 | Yes | Yes (built and run on Apple Silicon) | **Yes** |
| macOS amd64 | Yes | No | No |
| Windows amd64 / arm64 | Yes | No | No |
| Linux | Commented out | No | No |

Do not ship a binary for a platform nobody here ran. Add a row's "Shipped" only after someone
builds and verifies it on that platform.

Build on a real Apple Silicon Mac, never cross-compile. The maintainer's Mac qualifies.

## Signing, and what it costs users

Builds here are **ad-hoc signed** (`Signature=adhoc`). This fork has no Apple Developer ID
certificate or notarization secrets. Two consequences to state in every release's notes:

- **Every new build is a new identity to macOS.** The Accessibility grant (on macOS 27 it is
  under "Device Control and Data Access") is tied to the app's signature, so installing a new
  build silently invalidates it: hotkeys register but the simulated copy returns an empty
  clipboard. Fix: remove Kai from the list and add it again, or run
  `tccutil reset Accessibility net.dtapp.kai` and relaunch. (Screen Recording stayed granted
  across every rebuild seen so far, so it is not listed here.)
- **Gatekeeper.** A zip downloaded through a browser gets a quarantine flag and will be blocked
  as "unidentified developer". `gh release download` does not set the flag. Otherwise run
  `xattr -dr com.apple.quarantine /Applications/Kai.app`.

## The in-app updater (release-blocking until decided)

`main.go` (around line 642) hardcodes the updater to **upstream**: `CnbRepo: "dtapp/kai"`,
`GithubRepo: "dtapps/kai"`. Two consequences:

1. A release cut from this repo is invisible to installed apps.
2. Installed apps can offer **upstream's** build, and accepting it would replace this fork's
   fixes. The log shows this today: `Stable update ready: v0.2.0`.

Until this is decided and fixed (repoint the updater at this repo, which needs a way to read a
private repo's releases without embedding a token in a distributed app, or disable in-app
updates for fork builds), the release checklist has a step that says so out loud and the
release notes must tell users to decline the update prompt. This is a product-behavior change,
so it is tracked in its own issue, [#178](https://github.com/Performant-Labs/kai-private/issues/178), and not done as part of the process work.

## What a release produces

1. **A git tag + `CHANGELOG.md` entry.** `vX.Y.Z`, annotated. The `## [Unreleased]` section
   becomes `## [X.Y.Z] - YYYY-MM-DD` and stays hand-written, not generated from commit messages.
2. **A GitHub Release** on `Performant-Labs/kai-private` with:
   - `Kai-X.Y.Z-darwin-arm64.zip`, made with `ditto -c -k --keepParent bin/Kai.app <zip>`
     (round trip checked: the signature stays valid after unzip; a plain `zip` can break it).
   - `SHA256SUMS`.

   This is outward-facing. Confirm with whoever is driving the release before publishing, every
   time.

**`.github/workflows/release.yml` is not used for this fork's releases.** It is upstream's,
kept unedited so upstream changes still merge cleanly. It builds four platforms on billed
hosted runners (three of which this fork does not ship), expects updater, CNB, PostHog and
signing secrets this fork does not have (empty tokens would be baked into the binary), and its
release step targets a channel no installed app watches. Revisit it if this fork gains a
Developer ID and a working update channel.

## CHANGELOG entry structure

Same four sections as Holler, in this order, each omitted when empty:

```markdown
## [X.Y.Z] - YYYY-MM-DD

### Enhancements
- New capability or additive change, one line each.

### Breaking Changes
- Anything that changes settings files, on-disk data, or hotkey behavior users rely on.

### Bug Fixes
- Real fixes, one line each, linking the issue.

### Known Issues
- Real, still-open gaps a user should know before they hit them. Link the issue.
```

## Known issues

A documented issue is not by itself a release blocker; silence is the failure. Before writing
the CHANGELOG, skim open `bug` issues (`gh issue list --repo Performant-Labs/kai-private
--label bug --state open`) and anything plainly a defect without the label. One line each: what
it is, when it bites, a link. The list for a specific release lives in that release's checklist
issue, not here.

## Who cuts a release, and when

Manual, on demand. No cadence and no automated trigger.

## Step by step

All version and changelog work lands on ONE branch, `release/vX.Y.Z`, in one PR. Kai's PRs go
to `Performant-Labs/kai-private`, never upstream.

1. Confirm the release commit (usually `master`'s tip) has a green CI run, PR-Agent above 90.
2. Gather known issues now, before writing the CHANGELOG.
3. Branch `release/vX.Y.Z` off the release commit.
4. Decide the bump from `CHANGELOG.md`'s `## [Unreleased]`.
5. `scripts/release-bump.sh X.Y.Z`.
6. Restructure `CHANGELOG.md`: `[Unreleased]` becomes `[X.Y.Z] - date`, Known Issues verbatim
   from step 2, a fresh empty `[Unreleased]` above.
7. Commit, open a PR, merge, confirm CI is green on the merge commit.
8. `git tag -a vX.Y.Z -m "vX.Y.Z"` on the merge commit, `git push origin vX.Y.Z`.
9. Build on Apple Silicon **from that tag**, in a clean checkout:
   `make darwin-package VERSION=X.Y.Z`.
10. Quit any running Kai, then `scripts/release-verify.sh bin/Kai.app X.Y.Z`. It must print PASS.
11. Run the manual smoke matrix (checklist) against the same `bin/Kai.app`.
12. `ditto -c -k --keepParent bin/Kai.app Kai-X.Y.Z-darwin-arm64.zip`, then
    `shasum -a 256 Kai-X.Y.Z-darwin-arm64.zip > SHA256SUMS`.
13. Extract the CHANGELOG's `## [X.Y.Z]` section into a standalone notes file. Add the signing
    and updater notes from above.
14. **Confirm before publishing**, then
    `gh release create vX.Y.Z Kai-X.Y.Z-darwin-arm64.zip SHA256SUMS --notes-file <notes>`.
15. **Verify the published artifact, not just the local build.** In a clean directory,
    `gh release download vX.Y.Z`, check the checksum, unzip with `ditto -x -k`, and run
    `scripts/release-verify.sh <that Kai.app> X.Y.Z` again.

For the checklist to run each time, copy
[`docs/release-checklist.md`](release-checklist.md) into a new issue titled
`Release checklist: vX.Y.Z`. Do not check boxes on the template itself.
