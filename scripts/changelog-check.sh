#!/usr/bin/env bash
# List merged PRs that CHANGELOG.md's [Unreleased] section does not cover.
#
#   scripts/changelog-check.sh [--strict] [--include-bots] [--since-tag TAG | --since ISO8601]
#                              [--file CHANGELOG.md] [--pr-json FILE]
#
# Not a gate: it prints and exits 0 (use --strict to exit 1 when anything is uncovered). It is a
# step in docs/release-checklist.md, not a CI check.
#
# A PR is covered when its number, or an issue number in its title, or a "Closes/Fixes/Resolves
# #N" in its body, appears in the [Unreleased] section of the changelog, OUTSIDE the "Known
# Issues" subsection. Known Issues does not count: it mentions issues that are still open, so it
# says nothing about a fix having been recorded (the first dry run was fooled by exactly that).
#
# A PR that deliberately gets no entry is listed in a comment inside [Unreleased]:
#   <!-- changelog-skip: #4 #21 -->
#
# Ignored without needing a skip line: PRs by bots (dependency bumps) and titles that start with
# ci / chore / docs / test.
#
# Range: PRs merged after the last v* release tag; with no tag yet, every merged PR.
set -euo pipefail

repo="Performant-Labs/kai-private"
file="CHANGELOG.md"
since=""
strict=0
bots=false
prjson=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --strict) strict=1 ;;
    --include-bots) bots=true ;;
    --since) since="$2"; shift ;;
    --since-tag) since="$(TZ=UTC git log -1 --date=format-local:'%Y-%m-%dT%H:%M:%SZ' --format=%cd "$2")"; shift ;;
    --file) file="$2"; shift ;;
    --pr-json) prjson="$2"; shift ;;
    -h|--help) sed -n '2,24p' "$0"; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done

[[ -f "$file" ]] || { echo "no $file" >&2; exit 2; }
grep -q '^## \[Unreleased\]' "$file" || { echo "$file has no [Unreleased] section" >&2; exit 2; }

if [[ -z "$since" ]]; then
  tag="$(git describe --tags --abbrev=0 --match 'v[0-9]*' 2>/dev/null || true)"
  if [[ -n "$tag" ]]; then
    since="$(TZ=UTC git log -1 --date=format-local:'%Y-%m-%dT%H:%M:%SZ' --format=%cd "$tag")"
    echo "range: merged after $tag ($since)"
  else
    echo "range: every merged PR (no v* release tag yet)"
  fi
else
  echo "range: merged after $since"
fi

# The [Unreleased] entries, without the Known Issues subsection.
entries="$(awk '
  /^## \[Unreleased\]/ { u = 1; next }
  u && /^## \[/        { u = 0 }
  u && /^### Known Issues/ { k = 1; next }
  u && /^### /         { k = 0 }
  u && !k              { print }
' "$file")"
# `|| true`: an empty [Unreleased] (the state right after every release) has no numbers, and
# grep exits 1 on no match, which pipefail + set -e would turn into a silent exit.
cited="$(printf '%s\n' "$entries" | { grep -o '#[0-9]\+' || true; } | tr -d '#' | sort -un | jq -Rs 'split("\n") | map(select(length>0) | tonumber)')"

if [[ -n "$prjson" ]]; then
  prs="$(cat "$prjson")"
else
  prs="$(gh pr list --repo "$repo" --state merged --limit 500 --json number,title,author,mergedAt,body)"
fi

uncovered="$(printf '%s' "$prs" | jq -r --argjson cited "$cited" --arg since "$since" --argjson bots "$bots" '
  .[]
  | select($since == "" or .mergedAt > $since)
  | select($bots or (.author.login | test("dependabot|\\[bot\\]|^app/") | not))
  | select(.title | test("^(ci|chore|docs|test)(\\(|:)"; "i") | not)
  | . as $p
  | ([$p.number]
     + [$p.title | scan("#([0-9]+)") | .[0] | tonumber]
     + [($p.body // "") | scan("(?i)(?:closes|fixes|resolves) #([0-9]+)") | .[0] | tonumber]) as $refs
  | select((($refs - $cited) | length) == ($refs | length))
  | "  #\(.number)\t\(.mergedAt[:10])\t\(.title)"
' | sort -t'#' -k2 -n)"

if [[ -z "$uncovered" ]]; then
  echo "ok: every merged PR in range is covered by [Unreleased] (or skipped)"
  exit 0
fi

echo "NOT covered by [Unreleased] ($(printf '%s\n' "$uncovered" | wc -l | tr -d ' ')):"
printf '%s\n' "$uncovered"
echo
echo "For each: add an entry citing its number or issue, or add it to a <!-- changelog-skip: #N --> comment."
echo "For 'Implements #N' PRs, read the issue and the diff; the title does not say what changed."
[[ $strict -eq 1 ]] && exit 1
exit 0
