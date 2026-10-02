#!/usr/bin/env bash
# Tests for scripts/release-notes.sh and scripts/release-check-version.sh, on small fixtures.
#   bash scripts/release-notes.test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
notes="$here/release-notes.sh"
check="$here/release-check-version.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/build/darwin"
cat >"$tmp/CHANGELOG.md" <<'CL'
# Changelog

## [Unreleased]
<!-- changelog-skip: #4
 #21 -->

## [1.2.3] - 2026-01-02
<!-- changelog-skip: #9 -->

### Bug Fixes
- Fixed the thing (#5).

## [1.2.2] - 2026-01-01

### Bug Fixes
- Older fix (#1).

## [1.0.0] - 2025-12-01
CL
cat >"$tmp/releasing.md" <<'RL'
## Something

```markdown
not this one vX.Y.Z
```

## Release notes preface

Intro text.

```markdown
**Installing.** Get vX.Y.Z via `Kai-X.Y.Z.zip`.
```

## After
RL
cat >"$tmp/build/config.yml" <<'Y'
info:
  version: "1.2.3"
other:
  version: "1.2.3"
Y
cat >"$tmp/build/darwin/Info.plist" <<'P'
<plist><dict>
<key>CFBundleShortVersionString</key>
<string>1.2.3</string>
<key>CFBundleVersion</key>
<string>1.2.3</string>
</dict></plist>
P

fail=0; out=""; code=0
has()   { if printf '%s' "$out" | grep -q -- "$2"; then echo "ok   $1"; else echo "FAIL $1 (wanted /$2/)"; fail=1; fi; }
hasnt() { if printf '%s' "$out" | grep -q -- "$2"; then echo "FAIL $1 (did not want /$2/)"; fail=1; else echo "ok   $1"; fi; }
exits() { if [[ "$code" -eq "$2" ]]; then echo "ok   $1"; else echo "FAIL $1 (exit $code, wanted $2)"; fail=1; fi; }
runn()  { out="$("$notes" "$@" 2>&1)"; code=$?; }
runc()  { out="$("$check" "$@" 2>&1)"; code=$?; }

runn 1.2.3 "$tmp/CHANGELOG.md" "$tmp/releasing.md"
exits "notes for a version that exists succeed" 0
has   "the Installing paragraph comes first, with the version filled in" '^\*\*Installing\.\*\* Get v1.2.3 via `Kai-1.2.3.zip`'
hasnt "  ... and it is the preface block, not the earlier markdown block" 'not this one'
has   "the section heading is kept" '^## \[1.2.3\] - 2026-01-02'
has   "the section's entries are kept" 'Fixed the thing'
hasnt "the changelog-skip comment is dropped" 'changelog-skip'
hasnt "the next section is not included" 'Older fix'
hasnt "the Unreleased section is not included" '#21'

runn 9.9.9 "$tmp/CHANGELOG.md" "$tmp/releasing.md"
exits "a version with no section fails" 1
has   "  ... and says so" "no '## \[9.9.9\]' section"

runn 1.0.0 "$tmp/CHANGELOG.md" "$tmp/releasing.md"
exits "an empty section fails (no blank release notes)" 1
has   "  ... and says so" 'has no entries'

runn 1.2.3 "$tmp/CHANGELOG.md" /dev/null
exits "a missing Installing paragraph fails" 1

runn not-a-version
exits "a bad version is a usage error" 2

runc 1.2.3 "$tmp"
exits "files that agree pass" 0
has   "  ... and say ok" 'all say 1.2.3'

runc 1.2.4 "$tmp"
exits "files that disagree with the version fail" 1
has   "  ... naming config.yml" 'build/config.yml has version "1.2.3"'
has   "  ... naming Info.plist" 'Info.plist CFBundleVersion'
has   "  ... naming the changelog" "no '## \[1.2.4\]' section"

[[ $fail -eq 0 ]] && echo "ALL PASS" || { echo "SOME FAILED"; exit 1; }
