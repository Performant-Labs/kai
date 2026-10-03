#!/usr/bin/env bash
# Say what kind of signature a Kai.app has and whether macOS privacy grants (Accessibility, Screen
# Recording, Input Monitoring) will survive a rebuild.
#
#   scripts/sign-check.sh /path/to/Kai.app [--require-stable]
#
# Kinds, from the designated requirement (`codesign -dr -`):
#   ad-hoc                               requirement is the binary's cdhash: grants are lost on EVERY rebuild
#   Developer ID                         requirement anchors to Apple and a Developer ID certificate
#   stable local/development certificate requirement names a certificate (`certificate leaf = H"..."`,
#                                        or `certificate root = H"..."` for a self-signed one): grants survive rebuilds
# Exit 0 always (information), except: a missing/unreadable app exits 1, bad usage exits 2, and
# --require-stable exits 1 when the requirement depends on the binary hash.
set -uo pipefail

app=""; require=0
for a in "$@"; do
  case "$a" in
    --require-stable) require=1 ;;
    -*) echo "usage: $0 /path/to/Kai.app [--require-stable]" >&2; exit 2 ;;
    *) app="$a" ;;
  esac
done
[[ -n "$app" ]] || { echo "usage: $0 /path/to/Kai.app [--require-stable]" >&2; exit 2; }
[[ -d "$app" ]] || { echo "error: no such app: $app" >&2; exit 1; }

# `codesign -dr -` prints "Executable=..." then "designated => <requirement>" on stdout/stderr.
raw="$(codesign -dr - "$app" 2>&1)" || { echo "error: cannot read the signature of $app (unsigned?)" >&2; echo "$raw" >&2; exit 1; }
req="$(printf '%s\n' "$raw" | sed -n 's/^#* *designated => //p' | head -1)"
[[ -n "$req" ]] || { echo "error: no designated requirement found for $app" >&2; exit 1; }

if [[ "$req" == *cdhash* ]] && [[ "$req" != *"certificate leaf"* ]] && [[ "$req" != *"anchor apple generic"* ]]; then
  kind="ad-hoc"; stable=0
  verdict="grants are LOST on every rebuild (the requirement is the binary's hash)"
elif [[ "$req" == *"certificate 1[field.1.2.840.113635.100.6.2.6]"* || "$req" == *"1.2.840.113635.100.6.2.6"* ]]; then
  kind="Developer ID"; stable=1
  verdict="grants survive rebuilds"
elif [[ "$req" == *"certificate leaf"* || "$req" == *"certificate root"* || "$req" == *"anchor apple generic"* || "$req" == *"anchor "* ]]; then
  kind="stable local/development certificate"; stable=1
  verdict="grants survive rebuilds signed with the same certificate"
else
  kind="ad-hoc"; stable=0
  verdict="grants are LOST on every rebuild (the requirement does not name a certificate)"
fi

echo "signature kind: $kind"
echo "requirement:    $req"
echo "rebuilds:       $verdict"
if [[ $require -eq 1 && $stable -eq 0 ]]; then
  echo "error: --require-stable: this signature depends on the binary hash (see docs/dev-signing.md)" >&2
  exit 1
fi
exit 0
