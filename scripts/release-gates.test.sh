#!/usr/bin/env bash
# Tests for scripts/release-gates.sh, in throwaway git repos with a fake `gh` on PATH.
#   bash scripts/release-gates.test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
gates="$here/release-gates.sh"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
export GIT_AUTHOR_NAME=t GIT_AUTHOR_EMAIL=t@t GIT_COMMITTER_NAME=t GIT_COMMITTER_EMAIL=t@t

git init -q --bare "$tmp/origin.git"
git clone -q "$tmp/origin.git" "$tmp/work" 2>/dev/null
cd "$tmp/work"
git checkout -q -b master
echo a >f && git add f && git commit -qm one
git tag -a v1.0.0 -m v1.0.0                       # on master
git push -q origin master
git checkout -q -b side
echo b >f && git commit -qam two                   # not on master
git tag -a v1.0.1 -m v1.0.1
git checkout -q master

mkdir "$tmp/bin"
cat >"$tmp/bin/gh" <<'FAKE'
#!/usr/bin/env bash
# Prints canned rows: check-runs from $FAKE_RUNS, releases from $FAKE_RELEASES. Fails when FAKE_GH_FAIL is set.
[[ -z "${FAKE_GH_FAIL:-}" ]] || exit 1
case "$*" in
  *check-runs*) printf '%b' "${FAKE_RUNS:-}" ;;
  *releases*)   printf '%b' "${FAKE_RELEASES:-}" ;;
esac
FAKE
chmod +x "$tmp/bin/gh"

green='build\tcompleted\tsuccess\nlint\tcompleted\tsuccess\nreview\tcompleted\tskipped\n'
fail=0; out=""; code=0
has()   { if printf '%s' "$out" | grep -q -- "$2"; then echo "ok   $1"; else echo "FAIL $1 (wanted /$2/)"; fail=1; fi; }
hasnt() { if printf '%s' "$out" | grep -q -- "$2"; then echo "FAIL $1 (did not want /$2/)"; fail=1; else echo "ok   $1"; fi; }
exits() { if [[ "$code" -eq "$2" ]]; then echo "ok   $1"; else echo "FAIL $1 (exit $code, wanted $2)"; fail=1; fi; }
run()   { out="$(PATH="$tmp/bin:$PATH" "$gates" "$@" 2>&1)"; code=$?; }

FAKE_RUNS="$green" FAKE_RELEASES="" run v1.0.0
exits "a tag on master with green CI and no release passes" 0
has   "  ... and says CI is green" 'CI is green'

FAKE_RUNS="$green" FAKE_RELEASES="" run v1.0.1
exits "a tag not on master fails" 1
has   "  ... and says so" 'is not on origin/master'

FAKE_RUNS="$green" run v9.9.9
exits "a tag that does not exist fails" 1

FAKE_RUNS="${green}slow\tin_progress\t\n" FAKE_RELEASES="" run v1.0.0
exits "a CI check still running fails" 1
has   "  ... naming it" 'slow (in_progress'

FAKE_RUNS="${green}broken\tcompleted\tfailure\n" FAKE_RELEASES="" run v1.0.0
exits "a failed CI check fails" 1
has   "  ... naming it" 'broken (completed failure)'

FAKE_RUNS="${green}Mine\tin_progress\t\n" FAKE_RELEASES="" EXCLUDE_CHECK="Mine" run v1.0.0
exits "an excluded check (the workflow's own job) is ignored" 0

FAKE_RUNS="" FAKE_RELEASES="" run v1.0.0
exits "no CI runs at all fails" 1

FAKE_RUNS="$green" FAKE_RELEASES="https://github.com/x/y/releases/tag/v1.0.0\n" run v1.0.0
exits "an existing release for the tag fails" 1
has   "  ... and says so" 'already exists'

FAKE_RUNS="$green" FAKE_RELEASES="https://github.com/x/y/releases/tag/v1.0.0\n" run v1.0.0 --no-release-check
exits "--no-release-check skips that gate" 0

FAKE_GH_FAIL=1 run v1.0.0
exits "an unreadable GitHub fails closed" 1

run not-a-tag
exits "a bad tag name is a usage error" 2

[[ $fail -eq 0 ]] && echo "ALL PASS" || { echo "SOME FAILED"; exit 1; }
