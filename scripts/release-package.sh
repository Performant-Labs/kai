#!/usr/bin/env bash
# Package a verified Kai.app for a release: the zip, the disk image, and SHA256SUMS.
#
#   scripts/release-package.sh /path/to/Kai.app X.Y.Z [outdir]
#
# It packages the app you hand it and NEVER rebuilds. Every build embeds its build time and
# regenerates Assets.car, so a rebuild is a different binary from the one release-verify.sh
# approved; the zip and the disk image must both contain exactly that one.
#
# Produces, in outdir (default: the current directory):
#   Kai-X.Y.Z-darwin-arm64.zip   made with ditto (a plain zip can break the signature)
#   Kai-X.Y.Z-darwin-arm64.dmg   Kai.app plus an Applications shortcut, to drag into
#   SHA256SUMS                   covering both
# and then checks what it made: each archive unpacks to an app identical to the input with a
# valid signature, and the checksums verify. Exits 1 on the first problem.
set -euo pipefail

app="${1:-}"; ver="${2:-}"; out="${3:-.}"
[[ -n "$app" && -n "$ver" ]] || { echo "usage: $0 /path/to/Kai.app X.Y.Z [outdir]" >&2; exit 2; }
[[ "$ver" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "version must be plain X.Y.Z, got '$ver'" >&2; exit 2; }
die() { echo "FAIL: $*" >&2; exit 1; }

[[ -d "$app/Contents" ]] || die "$app is not an app bundle"
have="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$app/Contents/Info.plist" 2>/dev/null || true)"
[[ "$have" == "$ver" ]] || die "the app reports version '$have', not $ver (package the app that was built for this release)"
codesign --verify --deep --strict "$app" 2>/dev/null || die "the app's signature is not valid"
mkdir -p "$out"; out="$(cd "$out" && pwd)"

name="Kai-$ver-darwin-arm64"
work="$(mktemp -d)"
mnt=""
detach() { hdiutil detach -quiet "$1" >/dev/null 2>&1 || { sleep 1; hdiutil detach -force -quiet "$1" >/dev/null 2>&1; }; }
cleanup() { [[ -n "$mnt" ]] && detach "$mnt" || true; rm -rf "$work"; }
trap cleanup EXIT

echo "packaging $app (version $ver)"

# The zip.
rm -f "$out/$name.zip"
ditto -c -k --keepParent "$app" "$out/$name.zip"

# The disk image, from a copy of the same app.
mkdir "$work/stage"
ditto "$app" "$work/stage/Kai.app"
ln -s /Applications "$work/stage/Applications"
rm -f "$out/$name.dmg"
hdiutil create -volname "Kai $ver" -srcfolder "$work/stage" -ov -format UDZO "$out/$name.dmg" >/dev/null \
  || die "could not create the disk image"

# The checksums, over both.
( cd "$out" && shasum -a 256 "$name.zip" "$name.dmg" >SHA256SUMS )

echo "verifying what was made"
( cd "$out" && shasum -a 256 -c SHA256SUMS >/dev/null ) || die "SHA256SUMS does not verify"

mkdir "$work/unzip"
ditto -x -k "$out/$name.zip" "$work/unzip"
diff -r "$work/unzip/Kai.app" "$app" >/dev/null || die "the zip does not contain an app identical to the input"
codesign --verify --deep --strict "$work/unzip/Kai.app" 2>/dev/null || die "the zip's app has an invalid signature"
echo "  zip: the app inside is identical to the input, signature valid"

mnt="$work/mnt"; mkdir "$mnt"
# A first attach right after creating the image has failed once with "Resource temporarily
# unavailable", so try a few times.
attached=0
for _ in 1 2 3 4; do
  if hdiutil attach -readonly -nobrowse -noverify -mountpoint "$mnt" "$out/$name.dmg" >/dev/null 2>&1; then attached=1; break; fi
  sleep 2
done
[[ $attached -eq 1 ]] || die "could not mount the disk image"
diff -r "$mnt/Kai.app" "$app" >/dev/null || die "the disk image does not contain an app identical to the input"
codesign --verify --deep --strict "$mnt/Kai.app" 2>/dev/null || die "the disk image's app has an invalid signature"
[[ -L "$mnt/Applications" ]] || die "the disk image has no Applications shortcut"
detach "$mnt" && mnt=""
echo "  dmg: the app inside is identical to the input, signature valid, Applications shortcut present"

echo "done, in $out:"
( cd "$out" && ls -l "$name.zip" "$name.dmg" SHA256SUMS | awk '{printf "  %-34s %10d bytes\n", $9, $5}' )
