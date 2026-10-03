#!/usr/bin/env bash
# Create the self-signed code-signing certificate that signs releases, in your login keychain.
#
#   scripts/release-cert.sh [NAME]        NAME defaults to "Kai Release"
#
# Run it once, on the Mac that builds releases. It is the release counterpart of the "Kai Dev"
# certificate (docs/dev-signing.md): signing with it makes the code requirement
# `identifier + certificate leaf` instead of a binary hash, so macOS permissions survive rebuilds.
# It is NOT a Developer ID certificate: Gatekeeper still blocks a downloaded copy.
#
# What it does, and nothing else:
#   - makes a 2048-bit RSA key and a 10-year self-signed certificate (code signing only) in a
#     temporary folder, with the system's LibreSSL (a Homebrew OpenSSL 3 writes a .p12 that
#     `security import` cannot read);
#   - imports key and certificate into the login keychain, allowing codesign to use the key;
#   - deletes the temporary files. The temporary passphrase is random, never printed or stored.
# It does not change keychain trust settings (so it needs no admin password): codesign can sign
# with an untrusted self-signed certificate. It refuses to run if an identity with that name exists.
#
# The first signing with the new key makes macOS ask "codesign wants to sign using key": click
# Always Allow. Back the key up yourself: Keychain Access > My Certificates > the certificate >
# Export as .p12, and keep it in your password manager. Losing the key means a new identity (every
# user grants permissions again); a leaked key lets anyone sign as this identity.
set -euo pipefail

name="${1:-Kai Release}"
keychain="$HOME/Library/Keychains/login.keychain-db"
ssl=/usr/bin/openssl

[[ "$(uname -s)" == "Darwin" ]] || { echo "this runs on macOS" >&2; exit 1; }
[[ -x "$ssl" ]] || { echo "no $ssl" >&2; exit 1; }
[[ -f "$keychain" ]] || { echo "no login keychain at $keychain" >&2; exit 1; }

if security find-identity -p codesigning "$keychain" 2>/dev/null | grep -F -q "\"$name\""; then
  echo "an identity named \"$name\" already exists in the login keychain; not creating another:"
  security find-identity -p codesigning "$keychain" | grep -F "\"$name\""
  exit 0
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
pass="$("$ssl" rand -hex 16)"

cat >"$tmp/req.cnf" <<CNF
[req]
distinguished_name = dn
x509_extensions = ext
prompt = no
[dn]
CN = $name
O = Performant Labs
[ext]
basicConstraints = critical,CA:false
keyUsage = critical,digitalSignature
extendedKeyUsage = critical,codeSigning
subjectKeyIdentifier = hash
CNF

"$ssl" req -x509 -newkey rsa:2048 -nodes -days 3650 -config "$tmp/req.cnf" \
  -keyout "$tmp/key.pem" -out "$tmp/cert.pem" 2>/dev/null
"$ssl" pkcs12 -export -inkey "$tmp/key.pem" -in "$tmp/cert.pem" -name "$name" \
  -out "$tmp/id.p12" -passout "pass:$pass"
security import "$tmp/id.p12" -k "$keychain" -P "$pass" -T /usr/bin/codesign >/dev/null

echo "created \"$name\" in the login keychain:"
security find-identity -p codesigning "$keychain" | grep -F "\"$name\"" || {
  echo "FAIL: the identity is not listed after the import" >&2; exit 1; }
echo
echo "SHA-1 of the certificate (this is the 'certificate leaf' in the code requirement):"
"$ssl" x509 -in "$tmp/cert.pem" -noout -fingerprint -sha1 | sed 's/.*=//; s/://g'
echo
echo "Next: back the key up (Keychain Access > My Certificates > $name > Export as .p12)."
