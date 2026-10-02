#!/usr/bin/env bash
# Build, verify and package a release from a tag on this Mac, and create a DRAFT GitHub Release.
# This is docs/releasing.md steps 11 to 16 in one command.
#
#   scripts/release-local.sh vX.Y.Z [--dry-run] [--yes] [--out DIR]
#
#   --dry-run  build, verify and package, but create no release
#   --yes      do not ask before creating the draft (needed when there is no terminal)
#   --out DIR  keep the files in DIR (default: bin/release/vX.Y.Z in this checkout, git-ignored)
#
# Run it from your checkout, after the release PR is merged and the tag is pushed (steps 1 to
# 10). It never touches your working tree: it builds in a fresh detached worktree of the tag, so
# the result is the tag as committed, whatever else is checked out. Why a Mac and not a hosted
# runner: the app targets the newest Apple SDK, which GitHub's runners do not have yet (the first
# run of release-kai.yml failed to compile apple_correct.swift for that reason, issue #31).
#
# It stops at the first problem. It never deletes or overwrites a release. A draft is visible
# only to people who can push to the repository: run the smoke matrix
# (docs/release-checklist.md) on the Kai.app it leaves in DIR, then publish with
# `gh release edit vX.Y.Z --draft=false`.
set -euo pipefail

repo="${GH_REPO:-Performant-Labs/kai}"
here="$(cd "$(dirname "$0")" && pwd)"
tag=""; dry=0; yes=0; out=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) dry=1 ;;
    --yes) yes=1 ;;
    --out) [[ $# -ge 2 && -n "$2" ]] || { echo "--out needs a directory" >&2; exit 2; }; out="$2"; shift ;;
    -h|--help) sed -n '2,19p' "$0"; exit 0 ;;
    v[0-9]*.[0-9]*.[0-9]*) tag="$1" ;;
    *) echo "unknown argument: $1 (usage: $0 vX.Y.Z [--dry-run] [--yes] [--out DIR])" >&2; exit 2 ;;
  esac
  shift
done
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "usage: $0 vX.Y.Z [--dry-run] [--yes] [--out DIR]" >&2; exit 2; }
ver="${tag#v}"
# Make --out absolute now, against the directory the script was started in: the build runs in a
# throwaway worktree, which is deleted at the end, so a relative path would be lost with it.
if [[ -n "$out" ]]; then mkdir -p "$out" && out="$(cd "$out" && pwd)" || { echo "cannot create --out $out" >&2; exit 2; }; fi
die() { echo "FAIL: $*" >&2; exit 1; }
step() { printf '\n== %s\n' "$*"; }

root="$(git rev-parse --show-toplevel 2>/dev/null)" || die "run this inside a checkout of $repo"
cd "$root"
[[ -n "$out" ]] || out="$root/bin/release/$tag"

step "machine"
[[ "$(uname -s)" == "Darwin" && "$(uname -m)" == "arm64" ]] || die "releases are built on macOS Apple Silicon (this is $(uname -s) $(uname -m); a Rosetta shell counts as x86_64)"
missing=()
for t in git gh go pnpm node wails3 jq make ditto codesign shasum perl hdiutil; do command -v "$t" >/dev/null 2>&1 || missing+=("$t"); done
xcrun --find swiftc >/dev/null 2>&1 || missing+=("swiftc (Xcode command line tools)")
[[ ${#missing[@]} -eq 0 ]] || die "missing tools: ${missing[*]}"
! pgrep -x Kai >/dev/null 2>&1 || die "Kai is running: quit it (release-verify.sh launches its own copy)"
git remote get-url origin | grep -Eqi "github\.com[:/]${repo}(\.git)?$" || die "origin is not $repo"
gh auth status >/dev/null 2>&1 || die "gh is not logged in"
[[ "$(gh api "repos/$repo" --jq .permissions.push 2>/dev/null)" == "true" || $dry -eq 1 ]] || die "gh cannot push to $repo (a draft release needs it)"
echo "ok"

step "tag and gates"
git fetch -q --tags origin
local_sha="$(git rev-parse -q --verify "refs/tags/$tag^{commit}")" || die "no tag $tag: push it first (docs/releasing.md step 10)"
remote_sha="$(git ls-remote origin "refs/tags/$tag^{}" | cut -f1)"
[[ -n "$remote_sha" ]] || remote_sha="$(git ls-remote origin "refs/tags/$tag" | cut -f1)"   # lightweight tag
[[ "$remote_sha" == "$local_sha" ]] || die "$tag is ${local_sha:0:7} here but '${remote_sha:0:7}' on origin: push the tag, or fetch it"
if [[ $dry -eq 1 ]]; then "$here/release-gates.sh" "$tag" --no-release-check; else "$here/release-gates.sh" "$tag"; fi \
  || die "a gate failed (see above)"

work="$(mktemp -d)"
cleanup() { git -C "$root" worktree remove --force "$work/tree" >/dev/null 2>&1 || true; rm -rf "$work"; }
trap cleanup EXIT
step "fresh worktree of $tag"
git worktree add -q --detach "$work/tree" "$tag"
cd "$work/tree"

step "version files and CHANGELOG say $ver"
"$here/release-check-version.sh" "$ver" .

step "release notes"
mkdir -p "$out/assets"
"$here/release-notes.sh" "$ver" >"$out/RELEASE_NOTES.md"
echo "written: $out/RELEASE_NOTES.md"

step "dependencies"
make deps

# docs/releasing.md step 11. VERSION must be passed (the default is `git describe`, which would
# put a `v` in the version the app reports). The environment is cleaned of anything the Makefile
# would bake into the binary or that would change the signature: releases are ad-hoc signed and
# carry no token.
step "build Kai.app (this takes a few minutes)"
env -u GITHUB_TOKEN -u CNB_TOKEN -u POSTHOG_TOKEN -u POSTHOG_PROJECT_ID -u KAI_SIGN_IDENTITY \
  make darwin-package VERSION="$ver"

step "the tree is the tag"
"$here/release-tree-check.sh" "$tag"

step "verify (launches the app for 20 seconds)"
"$here/release-verify.sh" bin/Kai.app "$ver"

step "package"
"$here/release-package.sh" bin/Kai.app "$ver" "$out/assets"
rm -rf "$out/Kai.app"
ditto bin/Kai.app "$out/Kai.app"
echo "the verified app, for the smoke matrix: $out/Kai.app"

if [[ $dry -eq 1 ]]; then
  printf '\nDry run done: no release created. Files are in %s\n' "$out"
  exit 0
fi

step "draft release"
if [[ $yes -ne 1 ]]; then
  [[ -t 0 ]] || die "no terminal to ask on: pass --yes to create the draft"
  read -r -p "Create a DRAFT release $tag on $repo with the files in $out/assets? [y/N] " ans
  [[ "$ans" == "y" || "$ans" == "Y" ]] || { echo "not created. Files are in $out"; exit 1; }
fi
gh release create "$tag" "$out"/assets/* --repo "$repo" --draft --verify-tag \
  --title "Kai $tag" --notes-file "$out/RELEASE_NOTES.md"
cat <<MSG

Draft created. Next:
  1. Run the smoke matrix (docs/release-checklist.md) on $out/Kai.app
  2. Publish:  gh release edit $tag --repo $repo --draft=false
  3. Check the published release: scripts/install-release.sh $tag --dest "\$(mktemp -d)" --keep-permissions
MSG
