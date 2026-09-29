#!/usr/bin/env bash
# Tests for scripts/changelog-check.sh, using a canned PR list and changelog (no network).
#   bash scripts/changelog-check.test.sh
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
check="$here/changelog-check.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cat >"$tmp/CHANGELOG.md" <<'EOF'
# Changelog

## [Unreleased]
<!-- changelog-skip: #40 -->

### Enhancements
- Covered by its own PR number (#10).
- Covered through the issue in its title (#21).

### Bug Fixes
- Covered by a closing keyword in the PR body (#31).

### Known Issues
- Mentions an open issue that a merged PR touched (#60). This must NOT count as coverage.

## [0.0.9] - 2026-01-01

### Enhancements
- An older release citing #70. Also must not count.
EOF

cat >"$tmp/prs.json" <<'EOF'
[
  {"number":10,"title":"Plain title, cited by PR number","author":{"login":"dev"},"mergedAt":"2026-09-01T00:00:00Z","body":""},
  {"number":11,"title":"Implements #21","author":{"login":"dev"},"mergedAt":"2026-09-01T00:00:00Z","body":""},
  {"number":12,"title":"Title with no ref","author":{"login":"dev"},"mergedAt":"2026-09-01T00:00:00Z","body":"Closes #31"},
  {"number":40,"title":"Deliberately skipped","author":{"login":"dev"},"mergedAt":"2026-09-01T00:00:00Z","body":""},
  {"number":41,"title":"UNCOVERED plain","author":{"login":"dev"},"mergedAt":"2026-09-01T00:00:00Z","body":""},
  {"number":42,"title":"UNCOVERED issue ref (#60)","author":{"login":"dev"},"mergedAt":"2026-09-01T00:00:00Z","body":""},
  {"number":43,"title":"Cited only in an older release (#70)","author":{"login":"dev"},"mergedAt":"2026-09-01T00:00:00Z","body":""},
  {"number":44,"title":"chore(deps): a dependency bump","author":{"login":"dev"},"mergedAt":"2026-09-01T00:00:00Z","body":""},
  {"number":45,"title":"docs(#5): a docs change","author":{"login":"dev"},"mergedAt":"2026-09-01T00:00:00Z","body":""},
  {"number":46,"title":"Bot PR","author":{"login":"app/dependabot"},"mergedAt":"2026-09-01T00:00:00Z","body":""},
  {"number":47,"title":"UNCOVERED but old","author":{"login":"dev"},"mergedAt":"2026-01-01T00:00:00Z","body":""}
]
EOF

fail=0
expect() { # description, pattern, output
  if printf '%s' "$3" | grep -q -- "$2"; then echo "ok   $1"; else echo "FAIL $1 (wanted /$2/)"; fail=1; fi
}
reject() {
  if printf '%s' "$3" | grep -q -- "$2"; then echo "FAIL $1 (did not want /$2/)"; fail=1; else echo "ok   $1"; fi
}

out="$("$check" --file "$tmp/CHANGELOG.md" --pr-json "$tmp/prs.json" --since 2026-06-01T00:00:00Z)"
reject "PR number cited"                   '#10\b'   "$out"
reject "issue in title cited"              '#11\b'   "$out"
reject "closing keyword in body"           '#12\b'   "$out"
reject "skipped via changelog-skip"        '#40\b'   "$out"
expect "uncovered plain PR is listed"      '#41\b'   "$out"
expect "Known Issues does not cover"       '#42\b'   "$out"
expect "older release does not cover"      '#43\b'   "$out"
reject "chore title ignored"               '#44\b'   "$out"
reject "docs title ignored"                '#45\b'   "$out"
reject "bot PR ignored"                    '#46\b'   "$out"
reject "PR before --since ignored"         '#47\b'   "$out"

out="$("$check" --file "$tmp/CHANGELOG.md" --pr-json "$tmp/prs.json" --since 2026-06-01T00:00:00Z --include-bots)"
expect "--include-bots lists the bot PR"   '#46\b'   "$out"

out="$("$check" --file "$tmp/CHANGELOG.md" --pr-json "$tmp/prs.json")"
expect "no --since and no tag: old PR too" '#47\b'   "$out"

if "$check" --file "$tmp/CHANGELOG.md" --pr-json "$tmp/prs.json" --since 2026-06-01T00:00:00Z --strict >/dev/null; then
  echo "FAIL --strict should exit 1 when something is uncovered"; fail=1
else
  echo "ok   --strict exits 1 when something is uncovered"
fi

printf '[]' >"$tmp/none.json"
if out="$("$check" --file "$tmp/CHANGELOG.md" --pr-json "$tmp/none.json" --strict)"; then
  expect "--strict exits 0 when all covered" 'ok: every merged PR' "$out"
else
  echo "FAIL --strict should exit 0 when nothing is uncovered"; fail=1
fi

# An empty [Unreleased] is the state right after a release: every PR merged since is uncovered,
# and the script must say so and not exit silently.
cat >"$tmp/empty.md" <<'EOF'
# Changelog

## [Unreleased]

## [0.1.0] - 2026-09-29

### Enhancements
- Something (#10).
EOF
out="$("$check" --file "$tmp/empty.md" --pr-json "$tmp/prs.json" --since 2026-06-01T00:00:00Z)" \
  || { echo "FAIL empty [Unreleased] made the script exit non-zero"; fail=1; out=""; }
expect "empty [Unreleased] lists uncovered PRs" '#41\b' "$out"
expect "empty [Unreleased]: a released entry does not cover" '#10\b' "$out"

[[ $fail -eq 0 ]] && echo "PASS" || { echo "FAILED"; exit 1; }
