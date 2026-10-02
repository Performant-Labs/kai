#!/usr/bin/env bash
# Check that a tag's tree, its version files and its CHANGELOG all say the same version.
#
#   scripts/release-check-version.sh X.Y.Z [repo-root]
#
# docs/releasing.md: the tree at the tag must build an app that reports X.Y.Z, so the release
# commit sets the version in build/config.yml and build/darwin/Info.plist, and the CHANGELOG
# has a `## [X.Y.Z]` section. A build of a tree that disagrees would ship a mislabelled app.
# Runs on Linux and macOS (no PlistBuddy). Reports every mismatch, exits 1 if there is any.
set -uo pipefail

ver="${1:-}"
root="${2:-.}"
[[ "$ver" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "usage: $0 X.Y.Z [repo-root]" >&2; exit 2; }

bad=0
fail() { echo "FAIL: $*" >&2; bad=1; }

cfg="$root/build/config.yml"
plist="$root/build/darwin/Info.plist"
changelog="$root/CHANGELOG.md"

# Every `version: "..."` value in config.yml (release-bump.sh sets them all).
if [[ -f "$cfg" ]]; then
  vals="$(sed -nE 's/^[[:space:]]*version:[[:space:]]*"([^"]*)".*/\1/p' "$cfg")"
  [[ -n "$vals" ]] || fail "no version: lines found in build/config.yml"
  while IFS= read -r v; do
    [[ -z "$v" || "$v" == "$ver" ]] || fail "build/config.yml has version \"$v\", expected \"$ver\""
  done <<<"$vals"
else
  fail "build/config.yml is missing"
fi

if [[ -f "$plist" ]]; then
  for key in CFBundleShortVersionString CFBundleVersion; do
    v="$(perl -0ne "print \$1 if m{<key>$key</key>\s*<string>([^<]*)</string>}" "$plist")"
    [[ "$v" == "$ver" ]] || fail "build/darwin/Info.plist $key is '${v:-missing}', expected '$ver'"
  done
else
  fail "build/darwin/Info.plist is missing"
fi

if [[ -f "$changelog" ]]; then
  grep -qE "^## \[$ver\]" "$changelog" || fail "CHANGELOG.md has no '## [$ver]' section"
else
  fail "CHANGELOG.md is missing"
fi

[[ $bad -eq 0 ]] && echo "ok: build/config.yml, Info.plist and CHANGELOG.md all say $ver"
exit $bad
