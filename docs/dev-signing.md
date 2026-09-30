# Signing builds with a stable certificate (so macOS grants survive rebuilds)

## Two apps: Kai (released) and Kai-dev (development)

The development build is a **different app** from the released one, so macOS keeps their
permissions apart and you can tell them apart:

|                     | released app             | development app             |
| ------------------- | ------------------------ | --------------------------- |
| name in Privacy & Security | `Kai`             | `Kai-dev`                   |
| bundle identifier   | `net.dtapp.kai`          | `net.dtapp.kai.dev`         |
| bundle              | `bin/Kai.app`            | `bin/Kai-dev.app`           |
| built with          | `make darwin-package VERSION=X.Y.Z` | `make dev-app`   |
| signed with         | ad-hoc unless `KAI_SIGN_IDENTITY` is set | the `Kai Dev` certificate, always |
| data folder         | `~/.kai`                 | `~/.kai.dev`                |
| single-instance ID  | `cnb.cool.dtapp.kai`     | `cnb.cool.dtapp.kai.dev`    |

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
`identifier "net.dtapp.kai" and certificate leaf = H"..."`, which does not change between
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
build test builds with `make darwin-package`: that is the released app's path (`net.dtapp.kai`,
`~/.kai`, ad-hoc unless told otherwise).

`make dev` (live reload) also runs the app as `bin/Kai-dev.app`; it signs ad-hoc unless
`KAI_SIGN_IDENTITY="Kai Dev"` is passed.

## Sign the released app with it (optional)

    make darwin-package VERSION=X.Y.Z KAI_SIGN_IDENTITY="Kai Dev"

`KAI_SIGN_IDENTITY` also works with `darwin-build`, `darwin-package-dmg`, `make dev`, and plain
`KAI_SIGN_IDENTITY="Kai Dev" wails3 task darwin:package`. Unset, the command is the historical
`codesign --force --deep --sign -`. If the named identity does not exist the build **fails**
with a message; it never silently falls back to ad-hoc.

## Check it

    scripts/sign-check.sh bin/Kai-dev.app                    # prints kind, requirement, verdict
    scripts/sign-check.sh bin/Kai-dev.app --require-stable   # exit 1 if the requirement is a binary hash
    codesign -dr - bin/Kai-dev.app    # expect: identifier "net.dtapp.kai.dev" and certificate leaf = H"..."

`scripts/release-verify.sh` prints the same kind as an informational line; it never fails a
release for being ad-hoc.

## One-time cleanup after switching identity

Old grants are bound to old hashes, so reset them once, then grant once in Settings > Shortcuts.
For the dev app (`net.dtapp.kai.dev`, a new identity: it has no grants yet, so this only clears
anything left from an earlier attempt):

    tccutil reset Accessibility net.dtapp.kai.dev
    tccutil reset ScreenCapture net.dtapp.kai.dev
    tccutil reset ListenEvent net.dtapp.kai.dev

For the released app, use `net.dtapp.kai` in the same three commands. Old rows named "Kai" that
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
