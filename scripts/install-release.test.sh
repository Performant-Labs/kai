#!/usr/bin/env bash
# Tests for scripts/install-release.sh, using small signed fake apps, zips and disk images,
# files tagged with the quarantine flag the way a browser does, and a fake `gh` (no network).
# It installs into throwaway directories and uses a test bundle id, so it never touches
# /Applications or your real Accessibility grant. Meant for a Mac (needs hdiutil, ditto, codesign).
#   bash scripts/install-release.test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
inst="$here/install-release.sh"
tmp="$(mktemp -d)"
fake_pid=""
cleanup() { [[ -n "$fake_pid" ]] && kill "$fake_pid" 2>/dev/null; rm -rf "$tmp"; }
trap cleanup EXIT

export KAI_INSTALL_BUNDLE_ID="test.kai.installer"
fail=0; out=""; code=0

has()   { if printf '%s' "$out" | grep -q -- "$2"; then echo "ok   $1"; else echo "FAIL $1 (wanted /$2/)"; fail=1; fi; }
hasnt() { if printf '%s' "$out" | grep -q -- "$2"; then echo "FAIL $1 (did not want /$2/)"; fail=1; else echo "ok   $1"; fi; }
exit_is() { if [[ "$code" == "$2" ]]; then echo "ok   $1"; else echo "FAIL $1 (exit $code, wanted $2)"; fail=1; fi; }
truth() { if eval "$2"; then echo "ok   $1"; else echo "FAIL $1"; fail=1; fi; }
run() { out="$("$inst" "$@" 2>&1)"; code=$?; }

# A tiny signed app. Different $2 gives a different binary, like a different build.
mkapp() { # dir version [bundle-id]
  local d="$1" v="$2" id="${3:-test.kai.installer}"
  mkdir -p "$d/Kai.app/Contents/MacOS"
  printf 'static const char v[] = "%s"; int main(void){ return v[0] == 0; }\n' "$v" | cc -x c - -o "$d/Kai.app/Contents/MacOS/Kai" 2>/dev/null
  cat >"$d/Kai.app/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>$id</string>
<key>CFBundleExecutable</key><string>Kai</string>
<key>CFBundleShortVersionString</key><string>$v</string>
<key>CFBundleVersion</key><string>$v</string>
</dict></plist>
EOF
  codesign --force --deep --sign - "$d/Kai.app" >/dev/null 2>&1
}
quarantine() { xattr -w com.apple.quarantine "0083;5f000000;Safari;" "$1"; }
has_q() { xattr -lr "$1" 2>/dev/null | grep -q com.apple.quarantine; }

images_before="$(hdiutil info | grep -c image-path)"
mkdir "$tmp/v1" "$tmp/v2" "$tmp/apps"
mkapp "$tmp/v1" 1.0.0
mkapp "$tmp/v2" 2.0.0
ditto -c -k --keepParent "$tmp/v1/Kai.app" "$tmp/Kai-1.0.0-darwin-arm64.zip"
ditto -c -k --keepParent "$tmp/v2/Kai.app" "$tmp/Kai-2.0.0-darwin-arm64.zip"
mkdir "$tmp/dmgsrc" && cp -R "$tmp/v1/Kai.app" "$tmp/dmgsrc/" \
  && hdiutil create -volname Kai -srcfolder "$tmp/dmgsrc" -ov -format UDZO "$tmp/Kai-1.0.0-darwin-arm64.dmg" >/dev/null 2>&1
[[ -f "$tmp/Kai-1.0.0-darwin-arm64.dmg" ]] || { echo "could not build the test disk image"; exit 1; }

# A browser download: zip carrying the quarantine flag, installed with --from.
quarantine "$tmp/Kai-1.0.0-darwin-arm64.zip"
truth "the test zip really is quarantined"       'has_q "$tmp/Kai-1.0.0-darwin-arm64.zip"'
run --from "$tmp/Kai-1.0.0-darwin-arm64.zip" --dest "$tmp/apps"
has  "--from zip installs"                        'installed Kai 1.0.0'
exit_is "  ... and exits 0"                       0
truth "  ... the installed app is not quarantined" '! has_q "$tmp/apps/Kai.app"'
truth "  ... the source file's flag was cleared"   '! has_q "$tmp/Kai-1.0.0-darwin-arm64.zip"'
has  "  ... with no checksum file, it warns"      'WARN: no SHA256SUMS'
has  "  ... a first install clears the old grant" 'old Accessibility grant no longer applies'

# The second defence on its own: when the flag cannot be cleared on the source file (read-only),
# extraction copies it onto the app, and only clearing the installed copy keeps it off.
cp "$tmp/Kai-1.0.0-darwin-arm64.zip" "$tmp/ro.zip"
quarantine "$tmp/ro.zip"; chmod 444 "$tmp/ro.zip"
mkdir -p "$tmp/apps-ro"
run --from "$tmp/ro.zip" --dest "$tmp/apps-ro"
has  "a read-only quarantined zip still installs"                  'installed Kai 1.0.0'
truth "  ... its flag really could not be cleared at the source"    'has_q "$tmp/ro.zip"'
truth "  ... and the installed app is still not quarantined"        '! has_q "$tmp/apps-ro/Kai.app"'
chmod 644 "$tmp/ro.zip"

# The same build again: nothing about permissions should be touched.
run --from "$tmp/Kai-1.0.0-darwin-arm64.zip" --dest "$tmp/apps"
has  "the same build again keeps the grant"       'same build as the one that was installed'
hasnt "  ... and does not clear it"               'was cleared'

# A different build.
run --from "$tmp/Kai-2.0.0-darwin-arm64.zip" --dest "$tmp/apps"
has  "a new build replaces the old one"           'installed Kai 2.0.0'
has  "  ... and clears the stale grant"           'was cleared'
run --from "$tmp/Kai-1.0.0-darwin-arm64.zip" --dest "$tmp/apps" --keep-permissions
has  "--keep-permissions leaves the grant alone"  'left the Accessibility grant alone'

# A disk image.
quarantine "$tmp/Kai-1.0.0-darwin-arm64.dmg"
run --from "$tmp/Kai-1.0.0-darwin-arm64.dmg" --dest "$tmp/apps2" 2>/dev/null || true
mkdir -p "$tmp/apps2"
run --from "$tmp/Kai-1.0.0-darwin-arm64.dmg" --dest "$tmp/apps2"
has  "--from dmg installs"                        'installed Kai 1.0.0'
truth "  ... the installed app is not quarantined" '! has_q "$tmp/apps2/Kai.app"'
truth "  ... and the image is unmounted again"     '[[ "$(hdiutil info | grep -c image-path)" == "$images_before" ]]'

# Checksums beside the file.
mkdir "$tmp/ck"; cp "$tmp/Kai-1.0.0-darwin-arm64.zip" "$tmp/ck/"
( cd "$tmp/ck" && shasum -a 256 Kai-1.0.0-darwin-arm64.zip >SHA256SUMS )
mkdir -p "$tmp/apps3"
run --from "$tmp/ck/Kai-1.0.0-darwin-arm64.zip" --dest "$tmp/apps3"
has  "a matching SHA256SUMS is accepted"          'checksum ok'
echo "0000000000000000000000000000000000000000000000000000000000000000  Kai-1.0.0-darwin-arm64.zip" >"$tmp/ck/SHA256SUMS"
mkdir -p "$tmp/apps4"
run --from "$tmp/ck/Kai-1.0.0-darwin-arm64.zip" --dest "$tmp/apps4"
has  "a wrong checksum is refused"                'checksum mismatch'
exit_is "  ... with exit 1"                       1
truth "  ... and nothing was installed"            '[[ ! -e "$tmp/apps4/Kai.app" ]]'

# Refusing bad apps.
mkapp "$tmp/other" 1.0.0 "com.someone.else"
ditto -c -k --keepParent "$tmp/other/Kai.app" "$tmp/other.zip"
mkdir -p "$tmp/apps5"
run --from "$tmp/other.zip" --dest "$tmp/apps5"
has  "the wrong bundle id is refused"             'not a Kai build'
mkapp "$tmp/tamper" 1.0.0
printf 'extra' >>"$tmp/tamper/Kai.app/Contents/MacOS/Kai"
ditto -c -k --keepParent "$tmp/tamper/Kai.app" "$tmp/tamper.zip"
run --from "$tmp/tamper.zip" --dest "$tmp/apps5"
has  "a tampered binary is refused"               'signature is not valid'
truth "  ... and nothing was installed"            '[[ ! -e "$tmp/apps5/Kai.app" ]]'
echo hi >"$tmp/notes.txt"
run --from "$tmp/notes.txt" --dest "$tmp/apps5"
has  "a file that is not a zip or dmg is refused" 'expected a .zip or .dmg'

# Never replace a running Kai: a copy of sleep named Kai, re-signed so macOS lets it run.
mkdir -p "$tmp/apps6/Kai.app/Contents/MacOS"
cp /bin/sleep "$tmp/apps6/Kai.app/Contents/MacOS/Kai" && codesign --force --sign - "$tmp/apps6/Kai.app/Contents/MacOS/Kai" 2>/dev/null
"$tmp/apps6/Kai.app/Contents/MacOS/Kai" 120 & fake_pid=$!
sleep 1
run --from "$tmp/Kai-1.0.0-darwin-arm64.zip" --dest "$tmp/apps6"
has  "a running Kai is not replaced"              'Kai is running from'
exit_is "  ... with exit 1"                       1
kill "$fake_pid" 2>/dev/null; wait "$fake_pid" 2>/dev/null; fake_pid=""
run --from "$tmp/Kai-1.0.0-darwin-arm64.zip" --dest "$tmp/apps7" 2>/dev/null; mkdir -p "$tmp/apps7"
run --from "$tmp/Kai-1.0.0-darwin-arm64.zip" --dest "$tmp/apps7"
has  "a Kai running elsewhere does not block"     'installed Kai 1.0.0'

# The download path, with a fake gh.
mkdir "$tmp/fakebin"
cat >"$tmp/fakebin/gh" <<'EOF'
#!/usr/bin/env bash
case "$1 $2" in
  "release view") echo "${FAKE_TAG:-v1.0.0}" ;;
  "release download")
    shift 2; tag="$1"; shift; dir=""
    while [ $# -gt 0 ]; do case "$1" in --dir) dir="$2"; shift ;; esac; shift; done
    [ -n "${FAKE_NOASSET:-}" ] && { mkdir -p "$dir"; exit 0; }
    mkdir -p "$dir"; cp "$FAKE_ZIP" "$dir/"
    if [ -n "${FAKE_BAD:-}" ]; then echo "0000000000000000000000000000000000000000000000000000000000000000  $(basename "$FAKE_ZIP")" >"$dir/SHA256SUMS"
    else (cd "$dir" && shasum -a 256 "$(basename "$FAKE_ZIP")" >SHA256SUMS); fi ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$tmp/fakebin/gh"
export FAKE_ZIP="$tmp/Kai-1.0.0-darwin-arm64.zip"
gh_run() { out="$(PATH="$tmp/fakebin:$PATH" "$inst" "$@" 2>&1)"; code=$?; }
mkdir -p "$tmp/g1" "$tmp/g2" "$tmp/g3" "$tmp/g4"
gh_run --dest "$tmp/g1"
has  "no argument installs the latest release"    'downloading v1.0.0'
has  "  ... verifies the checksum"                'checksum ok'
has  "  ... and installs it"                      'installed Kai 1.0.0'
gh_run v1.0.0 --dest "$tmp/g2"
has  "a named version is downloaded"              'downloading v1.0.0'
out="$(FAKE_BAD=1 PATH="$tmp/fakebin:$PATH" "$inst" --dest "$tmp/g3" 2>&1)"; code=$?
has  "a corrupted download is refused"            'checksum mismatch'
truth "  ... and nothing was installed"            '[[ ! -e "$tmp/g3/Kai.app" ]]'
out="$(FAKE_NOASSET=1 PATH="$tmp/fakebin:$PATH" "$inst" --dest "$tmp/g4" 2>&1)"; code=$?
has  "a release with no zip is refused"           'has no zip asset'

# Arguments.
run v1.0.0 --from "$tmp/Kai-1.0.0-darwin-arm64.zip"
exit_is "a version and --from together is rejected" 2
run --nonsense
has  "an unknown argument is rejected"            'unknown argument'
run --from "$tmp/Kai-1.0.0-darwin-arm64.zip" --dest "$tmp/does-not-exist"
has  "a missing destination is refused"           'not a directory'

[[ $fail -eq 0 ]] && echo "PASS" || { echo "FAILED"; exit 1; }
