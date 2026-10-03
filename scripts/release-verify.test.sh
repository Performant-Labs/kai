#!/usr/bin/env bash
# Tests for scripts/release-verify.sh steps 1-2 (version and signature), using a fake app whose
# binary exits at once, so step 4 (launch) fails AFTER the signature step has printed. Uses a
# fake `codesign` on PATH so each kind of signature can be shown.
#   bash scripts/release-verify.test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
rv="$here/release-verify.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/bin" "$tmp/Kai.app/Contents/MacOS"
printf '#!/bin/sh\nexit 0\n' >"$tmp/Kai.app/Contents/MacOS/Kai"; chmod +x "$tmp/Kai.app/Contents/MacOS/Kai"
cat >"$tmp/Kai.app/Contents/Info.plist" <<'PL'
<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>CFBundleShortVersionString</key><string>1.2.3</string></dict></plist>
PL
cat >"$tmp/bin/codesign" <<'FAKE'
#!/usr/bin/env bash
case "$1" in
  --verify) [[ "${FAKE_KIND:-}" == invalid ]] && { echo "invalid signature" >&2; exit 1; }; exit 0 ;;
  -dv) printf 'Identifier=com.performantlabs.kai\nSignature=adhoc\n' >&2 ;;
  -dr)
    case "${FAKE_KIND:-}" in
      adhoc) printf 'Executable=/x\n# designated => cdhash H"baa23ec59eab0b294f93e36b9f7668c3a89f01e4"\n' >&2 ;;
      cert)  printf 'Executable=/x\ndesignated => identifier "com.performantlabs.kai" and certificate leaf = H"dc5b8b8daa588f1b265ff845c19c5384d6c969b2"\n' >&2 ;;
    esac ;;
esac
FAKE
chmod +x "$tmp/bin/codesign"

fail=0; out=""; code=0
has()   { if printf '%s' "$out" | grep -q -- "$2"; then echo "ok   $1"; else echo "FAIL $1 (wanted /$2/)"; fail=1; fi; }
hasnt() { if printf '%s' "$out" | grep -q -- "$2"; then echo "FAIL $1 (did not want /$2/)"; fail=1; else echo "ok   $1"; fi; }
run() { local kind="$1"; shift; out="$(PATH="$tmp/bin:$PATH" FAKE_KIND="$kind" "$rv" "$@" 2>&1)"; code=$?; }

run adhoc "$tmp/Kai.app" 1.2.3 1
has   "an ad-hoc build reports its kind"                    'signature kind: ad-hoc'
has   "  ... and that grants are lost on rebuild"            'LOST on every rebuild'
hasnt "  ... and does NOT fail the release at the signature" 'FAIL: signature'
has   "  ... it goes on to step 3"                           '3/4 embedded personal paths'

run cert "$tmp/Kai.app" 1.2.3 1
has   "a certificate build reports a stable certificate"    'signature kind: stable local/development certificate'
has   "  ... and goes on to step 3"                          '3/4 embedded personal paths'

run invalid "$tmp/Kai.app" 1.2.3 1
has   "an invalid signature still fails the release"        'FAIL: signature invalid'
hasnt "  ... before any later step"                          '3/4'

run adhoc "$tmp/Kai.app" 9.9.9 1
has   "a version mismatch still fails at step 1"             'FAIL: Info.plist says 1.2.3'

[[ $fail -eq 0 ]] && echo "ALL PASS" || { echo "SOME FAILED"; exit 1; }
