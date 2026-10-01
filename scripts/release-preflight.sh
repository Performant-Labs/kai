#!/usr/bin/env bash
# Check that this machine and this commit are ready to cut a release. Run it first.
#
#   scripts/release-preflight.sh [vX.Y.Z] [--skip-github] [--any-branch]
#
#   vX.Y.Z         the version about to be released; also checks nothing already uses it
#   --skip-github  skip every check that needs the network (for offline use and for the tests)
#   --any-branch   do not require master == origin/master (for trying the script on a branch)
#
# Prints ok / WARN / FAIL per check with a fix hint, runs every check even after a failure, and
# exits 1 if anything FAILed. WARN never fails the run: it marks something to decide or record
# in the release checklist. docs/release-checklist.md's Pre-flight section is the caller.
set -uo pipefail

repo="${KAI_RELEASE_REPO:-Performant-Labs/kai}"
version=""
skip_gh=0
any_branch=0
for arg in "$@"; do
  case "$arg" in
    --skip-github) skip_gh=1 ;;
    --any-branch) any_branch=1 ;;
    v[0-9]*.[0-9]*.[0-9]*) version="$arg" ;;
    -h|--help) sed -n '2,13p' "$0"; exit 0 ;;
    *) echo "unknown argument: $arg (usage: $0 [vX.Y.Z] [--skip-github] [--any-branch])" >&2; exit 2 ;;
  esac
done

fails=0
warns=0
ok()   { printf '  ok    %s\n' "$*"; }
info() { printf '  ..    %s\n' "$*"; }
warn() { printf '  WARN  %s\n' "$*"; warns=$((warns + 1)); }
fail() { printf '  FAIL  %s\n' "$*"; fails=$((fails + 1)); }
hint() { printf '        %s\n' "$*"; }

here="$(cd "$(dirname "$0")" && pwd)"

echo "Machine"

# 1. Apple Silicon macOS, and not a shell running under Rosetta.
os="$(uname -s)"; arch="$(uname -m)"
if [[ "$os" == "Darwin" && "$arch" == "arm64" ]]; then
  ok "macOS on Apple Silicon"
elif [[ "$os" == "Darwin" && "$(sysctl -n hw.optional.arm64 2>/dev/null)" == "1" ]]; then
  fail "this shell runs under Rosetta ($arch) on an Apple Silicon Mac"
  hint "open a native arm64 terminal: a build from here would not be an arm64 build"
else
  fail "releases are built on macOS Apple Silicon; this is $os $arch"
fi

# 2. Required tools.
missing=()
for t in git gh go pnpm node wails3 sqlc jq make ditto codesign shasum perl; do
  command -v "$t" >/dev/null 2>&1 || missing+=("$t")
done
[[ -x /usr/libexec/PlistBuddy ]] || missing+=("PlistBuddy")
xcrun --find swiftc >/dev/null 2>&1 || missing+=("swiftc (Xcode command line tools)")
if [[ ${#missing[@]} -eq 0 ]]; then
  ok "all required tools are installed"
else
  fail "missing tools: ${missing[*]}"
  hint "see 'make deps' / 'make tool-deps' for the Go tools; install Xcode command line tools for swiftc"
fi

# Everything below needs a repo.
if ! root="$(git rev-parse --show-toplevel 2>/dev/null)"; then
  fail "not inside a git checkout of $repo"
  echo; echo "FAIL: $fails problem(s)"; exit 1
fi
cd "$root"

# 3. Go is at least go.mod's version.
if [[ -f go.mod ]] && command -v go >/dev/null 2>&1; then
  want_go="$(awk '/^go /{print $2; exit}' go.mod)"
  have_go="$(go env GOVERSION 2>/dev/null | sed 's/^go//')"
  if [[ -n "$want_go" && -n "$have_go" && "$(printf '%s\n%s\n' "$want_go" "$have_go" | sort -V | head -1)" == "$want_go" ]]; then
    ok "go $have_go (go.mod needs $want_go)"
  else
    fail "go $have_go is older than go.mod's $want_go"
    hint "install Go $want_go or newer"
  fi
fi

# 4. Node: a different major than frontend/.nvmrc is only a warning. The .nvmrc says 25.9.0 while
#    builds here have worked on 26.x, so a hard check would fail a machine that works.
if [[ -f frontend/.nvmrc ]] && command -v node >/dev/null 2>&1; then
  want_node="$(tr -d 'v \n' <frontend/.nvmrc)"
  have_node="$(node -v | tr -d 'v')"
  if [[ "${want_node%%.*}" == "${have_node%%.*}" ]]; then
    ok "node $have_node (frontend/.nvmrc: $want_node)"
  else
    warn "node $have_node differs from frontend/.nvmrc ($want_node)"
    hint "builds have worked on a newer major; note it in the checklist, and set NODE_OPTIONS=--no-experimental-webstorage for the frontend tests"
  fi
fi

# 5. Kai must not be running: release-verify.sh refuses while it is.
if pgrep -x Kai >/dev/null 2>&1; then
  fail "Kai is running"
  hint "quit it: release-verify.sh needs to launch its own copy, and Kai allows one instance"
else
  ok "Kai is not running"
fi

echo "Repo and commit"

# 6. origin is Performant-Labs/kai, never upstream. The public repo is still technically a GitHub
# fork of dtapps/kai, so the check names the exact repo rather than "any fork".
origin="$(git remote get-url origin 2>/dev/null || true)"
if [[ "$origin" =~ github\.com[:/]${repo}(\.git)?$ ]]; then
  ok "origin is $repo"
elif [[ "$origin" == *dtapps/kai* ]]; then
  fail "origin points at upstream dtapps/kai: $origin"
  hint "releases and PRs go to $repo, never upstream"
else
  fail "origin is '$origin', expected $repo"
fi

# 7. Clean tree.
dirty="$(git status --porcelain)"
if [[ -z "$dirty" ]]; then
  ok "working tree is clean"
else
  fail "working tree has uncommitted changes:"
  printf '%s\n' "$dirty" | head -5 | sed 's/^/          /'
  hint "release from a clean checkout"
fi

# 8. On master, equal to origin/master.
branch="$(git symbolic-ref --short -q HEAD || echo "(detached)")"
if [[ $any_branch -eq 1 ]]; then
  info "branch check skipped (--any-branch); on $branch"
elif [[ "$branch" != "master" ]]; then
  fail "on '$branch', the release commit is master's tip"
  hint "git checkout master"
elif [[ $skip_gh -eq 1 ]]; then
  info "master vs origin/master skipped (--skip-github)"
elif ! git fetch -q origin master 2>/dev/null; then
  fail "could not fetch origin/master"
else
  if [[ "$(git rev-parse HEAD)" == "$(git rev-parse origin/master)" ]]; then
    ok "master is at origin/master ($(git rev-parse --short HEAD))"
  else
    behind="$(git rev-list --count HEAD..origin/master)"; ahead="$(git rev-list --count origin/master..HEAD)"
    fail "master differs from origin/master (behind $behind, ahead $ahead)"
    hint "git pull --ff-only origin master"
  fi
fi
sha="$(git rev-parse HEAD 2>/dev/null || true)"

# 9. The updater still points at upstream? (checklist step 5)
if [[ -f main.go ]]; then
  if grep -Eq 'GithubRepo:[[:space:]]*"dtapps/kai"' main.go; then
    warn "the in-app updater still points at upstream dtapps/kai (#178)"
    hint "record the state in checklist step 5; release notes must tell users to decline the update prompt"
  else
    ok "the in-app updater no longer points at upstream"
  fi
fi

if [[ $skip_gh -eq 1 ]]; then
  echo "GitHub"
  info "skipped (--skip-github)"
else
  echo "GitHub"

  # 10. gh works and can push.
  if ! gh auth status >/dev/null 2>&1; then
    fail "gh is not authenticated"
    hint "gh auth login"
  elif [[ "$(gh api "repos/$repo" --jq .permissions.push 2>/dev/null)" == "true" ]]; then
    ok "gh is authenticated with push rights to $repo"
  else
    fail "gh cannot push to $repo"
    hint "creating a tag and a release needs push access"
  fi

  # 11. CI on this exact commit: finished, and green.
  if [[ -n "$sha" ]]; then
    runs="$(gh api "repos/$repo/commits/$sha/check-runs" --paginate \
      --jq '.check_runs[] | [.name, .status, (.conclusion // ""), .html_url] | @tsv' 2>/dev/null || true)"
    if [[ -z "$runs" ]]; then
      fail "no CI runs found for ${sha:0:7}"
      hint "was it pushed? check the Actions tab"
    else
      total="$(printf '%s\n' "$runs" | wc -l | tr -d ' ')"
      pending="$(printf '%s\n' "$runs" | awk -F'\t' '$2 != "completed"' | cut -f1)"
      failed="$(printf '%s\n' "$runs" | awk -F'\t' '$2 == "completed" && $3 ~ /^(failure|cancelled|timed_out|action_required|startup_failure|stale)$/' | cut -f1)"
      if [[ -n "$failed" ]]; then
        fail "CI failed on ${sha:0:7}: $(printf '%s' "$failed" | tr '\n' ',' | sed 's/,$//; s/,/, /g')"
      elif [[ -n "$pending" ]]; then
        fail "CI still running on ${sha:0:7}: $(printf '%s' "$pending" | tr '\n' ',' | sed 's/,$//; s/,/, /g')"
        hint "wait for it; do not release a commit whose run you have not seen finish"
      else
        gojob="$(printf '%s\n' "$runs" | awk -F'\t' '$1 == "ci / go / build-lint-test" {print $4}')"
        ok "CI is green on ${sha:0:7} ($total checks). Link for checklist step 3: ${gojob:-see the Actions tab}"
      fi
    fi
  fi

  # 12. The version is free.
  if [[ -n "$version" ]]; then
    taken=()
    git rev-parse -q --verify "refs/tags/$version" >/dev/null 2>&1 && taken+=("local tag")
    git ls-remote --exit-code --tags origin "refs/tags/$version" >/dev/null 2>&1 && taken+=("tag on origin")
    git ls-remote --exit-code --heads origin "release/$version" >/dev/null 2>&1 && taken+=("branch release/$version")
    gh api "repos/$repo/releases/tags/$version" >/dev/null 2>&1 && taken+=("GitHub release")
    if [[ ${#taken[@]} -eq 0 ]]; then
      ok "$version is not used yet"
    else
      fail "$version already exists: ${taken[*]}"
      hint "pick the next version, or finish that release instead"
    fi
  else
    info "no version given: not checking that a tag or release name is free"
  fi

  # 13. Open PRs (checklist step 4): real work vs dependency bumps.
  prs="$(gh pr list --repo "$repo" --state open --limit 200 --json author --jq '.[].author.login' 2>/dev/null || true)"
  if [[ -n "$prs" ]]; then
    bots="$(printf '%s\n' "$prs" | grep -Ec 'dependabot|\[bot\]|^app/' || true)"
    real="$(( $(printf '%s\n' "$prs" | wc -l | tr -d ' ') - bots ))"
    if [[ "$real" -gt 0 ]]; then
      warn "$real open non-bot PR(s): is any real work left unmerged? (checklist step 4)"
    else
      ok "no open non-bot PRs"
    fi
    info "$bots open dependency bump PR(s): merge the ones wanted, record why the rest wait"
  else
    ok "no open PRs"
  fi

  # 14. Changelog coverage, early (checklist step 8 is where it gets fixed).
  if [[ -x "$here/changelog-check.sh" && -f CHANGELOG.md ]]; then
    cc="$("$here/changelog-check.sh" 2>/dev/null || true)"
    n="$(printf '%s\n' "$cc" | sed -n 's/^NOT covered by \[Unreleased\] (\([0-9]*\)).*/\1/p')"
    if [[ -n "$n" ]]; then
      info "$n merged PR(s) are not covered by [Unreleased] yet (checklist step 8)"
    elif printf '%s' "$cc" | grep -q '^ok:'; then
      ok "every merged PR is covered by [Unreleased]"
    fi
  fi
fi

echo
if [[ $fails -gt 0 ]]; then
  echo "FAIL: $fails problem(s), $warns warning(s). Fix the FAIL lines before starting the release."
  exit 1
fi
echo "PASS: ready to start${version:+ $version}. $warns warning(s): record each in the checklist."
