#!/usr/bin/env bash
# Tests for scripts/release.sh: the static release workflow (issue #78), in throwaway repos
# with fake wrapped scripts first on PATH, no network. Harness shape follows
# scripts/release-preflight.test.sh (mkrepo/run/has + fake gh + a fake git wrapping the real
# one, origin = a local bare repo). The contract under test is docs/handoffs/0078-issue-78/
# handoff-A.md (rulings R1-R12) as it amends docs/handoffs/0078-issue-78/brief.md (AC1-AC7).
# Part of test.unit.command (issue #78, brief item 9).
#   bash scripts/release.test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
release="$here/release.sh"
tmp="$(mktemp -d)"
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT

fail=0; out=""; code=0
has()   { if printf '%s' "$out" | grep -qE -- "$2"; then echo "ok   $1"; else echo "FAIL $1 (wanted /$2/)"; fail=1; fi; }
hasnt() { if printf '%s' "$out" | grep -qE -- "$2"; then echo "FAIL $1 (did not want /$2/)"; fail=1; else echo "ok   $1"; fi; }
exit_is() { if [[ "$code" == "$2" ]]; then echo "ok   $1"; else echo "FAIL $1 (exit $code, wanted $2)"; fail=1; fi; }
exit_nonzero() { if [[ "$code" -ne 0 ]]; then echo "ok   $1"; else echo "FAIL $1 (exit $code, wanted non-zero)"; fail=1; fi; }
fhas()  { if grep -qE -- "$3" "$2" 2>/dev/null; then echo "ok   $1"; else echo "FAIL $1 (wanted /$3/ in $(basename "$2"))"; fail=1; fi; }
fhasnt(){ if grep -qE -- "$3" "$2" 2>/dev/null; then echo "FAIL $1 (did not want /$3/ in $(basename "$2"))"; fail=1; else echo "ok   $1"; fi; }
exists() { if [[ -e "$2" ]]; then echo "ok   $1"; else echo "FAIL $1 (missing ${2#"$tmp/"})"; fail=1; fi; }
count_is() { # name file want-line-count
  local n=0; [[ -f "$2" ]] && n="$(wc -l <"$2" | tr -d ' ')"
  if [[ "$n" == "$3" ]]; then echo "ok   $1"; else echo "FAIL $1 ($(basename "$2") has $n lines, wanted $3)"; fail=1; fi; }
match_is() { # name file pattern want-match-count
  local n=""; [[ -f "$2" ]] && n="$(grep -cE -- "$3" "$2" 2>/dev/null || true)"
  [[ -z "$n" ]] && n=0
  if [[ "$n" == "$4" ]]; then echo "ok   $1"; else echo "FAIL $1 ($(basename "$2") =~ /$3/ matched $n, wanted $4)"; fail=1; fi; }
val_is() { # name file want-content
  local v=""; [[ -f "$2" ]] && v="$(tr -d '[:space:]' <"$2")"
  if [[ "$v" == "$3" ]]; then echo "ok   $1"; else echo "FAIL $1 ($(basename "$2") is '$v', wanted '$3')"; fail=1; fi; }
jq_is() { # name file filter want
  local got=""; [[ -f "$2" ]] && got="$(jq -r "$3" "$2" 2>/dev/null)" || true
  if [[ "$got" == "$4" ]]; then echo "ok   $1"; else echo "FAIL $1 (jq '$3' gave '$got', wanted '$4')"; fail=1; fi; }

[[ -x /usr/bin/git ]] || { echo "harness needs /usr/bin/git"; exit 2; }
RG=/usr/bin/git

# ------------------------------------------------------- fixture artifacts (phase 13, R10)
# A real zip (real ditto/shasum/cmp/diff run against it) and a fake dmg only the fake hdiutil
# ever opens. SHA256SUMS covers the three files: checklist step 28's shape.
FX="$tmp/fixtures"; mkdir -p "$FX/Kai.app" "$FX/download"
echo "fake app" >"$FX/Kai.app/Info.plist"
( cd "$FX" && ditto -c -k --keepParent Kai.app download/Kai-0.5.1-darwin-arm64.zip )
cp "$FX/download/Kai-0.5.1-darwin-arm64.zip" "$FX/download/updater-Kai-0.5.1-darwin-arm64.zip"
: >"$FX/download/Kai-0.5.1-darwin-arm64.dmg"
( cd "$FX/download" && shasum -a 256 Kai-0.5.1-darwin-arm64.zip \
    updater-Kai-0.5.1-darwin-arm64.zip Kai-0.5.1-darwin-arm64.dmg >SHA256SUMS )

# ------------------------------------------------------- the fake bin (shared, stateless)
# Every script release.sh may invoke by bare name (Ruling R7) resolves here first. All state
# lives in $FAKE_STATE_DIR (one dir per scenario), never in these files.
FB="$tmp/fakebin"; mkdir -p "$FB"

cat >"$FB/git" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >>"\$FAKE_STATE_DIR/git.log"
exec $RG "\$@"
EOF

cat >"$FB/pgrep" <<'EOF'
#!/usr/bin/env bash
# Kai-running is steered by a marker file the fake release-local.sh sets; the fake osascript /
# pkill clear it when told to quit (that is the OS effect of quitting Kai).
if [[ "$*" == "-x Kai" && -e "$FAKE_STATE_DIR/kai-running" ]]; then exit 0; fi
exit 1
EOF

cat >"$FB/pkill" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/pkill.log"
[[ "$*" == "-x Kai" ]] && rm -f "$FAKE_STATE_DIR/kai-running"
exit 0
EOF

cat >"$FB/osascript" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/osascript.log"
case "$*" in
  *[Qq]uit*) rm -f "$FAKE_STATE_DIR/kai-running" ;;   # quitting Kai has its OS effect
esac
exit 0
EOF

cat >"$FB/make" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/make.log"
if [[ "${1:-}" == "bindings" ]]; then
  mkdir -p "$PWD/frontend/bindings"
  echo "// regenerated (fake)" >"$PWD/frontend/bindings/generated.ts"
  rm -f "$FAKE_STATE_DIR/local-fail"    # the drift is fixed once bindings are regenerated
fi
exit 0
EOF

cat >"$FB/hdiutil" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/hdiutil.log"
case "${1:-}" in
  attach) mkdir -p "$FAKE_STATE_DIR/mnt/Kai.app"
          cp -R "$FAKE_FIXTURES/Kai.app/." "$FAKE_STATE_DIR/mnt/Kai.app/"
          printf '/dev/disk9s1\tApple_HFS\t%s\n' "$FAKE_STATE_DIR/mnt" ;;
  detach) rm -rf "$FAKE_STATE_DIR/mnt"; echo "ejected" ;;
esac
exit 0
EOF

cat >"$FB/codesign-app.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/codesign.log"
if [[ "${1:-}" == "--check" && -e "$FAKE_STATE_DIR/codesign-fail" ]]; then
  echo 'FAIL: no identity "Kai Release" matches in any keychain' >&2; exit 1
fi
exit 0
EOF

cat >"$FB/release-preflight.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/preflight.log"
if [[ -e "$FAKE_STATE_DIR/preflight-fail" ]]; then
  echo "FAIL: 1 problem(s), 0 warning(s)." >&2; exit 1
fi
echo "  WARN  node v26.0 differs from frontend/.nvmrc (fake row, for the marker comment)"
echo "PASS: ready to start $1. 1 warning(s): record each in the checklist."
exit 0
EOF

cat >"$FB/changelog-check.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/changelog.log"
if [[ "$*" == *"--strict"* && -e "$FAKE_STATE_DIR/changelog-strict-fail" ]]; then
  echo "NOT covered by [Unreleased] (1):"
  echo "  #501	2026-10-01	Fake uncovered PR (#501)"
  exit 1
fi
echo "ok: every merged PR in range is covered by [Unreleased] (or skipped)"
exit 0
EOF

cat >"$FB/release-bump.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/bump.log"
n=0; [[ -f "$FAKE_STATE_DIR/bump.count" ]] && n="$(cat "$FAKE_STATE_DIR/bump.count")"
n=$((n + 1)); echo "$n" >"$FAKE_STATE_DIR/bump.count"
if [[ "$n" -le "${FAKE_BUMP_FAILS:-0}" ]]; then
  echo "FAIL: bump exploded (fake, invocation $n)" >&2; exit 1
fi
[[ -f build/config.yml ]] && sed -i.bak -E "s/version: \"[^\"]*\"/version: \"$1\"/" build/config.yml && rm -f build/config.yml.bak
echo "version set to $1"
exit 0
EOF

cat >"$FB/release-local.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/local.log"
tag=""; dry=0
for a in "$@"; do case "$a" in --dry-run) dry=1 ;; v[0-9]*.[0-9]*.[0-9]*) tag="$a" ;; esac; done
ver="${tag#v}"; out="$PWD/bin/release/$tag"
if [[ -e "$FAKE_STATE_DIR/local-fail" ]]; then
  kind="$(cat "$FAKE_STATE_DIR/local-fail")"
  case "$kind" in
    once)    rm -f "$FAKE_STATE_DIR/local-fail"
             echo "FAIL: unknown transient failure (fake)" >&2; exit 1 ;;
    kai)     rm -f "$FAKE_STATE_DIR/local-fail"; : >"$FAKE_STATE_DIR/kai-running"
             echo "FAIL: Kai is running: quit it (release-verify.sh launches its own copy)" >&2; exit 1 ;;
    bindings) echo "FAIL: the tree is not the tag: frontend/bindings differs (run make bindings and commit the result, #73)" >&2; exit 1 ;;
    exists-draft)     echo "FAIL: a release for $tag already exists: https://github.com/Performant-Labs/kai/releases/$tag (draft)" >&2; exit 1 ;;
    exists-published) echo "FAIL: a release for $tag already exists: https://github.com/Performant-Labs/kai/releases/$tag (published, isDraft: false)" >&2; exit 1 ;;
  esac
fi
mkdir -p "$out/assets" "$out/Kai.app"
echo "notes for $ver" >"$out/RELEASE_NOTES.md"
echo app >"$out/Kai.app/Info.plist"
: >"$out/assets/Kai-$ver-darwin-arm64.zip"
: >"$out/assets/updater-Kai-$ver-darwin-arm64.zip"
: >"$out/assets/Kai-$ver-darwin-arm64.dmg"
: >"$out/assets/SHA256SUMS"
echo "packaged into $out"
if [[ "$dry" -eq 1 ]]; then printf '\nDry run done: no release created. Files are in %s\n' "$out"; exit 0; fi
gh release create "$tag" "$out"/assets/* --repo Performant-Labs/kai --draft --verify-tag \
  --title "Kai $tag" --notes-file "$out/RELEASE_NOTES.md"
printf '\nDraft created. Next:\n  1. Run the smoke matrix (docs/release-checklist.md) on %s/Kai.app\n  2. Publish:  gh release edit %s --repo Performant-Labs/kai --draft=false\n' "$out" "$tag"
exit 0
EOF

cat >"$FB/release-ask-ai.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/askai.log"
n=0; [[ -f "$FAKE_STATE_DIR/askai.count" ]] && n="$(cat "$FAKE_STATE_DIR/askai.count")"
n=$((n + 1)); echo "$n" >"$FAKE_STATE_DIR/askai.count"
case "${FAKE_AI:-}" in
  retry) echo '{"diagnosis":"fake","action":"retry","argument":"","reason":"fake"}'; exit 0 ;;
  wait)  echo '{"diagnosis":"fake","action":"wait","argument":"","reason":"fake"}'; exit 0 ;;
  stop)  echo '{"diagnosis":"fake","action":"stop","argument":"","reason":"fake"}'; exit 0 ;;
  bad)   echo '{"diagnosis":"fake","action":"reboot-the-mac","argument":"","reason":"fake"}'; exit 0 ;;
  *)     exit 1 ;;   # no answer available: fail-closed
esac
EOF

for s in release-notes.sh release-check-version.sh release-tree-check.sh release-verify.sh \
         release-package.sh release-gates.sh sign-check.sh release-cert.sh; do
  cat >"$FB/$s" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >>"\$FAKE_STATE_DIR/$s.log"
echo "ok (fake $s)"
exit 0
EOF
done

cat >"$FB/install-release.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/install-release.log"
echo "checksum ok"; echo "installed Kai $1"; echo "quarantine flag cleared"
exit 0
EOF

cat >"$FB/gh" <<'EOF'
#!/usr/bin/env bash
# Fake gh: env-steered canned answers (FAKE_CI, FAKE_AGENT) + a small state dir
# ($FAKE_STATE_DIR/ghstate): issue-<N> = title/state, pr-<N> = state/head/head_sha,
# release-<tag>.json. Lists print TSV, or JSON when --json is passed. No network.
G="$FAKE_STATE_DIR/ghstate"
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/gh.log"
a="$*"
next() { n=100; [[ -f "$G/seq" ]] && n="$(cat "$G/seq")"; n=$((n + 1)); echo "$n" >"$G/seq"; echo "$n"; }
pragent() { # <pr-file> <want-json-flag> — the PR-Agent review comment (R1's machine block)
  local f="$1" head_sha score findings
  head_sha="$(sed -n 3p "$f" 2>/dev/null)"; score=90; findings='[{"state":"RESOLVED"}]'
  case "${FAKE_AGENT:-90}" in
    88)    score=88; findings='[{"state":"RESOLVED"},{"state":"RESOLVED"}]' ;;
    95o)   score=95; findings='[{"state":"OPEN"},{"state":"RESOLVED"}]' ;;
    90)    score=90; findings='[{"state":"RESOLVED"}]' ;;
  esac
  printf '<td>🏅&nbsp;<strong>Score</strong>: %s</td>\n<!-- pr-agent-review-state:v1\n{"findings":%s,"last_run":{"complete":true,"head_sha":"%s"}}\n-->' \
    "$score" "$findings" "$head_sha" | jq -Rs '{body: .}' | jq -s '.'
}
case "$a" in
  "auth status"*) exit 0 ;;
  *"/commits/"*"/check-runs"*)
    case "${FAKE_CI:-green}" in
      green)   printf 'ci / go / build-lint-test\tcompleted\tsuccess\nci / frontend / typecheck\tcompleted\tskipped\n' ;;
      failed)  printf 'ci / go / build-lint-test\tcompleted\tfailure\n' ;;
      none)    : ;;
    esac
    exit 0 ;;
  *"pr checks"*)
    case "${FAKE_CI:-green}" in
      green) printf 'ci / go / build-lint-test\tpass\thttps://x/1\nci / frontend / typecheck\tpass\thttps://x/2\n'; exit 0 ;;
      *)     printf 'ci / go / build-lint-test\tfail\thttps://x/1\n'; exit 1 ;;
    esac ;;
  *"-X DELETE"*"git/refs/heads/"*)
    ref="$(printf '%s' "$a" | sed -n 's/.*git\/refs\/heads\/\(release\/v[0-9.]*\).*/\1/p')"
    /usr/bin/git -C "$PWD" push -q origin --delete "$ref" && exit 0
    exit 1 ;;
  *"pulls/"*"/comments"*)
    pr="$(printf '%s' "$a" | sed -n 's/.*pulls\/\([0-9]*\).*/\1/p')"
    f="$G/pr-$pr"; [[ -f "$f" ]] || f="$(ls "$G"/pr-* 2>/dev/null | sed -n 1p)"
    if [[ -n "${f:-}" && -f "$f" ]]; then pragent "$f" json; else echo "[]"; fi
    exit 0 ;;
  *"releases/tags/"*)
    tag="$(printf '%s' "$a" | sed -n 's/.*releases\/tags\/\(v[0-9.]*\).*/\1/p')"
    [[ -f "$G/release-$tag.json" ]] && { cat "$G/release-$tag.json"; exit 0; }
    echo "HTTP 404" >&2; exit 1 ;;
  *"api repos/Performant-Labs/kai"*)
    if [[ -e "$G/no-push" ]]; then echo "false"; else echo "true"; fi
    exit 0 ;;
  *"issue create"*)
    title="$(printf '%s' "$a" | sed -n 's/.*--title \(Release checklist: v[0-9.]*\).*/\1/p')"
    [[ -n "$title" ]] || title="untitled"
    n="$(next)"; printf '%s\nopen\n' "$title" >"$G/issue-$n"; echo "$n"; exit 0 ;;
  *"issue list"*)
    if [[ "$a" == *"--search"* ]]; then
      for f in "$G"/issue-*; do
        [[ -f "$f" ]] || continue
        t="$(sed -n 1p "$f")"
        case "$t" in "Release checklist:"*)
          if [[ "$a" == *"--json"* ]]; then
            printf '{"number":%s,"title":"%s","state":"%s"}\n' "${f##*issue-}" "$t" "$(sed -n 2p "$f")"
          else
            printf '%s\t%s\t%s\n' "${f##*issue-}" "$t" "$(sed -n 2p "$f")"
          fi ;; esac
      done
      exit 0
    elif [[ "$a" == *"--label bug"* ]]; then
      if [[ "$a" == *"--json"* ]]; then echo '[{"number":501,"title":"Crash when the log fills up","state":"OPEN"}]';
      else printf '501\tCrash when the log fills up\tOPEN\n'; fi
      exit 0
    elif [[ "$a" == *"--label known-issue"* ]]; then
      if [[ "$a" == *"--json"* ]]; then echo '[{"number":502,"title":"Accessibility grant resets","state":"OPEN"}]';
      else printf '502\tAccessibility grant resets\tOPEN\n'; fi
      exit 0
    fi
    exit 0 ;;
  *"issue comment"*)
    n="$(printf '%s' "$a" | sed -n 's/issue comment \([0-9]*\).*/\1/p')"
    if [[ "$a" == *"--body-file"* ]]; then
      bf="$(printf '%s' "$a" | sed -n 's/.*--body-file \([^ ]*\).*/\1/p')"
      body="$(cat "$bf" 2>/dev/null || true)"
    else
      body="$(printf '%s' "$a" | sed -n 's/.*--body //p')"
    fi
    printf '=== comment issue %s\n%s\n' "$n" "$body" >>"$FAKE_STATE_DIR/comments.log"
    exit 0 ;;
  *"issue close"*)
    n="$(printf '%s' "$a" | sed -n 's/issue close \([0-9]*\).*/\1/p')"
    [[ -f "$G/issue-$n" ]] && printf '%s\nclosed\n' "$(sed -n 1p "$G/issue-$n")" >"$G/issue-$n"
    exit 0 ;;
  *"issue view"*)
    n="$(printf '%s' "$a" | sed -n 's/issue view \([0-9]*\).*/\1/p')"
    st="open"; [[ -f "$G/issue-$n" ]] && st="$(sed -n 2p "$G/issue-$n")"
    if [[ "$a" == *"--json"* ]]; then echo "{\"state\":\"$st\"}"; else echo "$st"; fi
    exit 0 ;;
  *"pr create"*)
    head="$(printf '%s' "$a" | sed -n 's/.*--head \(release\/v[0-9.]*\).*/\1/p')"
    head_sha="$(/usr/bin/git -C "$PWD" rev-parse "refs/heads/$head" 2>/dev/null || echo unknown)"
    n="$(next)"; printf 'OPEN\n%s\n%s\n' "$head" "$head_sha" >"$G/pr-$n"
    echo "https://github.com/Performant-Labs/kai/pull/$n"; exit 0 ;;
  *"pr list"*)
    if [[ "$a" == *"--head"* ]]; then
      head="$(printf '%s' "$a" | sed -n 's/.*--head \(release\/v[0-9.]*\).*/\1/p')"
      for f in "$G"/pr-*; do
        [[ -f "$f" ]] || continue
        [[ "$(sed -n 2p "$f")" == "$head" ]] || continue
        if [[ "$a" == *"--json"* ]]; then
          printf '{"number":%s,"state":"%s","headRefOid":"%s"}\n' "${f##*pr-}" "$(sed -n 1p "$f")" "$(sed -n 3p "$f")"
        else
          printf '%s\t%s\n' "${f##*pr-}" "$(sed -n 1p "$f")"
        fi
      done
    fi
    exit 0 ;;
  *"pr merge"*)
    n="$(printf '%s' "$a" | sed -n 's/pr merge \([0-9]*\).*/\1/p')"
    if [[ -f "$G/pr-$n" ]]; then
      head="$(sed -n 2p "$G/pr-$n")"
      printf 'MERGED\n%s\n%s\n' "$head" "$(sed -n 3p "$G/pr-$n")" >"$G/pr-$n"
      /usr/bin/git -C "$PWD" push -q origin --delete "$head" 2>/dev/null
    fi
    exit 0 ;;
  *"pr view"*)
    n="$(printf '%s' "$a" | sed -n 's/pr view \([0-9]*\).*/\1/p')"
    f="$G/pr-$n"; [[ -f "$f" ]] || f="$(ls "$G"/pr-* 2>/dev/null | sed -n 1p)"
    if [[ -n "${f:-}" && -f "$f" && "$a" == *"comments"* ]]; then pragent "$f" json; exit 0; fi
    state="OPEN"; [[ -n "${f:-}" && -f "$f" ]] && state="$(sed -n 1p "$f")"
    sha=""; [[ -n "${f:-}" && -f "$f" ]] && sha="$(sed -n 3p "$f")"
    echo "{\"state\":\"$state\",\"headRefOid\":\"$sha\"}"
    exit 0 ;;
  *"release create"*)
    tag="$(printf '%s' "$a" | sed -n 's/.*release create \(v[0-9.]*\).*/\1/p')"
    ver="${tag#v}"
    printf '{"tag_name":"%s","isDraft":true,"assets":["Kai-%s-darwin-arm64.zip","updater-Kai-%s-darwin-arm64.zip","Kai-%s-darwin-arm64.dmg","SHA256SUMS"]}\n' \
      "$tag" "$ver" "$ver" "$ver" >"$G/release-$tag.json"
    exit 0 ;;
  *"release view"*)
    tag="$(printf '%s' "$a" | sed -n 's/.*release view \(v[0-9.]*\).*/\1/p')"
    [[ -f "$G/release-$tag.json" ]] && { cat "$G/release-$tag.json"; exit 0; }
    echo "release not found" >&2; exit 1 ;;
  *"release edit"*)
    tag="$(printf '%s' "$a" | sed -n 's/.*release edit \(v[0-9.]*\).*/\1/p')"
    if [[ "$a" == *"--draft=false"* && -f "$G/release-$tag.json" ]]; then
      jq '.isDraft = false' "$G/release-$tag.json" >"$G/t" && mv "$G/t" "$G/release-$tag.json"
    fi
    exit 0 ;;
  *"release download"*)
    d="."; [[ "$a" == *" --dir "* ]] && d="$(printf '%s' "$a" | sed -n 's/.* --dir \([^ ]*\).*/\1/p')"
    cp "$FAKE_FIXTURES"/download/* "$d/"
    exit 0 ;;
  *) echo "gh fake: unhandled: $a" >&2 >>"$FAKE_STATE_DIR/gh-unhandled.log"; exit 1 ;;
esac
EOF

chmod +x "$FB"/*

# ---------------------------------------------------------------- scenario scaffolding
R=""; SD=""; SCN=""
r_env=()
mkscn() { # name — throwaway repo + local bare origin (no network) + per-scenario fake state
  SCN="$1"; R="$tmp/$SCN"; SD="$tmp/$SCN.sd"
  mkdir -p "$R" "$SD/ghstate" "$tmp/origins"
  r_env=()
  $RG -C "$R" init -q -b master
  $RG -C "$R" config user.name t
  $RG -C "$R" config user.email t@t
  $RG init -q --bare "$tmp/origins/$SCN.git"
  $RG -C "$R" remote add origin "$tmp/origins/$SCN.git"
  mkdir -p "$R/bin/release" "$R/docs" "$R/build/darwin"
  : >"$R/bin/release/kai-release-key.backed-up"     # phase-0 backup marker (R10/A6)
  cp "$here/../docs/release-checklist.md" "$R/docs/release-checklist.md"
  printf 'bin/\n' >"$R/.gitignore"
  cat >"$R/CHANGELOG.md" <<'MD'
# Changelog

## [Unreleased]
<!-- changelog-skip: release tooling, no user-facing change: #74 -->
- A fake enhancement entry so coverage is green (#101).

### Known Issues
- [#501](https://github.com/Performant-Labs/kai/issues/501) A known issue, still open.

## [0.4.0] - 2026-01-01
- Old release.
MD
  printf 'name: kai\nversion: "0.5.0"\n' >"$R/build/config.yml"
  printf '<?xml version="1.0"?>\n<plist><dict><key>CFBundleShortVersionString</key><string>0.5.0</string><key>CFBundleVersion</key><string>0.5.0</string></dict></plist>\n' >"$R/build/darwin/Info.plist"
  $RG -C "$R" add -A
  $RG -C "$R" commit -q -m fixtures
  $RG -C "$R" push -q origin master
  $RG -C "$R" fetch -q origin
  $RG -C "$R" branch -q --set-upstream-to=origin/master master 2>/dev/null || true
}

rrun() { # release.sh args...; steering in r_env; stdin is closed (AC1: no input)
  out="$(cd "$R" && env PATH="$FB:$PATH" FAKE_STATE_DIR="$SD" FAKE_FIXTURES="$FX" \
    KAI_RELEASE_REPO=Performant-Labs/kai ${r_env[@]+"${r_env[@]}"} "$release" "$@" </dev/null 2>&1)"
  code=$?
}

state() { printf '%s\n' "$R/bin/release/v0.5.1/state.json"; }

# C01 — the script under test must exist (the RED gate: everything below fails without it).
if [[ -x "$release" ]]; then
  echo "ok   C01 scripts/release.sh exists and is executable"
else
  echo "FAIL C01 scripts/release.sh exists and is executable (feature absent: F implements it)"
  fail=1
fi

# ================================================================ AC1 + AC2: one release's life
# R2/R8: dry-run = full phase order through local, release-local.sh vX.Y.Z --dry-run, no draft;
# branch/PR/merge/tag still happen (A1). Then --publish resumes (posture escalation, R6), then
# a full-success re-run is a no-op (AC2).
mkscn ac1life
rrun v0.5.1 --dry-run
exit_is     "AC1: v0.5.1 --dry-run with no input exits 0"                       0
has         "AC1: names the remaining owner step: draft"                        'draft'
has         "AC1: names the remaining owner step: smoke"                        'smoke'
has         "AC1: names the remaining owner step: publish"                      'publish'
hasnt       "AC1: never asks a terminal question"                               '\[y/N\]'
fhas        "AC1/R10p1: preflight wrapped with the version"                     "$SD/preflight.log"   '^v0\.5\.1$'
fhas        "AC1/R10p4: bump wrapped with the PLAIN version (no v)"             "$SD/bump.log"        '^0\.5\.1$'
fhas        "AC1/R8: local wrapped as 'v0.5.1 --dry-run'"                       "$SD/local.log"       '^v0\.5\.1 --dry-run$'
exists      "AC1: packaged notes stay in bin/release/v0.5.1"                    "$R/bin/release/v0.5.1/RELEASE_NOTES.md"
exists      "AC1: the verified Kai.app stays for the smoke matrix"              "$R/bin/release/v0.5.1/Kai.app"
exists      "AC1: staged asset: the zip"                                        "$R/bin/release/v0.5.1/assets/Kai-0.5.1-darwin-arm64.zip"
exists      "AC1: staged asset: the updater zip"                                "$R/bin/release/v0.5.1/assets/updater-Kai-0.5.1-darwin-arm64.zip"
exists      "AC1: staged asset: the dmg"                                        "$R/bin/release/v0.5.1/assets/Kai-0.5.1-darwin-arm64.dmg"
exists      "AC1: staged asset: SHA256SUMS"                                     "$R/bin/release/v0.5.1/assets/SHA256SUMS"
jq_is       "AC1/R6: state.json records the posture"                            "$(state)" '.posture' 'dry-run'
jq_is       "AC1/R2: state.json records phases done through local"              "$(state)" '.phases.local.done' 'true'
fhas        "AC1/R10p0: the checklist issue is opened by exact title"           "$SD/gh.log"          'issue create.*Release checklist: v0\.5\.1'
fhas        "AC1/R10p2: known-issue candidates land in the checklist issue"     "$SD/comments.log"    '501'
fhas        "AC1/R10p1+R11: the preflight WARN gets a marker comment"           "$SD/comments.log"    'release\.sh: v0\.5\.1 preflight'
fhas        "AC1/A1: the release branch is still pushed (dry-run is the release)" "$SD/git.log"       '^push( -q)? origin release/v0\.5\.1$'
fhas        "AC1/A1: the tag is still pushed (dry-run is the release)"          "$SD/git.log"         '^push( -q)? origin v0\.5\.1$'
fhas        "AC1/R10p6: the release PR targets the fork, not upstream"          "$SD/gh.log"          'pr create.*--repo Performant-Labs/kai'
fhas        "AC1/R10p6: the release PR targets master"                          "$SD/gh.log"          'pr create.*--base master'
fhas        "AC1/R10p8: merge is --merge --delete-branch"                       "$SD/gh.log"          'pr merge.*--merge.*--delete-branch'

# AC2/R6: resume at the phase boundary into the higher posture, then the no-op re-run.
rrun v0.5.1 --publish
exit_is     "AC2: --publish resumes after the dry-run boundary and exits 0"     0
fhas        "AC2/R10p12: publish is the canonical edit command"                 "$SD/gh.log"          'release edit v0\.5\.1.*--draft=false'
fhas        "AC2/R10p13: the installer runs against the release"                "$SD/install-release.log" '^v0\.5\.1 '
fhas        "AC2/R10p14: the checklist issue is closed"                         "$SD/gh.log"          'issue close'
jq_is       "AC2/R6: the posture escalated in state"                            "$(state)" '.posture' 'publish'
jq_is       "AC2: closeout recorded done"                                       "$(state)" '.phases.closeout.done' 'true'
rrun v0.5.1 --publish
exit_is     "AC2: re-running after full success is a no-op exit 0"              0
match_is    "AC2: no second checklist issue"                                    "$SD/gh.log" 'issue create' 1
match_is    "AC2: no second release PR"                                         "$SD/gh.log" 'pr create' 1
match_is    "AC2: no second publish"                                            "$SD/gh.log" 'release edit' 1
dups="$(awk '/^=== comment/{getline; print}' "$SD/comments.log" 2>/dev/null | sort | uniq -d)"
if [[ -z "$dups" ]]; then echo "ok   AC2/R11: no duplicate comments across kill/resume"; else echo "FAIL AC2/R11: duplicate comments: $dups"; fail=1; fi
badm="$(awk '/^=== comment/{getline; if ($0 !~ /^<!-- release\.sh: v/) print}' "$SD/comments.log" 2>/dev/null)"
if [[ -z "$badm" ]]; then echo "ok   AC2/R11: every comment starts with the release.sh marker"; else echo "FAIL AC2/R11: unmarked comment: $badm"; fail=1; fi

# ================================================================ AC4: the default posture
# R8: default = release-local.sh vX.Y.Z --yes, draft created (non-public), stop after the draft
# (+ smoke gate), printing the single --publish resume command. R1's boundary rides along: the
# fake PR-Agent state serves Score 90 / all RESOLVED / complete@head, and review passes.
mkscn default
rrun v0.5.1
exit_is     "AC4/R1: score exactly 90, all findings RESOLVED, passes review"    0
fhas        "AC4/R8: default posture passes --yes to release-local.sh"          "$SD/local.log"       '^v0\.5\.1 --yes$'
jq_is       "AC4/R8: the draft exists"                                          "$SD/ghstate/release-v0.5.1.json" '.isDraft' 'true'
jq_is       "AC4/R10p11: smoke recorded skipped without --smoke"                "$(state)" '.smoke' 'skipped'
jq_is       "AC4/R10p11: the smoke phase itself is done (as skipped)"           "$(state)" '.phases.smoke.done' 'true'
jq_is       "AC4/R8: publish NOT done in the default posture"                   "$(state)" '.phases.publish.done' 'false'
fhas        "AC4/R10p11: smoke-skipped is recorded in the checklist issue"      "$SD/comments.log"    'skipped'
has         "AC4/R8: prints the single --publish resume command"                '--publish'
rrun v0.5.1 --publish
exit_is     "AC4: --publish continues from the draft and completes"             0
jq_is       "AC4: published (not draft)"                                        "$SD/ghstate/release-v0.5.1.json" '.isDraft' 'false'
jq_is       "AC4: publish phase recorded done"                                  "$(state)" '.phases.publish.done' 'true'
rrun v0.5.1 --dry-run
exit_nonzero "AC4/R6: a lower posture than recorded is refused"                 
has         "AC4/R6: the refusal names the posture conflict"                    'posture|dry-run'

# AC4/R10p11: --smoke stops awaiting the owner's matrix; --smoke-done records it.
mkscn smoke
rrun v0.5.1 --smoke
exit_is     "AC4: --smoke exits 0 awaiting the owner"                           0
jq_is       "AC4: awaiting smoke is not 'skipped'"                              "$(state)" '.smoke == "skipped"' 'false'
fhas        "AC4/R10p11: a smoke-phase comment is posted"                       "$SD/comments.log"    'release\.sh: v0\.5\.1 smoke'
rrun v0.5.1 --publish --smoke-done
exit_is     "AC4: --smoke-done resumes and completes"                           0
jq_is       "AC4/R10p11: smoke done is recorded owner-asserted"                 "$(state)" '.smoke | startswith("done (owner-asserted")' 'true'

# ================================================================ AC3: unknown failures and the AI
# R3/R4: one consultation per unknown-failure event; the executor re-runs the phase on retry.
mkscn retry
r_env=(FAKE_BUMP_FAILS=1 FAKE_AI=retry)
rrun v0.5.1
exit_is     "AC3: unknown failure + allowlisted retry recovers"                 0
val_is      "AC3: the phase command ran exactly twice (fail, then retry)"       "$SD/bump.count" '2'
count_is    "AC3/R3: exactly ONE consultation for the failure"                  "$SD/askai.log" 1
jq_is       "AC3/R3: the consultation is recorded in state"                     "$(state)" '.consultations | length' '1'
fhas        "AC3: the consultation names its phase"                             "$SD/askai.log" 'bump'
exists      "AC3: the run still reaches the draft"                              "$SD/ghstate/release-v0.5.1.json"

# R3: a second consultation is allowed only after the first answer was executed and failed again.
mkscn aitwo
r_env=(FAKE_BUMP_FAILS=2 FAKE_AI=retry)
rrun v0.5.1
exit_is     "AC3/R3: second failure after an executed retry may consult again"  0
val_is      "AC3/R3: three bump runs (fail, retry-fail, retry)"                 "$SD/bump.count" '3'
count_is    "AC3/R3: exactly two consultations"                                 "$SD/askai.log" 2
jq_is       "AC3/R3: cap not reached"                                           "$(state)" '.consultations | length' '2'

# R3: a third failure in the same phase NEVER consults again — straight to stop.
mkscn aicap
r_env=(FAKE_BUMP_FAILS=3 FAKE_AI=retry)
rrun v0.5.1
exit_nonzero "AC3/R3: third failure stops (no third consultation)"
val_is      "AC3/R3: the phase command ran three times only"                    "$SD/bump.count" '3'
count_is    "AC3/R3: consultations capped at 2 per phase"                       "$SD/askai.log" 2
jq_is       "AC3/R3: state records exactly two consultations"                   "$(state)" '.consultations | length' '2'
has         "AC3/brief7: the stop prints a resume command"                      'resume'

# AC3: an out-of-allowlist answer is refused; the executor never runs it.
mkscn aibad
r_env=(FAKE_BUMP_FAILS=1 FAKE_AI=bad)
rrun v0.5.1
exit_nonzero "AC3: out-of-allowlist action takes the stop path"
val_is      "AC3: the executor did NOT re-run the phase"                        "$SD/bump.count" '1'
count_is    "AC3: the AI was consulted once"                                    "$SD/askai.log" 1
has         "AC3: the refusal says so"                                          'refus|allowlist'

# AC3/R9: no answer (missing/failing AI) is fail-closed: stop, no retry.
mkscn ainone
r_env=(FAKE_BUMP_FAILS=1)
rrun v0.5.1
exit_nonzero "AC3/R9: no answer available takes the stop path (fail-closed)"
val_is      "AC3/R9: nothing was retried without an allowlisted answer"         "$SD/bump.count" '1'
count_is    "AC3/R9: the consultation was attempted (and produced nothing)"     "$SD/askai.log" 1
has         "AC3/brief7: the stop prints a resume command"                      'resume'

# ================================================================ AC2: state is a cache
# R6: absent state is fine — phases re-derive from postconditions; nothing public is redone.
mkscn nostate
rrun v0.5.1
exit_is     "AC2: default posture completes first"                              0
rm -f "$R/bin/release/v0.5.1/state.json"
rrun v0.5.1 --publish
exit_is     "AC2/R6: absent state re-derives from postconditions"               0
match_is    "AC2/R6: re-derivation opens no second PR"                          "$SD/gh.log" 'pr create' 1
match_is    "AC2/R6: re-derivation publishes once"                              "$SD/gh.log" 'release edit' 1
jq_is       "AC2/R6: the rebuilt state reaches closeout"                        "$(state)" '.phases.closeout.done' 'true'

# ================================================================ AC3: the four known blockers
# (1) Kai running at phase 10: handler quit-kai is dispatchable ONLY with --quit-kai (R10p10).
mkscn kai
echo kai >"$SD/local-fail"
rrun v0.5.1
exit_nonzero "AC3: Kai running without --quit-kai stops"
count_is    "AC3: the known blocker never consults the AI"                      "$SD/askai.log" 0
count_is    "AC3: local ran once, no blind retry"                               "$SD/local.log" 1
has         "AC3: the stop tells the owner about --quit-kai"                    '--quit-kai'

mkscn kaiquit
echo kai >"$SD/local-fail"
rrun v0.5.1 --quit-kai
exit_is     "AC3: --quit-kai dispatches the static handler and proceeds"        0
count_is    "AC3: the known blocker never consults the AI"                      "$SD/askai.log" 0
count_is    "AC3: local re-ran after the quit"                                  "$SD/local.log" 2
if { [[ -f "$SD/osascript.log" ]] && grep -qi 'quit' "$SD/osascript.log"; } \
   || { [[ -f "$SD/pkill.log" ]] && grep -q 'Kai' "$SD/pkill.log"; }; then
  echo "ok   AC3: Kai is actually quit (osascript or pkill)"
else
  echo "FAIL AC3: Kai is actually quit (osascript or pkill)"; fail=1
fi
exists      "AC3: the run reaches the draft"                                    "$SD/ghstate/release-v0.5.1.json"

# (2) Leftover merged release/vX.Y.Z: the #74 rule — skip phases 3-6, delete the branch (R10p3).
mkscn leftover
$RG -C "$R" checkout -q -b release/v0.5.1
$RG -C "$R" push -q origin release/v0.5.1
$RG -C "$R" checkout -q master
printf '101\n' >"$SD/ghstate/seq"
printf 'MERGED\nrelease/v0.5.1\n%s\n' "$($RG -C "$R" rev-parse release/v0.5.1)" >"$SD/ghstate/pr-101"
rrun v0.5.1
exit_is     "AC3/#74: merged leftover branch is a group-skip, not a stop"       0
count_is    "AC3/#74: phases 3-6 are skipped (no bump)"                         "$SD/bump.log" 0
fhas        "AC3/#74: the leftover branch is deleted via gh api -X DELETE"      "$SD/gh.log" 'api -X DELETE.*git/refs/heads/release/v0\.5\.1'
count_is    "AC3/#74: the static handler never consults the AI"                 "$SD/askai.log" 0
exists      "AC3/#74: the run still reaches the draft"                          "$SD/ghstate/release-v0.5.1.json"

# (3) Bindings drift at phase 10: handler regen-bindings = make bindings + commit (R10p10, #73).
mkscn bindings
echo bindings >"$SD/local-fail"
rrun v0.5.1
exit_is     "AC3/#73: bindings drift is handled by the static handler"          0
fhas        "AC3/#73: make bindings runs"                                       "$SD/make.log" '^bindings$'
fhas        "AC3/#73: the regenerated bindings are committed"                   "$SD/git.log" 'frontend/bindings'
count_is    "AC3/#73: the static handler never consults the AI"                 "$SD/askai.log" 0
count_is    "AC3/#73: local re-ran after the regen"                             "$SD/local.log" 2
exists      "AC3/#73: the run still reaches the draft"                          "$SD/ghstate/release-v0.5.1.json"

# (4) Missing signing identity: stop, release-cert.sh hint, no AI (brief item 4; R5 stop 1).
mkscn identity
: >"$SD/codesign-fail"
rrun v0.5.1
exit_nonzero "AC3: missing identity stops"
has         "AC3/R5-1: the stop names release-cert.sh"                          'release-cert\.sh'
count_is    "AC3: a hard stop never consults the AI"                            "$SD/askai.log" 0
count_is    "AC3: nothing runs after phase 0 fails"                             "$SD/preflight.log" 0

# ================================================================ R10 phase 2: stop-for-owner
# Uncurated Known Issues stop [S]; the printed skip line is A10's contract; resume on the marker.
mkscn chgstop
cat >"$R/CHANGELOG.md" <<'MD'
# Changelog

## [Unreleased]
<!-- changelog-skip: release tooling, no user-facing change: #74 -->
- A fake enhancement entry so coverage is green (#101).

## [0.4.0] - 2026-01-01
- Old release.
MD
$RG -C "$R" commit -qam uncurated || true
rrun v0.5.1
exit_nonzero "R10p2: uncurated Known Issues stop for the owner"
has         "R10p2: the stop names the Known Issues gap"                        'Known Issues'
has         "A10: the stop prints the changelog-skip line to add"               'changelog-skip'
count_is    "R10p2: a stop-for-owner never consults the AI"                     "$SD/askai.log" 0
printf '<!-- known-issues: none 2026-10-19 -->\n' >>"$R/CHANGELOG.md"
rrun v0.5.1
exit_is     "R10p2: the none-marker unblocks the phase"                         0
exists      "R10p2: the run then reaches the draft"                             "$SD/ghstate/release-v0.5.1.json"

# Uncovered PRs from changelog-check.sh --strict also stop [S].
mkscn chgstrict
: >"$SD/changelog-strict-fail"
rrun v0.5.1
exit_nonzero "R10p2: uncovered PRs stop for the owner"
has         "R10p2: the uncovered PR is named"                                  '#501'
has         "A10: the stop prints the changelog-skip line to add"               'changelog-skip'

# ================================================================ R6: lock and corrupt state
mkscn locklive
mkdir -p "$R/bin/release/v0.5.1/.lock"
echo $$ >"$R/bin/release/v0.5.1/.lock/pid"
rrun v0.5.1
exit_nonzero "R6: a live lock refuses a second invocation"
has         "R6: the refusal names the running release.sh and PID"              'another release\.sh is running'
has         "R6: the refusal shows the live PID"                                "PID $$"

mkscn lockdead
mkdir -p "$R/bin/release/v0.5.1/.lock"
( /bin/sleep 0.2 ) & deadpid=$!; wait "$deadpid" 2>/dev/null
echo "$deadpid" >"$R/bin/release/v0.5.1/.lock/pid"
rrun v0.5.1
exit_is     "R6: a dead owner's lock is removed and retaken"                    0
hasnt       "R6: no false lock refusal"                                         'another release\.sh is running'

mkscn corrupt
mkdir -p "$R/bin/release/v0.5.1"
printf 'this is { not json\n' >"$R/bin/release/v0.5.1/state.json"
rrun v0.5.1
exit_nonzero "R6: unparseable state stops"
has         "R6: the stop prints the state.json path"                           'state\.json'
has         "R6: the stop says how to rebuild"                                  'delete it to rebuild from postconditions'

# ================================================================ AC6/R5: hard stops, no AI
# (2) tag or release already exists — draft and published variants. Never delete/overwrite.
mkscn hsdraft
echo exists-draft >"$SD/local-fail"
rrun v0.5.1
exit_nonzero "AC6/R5-2: an existing DRAFT release is a hard stop"
has         "AC6/R5-2: the existing release is named"                           'already exists'
count_is    "AC6/R5-2: hard stops never consult the AI"                         "$SD/askai.log" 0
fhasnt      "AC6: never deletes a release"                                      "$SD/gh.log" 'release delete'
fhasnt      "AC6: never edits the existing release"                             "$SD/gh.log" 'release edit'

mkscn hspub
echo exists-published >"$SD/local-fail"
rrun v0.5.1
exit_nonzero "AC6/R5-2: an existing PUBLISHED release is a hard stop"
has         "AC6/R5-2: the existing release is named"                           'already exists'
count_is    "AC6/R5-2: hard stops never consult the AI"                         "$SD/askai.log" 0
fhasnt      "AC6: never deletes a release"                                      "$SD/gh.log" 'release delete'
fhasnt      "AC6: never edits the existing release"                             "$SD/gh.log" 'release edit'

# (3) CI red on the release commit (phase 7 review gate, R10p7 [H]).
mkscn hsci
r_env=(FAKE_CI=failed)
rrun v0.5.1
exit_nonzero "AC6/R5-3: red CI on the release commit is a hard stop"
count_is    "AC6/R5-3: hard stops never consult the AI"                         "$SD/askai.log" 0
count_is    "AC6/R5-3: nothing is built after red CI"                           "$SD/local.log" 0

# (4) PR-Agent: score 88 (below the 90 boundary) and score 95 with one OPEN finding (R1).
mkscn hsagent88
r_env=(FAKE_AGENT=88)
rrun v0.5.1
exit_nonzero "AC6/R1: PR-Agent score 88 is a hard stop"
count_is    "AC6/R1: hard stops never consult the AI"                           "$SD/askai.log" 0
has         "AC6/R1: the stop names the review gate"                            'score|PR-Agent|finding|90'

mkscn hsagent95o
r_env=(FAKE_AGENT=95o)
rrun v0.5.1
exit_nonzero "AC6/R1: an unresolved finding stops even at score 95"
count_is    "AC6/R1: hard stops never consult the AI"                           "$SD/askai.log" 0
has         "AC6/R1: the stop names the unresolved finding"                     'unresolved|OPEN|finding'

# Phase-0 [H]: no gh push rights.
mkscn nopush
: >"$SD/ghstate/no-push"
rrun v0.5.1
exit_nonzero "R10p0: no push rights is a hard stop"
has         "R10p0: the stop names push rights"                                 'push'
count_is    "R10p0: hard stops never consult the AI"                            "$SD/askai.log" 0

# R5's shared refusal loop: each of the five hard stops refuses EVEN with an AI answer
# saying retry — the hard-stop check precedes consultation (R4), so the AI is never asked.
refusal() { # one sub-case; steering already set (markers + r_env incl. FAKE_AI=retry)
  rrun v0.5.1
  exit_nonzero "R5-loop $SCN: hard stop stands even with an AI that would say retry"
  count_is    "R5-loop $SCN: the AI is never consulted for a hard stop"         "$SD/askai.log" 0
}
mkscn loopidentity; : >"$SD/codesign-fail"; r_env=(FAKE_AI=retry); refusal
count_is "R5-loop identity: stops before any phase work"                        "$SD/preflight.log" 0
mkscn loopexists; echo exists-published >"$SD/local-fail"; r_env=(FAKE_AI=retry); refusal
count_is "R5-loop exists: local is not retried"                                 "$SD/local.log" 1
mkscn loopci; r_env=(FAKE_AI=retry FAKE_CI=failed); refusal
count_is "R5-loop ci: nothing is built"                                         "$SD/local.log" 0
mkscn loopagent88; r_env=(FAKE_AI=retry FAKE_AGENT=88); refusal
count_is "R5-loop agent88: nothing is built"                                    "$SD/local.log" 0
mkscn loopagent95o; r_env=(FAKE_AI=retry FAKE_AGENT=95o); refusal
count_is "R5-loop agent95o: nothing is built"                                   "$SD/local.log" 0

# ================================================================ AC7: docs and CHANGELOG
fhas "AC7: docs/releasing.md points at scripts/release.sh as the operator"      "$here/../docs/releasing.md" 'scripts/release\.sh'
fhas "AC7: docs/release-checklist.md points at scripts/release.sh"              "$here/../docs/release-checklist.md" 'scripts/release\.sh'
fhas "AC7: CHANGELOG [Unreleased] carries the #78 skip line"                    "$here/../CHANGELOG.md" 'changelog-skip: release tooling, no user-facing change: #78'

# ================================================================ R5(5): published-tag safety
# Negative assertion over EVERY git invocation this harness has recorded: no tag deletion, no
# force push, no non-fast-forward tag write anywhere (the leftover handler deletes heads only).
: >"$tmp/all-git.log"
for f in "$tmp"/*.sd/git.log; do [[ -f "$f" ]] && cat "$f" >>"$tmp/all-git.log"; done
fhasnt "AC6/R5-5: no tag is ever deleted"          "$tmp/all-git.log" 'tag (-d|--delete)'
fhasnt "AC6/R5-5: no force push is ever issued"    "$tmp/all-git.log" 'push.*(-f |--force |\+refs/tags)'

[[ $fail -eq 0 ]] && echo "PASS" || { echo "FAILED"; exit 1; }
