#!/usr/bin/env bash
# Install a Kai release, without macOS blocking it as coming from an unidentified developer.
#
#   scripts/install-release.sh [vX.Y.Z]        download that release with gh (default: the latest)
#   scripts/install-release.sh --from FILE     install from a .zip or .dmg you downloaded yourself
#
#   --dest DIR            install into DIR instead of /Applications
#   --keep-permissions    never touch the Accessibility grant
#
# Why this exists: a file downloaded by a browser gets the com.apple.quarantine flag, and macOS
# refuses to open a quarantined, ad-hoc-signed app. `gh release download` sets no flag, so the
# default path here already avoids it, and --from clears the flag on a file you got the other way.
# Removing the flag from a build made by this project's own release process, on your own Mac, is
# ordinary; it does not make the app trusted for anyone else. The real fix is a Developer ID
# signature plus notarization, which this fork does not have (see docs/releasing.md).
#
# What it does: gets the zip (verifying its checksum when one is available), extracts Kai.app,
# checks the signature and bundle id, replaces the installed app, clears the quarantine flag from
# the installed copy, and, only if the app's code actually changed, resets the now-invalid
# Accessibility grant so you can add Kai again. It refuses to replace a running Kai.
set -euo pipefail

repo="${KAI_RELEASE_REPO:-Performant-Labs/kai-private}"
bundle_id="${KAI_INSTALL_BUNDLE_ID:-net.dtapp.kai}"   # overridable so tests never touch real permissions
dest="/Applications"
tag=""
from=""
keep_perms=0

while [[ $# -gt 0 ]]; do
  case "$1" in
    --from) from="${2:-}"; shift ;;
    --dest) dest="${2:-}"; shift ;;
    --keep-permissions) keep_perms=1 ;;
    -h|--help) sed -n '2,23p' "$0"; exit 0 ;;
    v[0-9]*.[0-9]*.[0-9]*) tag="$1" ;;
    *) echo "unknown argument: $1 (see --help)" >&2; exit 2 ;;
  esac
  shift
done
[[ -n "$from" && -n "$tag" ]] && { echo "give a version or --from, not both" >&2; exit 2; }
[[ -d "$dest" ]] || { echo "FAIL: $dest is not a directory" >&2; exit 1; }
target="$dest/Kai.app"

die() { echo "FAIL: $*" >&2; exit 1; }

# Never replace a running Kai (a Kai running from somewhere else does not matter).
for pid in $(pgrep -x Kai 2>/dev/null || true); do
  exe="$(ps -p "$pid" -o comm= 2>/dev/null || true)"
  if [[ "$exe" == "$target/"* ]]; then
    die "Kai is running from $target. Quit it first, then run this again."
  fi
done

work="$(mktemp -d)"
mnt=""
detach() { hdiutil detach -quiet "$1" >/dev/null 2>&1 || { sleep 1; hdiutil detach -force -quiet "$1" >/dev/null 2>&1; }; }
cleanup() { [[ -n "$mnt" ]] && detach "$mnt" || true; rm -rf "$work"; }
trap cleanup EXIT

# 1. Get the archive.
if [[ -n "$from" ]]; then
  [[ -f "$from" ]] || die "no such file: $from"
  src="$from"
  xattr -d com.apple.quarantine "$src" 2>/dev/null || true
  sums="$(dirname "$src")/SHA256SUMS"
  if [[ -f "$sums" ]] && grep -q " $(basename "$src")\$" "$sums"; then
    (cd "$(dirname "$src")" && grep " $(basename "$src")\$" SHA256SUMS | shasum -a 256 -c - >/dev/null 2>&1) \
      || die "checksum mismatch for $src against $sums: do not install this file"
    echo "checksum ok (against $sums)"
  else
    echo "WARN: no SHA256SUMS next to $src, so its checksum was not verified"
  fi
else
  command -v gh >/dev/null 2>&1 || die "gh is not installed (or use --from FILE)"
  [[ -n "$tag" ]] || tag="$(gh release view --repo "$repo" --json tagName --jq .tagName)" || die "could not find the latest release"
  echo "downloading $tag from $repo"
  gh release download "$tag" --repo "$repo" --dir "$work/dl" --pattern '*.zip' --pattern SHA256SUMS >/dev/null \
    || die "could not download $tag"
  # `|| true`: with no zip, ls fails and pipefail + set -e would end the script without a message.
  src="$(ls "$work"/dl/*.zip 2>/dev/null | head -1 || true)"
  [[ -n "$src" ]] || die "$tag has no zip asset"
  (cd "$work/dl" && grep " $(basename "$src")\$" SHA256SUMS | shasum -a 256 -c - >/dev/null 2>&1) \
    || die "checksum mismatch for the downloaded $(basename "$src")"
  echo "checksum ok"
fi

# 2. Extract Kai.app from a zip or a disk image.
mkdir -p "$work/x"
case "$src" in
  *.zip) ditto -x -k "$src" "$work/x" ;;
  *.dmg)
    mnt="$work/mnt"; mkdir -p "$mnt"
    attached=0
    for _ in 1 2 3; do
      if hdiutil attach -readonly -nobrowse -noverify -mountpoint "$mnt" "$src" >/dev/null 2>&1; then attached=1; break; fi
      sleep 2
    done
    [[ $attached -eq 1 ]] || die "could not mount $src"
    [[ -d "$mnt/Kai.app" ]] || die "no Kai.app inside $src"
    ditto "$mnt/Kai.app" "$work/x/Kai.app"
    detach "$mnt" && mnt=""
    ;;
  *) die "expected a .zip or .dmg, got $src" ;;
esac
new="$work/x/Kai.app"
[[ -d "$new" ]] || die "no Kai.app in $src"

# 3. Check what we are about to install.
plist="$new/Contents/Info.plist"
got_id="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleIdentifier' "$plist" 2>/dev/null || true)"
[[ "$got_id" == "$bundle_id" ]] || die "bundle id is '$got_id', expected $bundle_id: not a Kai build"
codesign --verify --deep --strict "$new" 2>/dev/null || die "the code signature is not valid: do not install this"
version="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$plist")"

# 4. Did the code change compared with what is installed? (decides about the permission grant)
changed=1
if [[ -x "$target/Contents/MacOS/Kai" ]]; then
  old_sum="$(shasum -a 256 <"$target/Contents/MacOS/Kai")"
  new_sum="$(shasum -a 256 <"$new/Contents/MacOS/Kai")"
  [[ "$old_sum" == "$new_sum" ]] && changed=0
fi

# 5. Replace, and clear the quarantine flag from the installed copy.
rm -rf "$target"
ditto "$new" "$target"
xattr -dr com.apple.quarantine "$target" 2>/dev/null || true
if xattr -lr "$target" 2>/dev/null | grep -q com.apple.quarantine; then
  die "installed, but the quarantine flag is still on $target"
fi
codesign --verify --deep --strict "$target" 2>/dev/null || die "the installed copy's signature is not valid"
echo "installed Kai $version at $target (quarantine flag cleared, signature valid)"

# 6. The permission grant.
if [[ $keep_perms -eq 1 ]]; then
  echo "left the Accessibility grant alone (--keep-permissions)"
elif [[ $changed -eq 0 ]]; then
  echo "same build as the one that was installed: the Accessibility grant is still valid"
else
  tccutil reset Accessibility "$bundle_id" >/dev/null 2>&1 || true
  echo "This build has a new signature, so the old Accessibility grant no longer applies and was cleared."
  echo "Open Kai, then add it again under Privacy & Security (on macOS 27: \"Device Control and Data Access\")."
fi
echo "Open it with: open \"$target\""
