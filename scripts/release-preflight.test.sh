#!/usr/bin/env bash
# Tests for scripts/release-preflight.sh: the failure paths, in throwaway repos, with no network
# (--skip-github). Meant for the release Mac; it assumes macOS Apple Silicon with the release
# tools installed, like the script it tests.
#   bash scripts/release-preflight.test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
pf="$here/release-preflight.sh"
tmp="$(mktemp -d)"
fake_pid=""
cleanup() { [[ -n "$fake_pid" ]] && kill "$fake_pid" 2>/dev/null; rm -rf "$tmp"; }
trap cleanup EXIT

good_origin="https://github.com/Performant-Labs/kai-private.git"
fail=0
out=""; code=0

mkrepo() { # dir [origin-url]
  local d="$1" url="${2:-$good_origin}"
  mkdir -p "$d" && git -C "$d" init -q -b master
  git -C "$d" -c user.name=t -c user.email=t@t commit -q --allow-empty -m init
  git -C "$d" remote add origin "$url"
}
run() { # dir args...
  local d="$1"; shift
  out="$(cd "$d" && "$pf" "$@" 2>&1)"; code=$?
}
has()  { if printf '%s' "$out" | grep -q -- "$2"; then echo "ok   $1"; else echo "FAIL $1 (wanted /$2/)"; fail=1; fi; }
hasnt(){ if printf '%s' "$out" | grep -q -- "$2"; then echo "FAIL $1 (did not want /$2/)"; fail=1; else echo "ok   $1"; fi; }
exit_is() { if [[ "$code" == "$2" ]]; then echo "ok   $1"; else echo "FAIL $1 (exit $code, wanted $2)"; fail=1; fi; }

# A repo that should pass every local check.
mkrepo "$tmp/good"
run "$tmp/good" --skip-github
has  "clean master with the right origin passes"     '^PASS'
exit_is "  ... and exits 0"                          0
has  "--skip-github says so"                         'skipped (--skip-github)'

# origin.
mkrepo "$tmp/up" "git@github.com:dtapps/kai.git"
run "$tmp/up" --skip-github
has  "upstream origin fails"                          'FAIL.*upstream dtapps/kai'
exit_is "  ... and exits 1"                          1
mkrepo "$tmp/other" "git@github.com:someone/else.git"
run "$tmp/other" --skip-github
has  "unknown origin fails"                           "FAIL.*expected Performant-Labs/kai-private"

# Tree and branch.
mkrepo "$tmp/dirty"; echo x >"$tmp/dirty/untracked.txt"
run "$tmp/dirty" --skip-github
has  "a dirty tree fails"                             'FAIL.*uncommitted changes'
mkrepo "$tmp/br"; git -C "$tmp/br" checkout -q -b topic
run "$tmp/br" --skip-github
has  "not on master fails"                            "FAIL.*on 'topic'"
run "$tmp/br" --skip-github --any-branch
has  "--any-branch skips the branch check"            'branch check skipped'
hasnt "  ... and does not fail on it"                 "FAIL.*on 'topic'"

# Go and Node versions.
mkrepo "$tmp/oldgo"; printf 'module x\n\ngo 99.0.0\n' >"$tmp/oldgo/go.mod"
git -C "$tmp/oldgo" add go.mod && git -C "$tmp/oldgo" -c user.name=t -c user.email=t@t commit -q -m go
run "$tmp/oldgo" --skip-github
has  "go older than go.mod fails"                     'FAIL.*older than go.mod'
mkrepo "$tmp/node"; mkdir "$tmp/node/frontend"; echo 1.0.0 >"$tmp/node/frontend/.nvmrc"
git -C "$tmp/node" add . && git -C "$tmp/node" -c user.name=t -c user.email=t@t commit -q -m nvm
run "$tmp/node" --skip-github
has  "a node major mismatch only warns"               'WARN.*differs from frontend/.nvmrc'
exit_is "  ... and exits 0"                          0

# The updater.
mkrepo "$tmp/upd"; printf 'GithubRepo:  "dtapps/kai",\n' >"$tmp/upd/main.go"
git -C "$tmp/upd" add . && git -C "$tmp/upd" -c user.name=t -c user.email=t@t commit -q -m m
run "$tmp/upd" --skip-github
has  "updater pointing at upstream warns"             'WARN.*updater still points at upstream'
mkrepo "$tmp/upd2"; printf 'GithubRepo:  "Performant-Labs/kai-private",\n' >"$tmp/upd2/main.go"
git -C "$tmp/upd2" add . && git -C "$tmp/upd2" -c user.name=t -c user.email=t@t commit -q -m m
run "$tmp/upd2" --skip-github
has  "updater pointing elsewhere is ok"               'ok.*no longer points at upstream'

# Missing tool: a PATH that has every required tool except wails3.
mkdir "$tmp/bin"
for t in git gh go pnpm node sqlc jq make ditto codesign shasum perl pgrep uname awk sed sort head tr wc tee grep cat; do
  p="$(command -v "$t" 2>/dev/null || true)"; [[ -n "$p" ]] && ln -sf "$p" "$tmp/bin/$t"
done
out="$(cd "$tmp/good" && PATH="$tmp/bin:/usr/bin:/bin" "$pf" --skip-github 2>&1)"; code=$?
has  "a missing tool fails and names it"              'FAIL.*missing tools:.*wails3'
exit_is "  ... and exits 1"                          1

# Kai running (a copy of sleep named Kai, so a real Kai is never touched).
if pgrep -x Kai >/dev/null 2>&1; then
  echo "skip Kai-running case: a real Kai is running, leaving it alone"
else
  run "$tmp/good" --skip-github
  has  "Kai not running is ok"                        'ok.*Kai is not running'
  # Copying a system binary invalidates its signature and macOS kills it, so re-sign the copy.
  cp /bin/sleep "$tmp/Kai" && codesign --force --sign - "$tmp/Kai" 2>/dev/null
  "$tmp/Kai" 120 & fake_pid=$!
  sleep 1
  run "$tmp/good" --skip-github
  has  "a running Kai fails"                          'FAIL.*Kai is running'
  exit_is "  ... and exits 1"                        1
  kill "$fake_pid" 2>/dev/null; wait "$fake_pid" 2>/dev/null; fake_pid=""
fi

# The GitHub section, with a fake `gh` first on PATH (canned answers, no network). --any-branch
# skips the fetch, and no version is given so nothing runs `git ls-remote`.
mkdir "$tmp/fakebin"
cat >"$tmp/fakebin/gh" <<'EOF'
#!/usr/bin/env bash
case "$*" in
  "auth status"*) exit 0 ;;
  *"--jq .permissions.push"*) echo "${FAKE_PUSH:-true}" ;;
  *check-runs*)
    case "${FAKE_CI:-green}" in
      green)   printf 'ci / go / build-lint-test\tcompleted\tsuccess\thttps://x/1\nci / other\tcompleted\tskipped\thttps://x/2\n' ;;
      failed)  printf 'ci / go / build-lint-test\tcompleted\tfailure\thttps://x/1\n' ;;
      pending) printf 'ci / go / build-lint-test\tin_progress\t\thttps://x/1\n' ;;
      none)    ;;
    esac ;;
  *"pr list"*) printf '%b' "${FAKE_PRS:-}" ;;
  *) exit 1 ;;
esac
EOF
chmod +x "$tmp/fakebin/gh"
gh_run() { out="$(cd "$tmp/good" && PATH="$tmp/fakebin:$PATH" "$@" 2>&1)"; code=$?; }

gh_run env FAKE_CI=green "$pf" --any-branch
has  "green CI passes and prints the run link"        'ok.*CI is green.*https://x/1'
has  "  ... and push rights are confirmed"            'ok.*push rights'
exit_is "  ... and exits 0"                          0
gh_run env FAKE_CI=failed "$pf" --any-branch
has  "failed CI fails"                                'FAIL.*CI failed on'
exit_is "  ... and exits 1"                          1
gh_run env FAKE_CI=pending "$pf" --any-branch
has  "CI still running fails"                         'FAIL.*CI still running on'
gh_run env FAKE_CI=none "$pf" --any-branch
has  "no CI runs at all fails"                        'FAIL.*no CI runs found'
gh_run env FAKE_PUSH=false "$pf" --any-branch
has  "no push rights fails"                           'FAIL.*cannot push'
gh_run env FAKE_PRS='app/dependabot\napp/dependabot\n' "$pf" --any-branch
has  "only bot PRs: no open non-bot PRs"              'ok.*no open non-bot PRs'
has  "  ... and the bump count is reported"           '2 open dependency bump'
gh_run env FAKE_PRS='app/dependabot\naangelinsf\n' "$pf" --any-branch
has  "a human PR warns"                               'WARN.*1 open non-bot PR'
exit_is "  ... and still exits 0"                    0

# Not a repo, bad arguments.
mkdir "$tmp/empty"
run "$tmp/empty" --skip-github
has  "outside a git checkout fails"                   'FAIL.*not inside a git checkout'
exit_is "  ... and exits 1"                          1
run "$tmp/good" --nonsense
has  "an unknown argument is rejected"                'unknown argument'
exit_is "  ... with exit 2"                          2
run "$tmp/good" v1.2.3 --skip-github
has  "a version argument is accepted"                 '^PASS: ready to start v1.2.3'

[[ $fail -eq 0 ]] && echo "PASS" || { echo "FAILED"; exit 1; }
