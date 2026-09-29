#!/usr/bin/env bash
# After building a release, confirm the tree is the tag, apart from icon generation.
#
#   scripts/release-tree-check.sh vX.Y.Z
#
# `make darwin-package` always runs icon generation (build/Taskfile.yml, generate:icons):
#   - build/darwin/icons.icns is tracked and gets rewritten (deterministically, to a small
#     fallback icns; the real icon on current macOS comes from Assets.car);
#   - build/darwin/Assets.car is untracked and differs on every run.
# Committing either would not make a build reproducible, so they are allowed to differ. Anything
# else changed means the app was not built from the tag as committed.
set -euo pipefail

want="${1:-}"
[[ "$want" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "usage: $0 vX.Y.Z" >&2; exit 2; }

got="$(git describe --tags --exact-match HEAD 2>/dev/null || true)"
[[ "$got" == "$want" ]] || { echo "FAIL: HEAD is '${got:-not a tag}', expected $want" >&2; exit 1; }

allowed='^( M|\?\?) build/darwin/(icons\.icns|Assets\.car)$'
unexpected="$(git status --porcelain | grep -Ev "$allowed" || true)"
if [[ -n "$unexpected" ]]; then
  echo "FAIL: the tree differs from $want beyond icon generation:" >&2
  printf '%s\n' "$unexpected" >&2
  exit 1
fi

if [[ -z "$(git status --porcelain)" ]]; then
  echo "ok: HEAD is $want; the tree is clean"
else
  echo "ok: HEAD is $want; only icon-generation outputs differ:"
  git status --porcelain | sed 's/^/    /'
fi
