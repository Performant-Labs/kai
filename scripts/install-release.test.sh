#!/usr/bin/env bash
# Tests for scripts/install-release.sh, using small signed fake apps, zips and disk images,
# files tagged with the quarantine flag the way a browser does, a fake `gh` (no network), and
# throwaway self-signed certificates in a throwaway keychain (deleted again on cleanup) for the
# signing-identity cases. It installs into throwaway directories and uses a test bundle id, so it
# never touches /Applications, your keychain, or your real Accessibility grant. Meant for a Mac
# (needs hdiutil, ditto, codesign, security, openssl).
#   bash scripts/install-release.test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
inst="$here/install-release.sh"
tmp="$(mktemp -d)"
fake_pid=""
cleanup() {
  [[ -n "$fake_pid" ]] && kill "$fake_pid" 2>/dev/null
  [[ -n "${kc:-}" ]] && security delete-keychain "$kc" >/dev/null 2>&1 || true
  rm -rf "$tmp"
}
trap cleanup EXIT

export KAI_INSTALL_BUNDLE_ID="test.kai.installer"
fail=0; out=""; code=0

has()   { if printf '%s' "$out" | grep -q -- "$2"; then echo "ok   $1"; else echo "FAIL $1 (wanted /$2/)"; fail=1; fi; }
hasnt() { if printf '%s' "$out" | grep -q -- "$2"; then echo "FAIL $1 (did not want /$2/)"; fail=1; else echo "ok   $1"; fi; }
exit_is() { if [[ "$code" == "$2" ]]; then echo "ok   $1"; else echo "FAIL $1 (exit $code, wanted $2)"; fail=1; fi; }
truth() { if eval "$2"; then echo "ok   $1"; else echo "FAIL $1"; fail=1; fi; }
run() { out="$("$inst" "$@" 2>&1)"; code=$?; }

# A tiny signed app. Different $2 gives a different binary, like a different build. With $4 it is
# signed with that throwaway certificate (made below) instead of ad-hoc, like a release build.
mkapp() { # dir version [bundle-id] [cert-name]
  local d="$1" v="$2" id="${3:-test.kai.installer}" cert="${4:-}"
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
  if [[ -n "$cert" ]]; then
    codesign --force --keychain "$kc" --sign "$cert" "$d/Kai.app" >/dev/null 2>&1
  else
    codesign --force --deep --sign - "$d/Kai.app" >/dev/null 2>&1
  fi
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

# Two throwaway self-signed code-signing certificates ("Kai Test A" and "Kai Test B") in a
# throwaway keychain: a miniature scripts/release-cert.sh. With these, two different builds can
# share a signing identity (same designated requirement, different binary hash) — the situation
# where the Accessibility grant must survive an update. The keychain is deleted on cleanup;
# nothing touches the login keychain. An untrusted fresh certificate is listed as
# CSSMERR_TP_NOT_TRUSTED and codesign signs with it fine (docs/dev-signing.md).
kc="$tmp/kai-tests.keychain-db"; kcpass="kai-tests"
mkcert() { # name
  cat >"$tmp/cert.cnf" <<CNF
[req]
distinguished_name = dn
x509_extensions = ext
prompt = no
[dn]
CN = $1
O = Kai Tests
[ext]
basicConstraints = critical,CA:false
keyUsage = critical,digitalSignature
extendedKeyUsage = critical,codeSigning
subjectKeyIdentifier = hash
CNF
  /usr/bin/openssl req -x509 -newkey rsa:2048 -nodes -days 1 -config "$tmp/cert.cnf" \
    -keyout "$tmp/cert.key" -out "$tmp/cert.crt" 2>/dev/null
  /usr/bin/openssl pkcs12 -export -inkey "$tmp/cert.key" -in "$tmp/cert.crt" -name "$1" \
    -out "$tmp/cert.p12" -passout "pass:$kcpass" 2>/dev/null
  security import "$tmp/cert.p12" -k "$kc" -P "$kcpass" -T /usr/bin/codesign >/dev/null 2>&1
}
security create-keychain -p "$kcpass" "$kc"
security set-keychain-settings -t 3600 "$kc"     # it must not lock while the tests run
security unlock-keychain -p "$kcpass" "$kc"
mkcert "Kai Test A"; mkcert "Kai Test B"
security set-key-partition-list -S apple-tool:,apple: -k "$kcpass" "$kc" >/dev/null 2>&1
ids="$(security find-identity -p codesigning "$kc" 2>/dev/null)"
printf '%s' "$ids" | grep -q '"Kai Test A"' && printf '%s' "$ids" | grep -q '"Kai Test B"' \
  || { echo "could not make the throwaway code-signing certificates"; exit 1; }

# Builds signed with those certificates: 1.0.0 and 2.0.0 by certificate A (same identity, code
# changed), 2.0.0 by certificate B (a different identity).
mkdir "$tmp/a1" "$tmp/a2" "$tmp/b2"
mkapp "$tmp/a1" 1.0.0 test.kai.installer "Kai Test A"
mkapp "$tmp/a2" 2.0.0 test.kai.installer "Kai Test A"
mkapp "$tmp/b2" 2.0.0 test.kai.installer "Kai Test B"
ditto -c -k --keepParent "$tmp/a1/Kai.app" "$tmp/Kai-certA-1.0.0.zip"
ditto -c -k --keepParent "$tmp/a2/Kai.app" "$tmp/Kai-certA-2.0.0.zip"
ditto -c -k --keepParent "$tmp/b2/Kai.app" "$tmp/Kai-certB-2.0.0.zip"

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
has  "the same build again keeps the grant"       'same signing identity: the Accessibility grant is still valid'
hasnt "  ... and does not clear it"               'was cleared'

# A different build.
run --from "$tmp/Kai-2.0.0-darwin-arm64.zip" --dest "$tmp/apps"
has  "a new build replaces the old one"           'installed Kai 2.0.0'
has  "  ... and clears the stale grant"           'was cleared'
run --from "$tmp/Kai-1.0.0-darwin-arm64.zip" --dest "$tmp/apps" --keep-permissions
has  "--keep-permissions leaves the grant alone"  'left the Accessibility grant alone'

# The Accessibility grant follows the signing identity (the designated requirement, what
# `codesign -dr -` prints), not the binary hash: the bug was that every new build cleared it
# even though the certificate had not changed (#126).
mkdir -p "$tmp/idA"
run --from "$tmp/Kai-certA-1.0.0.zip" --dest "$tmp/idA"
has  "a certificate-signed build installs"        'installed Kai 1.0.0'
run --from "$tmp/Kai-certA-2.0.0.zip" --dest "$tmp/idA"
has  "the same certificate on a new build installs" 'installed Kai 2.0.0'
has  "  ... and keeps the grant"                    'same signing identity: the Accessibility grant is still valid'
hasnt "  ... and does not clear it"                 'was cleared'
mkdir -p "$tmp/idB"
run --from "$tmp/Kai-certA-1.0.0.zip" --dest "$tmp/idB"
run --from "$tmp/Kai-certB-2.0.0.zip" --dest "$tmp/idB"
has  "a different certificate clears the grant"   'was cleared'
mkdir -p "$tmp/adhoc"
run --from "$tmp/Kai-1.0.0-darwin-arm64.zip" --dest "$tmp/adhoc"
run --from "$tmp/Kai-certA-1.0.0.zip" --dest "$tmp/adhoc"
has  "an ad-hoc-signed old build still clears the grant" 'was cleared'
# Fail-safe: an old install whose signature cannot be read (here: stripped) is a changed
# identity as far as anyone can tell, so the grant is cleared, as on a first install.
mkdir -p "$tmp/nosig"
run --from "$tmp/Kai-certA-1.0.0.zip" --dest "$tmp/nosig"
codesign --remove-signature "$tmp/nosig/Kai.app"
run --from "$tmp/Kai-certA-2.0.0.zip" --dest "$tmp/nosig"
has  "an old requirement that cannot be read clears the grant" 'was cleared'

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
    mkdir -p "$dir"
    # Like the real release: the release zip, the updater zip (same bytes, "updater-" name), the
    # dmg and SHA256SUMS. The updater zip is created before the release zip on purpose, and
    # `gh` honours --pattern only as a glob on the name (so '*.zip' matches both zips), which is
    # why the installer has to choose explicitly.
    up="updater-$(basename "$FAKE_ZIP")"
    if [ -n "${FAKE_UPDATER_OTHER:-}" ]; then cp "$FAKE_OTHER_ZIP" "$dir/$up"; else cp "$FAKE_ZIP" "$dir/$up"; fi
    cp "$FAKE_ZIP" "$dir/"
    if [ -n "${FAKE_BAD:-}" ]; then echo "0000000000000000000000000000000000000000000000000000000000000000  $(basename "$FAKE_ZIP")" >"$dir/SHA256SUMS"
    else (cd "$dir" && shasum -a 256 "$(basename "$FAKE_ZIP")" "$up" >SHA256SUMS); fi ;;
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
# The release also carries updater-Kai-...zip. The installer must use the normal zip, not that one
# (here the updater zip holds a different build: 2.0.0, so picking it would install the wrong app).
export FAKE_OTHER_ZIP="$tmp/Kai-2.0.0-darwin-arm64.zip"
mkdir -p "$tmp/g5"
out="$(FAKE_UPDATER_OTHER=1 PATH="$tmp/fakebin:$PATH" "$inst" --dest "$tmp/g5" 2>&1)"; code=$?
has  "with an updater zip in the release, the normal zip is installed" 'installed Kai 1.0.0'
hasnt "  ... never the updater zip's app"          'installed Kai 2.0.0'
exit_is "  ... exit 0"                            0
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
run --from
has  "--from with no value says so"               '--from needs a value'
exit_is "  ... with exit 2"                       2
run --dest
has  "--dest with no value says so"               '--dest needs a value'
run --from "" 
has  "--from with an empty value says so"         '--from needs a value'

[[ $fail -eq 0 ]] && echo "PASS" || { echo "FAILED"; exit 1; }
