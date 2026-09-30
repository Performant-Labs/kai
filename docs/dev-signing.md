# Signing builds with a stable certificate (so macOS grants survive rebuilds)

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

## Build with it

    make darwin-package VERSION=X.Y.Z KAI_SIGN_IDENTITY="Kai Dev"

`KAI_SIGN_IDENTITY` also works with `darwin-build`, `darwin-package-dmg`, `make dev`, and plain
`KAI_SIGN_IDENTITY="Kai Dev" wails3 task darwin:package`. Unset, the command is the historical
`codesign --force --deep --sign -`. If the named identity does not exist the build **fails**
with a message; it never silently falls back to ad-hoc.

## Check it

    scripts/sign-check.sh bin/Kai.app                    # prints kind, requirement, verdict
    scripts/sign-check.sh bin/Kai.app --require-stable   # exit 1 if the requirement is a binary hash

`scripts/release-verify.sh` prints the same kind as an informational line; it never fails a
release for being ad-hoc.

## One-time cleanup after switching identity

Old grants are bound to old hashes, so reset them once, then grant once in Settings > Shortcuts:

    tccutil reset Accessibility net.dtapp.kai
    tccutil reset ScreenCapture net.dtapp.kai
    tccutil reset ListenEvent net.dtapp.kai

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
