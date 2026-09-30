#!/usr/bin/env bash
# Sign a Kai .app bundle. Used by build/darwin/Taskfile.yml for the release and dev variants.
#
#   scripts/codesign-app.sh /path/to/Kai.app
#
# With KAI_SIGN_IDENTITY unset (or empty) this is the historical ad-hoc signature, byte for byte:
#   codesign --force --deep --sign - APP
# With KAI_SIGN_IDENTITY="Kai Dev" (a certificate name or its SHA-1 hash) it signs with that
# certificate, so the code requirement is "identifier + certificate" and survives rebuilds
# (see docs/dev-signing.md). If the named identity cannot be found the build FAILS: it never
# falls back to ad-hoc when an identity was asked for.
set -euo pipefail

app="${1:-}"
[[ -n "$app" ]] || { echo "usage: $0 /path/to/Kai.app" >&2; exit 2; }

identity="${KAI_SIGN_IDENTITY:-}"
if [[ -z "$identity" ]]; then
  exec codesign --force --deep --sign - "$app"
fi

# Not -v: a self-signed certificate that is not trusted is still usable by codesign, and codesign
# itself is the judge of that. This check only turns "no such identity" into a readable error.
listing="$(security find-identity -p codesigning 2>/dev/null || true)"
if ! printf '%s\n' "$listing" | grep -F -q -- "$identity"; then
  {
    echo "error: KAI_SIGN_IDENTITY=\"$identity\" is set, but no code-signing identity with that"
    echo "name or hash exists in your keychains. Refusing to fall back to an ad-hoc signature."
    echo "Create the certificate once (docs/dev-signing.md) or unset KAI_SIGN_IDENTITY."
    echo "Identities found:"
    printf '%s\n' "$listing" | sed 's/^/    /'
  } >&2
  exit 1
fi

exec codesign --force --deep --sign "$identity" "$app"
