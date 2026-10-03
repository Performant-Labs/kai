#!/usr/bin/env bash
# Tests for scripts/sign-check.sh, using a fake `codesign` on PATH that prints the `-dr -` output
# seen on a real Mac for each kind of signature.
#   bash scripts/sign-check.test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
sc="$here/sign-check.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/bin" "$tmp/Kai.app"
cat >"$tmp/bin/codesign" <<'FAKE'
#!/usr/bin/env bash
case "${FAKE_KIND:-}" in
  adhoc)  printf 'Executable=/x/Kai\n# designated => cdhash H"baa23ec59eab0b294f93e36b9f7668c3a89f01e4"\n' >&2 ;;
  cert)   printf 'Executable=/x/Kai\ndesignated => identifier "com.performantlabs.kai" and certificate leaf = H"dc5b8b8daa588f1b265ff845c19c5384d6c969b2"\n' >&2 ;;
  devcert) printf 'Executable=/x/Kai\ndesignated => identifier "com.performantlabs.kai.dev" and certificate leaf = H"2a573c82aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"\n' >&2 ;;
  devadhoc) printf 'Executable=/x/Kai\n# designated => cdhash H"baa23ec59eab0b294f93e36b9f7668c3a89f01e4"\n' >&2 ;;
  appledev) printf 'Executable=/x/Kai\ndesignated => identifier "com.performantlabs.kai" and anchor apple generic and certificate leaf[subject.CN] = "Apple Development: a@b.c (ABCDE12345)" and certificate 1[field.1.2.840.113635.100.6.2.1] /* exists */\n' >&2 ;;
  devid)  printf 'Executable=/x/Kai\ndesignated => identifier "com.performantlabs.kai" and anchor apple generic and certificate 1[field.1.2.840.113635.100.6.2.6] /* exists */ and certificate leaf[field.1.2.840.113635.100.6.1.13] /* exists */ and certificate leaf[subject.OU] = "TEAMID1234"\n' >&2 ;;
  unsigned) echo "$2: code object is not signed at all" >&2; exit 1 ;;
esac
FAKE
chmod +x "$tmp/bin/codesign"

fail=0; out=""; code=0
has()   { if printf '%s' "$out" | grep -q -- "$2"; then echo "ok   $1"; else echo "FAIL $1 (wanted /$2/)"; fail=1; fi; }
hasnt() { if printf '%s' "$out" | grep -q -- "$2"; then echo "FAIL $1 (did not want /$2/)"; fail=1; else echo "ok   $1"; fi; }
exit_is() { if [[ "$code" == "$2" ]]; then echo "ok   $1"; else echo "FAIL $1 (exit $code, wanted $2)"; fail=1; fi; }
run() { local kind="$1"; shift; out="$(PATH="$tmp/bin:$PATH" FAKE_KIND="$kind" "$sc" "$@" 2>&1)"; code=$?; }

run adhoc "$tmp/Kai.app"
has "ad-hoc is reported as ad-hoc"                     'signature kind: ad-hoc'
has "  ... and says grants are lost on every rebuild"   'LOST on every rebuild'
has "  ... and prints the requirement"                  'cdhash H"baa23'
exit_is "  ... exit 0 (information only)"               0
run adhoc "$tmp/Kai.app" --require-stable
exit_is "--require-stable fails for ad-hoc"             1
has "  ... and says why"                                'depends on the binary hash'
run adhoc --require-stable "$tmp/Kai.app"
exit_is "  ... flag order does not matter"              1

run cert "$tmp/Kai.app"
has "a certificate requirement is a stable certificate" 'signature kind: stable local/development certificate'
has "  ... and says grants survive rebuilds"            'survive rebuilds'
hasnt "  ... and is not called ad-hoc"                  'signature kind: ad-hoc'
exit_is "  ... exit 0"                                  0
run cert "$tmp/Kai.app" --require-stable
exit_is "--require-stable passes for a certificate"     0

# The dev variant (Kai-dev, com.performantlabs.kai.dev) is classified like any other bundle id.
mkdir "$tmp/Kai-dev.app"
run devcert "$tmp/Kai-dev.app" --require-stable
has "a Kai-dev app signed with the certificate is stable" 'signature kind: stable local/development certificate'
has "  ... and the requirement shows the dev identifier"  'identifier "com.performantlabs.kai.dev"'
exit_is "  ... and passes --require-stable"                0
run devadhoc "$tmp/Kai-dev.app" --require-stable
has "a Kai-dev app signed ad-hoc is reported as ad-hoc"   'signature kind: ad-hoc'
exit_is "  ... and fails --require-stable"                 1

run appledev "$tmp/Kai.app" --require-stable
has "Apple Development is a stable certificate"         'stable local/development certificate'
exit_is "  ... and passes --require-stable"             0

run devid "$tmp/Kai.app" --require-stable
has "Developer ID is reported as Developer ID"          'signature kind: Developer ID'
exit_is "  ... and passes --require-stable"             0

run adhoc "$tmp/nope.app"
exit_is "a missing app fails cleanly"                   1
has "  ... naming it"                                   'no such app'
run unsigned "$tmp/Kai.app"
exit_is "an unsigned app fails cleanly"                 1
run adhoc
exit_is "no argument is a usage error"                  2

[[ $fail -eq 0 ]] && echo "ALL PASS" || { echo "SOME FAILED"; exit 1; }
