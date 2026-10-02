#!/usr/bin/env bash
# Print the release notes for a version: the "Installing" paragraph from docs/releasing.md, then
# the CHANGELOG's `## [X.Y.Z]` section without its internal `<!-- changelog-skip: ... -->` comment.
#
#   scripts/release-notes.sh X.Y.Z [CHANGELOG.md] [docs/releasing.md]
#
# The paragraph is read from docs/releasing.md ("Release notes preface") so it is defined in one
# place; X.Y.Z / vX.Y.Z in it become the version. Exits 1 when the section or the paragraph is
# missing, or the section is empty: a release with blank notes must not go out.
set -euo pipefail

ver="${1:-}"
changelog="${2:-CHANGELOG.md}"
releasing="${3:-docs/releasing.md}"
[[ "$ver" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "usage: $0 X.Y.Z [CHANGELOG.md] [docs/releasing.md]" >&2; exit 2; }

preface="$(awk '
  /^## Release notes preface/ { seen = 1; next }
  seen && !open && /^```markdown$/ { open = 1; next }
  open && /^```$/ { exit }
  open { print }
' "$releasing")"
[[ -n "$preface" ]] || { echo "FAIL: no Installing paragraph found under 'Release notes preface' in $releasing" >&2; exit 1; }

section="$(awk -v h="## [$ver]" '
  index($0, h) == 1 { on = 1; print; next }
  on && /^## \[/ { exit }
  on { print }
' "$changelog")"
[[ -n "$section" ]] || { echo "FAIL: no '## [$ver]' section in $changelog" >&2; exit 1; }

# Drop the internal changelog-skip comment (one line or several).
section="$(printf '%s\n' "$section" | perl -0pe 's/<!--\s*changelog-skip:.*?-->[ \t]*\n?//gs')"
body="$(printf '%s\n' "$section" | sed '1d' | grep -c '[^[:space:]]' || true)"
[[ "$body" -gt 0 ]] || { echo "FAIL: the '## [$ver]' section of $changelog has no entries" >&2; exit 1; }

printf '%s\n' "$preface" | sed "s/vX\.Y\.Z/v$ver/g; s/X\.Y\.Z/$ver/g"
printf '\n%s\n' "$section"
