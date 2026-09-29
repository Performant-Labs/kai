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

**The first release has no earlier release tag.** Treat everything in `[Unreleased]` as its
content and cut `0.1.0`. The one tag on the remote, `backup/master-before-subject-rewrite-2026-09-26`,
is a backup unreachable from `master`; ignore it. From the second release on, "since the last
release" means since the latest `v*` tag.

## Were the tests run? What proves it?

**The release commit is `master` at a commit where every CI check is green** on that exact
commit (`gh pr checks <merging PR>` or the Actions run for the merge commit). Kai's checks:

| Check | What it proves |
| --- | --- |
| `ci / go / build-lint-test` | sqlc, i18n merge, real frontend build, golangci-lint, Go tests |
| `ci / frontend / typecheck` | Svelte / TypeScript typecheck |
| `ci / Secret scan`, `Pre-flight (actionlint)`, `Release conformance` | Repo hygiene |

PR-Agent is not in that table on purpose. It runs on pull requests, not on `master`: on the
release commit `pr-review / pr-agent` shows as skipped. Every PR merged into the release already
cleared it. What is left to check is the **release PR itself**, whose score must be above 90
with every finding read and fixed or dismissed with a reason.

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
- **Gatekeeper.** A file downloaded by a browser gets the `com.apple.quarantine` flag, and macOS
  blocks a quarantined, ad-hoc-signed app as coming from an unidentified developer. Checked:
  `gh release download` does not set the flag, and a disk image does not help (the app inside is
  still blocked). `scripts/install-release.sh` is the supported way to install a release without
  the block: it downloads with `gh` (no flag), or with `--from FILE` takes a zip or disk image you
  already downloaded and clears the flag from it. Either way it verifies the checksum when one is
  available, checks the bundle id and signature, refuses to replace a running Kai, clears the flag
  on the installed copy, and clears the stale Accessibility grant **only if the app's code
  changed**. Removing the flag from a build made by this project's own release process, on your
  own Mac, is ordinary; it does not make the app trusted for anyone else.
- **The real fix is not available yet:** a Developer ID signature plus notarization. It needs an
  Apple Developer Program membership; the release Mac has no Developer ID certificate and
  1Password has none either. It would also stop every build invalidating the Accessibility grant.
  Until then the two bullets above are the cost of shipping ad-hoc signed.

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
2. **A GitHub Release** on `Performant-Labs/kai-private` with three assets, all made by
   `scripts/release-package.sh bin/Kai.app X.Y.Z`:
   - `Kai-X.Y.Z-darwin-arm64.zip`, made with `ditto` (a plain `zip` can break the signature).
   - `Kai-X.Y.Z-darwin-arm64.dmg`, a disk image with `Kai.app` and an Applications shortcut. A
     `.app` is a folder and a release asset must be one file, so it has to be archived; the zip
     unpacks to `Kai.app` and the disk image is the usual Mac form.
   - `SHA256SUMS`, covering both.

   The script packages the app it is given and **never rebuilds**: every build embeds its build
   time and regenerates `Assets.car`, so a rebuild is a different binary from the one
   `release-verify.sh` approved. It checks that each archive unpacks to an app identical to the
   input with a valid signature. Package **once** and do not repackage after publishing: the zip
   is reproducible (identical hash on a second run), the disk image is not (it embeds timestamps).

   This is outward-facing. Confirm with whoever is driving the release before publishing, every
   time.

**`.github/workflows/release.yml` is not used for this fork's releases.** It is upstream's,
kept unedited so upstream changes still merge cleanly. It builds four platforms on billed
hosted runners (three of which this fork does not ship), expects updater, CNB, PostHog and
signing secrets this fork does not have (empty tokens would be baked into the binary), and its
release step targets a channel no installed app watches. Revisit it if this fork gains a
Developer ID and a working update channel.

## What a build changes in the tree

Building from a clean checkout of the tag leaves two files different, always:

- `build/darwin/icons.icns` (tracked) is rewritten, and
- `build/darwin/Assets.car` (untracked) is created,

because `make darwin-package` always runs icon generation with the Icon Composer flags
(`build/Taskfile.yml`, `generate:icons`). Checked when this was written: the rewritten `icons.icns`
is the same every run but differs from the committed one (a small fallback icns; the icon current
macOS shows comes from `Assets.car`), and `Assets.car` differs on every run. Committing either
would not make a build reproducible, and it would change the app icon in a process change, so
they are allowed to differ. The icon itself is fixed by the tracked sources (`build/appicon.png`,
`build/appicon.icon`).

Anything **else** changed after the build means the app was not built from the tag as committed.
`scripts/release-tree-check.sh vX.Y.Z` checks that HEAD is exactly the tag and that only those two
files differ.

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

## Changelog coverage

A changelog only helps if every change that merged is in it, and nothing forces that at merge
time. The first dry run of this process found 21 of the 39 merged PRs it considers missing,
including #174 and #176, whose changes were absent even though their issue numbers appeared in
Known Issues. So each release runs `scripts/changelog-check.sh`, which lists merged PRs the
`[Unreleased]` section does not cover.

- A PR is **covered** when its number, an issue number in its title, or a "Closes/Fixes/Resolves
  #N" in its body appears in `[Unreleased]`, **outside** Known Issues. Known Issues names issues
  that are still open, so it says nothing about a fix having been recorded.
- A PR that deliberately gets no entry (CI, test-harness and similar plumbing) goes in the
  `<!-- changelog-skip: ... -->` comment at the top of `[Unreleased]`, with a reason. Bot PRs and
  titles starting `ci`, `chore`, `docs` or `test` are ignored without a skip line.
- **Do not write an entry from a title alone.** "Implements #NN" pipeline PRs say nothing about
  what changed; read the issue's acceptance section and the diff, and check the current code when
  the issue allowed more than one outcome.
- The script prints and exits 0. It is a checklist step, not a CI gate: gating every PR on a
  changelog line would change how all pipeline PRs pass, and that is a separate decision.
- `bash scripts/changelog-check.test.sh` tests the script against a canned PR list.

## Known issues

A documented issue is not by itself a release blocker; silence is the failure. But the issue
tracker is not a reliable list of them, so build the list deliberately:

- Search **both** labels: `gh issue list --repo Performant-Labs/kai-private --label bug --state
  open` and `--label known-issue --state open`. `known-issue` means "a real, accepted gap worth
  naming in the next release's Known Issues, not necessarily a defect"; apply it when an issue
  qualifies. Then skim the whole open list anyway: real defects have been filed with no label.
- **Open does not mean live.** An issue can already be fixed by a later change (#154 was filed
  against the old menu-bar-only mode and may be fixed by the Dock icon change), or stay open
  after its PR merged because the PR did not say "Closes" (#161). For each candidate, confirm it
  still happens on the build you are releasing, or write it as "unverified" with the reason.
- One line each: what it is, when it bites, a link.
- Carry the **standing lines** below into every release until their cause is fixed. They are not
  issues to skim for; they are consequences of how this fork ships.

Standing Known Issues lines (verbatim, in the CHANGELOG's Known Issues):

- The in-app updater is hardcoded to upstream `dtapps/kai` and can offer an upstream build over
  this one. Decline the update prompt (#178).
- Every new build is a new identity to macOS, so the Accessibility grant must be removed and
  added again after installing one.

Because release notes are extracted from the CHANGELOG, this is how the updater and Accessibility
warnings reach the notes without anyone writing them separately at publish time. The third
consequence of ad-hoc signing, the Gatekeeper quarantine, is not a known issue but an install
instruction, so it lives in the release notes preface (next section).

## Release notes preface

The release notes are the CHANGELOG's `## [X.Y.Z]` section, minus the internal
`<!-- changelog-skip: ... -->` comment, with this one paragraph in front and nothing else. It
answers the first question a reader has (how do I install it, and why does Alt+A not work) and
carries the Gatekeeper warning. Fill in the version:

```markdown
**Installing.** macOS on Apple Silicon only. Download with `gh release download vX.Y.Z --repo Performant-Labs/kai-private`: a browser download is quarantined by macOS and blocked as coming from an unidentified developer (if that happens, run `xattr -dr com.apple.quarantine /Applications/Kai.app`). Unzip, move `Kai.app` to `/Applications`, and check the zip against `SHA256SUMS`. This build is ad-hoc signed, so remove Kai from Privacy & Security and add it again (on macOS 27 the list is "Device Control and Data Access") before Alt+A can copy text. The app is attached twice: `Kai-X.Y.Z-darwin-arm64.zip`, and `Kai-X.Y.Z-darwin-arm64.dmg`, a disk image with `Kai.app` inside to drag to Applications. If you have a checkout of the repo, `scripts/install-release.sh vX.Y.Z` downloads and installs it without the quarantine block.
```

(v0.1.0's published notes stop after the disk-image sentence: the installer script did not exist yet.)

After publishing, add a last line linking the release checklist issue.

The list for a specific release lives in that release's checklist issue, not here.

## Who cuts a release, and when

Manual, on demand. No cadence and no automated trigger.

## Step by step

All version and changelog work lands on ONE branch, `release/vX.Y.Z`, in one PR. Kai's PRs go
to `Performant-Labs/kai-private`, never upstream.

1. Run `scripts/release-preflight.sh vX.Y.Z` on the release Mac, in a clean checkout of `master`. It
   checks the machine (Apple Silicon macOS, tools, Go version, Kai not running) and the commit
   (clean, at `origin/master`, `origin` is the fork, CI green on that exact commit, the version
   free, `gh` able to push). Fix every FAIL first; record each WARN.
2. Confirm the release commit (usually `master`'s tip) has a green CI run.
3. Gather known issues now, before writing the CHANGELOG (see "Known issues": both labels, the
   whole open list, confirm each is still live, plus the standing lines).
4. Run `scripts/changelog-check.sh`. Give every PR it lists an entry under `[Unreleased]`, or a
   reasoned skip, until it prints `ok`.
5. Branch `release/vX.Y.Z` off the release commit.
6. Decide the bump from `CHANGELOG.md`'s `## [Unreleased]` (the first release is `0.1.0`).
7. `scripts/release-bump.sh X.Y.Z`.
8. Restructure `CHANGELOG.md`: `[Unreleased]` becomes `[X.Y.Z] - date`, Known Issues verbatim
   from step 3 (standing lines included), a fresh empty `[Unreleased]` above.
9. Commit, open a PR, get PR-Agent above 90, merge, confirm CI is green on the merge commit.
10. `git tag -a vX.Y.Z -m "vX.Y.Z"` on the merge commit, `git push origin vX.Y.Z`. (Holler signs
   its tags; no signing key is configured for this repo, so these are annotated only.)
11. Build on Apple Silicon **from that tag**, in a clean checkout:
    `make darwin-package VERSION=X.Y.Z`, then `scripts/release-tree-check.sh vX.Y.Z`.
12. Quit any running Kai, then `scripts/release-verify.sh bin/Kai.app X.Y.Z`. It must print PASS.
    (It launches the app, so the Dock icon bounces and Kai briefly takes focus.)
13. Run the manual smoke matrix (checklist) against the same `bin/Kai.app`.
14. `scripts/release-package.sh bin/Kai.app X.Y.Z`: the zip, the disk image and `SHA256SUMS`,
    each checked against the app. Package once, from the verified app; never rebuild.
15. Build the notes file: the CHANGELOG's `## [X.Y.Z]` section (minus the `changelog-skip`
    comment) with the Installing paragraph from "Release notes preface" in front. Nothing else
    is added: the updater and Accessibility warnings are already in its Known Issues.
16. **Confirm before publishing**, then
    `gh release create vX.Y.Z Kai-X.Y.Z-darwin-arm64.zip Kai-X.Y.Z-darwin-arm64.dmg SHA256SUMS
    --notes-file <notes>`.
17. **Verify the published artifact, not just the local build.** In a clean directory,
    `gh release download vX.Y.Z`, check the checksums, unzip with `ditto -x -k`, and run
    `scripts/release-verify.sh <that Kai.app> X.Y.Z` again. Mount the published disk image and
    check its `Kai.app` is identical to the unzipped one. Then run the installer against the real
    release: `scripts/install-release.sh vX.Y.Z --dest "$(mktemp -d)" --keep-permissions` must
    download, verify, install and clear the flag.

For the checklist to run each time, copy
[`docs/release-checklist.md`](release-checklist.md) into a new issue titled
`Release checklist: vX.Y.Z`. Do not check boxes on the template itself.
