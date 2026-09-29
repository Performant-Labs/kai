#!/usr/bin/env bash
# Set the release version in the two places a macOS build reads it from.
#
#   scripts/release-bump.sh X.Y.Z
#
# Edits exactly:
#   - build/config.yml         every `version: "..."` line (release.yml's own sed does the same)
#   - build/darwin/Info.plist  CFBundleShortVersionString and CFBundleVersion
#
# Deliberately NOT `make build-assets`: that regenerates nine files (Windows, Linux and iOS
# metadata included) from templates and drops hand-written content from Info.plist
# (verified when this script was written). A targeted edit keeps the diff to what a release
# needs.
set -euo pipefail

version="${1:-}"
if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "usage: $0 X.Y.Z   (plain SemVer, no leading v, no suffix)" >&2
  exit 2
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
cfg="$root/build/config.yml"
plist="$root/build/darwin/Info.plist"

sed -i.bak -E "s/^([[:space:]]*)version:[[:space:]]*\"[^\"]*\"/\1version: \"$version\"/" "$cfg"
rm -f "$cfg.bak"

# Text substitution, not PlistBuddy: PlistBuddy rewrites the whole file and drops the XML
# comments (tested: a 63-line diff for a 2-line change).
perl -0pi -e "s{(<key>CFBundleShortVersionString</key>\s*<string>)[^<]*(</string>)}{\${1}$version\${2}}; s{(<key>CFBundleVersion</key>\s*<string>)[^<]*(</string>)}{\${1}$version\${2}}" "$plist"

echo "version set to $version:"
grep -n "version:" "$cfg"
grep -A1 -E "CFBundle(ShortVersionString|Version)</key>" "$plist" | grep string
