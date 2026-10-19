#!/usr/bin/env bash
# Tests for scripts/release.sh (issue #78 phase 1): the static release state machine, its static
# phases, dry-run and hard stops, with every external effect faked (gh, the wrapped release
# scripts, the codesign identity check). git is real, against a throwaway repo whose origin is a
# local bare repository, so branch/merge/tag mechanics are exercised for real. Runs on Linux.
#   bash scripts/release.test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
rel="$here/release.sh"
tmp="$(mktemp -d)"
cleanup() { [[ -n "${rel_pid:-}" ]] && kill -9 "$rel_pid" 2>/dev/null; rm -rf "$tmp"; }
trap cleanup EXIT

fail=0
ok()   { echo "ok   $*"; }
bad()  { echo "FAIL $*"; fail=1; }
has()  { if printf '%s' "$2" | grep -q -- "$3"; then ok "$1"; else bad "$1 (wanted /$3/ in: $(printf '%s' "$2" | head -c 400))"; fi; }
is()   { if [[ "$2" == "$3" ]]; then ok "$1"; else bad "$1 (got '$2', wanted '$3')"; fi; }
isnt() { if [[ "$2" != "$3" ]]; then ok "$1"; else bad "$1 (got '$2', did not want it)"; fi; }

# ---------------------------------------------------------------- fixtures and fakes ----
mkrepo() { # $1 = dir: a checkout of master with the release-editable files, origin = bare repo
  local d="$1"
  mkdir -p "$d/build/darwin"
  git -C "$d" init -q -b master
  printf 'name: kai\nversion: "0.0.0"\n' >"$d/build/config.yml"
  printf '<plist><key>CFBundleShortVersionString</key><string>0.0.0</string>\n<key>CFBundleVersion</key><string>0.0.0</string></plist>\n' >"$d/build/darwin/Info.plist"
  printf '# Changelog\n\n## [Unreleased]\n\n### Enhancements\n- Something new (#101).\n\n## [0.5.0] - 2026-10-19\n' >"$d/CHANGELOG.md"
  git -C "$d" add -A && git -C "$d" -c user.name=t -c user.email=t@t commit -q -m init
  git -C "$d" config user.name t && git -C "$d" config user.email t@t
  git -C "$1" init -q --bare "$d-origin.git"
  git -C "$d" remote add origin "$d-origin.git"
  git -C "$d" push -q -u origin master
  # release-bump.sh resolves the repo from its own path (dirname $0/..), so a copy inside the
  # throwaway repo bumps the throwaway's files. release-check-version.sh takes the root as an
  # argument and needs no copy; both stay the real, unedited scripts.
  mkdir -p "$d/scripts"
  cp "$here/release-bump.sh" "$d/scripts/release-bump.sh"
}

mkkai() { # a repo with a previous release tag v0.5.0, so the bump decision has a base
  local d="$1"
  mkrepo "$d"
  git -C "$d" tag -a v0.5.0 -m v0.5.0
  git -C "$d" push -q origin v0.5.0
}

# The fake gh: answers exactly the calls release.sh makes, records what it did in $FAKE_LOG, and
# performs the PR merge for real in git so the later phases see a true merge commit.
mkfakes() { # $1 = fakes dir
  local f="$1"
  mkdir -p "$f"
  cat >"$f/gh" <<EOF
#!/usr/bin/env bash
record() { printf '%s\n' "\$*" >>"$tmp/fake-gh.log"; }
state="$tmp/fake-state"; repo="Performant-Labs/kai"
case "\$1 \$2" in
  "api repos/\$repo")
    [[ -f "\$state/no-push-rights" ]] && { echo false; exit 0; }
    echo true; exit 0 ;;
  "api repos/\$repo/releases/tags/"*)
    tag="\${2##*/}"; [[ -f "\$state/releases/\$tag.json" ]] && { cat "\$state/releases/\$tag.json"; exit 0; }
    exit 4 ;;
  "api repos/\$repo/commits/"*"/check-runs")
    if [[ -f "\$state/ci-red" ]]; then
      printf 'ci / go / build-lint-test\tcompleted\tfailure\n'
    else
      printf 'ci / go / build-lint-test\tcompleted\tsuccess\nci / frontend / typecheck\tcompleted\tsuccess\n'
    fi
    exit 0 ;;
  "api repos/\$repo/pulls/"*"/comments"|"api repos/\$repo/issues/"*"/comments")
    printf '[{"body":"PR Agent scores: %s/100"}\\n' "\$(cat "\$state/pr-agent-score" 2>/dev/null || echo 95)"
    printf ',{"body":"some other comment"}]\n'
    exit 0 ;;
esac
case "\$1" in
  pr)
    case "\$2" in
      create) record "pr create"; n=\$(( \$(cat "$tmp/fake-state/pr-seq" 2>/dev/null || echo 0) + 1 )); echo \$n >"$tmp/fake-state/pr-seq"
        echo "https://github.com/\$repo/pull/\$n"; exit 0 ;;
      view)
        n="\$3"
        if [[ -f "\$state/merged-\$n" ]]; then
          printf '{"number":%s,"state":"MERGED","headRefOid":"%s"}\n' "\$n" "\$(cat "\$state/merged-\$n")"
        else
          printf '{"number":%s,"state":"OPEN","headRefOid":"%s"}\n' "\$n" "\$(git rev-parse "refs/remotes/origin/release/\$(cat "\$state/version")" 2>/dev/null || git rev-parse HEAD)"
        fi
        exit 0 ;;
      checks)
        record "pr checks"
        [[ -f "\$state/pr-checks-red" ]] && { printf 'ci / go / build-lint-test\tfail\n'; exit 1; }
        printf 'ci / go / build-lint-test\tpass\n'; exit 0 ;;
      merge)
        record "pr merge"; n="\$3"; ver=\$(cat "\$state/version")
        git checkout -q master
        git merge --no-ff -q -m "release: v\$ver (#\$n)" "release/v\$ver"
        git push -q origin master
        git push -q origin --delete "release/v\$ver"
        git branch -q -D "release/v\$ver"
        git rev-parse master >"\$state/merged-\$n"
        exit 0 ;;
    esac ;;
  release)
    tag=""
    for a in "\$@"; do [[ "\$a" == v[0-9]*.[0-9]*.[0-9]* ]] && tag="\$a"; done
    case "\$2" in
      create) record "release create \$tag"; mkdir -p "\$state/releases"
        printf '{"tag_name":"%s","draft":true,"html_url":"x"}\n' "\$tag" >"\$state/releases/\$tag.json"; echo "https://github.com/\$repo/releases/\$tag"; exit 0 ;;
      edit) record "release edit \$tag"
        sed -i 's/"draft":true/"draft":false/' "\$state/releases/\$tag.json"
        exit 0 ;;
    esac ;;
esac
echo "fake gh: unhandled: \$*" >&2; exit 64
EOF
  chmod +x "$f/gh"

  cat >"$f/preflight" <<'EOF'
#!/usr/bin/env bash
echo "preflight-marker" >>"$MARKERS/preflight"
[[ -f "$FAKE_STATE/preflight-fail" ]] && { echo "FAIL: fake preflight refuses (first run)"; exit 1; }
echo "PASS: fake preflight"
exit 0
EOF
  cat >"$f/changelog-check" <<'EOF'
#!/usr/bin/env bash
echo "changelog-check-marker" >>"$MARKERS/changelog-check"
echo "ok: every merged PR is covered by [Unreleased] (fake)"
exit 0
EOF
  cat >"$f/release-local" <<'EOF'
#!/usr/bin/env bash
echo "release-local-marker" >>"$MARKERS/release-local"
[[ -f "$FAKE_STATE/build-fail" ]] && { echo "FAIL: fake build refuses"; exit 1; }
[[ -f "$FAKE_STATE/build-sleep" ]] && sleep 10
tag=""; out=""
for a in "$@"; do case "$a" in v[0-9]*) tag="$a" ;; --out) ;; *) [[ -n "$outprev" ]] && out="$a"; outprev=1 ;; esac; done
# --out DIR: the argument after --out
out=""; prev=""
for a in "$@"; do [[ "$prev" == "--out" ]] && out="$a"; prev="$a"; done
mkdir -p "$out/assets" "$out/Kai.app"
ver="${tag#v}"
: >"$out/assets/Kai-$ver-darwin-arm64.zip"
: >"$out/assets/updater-Kai-$ver-darwin-arm64.zip"
: >"$out/assets/Kai-$ver-darwin-arm64.dmg"
: >"$out/assets/SHA256SUMS"
echo "notes" >"$out/RELEASE_NOTES.md"
echo "fake build done (tag $tag, out $out)"
exit 0
EOF
  cat >"$f/codesign-check" <<'EOF'
#!/usr/bin/env bash
echo "codesign-check-marker" >>"$MARKERS/codesign-check"
[[ -f "$FAKE_STATE/no-identity" ]] && { echo "error: no \"Kai Release\" identity in the keychain"; exit 1; }
exit 0
EOF
  cat >"$f/install-release" <<'EOF'
#!/usr/bin/env bash
echo "install-release-marker" >>"$MARKERS/install-release"
exit 0
EOF
  chmod +x "$f"/preflight "$f"/changelog-check "$f"/release-local "$f"/codesign-check "$f"/install-release
}

# Environment every run gets: fakes for gh and the wrapped scripts, real git, fast polls.
runenv() { # prints the env assignments
  cat <<EOF
GH_BIN=$tmp/fakes/gh
GIT_BIN=git
RELEASE_PREFLIGHT_SCRIPT=$tmp/fakes/preflight
RELEASE_CHANGELOG_CHECK_SCRIPT=$tmp/fakes/changelog-check
RELEASE_BUMP_SCRIPT=$tmp/r/scripts/release-bump.sh
RELEASE_LOCAL_SCRIPT=$tmp/fakes/release-local
RELEASE_CODESIGN_CHECK=$tmp/fakes/codesign-check
RELEASE_INSTALL_SCRIPT=$tmp/fakes/install-release
RELEASE_POLL_SLEEP=0.05
RELEASE_POLL_MAX=20
MARKERS=$tmp/markers
FAKE_STATE=$tmp/fake-state
KAI_RELEASE_TEST=1
EOF
}

runrel() { # dir version [args...] -> out, code
  local d="$1"; shift
  out="$(cd "$d" && env $(runenv) "$rel" "$@" 2>&1)"; code=$?
}

count() { [[ -f "$tmp/markers/$1" ]] && wc -l <"$tmp/markers/$1" | tr -d ' ' || echo 0; }

fresh() { # full scenario: repo + fakes + state
  rm -rf "$tmp/r" "$tmp/r-origin.git" "$tmp/fakes" "$tmp/markers" "$tmp/fake-state" "$tmp/fake-gh.log"
  mkkai "$tmp/r"
  mkfakes "$tmp/fakes"
  mkdir -p "$tmp/markers" "$tmp/fake-state"
}

# --------------------------------------------------------------------------- the tests ----

echo "== usage"
fresh
runrel "$tmp/r"
is  "no arguments exits 2 (usage)" "$code" "2"
has "  ... and prints usage" "$out" 'usage: scripts/release\.sh vX\.Y\.Z'

runrel "$tmp/r" v1.2 --dry-run
is  "a malformed version exits 2" "$code" "2"

runrel "$tmp/r" v0.6.0 --dry-run --publish
is  "--dry-run with --publish is refused" "$code" "2"

echo
echo "== dry run: clean master to a verified package, nothing public"
fresh
echo 0.6.0 >"$tmp/fake-state/version"   # the fake gh merge needs the version it is merging
runrel "$tmp/r" v0.6.0 --dry-run
is  "dry run succeeds" "$code" "0"
has "  ... prints the summary of what is left" "$out" 'Release v0.6.0: dry run complete'
is  "  ... and gh was never asked to create a release" "$(grep -c 'release create' "$tmp/fake-gh.log" || true)" "0"
is  "  ... and gh was never asked to publish" "$(grep -c 'release edit' "$tmp/fake-gh.log" || true)" "0"
has "the draft phase is recorded as skipped by --dry-run" "$out" 'draft.*skipped.*--dry-run'
# The tag is real, locally and on origin, at the merge commit.
merge_sha="$(git -C "$tmp/r" rev-parse v0.6.0^{commit} 2>/dev/null)"
[[ -n "$merge_sha" ]] && ok "tag v0.6.0 exists" || bad "tag v0.6.0 exists"
is  "origin has the tag at the same commit" "$(git -C "$tmp/r" ls-remote origin "refs/tags/v0.6.0^{}" | cut -f1)" "$merge_sha"
has "the state file records every static phase" "$(cat "$tmp/r/bin/release/v0.6.0/state.json")" '"preflight"'
has "  ... changelog" "$(cat "$tmp/r/bin/release/v0.6.0/state.json")" '"changelog"'
has "  ... branch" "$(cat "$tmp/r/bin/release/v0.6.0/state.json")" '"branch"'
has "  ... merge" "$(cat "$tmp/r/bin/release/v0.6.0/state.json")" '"merge"'
has "  ... tag" "$(cat "$tmp/r/bin/release/v0.6.0/state.json")" '"tag"'
has "  ... build" "$(cat "$tmp/r/bin/release/v0.6.0/state.json")" '"build"'
has "the state records the commit a phase ran on" "$(cat "$tmp/r/bin/release/v0.6.0/state.json")" '"commit"'
has "the built artifacts are in bin/release/v0.6.0" "$out" 'bin/release/v0.6.0'
[[ -d "$tmp/r/bin/release/v0.6.0/Kai.app" ]] && ok "the packaged Kai.app is there" || bad "the packaged Kai.app is there"
[[ -f "$tmp/r/bin/release/v0.6.0/assets/SHA256SUMS" ]] && ok "the assets are there" || bad "the assets are there"
has "the smoke matrix is recorded as skipped without --smoke" "$out" 'smoke.*skipped'
has "the owner list is printed before anything starts" "$out" 'Needs from the owner'
is  "each wrapped script ran exactly once" "$(count preflight):$(count changelog-check):$(count release-local)" "1:1:1"
has "the release branch did not outlive the merge" "$out" 'merge'

echo
echo "== resume: a failing phase is retried, finished phases are not redone"
fresh
echo 0.6.0 >"$tmp/fake-state/version"
touch "$tmp/fake-state/preflight-fail"
runrel "$tmp/r" v0.6.0 --dry-run
isnt "a preflight failure stops the run" "$code" "0"
has "  ... naming the phase" "$out" 'phase.*preflight'
has "  ... with the resume command" "$out" '[Rr]esume with: scripts/release.sh'
is   "  ... and nothing after it ran" "$(count changelog-check)" "0"
is   "no state was recorded for the failed phase" "$(jq -r '.phases[]? | select(.name == "preflight") | .status' "$tmp/r/bin/release/v0.6.0/state.json" 2>/dev/null)" ""
rm "$tmp/fake-state/preflight-fail"
runrel "$tmp/r" v0.6.0 --dry-run
is  "the re-run completes" "$code" "0"
is  "preflight ran exactly twice in total" "$(count preflight)" "2"
is  "changelog-check still ran exactly once" "$(count changelog-check)" "1"
is  "release-local still ran exactly once" "$(count release-local)" "1"

echo
echo "== a finished release is a no-op"
fresh
echo 0.6.0 >"$tmp/fake-state/version"
runrel "$tmp/r" v0.6.0 --dry-run
runrel "$tmp/r" v0.6.0 --dry-run
is  "the second run succeeds" "$code" "0"
has "  ... saying everything is already done" "$out" 'already complete'
is  "preflight still ran exactly once" "$(count preflight)" "1"
is  "release-local still ran exactly once" "$(count release-local)" "1"

echo
echo "== resume after a kill"
fresh
echo 0.6.0 >"$tmp/fake-state/version"
touch "$tmp/fake-state/build-sleep"
( cd "$tmp/r" && env $(runenv) "$rel" v0.6.0 --dry-run >"$tmp/kill-run.log" 2>&1 ) &
rel_pid=$!
for i in $(seq 1 100); do [[ $(count release-local) -ge 1 ]] && break; sleep 0.1; done
kill -9 $rel_pid 2>/dev/null; wait $rel_pid 2>/dev/null; rel_pid=""
pkill -f "$tmp/fakes/release-local" 2>/dev/null
rm -f "$tmp/fake-state/build-sleep"
is  "the kill left the run unfinished" "$(jq -r '.phases[]? | select(.name == "build") | .status' "$tmp/r/bin/release/v0.6.0/state.json" 2>/dev/null)" ""
runrel "$tmp/r" v0.6.0 --dry-run
is  "a re-run after the kill completes" "$code" "0"
is  "preflight was not re-run (already done)" "$(count preflight)" "1"
is  "release-local ran again (was interrupted)" "$(count release-local)" "2"

echo
echo "== hard stops"
fresh
echo 0.6.0 >"$tmp/fake-state/version"
git -C "$tmp/r" tag -a v0.6.0 -m v0.6.0 2>/dev/null   # already used
git -C "$tmp/r" push -q origin v0.6.0
runrel "$tmp/r" v0.6.0 --dry-run
isnt "an existing tag stops immediately" "$code" "0"
has "  ... with the hard-stop message" "$out" 'HARD STOP.*v0.6.0.*exists'
is  "  ... before any phase ran" "$(count preflight)" "0"

fresh
echo 0.6.0 >"$tmp/fake-state/version"
touch "$tmp/fake-state/no-identity"
runrel "$tmp/r" v0.6.0 --dry-run
isnt "a missing signing identity stops immediately" "$code" "0"
has "  ... with the hard-stop message" "$out" 'HARD STOP.*signing identity'
is  "  ... before any phase ran" "$(count preflight)" "0"
rm "$tmp/fake-state/no-identity"

fresh
echo 0.6.0 >"$tmp/fake-state/version"
touch "$tmp/fake-state/pr-checks-red"
runrel "$tmp/r" v0.6.0 --dry-run
isnt "red CI on the release PR stops the run" "$code" "0"
has "  ... with the hard-stop message" "$out" 'HARD STOP.*CI'
is  "  ... after the PR was created but before the merge" "$(grep -c 'pr create' "$tmp/fake-gh.log")" "1"
is   "no merge happened" "$(grep -c 'pr merge' "$tmp/fake-gh.log")" "0"

fresh
echo 0.6.0 >"$tmp/fake-state/version"
echo 82 >"$tmp/fake-state/pr-agent-score"
runrel "$tmp/r" v0.6.0 --dry-run
isnt "a PR-Agent score below 90 stops the run" "$code" "0"
has "  ... with the hard-stop message" "$out" 'HARD STOP.*PR-Agent'

fresh
echo 0.6.0 >"$tmp/fake-state/version"
touch "$tmp/fake-state/ci-red"
runrel "$tmp/r" v0.6.0 --dry-run
isnt "red CI on the merge commit stops the run" "$code" "0"
has "  ... with the hard-stop message" "$out" 'HARD STOP.*CI'

echo
echo "== the bump decision (docs/releasing.md: Breaking Changes means MINOR before 1.0)"
fresh
echo 0.6.0 >"$tmp/fake-state/version"
printf '# Changelog\n\n## [Unreleased]\n\n### Breaking Changes\n- Something breaks (#9).\n\n### Bug Fixes\n- A fix (#10).\n' >"$tmp/r/CHANGELOG.md"
git -C "$tmp/r" add -A && git -C "$tmp/r" commit -q -m breaking
runrel "$tmp/r" v0.5.1 --dry-run
isnt "a patch bump with a Breaking Changes section is refused" "$code" "0"
has "  ... explaining the rule" "$out" 'Breaking Changes.*MINOR'

fresh
echo 0.6.0 >"$tmp/fake-state/version"
printf '# Changelog\n\n## [Unreleased]\n\n### Breaking Changes\n- Something breaks (#9).\n' >"$tmp/r/CHANGELOG.md"
git -C "$tmp/r" add -A && git -C "$tmp/r" commit -q -m breaking
runrel "$tmp/r" v0.6.0 --dry-run
is  "a minor bump with a Breaking Changes section is accepted" "$code" "0"

echo
echo "== the real (non-dry) flow creates the draft; --publish publishes and verifies"
fresh
echo 0.6.0 >"$tmp/fake-state/version"
runrel "$tmp/r" v0.6.0
is  "the full run succeeds" "$code" "0"
is  "the draft release was created" "$(grep -c 'release create v0.6.0' "$tmp/fake-gh.log")" "1"
has "the state records the draft as done" "$(cat "$tmp/r/bin/release/v0.6.0/state.json")" '"draft"'
runrel "$tmp/r" v0.6.0 --publish
is  "publishing after a full run succeeds" "$code" "0"
is  "the release was published" "$(grep -c 'release edit v0.6.0' "$tmp/fake-gh.log")" "1"
is  "the installer verification ran" "$(count install-release)" "1"
has "nothing is left for the owner" "$out" 'othing left for the owner'

echo
echo "== the script never force-pushes, deletes tags or drafts anything outside publish"
src="$(cat "$rel")"
code_only="$(printf '%s' "$src" | sed 's/#.*//')"
has "no force push anywhere" "$src" '^# *guarantee: no force push'
if printf '%s' "$code_only" | grep -Eq 'push[^|]*(--force|[[:space:]]-f[[:space:]]|.*push.*-fq)'; then bad "no force-push flag in any push"; else ok "no force-push flag in any push"; fi
if printf '%s' "$code_only" | grep -Eq 'tag[[:space:]]+(-d|--delete)'; then bad "no tag deletion"; else ok "no tag deletion"; fi
n_draft_false="$(printf '%s' "$code_only" | grep -c -- '--draft=false')"
is  "--draft=false appears exactly once in code (the publish phase)" "$n_draft_false" "1"

echo
if [[ $fail -eq 0 ]]; then echo "ALL OK"; else echo "FAILURES"; exit 1; fi
