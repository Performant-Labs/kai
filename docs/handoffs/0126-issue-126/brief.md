# Brief: #126 The installer clears the Accessibility grant on every update, although the signing identity is unchanged

## Problem

Seen twice (installing 0.6.0 and 0.6.2 with `scripts/install-release.sh`): the installer prints "This build has a new signature, so the old Accessibility grant no longer applies and was cleared", and the owner has to add Kai again under Privacy & Security > Device Control and Data Access. Until they do, the global shortcuts (Alt-A) cannot capture the selected text.

## Cause

Both builds are signed by the same `Kai Release` certificate (#36). On the installed 0.6.2 the designated requirement is `identifier "com.performantlabs.kai" and certificate root = H"cd95..."`, which does not change from build to build, so the grant should survive an update. The installer appears to decide by the app's code hash, which changes with every build, instead of by the designated requirement, so it resets a grant that is still valid (`scripts/install-release.sh`, the `tccutil reset Accessibility` branch near the end).

## Wanted

- Reset the Accessibility grant only when the designated requirement (signing identity) actually changed: compare `codesign -dr -` of the old and the new app.
- Say so when it keeps the grant ("same signing identity: the Accessibility grant is still valid"), and keep `--keep-permissions` working.
- If the old app was ad-hoc signed (its requirement is a code hash), keep the reset: that grant really is invalid for the new build.

## Done when

- A test for the comparison covers three cases: same identity, changed identity, ad-hoc to certificate.
- Checked on a real update from one release build to the next on a Mac: macOS keeps the grant and the installer does not clear it.
