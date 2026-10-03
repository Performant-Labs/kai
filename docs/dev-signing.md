# Signing builds with a stable certificate (so macOS grants survive rebuilds)

## Two apps: Kai (released) and Kai-dev (development)

The development build is a **different app** from the released one, so macOS keeps their
permissions apart and you can tell them apart:

|                            | released app                        | development app                     |
| -------------------------- | ----------------------------------- | ----------------------------------- |
| name in Privacy & Security | `Kai`                               | `Kai-dev`                           |
| bundle identifier          | `com.performantlabs.kai`            | `com.performantlabs.kai.dev`        |
| bundle                     | `bin/Kai.app`                       | `bin/Kai-dev.app`                   |
| built with                 | `make darwin-package VERSION=X.Y.Z` | `make dev-app`                      |
| signed with                | ad-hoc unless `KAI_SIGN_IDENTITY` is set | the `Kai Dev` certificate, always |
| data folder                | `~/.kai`                            | `~/.kai.dev`                        |
| single-instance ID         | `com.performantlabs.kai`            | `com.performantlabs.kai.dev`        |

The dev identifier is the released one with `.dev` appended (`build/darwin/Info.dev.plist`), so it
follows if the released identifier is ever renamed. The executable inside both bundles is still
called `Kai` (it is the Go binary; the bundle name is what Privacy & Security shows).

In Settings > Privacy & Security, the row named **Kai-dev** is the development build and the row
named **Kai** is the released one; a grant on one does not apply to the other. Before this split
both were called "Kai" with the same identifier, so macOS treated them as one app and a dev build
could overwrite or reuse the released app's settings in `~/.kai`.

**Run only one of them at a time.** They have separate data folders and separate single-instance
locks, so nothing stops both from starting, but both register the same global hotkeys and both
listen for the double Cmd+C. The hotkeys use Carbon `RegisterEventHotKey`, which is exclusive: the
second registration is expected to fail (Kai logs `hotkey register failed` and the shortcut is
missing from its active list; not exercised headlessly), and the double Cmd+C listener is a
passive event tap, so both apps react to it. Quit one before starting the other. (Before the split,
launching one while the other ran did not start a second process at all: they shared a single-instance
lock, so the newcomer exited and brought the running app forward.)

What differs at runtime in a dev build (`buildinfo.IsDev()`): the data folder is `~/.kai.dev`, the
single-instance ID has a `.dev` suffix, the startup update check is skipped, analytics reporting is off (`analytics.EnableDevUpload` is the only switch), and the Swift bridge library is loaded from the source tree
first. Nothing in the UI (tray, window titles) changes.

Builds are signed ad-hoc by default. macOS identifies an ad-hoc app by the exact hash of its
binary, so **every rebuild is a different app**: Accessibility, Screen Recording and Input
Monitoring grants are lost, and the old rows in Privacy & Security stay bound to old hashes.

Signing with a stable certificate changes the requirement to
`identifier "com.performantlabs.kai" and certificate leaf = H"..."`, which does not change between
builds. This needs only a free certificate. It is **not** Developer ID or notarization (that
needs a paid membership) and does not help on other people's Macs.

## One-time: make the certificate

Keychain Access > Certificate Assistant > Create a Certificate...

- Name: `Kai Dev`
- Identity Type: Self Signed Root
- Certificate Type: Code Signing

(A free Apple Development certificate from Xcode also works; use its name, e.g.
`Apple Development: you@example.com (TEAMID)`, or its SHA-1 hash.)

## Build the dev app

    make dev-app                 # bin/Kai-dev.app, DEV=true, signed with "Kai Dev"

This is the command for test builds. It defaults `KAI_SIGN_IDENTITY` to `Kai Dev` (override it
with `make dev-app KAI_SIGN_IDENTITY="Apple Development: ..."`; an empty value still means
`Kai Dev`), fixes `DEV=true`, and checks first, before compiling anything, that the certificate
exists: if not, it stops with a message pointing here and **never** falls back to ad-hoc. Do not
build test builds with `make darwin-package`: that is the released app's path (`com.performantlabs.kai`,
`~/.kai`, ad-hoc unless told otherwise).

`make dev` (live reload) also runs the app as `bin/Kai-dev.app`; it signs ad-hoc unless
`KAI_SIGN_IDENTITY="Kai Dev"` is passed.

## Releases use a different certificate

Releases are signed with `Kai Release`, not `Kai Dev`: see "Signing, and what it costs users" in
[releasing.md](releasing.md) and `scripts/release-cert.sh`. The commands below are for signing a
local test build of the released app with the dev certificate.

## Sign a local build of the released app with it (optional)

    make darwin-package VERSION=X.Y.Z KAI_SIGN_IDENTITY="Kai Dev"

`KAI_SIGN_IDENTITY` also works with `darwin-build`, `darwin-package-dmg`, `make dev`, and plain
`KAI_SIGN_IDENTITY="Kai Dev" wails3 task darwin:package`. Unset, the command is the historical
`codesign --force --deep --sign -`. If the named identity does not exist the build **fails**
with a message; it never silently falls back to ad-hoc.

## Check it

    scripts/sign-check.sh bin/Kai-dev.app                    # prints kind, requirement, verdict
    scripts/sign-check.sh bin/Kai-dev.app --require-stable   # exit 1 if the requirement is a binary hash
    codesign -dr - bin/Kai-dev.app    # expect: identifier "com.performantlabs.kai.dev" and certificate leaf = H"..."

`scripts/release-verify.sh` prints the same kind as an informational line; it never fails a
release for being ad-hoc.

## The identifier changed from `net.dtapp.kai` (issue #27)

Releases before 0.4.0 used upstream's namespace, `net.dtapp.kai` (dev: `net.dtapp.kai.dev`); from
0.4.0 the identifiers are Performant Labs' own: `com.performantlabs.kai` and
`com.performantlabs.kai.dev`. To macOS the new app is a different one, so, once:

- Grant Accessibility ("Device Control and Data Access"), Screen Recording and Input Monitoring
  again in Settings > Shortcuts. Rows named "Kai" from the old identifier stay in Privacy &
  Security: remove them (the new app has its own row), or clear them with the same three
  `tccutil reset` commands as below, using `net.dtapp.kai` (and `net.dtapp.kai.dev` for the dev app).
- Quit the old Kai before opening the new one: the two do not share the single-instance lock, so
  both would run and fight over the tray and the hotkeys.
- Kai's own data (`~/.kai`, `~/.kai.dev`) does not depend on the identifier and carries over. The
  few interface preferences the web view remembers itself (the last Settings tab, a window's pin)
  are stored per identifier and start fresh.
- The `Kai Dev` certificate does not change. Its requirement is
  `identifier "..." and certificate leaf = H"..."`, so the leaf stays and only the identifier part is
  new: the dev app is a new identity with no grants, as in "One-time cleanup" below.

## One-time cleanup after switching identity

Old grants are bound to old hashes, so reset them once, then grant once in Settings > Shortcuts.
For the dev app (`com.performantlabs.kai.dev`, a new identity: it has no grants yet, so this only clears
anything left from an earlier attempt):

    tccutil reset Accessibility com.performantlabs.kai.dev
    tccutil reset ScreenCapture com.performantlabs.kai.dev
    tccutil reset ListenEvent com.performantlabs.kai.dev

For the released app, use `com.performantlabs.kai` in the same three commands. Old rows named "Kai" that
were granted to dev builds before the split belong to the released identifier: reset that one too
and grant the released app afresh.

After that, rebuilds signed with the same certificate keep the grants. Signing with a different
certificate (or going back to ad-hoc) is a different identity and needs the cleanup again.

## What has and has not been verified

Verified (headless, with a throwaway keychain that was deleted afterwards): two different
binaries with the same bundle id, signed with a self-signed code-signing certificate, print the
**same** designated requirement (`identifier ... and certificate leaf = H"..."`), whereas the
same two binaries signed ad-hoc print two different `cdhash` requirements. A freshly made
self-signed certificate is reported `CSSMERR_TP_NOT_TRUSTED` by `security find-identity` but
`codesign` signs with it fine.

**Not verified:** that macOS TCC actually keeps an Accessibility / Screen Recording / Input
Monitoring grant across a rebuild with this requirement. TCC cannot be exercised headlessly.
Apple documents that TCC matches an app by its code requirement, which is why a stable
requirement should work, but confirm on the Mac: grant once, rebuild with the same identity,
and check the grant is still on.
