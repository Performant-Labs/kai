#!/usr/bin/env bash
# Verify a built Kai.app before it is attached to a release, or after it is downloaded from one.
#
#   scripts/release-verify.sh /path/to/Kai.app X.Y.Z [seconds]
#
# Checks, in order (stops at the first failure):
#   1. Info.plist reports X.Y.Z.
#   2. The code signature is valid; prints its kind (ad-hoc, stable certificate or Developer ID)
#      as an informational line via scripts/sign-check.sh. An ad-hoc signature does NOT fail this.
#   3. No personal home-directory path is embedded in the binary.
#   4. The app LAUNCHES and STAYS RUNNING for [seconds] (default 20) and its own startup log
#      reports X.Y.Z. This is the check that matters most: issue #167 shipped a build that
#      passed every test and crashed about one second after every real launch.
#
# The launch runs against a throwaway HOME, so it never touches your real ~/.kai. It refuses to
# run while Kai is already running, because Kai allows one instance and a second launch would
# just hand off to the first and exit, proving nothing.
set -euo pipefail

app="${1:-}"
want="${2:-}"
secs="${3:-20}"
if [[ -z "$app" || -z "$want" ]]; then
  echo "usage: $0 /path/to/Kai.app X.Y.Z [seconds]" >&2
  exit 2
fi

bin="$app/Contents/MacOS/Kai"
plist="$app/Contents/Info.plist"
[[ -x "$bin" ]] || { echo "FAIL: no executable at $bin" >&2; exit 1; }

echo "1/4 Info.plist version"
got="$(/usr/libexec/PlistBuddy -c 'Print :CFBundleShortVersionString' "$plist")"
[[ "$got" == "$want" ]] || { echo "FAIL: Info.plist says $got, expected $want" >&2; exit 1; }
echo "    ok: $got"

echo "2/4 code signature"
codesign --verify --deep --strict "$app" 2>&1 || { echo "FAIL: signature invalid" >&2; exit 1; }
codesign -dv "$app" 2>&1 | grep -E "^(Identifier|Signature|Authority)" | sed 's/^/    /' || true
# Informational only: never fails a release (v0.1.0 is ad-hoc on purpose).
"$(dirname "$0")/sign-check.sh" "$app" 2>&1 | sed -n '1p;3p' | sed 's/^/    /' || true

echo "3/4 embedded personal paths"
if strings -a "$bin" | grep -E "/Users/[A-Za-z0-9._-]+/" | head -5 | grep -q .; then
  echo "FAIL: the binary embeds a personal home-directory path:" >&2
  strings -a "$bin" | grep -E "/Users/[A-Za-z0-9._-]+/" | head -5 | sed 's/^/    /' >&2
  exit 1
fi
echo "    ok: none"

echo "4/4 launch and stay running for ${secs}s"
if pgrep -x Kai >/dev/null; then
  echo "FAIL: Kai is already running. Quit it first (a second launch would hand off and exit)." >&2
  exit 1
fi
home="$(mktemp -d)"
log="$home/.kai/logs/kai.log"
HOME="$home" "$bin" >"$home/stdout.txt" 2>&1 &
pid=$!
cleanup() { kill "$pid" 2>/dev/null || true; wait "$pid" 2>/dev/null || true; rm -rf "$home"; }
trap cleanup EXIT

for ((i = 1; i <= secs; i++)); do
  sleep 1
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "FAIL: Kai exited after ${i}s. Its output:" >&2
    tail -30 "$home/stdout.txt" >&2
    exit 1
  fi
done
echo "    ok: still running after ${secs}s"

if ! grep -q "Kai starting... version=$want" "$log" 2>/dev/null; then
  echo "FAIL: startup log does not report version $want:" >&2
  { grep "Kai starting" "$log" || echo "    (no startup line; log missing)"; } >&2 2>/dev/null
  exit 1
fi
echo "    ok: startup log reports $want"
echo "PASS"
