#!/usr/bin/env bash
# Tests for scripts/release-package.sh, using a small signed fake app. Needs a Mac (hdiutil,
# ditto, codesign, cc).
#   bash scripts/release-package.test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
pkg="$here/release-package.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

fail=0; out=""; code=0
has()   { if printf '%s' "$out" | grep -q -- "$2"; then echo "ok   $1"; else echo "FAIL $1 (wanted /$2/)"; fail=1; fi; }
truth() { if eval "$2"; then echo "ok   $1"; else echo "FAIL $1"; fail=1; fi; }
exit_is() { if [[ "$code" == "$2" ]]; then echo "ok   $1"; else echo "FAIL $1 (exit $code, wanted $2)"; fail=1; fi; }
run() { out="$("$pkg" "$@" 2>&1)"; code=$?; }

mkapp() { # dir version
  local d="$1" v="$2"
  mkdir -p "$d/Kai.app/Contents/MacOS"
  printf 'static const char v[] = "%s"; int main(void){ return v[0] == 0; }\n' "$v" | cc -x c - -o "$d/Kai.app/Contents/MacOS/Kai" 2>/dev/null
  cat >"$d/Kai.app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>test.kai.packager</string>
<key>CFBundleExecutable</key><string>Kai</string>
<key>CFBundleShortVersionString</key><string>$v</string>
<key>CFBundleVersion</key><string>$v</string>
</dict></plist>
EOF
  codesign --force --deep --sign - "$d/Kai.app" >/dev/null 2>&1
}

images_before="$(hdiutil info | grep -c image-path)"
mkdir "$tmp/in" "$tmp/out"
mkapp "$tmp/in" 1.2.3

run "$tmp/in/Kai.app" 1.2.3 "$tmp/out"
has  "packaging succeeds and says so"               'dmg: the app inside is identical'
exit_is "  ... exit 0"                              0
truth "the zip exists, named for the version"        '[[ -s "$tmp/out/Kai-1.2.3-darwin-arm64.zip" ]]'
truth "the disk image exists, named for the version" '[[ -s "$tmp/out/Kai-1.2.3-darwin-arm64.dmg" ]]'
truth "SHA256SUMS lists both files"                  '[[ "$(grep -c "Kai-1.2.3-darwin-arm64" "$tmp/out/SHA256SUMS")" == 2 ]]'
truth "  ... and the checksums verify"               '(cd "$tmp/out" && shasum -a 256 -c SHA256SUMS >/dev/null 2>&1)'
truth "the disk image is unmounted afterwards"       '[[ "$(hdiutil info | grep -c image-path)" == "$images_before" ]]'

# The zip really unpacks to the same app, and the disk image really shows the app plus a shortcut.
mkdir "$tmp/chk"; ditto -x -k "$tmp/out/Kai-1.2.3-darwin-arm64.zip" "$tmp/chk"
truth "the zip unpacks to an identical app"          'diff -r "$tmp/chk/Kai.app" "$tmp/in/Kai.app" >/dev/null'
mkdir "$tmp/mnt"; hdiutil attach -readonly -nobrowse -noverify -mountpoint "$tmp/mnt" "$tmp/out/Kai-1.2.3-darwin-arm64.dmg" >/dev/null 2>&1
truth "the disk image has Kai.app and an Applications shortcut" '[[ -d "$tmp/mnt/Kai.app" && -L "$tmp/mnt/Applications" ]]'
hdiutil detach -quiet "$tmp/mnt" >/dev/null 2>&1

# Running it again replaces the files.
run "$tmp/in/Kai.app" 1.2.3 "$tmp/out"
exit_is "running it again works"                    0

# Refusals.
run "$tmp/in/Kai.app" 9.9.9 "$tmp/out2"
has  "a version that differs from the app is refused" 'reports version'
truth "  ... and nothing was written"                  '[[ ! -e "$tmp/out2/SHA256SUMS" ]]'
mkapp "$tmp/bad" 1.2.3
printf 'x' >>"$tmp/bad/Kai.app/Contents/MacOS/Kai"
run "$tmp/bad/Kai.app" 1.2.3 "$tmp/out3"
has  "an app with a broken signature is refused"      'signature is not valid'
run "$tmp/nothing.app" 1.2.3 "$tmp/out4"
has  "something that is not an app bundle is refused" 'not an app bundle'
run "$tmp/in/Kai.app" v1.2.3 "$tmp/out5"
has  "a version with a leading v is rejected"         'plain X.Y.Z'
exit_is "  ... with exit 2"                           2
run
has  "no arguments prints the usage"                   'usage:'
exit_is "  ... with exit 2"                            2

[[ $fail -eq 0 ]] && echo "PASS" || { echo "FAILED"; exit 1; }
