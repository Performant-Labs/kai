#!/usr/bin/env bash
# Tests for the Kai-dev variant plumbing (Makefile `dev-app`, build/darwin/Taskfile.yml,
# build/darwin/Info.dev.plist, `codesign-app.sh --check`) and for the guarantee that the prod build
# path is unchanged. No build, no signing, no keychain: dry runs and a fake `security`.
#   bash scripts/dev-variant.test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/.." && pwd)"
ca="$here/codesign-app.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

fail=0; out=""; code=0
has()   { if printf '%s' "$out" | grep -q -- "$2"; then echo "ok   $1"; else echo "FAIL $1 (wanted /$2/)"; fail=1; fi; }
hasnt() { if printf '%s' "$out" | grep -q -- "$2"; then echo "FAIL $1 (found /$2/)"; fail=1; else echo "ok   $1"; fi; }
truth() { if eval "$2"; then echo "ok   $1"; else echo "FAIL $1"; fail=1; fi; }
exit_is() { if [[ "$code" == "$2" ]]; then echo "ok   $1"; else echo "FAIL $1 (exit $code, wanted $2)"; fail=1; fi; }
# Value of a top-level plist <key>: the <string> that follows it.
plist_val() { perl -0ne "print \$1 if m{<key>$2</key>\s*<string>([^<]*)</string>}" "$1"; }
sha() { shasum -a 256 | cut -c1-64; }

# Pin every variable that .env or the environment could change the dry-run output with.
V=(VERSION=1.2.3 BUILD_TIME=T GIT_COMMIT=abc GITHUB_TOKEN= CNB_TOKEN= POSTHOG_TOKEN= POSTHOG_PROJECT_ID=)
mk() { make --no-print-directory -n -C "$root" "$@" "${V[@]}" 2>&1; }

echo "-- Info.plist files"
prod="$root/build/darwin/Info.plist"; dev="$root/build/darwin/Info.dev.plist"
truth "prod plist: CFBundleName is Kai"                       '[[ "$(plist_val "$prod" CFBundleName)" == "Kai" ]]'
truth "prod plist: identifier is net.dtapp.kai"               '[[ "$(plist_val "$prod" CFBundleIdentifier)" == "net.dtapp.kai" ]]'
truth "prod plist: executable is Kai"                         '[[ "$(plist_val "$prod" CFBundleExecutable)" == "Kai" ]]'
prod_sha="$(perl -0pe 's{(<key>CFBundleShortVersionString</key>\s*<string>)[^<]*}{$1VER}; s{(<key>CFBundleVersion</key>\s*<string>)[^<]*}{$1VER}' "$prod" | sha)"
truth "prod plist is byte-for-byte as before (version keys aside)" '[[ "$prod_sha" == 2535f7648ebd8928328af02a1be2518f4a8e06954bf7415c033cbe26c7d8bb6f ]]'
truth "dev plist: CFBundleName is Kai-dev"                    '[[ "$(plist_val "$dev" CFBundleName)" == "Kai-dev" ]]'
truth "dev plist: CFBundleDisplayName is Kai-dev"             '[[ "$(plist_val "$dev" CFBundleDisplayName)" == "Kai-dev" ]]'
truth "dev plist: identifier is net.dtapp.kai.dev"            '[[ "$(plist_val "$dev" CFBundleIdentifier)" == "net.dtapp.kai.dev" ]]'
truth "dev plist: identifier is the prod one plus .dev"       '[[ "$(plist_val "$dev" CFBundleIdentifier)" == "$(plist_val "$prod" CFBundleIdentifier).dev" ]]'
truth "dev plist: executable stays Kai (the Go binary's name)" '[[ "$(plist_val "$dev" CFBundleExecutable)" == "Kai" ]]'

echo "-- release-bump leaves the dev plist alone"
mkdir -p "$tmp/tree/scripts" "$tmp/tree/build/darwin"
cp "$here/release-bump.sh" "$tmp/tree/scripts/"; cp "$root/build/config.yml" "$tmp/tree/build/"
cp "$prod" "$dev" "$tmp/tree/build/darwin/"
before="$(sha <"$tmp/tree/build/darwin/Info.dev.plist")"
bash "$tmp/tree/scripts/release-bump.sh" 9.8.7 >/dev/null 2>&1
truth "release-bump edits the prod plist version"             '[[ "$(plist_val "$tmp/tree/build/darwin/Info.plist" CFBundleShortVersionString)" == "9.8.7" ]]'
truth "release-bump does not touch Info.dev.plist"            '[[ "$(sha <"$tmp/tree/build/darwin/Info.dev.plist")" == "$before" ]]'

echo "-- prod make targets are unchanged"
for spec in "darwin-build b0afdf6df724142581725d1ab0d703369ca98e63090f2897721c6bd8cd4cef01" \
            "darwin-package 3b4841f4f56b2374fb2f691578d26f5cf431c71542974c80936df2daa7ede4e1" \
            "darwin-package-dmg fc2acca6b6aec9fef324b8d7148d75e54ee0f4bf227ce3abb23f2f753da48c94" \
            "dev 60d464eeb60f5679ecaa62b68c357e0d258718688027391aeac26506aea9ea33"; do
  t="${spec%% *}"; want="${spec##* }"
  got="$(mk "$t" | sha)"
  truth "make $t prints exactly the historical command line"  '[[ "$got" == "$want" ]]'
done
out="$(mk darwin-package)"
has   "darwin-package still builds with DEV=false"            'DEV=false'
hasnt "darwin-package passes no identity (ad-hoc)"            'KAI_SIGN_IDENTITY'
hasnt "darwin-package does not touch the dev app"             'Kai-dev\|package:dev'

echo "-- make dev-app"
out="$(mk dev-app)"
has "dev-app builds with DEV=true"                            'DEV=true'
has "  ... through the dev packaging task"                    'darwin:package:dev'
has "  ... signing with the Kai Dev certificate by default"   'KAI_SIGN_IDENTITY="\?Kai Dev"\?'
has "  ... after a fast identity pre-check"                   'codesign-app.sh --check'
out="$(mk dev-app KAI_SIGN_IDENTITY="Other Cert")"
has "an explicit KAI_SIGN_IDENTITY overrides the default"     'KAI_SIGN_IDENTITY="\?Other Cert"\?'
hasnt "  ... and Kai Dev is then not used"                    'Kai Dev'
out="$(mk dev-app DEV=false)"
has "DEV=false on the command line cannot turn dev-app into a prod build" 'DEV=true'
out="$(mk dev-app KAI_SIGN_IDENTITY=)"
has "an empty KAI_SIGN_IDENTITY still means Kai Dev, never ad-hoc" 'KAI_SIGN_IDENTITY="\?Kai Dev"\?'
truth "dev-app is a .PHONY target listed in help"             'grep -q "^dev-app: ##" "$root/Makefile" && grep -Eq "^\.PHONY:.* dev-app( |$)" "$root/Makefile"'

echo "-- Taskfile"
tf="$root/build/darwin/Taskfile.yml"
truth "the dev bundle is Kai-dev.app"                         'grep -q "Kai-dev.app" "$tf"'
truth "  ... built from Info.dev.plist"                       'grep -q "build/darwin/Info.dev.plist" "$tf"'
truth "  ... and the old Kai.dev.app name is gone"            '! grep -q "\.dev\.app" "$tf"'
truth "there is a darwin:package:dev task"                    'grep -Eq "^  package:dev:" "$tf"'
truth "prod bundle task still copies Info.plist and is untouched" 'grep -q "cp build/darwin/Info.plist \"{{.BIN_DIR}}/{{.APP_NAME}}.app/Contents\"" "$tf"'
truth "the dev packaging task launches nothing"               '! awk "/^  package:dev:/,/^  [a-z:]+:\$/ && !/^  package:dev:/" "$tf" | grep -q "MacOS/{{.APP_NAME}}\"'"'"'"'

echo "-- codesign-app.sh --check (fast identity pre-check)"
mkdir "$tmp/bin"
cat >"$tmp/bin/codesign" <<'FAKE'
#!/usr/bin/env bash
echo "codesign $*" >>"$FAKE_LOG"
FAKE
cat >"$tmp/bin/security" <<'FAKE'
#!/usr/bin/env bash
if [[ "$1" == find-identity ]]; then
  echo "Policy: Code Signing"
  echo '  1) DC5B8B8DAA588F1B265FF845C19C5384D6C969B2 "Kai Dev" (CSSMERR_TP_NOT_TRUSTED)'
  echo "     1 identities found"
fi
FAKE
chmod +x "$tmp/bin/"*
run() { : >"$tmp/log"; out="$(env PATH="$tmp/bin:$PATH" FAKE_LOG="$tmp/log" "$@" 2>&1)"; code=$?; }

run env KAI_SIGN_IDENTITY="Kai Dev" "$ca" --check
exit_is "a present identity passes the check"                 0
truth "  ... and signs nothing"                               '[[ ! -s "$tmp/log" ]]'
run env KAI_SIGN_IDENTITY="Kai Dev" "$ca" --check "$tmp/anything.app"
exit_is "the check takes no app argument seriously (still 0)" 0
run env KAI_SIGN_IDENTITY="Kai Dev Missing" "$ca" --check
exit_is "a missing identity fails the check"                  1
has "  ... naming the identity"                               'Kai Dev Missing'
has "  ... pointing at docs/dev-signing.md"                   'docs/dev-signing.md'
has "  ... refusing to fall back to ad-hoc"                   'Refusing to fall back to an ad-hoc'
truth "  ... and signs nothing"                               '[[ ! -s "$tmp/log" ]]'
run env -u KAI_SIGN_IDENTITY "$ca" --check
exit_is "no identity at all (ad-hoc build) passes the check"  0
run env KAI_SIGN_IDENTITY="Kai Dev" "$ca" bin/Kai-dev.app
truth "signing a Kai-dev.app path is the same command"        '[[ "$(cat "$tmp/log")" == "codesign --force --deep --sign Kai Dev bin/Kai-dev.app" ]]'

exit "$fail"
