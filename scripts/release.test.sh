#!/usr/bin/env bash
# Tests for scripts/release.sh: the static release workflow (issue #78), in throwaway repos
# with fake wrapped scripts first on PATH, no network. Harness shape follows
# scripts/release-preflight.test.sh (mkrepo/run/has + fake gh + a fake git wrapping the real
# one, origin = a local bare repo). The contract under test is docs/handoffs/0078-issue-78/
# handoff-A.md (rulings R1-R12) as it amends docs/handoffs/0078-issue-78/brief.md (AC1-AC7).
# Part of test.unit.command (issue #78, brief item 9).
# PR #100 round 5: the fake gh is STATE-REALISTIC — pr list defaults to OPEN and honors
# --state exactly like the CLI; pr view reports a MERGED PR's mergeCommit, and that commit
# is landed in ORIGIN only (fake gh pr merge / seed_merged_pr below), so the local clone
# cannot resolve it until a git fetch installs it, exactly like a GitHub-side merge.
# PR #100 round 6: the fake release-check-version.sh is REALISTIC — it execs the REAL
# script against the fake tree, and the fake release-bump.sh edits both version surfaces
# the real one edits (finding e989fe9ce3ec ACTIVE, release.sh:411-418). The always-exit-0
# check fake made ph_restructure's postcondition probe vacuous: restructure_apply never
# ran in tests, no test CHANGELOG was ever restructured, and the stateloss spurious stop
# in ph_changelog was invisible.
# Round 7 (the PERFECT round): (A) the --dry-run + --smoke/--smoke-done posture
# contradiction must be refused up front (drystop); (B) carried items pinned — per-handler
# blocker retry counters (handlercounts; the shared counter was the defect), the
# release-ask-ai.sh checklist grep must treat the command description as a FIXED string
# (escapere, in release-ask-ai.test.sh), and EVERY ph_verify stop path trap-cleans its
# workdir and --dest dir (verifytrapall legs); (C) harness hygiene — fail-by-default
# recording stubs (opencode/curl/codesign exit 127 "UNEXPECTED REAL <tool>"), the fake
# git hardened (local object ops pass, network subverbs only to the scenario's local
# origin), a stubtrip self-test and a norealbins finale; (D) class sweeps — the flag
# grid and per-group state-loss rows; (E) a b32 case runs the whole flow under /bin/bash
# 3.2 (the shebang is env-bash; stock macOS resolves it to 3.2).
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
# Round 7 C1, hardened fake git: LOCAL object operations pass through to the real git;
# any network-touching subverb (push/fetch/pull/ls-remote/clone/submodule) is refused
# with 127 "UNEXPECTED REAL git" unless this case steers it — either the argv names the
# scenario's local bare origin (\$FAKE_ORIGIN, set/exported by mkscn) or the resolved
# remote IS that origin or FAKE_GIT_ALLOW_NET=1. Every trip is recorded in
# \$FAKE_STATE_DIR/unexpected.log for the norealbins finale; the real network is never
# reached (the refusal fires before the exec).
net=""
for a in "\$@"; do
  case "\$a" in push|fetch|pull|ls-remote|clone|submodule) net="\$a"; break ;; esac
done
if [[ -n "\$net" ]]; then
  ok=0
  [[ "\${FAKE_GIT_ALLOW_NET:-}" == "1" ]] && ok=1
  if [[ \$ok -eq 0 && -n "\${FAKE_ORIGIN:-}" ]]; then
    [[ "\$*" == *"\$FAKE_ORIGIN"* ]] && ok=1
    if [[ \$ok -eq 0 ]]; then
      u="\$($RG remote get-url origin 2>/dev/null || true)"
      [[ "\$u" == "\$FAKE_ORIGIN" ]] && ok=1
    fi
  fi
  if [[ \$ok -eq 0 ]]; then
    printf 'UNEXPECTED REAL git (network %s): %s\n' "\$net" "\$*" >&2
    printf 'UNEXPECTED REAL git (network %s): %s\n' "\$net" "\$*" >>"\$FAKE_STATE_DIR/unexpected.log"
    exit 127
  fi
fi
# PR100-R4b: an env-steered FAILING 'git push ... release/vX.Y.Z'. The same argv serves
# ph_branch (occurrence 1) and release_pr_apply (occurrence 2+), so the steer is by
# occurrence: FAKE_PUSH_FAIL_AT=N fails the Nth and every later release-branch push.
if [[ -n "\${FAKE_PUSH_FAIL_AT:-}" && "\$1" == "push" && "\$*" == *"release/v"* ]]; then
  n=0; [[ -f "\$FAKE_STATE_DIR/pushcount" ]] && n="\$(cat "\$FAKE_STATE_DIR/pushcount")"
  n=\$((n + 1)); echo "\$n" >"\$FAKE_STATE_DIR/pushcount"
  if [[ "\$n" -ge "\$FAKE_PUSH_FAIL_AT" ]]; then
    echo "error: (fake) push of the release branch refused (occurrence \$n)" >&2
    exit 1
  fi
fi
exec $RG "\$@"
EOF

# Round 7 C1: tools NO scenario ever steers get a fail-by-default RECORDING stub: argv
# recorded to unexpected.log, "UNEXPECTED REAL <tool>" on stderr, exit 127. If release.sh
# (or any wrapped script dispatched under this PATH) ever reaches for the real opencode,
# curl or codesign, the phase fails loudly and the norealbins finale names the trip. The
# steered fakes above (gh, git, hdiutil, osascript, pgrep, pkill, make, ...) never fall
# through to a real binary either — their unhandled shapes exit 1 with "unhandled".
for t in opencode curl codesign; do
  cat >"$FB/$t" <<EOF
#!/usr/bin/env bash
printf '%s %s\n' "$t" "\$*" >>"\$FAKE_STATE_DIR/unexpected.log"
printf 'UNEXPECTED REAL $t: %s\n' "\$*" >&2
exit 127
EOF
done

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
  rm -f "$FAKE_STATE_DIR/local-fail" "$FAKE_STATE_DIR/bump-drift"   # drift is fixed once regenerated
fi
exit 0
EOF

cat >"$FB/hdiutil" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/hdiutil.log"
case "${1:-}" in
  attach) # Round 7 B3 steers: mount-fail = the attach itself fails (no mount point);
          # dmg-diff = the image mounts but its Kai.app differs from the zip's.
          if [[ -e "$FAKE_STATE_DIR/mount-fail" ]]; then
            echo "hdiutil: (fake) attach failed" >&2; exit 1
          fi
          mkdir -p "$FAKE_STATE_DIR/mnt/Kai.app"
          cp -R "$FAKE_FIXTURES/Kai.app/." "$FAKE_STATE_DIR/mnt/Kai.app/"
          [[ -e "$FAKE_STATE_DIR/dmg-diff" ]] && echo different >"$FAKE_STATE_DIR/mnt/Kai.app/Info.plist"
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
# Round 7 B1 (handlercounts): an explicit per-invocation failure sequence. bump.seq holds
# one space-separated element per invocation; each invocation consumes the first element.
#   kai      — set kai-running and fail with the Kai-is-running signature (the quit
#              handler clears the marker, i.e. the OS effect of quitting Kai)
#   bindings — fail with the frontend/bindings drift signature. `make bindings` does NOT
#              consume the sequence (unlike the legacy bump-drift marker), so persistent
#              drift is steered by repeating the element.
# An `ok` element (or an exhausted sequence) falls through to the legacy behavior below.
if [[ -f "$FAKE_STATE_DIR/bump.seq" ]]; then
  seq_all="$(cat "$FAKE_STATE_DIR/bump.seq")"
  el="${seq_all%% *}"; rest="${seq_all#* }"
  [[ "$rest" == "$seq_all" ]] && rest=""
  printf '%s\n' "$rest" >"$FAKE_STATE_DIR/bump.seq"
  case "$el" in
    kai)      : >"$FAKE_STATE_DIR/kai-running"
              echo "FAIL: Kai is running: quit it (release-verify.sh launches its own copy)" >&2; exit 1 ;;
    bindings) echo "FAIL: the tree is not the tag: frontend/bindings differs (run make bindings and commit the result, #73)" >&2; exit 1 ;;
  esac
fi
# PR100-R4a premerge: while this marker stands, the bump log carries the bindings-drift
# signature (the fake make clears it, like local-fail — one regen fixes the drift).
if [[ -e "$FAKE_STATE_DIR/bump-drift" ]]; then
  echo "FAIL: frontend/bindings differs from the committed files (run make bindings and commit the result, #73)" >&2
  exit 1
fi
if [[ "$n" -le "${FAKE_BUMP_FAILS:-0}" ]]; then
  echo "FAIL: bump exploded (fake, invocation $n)" >&2; exit 1
fi
# PR100-R6 realism: the real bump edits BOTH version surfaces — every build/config.yml
# `version:` line AND build/darwin/Info.plist's CFBundleShortVersionString +
# CFBundleVersion (release-bump.sh's own four lines). The realistic
# release-check-version.sh fake verifies exactly those, so the fake bump must set them
# or the restructure postcondition could never hold after a bump.
[[ -f build/config.yml ]] && sed -i.bak -E "s/version: \"[^\"]*\"/version: \"$1\"/" build/config.yml && rm -f build/config.yml.bak
if [[ -f build/darwin/Info.plist ]]; then
  v="$1" perl -0pi -e 's{(<key>CFBundleShortVersionString</key>\s*<string>)[^<]*(</string>)}{${1}$ENV{v}${2}}; s{(<key>CFBundleVersion</key>\s*<string>)[^<]*(</string>)}{${1}$ENV{v}${2}}' build/darwin/Info.plist
fi
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

for s in release-notes.sh release-tree-check.sh release-verify.sh \
         release-package.sh release-gates.sh sign-check.sh release-cert.sh; do
  cat >"$FB/$s" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >>"\$FAKE_STATE_DIR/$s.log"
echo "ok (fake $s)"
exit 0
EOF
done

# PR #100 round 6: release-check-version.sh gets NO generic always-0 fake — it execs the
# REAL script against the fake tree (finding e989fe9ce3ec ACTIVE, release.sh:411-418).
# The real one verifies every build/config.yml `version:` line, both build/darwin/Info.plist
# CFBundle keys and the CHANGELOG's `## [X.Y.Z]` section, so ph_restructure's postcondition
# probe can genuinely FAIL (pre-restructure) and genuinely HOLD (post-restructure): the
# restructure postcondition path — restructure_apply and its re-check — is exercised for
# real. The old fake exited 0 unconditionally, the probe held before any work, no test
# CHANGELOG was ever restructured, and the stateloss spurious stop stayed hidden.
cat >"$FB/release-check-version.sh" <<EOF
#!/usr/bin/env bash
printf '%s\n' "\$*" >>"\$FAKE_STATE_DIR/release-check-version.sh.log"
exec "$here/release-check-version.sh" "\$@"
EOF

# release-verify.sh gets two steering knobs beyond the generic fake (PR #100 F2, R7 B3):
# the verify-term marker kills the parent release.sh mid-phase-13, while its temp workdir
# is live, to prove the workdir cleanup is trap-based (an end-of-function rm cannot run
# then); the verify-fail marker makes the check itself fail (a plain stop path).
cat >"$FB/release-verify.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/release-verify.sh.log"
if [[ -e "$FAKE_STATE_DIR/verify-term" ]]; then
  kill -TERM "$PPID" 2>/dev/null || true
  sleep 1
fi
if [[ -e "$FAKE_STATE_DIR/verify-fail" ]]; then
  echo "FAIL: release-verify exploded (fake)" >&2; exit 1
fi
echo "ok (fake release-verify.sh)"
exit 0
EOF

cat >"$FB/install-release.sh" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/install-release.log"
if [[ -e "$FAKE_STATE_DIR/install-fail" ]]; then
  echo "FAIL: install-release exploded (fake)" >&2; exit 1
fi
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
  local f="$1" head_sha score findings body rat
  case "${FAKE_AGENT:-90}" in
    nostate-*) # visible review only (no machine block), with timestamps like gh pr view --json comments,commits
      score=95; body='No major issues detected'; rat='2026-10-20T23:07:33Z'
      case "$FAKE_AGENT" in
        nostate-stale)    rat='2026-10-20T23:00:00Z' ;;
        nostate-findings) body='Possible Issue: something' ;;
        nostate-low)      score=80 ;;
      esac
      printf '<td>🏅&nbsp;<strong>Score</strong>: %s</td>\n<!-- pr-agent:review:full -->\n%s' "$score" "$body" \
        | jq -Rs --arg r "$rat" '{comments:[{body: ., createdAt: $r}], commits:[{committedDate:"2026-10-20T23:06:30Z"}]}'
      return 0 ;;
  esac
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
      # a stale failed run followed by a passing re-run of the same check (latest run wins)
      rerunok)  printf 'pr-title\tcompleted\tfailure\nci / go / build-lint-test\tcompleted\tsuccess\npr-title\tcompleted\tsuccess\n' ;;
      rerunbad) printf 'pr-title\tcompleted\tsuccess\npr-title\tcompleted\tfailure\n' ;;
      # nothing registered on the first read, then everything green (checks appear late)
      late)     c="$G/ci-late"; n="$(cat "$c" 2>/dev/null || echo 0)"; echo $((n+1)) >"$c"
                [[ "$n" -ge 1 ]] && printf 'ci / go / build-lint-test\tcompleted\tsuccess\n' ;;
      # a check that never finishes
      running) printf 'ci / go / build-lint-test\tin_progress\t\n' ;;
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
    # PR100-R4b: a counter knob, FAKE_PRCREATE_FAILS=N, mirroring FAKE_BUMP_FAILS — the
    # first N invocations fail (transient when N=1, persistent when N is large).
    n=0; [[ -f "$FAKE_STATE_DIR/prcreate.count" ]] && n="$(cat "$FAKE_STATE_DIR/prcreate.count")"
    n=$((n + 1)); echo "$n" >"$FAKE_STATE_DIR/prcreate.count"
    if [[ -n "${FAKE_PRCREATE_FAILS:-}" && "$n" -le "$FAKE_PRCREATE_FAILS" ]]; then
      echo "gh: (fake) pr create failed (invocation $n)" >&2; exit 1
    fi
    head="$(printf '%s' "$a" | sed -n 's/.*--head \(release\/v[0-9.]*\).*/\1/p')"
    head_sha="$(/usr/bin/git -C "$PWD" rev-parse "refs/heads/$head" 2>/dev/null || echo unknown)"
    n="$(next)"; printf 'OPEN\n%s\n%s\n' "$head" "$head_sha" >"$G/pr-$n"
    echo "https://github.com/Performant-Labs/kai/pull/$n"; exit 0 ;;
  *"pr list"*)
    if [[ "$a" == *"--head"* ]]; then
      head="$(printf '%s' "$a" | sed -n 's/.*--head \(release\/v[0-9.]*\).*/\1/p')"
      # PR #100 R5a realism: the real CLI lists only OPEN PRs by default; --state
      # all|merged|open|closed selects exactly (a state-blind list hid find_pr's defect).
      aa="${a//--state=/--state }"
      st="open"
      case "$aa" in
        *"--state all"*)    st="all" ;;
        *"--state merged"*) st="merged" ;;
        *"--state open"*)   st="open" ;;
        *"--state closed"*) st="closed" ;;
      esac
      for f in "$G"/pr-*; do
        [[ -f "$f" ]] || continue
        [[ "$(sed -n 2p "$f")" == "$head" ]] || continue
        case "$st:$(sed -n 1p "$f")" in
          all:*|open:OPEN|merged:MERGED|closed:CLOSED|closed:MERGED) ;;
          *) continue ;;
        esac
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
      head="$(sed -n 2p "$G/pr-$n")"; hsha="$(sed -n 3p "$G/pr-$n")"
      # PR #100 R5b realism: GitHub merges server-side — the merge commit lands on the
      # BASE branch of ORIGIN only; the caller's clone does not have it until it fetches.
      # pr-<N> line 4 carries its oid (what pr view reports as mergeCommit.oid).
      org="$(/usr/bin/git -C "$PWD" remote get-url origin 2>/dev/null || true)"
      base=""; msha=""
      [[ -n "$org" && -d "$org" ]] && base="$(/usr/bin/git --git-dir="$org" rev-parse refs/heads/master 2>/dev/null || true)"
      if [[ -n "$base" && -n "$hsha" && "$hsha" != "unknown" ]]; then
        /usr/bin/git -C "$PWD" push -q "$org" "$hsha":refs/heads/__gh-merge 2>/dev/null || true
        /usr/bin/git --git-dir="$org" update-ref -d refs/heads/__gh-merge 2>/dev/null || true
        msha="$(GIT_AUTHOR_NAME=github GIT_AUTHOR_EMAIL=noreply@github.com \
                GIT_COMMITTER_NAME=github GIT_COMMITTER_EMAIL=noreply@github.com \
                /usr/bin/git --git-dir="$org" commit-tree "$base^{tree}" -p "$base" -p "$hsha" \
                -m "Merge pull request #$n from Performant-Labs/${head//\//-}" 2>/dev/null || true)"
        [[ -n "$msha" ]] && /usr/bin/git --git-dir="$org" update-ref refs/heads/master "$msha"
      fi
      printf 'MERGED\n%s\n%s\n%s\n' "$head" "$hsha" "$msha" >"$G/pr-$n"
      /usr/bin/git -C "$PWD" push -q origin --delete "$head" 2>/dev/null
    fi
    exit 0 ;;
  *"pr view"*)
    n="$(printf '%s' "$a" | sed -n 's/pr view \([0-9]*\).*/\1/p')"
    f="$G/pr-$n"; [[ -f "$f" ]] || f="$(ls "$G"/pr-* 2>/dev/null | sed -n 1p)"
    if [[ -n "${f:-}" && -f "$f" && "$a" == *"comments"* ]]; then pragent "$f" json; exit 0; fi
    state="OPEN"; [[ -n "${f:-}" && -f "$f" ]] && state="$(sed -n 1p "$f")"
    sha=""; [[ -n "${f:-}" && -f "$f" ]] && sha="$(sed -n 3p "$f")"
    msha=""; [[ -n "${f:-}" && -f "$f" ]] && msha="$(sed -n 4p "$f")"
    # PR #100 R5 realism: real gh returns exactly the requested fields; a MERGED PR
    # carries its mergeCommit (an oid the local clone lacks until fetch — R5b), an
    # unmerged one reports null.
    fields="$(printf '%s' "$a" | sed -n 's/.*--json \([^ ]*\).*/\1/p' | tr ',' ' ')"
    [[ -n "$fields" ]] || fields="state headRefOid"
    j=""
    for fld in $fields; do
      case "$fld" in
        state)      j="${j:+$j,}\"state\":\"$state\"" ;;
        headRefOid) j="${j:+$j,}\"headRefOid\":\"$sha\"" ;;
        mergeCommit)
          if [[ "$state" == "MERGED" && -n "$msha" ]]; then
            j="${j:+$j,}\"mergeCommit\":{\"oid\":\"$msha\"}"
          else
            j="${j:+$j,}\"mergeCommit\":null"
          fi ;;
        *)          j="${j:+$j,}\"$fld\":null" ;;
      esac
    done
    echo "{$j}"
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
    # Round 7 B3 steers (verifytrapall legs): dl-fail = the download itself fails;
    # dl-partial = only the zip arrives (incomplete assets); dl-badsums = a checksum
    # line that cannot verify; dl-upmismatch = the updater zip differs from the release
    # zip BUT the shipped SHA256SUMS covers the differing bytes (so shasum -c passes and
    # cmp fails — the check order is sums, then byte-identity); dl-notzip = both zips are
    # checksum-valid text files (ditto -x -k then fails on a non-zip).
    if [[ -e "$FAKE_STATE_DIR/dl-fail" ]]; then
      echo "gh: (fake) release download failed" >&2; exit 1
    fi
    if [[ -e "$FAKE_STATE_DIR/dl-partial" ]]; then
      cp "$FAKE_FIXTURES/download/Kai-0.5.1-darwin-arm64.zip" "$d/"
      exit 0
    fi
    cp "$FAKE_FIXTURES"/download/* "$d/"
    if [[ -e "$FAKE_STATE_DIR/dl-badsums" ]]; then
      printf '0000000000000000000000000000000000000000000000000000000000000000  Kai-0.5.1-darwin-arm64.zip\n' >>"$d/SHA256SUMS"
    fi
    if [[ -e "$FAKE_STATE_DIR/dl-upmismatch" ]]; then
      printf 'differs' >>"$d/updater-Kai-0.5.1-darwin-arm64.zip"
      ( cd "$d" && shasum -a 256 Kai-0.5.1-darwin-arm64.zip \
          updater-Kai-0.5.1-darwin-arm64.zip Kai-0.5.1-darwin-arm64.dmg >SHA256SUMS )
    fi
    if [[ -e "$FAKE_STATE_DIR/dl-notzip" ]]; then
      printf 'this is not a zip\n' >"$d/Kai-0.5.1-darwin-arm64.zip"
      cp "$d/Kai-0.5.1-darwin-arm64.zip" "$d/updater-Kai-0.5.1-darwin-arm64.zip"
      ( cd "$d" && shasum -a 256 Kai-0.5.1-darwin-arm64.zip \
          updater-Kai-0.5.1-darwin-arm64.zip Kai-0.5.1-darwin-arm64.dmg >SHA256SUMS )
    fi
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
  # Round 7 C1: the hardened fake git steers network subverbs to THIS local bare repo.
  FAKE_ORIGIN="$tmp/origins/$SCN.git"; export FAKE_ORIGIN
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
    KAI_RELEASE_REPO=Performant-Labs/kai KAI_CI_WAIT=0 KAI_CI_POLL=1 ${r_env[@]+"${r_env[@]}"} "$release" "$@" </dev/null 2>&1)"
  code=$?
}

state() { printf '%s\n' "$R/bin/release/v0.5.1/state.json"; }

# PR #100 R5 realism seeds. seed_merged_pr leaves a release PR the way GitHub leaves it
# after `gh pr merge --merge --delete-branch`: the PR is MERGED, refs/heads/release/v0.5.1
# is gone from origin, and the MERGE COMMIT exists only in origin (on master) — the local
# clone cannot resolve it until a git fetch installs it. The pr file's line 4 carries its
# oid (pr view's mergeCommit.oid); $SD/merge.sha is the harness-side copy for assertions.
seed_merged_pr() { # <pr-number>
  local n="$1" org base head msha
  org="$tmp/origins/$SCN.git"
  head="$($RG -C "$R" rev-parse release/v0.5.1 2>/dev/null || $RG -C "$R" rev-parse HEAD)"
  base="$($RG --git-dir="$org" rev-parse refs/heads/master)"
  $RG -C "$R" push -q "$org" "$head":refs/heads/__seed 2>/dev/null || true   # parent objects must exist in origin
  $RG --git-dir="$org" update-ref -d refs/heads/__seed 2>/dev/null || true
  msha="$(GIT_AUTHOR_NAME=github GIT_AUTHOR_EMAIL=noreply@github.com \
          GIT_COMMITTER_NAME=github GIT_COMMITTER_EMAIL=noreply@github.com \
          $RG --git-dir="$org" commit-tree "$base^{tree}" -p "$base" -p "$head" \
          -m "Merge pull request #$n from Performant-Labs/release-v0.5.1" 2>/dev/null || true)"
  [[ -n "$msha" ]] || { echo "FAIL seed_merged_pr ($SCN): no merge commit created"; fail=1; return 1; }
  $RG --git-dir="$org" update-ref refs/heads/master "$msha"
  printf 'MERGED\nrelease/v0.5.1\n%s\n%s\n' "$head" "$msha" >"$SD/ghstate/pr-$n"
  printf '%s\n' "$msha" >"$SD/merge.sha"
}

# seed_state writes the state.json an earlier run interrupted after <last-done-phase>
# would leave: every phase through <last> done, the rest not, the checklist issue and the
# PR already in ghstate. mergeSha appears from `merge` on, exactly like ph_merge's own
# postcondition write (release.sh st_update '.mergeSha = $m').
seed_state() { # <last-done-phase>
  local last="$1" p passed=0 first=1
  mkdir -p "$(dirname "$(state)")"
  {
    printf '{"posture":"publish","tag":"v0.5.1","issue":102,"pr":101'
    printf ',"headSha":"%s"' "$(sed -n 3p "$SD/ghstate/pr-101")"
    case " merge tag local smoke publish verify-published closeout " in
      *" $last "*) printf ',"mergeSha":"%s"' "$(sed -n 4p "$SD/ghstate/pr-101")" ;;
    esac
    printf ',"phases":{'
    for p in needs preflight changelog branch bump restructure release-pr review merge \
             tag local smoke publish verify-published closeout; do
      [[ $first -eq 1 ]] || printf ','
      first=0
      if [[ $passed -eq 0 ]]; then
        printf '"%s":{"done":true,"commit":"%s"}' "$p" "$($RG -C "$R" rev-parse HEAD)"
      else
        printf '"%s":{"done":false}' "$p"
      fi
      [[ "$p" == "$last" ]] && passed=1
    done
    printf '},"consultations":[],"smoke":"pending"}\n'
  } >"$(state)"
}

# C01 — the script under test must exist (the RED gate: everything below fails without it).
if [[ -x "$release" ]]; then
  echo "ok   C01 scripts/release.sh exists and is executable"
else
  echo "FAIL C01 scripts/release.sh exists and is executable (feature absent: F implements it)"
  fail=1
fi

# ================================================================ AC1 + AC2: one release's life
# R2/R8 (issue #103): the DEFAULT posture performs the whole release through the draft:
# release-local.sh vX.Y.Z --yes. --dry-run no longer performs any of it (see "#103" below).
# Then --publish resumes (posture escalation, R6), then a full-success re-run is a no-op (AC2).
mkscn ac1life
rrun v0.5.1
exit_is     "AC1: v0.5.1 with no input exits 0"                                 0
has         "AC1: names the remaining owner step: publish"                      'publish'
hasnt       "AC1: never asks a terminal question"                               '\[y/N\]'
fhas        "AC1/R10p1: preflight wrapped with the version"                     "$SD/preflight.log"   '^v0\.5\.1$'
fhas        "AC1/R10p4: bump wrapped with the PLAIN version (no v)"             "$SD/bump.log"        '^0\.5\.1$'
fhas        "AC1/R8: local wrapped as 'v0.5.1 --yes' (the draft)"               "$SD/local.log"       '^v0\.5\.1 --yes$'
exists      "AC1: packaged notes stay in bin/release/v0.5.1"                    "$R/bin/release/v0.5.1/RELEASE_NOTES.md"
exists      "AC1: the verified Kai.app stays for the smoke matrix"              "$R/bin/release/v0.5.1/Kai.app"
exists      "AC1: staged asset: the zip"                                        "$R/bin/release/v0.5.1/assets/Kai-0.5.1-darwin-arm64.zip"
exists      "AC1: staged asset: the updater zip"                                "$R/bin/release/v0.5.1/assets/updater-Kai-0.5.1-darwin-arm64.zip"
exists      "AC1: staged asset: the dmg"                                        "$R/bin/release/v0.5.1/assets/Kai-0.5.1-darwin-arm64.dmg"
exists      "AC1: staged asset: SHA256SUMS"                                     "$R/bin/release/v0.5.1/assets/SHA256SUMS"
jq_is       "AC1/R6: state.json records the posture"                            "$(state)" '.posture' 'default'
jq_is       "AC1/R2: state.json records phases done through local"              "$(state)" '.phases.local.done' 'true'
fhas        "AC1/R10p0: the checklist issue is opened by exact title"           "$SD/gh.log"          'issue create.*Release checklist: v0\.5\.1'
fhas        "AC1/R10p2: known-issue candidates land in the checklist issue"     "$SD/comments.log"    '501'
fhas        "AC1/R10p1+R11: the preflight WARN gets a marker comment"           "$SD/comments.log"    'release\.sh: v0\.5\.1 preflight'
fhas        "AC1/A1: the release branch is pushed" "$SD/git.log"       '^push( -q)? origin release/v0\.5\.1$'
fhas        "AC1/A1: the tag is pushed"                                       "$SD/git.log"         '^push( -q)? origin v0\.5\.1$'
fhas        "AC1/R10p6: the release PR targets the fork, not upstream"          "$SD/gh.log"          'pr create.*--repo Performant-Labs/kai'
fhas        "AC1/R10p6: the release PR targets master"                          "$SD/gh.log"          'pr create.*--base master'
fhas        "AC1/R10p8: merge is --merge --delete-branch"                       "$SD/gh.log"          'pr merge.*--merge.*--delete-branch'

# AC2/R6: resume at the draft boundary into the higher posture, then the no-op re-run.
rrun v0.5.1 --publish
exit_is     "AC2: --publish resumes after the draft and exits 0"     0
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

# ================================================================ round 7 A: the --dry-run flag gate (drystop)
# The smoke flags are owner gates that live AFTER the dry-run boundary (phase 11 vs the
# boundary exit after phase 10): the dry-run posture exits before ph_smoke, so --smoke
# and --smoke-done combined with --dry-run are posture contradictions that today are
# SILENTLY IGNORED (the run completes the dry run, exit 0, nothing smoke-related
# recorded — a flag combination the gate skipped). The contract: refuse UP FRONT —
# non-zero exit, a message naming both flags and why (the smoke matrix needs the default
# posture; smoke recording happens after the draft) — BEFORE any phase runs (no gh call,
# no wrapped tool, no VCS op). The green companions: --dry-run alone is pinned by ac1life
# and --smoke alone by `smoke` above (the refusal must not over-reach); --dry-run
# --publish is the existing combination refusal (release.sh:46), pinned here so the new
# gate matches that shape; --quit-kai is posture-independent and must still ACT (dryquit).
mkscn drystop
rrun v0.5.1 --dry-run --smoke
exit_nonzero "R7/drystop: --dry-run --smoke is refused up front"
has         "R7/drystop: the refusal names --dry-run"                    '--dry-run'
has         "R7/drystop: the refusal names --smoke"                      '--smoke'
has         "R7/drystop: the refusal says why (matrix/posture/draft)"    'matrix|default posture|draft'
count_is    "R7/drystop: no gh call happens (refusal precedes phases)"   "$SD/gh.log" 0
count_is    "R7/drystop: no wrapped tool runs (preflight)"               "$SD/preflight.log" 0
fhasnt      "R7/drystop: no VCS op happens (no checkout/push/commit)"    "$SD/git.log" 'checkout|^push|^commit|^tag '

mkscn drystopdone
rrun v0.5.1 --dry-run --smoke-done
exit_nonzero "R7/drystop: --dry-run --smoke-done is refused up front"
has         "R7/drystop: the refusal names --smoke-done"                 '--smoke-done'
has         "R7/drystop: the refusal names --dry-run"                    '--dry-run'
count_is    "R7/drystop: no gh call happens"                             "$SD/gh.log" 0
count_is    "R7/drystop: no wrapped tool runs (preflight)"               "$SD/preflight.log" 0

mkscn drystopub
rrun v0.5.1 --dry-run --publish
exit_nonzero "R7/drystop: --dry-run --publish is refused (existing gate, pinned)"
has         "R7/drystop: the dry/publish refusal names the conflict"     'not both'
count_is    "R7/drystop: no gh call happens"                             "$SD/gh.log" 0

# Issue #103: quitting an app is a mutation, so --quit-kai under --dry-run is refused up front
# like the other mutating flags; --quit-kai alone (default/publish) still acts.
mkscn dryquit
: >"$SD/kai-running"
rrun v0.5.1 --dry-run --quit-kai
exit_nonzero "#103: --dry-run --quit-kai is refused up front"
has         "#103: the refusal names --dry-run"                          '--dry-run'
has         "#103: the refusal names --quit-kai"                         '--quit-kai'
count_is    "#103: no gh call happens (refusal precedes phases)"         "$SD/gh.log" 0
count_is    "#103: no wrapped tool runs (preflight)"                     "$SD/preflight.log" 0
fhasnt      "#103: Kai was not quit"                                     "$SD/osascript.log" '[Qq]uit'

mkscn quitkai_default
: >"$SD/kai-running"
rrun v0.5.1 --quit-kai
exit_is     "#103: --quit-kai alone (default posture) still acts"        0
fhas        "#103: the quit was dispatched (osascript)"                  "$SD/osascript.log" '[Qq]uit'

# ================================================================ #103: --dry-run is DRY
# --dry-run runs the read-only phases (needs, preflight, changelog) for real, prints one plan
# line per mutating phase, and performs nothing: no commit, push, tag, branch, PR, merge,
# release, issue create/comment/close, no release-local.sh, no state marked done.
same_is() { # name a b
  if [[ "$2" == "$3" ]]; then echo "ok   $1"; else echo "FAIL $1 ('$2' != '$3')"; fail=1; fi; }
out_count_is() { # name pattern want
  local n; n="$(printf '%s\n' "$out" | grep -cE -- "$2" || true)"
  if [[ "$n" == "$3" ]]; then echo "ok   $1"; else echo "FAIL $1 (output matched /$2/ $n times, wanted $3)"; fail=1; fi; }
MUT_GIT='^(push|commit|tag|merge|reset|add|stash|cherry-pick|rebase|worktree|am|apply|clean)( |$)|^checkout -b|^switch -c|^branch( -[a-zA-Z]+)? [^-]'
MUT_GH='pr (create|merge|close|edit|ready|comment)|release (create|edit|delete|upload)|issue (create|comment|close|edit|reopen)|(-X|--method)[ =]+(POST|PUT|PATCH|DELETE)|--input'

mkscn dryrun
head0="$($RG -C "$R" rev-parse HEAD)"
refs0="$($RG -C "$R" for-each-ref | shasum | cut -d' ' -f1)"
orefs0="$($RG --git-dir="$FAKE_ORIGIN" for-each-ref | shasum | cut -d' ' -f1)"
rrun v0.5.1 --dry-run
exit_is     "#103: --dry-run on a healthy repo exits 0"                        0
has         "#103: the needs phase ran for real"                                'phase 0: needs'
has         "#103: the preflight phase ran for real"                            'phase 1: preflight'
has         "#103: the changelog phase ran for real"                            'phase 2: changelog'
fhas        "#103: release-preflight.sh really ran (read-only phase)"           "$SD/preflight.log"   '^v0\.5\.1$'
fhas        "#103: changelog-check.sh really ran (read-only phase)"             "$SD/changelog.log"   'strict'
out_count_is "#103: exactly one plan line per mutating phase (12)"              '^  \[plan\] ' 12
for ph in branch bump restructure release-pr review merge tag local smoke publish verify-published closeout; do
  has       "#103: the plan names the $ph phase"                                "^  \[plan\] $ph: would "
done
has         "#103: says nothing was changed"                                    '[Nn]othing was (created|changed|pushed)'
has         "#103: says how to do it for real"                                  'scripts/release\.sh v0\.5\.1'
fhasnt      "#103: no mutating git command ran"                                 "$SD/git.log"         "$MUT_GIT"
fhasnt      "#103: no mutating gh command ran"                                  "$SD/gh.log"          "$MUT_GH"
count_is    "#103: no checklist comment was posted"                             "$SD/comments.log" 0
count_is    "#103: release-bump.sh did not run"                                 "$SD/bump.log" 0
count_is    "#103: release-local.sh did not run"                                "$SD/local.log" 0
count_is    "#103: the AI helper was not consulted"                             "$SD/askai.log" 0
same_is     "#103: HEAD is unchanged"                                           "$head0" "$($RG -C "$R" rev-parse HEAD)"
same_is     "#103: local refs are unchanged"                                    "$refs0" "$($RG -C "$R" for-each-ref | shasum | cut -d' ' -f1)"
same_is     "#103: the remote's refs are unchanged"                             "$orefs0" "$($RG --git-dir="$FAKE_ORIGIN" for-each-ref | shasum | cut -d' ' -f1)"
same_is     "#103: the working tree is clean"                                   "" "$($RG -C "$R" status --porcelain)"
jq_is       "#103: the posture is recorded"                                     "$(state)" '.posture' 'dry-run'
jq_is       "#103: NO phase is recorded done after a dry run"                   "$(state)" '[.phases[] | select(.done)] | length' '0'
has         "#103: the skipped checklist-issue creation is reported"            'dry-run\] skipped: gh issue create'

# A real run after a dry run starts every phase fresh and performs them (escalation works).
rrun v0.5.1
exit_is     "#103: a default run after a dry run succeeds"                      0
fhas        "#103: the release branch is pushed by the real run"                "$SD/git.log"         '^push( -q)? origin release/v0\.5\.1$'
fhas        "#103: the tag is pushed by the real run"                           "$SD/git.log"         '^push( -q)? origin v0\.5\.1$'
match_is    "#103: exactly one release PR is created, by the real run"          "$SD/gh.log" 'pr create' 1
fhas        "#103: the draft is built by the real run"                          "$SD/local.log"       '^v0\.5\.1 --yes$'
jq_is       "#103: the posture escalated to default"                            "$(state)" '.posture' 'default'

# A read-only phase that fails stops the dry run WITHOUT posting to the checklist issue.
mkscn dryfail
: >"$SD/preflight-fail"
rrun v0.5.1 --dry-run
exit_nonzero "#103: a failing preflight stops a dry run"
has         "#103: the stop names the phase"                                    'STOP.*preflight'
fhasnt      "#103: the stop did not touch the checklist issue"                  "$SD/gh.log"          "$MUT_GH"
count_is    "#103: no stop comment was posted"                                  "$SD/comments.log" 0
jq_is       "#103: nothing recorded done"                                       "$(state)" '[.phases[] | select(.done)] | length' '0'

# Kai running under a dry run is a stop (it cannot be quit), never a quit.
mkscn drykai
: >"$SD/kai-running"
rrun v0.5.1 --dry-run
exit_nonzero "#103: Kai running stops a dry run"
has         "#103: the stop says Kai is running"                                'Kai is running'
fhasnt      "#103: Kai was not quit"                                            "$SD/osascript.log"   '[Qq]uit'

# Mutation self-test (issue #103): with the DRYSTOP block neutered, a "dry run" mutates, and the
# assertions above must be able to SEE it. Otherwise they would pass vacuously.
mkscn drymut
awk '/DRYSTOP/ { print "if false; then"; next } { print }' "$release" >"$tmp/release.mutant.sh"
chmod +x "$tmp/release.mutant.sh"
release_real="$release"; release="$tmp/release.mutant.sh"
rrun v0.5.1 --dry-run
release="$release_real"
fhas        "#103/mutation: with the stop removed the mutating git commands appear"   "$SD/git.log"  "$MUT_GIT"
fhas        "#103/mutation: with the stop removed the mutating gh commands appear"    "$SD/gh.log"   "$MUT_GH"
jq_is       "#103/defence: mark_done has its own dry-run guard (state stays clean even then)" "$(state)" '[.phases[] | select(.done)] | length' '0'

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
seed_merged_pr 101   # R5 realism: a MERGED PR carries a merge commit (on origin/master only)
rrun v0.5.1
exit_is     "AC3/#74: merged leftover branch is a group-skip, not a stop"       0
count_is    "AC3/#74: phases 3-6 are skipped (no bump)"                         "$SD/bump.log" 0
fhas        "AC3/#74: the leftover branch is deleted via gh api -X DELETE"      "$SD/gh.log" 'api -X DELETE.*git/refs/heads/release/v0\.5\.1'
count_is    "AC3/#74: the static handler never consults the AI"                 "$SD/askai.log" 0
exists      "AC3/#74: the run still reaches the draft"                          "$SD/ghstate/release-v0.5.1.json"

# (2b) PR #100 F1: the same merge seen from the resume side — state.json ABSENT, the release
# PR MERGED, its remote branch already deleted (gh pr merge --delete-branch), and the local
# release/vX.Y.Z still checked out. ph_branch must group-skip phases 3-6 exactly like the
# leftover-branch path above (#74): re-running release-bump.sh would leave uncommitted
# version edits on the tree (no release_pr_apply runs afterwards to commit them).
mkscn mergedpr
$RG -C "$R" checkout -q -b release/v0.5.1
printf '101\n' >"$SD/ghstate/seq"
seed_merged_pr 101   # R5 realism: a MERGED PR carries a merge commit (on origin/master only)
rm -f "$(state)"   # state.json absent: the resume must re-derive from postconditions (R6)
rrun v0.5.1
exit_is     "PR100-F1: MERGED PR + deleted branch + absent state completes"      0
count_is    "PR100-F1: bump is NOT re-run (group-skip mirrors #74)"              "$SD/bump.log" 0
jq_is       "PR100-F1: bump marked done by the group-skip"                       "$(state)" '.phases.bump.done' 'true'
jq_is       "PR100-F1: restructure marked done by the group-skip"                "$(state)" '.phases.restructure.done' 'true'
jq_is       "PR100-F1: release-pr marked done by the group-skip"                 "$(state)" '.phases["release-pr"].done' 'true'
dirty="$($RG -C "$R" status --porcelain 2>/dev/null)"
if [[ -z "$dirty" ]]; then echo "ok   PR100-F1: the tree stays clean (no uncommitted version edits)"; else echo "FAIL PR100-F1: uncommitted version edits: $dirty"; fail=1; fi
exists      "PR100-F1: the run still reaches the draft"                          "$SD/ghstate/release-v0.5.1.json"

# (3) Bindings drift, by which side of the tag it fires (PR #100 round 4, R4a "Post-tag
# source mutation", finding 3fe580bc5f88 ACTIVE, release.sh:560-621): the tag is pushed at
# phase 9 and the drift can surface at phase 10, where handler_regen_bindings commits
# regenerated frontend/bindings to the still-checked-out release branch — moving it past the
# already-pushed tag. docs/releasing.md: "The git tag vX.Y.Z is the source of truth" and
# "Anything else changed after the build means the app was not built from the tag as
# committed", so POST-tag drift is a hard stop for the owner: no regen (it would only dirty
# or mutate the tagged tree), no commit to release/*, no consultation (a static handler that
# refuses). `posttagdrift` supersedes the round-1 `bindings` case, which pinned that phase-10
# commit as correct; `premergedrift` below keeps the #73 regen+commit contract where it is
# still valid — PRE-merge, no tag exists yet, the commit rides the release branch through the
# phase-7 review gate (the pipeline's re-review) and the eventual tag includes it.
mkscn posttagdrift
echo bindings >"$SD/local-fail"
r_env=(FAKE_AI=retry)   # an AI is available and would say retry — this class still never asks
rrun v0.5.1
exit_nonzero "PR100-R4a: post-tag bindings drift is a hard stop, not a regen+commit"
jq_is      "PR100-R4a: premise: the tag phase was already done when the drift fired" "$(state)" '.phases.tag.done' 'true'
count_is   "PR100-R4a: no regeneration is attempted post-tag"                       "$SD/make.log" 0
fhasnt     "PR100-R4a: nothing is committed to the tagged release branch"           "$SD/git.log" 'regenerate frontend/bindings'
count_is   "PR100-R4a: the post-tag refusal never consults the AI"                  "$SD/askai.log" 0
count_is   "PR100-R4a: local ran once, no blind retry"                              "$SD/local.log" 1
has        "PR100-R4a: the stop names the tag as the truth (an owner decision)"     'truth'
has        "PR100-R4a: the stop names the bindings drift"                           'bindings'
fhas       "PR100-R4a: the stop is visible on the checklist issue"                  "$SD/comments.log" 'STOP at phase local'
jq_is      "PR100-R4a: local is not marked done"                                    "$(state)" '.phases.local.done' 'false'
match_is   "PR100-R4a: nothing is drafted past a post-tag drift stop"               "$SD/gh.log" 'release create' 0

# The pre-merge side: the #73 handler stays exactly as ruled where no tag exists yet
# (steered at phase 4 — the cheapest phase whose log can carry the drift signature; the
# dispatch under test is phase_run's, generic across phases).
mkscn premergedrift
: >"$SD/bump-drift"
rrun v0.5.1
exit_is     "PR100-R4a: pre-merge bindings drift still regenerates and proceeds"         0
fhas        "PR100-R4a: make bindings runs pre-merge"                                    "$SD/make.log" '^bindings$'
fhas        "PR100-R4a: the regenerated bindings are committed to the release branch"    "$SD/git.log" 'regenerate frontend/bindings'
count_is    "PR100-R4a: the pre-merge handler never consults the AI"                     "$SD/askai.log" 0
val_is      "PR100-R4a: bump re-ran after the regen (fail, then retry)"                  "$SD/bump.count" '2'
jq_is       "PR100-R4a: the eventual tag then includes the regen (tag done)"             "$(state)" '.phases.tag.done' 'true'
exists      "PR100-R4a: the run still reaches the draft"                                 "$SD/ghstate/release-v0.5.1.json"

# ================================================================ round 7 B1: per-handler retry counters
# phase_run's blocker-handler retry cap is PER HANDLER: quit-kai and regen-bindings each
# get their two dispatches within one phase run. The old SHARED counter (one `handlers`
# variable for both tables) made quit-kai twice + regen-bindings once impossible in one
# phase — the third event tripped "bindings drift persists after regeneration" though the
# regen handler had never run at all. The AI-consultation cap stays 2 PER PHASE and per
# phase only (unchanged; pinned by aicap below). Steered at phase 4 via the bump.seq
# per-invocation sequence: the dispatch logic under test is phase_run's, generic across
# phases (premergedrift's precedent).
mkscn handlercounts
printf 'kai kai bindings bindings\n' >"$SD/bump.seq"
rrun v0.5.1 --quit-kai
exit_is     "R7/handlercounts: quit-kai x2 + regen-bindings x2 all fit one phase run" 0
match_is    "R7/handlercounts: the quit handler dispatched exactly twice"  "$SD/osascript.log" '[Qq]uit' 2
match_is    "R7/handlercounts: the regen handler dispatched exactly twice" "$SD/make.log"      '^bindings$' 2
val_is      "R7/handlercounts: bump ran 5 times (4 blocked + 1 clean)"     "$SD/bump.count" '5'
count_is    "R7/handlercounts: known blockers never consult the AI"        "$SD/askai.log" 0
exists      "R7/handlercounts: the run still reaches the draft"            "$SD/ghstate/release-v0.5.1.json"

# The caps themselves stand (per handler, not in total): a THIRD quit attempt stops...
mkscn handlercapkai
printf 'kai kai kai\n' >"$SD/bump.seq"
rrun v0.5.1 --quit-kai
exit_nonzero "R7/handlercounts: a third quit attempt still stops"
has         "R7/handlercounts: the stop is the two-quits stop"             'still running after two quit attempts'
match_is    "R7/handlercounts: exactly two quits dispatched"               "$SD/osascript.log" '[Qq]uit' 2
count_is    "R7/handlercounts: no consultation for a known blocker"        "$SD/askai.log" 0

# ...and persistent bindings drift still allows exactly two regens, then stops.
mkscn handlercapregen
printf 'bindings bindings bindings\n' >"$SD/bump.seq"
rrun v0.5.1
exit_nonzero "R7/handlercounts: persistent drift stops after its two regens"
has         "R7/handlercounts: the stop is the drift-persists stop"        'persists after regeneration'
match_is    "R7/handlercounts: the regen handler dispatched exactly twice" "$SD/make.log" '^bindings$' 2
val_is      "R7/handlercounts: bump ran three times only"                  "$SD/bump.count" '3'
count_is    "R7/handlercounts: no consultation for a known blocker"        "$SD/askai.log" 0

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
# PR100-R2 companion: a none-marker only counts INSIDE [Unreleased] (the doc contract the stop
# message states). This resume therefore places the fresh marker there — the old form appended
# it at EOF, under the released [0.4.0] section, which only ever unblocked the phase because
# ph_changelog grepped the whole file.
cat >"$R/CHANGELOG.md" <<MD
# Changelog

## [Unreleased]
<!-- changelog-skip: release tooling, no user-facing change: #74 -->
<!-- known-issues: none $(date +%Y-%m-%d) -->
- A fake enhancement entry so coverage is green (#101).

## [0.4.0] - 2026-01-01
- Old release.
MD
$RG -C "$R" commit -qam known-issues-none || true
rrun v0.5.1
exit_is     "R10p2: a none-marker inside [Unreleased] unblocks the phase"       0
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

# (3b) A re-run supersedes the stale failed run of the same check; a later failure still stops;
# late-registering and never-finishing checks are waited for, then stop with the pending name.
mkscn hscirerun
r_env=(FAKE_CI=rerunok)
rrun v0.5.1
hasnt       "R10p7: a passing re-run supersedes a stale failed run of the same check" 'CI is not green'
mkscn hscirerunbad
r_env=(FAKE_CI=rerunbad)
rrun v0.5.1
exit_nonzero "R10p7: the latest run failing is still a hard stop"
has         "R10p7: the stop names the failed check"                                   'CI is not green.*pr-title'
mkscn hscilate
r_env=(FAKE_CI=late KAI_CI_WAIT=5 KAI_CI_POLL=1)
rrun v0.5.1
hasnt       "R10p7: checks that register late are waited for, not a hard stop"          'CI is not green'
mkscn hsciwait
r_env=(FAKE_CI=running)
rrun v0.5.1
exit_nonzero "R10p7: a check that never finishes stops after the wait"
has         "R10p7: the stop names the running check"                                  'CI is not green.*in_progress'

# (3c) No machine block (the real repo's PR-Agent posts only the visible review): fail-closed fallback.
mkscn hsnostateok
r_env=(FAKE_AGENT=nostate-ok)
rrun v0.5.1
hasnt       "R1: a fresh visible review, score>=90, no issues passes without a machine block" 'hard stop: the PR-Agent review gate failed'
for k in stale findings low; do
  mkscn hsnostate$k
  r_env=(FAKE_AGENT=nostate-$k)
  rrun v0.5.1
  exit_nonzero "R1: no machine block + $k review is a hard stop"
  has         "R1: no machine block + $k review: the stop names the PR-Agent gate" 'hard stop: the PR-Agent review gate failed'
done

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

# ================================================================ PR #100 F2: phase-13 temp hygiene
# ph_verify's release-verify.* workdir and the install-release --dest dir must not survive
# ANY exit: success, a hard stop, or a forced mid-verify kill. TMPDIR is steered per
# scenario (release.sh expands it into the workdir template itself), and the --dest dir is
# checked by the absolute path the fake install-release.sh recorded — macOS mktemp's
# no-template form ignores $TMPDIR, so it cannot be steered, only observed.
vleft() { # name — no release-verify.* workdir survives under the steered TMPDIR
  if [[ -z "$(ls -d "$SD/tmp"/release-verify.* 2>/dev/null)" ]]; then
    echo "ok   $1"
  else
    echo "FAIL $1 ($(ls -d "$SD/tmp"/release-verify.* 2>/dev/null | tr '\n' ' '))"; fail=1
  fi }
destleft() { # name — the install-release --dest dir from the recorded argv is gone
  local d; d="$(sed -n 's/.* --dest \([^ ]*\).*/\1/p' "$SD/install-release.log" 2>/dev/null | tail -1)"
  if [[ -n "$d" && ! -e "$d" ]]; then echo "ok   $1"; else echo "FAIL $1 (--dest ${d:-unset} still exists)"; fail=1; fi }
tmpleft() { # name — nothing at all survives under the steered TMPDIR
  local n=0; [[ -d "$SD/tmp" ]] && n="$(find "$SD/tmp" -mindepth 1 2>/dev/null | wc -l | tr -d ' ')"
  if [[ "$n" -eq 0 ]]; then echo "ok   $1"; else
    echo "FAIL $1 (leftovers: $(find "$SD/tmp" -mindepth 1 2>/dev/null | sed "s#$SD/tmp/##" | tr '\n' ' '))"; fail=1
  fi }

# Success path: even a clean verify leaves nothing behind (today only the workdir is
# cleaned; the inline --dest mktemp leaks unconditionally — release.sh:680).
mkscn verifyok
rrun v0.5.1
exit_is     "PR100-F2: verifyok setup: default posture completes first"          0
mkdir -p "$SD/tmp"
r_env=(TMPDIR="$SD/tmp")
rrun v0.5.1 --publish
exit_is     "PR100-F2: verify completes on the success path"                     0
vleft       "PR100-F2: no release-verify.* workdir survives a successful verify"
destleft    "PR100-F2: the install-release --dest dir does not survive success"
tmpleft     "PR100-F2: nothing at all survives a successful verify"

# Hard stop mid-verify (install-release fails after its --dest dir exists): do_stop exits
# between the two temp dirs; both must be cleaned on the way out.
mkscn verifystop
rrun v0.5.1
exit_is     "PR100-F2: verifystop setup: default posture completes first"        0
: >"$SD/install-fail"
mkdir -p "$SD/tmp"
r_env=(TMPDIR="$SD/tmp")
rrun v0.5.1 --publish
exit_nonzero "PR100-F2: an install failure mid-verify stops the run"
has         "PR100-F2: the stop names phase verify-published"                    'verify-published'
vleft       "PR100-F2: no release-verify.* workdir survives a hard stop"
destleft    "PR100-F2: the install-release --dest dir does not survive a hard stop"
tmpleft     "PR100-F2: nothing at all survives a hard stop"

# Forced mid-verify exit (SIGTERM while the workdir is live): the cleanup must be
# trap-based — an rm at the end of the happy path cannot run after a kill; a trap can.
mkscn verifyterm
rrun v0.5.1
exit_is     "PR100-F2: verifyterm setup: default posture completes first"        0
: >"$SD/verify-term"
mkdir -p "$SD/tmp"
r_env=(TMPDIR="$SD/tmp")
rrun v0.5.1 --publish
exit_nonzero "PR100-F2: a forced mid-verify kill exits non-zero"
vleft       "PR100-F2: trap-based cleanup still runs on a forced mid-verify exit"
tmpleft     "PR100-F2: nothing at all survives a forced mid-verify exit"

# ================================================================ round 7 B3: EVERY ph_verify stop path
# is trap-cleaned (the verify workdir; the installer --dest dir is created only on the
# last two stops and is pinned by verifystop/verifyok above). verifyok/verifystop/
# verifyterm cover success, the installer failure and the forced kill; the legs below
# drive each REMAINING stop inside ph_verify: download fail, incomplete assets,
# checksum fail, updater-zip mismatch, ditto fail, release-verify fail, mount fail,
# dmg mismatch. Each leg: steer, resume --publish, assert the stop names its cause AND
# nothing survives under the steered TMPDIR.
vleg() { # <name> <marker> <stop-regex>
  mkscn "$1"
  rrun v0.5.1
  exit_is "R7/$1: setup: default posture completes first" 0
  : >"$SD/$2"
  mkdir -p "$SD/tmp"
  r_env=(TMPDIR="$SD/tmp")
  rrun v0.5.1 --publish
  exit_nonzero "R7/$1: the steered verify failure stops the run"
  has     "R7/$1: the stop names phase verify-published"        'verify-published'
  has     "R7/$1: the stop names the cause ($3)"                "$3"
  vleft   "R7/$1: no release-verify.* workdir survives the stop"
  tmpleft "R7/$1: nothing at all survives the stop"
}
vleg vtdownload   dl-fail       'release download .* failed|download'
vleg vtassets     dl-partial    'incomplete'
vleg vtsums       dl-badsums    'SHA256SUMS'
vleg vtupmismatch dl-upmismatch 'byte-identical|updater'
vleg vtditto      dl-notzip     'ditto'
vleg vtverify     verify-fail   'release-verify\.sh failed'
vleg vtmount      mount-fail    'mount'
vleg vtdmg        dmg-diff      'dmg'

# ================================================================ PR #100 re-review: 2 new ACTIVE findings
# (R1) ph_tag resume gap on an unpushed tag: the annotated tag already exists LOCALLY at the
# merge commit, the remote never got it (the push after `git tag -a` was interrupted), and
# state is gone. Re-running must push the EXISTING local tag and mark tag done — a second
# `git tag -a` fails the way real git does ("tag 'vX.Y.Z' already exists", the fake execs the
# real git) and must never be attempted (R5: a tag is never moved or deleted). Today the
# phase stops on that failed re-tag instead (release.sh:570-580 checks only the REMOTE tag).
mkscn tagpush
rrun v0.5.1
exit_is     "PR100-R1: tagpush setup: default posture completes first"           0
$RG -C "$R" push -q origin --delete v0.5.1   # harness-side: the remote lost the tag; the LOCAL one stays
rm -f "$(state)"                             # state absent: the resume re-derives (R6)
rrun v0.5.1 --publish
exit_is     "PR100-R1: an unpushed local tag at the merge commit is pushed, not re-created" 0
match_is    "PR100-R1: the EXISTING local tag is pushed exactly once"           "$SD/git.log" '^push( -q)? origin v0\.5\.1$' 2
match_is    "PR100-R1: no second 'git tag -a' is attempted on resume"           "$SD/git.log" '^tag -a v0\.5\.1' 1
jq_is       "PR100-R1: tag marked done by the push"                             "$(state)" '.phases.tag.done' 'true'
jq_is       "PR100-R1: the resumed run runs to completion"                       "$(state)" '.phases.closeout.done' 'true'

# (R2) ph_changelog gate bypass via a stale marker: the none-marker check greps the WHOLE
# file (release.sh:399), so an old `known-issues: none` marker sitting under a RELEASED
# version section satisfies the gate while [Unreleased] has no Known Issues bullets and no
# fresh marker. The gate must still stop for the owner: only a marker inside [Unreleased]
# counts (the repaired chgstop resume above pins that positive half).
mkscn stalemarker
cat >"$R/CHANGELOG.md" <<'MD'
# Changelog

## [Unreleased]
<!-- changelog-skip: release tooling, no user-facing change: #74 -->
- A fake enhancement entry so coverage is green (#101).

## [0.4.0] - 2026-01-01
- Old release.

<!-- known-issues: none 2026-09-01 -->
MD
$RG -C "$R" commit -qam stale-known-issues-marker || true
rrun v0.5.1
exit_nonzero "PR100-R2: a stale none-marker under a RELEASED section does not bypass the gate"
has         "PR100-R2: the stop still names the uncurated Known Issues"         'no curated Known Issues'
count_is    "PR100-R2: the stop-for-owner never consults the AI"                "$SD/askai.log" 0
jq_is       "PR100-R2: changelog is NOT marked done by a stale marker"          "$(state)" '.phases.changelog.done' 'false'
count_is    "PR100-R2: nothing past the gate runs (no bump)"                    "$SD/bump.log" 0

# ================================================================ PR #100 round 3: 1 ACTIVE finding
# (R3) ph_smoke silent downgrade (release.sh:646-649): a --smoke run records .smoke =
# "awaiting-owner" and exits 0 WITHOUT marking the phase done (an owner gate). A resume
# without --smoke-done — with or without --publish — falls through the no-flag branch,
# writes .smoke = "skipped", marks the phase done and (under --publish) publishes: the
# manual smoke matrix is silently skipped. The resume must refuse instead, telling the
# owner to pass --smoke-done, and record nothing (awaiting-owner stands; publish never
# runs). The valid transitions stay pinned above and at the end of this case: absent +
# no-flag -> skipped (scenario `default`), absent + --smoke -> awaiting-owner exit 0
# (scenario `smoke` and the setup here), awaiting + --smoke-done -> done (here).
mkscn smokeresume
rrun v0.5.1 --smoke
exit_is     "PR100-R3: setup: --smoke stops awaiting the owner"                   0
jq_is       "PR100-R3: setup: state records awaiting-owner"                       "$(state)" '.smoke' 'awaiting-owner'
jq_is       "PR100-R3: setup: the smoke phase is not done (owner gate)"           "$(state)" '.phases.smoke.done' 'false'
# The defect, variant 1: re-run with NO flags — today it silently downgrades to skipped.
rrun v0.5.1
exit_nonzero "PR100-R3: a no-flag re-run is refused, not downgraded"
has         "PR100-R3: the refusal tells the owner to pass --smoke-done"          '--smoke-done'
jq_is       "PR100-R3: no-flag: awaiting-owner is not downgraded to skipped"      "$(state)" '.smoke' 'awaiting-owner'
jq_is       "PR100-R3: no-flag: the smoke phase stays not-done"                   "$(state)" '.phases.smoke.done' 'false'
fhasnt      "PR100-R3: no-flag: no smoke-skipped comment is posted"               "$SD/comments.log" 'smoke: skipped'
# The defect, variant 2 (the finding's exact shape): --publish resume without --smoke-done.
rrun v0.5.1 --publish
exit_nonzero "PR100-R3: a --publish resume without --smoke-done is refused"
has         "PR100-R3: that refusal also names --smoke-done"                      '--smoke-done'
count_is    "PR100-R3: the owner-gate refusal never consults the AI"              "$SD/askai.log" 0
jq_is       "PR100-R3: publish-resume: awaiting-owner still stands"               "$(state)" '.smoke' 'awaiting-owner'
jq_is       "PR100-R3: publish-resume: the smoke phase stays not-done"            "$(state)" '.phases.smoke.done' 'false'
jq_is       "PR100-R3: publish never runs without the owner's smoke record"       "$(state)" '.phases.publish.done' 'false'
match_is    "PR100-R3: no release edit without the owner's smoke record"          "$SD/gh.log" 'release edit' 0
jq_is       "PR100-R3: the draft stays a draft"                                   "$SD/ghstate/release-v0.5.1.json" '.isDraft' 'true'
# The valid transition out of awaiting-owner still works after the refusals.
rrun v0.5.1 --publish --smoke-done
exit_is     "PR100-R3: --smoke-done finally resumes and completes"                0
jq_is       "PR100-R3: the owner assertion is recorded after the refusals"        "$(state)" '.smoke | startswith("done (owner-asserted")' 'true'
jq_is       "PR100-R3: the run reaches closeout"                                  "$(state)" '.phases.closeout.done' 'true'

# ================================================================ PR #100 round 4: 2 ACTIVE findings
# (R4b) "Masked exit status" (finding 5f2708832375 ACTIVE, release.sh:479-487):
# release_pr_apply ends with `rm -f "$bf"`, so a failing `git push` or `gh pr create`
# inside it is invisible to phase_run — the phase reports success, never consults, never
# retries, and the run learns only later ("no PR found") or never. The apply's failures must
# reach phase_run and follow the unknown-failure contract (R3/R4): exactly one consultation
# on the first failure, retry on the AI's answer, stop at the cap of 2 — never a silent
# success, never a mark_done release-pr on a failure. Steered for both commands the finding
# names: gh pr create (the FAKE_PRCREATE_FAILS counter, like FAKE_BUMP_FAILS) and the
# release-branch git push (occurrence-steered in the fake git: ph_branch's push is #1,
# release_pr_apply's is #2+).
# (a) transient create failure: consult exactly once, retry, recover.
mkscn maskedexit
r_env=(FAKE_PRCREATE_FAILS=1 FAKE_AI=retry)
rrun v0.5.1
exit_is     "PR100-R4b: a failed gh pr create is not masked into phase success"          0
count_is    "PR100-R4b: the failure consulted the AI exactly once"                       "$SD/askai.log" 1
val_is      "PR100-R4b: the apply re-ran after the consult (create attempted twice)"     "$SD/prcreate.count" '2'
jq_is       "PR100-R4b: the consultation is recorded for phase release-pr"               "$(state)" '[.consultations[]? | select(.phase == "release-pr")] | length' '1'
fhas        "PR100-R4b: the consultation names its phase"                                "$SD/askai.log" 'release-pr'
jq_is       "PR100-R4b: release-pr then legitimately done"                               "$(state)" '.phases["release-pr"].done' 'true'
exists      "PR100-R4b: the recovered run still reaches the draft"                       "$SD/ghstate/release-v0.5.1.json"
# (b) persistent create failure: the consult-retry loop runs to the cap, then stops.
mkscn maskedexitcap
r_env=(FAKE_PRCREATE_FAILS=99 FAKE_AI=retry)
rrun v0.5.1
exit_nonzero "PR100-R4b: a persistently failing gh pr create stops (no silent success)"
val_is      "PR100-R4b: three apply attempts through the consult loop"                   "$SD/prcreate.count" '3'
count_is    "PR100-R4b: consultations capped at 2 per phase"                             "$SD/askai.log" 2
has         "PR100-R4b: the stop is the consultation-cap stop, not a generic no-PR stop" 'still failing after 2 consultations'
# (c) the finding's other command: a failing git push in release_pr_apply. Same contract,
# proved on the push path — today the masked push failure even ends in a full "success".
mkscn maskedexitpush
r_env=(FAKE_PUSH_FAIL_AT=2 FAKE_AI=retry)
rrun v0.5.1
exit_nonzero "PR100-R4b: a failing git push in release_pr_apply is not masked into success"
match_is    "PR100-R4b: the release-branch push is retried through the loop (1 branch + 3 apply)" "$SD/git.log" '^push( -q)? origin release/v0\.5\.1$' 4
count_is    "PR100-R4b: the push failure consults the AI (capped at 2)"                  "$SD/askai.log" 2
has         "PR100-R4b: the failure surfaces as the consultation stop"                   'still failing after 2 consultations'
jq_is       "PR100-R4b: release-pr is NOT marked done on a masked push failure"          "$(state)" '.phases["release-pr"].done' 'false'
match_is    "PR100-R4b: no draft is created through a masked failure"                    "$SD/gh.log" 'release create' 0
fhas        "PR100-R4b: the stop is visible on the checklist issue"                      "$SD/comments.log" 'STOP at phase release-pr'

# ================================================================ PR #100 round 5: 2 ACTIVE findings
# (R5a) find_pr is state-blind (release.sh:160-163): real `gh pr list --head X` with no
# --state lists only OPEN PRs, so every path that must SEE the merged release PR —
# ph_branch's group-skip, ph_review's settled gate, ph_merge's re-query and resume,
# ph_tag's derivation — finds nothing against the real CLI. The fake gh now defaults pr
# list to open and honors --state all|merged|open|closed exactly; MERGED PRs are seeded
# (and merged by the fake gh pr merge) in GitHub's real shape: the merge commit exists
# ONLY in origin until a local git fetch installs it.
# (R5b) merge_sha_of/ph_tag (release.sh:566-608): the mergeCommit gh reports lives on the
# remote; `git tag -a $tag $msha` fails locally ("Failed to resolve ... as a valid ref")
# unless the run fetches first. The tag phase must ensure the merge commit is locally
# present before creating the annotated ref. (The ph_branch half of this pin is the
# existing `mergedpr` scenario above, which this round's realism turns RED.)
peel_is() { # name want-sha — the local annotated tag peels to want
  local got; got="$($RG -C "$R" rev-parse -q --verify 'v0.5.1^{}' 2>/dev/null || true)"
  if [[ "$got" == "$2" ]]; then echo "ok   $1"; else echo "FAIL $1 (v0.5.1^{} is '${got:0:7}', wanted '${2:0:7}')"; fail=1; fi }
unresolved_is() { # name sha — premise: the object is NOT in the local clone (cat-file,
  # not rev-parse: rev-parse resolves any well-formed full sha, present or not)
  if $RG -C "$R" cat-file -e "$2" 2>/dev/null; then
    echo "FAIL $1 (${2:0:7} is already local — the harness premise broke)"; fail=1
  else
    echo "ok   $1"
  fi }
fetched_before_tag() { # name — a fetch (or pull) is recorded before the first annotated tag
  local f t
  f="$(grep -nE '^(fetch|pull)' "$SD/git.log" 2>/dev/null | head -1 | cut -d: -f1)"
  t="$(grep -nE '^tag -a ' "$SD/git.log" 2>/dev/null | head -1 | cut -d: -f1)"
  if [[ -n "$f" && -n "$t" && "$f" -lt "$t" ]]; then echo "ok   $1"
  else echo "FAIL $1 (fetch/pull at line ${f:-none}, tag -a at line ${t:-none} of git.log)"; fail=1; fi }

# (R5a-review) resume with the PR MERGED and its branch deleted: state stops just before
# the review gate. The gate must settle via the MERGED PR (find_pr asking the right
# state), never re-merge, and record the merge commit gh reports.
mkscn mergedvisreview
$RG -C "$R" checkout -q -b release/v0.5.1
printf '102\n' >"$SD/ghstate/seq"
printf 'Release checklist: v0.5.1\nopen\n' >"$SD/ghstate/issue-102"
seed_merged_pr 101
seed_state release-pr
rrun v0.5.1 --publish
exit_is          "PR100-R5a: ph_review resume settles on the MERGED PR"              0
has              "PR100-R5a: the gate is settled by the recorded merge"              'already merged'
match_is         "PR100-R5a: a merged PR is never merged again"                      "$SD/gh.log" 'pr merge' 0
jq_is            "PR100-R5a: review marked done by the settle"                       "$(state)" '.phases.review.done' 'true'
jq_is            "PR100-R5a: merge marked done from the merged PR"                   "$(state)" '.phases.merge.done' 'true'
jq_is            "PR100-R5a: mergeSha is the merge commit, not the PR head"          "$(state)" '.mergeSha' "$(cat "$SD/merge.sha")"

# (R5a-merge) the same resume one phase later: state stops just before merge. ph_merge
# must ADOPT the MERGED PR (no second merge) and derive the merge commit gh reports.
mkscn mergedvismerge
$RG -C "$R" checkout -q -b release/v0.5.1
printf '102\n' >"$SD/ghstate/seq"
printf 'Release checklist: v0.5.1\nopen\n' >"$SD/ghstate/issue-102"
seed_merged_pr 101
seed_state review
rrun v0.5.1 --publish
exit_is          "PR100-R5a: ph_merge resume adopts the MERGED PR"                   0
match_is         "PR100-R5a: no second merge of an already-merged PR"                "$SD/gh.log" 'pr merge' 0
jq_is            "PR100-R5a: merge marked done by adoption"                          "$(state)" '.phases.merge.done' 'true'
jq_is            "PR100-R5a: mergeSha is the merge commit"                           "$(state)" '.mergeSha' "$(cat "$SD/merge.sha")"
peel_is          "PR100-R5b: the tag lands on that merge commit"                     "$(cat "$SD/merge.sha")"
fetched_before_tag "PR100-R5b: the merge commit is fetched before tagging"

# (R5b-resume) ph_merge already recorded mergeSha (its own postcondition) and the run
# died before ph_tag; the merge commit exists only in origin. Tagging must ensure local
# presence (fetch) before creating the annotated ref.
mkscn tagresume
$RG -C "$R" checkout -q -b release/v0.5.1
printf '102\n' >"$SD/ghstate/seq"
printf 'Release checklist: v0.5.1\nopen\n' >"$SD/ghstate/issue-102"
seed_merged_pr 101
seed_state merge
unresolved_is    "PR100-R5b: premise: the merge commit is NOT local before the run"   "$(cat "$SD/merge.sha")"
rrun v0.5.1 --publish
exit_is          "PR100-R5b: ph_tag fetches the merge commit and completes"           0
peel_is          "PR100-R5b: the annotated tag is created at the merge commit"        "$(cat "$SD/merge.sha")"
jq_is            "PR100-R5b: tag marked done"                                         "$(state)" '.phases.tag.done' 'true'
fetched_before_tag "PR100-R5b: the fetch precedes git tag -a in git.log"

# (R5b-live) the full workflow: the merge happens mid-run (the fake gh pr merge lands it
# in origin only); by the time ph_tag runs the merge commit must be local (fetched) and
# the tag must land on IT — the merge commit — not on the PR head the old fake reported.
mkscn mergefetch
rrun v0.5.1
exit_is          "PR100-R5b: the workflow fetches its own merge commit and completes" 0
prf="$(ls "$SD"/ghstate/pr-* | sed -n 1p)"
if [[ "$(sed -n 3p "$prf")" != "$(sed -n 4p "$prf")" ]]; then
  echo "ok   PR100-R5b: premise: the fake merge produced a distinct merge commit"
else
  echo "FAIL PR100-R5b: premise: merge commit == PR head (harness broke)"; fail=1
fi
peel_is          "PR100-R5b: the tag marks the MERGE commit the workflow created"     "$(sed -n 4p "$prf")"
jq_is            "PR100-R5b: state records that merge commit as mergeSha"             "$(state)" '.mergeSha' "$(sed -n 4p "$prf")"
fetched_before_tag "PR100-R5b: the fetch precedes the annotation in the live flow"
exists           "PR100-R5b: the run still reaches the draft"                         "$SD/ghstate/release-v0.5.1.json"

# ================================================================ PR #100 round 6: 1 ACTIVE finding
# (R6) "Resume after state loss" (finding e989fe9ce3ec ACTIVE, release.sh:411-418):
# with state.json absent AFTER a completed release, ph_changelog is not phase_done and
# re-runs its gate against the ALREADY-RESTRUCTURED CHANGELOG — restructure_apply moved
# [Unreleased]'s content under `## [X.Y.Z] - date`, leaving [Unreleased] empty, so the
# Known Issues gate finds no bullet and no none-marker inside it and SPURIOUSLY STOPS.
# That breaks the state-is-a-cache / postcondition-first resume promise (release.sh's
# own header: "absent state re-derives ... re-running a finished phase is a no-op"; R6).
# The re-run must instead recognize the changelog/restructure postconditions from repo
# state (the `## [X.Y.Z]` section, the bumped version files, the merged PR, the tag):
# no spurious stop, no re-run of the gate, a no-op/continuation through the finished
# phases. The scenario completes a real release first — the realistic
# release-check-version.sh fake makes run 1 genuinely restructure — asserts every
# postcondition premise, then deletes state.json and re-runs.
mkscn stateloss
rrun v0.5.1
exit_is     "PR100-R6: setup: the release completes (default posture)"               0
# Premises, asserted not assumed — if one breaks it is the harness that is wrong.
fhas        "PR100-R6: premise: the CHANGELOG was restructured for the release"      "$R/CHANGELOG.md" '^## \[0\.5\.1\] - '
match_is    "PR100-R6: premise: exactly one released section"                       "$R/CHANGELOG.md" '^## \[0\.5\.1\]' 1
unrel="$(awk '/^## \[Unreleased\]/{u=1;next} u && /^## \[/{u=0} u{print}' "$R/CHANGELOG.md")"
if printf '%s\n' "$unrel" | grep -qE '^[[:space:]]*- |Known Issues|known-issues'; then
  echo "FAIL PR100-R6: premise: [Unreleased] still carries content (harness broke)"; fail=1
else
  echo "ok   PR100-R6: premise: [Unreleased] is empty above the released section"
fi
fhas        "PR100-R6: premise: config.yml says the release version"                "$R/build/config.yml" 'version: "0\.5\.1"'
pn="$(grep -oE '<string>0\.5\.1</string>' "$R/build/darwin/Info.plist" 2>/dev/null | wc -l | tr -d ' ')"
if [[ "$pn" == "2" ]]; then
  echo "ok   PR100-R6: premise: both plist keys say the release version"
else
  echo "FAIL PR100-R6: premise: plist has $pn 0.5.1 strings, wanted 2 (harness broke)"; fail=1
fi
if [[ -n "$($RG -C "$R" ls-remote origin refs/tags/v0.5.1 2>/dev/null)" ]]; then
  echo "ok   PR100-R6: premise: the tag is on origin"
else
  echo "FAIL PR100-R6: premise: the tag is missing on origin (harness broke)"; fail=1
fi
prm="$(sed -n 1p "$(ls "$SD"/ghstate/pr-* | sed -n 1p)")"
if [[ "$prm" == "MERGED" ]]; then
  echo "ok   PR100-R6: premise: the release PR is MERGED"
else
  echo "FAIL PR100-R6: premise: PR state is '$prm' (harness broke)"; fail=1
fi
count_is    "PR100-R6: premise: the changelog gate ran exactly once in run 1"       "$SD/changelog.log" 1
# The defect: state.json ABSENT after the completed release.
rm -f "$(state)"
rrun v0.5.1 --publish
exit_is     "PR100-R6: a stateloss re-run is a no-op continuation, not a stop"      0
hasnt       "PR100-R6: no spurious Known-Issues stop"                               'no curated Known Issues'
hasnt       "PR100-R6: no stop at the changelog phase at all"                       'phase .changelog.'
count_is    "PR100-R6: the changelog gate is NOT re-run (postcondition recognized)" "$SD/changelog.log" 1
val_is      "PR100-R6: bump is not re-run (the group-skip re-derives it)"           "$SD/bump.count" 1
match_is    "PR100-R6: restructure is not re-applied (one released section stands)"  "$R/CHANGELOG.md" '^## \[0\.5\.1\]' 1
match_is    "PR100-R6: no second release PR"                                        "$SD/gh.log" 'pr create' 1
match_is    "PR100-R6: no second annotated tag"                                     "$SD/git.log" '^tag -a v0\.5\.1' 1
match_is    "PR100-R6: no second draft/release create"                              "$SD/gh.log" 'release create' 1
match_is    "PR100-R6: no second publish"                                           "$SD/gh.log" 'release edit' 1
jq_is       "PR100-R6: changelog re-derived done from repo state"                   "$(state)" '.phases.changelog.done' 'true'
jq_is       "PR100-R6: the rebuilt state reaches closeout"                          "$(state)" '.phases.closeout.done' 'true'

# Round 7 D: the same absent-state walk, per phase group (the stateloss pattern
# generalized — each group's done-ness derivation from repo state, asserted one by one;
# the tag group's standalone absent-state case is tagpush, the branch group's standalone
# cases are leftover/mergedpr, and the changelog/bump/restructure/pr/publish rows above
# already carry their groups). These four rows close the set:
has         "PR100-R6/D: branch group skips phases 3-6 via the merged PR"     'skipping phases 3-6'
has         "PR100-R6/D: review group settles on the merge"                   'settled by its merge'
match_is    "PR100-R6/D: merge group: the merged PR is not re-merged"        "$SD/gh.log" 'pr merge' 1
count_is    "PR100-R6/D: local group: the build is not re-run (4 assets)"    "$SD/local.log" 1

# ================================================================ round 7 C2: the stubs actually trip
# Prove the fail-by-default stubs refuse when nothing steers them — invoke each directly
# with no steering and assert the 127 + "UNEXPECTED REAL <tool>" propagates, the trip is
# recorded, and the hardened git refuses a network subverb aimed at a NON-local remote
# while local object operations still pass through. A scenario that forgot to steer would
# fail its own rows exactly this way, and the norealbins finale below scans every
# scenario's unexpected.log for the same trips. The probes live outside $tmp/*.sd and are
# reset (rm -rf) so the deliberate trips never feed the finale.
PSD="$tmp/stubtrip-probe"; rm -rf "$PSD"; mkdir -p "$PSD"
o="$(FAKE_STATE_DIR="$PSD" "$FB/opencode" run --format json probe 2>&1)"; c=$?
if [[ "$c" -eq 127 ]] && printf '%s' "$o" | grep -q 'UNEXPECTED REAL opencode'; then
  echo "ok   R7/stubtrip: the opencode stub refuses unsteered (127 + UNEXPECTED REAL)"
else
  echo "FAIL R7/stubtrip: the opencode stub did not refuse (exit $c: $o)"; fail=1
fi
o="$(FAKE_STATE_DIR="$PSD" "$FB/curl" -fsSL https://example.invalid 2>&1)"; c=$?
if [[ "$c" -eq 127 ]] && printf '%s' "$o" | grep -q 'UNEXPECTED REAL curl'; then
  echo "ok   R7/stubtrip: the curl stub refuses unsteered (127 + UNEXPECTED REAL)"
else
  echo "FAIL R7/stubtrip: the curl stub did not refuse (exit $c: $o)"; fail=1
fi
o="$(FAKE_STATE_DIR="$PSD" "$FB/codesign" --verify Kai.app 2>&1)"; c=$?
if [[ "$c" -eq 127 ]] && printf '%s' "$o" | grep -q 'UNEXPECTED REAL codesign'; then
  echo "ok   R7/stubtrip: the codesign stub refuses unsteered (127 + UNEXPECTED REAL)"
else
  echo "FAIL R7/stubtrip: the codesign stub did not refuse (exit $c: $o)"; fail=1
fi
mkdir -p "$PSD/repo"
$RG -C "$PSD/repo" init -q -b master
$RG -C "$PSD/repo" remote add origin https://example.invalid/x.git
o="$(cd "$PSD/repo" && env PATH="$FB:$PATH" FAKE_STATE_DIR="$PSD" git push origin master 2>&1)"; c=$?
if [[ "$c" -eq 127 ]] && printf '%s' "$o" | grep -q 'UNEXPECTED REAL git'; then
  echo "ok   R7/stubtrip: the git wrapper refuses a network subverb to a non-local remote"
else
  echo "FAIL R7/stubtrip: the git wrapper let a non-local network op through (exit $c: $o)"; fail=1
fi
o="$(cd "$PSD/repo" && env PATH="$FB:$PATH" FAKE_STATE_DIR="$PSD" git rev-parse --git-dir 2>&1)"; c=$?
if [[ "$c" -eq 0 ]]; then
  echo "ok   R7/stubtrip: local git object operations still pass through"
else
  echo "FAIL R7/stubtrip: local git object op refused (exit $c: $o)"; fail=1
fi
if [[ -s "$PSD/unexpected.log" ]]; then
  echo "ok   R7/stubtrip: the deliberate trips were recorded for the finale scan"
else
  echo "FAIL R7/stubtrip: the trips left no record in unexpected.log"; fail=1
fi
rm -rf "$PSD"   # reset

# ================================================================ round 7 E: /bin/bash 3.2 compatibility
# release.sh's shebang is `#!/usr/bin/env bash`; on a stock macOS that resolves to
# bash 3.2 (/bin/bash), not a brew bash 5. Run one whole scenario with the interpreter
# pinned to /bin/bash: the full flow (every phase function, the recovery loop, the traps)
# must complete under 3.2. (The harness files themselves are verified by running the
# whole suite under /bin/bash — see handoff-T-rework7.md for that evidence.)
mkscn b32
out="$(cd "$R" && env PATH="$FB:$PATH" FAKE_STATE_DIR="$SD" FAKE_FIXTURES="$FX" \
  /bin/bash "$release" v0.5.1 </dev/null 2>&1)"
code=$?
exit_is     "R7/b32: the full release flow runs under /bin/bash (3.2)"      0
exists      "R7/b32: the run reaches the draft"                            "$SD/ghstate/release-v0.5.1.json"
jq_is       "R7/b32: state records the completed default posture"          "$(state)" '.phases.smoke.done' 'true'

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

# ================================================================ round 7 C1 finale: no unsteered real binary
# Any trip of a fail-by-default stub (opencode/curl/codesign) or of the hardened git's
# network refusal, in ANY scenario, is a failing case (stubtrip's deliberate probes are
# reset above and live outside $tmp/*.sd, so a hit here means a real escape reached for
# a real binary — the class that let a real `opencode run` fire from inside a test).
nhit=0
for f in "$tmp"/*.sd/unexpected.log; do
  [[ -f "$f" ]] || continue
  while IFS= read -r line; do
    echo "FAIL R7/norealbins: unsteered real-binary reach in $(basename "${f%.sd.*}"): $line"
    fail=1; nhit=1
  done <"$f"
done
if [[ "$nhit" -eq 0 ]]; then
  echo "ok   R7/norealbins: no unsteered real-binary reach in any scenario"
fi

[[ $fail -eq 0 ]] && echo "PASS" || { echo "FAILED"; exit 1; }
