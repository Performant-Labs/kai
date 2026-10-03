# Releasing

How to cut a release of Kai from `Performant-Labs/kai` (public). Adapted from Holler's
`docs/releasing.md`; the differences are called out where they matter. Decision record: issue
[#177](https://github.com/Performant-Labs/kai-private/issues/177) (archived repository).

This repo is the public home of Kai and began as a fork of `dtapps/kai` (GitHub still lists it as a fork). A release is one macOS Apple Silicon app,
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
`0.1.0`. It does not track upstream's numbers (see "The in-app updater"; installed apps do not see this fork's releases yet).

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

Releases are signed with a **self-signed certificate, "Kai Release"**, made once on the release Mac
with `scripts/release-cert.sh` ([#36](https://github.com/Performant-Labs/kai/issues/36)); before
0.4.0 they were ad-hoc signed. `scripts/release-local.sh` signs with it, stops if it is missing,
never falls back to ad-hoc (`--ad-hoc` asks for that on purpose), and requires the result to be
stable (`scripts/sign-check.sh --require-stable`). This fork still has no Apple Developer ID
certificate or notarization.

- **What the certificate fixes.** An ad-hoc signature's code requirement is the binary's hash, so
  every build is a new identity to macOS and installing one silently invalidates the Accessibility
  grant (on macOS 27 it is under "Device Control and Data Access"): hotkeys register but the
  simulated copy returns an empty clipboard. A certificate's requirement is
  `identifier "com.performantlabs.kai" and certificate root = H"<its SHA-1>"` (a self-signed
  certificate is its own root), which does not change between builds, so grants survive rebuilds on
  the release Mac. **Unverified:** that they also survive an update on another person's Mac (this
  repo's earlier notes say a self-signed certificate "does not help on other people's Macs"); #36
  tracks checking it with two releases on a second Mac. Until then keep the standing Known Issues
  line below.
- **The key is the identity.** Lose it and every user grants permissions again once; leak it and
  anyone can sign an app macOS treats as the same identity (it would still not pass Gatekeeper as
  trusted). Back it up as a `.p12` in the password manager and keep it only on the release Mac. A
  certificate made with `release-cert.sh` lasts 10 years.
- **Gatekeeper.** A self-signed certificate is not trusted, so a file downloaded by a browser gets
  the `com.apple.quarantine` flag and macOS blocks it as coming from an unidentified developer.
  Checked: `gh release download` does not set the flag, and a disk image does not help (the app
  inside is still blocked). `scripts/install-release.sh` is the supported way to install a release
  without the block: it downloads with `gh` (no flag), or with `--from FILE` takes a zip or disk
  image you already downloaded and clears the flag from it. Either way it verifies the checksum
  when one is available, checks the bundle id and signature, refuses to replace a running Kai,
  clears the flag on the installed copy, and clears the stale Accessibility grant **only if the
  app's code changed**. Removing the flag from a build made by this project's own release
  process, on your own Mac, is ordinary; it does not make the app trusted for anyone else.
- **The real fix is not available yet:** a Developer ID signature plus notarization. It needs an
  Apple Developer Program membership; the release Mac has no Developer ID certificate and
  1Password has none either. It would remove the Gatekeeper block. Until then, say in the notes
  how to install.
- **Developer builds** use the separate `Kai Dev` certificate and the `Kai-dev` app
  (`com.performantlabs.kai.dev`, `make dev-app`): see [dev-signing.md](dev-signing.md).

## The in-app updater (public repository)

The updater polls `Performant-Labs/kai` (`buildinfo.UpdaterGithubRepo`), never upstream
`dtapps/kai`, and the CNB (cnb.cool) source is gone ([#178](https://github.com/Performant-Labs/kai-private/issues/178), archived repository).
Consequences:

1. An installed app can no longer be offered upstream's build over this fork's.
2. The repository is public, so an anonymous update check can read its releases without a token;
   do not ship a token inside the app. Until the first release exists there (and the updater has
   been seen to find it on a real install, the remaining part of
   [#196](https://github.com/Performant-Labs/kai-private/issues/196), archived repository), an
   update check finds nothing, reads it as "no update", and the app stays quiet (a debug log
   line, no warning, no dialog). Builds installed before that point never see an update; users
   install those by hand.
3. A release with no `updater-` asset or no `SHA256SUMS` (next point) is also invisible to the
   updater, even on a public repository.
4. Even on a readable repo the updater ignores an asset unless its name is
   `updater-<anything>-<platform>-<arch>.zip` (also `.tar.gz`/`.tgz`; the lower-cased name starts
   with `updater-` and contains `darwin` and `arm64` for this Mac build), and it needs `SHA256SUMS`
   to list that exact file name. `release-package.sh` produces
   `updater-Kai-X.Y.Z-darwin-arm64.zip` for that reason: a copy of the release zip, same bytes,
   the same top-level `Kai.app`. The name is defined once, on the `updater_zip=` line of
   `scripts/release-package.sh`, and `pkg/wails-updater-providers/updater_asset_test.go` reads that
   line and checks the updater's matcher accepts it, so renaming it in either place fails a test.
   This is in place on the release side only; the first public release and a real update check are still to come (2 above).

So there is nothing to warn about for the updater at release time, but the release notes must not
promise auto-update.

## What a release produces

1. **A git tag + `CHANGELOG.md` entry.** `vX.Y.Z`, annotated. The `## [Unreleased]` section
   becomes `## [X.Y.Z] - YYYY-MM-DD` and stays hand-written, not generated from commit messages.
2. **A GitHub Release** on `Performant-Labs/kai` with four assets, all made by
   `scripts/release-package.sh bin/Kai.app X.Y.Z`:
   - `Kai-X.Y.Z-darwin-arm64.zip`, made with `ditto` (a plain `zip` can break the signature).
   - `Kai-X.Y.Z-darwin-arm64.dmg`, a disk image with `Kai.app` and an Applications shortcut. A
     `.app` is a folder and a release asset must be one file, so it has to be archived; the zip
     unpacks to `Kai.app` and the disk image is the usual Mac form.
   - `updater-Kai-X.Y.Z-darwin-arm64.zip`, a byte-identical copy of the zip under the name the
     in-app updater requires (see "The in-app updater"). Do not rename or drop it. The installer
     script skips it and uses the plain zip.
   - `SHA256SUMS`, covering the zip, the updater zip and the disk image.

   The script packages the app it is given and **never rebuilds**: every build embeds its build
   time and regenerates `Assets.car`, so a rebuild is a different binary from the one
   `release-verify.sh` approved. It checks that each archive unpacks to an app identical to the
   input with a valid signature. Package **once** and do not repackage after publishing: the zip
   is reproducible (identical hash on a second run), the disk image is not (it embeds timestamps).

   This is outward-facing. Confirm with whoever is driving the release before publishing, every
   time.

**`.github/workflows/release.yml` is not used for this fork's releases** (see "Automated release (draft)" below for the one that is). It is upstream's,
kept unedited so upstream changes still merge cleanly. It builds four platforms on billed
hosted runners (three of which this fork does not ship), expects CNB, PostHog and
signing secrets this fork does not have (empty tokens would be baked into the binary), and its
release step targets a channel no installed app watches. Revisit it if this fork gains a
Developer ID and a working update channel.

## Automated release (draft)

**`scripts/release-local.sh vX.Y.Z` does steps 11 to 16 in one command, on your Mac.** Run it from
a checkout of `master` after the release PR is merged and the tag is pushed (steps 1 to 10):

```bash
scripts/release-local.sh v0.3.1 --dry-run   # build, verify, package; create nothing
scripts/release-local.sh v0.3.1             # the same, then asks before creating the DRAFT release
```

It builds in a fresh detached worktree of the tag, so your working tree is never touched and the
result is the tag as committed. In order it:

1. checks the machine (Apple Silicon, tools, Kai not running, `gh` logged in with push rights);
2. checks the tag is the same commit here and on `origin`, then runs `scripts/release-gates.sh`:
   the commit is on `origin/master`, every CI check on that exact commit passed, and no release
   (draft included) exists for the tag (it never deletes or overwrites one, unlike upstream's workflow);
3. checks the version files and CHANGELOG say the tag's version (`release-check-version.sh`) and
   builds the notes (`release-notes.sh`: the Installing paragraph below plus the CHANGELOG section);
4. builds with `make darwin-package VERSION=X.Y.Z`, with no token in the environment, then runs
   `release-tree-check.sh`, `release-verify.sh` and `release-package.sh`;
5. asks, then creates a **draft** release with the four assets. The files, the notes and the
   verified `Kai.app` stay in `bin/release/vX.Y.Z/`.

What stays manual, on purpose:

- **Step 13, the smoke matrix.** CI cannot see hotkeys, windows or permissions (#163). Run it on
  the `Kai.app` the script leaves in `bin/release/vX.Y.Z/`, then publish the draft:
  `gh release edit vX.Y.Z --repo Performant-Labs/kai --draft=false`. The draft is the confirmation
  step 16 asks for.
- **Step 17**, checking the published artifact and running the installer against it.
- **Steps 1 to 10**, the release branch, PR and tag. A workflow that opens the release PR needs a
  token that can start CI on it, which `GITHUB_TOKEN` cannot; that is a later phase of
  [#31](https://github.com/Performant-Labs/kai/issues/31).

**Why not a hosted runner.** `.github/workflows/release-kai.yml` does the same on a GitHub-hosted
runner when a `vX.Y.Z` tag is pushed, but it cannot build the app today: the first run (v0.3.1)
failed to compile `apple_correct.swift` because the app targets a newer Apple SDK than the
`macos-26` runner has. It stays in the repository, using the same `release-gates.sh`, for when a
runner with the right SDK exists; until then a tag push runs it and it fails at the build step,
after the gates, without creating anything.

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

- Search **both** labels: `gh issue list --repo Performant-Labs/kai --label bug --state
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

Standing Known Issues lines (in the CHANGELOG's Known Issues, reworded to that release's facts):

- Permissions are granted to the app's signing identity. Releases are signed with the stable
  "Kai Release" certificate, so later updates should keep them, but that is not verified on other
  Macs yet ([#36](https://github.com/Performant-Labs/kai/issues/36)): say so, and say to remove Kai
  from the list and add it again if Alt+A stops copying after an update. A change of the app
  identifier or the certificate resets them for everyone once; say that in the release that does it.

Because release notes are extracted from the CHANGELOG, this is how the Accessibility warning
reaches the notes without anyone writing it separately at publish time. The third
consequence of ad-hoc signing, the Gatekeeper quarantine, is not a known issue but an install
instruction, so it lives in the release notes preface (next section).

## Release notes preface

The release notes are the CHANGELOG's `## [X.Y.Z]` section, minus the internal
`<!-- changelog-skip: ... -->` comment, with this one paragraph in front and nothing else. It
answers the first question a reader has (how do I install it, and why does Alt+A not work) and
carries the Gatekeeper warning. Fill in the version:

```markdown
**Installing.** macOS on Apple Silicon only. Download with `gh release download vX.Y.Z --repo Performant-Labs/kai`: a browser download is quarantined by macOS and blocked as coming from an unidentified developer (if that happens, run `xattr -dr com.apple.quarantine /Applications/Kai.app`). Unzip, move `Kai.app` to `/Applications`, and check the zip against `SHA256SUMS`. This build is signed with a self-signed certificate, not a Developer ID. Grant Kai access in Privacy & Security (on macOS 27 the list is "Device Control and Data Access") before Alt+A can copy text; if an older Kai left a row there, remove it first. The app is attached twice: `Kai-X.Y.Z-darwin-arm64.zip`, and `Kai-X.Y.Z-darwin-arm64.dmg`, a disk image with `Kai.app` inside to drag to Applications. (`updater-Kai-X.Y.Z-darwin-arm64.zip` is the same zip under the name the in-app updater needs; you can ignore it.) If you have a checkout of the repo, `scripts/install-release.sh vX.Y.Z` downloads and installs it without the quarantine block.
```

(v0.1.0's published notes stop after the disk-image sentence: the installer script did not exist yet.)

After publishing, add a last line linking the release checklist issue.

The list for a specific release lives in that release's checklist issue, not here.

## Who cuts a release, and when

Manual, on demand. No cadence and no automated trigger.

## Step by step

All version and changelog work lands on ONE branch, `release/vX.Y.Z`, in one PR. Kai's PRs go
to `Performant-Labs/kai`, never upstream.

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
14. `scripts/release-package.sh bin/Kai.app X.Y.Z`: the zip, the updater zip, the disk image and
    `SHA256SUMS`, each checked against the app. Package once, from the verified app; never rebuild.
15. Build the notes file: the CHANGELOG's `## [X.Y.Z]` section (minus the `changelog-skip`
    comment) with the Installing paragraph from "Release notes preface" in front. Nothing else
    is added: the updater and Accessibility warnings are already in its Known Issues.
16. **Confirm before publishing**, then
    `gh release create vX.Y.Z Kai-X.Y.Z-darwin-arm64.zip updater-Kai-X.Y.Z-darwin-arm64.zip
    Kai-X.Y.Z-darwin-arm64.dmg SHA256SUMS --notes-file <notes>`.
17. **Verify the published artifact, not just the local build.** In a clean directory,
    `gh release download vX.Y.Z`, check the checksums (all three files), unzip with `ditto -x -k`, and run
    `scripts/release-verify.sh <that Kai.app> X.Y.Z` again. Mount the published disk image and
    check its `Kai.app` is identical to the unzipped one, and that the updater zip is byte-identical
    to the release zip (`cmp`). Then run the installer against the real
    release: `scripts/install-release.sh vX.Y.Z --dest "$(mktemp -d)" --keep-permissions` must
    download, verify, install and clear the flag.

For the checklist to run each time, copy
[`docs/release-checklist.md`](release-checklist.md) into a new issue titled
`Release checklist: vX.Y.Z`. Do not check boxes on the template itself.
