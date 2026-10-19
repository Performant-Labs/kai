#!/usr/bin/env bash
# The static release workflow (issue #78, phase 1): every phase of docs/releasing.md's step-by-step
# in a fixed order, with fixed rules and no questions. An AI is consulted only when a phase is
# blocked (phases 2 and 3, not this one); this script's failure mode is always a plain stop with
# the phase, the reason and the resume command.
#
#   scripts/release.sh vX.Y.Z [--publish] [--smoke] [--dry-run]
#
#     --dry-run  run everything up to the packaged build, but create no draft release and
#                publish nothing (the branch, the PR, the merge and the tag are real: rehearse
#                against a throwaway version, as phase 4's rehearsal will)
#     --publish  after the draft exists, publish it and run the post-release verification
#     --smoke    record the smoke matrix as to-be-run (phase 1 cannot run it; without the flag
#                it is recorded as skipped)
#
# State and resuming: bin/release/vX.Y.Z/state.json (git-ignored) records every finished phase
# and the commit it ran on. Each phase checks its own postcondition first, so a re-run continues
# where it stopped and a finished phase is a no-op. Nothing is ever redone by hand.
#
# The wrapped scripts (release-preflight.sh, changelog-check.sh, release-bump.sh,
# release-check-version.sh, release-local.sh, codesign-app.sh --check, install-release.sh) are
# CALLED, never modified. Every external tool is overridable so the tests (release.test.sh) can
# fake it; the defaults are the real thing:
#   GH_BIN  GIT_BIN  RELEASE_PREFLIGHT_SCRIPT  RELEASE_CHANGELOG_CHECK_SCRIPT
#   RELEASE_BUMP_SCRIPT  RELEASE_CHECK_VERSION_SCRIPT  RELEASE_LOCAL_SCRIPT
#   RELEASE_CODESIGN_CHECK  RELEASE_INSTALL_SCRIPT
#   RELEASE_POLL_SLEEP (seconds between CI polls, default 30)
#   RELEASE_POLL_MAX   (polls before giving up, default 120)
#   GH_REPO (default Performant-Labs/kai)
#
# guarantee: no force push, no tag deletion, no overwrite of an existing release anywhere in
# this script; the only `--draft=false` is the publish phase, which itself requires the draft
# this workflow created.
set -uo pipefail

repo="${GH_REPO:-Performant-Labs/kai}"
usage() { echo "usage: scripts/release.sh vX.Y.Z [--publish] [--smoke] [--dry-run]" >&2; }

# ------------------------------------------------------------------ arguments ----
tag=""; publish=0; smoke=0; dry=0
for arg in "$@"; do
  case "$arg" in
    --publish) publish=1 ;;
    --smoke) smoke=1 ;;
    --dry-run) dry=1 ;;
    v[0-9]*.[0-9]*.[0-9]*) tag="$arg" ;;
    -h|--help) sed -n '2,/^set -uo/p' "$0" | sed '$d'; exit 0 ;;
    *) echo "unknown argument: $arg" >&2; usage; exit 2 ;;
  esac
done
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { usage; exit 2; }
ver="${tag#v}"
if [[ $dry -eq 1 && $publish -eq 1 ]]; then
  echo "--dry-run and --publish contradict each other: a dry run creates nothing public." >&2
  exit 2
fi

GIT_BIN="${GIT_BIN:-git}"
GH_BIN="${GH_BIN:-gh}"
here="$(cd "$(dirname "$0")" && pwd)"
PREFLIGHT="${RELEASE_PREFLIGHT_SCRIPT:-$here/release-preflight.sh}"
CHANGELOG_CHECK="${RELEASE_CHANGELOG_CHECK_SCRIPT:-$here/changelog-check.sh}"
BUMP="${RELEASE_BUMP_SCRIPT:-$here/release-bump.sh}"
CHECK_VERSION="${RELEASE_CHECK_VERSION_SCRIPT:-$here/release-check-version.sh}"
LOCAL="${RELEASE_LOCAL_SCRIPT:-$here/release-local.sh}"
CODESIGN_CHECK="${RELEASE_CODESIGN_CHECK:-$here/codesign-app.sh}"
INSTALL="${RELEASE_INSTALL_SCRIPT:-$here/install-release.sh}"
POLL_SLEEP="${RELEASE_POLL_SLEEP:-30}"
POLL_MAX="${RELEASE_POLL_MAX:-120}"
identity="${KAI_RELEASE_SIGN_IDENTITY:-Kai Release}"

root="$("$GIT_BIN" rev-parse --show-toplevel 2>/dev/null)" || { echo "FAIL: run inside a checkout of $repo" >&2; exit 1; }
cd "$root"
statedir="$root/bin/release/$tag"
state="$statedir/state.json"
outdir="$statedir"
mkdir -p "$statedir"
[[ -f "$state" ]] || printf '{\n  "version": "%s",\n  "phases": []\n}\n' "$ver" >"$state"
exec > >(tee -a "$statedir/run.log") 2>&1

step()  { printf '\n== phase %s\n' "$*"; }
note()  { printf '   . %s\n' "$*"; }
hardstop() {
  echo
  echo "HARD STOP: $*"
  echo "  This is on the issue #78 hard-stop list: it cannot be overridden or skipped here."
  echo "  Fix it, then resume with: scripts/release.sh $tag"
  exit 1
}
phasefail() { # phase message
  echo
  echo "phase '$1' failed: $2"
  echo "  Resume with: scripts/release.sh $tag$( ((publish)) && printf ' --publish')$( ((smoke)) && printf ' --smoke')$( ((dry)) && printf ' --dry-run')"
  echo "  (Finished phases are not re-run; the state is in $state)"
  exit 1
}

# ------------------------------------------------------------------ the state file ----
state_write() { # jq options..., filter last; the file is appended by this function
  local t
  t="$(mktemp)" && jq "$@" "$state" >"$t" && mv "$t" "$state" \
    || { echo "FAIL: cannot update $state" >&2; exit 1; }
}
state_of() { jq -r --arg n "$1" '(.phases // [])[] | select(.name == $n) | .status' "$state" 2>/dev/null | head -1; }
state_field() { jq -r --arg n "$1" --arg k "$2" '((((.phases // [])[] | select(.name == $n)).detail // {})[$k]) // ""' "$state" 2>/dev/null; }
state_done() { # phase [detail-json]
  local sha; sha="$("$GIT_BIN" rev-parse HEAD 2>/dev/null || echo "")"
  local detail="${2:-}"
  [[ -n "$detail" ]] || detail='{}'
  state_write --arg n "$1" --arg s done --arg c "$sha" --arg a "$(date -u +%FT%TZ)" --argjson d "$detail" \
    '.phases = (((.phases // []) | map(select(.name != $n))) + [{name: $n, status: $s, commit: $c, at: $a, detail: $d}])'
  note "$1 done (at ${sha:0:7})"
}
state_skip() { # phase reason
  local sha; sha="$("$GIT_BIN" rev-parse HEAD 2>/dev/null || echo "")"
  state_write --arg n "$1" --arg s "skipped" --arg r "$2" --arg c "$sha" --arg a "$(date -u +%FT%TZ)" --argjson d "{\"reason\":\"$2\"}" \
    '.phases = (((.phases // []) | map(select(.name != $n))) + [{name: $n, status: $s, reason: $r, commit: $c, at: $a, detail: {reason: $r}}])'
  note "$1 skipped ($2)"
}

# ------------------------------------------------------------------ needs from the owner ----
# Printed before anything starts (issue #78: static checks, so nothing surprises the run hours
# later). Two of them stop the run when they fail: without the signing identity nothing can be
# built later, without push rights nothing can be tagged or released. "Kai is closed" is only
# warned about here (#75: the pre-flight warns; release-local.sh refuses while it runs).
echo "Needs from the owner (checked now, before anything starts):"
if KAI_SIGN_IDENTITY="$identity" "$CODESIGN_CHECK" --check >/dev/null 2>&1; then
  echo "  1. signing identity \"$identity\" is in the keychain        ok"
else
  echo "  1. signing identity \"$identity\" is in the keychain        MISSING"
  hardstop "the signing identity is missing (scripts/release-cert.sh creates it; see docs/releasing.md)"
fi
if [[ -f "$statedir/signing-key-backup.marker" ]]; then
  echo "     backup marker ($statedir/signing-key-backup.marker)     ok"
else
  echo "     backup marker ($statedir/signing-key-backup.marker)     none yet: back the .p12 up in the password manager and create the marker file"
fi
if pgrep -x Kai >/dev/null 2>&1; then
  echo "  2. Kai is closed                                          RUNNING (quit it before the build phase)"
else
  echo "  2. Kai is closed                                          ok"
fi
if [[ "$("$GH_BIN" api "repos/$repo" --jq .permissions.push 2>/dev/null)" == "true" ]]; then
  echo "  3. gh can push to $repo                          ok"
else
  echo "  3. gh can push to $repo                          NO"
  echo "  Fix it (gh auth login with push rights), then resume with: scripts/release.sh $tag"
  exit 1
fi
echo "  4. the macOS \"codesign wants to sign\" key prompt was allowed once  (cannot be checked; if it appears, allow it)"

# The tag/release-exists hard stop guards a NEW run against a name that is taken. A resumed run
# owns the tag and the draft its own state records, so it is exempt (anything else would make
# resuming after the tag or draft impossible).
tag_owned_by_state() { [[ "$(state_of tag)" == "done" || "$(state_of draft)" == "done" || "$(state_of publish)" == "done" ]]; }
if ! tag_owned_by_state; then
  if "$GIT_BIN" rev-parse -q --verify "refs/tags/$tag^{commit}" >/dev/null 2>&1 \
     || [[ -n "$("$GIT_BIN" ls-remote --tags origin "refs/tags/$tag" 2>/dev/null)" ]]; then
    hardstop "$tag already exists as a tag (here or on origin): pick the next version, or finish that release instead"
  fi
  if "$GH_BIN" api "repos/$repo/releases/tags/$tag" >/dev/null 2>&1; then
    hardstop "a GitHub release for $tag already exists: releases are never overwritten or moved"
  fi
fi

# ------------------------------------------------------------------ pure helpers ----

# The bump decision (docs/releasing.md "Decide the bump from CHANGELOG.md's [Unreleased]"):
# before 1.0 a Breaking Changes section forces MINOR; short of that, any Enhancements content
# is a MINOR too (0.5.0 followed 0.4.x that way); only a Bug-Fixes-only section is a PATCH.
# Echoes "minor" or "patch" for the [Unreleased] text on stdin.
bump_decision() {
  local breaking=0 enhancements=0 section=""
  while IFS= read -r line; do
    case "$line" in
      '## ['*) break ;;
      '### '*) section="${line#\#\#\# }" ;;
      '-'*|'*'*)
        [[ "$section" == "Breaking Changes" ]] && breaking=1
        [[ "$section" == "Enhancements" ]] && enhancements=1 ;;
    esac
  done
  if [[ $breaking -eq 1 || $enhancements -eq 1 ]]; then echo minor; else echo patch; fi
}

unreleased_text() { awk '/^## \[Unreleased\]/{f=1;next} f&&/^## \[/{exit} f' "$root/CHANGELOG.md"; }

next_version() { # latest vX.Y.Z on origin -> the version the bump rule wants
  local latest kind major minor patch
  latest="$("$GIT_BIN" ls-remote --tags origin 'refs/tags/v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null | awk -F/ '{print $NF}' | sort -V | tail -1)"
  [[ -n "$latest" ]] || { echo ""; return; }
  latest="${latest#v}"
  IFS=. read -r major minor patch <<<"$latest"
  kind="$(unreleased_text | bump_decision)"
  if [[ "$kind" == minor ]]; then echo "$major.$((minor + 1)).0"; else echo "$major.$minor.$((patch + 1))"; fi
}

# [Unreleased] becomes [X.Y.Z] - date with a fresh empty [Unreleased] above it. The changelog-skip
# comments at the top of the old section stay with the versioned section (the shape CHANGELOG.md
# already has). Reads stdin, writes stdout.
restructure_changelog() { # ver
  awk -v ver="$1" -v date="$(date +%F)" '
    !done && /^## \[Unreleased\][[:space:]]*$/ { print "## [Unreleased]"; print ""; print "## [" ver "] - " date; done = 1; next }
    { print }'
}

# ------------------------------------------------------------------ the phases ----
run_preflight() {
  step preflight
  "$PREFLIGHT" "$tag" || phasefail preflight "release-preflight.sh refused (see its FAIL lines above)"
  state_done preflight
}

run_changelog() {
  step changelog
  local cov
  cov="$("$CHANGELOG_CHECK" 2>&1)" || phasefail changelog "changelog-check.sh failed"
  printf '%s\n' "$cov" | grep -q '^ok:' || phasefail changelog "the [Unreleased] section does not cover every merged PR yet:$cov"
  local want
  want="$(next_version)"
  if [[ -n "$want" && "$want" != "$ver" ]]; then
    kind="$(unreleased_text | bump_decision)"
    phasefail changelog "the CHANGELOG's [Unreleased] asks for a $kind bump (v$want from the latest tag), but v$ver was requested (docs/releasing.md: Breaking Changes means MINOR before 1.0; Enhancements is a MINOR; only Bug Fixes is a PATCH)"
  fi
  note "coverage ok; the bump $ver matches [Unreleased]"
  state_done changelog "{\"bump_wanted\":\"$want\"}"
}

branch_postcondition() {
  if [[ "$(state_of merge)" == done ]]; then return 0; fi   # merged: the branch served its purpose
  "$GIT_BIN" rev-parse -q --verify "refs/heads/release/$tag" >/dev/null 2>&1 || return 1
  "$CHECK_VERSION" "$ver" . >/dev/null 2>&1
}

run_branch() {
  step branch
  if ! "$GIT_BIN" rev-parse -q --verify "refs/heads/release/$tag" >/dev/null 2>&1; then
    "$GIT_BIN" checkout -q -b "release/$tag" || phasefail branch "could not branch release/$tag"
  else
    "$GIT_BIN" checkout -q "release/$tag" || phasefail branch "could not check out release/$tag"
  fi
  "$BUMP" "$ver" || phasefail branch "release-bump.sh $ver failed"
  restructure_changelog "$ver" <"$root/CHANGELOG.md" >"$root/CHANGELOG.md.new" \
    && mv "$root/CHANGELOG.md.new" "$root/CHANGELOG.md" \
    || phasefail branch "could not restructure CHANGELOG.md"
  # Exactly the three files a release commit touches (docs/releasing.md); never `git add -A`,
  # which would sweep the run's own state and log under bin/ into the commit on a checkout
  # without the .gitignore entry.
  "$GIT_BIN" add build/config.yml build/darwin/Info.plist CHANGELOG.md \
    && "$GIT_BIN" commit -q -m "release: v$ver (version bump and changelog)" \
    || phasefail branch "could not commit the version bump"
  "$GIT_BIN" push -q -u origin "release/$tag" || phasefail branch "could not push release/$tag"
  "$CHECK_VERSION" "$ver" . || phasefail branch "the version files or CHANGELOG do not say $ver"
  state_done branch "{\"head\":\"$("$GIT_BIN" rev-parse HEAD)\"}"
}

run_pr() {
  step pr
  local url number
  url="$("$GH_BIN" pr create --repo "$repo" --base master --head "release/$tag" \
    --title "release: v$ver (version bump and changelog)" \
    --body "Release v$ver: the version bump and the CHANGELOG restructure, opened by scripts/release.sh (issue #78).")" \
    || phasefail pr "gh pr create failed"
  number="${url##*/}"
  [[ "$number" =~ ^[0-9]+$ ]] || phasefail pr "could not read the PR number from: $url"
  local head; head="$("$GIT_BIN" rev-parse "refs/remotes/origin/release/$tag")"
  note "release PR #$number at ${head:0:7}"
  state_done pr "{\"number\":$number,\"head\":\"$head\"}"
}

# Waits for CI and the PR-Agent score on the exact head (issue #78: PR-Agent above 90). Red CI is
# a hard stop, a pending check is a wait, a score that has not appeared yet is a wait.
run_wait_ci_pr() {
  step wait_ci_pr
  local number; number="$(state_field pr number)"
  [[ -n "$number" ]] || phasefail wait_ci_pr "no PR number in the state"
  local i head checks_out score
  for ((i = 0; i < POLL_MAX; i++)); do
    head="$("$GH_BIN" pr view "$number" --repo "$repo" --json headRefOid 2>/dev/null | jq -r .headRefOid)"
    if "$GH_BIN" pr checks "$number" --repo "$repo" >/dev/null 2>&1; then
      checks_out=green
    elif [[ $? == 8 ]]; then
      checks_out=pending
    else
      hardstop "CI is red on the release PR #$number (head ${head:0:7}): a red release commit is never merged"
    fi
    score="$("$GH_BIN" api "repos/$repo/issues/$number/comments" --jq '.[].body' 2>/dev/null; "$GH_BIN" api "repos/$repo/pulls/$number/comments" --jq '.[].body' 2>/dev/null)"
    score="$(printf '%s' "$score" | grep -oiE 'score[^0-9]*[0-9]+' | tail -1 | grep -oE '[0-9]+' || true)"
    if [[ "$checks_out" == green && -n "$score" ]]; then
      (( score >= 90 )) || hardstop "PR-Agent score is $score on PR #$number, below 90 with findings unresolved (issue #78)"
      state_done wait_ci_pr "{\"head\":\"$head\",\"pr_agent_score\":$score}"
      return
    fi
    note "waiting: CI $checks_out, PR-Agent score ${score:-not posted yet} (head ${head:0:7})"
    sleep "$POLL_SLEEP"
  done
  phasefail wait_ci_pr "CI or the PR-Agent score did not settle within the poll budget"
}

run_merge() {
  step merge
  local number; number="$(state_field pr number)"
  "$GH_BIN" pr merge "$number" --repo "$repo" --merge --delete-branch \
    || phasefail merge "gh pr merge failed"
  "$GIT_BIN" fetch -q origin master || phasefail merge "could not fetch master after the merge"
  "$GIT_BIN" checkout -q master || phasefail merge "could not check out master"
  "$GIT_BIN" pull --ff-only -q origin master || phasefail merge "could not fast-forward master"
  local merge_sha; merge_sha="$("$GIT_BIN" rev-parse HEAD)"
  state_done merge "{\"merge_commit\":\"$merge_sha\",\"pr\":$number}"
}

run_wait_ci_merge() {
  step wait_ci_merge
  local merge_sha; merge_sha="$(state_field merge merge_commit)"
  local i runs bad
  for ((i = 0; i < POLL_MAX; i++)); do
    runs="$("$GH_BIN" api "repos/$repo/commits/$merge_sha/check-runs" --paginate \
      --jq '.check_runs[] | [.name, .status, (.conclusion // "")] | @tsv' 2>/dev/null || true)"
    if [[ -n "$runs" ]]; then
      bad="$(printf '%s\n' "$runs" | awk -F'\t' '$2 == "completed" && $3 ~ /^(failure|cancelled|timed_out|action_required|startup_failure|stale)$/')"
      if [[ -n "$bad" ]]; then
        hardstop "CI is red on the release commit ${merge_sha:0:7}: $(printf '%s' "$bad" | cut -f1 | tr '\n' ', ')"
      fi
      if ! printf '%s\n' "$runs" | awk -F'\t' '$2 != "completed"' | grep -q .; then
        state_done wait_ci_merge "{\"commit\":\"$merge_sha\"}"
        return
      fi
    fi
    note "waiting for CI on the merge commit ${merge_sha:0:7}"
    sleep "$POLL_SLEEP"
  done
  phasefail wait_ci_merge "CI on the merge commit did not finish within the poll budget"
}

run_tag() {
  step tag
  local merge_sha; merge_sha="$(state_field merge merge_commit)"
  # A published tag is never moved or deleted (issue #78 hard stop): if any tag exists now, stop.
  if [[ -n "$("$GIT_BIN" ls-remote --tags origin "refs/tags/$tag" 2>/dev/null)" ]]; then
    hardstop "$tag appeared on origin while this run was going: refusing to move or overwrite a tag"
  fi
  "$GIT_BIN" tag -a "$tag" -m "$tag" "$merge_sha" || phasefail tag "could not tag $merge_sha"
  "$GIT_BIN" push origin "$tag" || phasefail tag "could not push $tag"
  local remote_sha; remote_sha="$("$GIT_BIN" ls-remote --tags origin "refs/tags/$tag^{}" | cut -f1)"
  [[ "$remote_sha" == "$merge_sha" ]] || phasefail tag "origin's $tag is not the merge commit"
  state_done tag "{\"tag\":\"$tag\",\"commit\":\"$merge_sha\"}"
}

build_postcondition() {
  [[ -f "$outdir/RELEASE_NOTES.md" && -d "$outdir/Kai.app" && -f "$outdir/assets/SHA256SUMS" ]]
}

run_build() {
  step build
  # release-local.sh in its own --dry-run mode: machine gates, fresh worktree of the tag, build,
  # tree check, verify, package, no release. The draft is the next phase, so the workflow (not
  # the wrapped script) owns it; this is the only way to keep both resumable without editing
  # release-local.sh.
  "$LOCAL" "$tag" --dry-run --yes --out "$outdir" || phasefail build "release-local.sh failed"
  build_postcondition || phasefail build "release-local.sh left no packaged app in $outdir"
  state_done build "{\"out\":\"$outdir\"}"
}

run_smoke() {
  step smoke
  if [[ $smoke -eq 1 ]]; then
    state_skip smoke "manual (--smoke): run the matrix from docs/release-checklist.md on $outdir/Kai.app"
    echo "  Run the smoke matrix now, on $outdir/Kai.app (docs/release-checklist.md)."
  else
    state_skip smoke "--smoke not given: recorded as skipped (issue #78: settled once, not asked per run)"
  fi
}

draft_postcondition() {
  "$GH_BIN" api "repos/$repo/releases/tags/$tag" 2>/dev/null | jq -e '.draft == true' >/dev/null 2>&1
}

run_draft() {
  step draft
  "$GH_BIN" release create "$tag" "$outdir"/assets/* --repo "$repo" --draft --verify-tag \
    --title "Kai $tag" --notes-file "$outdir/RELEASE_NOTES.md" \
    || phasefail draft "gh release create failed"
  draft_postcondition || phasefail draft "the draft for $tag did not appear"
  state_done draft "{\"out\":\"$outdir\"}"
}

run_publish() {
  step publish
  [[ "$(state_of draft)" == "done" ]] || phasefail publish "there is no finished draft to publish (run without --dry-run first)"
  # The only --draft=false in this script: publishing what this workflow drafted, on request.
  "$GH_BIN" release edit "$tag" --repo "$repo" --draft=false || phasefail publish "gh release edit failed"
  note "published; verifying the released artifact (docs/releasing.md step 17)"
  "$INSTALL" "$tag" --dest "$(mktemp -d)" || phasefail publish "the post-release installer verification failed"
  local f n=0
  for f in "Kai-$ver-darwin-arm64.zip" "updater-Kai-$ver-darwin-arm64.zip" "Kai-$ver-darwin-arm64.dmg" SHA256SUMS; do
    [[ -f "$outdir/assets/$f" ]] && n=$((n + 1))
  done
  [[ $n -eq 4 ]] || phasefail publish "expected the four release assets in $outdir/assets"
  state_done publish "{\"tag\":\"$tag\"}"
}

# ------------------------------------------------------------------ the state machine ----
PHASES=(preflight changelog branch pr wait_ci_pr merge wait_ci_merge tag build smoke draft publish)
ran_any=0
for phase in "${PHASES[@]}"; do
  # Flag-gated phases.
  [[ "$phase" != publish ]] || { ((publish)) || continue; }
  if [[ "$phase" == draft && $dry -eq 1 ]]; then
    [[ "$(state_of draft)" == done ]] || state_skip draft "--dry-run: nothing public is created"
    continue
  fi
  # Already finished: a no-op (the postconditions below also heal a lost state entry).
  st="$(state_of "$phase")"
  if [[ "$st" == done || "$st" == skipped ]]; then continue; fi
  case "$phase" in
    branch)    branch_postcondition && { state_done branch; continue; } ;;
    merge)     [[ "$("$GH_BIN" pr view "$(state_field pr number)" --repo "$repo" --json state 2>/dev/null | jq -r .state)" == MERGED ]] && { state_done merge "{\"merge_commit\":\"$("$GIT_BIN" rev-parse origin/master)\"}"; continue; } ;;
    tag)       [[ -n "$("$GIT_BIN" ls-remote --tags origin "refs/tags/$tag^{}" 2>/dev/null)" ]] && { state_done tag "{\"tag\":\"$tag\"}"; continue; } ;;
    build)     build_postcondition && { state_done build "{\"out\":\"$outdir\"}"; continue; } ;;
    draft)     draft_postcondition && { state_done draft "{\"out\":\"$outdir\"}"; continue; } ;;
  esac
  ran_any=1
  "run_$phase" || exit 1
done

# ------------------------------------------------------------------ what is left for the owner ----
echo
if [[ "$(state_of publish)" == done ]]; then
  echo "Release $tag: published and verified. Nothing left for the owner."
  exit 0
fi
if [[ "$(state_of draft)" == done ]]; then
  echo "Release $tag: the draft is up. Left for the owner:"
  echo "  1. Run the smoke matrix on $outdir/Kai.app (docs/release-checklist.md)"
  echo "  2. Publish: scripts/release.sh $tag --publish"
  exit 0
fi
((ran_any)) || echo "Release $tag: already complete up to the dry-run boundary; nothing to do."
echo "Release $tag: dry run complete. Left for the owner:"
echo "  1. The draft: re-run without --dry-run"
echo "  2. The smoke matrix on $outdir/Kai.app (docs/release-checklist.md)"
echo "  3. Publishing: scripts/release.sh $tag --publish"
