#!/usr/bin/env bash
# The checks that must hold before a tag is built and released. Used by scripts/release-local.sh
# (and by .github/workflows/release-kai.yml), so the rules live in one place.
#
#   scripts/release-gates.sh vX.Y.Z [--no-release-check]
#
# Run it inside a git checkout that has the tag. It checks, and reports every failure:
#   1. the tag exists and is a plain vX.Y.Z;
#   2. its commit is on origin/master (a tag on some other branch must not release);
#   3. every CI check on that exact commit has finished and passed (docs/releasing.md: do not
#      tag off a commit whose run you have not looked at; PR-Agent shows as skipped on master);
#   4. no GitHub release, draft included, already exists for the tag (never overwrite one);
#      skipped with --no-release-check.
#
# Environment: GH_REPO (default Performant-Labs/kai); EXCLUDE_CHECK, a check-run name to leave
# out of check 3 (the workflow's own job, which is in progress while it runs).
set -uo pipefail

repo="${GH_REPO:-Performant-Labs/kai}"
tag="${1:-}"
release_check=1
[[ "${2:-}" == "--no-release-check" ]] && release_check=0
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "usage: $0 vX.Y.Z [--no-release-check]" >&2; exit 2; }

bad=0
ok()   { printf '  ok    %s\n' "$*"; }
fail() { printf '  FAIL  %s\n' "$*" >&2; bad=1; }

if ! sha="$(git rev-parse -q --verify "refs/tags/$tag^{commit}")"; then
  fail "no tag $tag in this checkout (git fetch --tags)"
  exit 1
fi
ok "$tag is commit ${sha:0:7}"

# 2. On master.
if git fetch -q --no-tags origin +refs/heads/master:refs/remotes/origin/master 2>/dev/null \
   && git merge-base --is-ancestor "$sha" refs/remotes/origin/master; then
  ok "${sha:0:7} is on origin/master"
else
  fail "$tag (${sha:0:7}) is not on origin/master, or origin/master could not be fetched"
fi

# 3. CI green on that commit.
if runs="$(gh api "repos/$repo/commits/$sha/check-runs" --paginate \
    --jq '.check_runs[] | [.name, .status, (.conclusion // "")] | @tsv' 2>/dev/null)"; then
  [[ -z "${EXCLUDE_CHECK:-}" ]] || runs="$(printf '%s\n' "$runs" | awk -F'\t' -v x="$EXCLUDE_CHECK" '$1 != x')"
  if [[ -z "$runs" ]]; then
    fail "no CI runs found for ${sha:0:7}"
  elif printf '%s\n' "$runs" | awk -F'\t' '$2 != "completed" || $3 !~ /^(success|skipped|neutral)$/ { bad = 1 } END { exit !bad }'; then
    fail "CI is not green on ${sha:0:7}:"
    printf '%s\n' "$runs" | awk -F'\t' '$2 != "completed" || $3 !~ /^(success|skipped|neutral)$/ { printf "          %s (%s %s)\n", $1, $2, $3 }' >&2
  else
    ok "CI is green on ${sha:0:7} ($(printf '%s\n' "$runs" | wc -l | tr -d ' ') checks)"
  fi
else
  fail "could not read the CI runs for ${sha:0:7} (is gh logged in?)"
fi

# 4. No release for the tag yet.
if [[ $release_check -eq 1 ]]; then
  if found="$(gh api "repos/$repo/releases" --paginate --jq ".[] | select(.tag_name == \"$tag\") | .html_url" 2>/dev/null)"; then
    if [[ -n "$found" ]]; then fail "a release for $tag already exists: $found"; else ok "no release exists for $tag"; fi
  else
    fail "could not list the releases of $repo"
  fi
fi

exit $bad
