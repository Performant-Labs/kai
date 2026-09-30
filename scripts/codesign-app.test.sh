#!/usr/bin/env bash
# Tests for scripts/codesign-app.sh and the Makefile wiring of KAI_SIGN_IDENTITY, using a fake
# `codesign` and `security` on PATH (no real signing, no keychain access).
#   bash scripts/codesign-app.test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/.." && pwd)"
ca="$here/codesign-app.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin"
cat >"$tmp/bin/codesign" <<'FAKE'
#!/usr/bin/env bash
echo "codesign $*" >>"$FAKE_LOG"
FAKE
cat >"$tmp/bin/security" <<'FAKE'
#!/usr/bin/env bash
# `security find-identity -p codesigning` with one untrusted self-signed identity, as seen for real.
if [[ "$1" == find-identity ]]; then
  echo "Policy: Code Signing"
  if [[ " $* " == *" -v "* ]]; then echo "     0 valid identities found"; exit 0; fi   # untrusted: not listed by -v
  echo '  1) DC5B8B8DAA588F1B265FF845C19C5384D6C969B2 "Kai Dev" (CSSMERR_TP_NOT_TRUSTED)'
  echo "     1 identities found"
fi
FAKE
chmod +x "$tmp/bin/"*

fail=0; out=""; code=0
has()   { if printf '%s' "$out" | grep -q -- "$2"; then echo "ok   $1"; else echo "FAIL $1 (wanted /$2/)"; fail=1; fi; }
truth() { if eval "$2"; then echo "ok   $1"; else echo "FAIL $1"; fail=1; fi; }
exit_is() { if [[ "$code" == "$2" ]]; then echo "ok   $1"; else echo "FAIL $1 (exit $code, wanted $2)"; fail=1; fi; }
run() { : >"$tmp/log"; out="$(env PATH="$tmp/bin:$PATH" FAKE_LOG="$tmp/log" "$@" 2>&1)"; code=$?; }

# Unset: exactly today's ad-hoc command.
run env -u KAI_SIGN_IDENTITY "$ca" bin/Kai.app
exit_is "unset identity succeeds"                                0
truth "  ... runs exactly the historical ad-hoc command"          '[[ "$(cat "$tmp/log")" == "codesign --force --deep --sign - bin/Kai.app" ]]'
run env KAI_SIGN_IDENTITY= "$ca" bin/Kai.app
truth "an empty identity is the same as unset"                    '[[ "$(cat "$tmp/log")" == "codesign --force --deep --sign - bin/Kai.app" ]]'

# Set and present (untrusted self-signed certs are not in `-v`, so the check must not use -v).
run env KAI_SIGN_IDENTITY="Kai Dev" "$ca" bin/Kai.app
exit_is "a present identity succeeds"                             0
truth "  ... signs with that identity, keeping --force --deep"    '[[ "$(cat "$tmp/log")" == "codesign --force --deep --sign Kai Dev bin/Kai.app" ]]'
truth "  ... no hardened runtime or timestamp flags"              '! grep -Eq -- "--options|--timestamp" "$tmp/log"'
run env KAI_SIGN_IDENTITY=DC5B8B8DAA588F1B265FF845C19C5384D6C969B2 "$ca" bin/Kai.app
exit_is "an identity given by hash succeeds"                      0

# Set and absent: fail loudly, never sign ad-hoc.
run env KAI_SIGN_IDENTITY="No Such Cert" "$ca" bin/Kai.app
exit_is "a missing identity fails the build"                      1
has "  ... with a clear message"                                  'no code-signing identity'
has "  ... that refuses to fall back to ad-hoc"                   'Refusing to fall back to an ad-hoc'
truth "  ... and codesign was never run"                          '[[ ! -s "$tmp/log" ]]'
run env -u KAI_SIGN_IDENTITY "$ca"
exit_is "no app argument is a usage error"                        2

# Makefile plumbing: every darwin recipe that runs wails3 carries the identity.
for t in darwin-build darwin-package darwin-package-dmg dev; do
  mk="$(make -n -C "$root" "$t" KAI_SIGN_IDENTITY="Kai Dev" 2>&1)"
  if printf '%s' "$mk" | grep -q 'KAI_SIGN_IDENTITY=Kai Dev\|KAI_SIGN_IDENTITY="Kai Dev"'; then echo "ok   make $t passes the identity"; else echo "FAIL make $t does not pass the identity"; fail=1; fi
done
mk="$(make -n -C "$root" darwin-package 2>&1)"
if printf '%s' "$mk" | grep -q 'KAI_SIGN_IDENTITY=[^ ]'; then echo "FAIL make darwin-package invents an identity when unset"; fail=1; else echo "ok   make darwin-package passes no identity when unset"; fi

# Taskfile: both signing sites go through the script; no raw ad-hoc codesign is left.
tf="$root/build/darwin/Taskfile.yml"
truth "the Taskfile has no hard-coded ad-hoc codesign"            '! grep -Eq "codesign --force --deep --sign -" "$tf"'
truth "  ... both bundles are signed via scripts/codesign-app.sh" '[[ "$(grep -c "scripts/codesign-app.sh" "$tf")" -ge 2 ]]'

[[ $fail -eq 0 ]] && echo "ALL PASS" || { echo "SOME FAILED"; exit 1; }
