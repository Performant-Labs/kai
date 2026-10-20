#!/usr/bin/env bash
# The release workflow operator (issue #78): runs a release end to end, phase by phase,
# with no questions. The phase list, postconditions and failure classes are the R10 table
# in docs/handoffs/0078-issue-78/handoff-A.md; the governing rules live in
# docs/releasing.md and docs/release-checklist.md and are not restated here. It wraps the
# existing release-* scripts (one job each) and `gh`; it never duplicates their logic.
#
#   scripts/release.sh vX.Y.Z [--dry-run|--publish] [--smoke] [--smoke-done] [--quit-kai]
#
# Postures (R8): --dry-run = everything through the local build, no draft release;
# default = the same plus the draft (non-public), then stop; --publish = continue with
# publish, verify-published and closeout. Posture escalation only (R6).
#
# State is a cache at bin/release/vX.Y.Z/state.json (R6): atomic writes, a mkdir lock with
# an owner PID, resume postcondition-first (absent state re-derives; corrupt state stops),
# re-running a finished phase is a no-op.
#
# Failures: hard stops (R5) never consult the AI and never delete or move a release or a
# published tag; the four known blockers have static handlers and never consult (R10);
# an unknown failure consults scripts/release-ask-ai.sh once, capped at 2 consultations
# per phase (R3), and the executor runs only allowlisted actions itself (R4). Wrapped
# scripts are invoked by bare name (R7) so tests can interpose fakes on PATH.
set -uo pipefail

repo="${KAI_RELEASE_REPO:-Performant-Labs/kai}"
here="$(cd "$(dirname "$0")" && pwd)"

# ---------------------------------------------------------------- arguments
tag=""
want_dry=0; want_pub=0
smoke=0; smoke_done=0; quit_kai=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run)   want_dry=1 ;;
    --publish)   want_pub=1 ;;
    --smoke)     smoke=1 ;;
    --smoke-done) smoke_done=1 ;;
    --quit-kai)  quit_kai=1 ;;
    -h|--help)   sed -n '2,/^set -uo/p' "$0" | sed '$d'; exit 0 ;;
    v[0-9]*.[0-9]*.[0-9]*) tag="$1" ;;
    *) echo "unknown argument: $1 (usage: $0 vX.Y.Z [--dry-run|--publish] [--smoke] [--smoke-done] [--quit-kai])" >&2; exit 2 ;;
  esac
  shift
done
[[ "$tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "usage: $0 vX.Y.Z [--dry-run|--publish] [--smoke] [--smoke-done] [--quit-kai]" >&2; exit 2; }
[[ $want_dry -eq 0 || $want_pub -eq 0 ]] || { echo "give --dry-run or --publish, not both" >&2; exit 2; }
ver="${tag#v}"
posture="default"
[[ $want_dry -eq 0 ]] || posture="dry-run"
[[ $want_pub -eq 0 ]] || posture="publish"

root="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "FAIL: run this inside a checkout of $repo" >&2; exit 1; }
cd "$root"
statedir="$root/bin/release/$tag"
state="$statedir/state.json"
lockdir="$statedir/.lock"
mkdir -p "$statedir"

# ---------------------------------------------------------------- lock (R6)
LOCKED=0
VERIFY_TMP=""   # phase 13's throwaway dirs (PR #100 F2): trap-cleaned on every exit
cleanup() {
  local d
  for d in $VERIFY_TMP; do [[ -n "$d" ]] && rm -rf "$d"; done
  VERIFY_TMP=""
  if [[ "$LOCKED" -eq 1 && -f "$lockdir/pid" ]] \
     && [[ "$(cat "$lockdir/pid" 2>/dev/null || true)" == "$$" ]]; then
    rm -rf "$lockdir"
  fi
}
if [[ -d "$lockdir" && -f "$lockdir/pid" ]]; then
  lpid="$(cat "$lockdir/pid" 2>/dev/null || true)"
  if [[ -n "$lpid" ]] && kill -0 "$lpid" 2>/dev/null; then
    echo "refused: another release.sh is running for $tag (PID $lpid); let it finish or inspect it by hand" >&2
    exit 1
  fi
  rm -rf "$lockdir"   # a dead owner's lock is removed and retaken
fi
mkdir -p "$lockdir"
echo "$$" >"$lockdir/pid"
LOCKED=1
trap cleanup EXIT

# ---------------------------------------------------------------- state (R6)
rank_of() { case "$1" in dry-run) echo 0 ;; default) echo 1 ;; publish) echo 2 ;; *) echo 1 ;; esac; }

st_update() { # jq filter first, then jq options; atomic same-dir rename
  local filt="$1"; shift
  local tmp="$state.tmp.$$"
  if jq "$@" "$filt" "$state" >"$tmp" 2>/dev/null; then mv "$tmp" "$state"; else rm -f "$tmp"; fi
}
phase_done() { [[ "$(jq -r --arg p "$1" '.phases[$p].done // false' "$state" 2>/dev/null)" == "true" ]]; }
mark_done() { # one phase, or a list
  local p sha
  sha="$(git rev-parse HEAD 2>/dev/null || true)"
  for p in "$@"; do st_update '.phases[$p] = {done: true, commit: $c}' --arg p "$p" --arg c "$sha"; done
}
consult_count() { jq -r --arg p "$1" '[.consultations[]? | select(.phase == $p)] | length' "$state" 2>/dev/null || echo 0; }
record_consult() {
  st_update '.consultations += [{phase: $p, action: $a, at: $now}]' \
    --arg p "$1" --arg a "$2" --arg now "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
}

if [[ -f "$state" ]]; then
  if ! jq empty "$state" >/dev/null 2>&1; then
    echo "STOP: $state is not valid JSON — delete it to rebuild from postconditions (state is a cache; every phase re-derives its postcondition)." >&2
    exit 1
  fi
  recorded="$(jq -r '.posture // ""' "$state" 2>/dev/null)"
  if [[ -n "$recorded" && "$(rank_of "$posture")" -lt "$(rank_of "$recorded")" ]]; then
    echo "refused: posture escalation only — the recorded posture for $tag is '$recorded' and this run asked for '$posture' (a lower, dry-run posture would do less than was already done). Resume with --publish." >&2
    exit 1
  fi
  [[ -z "$recorded" || "$(rank_of "$posture")" -le "$(rank_of "$recorded")" ]] \
    || st_update '.posture = $p' --arg p "$posture"
else
  printf '{"posture":"%s","tag":"%s","phases":{%s},"consultations":[],"smoke":"pending"}\n' \
    "$posture" "$tag" \
    "$(for p in needs preflight changelog branch bump restructure release-pr review merge tag local smoke publish verify-published closeout; do printf '"%s":{"done":false},' "$p"; done | sed 's/,$//')" \
    >"$state"
fi
ISSUE="$(jq -r '.issue // ""' "$state" 2>/dev/null || true)"

# ---------------------------------------------------------------- dispatch and messaging (R7)
run_tool() { # invoke a wrapped script by bare name; PATH first, else the sibling
  local name="$1"; shift
  if command -v "$name" >/dev/null 2>&1; then "$name" "$@"; else "$here/$name" "$@"; fi
}
say() { printf '%s\n' "$*"; }
notify() {
  command -v osascript >/dev/null 2>&1 || return 0
  local msg="${1//\"/\'}"
  osascript -e "display notification \"$msg\" with title \"release.sh $tag\"" >/dev/null 2>&1 || true
}
issue_comment() { # <phase> <kind> <body> [log] — marker-tagged (R11), always --body-file (N-A4)
  [[ -n "${ISSUE:-}" ]] || return 0
  local phase="$1" kind="$2" body="$3" log="${4:-}"
  local f; f="$(mktemp "${TMPDIR:-/tmp}/release-comment.XXXXXX")"
  {
    printf '<!-- release.sh: %s %s %s -->\n' "$tag" "$phase" "$kind"
    printf '%s\n' "$body"
    if [[ -n "$log" && -f "$log" ]]; then printf 'log tail:\n'; tail -n 15 "$log"; fi
  } >"$f"
  gh issue comment "$ISSUE" --repo "$repo" --body-file "$f" >/dev/null 2>&1 || true
  rm -f "$f"
}
do_stop() { # <phase> <reason> [log] — loud stop: message, notification, checklist comment, resume
  local phase="$1" reason="$2" log="${3:-}"
  {
    printf '\nSTOP: %s, phase "%s": %s\n' "$tag" "$phase" "$reason"
    if [[ -n "$log" && -f "$log" ]]; then printf 'log tail (%s):\n' "$log"; tail -n 15 "$log" | sed 's/^/  /'; fi
    printf 'resume: scripts/release.sh %s\n' "$tag"
  } >&2
  notify "release $tag stopped at phase $phase"
  issue_comment "$phase" stop "STOP at phase $phase: $reason" "$log"
  exit 1
}

# ---------------------------------------------------------------- gh probes
find_pr() { # "<number> <state> <headSha>" for release/$tag, or nothing
  gh pr list --repo "$repo" --head "release/$tag" --json number,state,headRefOid 2>/dev/null \
    | jq -r -s 'flatten | .[0] | if . then "\(.number) \(.state) \(.headRefOid // "")" else empty end' 2>/dev/null || true
}
merge_sha_of() { # <pr> — the merge commit if gh reports one, else the PR head
  gh pr view "$1" --repo "$repo" --json mergeCommit,headRefOid 2>/dev/null \
    | jq -r '.mergeCommit.oid // .headRefOid // empty' 2>/dev/null || true
}
ci_green_on() { # <sha> — 0 green; 1 not (prints why). Same predicate as release-gates.sh.
  local sha="$1" out tsv bad
  out="$(gh api "repos/$repo/commits/$sha/check-runs" 2>/dev/null)" || { echo "could not read the CI check-runs for $sha"; return 1; }
  if printf '%s' "$out" | grep -q '^ *{'; then
    tsv="$(printf '%s' "$out" | jq -r '.check_runs[]? | [.name, .status, (.conclusion // "")] | @tsv' 2>/dev/null || true)"
  else
    tsv="$out"   # the harness's canned TSV shape (handoff-T-red Assumed 10)
  fi
  [[ -n "$tsv" ]] || { echo "no CI check-runs found for $sha"; return 1; }
  bad="$(printf '%s' "$tsv" | awk -F'\t' '$2 != "completed" || $3 !~ /^(success|skipped|neutral)$/ { print $1 " (" $2 " " $3 ")" }')"
  [[ -z "$bad" ]] || { printf '%s\n' "$bad"; return 1; }
  return 0
}
agent_gate() { # <pr> <headSha> — 0 pass; 1 fail (prints why). Rule R1.
  local pr="$1" head="$2" json bodies blk state score complete rhead unresolved
  json="$(gh pr view "$pr" --repo "$repo" --json comments 2>/dev/null || true)"
  [[ -n "$json" ]] || json="$(gh api "repos/$repo/pulls/$pr/comments" 2>/dev/null || true)"
  bodies="$(printf '%s' "$json" | jq -r 'if type == "array" then .[] else (.comments // []) end | .body // empty' 2>/dev/null || true)"
  if ! printf '%s' "$bodies" | grep -q 'pr-agent-review-state:v1'; then
    echo "no PR-Agent review state found on PR #$pr (the pr-agent-review-state:v1 comment)"
    return 1
  fi
  blk="$(printf '%s' "$bodies" | sed -n '/pr-agent-review-state:v1/,/-->/p' | sed '1d;$d')"
  state="$(printf '%s' "$blk" | grep -m1 '^{')"
  score="$(printf '%s' "$bodies" | sed -n 's~.*<strong>Score</strong>: \([0-9][0-9]*\)</td>.*~\1~p' | tail -1)"
  complete="$(printf '%s' "$state" | jq -r '.last_run.complete // false' 2>/dev/null || true)"
  rhead="$(printf '%s' "$state" | jq -r '.last_run.head_sha // ""' 2>/dev/null || true)"
  unresolved="$(printf '%s' "$state" | jq -r '[.findings[]? | select(.state != "RESOLVED")] | length' 2>/dev/null || true)"
  [[ "$unresolved" =~ ^[0-9]+$ ]] || unresolved=0
  if [[ "$complete" != "true" ]]; then echo "the PR-Agent review is not complete at the PR head (last_run.complete=$complete)"; return 1; fi
  if [[ -n "$rhead" && "$rhead" != "$head" ]]; then echo "the PR-Agent review is stale (review head ${rhead:0:7}, PR head ${head:0:7})"; return 1; fi
  if [[ -z "$score" || "$score" -lt 90 ]]; then echo "the PR-Agent score is ${score:-missing}: below the boundary 90 (90 passes; every finding must also be RESOLVED)"; return 1; fi
  if [[ "$unresolved" -gt 0 ]]; then echo "$unresolved unresolved PR-Agent finding(s) (state != RESOLVED) at score $score"; return 1; fi
  return 0
}

# ---------------------------------------------------------------- static handlers (known blockers)
handler_quit_kai() {
  say "  .. known blocker: Kai is running — quitting it (--quit-kai)"
  osascript -e 'tell application "Kai" to quit' >/dev/null 2>&1 || true
  sleep 1
  if pgrep -x Kai >/dev/null 2>&1; then pkill -x Kai || true; sleep 1; fi
  return 0
}
handler_regen_bindings() { # <phase> <log> — make bindings + commit (#73). PRE-merge only.
  # PR #100 R4a: once the tag is pushed (or the PR merged) the git tag is the source of
  # truth (docs/releasing.md) — regenerating and committing frontend/bindings then would
  # move the tree past the published tag, so the handler refuses instead: a hard stop for
  # the owner, no mutation of the tagged tree, no commit to release/*, no consultation.
  local phase="$1" log="$2"
  if phase_done tag || phase_done merge; then
    do_stop "$phase" "bindings drift, but $tag is already tagged/merged: the git tag $tag is the source of truth (docs/releasing.md) — regenerating frontend/bindings and committing it now would build from a tree that is not the tagged commit; an owner decision is required (fix forward and cut the next tag); the workflow never mutates the tagged tree (#73 refuses post-tag)" "$log"
  fi
  say "  .. known blocker: bindings drift — regenerating (make bindings) and committing (#73)"
  make bindings >>"$log" 2>&1 || return 1
  git add frontend/bindings >>"$log" 2>&1 || return 1
  # Commit only when the regen staged something; a real commit failure (e.g. a rejecting
  # hook) propagates to phase_run (PR #100 R4b class) instead of being swallowed.
  git diff --cached --quiet >>"$log" 2>&1 \
    || git commit -q -m "Release $tag: regenerate frontend/bindings (#73)" >>"$log" 2>&1 || return 1
  return 0
}

# ---------------------------------------------------------------- the executor's recovery loop (R3/R4)
# Consultable phases are R10's [A] class: bump, restructure, release-pr, local.
CONSULTABLE=" bump restructure release-pr local "

phase_run() { # <phase> <command-description> <command...>
  local phase="$1" desc="$2"; shift 2
  local log="$statedir/logs/$phase.log"
  mkdir -p "$statedir/logs"
  local handlers=0 rc answer arc action diagnosis base arg n why
  while :; do
    "$@" >"$log" 2>&1
    rc=$?
    cat "$log"
    [[ $rc -eq 0 ]] && return 0

    # Hard-stop signatures first (R5): a hard stop never consults, whatever the surface error.
    if grep -q 'already exists' "$log"; then
      do_stop "$phase" "hard stop: $(grep -m1 'already exists' "$log" | sed 's/^FAIL: //') — an existing release or tag is never deleted or overwritten; resolve it by hand" "$log"
    fi
    if grep -qi 'signing identity' "$log"; then
      do_stop "$phase" "hard stop: the signing identity is missing — create it once with scripts/release-cert.sh and back it up" "$log"
    fi

    # The static known-blocker table (R10): handled without the AI.
    if grep -q 'Kai is running' "$log" && pgrep -x Kai >/dev/null 2>&1; then
      if [[ $quit_kai -eq 1 ]]; then
        handlers=$((handlers + 1))
        [[ $handlers -le 2 ]] || do_stop "$phase" "Kai is still running after two quit attempts" "$log"
        handler_quit_kai
        continue
      fi
      do_stop "$phase" "Kai is running: re-run with --quit-kai to have the static handler quit it (the workflow never quits Kai unasked)" "$log"
    fi
    if grep -q 'frontend/bindings' "$log"; then
      handlers=$((handlers + 1))
      [[ $handlers -le 2 ]] || do_stop "$phase" "bindings drift persists after regeneration (#73)" "$log"
      handler_regen_bindings "$phase" "$log" || do_stop "$phase" "make bindings / commit failed (#73)" "$log"
      continue
    fi

    # Unknown failure: one AI consultation per event, cap 2 per phase (R3).
    case " $CONSULTABLE " in
      *" $phase "*) ;;
      *) do_stop "$phase" "phase failed and its failure class does not consult the AI" "$log" ;;
    esac
    n="$(consult_count "$phase")"
    if [[ "$n" -ge 2 ]]; then
      do_stop "$phase" "still failing after $n consultations (cap 2 per phase); fix the cause and resume" "$log"
    fi
    answer="$(run_tool release-ask-ai.sh "$phase" "$desc" "$log" 2>/dev/null)"
    arc=$?
    if [[ $arc -ne 0 || -z "$answer" ]]; then
      do_stop "$phase" "no answer from the consultation (fail-closed: a missing, failing or unparseable AI is a stop)" "$log"
    fi
    action="$(printf '%s' "$answer" | jq -r '.action // empty' 2>/dev/null || true)"
    diagnosis="$(printf '%s' "$answer" | jq -r '.diagnosis // empty' 2>/dev/null || true)"
    [[ -n "$action" ]] || do_stop "$phase" "the consultation's answer is not a {diagnosis,action,argument,reason} object (fail-closed)" "$log"
    record_consult "$phase" "$action"
    base="$action"; arg=""
    [[ "$base" != *" "* ]] || { arg="${base#* }"; base="${base%% *}"; }
    case "$base" in
      retry)
        say "  .. AI: retry — ${diagnosis:0:200}"
        continue
        ;;
      stop)
        do_stop "$phase" "the AI said stop: $diagnosis" "$log"
        ;;
      wait)
        say "  .. AI: wait — pausing once (bounded), then re-running the phase"
        sleep 60
        continue
        ;;
      run-handler)
        case "$arg" in
          quit-kai)
            [[ $quit_kai -eq 1 ]] || do_stop "$phase" "run-handler refused: quit-kai is dispatchable only with --quit-kai" "$log"
            handler_quit_kai
            continue
            ;;
          regen-bindings)
            handler_regen_bindings "$phase" "$log" || do_stop "$phase" "run-handler regen-bindings failed (#73)" "$log"
            continue
            ;;
          *)
            do_stop "$phase" "run-handler refused: no static handler named '$arg' (known handlers: quit-kai, regen-bindings)" "$log"
            ;;
        esac
        ;;
      propose-fix-pr)
        issue_comment "$phase" propose-fix-pr "The AI proposed a fix PR for phase $phase."$'\n'"diagnosis: $diagnosis"$'\n'"Open the fix PR off origin/master (never release/$tag); the release resumes only after it merges."
        do_stop "$phase" "propose-fix-pr recorded on the checklist issue: open the fix PR off origin/master; resume after it merges" "$log"
        ;;
      *)
        do_stop "$phase" "refused: the AI's action '$action' is not in the allowlist (retry | wait | run-handler <name> | propose-fix-pr | stop); the executor never runs it" "$log"
        ;;
    esac
  done
}

# ---------------------------------------------------------------- phases (R10)
ph_needs() {
  printf '\n== phase 0: needs (static checks before anything starts)\n'
  if phase_done needs; then say "  .. needs already done (checklist issue: #${ISSUE:-?})"; return 0; fi
  local ident="${KAI_RELEASE_SIGN_IDENTITY:-Kai Release}"
  say "  needs the owner to have, before anything starts:"
  say "    - the signing identity \"$ident\" in the keychain (scripts/release-cert.sh, once)"
  say "    - the key backed up: bin/release/kai-release-key.backed-up exists"
  say "    - gh push rights to $repo"
  say "    - Kai not running (or --quit-kai)"
  if [[ ! -f "$root/bin/release/kai-release-key.backed-up" ]]; then
    do_stop needs "the signing-key backup marker bin/release/kai-release-key.backed-up is missing: back the .p12 up (docs/releasing.md) and touch that file"
  fi
  if ! KAI_SIGN_IDENTITY="$ident" run_tool codesign-app.sh --check >/dev/null 2>&1; then
    do_stop needs "no signing identity \"$ident\" in the keychain: create it once with scripts/release-cert.sh, back it up, and touch bin/release/kai-release-key.backed-up"
  fi
  if [[ "$(gh api "repos/$repo" --jq .permissions.push 2>/dev/null || true)" != "true" ]]; then
    do_stop needs "gh cannot push to $repo (a release needs push rights)"
  fi
  if pgrep -x Kai >/dev/null 2>&1; then
    if [[ $quit_kai -eq 1 ]]; then handler_quit_kai; else
      do_stop needs "Kai is running and this run was not given --quit-kai; quit it or re-run with --quit-kai"
    fi
  fi
  local existing
  existing="$(gh issue list --repo "$repo" --search "Release checklist: $tag" --json number,title,state 2>/dev/null \
    | jq -r -s --arg t "Release checklist: $tag" 'flatten | map(select(.title == $t)) | .[0].number // empty' 2>/dev/null || true)"
  if [[ -n "$existing" ]]; then
    ISSUE="$existing"
    say "  .. checklist issue exists: #$ISSUE"
  else
    ISSUE="$(gh issue create --repo "$repo" --title "Release checklist: $tag" --body-file "$root/docs/release-checklist.md" 2>/dev/null | tail -n1 | sed 's#.*/\([0-9][0-9]*\)$#\1#')"
    [[ -n "$ISSUE" ]] || do_stop needs "could not open the checklist issue (gh issue create failed)"
    say "  ok  checklist issue #$ISSUE opened"
  fi
  st_update '.issue = $n' --arg n "$ISSUE"
  mark_done needs
}

ph_preflight() {
  printf '\n== phase 1: preflight\n'
  if phase_done preflight; then say "  .. preflight already done"; return 0; fi
  local log="$statedir/logs/preflight.log"
  mkdir -p "$statedir/logs"
  if ! run_tool release-preflight.sh "$tag" >"$log" 2>&1; then
    cat "$log"
    do_stop preflight "release-preflight.sh $tag failed — fix every FAIL line first (docs/release-checklist.md, Pre-flight)" "$log"
  fi
  cat "$log"
  st_update '.releaseCommit = $c' --arg c "$(git rev-parse HEAD 2>/dev/null || true)"
  local warns; warns="$(grep '  WARN  ' "$log" || true)"
  if [[ -n "$warns" ]]; then
    issue_comment preflight warn "release-preflight.sh reported WARN rows — recorded here per the checklist:"$'\n'"$warns"
    say "  .. WARN row(s) recorded in the checklist issue"
  fi
  mark_done preflight
}

ph_changelog() {
  printf '\n== phase 2: changelog (coverage + Known Issues)\n'
  if phase_done changelog; then say "  .. changelog already done"; return 0; fi
  local log="$statedir/logs/changelog.log"
  mkdir -p "$statedir/logs"
  # Known-issue candidates first: the owner needs them to curate (docs/releasing.md "Known issues").
  local cands
  cands="$(gh issue list --repo "$repo" --label bug --state open --json number,title,state 2>/dev/null | jq -r -s 'flatten | .[] | "- #\(.number) \(.title) (\(.state)) [bug]"' 2>/dev/null || true)"
  cands="$cands
$(gh issue list --repo "$repo" --label known-issue --state open --json number,title,state 2>/dev/null | jq -r -s 'flatten | .[] | "- #\(.number) \(.title) (\(.state)) [known-issue]"' 2>/dev/null || true)
- standing: the Accessibility / signing-identity warning from docs/releasing.md (standing lines)"
  if ! run_tool changelog-check.sh --strict >"$log" 2>&1; then
    cat "$log"
    local uncovered; uncovered="$(grep -oE '#[0-9]+' "$log" | sort -u | tr '\n' ' ')"
    do_stop changelog "uncovered PRs need a changelog entry or a skip line (${uncovered:-see log}). For each PR: add an entry citing its number or issue, or add under [Unreleased] a line like: <!-- changelog-skip: release tooling, no user-facing change: #N -->. Changelog prose is human-authored (docs/releasing.md). Next release, also add: <!-- changelog-skip: release tooling, no user-facing change: #<this release PR> --> (a PR cannot carry its own line)." "$log"
  fi
  cat "$log"
  local unreleased bullet none
  unreleased="$(awk '/^## \[Unreleased\]/{u=1;next} u && /^## \[/{u=0} u{print}' "$root/CHANGELOG.md")"
  if printf '%s\n' "$unreleased" | awk '/^### Known Issues/{k=1;next} /^### /{k=0} k && /^[[:space:]]*- /{f=1} END{exit f?0:1}'; then bullet=1; else bullet=0; fi
  # The none-marker counts only inside [Unreleased] (PR #100 R2): a stale marker under a
  # released version section says nothing about THIS release and must not satisfy the gate.
  if printf '%s\n' "$unreleased" | grep -q 'known-issues: none'; then none=1; else none=0; fi
  if [[ $bullet -eq 0 && $none -eq 0 ]]; then
    do_stop changelog "the [Unreleased] section has no curated Known Issues: add a '### Known Issues' subsection with one line per real, still-open issue (candidates below), or, when there are none, add the marker: <!-- known-issues: none $(date +%Y-%m-%d) -->. Candidates: $cands Next release, also add: <!-- changelog-skip: release tooling, no user-facing change: #<this release PR> -->." "$log"
  fi
  issue_comment changelog known-issues "Known-issue candidates (confirm each is still live on this build; standing lines from docs/releasing.md apply):"$'\n'"$cands"
  mark_done changelog
}

ph_branch() {
  printf '\n== phase 3: release branch\n'
  if phase_done branch; then say "  .. branch already done"; return 0; fi
  local rb; rb="$(git ls-remote origin "refs/heads/release/$tag" 2>/dev/null | awk 'NR==1{print $1}')"
  if [[ -n "$rb" ]]; then
    git fetch -q origin 2>/dev/null || true
    if git merge-base --is-ancestor "$rb" refs/remotes/origin/master 2>/dev/null; then
      say "  .. leftover branch release/$tag is merged into master (#74 rule): skipping phases 3-6 and deleting it"
      gh api -X DELETE "repos/$repo/git/refs/heads/release/$tag" >/dev/null 2>&1 \
        || do_stop branch "could not delete the leftover branch release/$tag (gh api -X DELETE failed)"
      mark_done branch bump restructure release-pr
      return 0
    fi
    do_stop branch "release/$tag exists on origin but is not merged into master — a release is in progress; finish it or pick another version"
  fi
  local info; info="$(find_pr)"
  if [[ -n "$info" ]]; then
    set -- $info
    if [[ "$2" == "MERGED" ]]; then
      say "  .. release PR #$1 already merged (its branch was deleted by the merge): skipping phases 3-6 like the leftover-branch rule (#74)"
      mark_done branch bump restructure release-pr
      return 0
    fi
    do_stop branch "release/$tag has an unmerged PR #$1 but no branch on origin — resolve that by hand"
  fi
  local rc; rc="$(jq -r '.releaseCommit // ""' "$state" 2>/dev/null || true)"
  [[ -n "$rc" ]] || rc="$(git rev-parse HEAD)"
  if git rev-parse --verify -q "refs/heads/release/$tag" >/dev/null 2>&1; then
    git checkout -q "release/$tag" || do_stop branch "git checkout release/$tag failed"
  else
    git checkout -q -b "release/$tag" "$rc" || do_stop branch "git checkout -b release/$tag $rc failed"
  fi
  git push -q origin "release/$tag" || do_stop branch "git push of release/$tag failed"
  say "  ok  release/$tag created at ${rc:0:7} and pushed"
  mark_done branch
}

ph_bump() {
  printf '\n== phase 4: version bump\n'
  if phase_done bump; then say "  .. bump already done"; return 0; fi
  phase_run bump "release-bump.sh $ver" run_tool release-bump.sh "$ver"
  mark_done bump
}

restructure_apply() {
  local tmp="$root/CHANGELOG.md.rs.$$"
  awk -v ver="$ver" -v date="$(date +%Y-%m-%d)" '
    BEGIN { printed = 0 }
    /^## \[Unreleased\]/ {
      if (!printed) { print "## [Unreleased]"; print ""; print "## [" ver "] - " date; printed = 1 }
      next
    }
    { print }
  ' "$root/CHANGELOG.md" >"$tmp" || { rm -f "$tmp"; return 1; }
  mv "$tmp" "$root/CHANGELOG.md"
  run_tool release-check-version.sh "$ver" . >/dev/null
}

ph_restructure() {
  printf '\n== phase 5: changelog restructure\n'
  if phase_done restructure; then say "  .. restructure already done"; return 0; fi
  if run_tool release-check-version.sh "$ver" . >/dev/null 2>&1 && grep -q '^## \[Unreleased\]' "$root/CHANGELOG.md"; then
    say "  .. version files and CHANGELOG already say $ver (postcondition holds)"
    mark_done restructure
    return 0
  fi
  phase_run restructure "restructure CHANGELOG.md for $ver (release-check-version.sh $ver .)" restructure_apply
  mark_done restructure
}

release_pr_apply() {
  # PR #100 R4b: every operation's exit status must reach phase_run — a rejecting commit
  # hook, a refused push, a failing gh pr create — so the R3/R4 consult-and-retry loop can
  # run; the closing rm must never mask them. Fail fast: nothing runs after a failure.
  git add -A || return 1
  git diff --cached --quiet || git commit -q -m "Release $tag: version bump and changelog restructure" || return 1
  git push -q origin "release/$tag" || return 1
  local bf rc
  bf="$(mktemp "${TMPDIR:-/tmp}/release-pr.XXXXXX")"
  printf 'Release %s: version bump and changelog restructure.\nManaged by scripts/release.sh (checklist issue #%s).\n' "$tag" "${ISSUE:-}" >"$bf"
  gh pr create --repo "$repo" --base master --head "release/$tag" --title "Release $tag" --body-file "$bf" >/dev/null
  rc=$?
  rm -f "$bf"
  return "$rc"
}

ph_release_pr() {
  printf '\n== phase 6: release PR\n'
  if phase_done release-pr; then say "  .. release-pr already done"; return 0; fi
  local info; info="$(find_pr)"
  if [[ -n "$info" ]]; then
    set -- $info
    say "  .. PR for release/$tag already exists: #$1 ($2)"
    st_update '.pr = $n | .headSha = $h' --arg n "$1" --arg h "$3"
    mark_done release-pr
    return 0
  fi
  phase_run release-pr "gh pr create --repo $repo --base master --head release/$tag" release_pr_apply
  info="$(find_pr)"
  [[ -n "$info" ]] || do_stop release-pr "no PR found for release/$tag after creating it"
  set -- $info
  st_update '.pr = $n | .headSha = $h' --arg n "$1" --arg h "$3"
  say "  ok  release PR #$1 opened"
  mark_done release-pr
}

ph_review() {
  printf '\n== phase 7: review gate (CI + PR-Agent, rule R1)\n'
  if phase_done review; then say "  .. review already done"; return 0; fi
  local info why
  info="$(find_pr)"
  [[ -n "$info" ]] || do_stop review "no release PR found for release/$tag"
  set -- $info
  local prn="$1" prst="$2" prhead="$3"
  if [[ "$prst" == "MERGED" ]]; then
    say "  .. PR #$prn already merged — the review gate was settled by its merge"
    st_update '.pr = $n | .headSha = $h' --arg n "$prn" --arg h "$prhead"
    mark_done review
    return 0
  fi
  if ! why="$(ci_green_on "$prhead")"; then
    do_stop review "hard stop: CI is not green on the release commit ${prhead:0:7}: $why"
  fi
  if ! why="$(agent_gate "$prn" "$prhead")"; then
    do_stop review "hard stop: the PR-Agent review gate failed on PR #$prn: $why"
  fi
  say "  ok  CI green and PR-Agent review passed on PR #$prn (score >= 90, no unresolved findings)"
  st_update '.pr = $n | .headSha = $h' --arg n "$prn" --arg h "$prhead"
  mark_done review
}

ph_merge() {
  printf '\n== phase 8: merge\n'
  if phase_done merge; then say "  .. merge already done"; return 0; fi
  local info why
  info="$(find_pr)"
  [[ -n "$info" ]] || do_stop merge "no release PR found for release/$tag"
  set -- $info
  local prn="$1" prst="$2" prhead="$3"
  if [[ "$prst" != "MERGED" ]]; then
    gh pr merge "$prn" --repo "$repo" --merge --delete-branch >/dev/null 2>&1 \
      || do_stop merge "gh pr merge failed for PR #$prn (a conflict needs the owner; docs/release-checklist.md step 13)"
    info="$(find_pr)"
    set -- $info
    prst="$2"
    [[ "$prst" == "MERGED" ]] || do_stop merge "PR #$prn did not end up MERGED (state: $prst)"
  fi
  local msha; msha="$(merge_sha_of "$1" "$3")"
  [[ -n "$msha" ]] || do_stop merge "could not determine the merge commit of PR #$1"
  if ! why="$(ci_green_on "$msha")"; then
    do_stop merge "hard stop: CI is red on the merge commit ${msha:0:7}: $why"
  fi
  say "  ok  PR #$1 merged; CI green on the merge commit ${msha:0:7}"
  st_update '.pr = $n | .mergeSha = $m' --arg n "$1" --arg m "$msha"
  mark_done merge
}

ph_tag() {
  printf '\n== phase 9: tag\n'
  if phase_done tag; then say "  .. tag already done"; return 0; fi
  local msha; msha="$(jq -r '.mergeSha // ""' "$state" 2>/dev/null || true)"
  if [[ -z "$msha" ]]; then
    local info; info="$(find_pr)"
    [[ -n "$info" ]] || do_stop tag "no release PR to derive the merge commit from"
    set -- $info
    msha="$(merge_sha_of "$1" "$3")"
  fi
  [[ -n "$msha" ]] || do_stop tag "could not determine the merge commit to tag"
  # Peel the annotated tag to compare commits; a tag is never moved or deleted (R5).
  local rtag; rtag="$(git ls-remote origin "refs/tags/$tag^{}" 2>/dev/null | awk 'NR==1{print $1}')"
  [[ -n "$rtag" ]] || rtag="$(git ls-remote origin "refs/tags/$tag" 2>/dev/null | awk 'NR==1{print $1}')"
  if [[ -n "$rtag" ]]; then
    [[ "$rtag" == "$msha" ]] || do_stop tag "hard stop: $tag already exists at ${rtag:0:7}, not the merge commit ${msha:0:7} — a tag is never moved; resolve it by hand"
    say "  .. $tag already at the merge commit"
    mark_done tag
    return 0
  fi
  # The remote lacks the tag, but an earlier run may have left it LOCALLY at the merge
  # commit (its push after `git tag -a` was interrupted). Push that existing tag — never
  # re-create it (`git tag -a` fails "already exists") and never move one at a wrong
  # commit (R5; PR #100 R1).
  local ltag; ltag="$(git rev-parse -q --verify "$tag^{}" 2>/dev/null || true)"
  if [[ -n "$ltag" ]]; then
    [[ "$ltag" == "$msha" ]] || do_stop tag "hard stop: local $tag already exists at ${ltag:0:7}, not the merge commit ${msha:0:7} — a tag is never moved; resolve it by hand"
    git push -q origin "$tag" || do_stop tag "git push of $tag failed"
    say "  ok  $tag already created locally at ${msha:0:7}; pushed it"
    mark_done tag
    return 0
  fi
  git tag -a "$tag" -m "$tag" "$msha" || do_stop tag "git tag -a $tag failed"
  git push -q origin "$tag" || do_stop tag "git push of $tag failed"
  say "  ok  tagged ${msha:0:7} as $tag and pushed"
  mark_done tag
}

ph_local() {
  printf '\n== phase 10: local build, verify, package (%s)\n' "$( [[ "$posture" == "dry-run" ]] && echo "--dry-run, no draft" || echo "--yes, draft" )"
  # No state.done fast-path here: the phase's postcondition depends on the posture (R8), so a
  # dry-run-completed local re-runs at --yes when the draft does not exist yet. The probe is
  # the postcondition; when it holds the phase is a no-op (R10 p10's "may rebuild" note).
  local mode="--yes"
  [[ "$posture" == "dry-run" ]] && mode="--dry-run"
  if [[ "$mode" == "--dry-run" ]]; then
    if [[ -f "$statedir/RELEASE_NOTES.md" && -d "$statedir/Kai.app" && -f "$statedir/assets/SHA256SUMS" ]]; then
      say "  .. packaged dry-run output already present in $statedir"
      mark_done local
      return 0
    fi
  else
    local n_assets
    n_assets="$(gh release view "$tag" --repo "$repo" --json assets 2>/dev/null | jq -r '.assets | length' 2>/dev/null || true)"
    if [[ "$n_assets" == "4" ]]; then
      say "  .. a release for $tag already exists with its 4 assets"
      mark_done local
      return 0
    fi
  fi
  phase_run local "release-local.sh $tag $mode" run_tool release-local.sh "$tag" "$mode"
  mark_done local
}

ph_smoke() {
  printf '\n== phase 11: smoke\n'
  if phase_done smoke; then say "  .. smoke already recorded ($(jq -r '.smoke // ""' "$state" 2>/dev/null))"; return 0; fi
  local v
  if [[ $smoke_done -eq 1 ]]; then
    v="done (owner-asserted $(date +%Y-%m-%d))"
    st_update '.smoke = $v' --arg v "$v"
    issue_comment smoke done "smoke matrix: $v — recorded by --smoke-done"
    mark_done smoke
    say "  ok  smoke recorded: $v"
    return 0
  fi
  if [[ $smoke -eq 1 ]]; then
    st_update '.smoke = $v' --arg v "awaiting-owner"
    issue_comment smoke awaiting-owner "smoke matrix awaiting the owner: run docs/release-checklist.md §Manual smoke matrix on $statedir/Kai.app, then resume with --smoke-done"
    say ""
    say "Smoke matrix awaiting the owner (docs/release-checklist.md, Manual smoke matrix)."
    say "Run it on $statedir/Kai.app, then record it:"
    say "resume: scripts/release.sh $tag --smoke-done --publish"
    notify "smoke matrix awaiting the owner"
    exit 0
  fi
  # PR #100 R3: awaiting-owner is an owner gate — a resume without --smoke-done must refuse;
  # the record is never downgraded to skipped and the run never continues to publish.
  if [[ "$(jq -r '.smoke // ""' "$state" 2>/dev/null)" == "awaiting-owner" ]]; then
    do_stop smoke "the smoke matrix is still awaiting the owner (docs/release-checklist.md §Manual smoke matrix, on $statedir/Kai.app) — resume with --smoke-done to record it"
  fi
  st_update '.smoke = $v' --arg v "skipped"
  issue_comment smoke skipped "smoke: skipped (--smoke not given this run; recorded per the checklist)"
  mark_done smoke
  say "  .. smoke recorded skipped (no --smoke)"
}

ph_publish() {
  printf '\n== phase 12: publish\n'
  if phase_done publish; then say "  .. publish already done"; return 0; fi
  # jq -r prints a boolean false as "false"; "// empty" would swallow it.
  local d
  d="$(gh release view "$tag" --repo "$repo" --json isDraft 2>/dev/null | jq -r '.isDraft' 2>/dev/null || true)"
  if [[ "$d" == "false" ]]; then
    say "  .. $tag is already published"
    mark_done publish
    return 0
  fi
  [[ "$d" == "true" ]] || do_stop publish "hard stop: no draft release $tag found to publish (gh release view said '${d:-nothing}')"
  gh release edit "$tag" --repo "$repo" --draft=false >/dev/null 2>&1 \
    || do_stop publish "hard stop: gh release edit failed — never delete or overwrite; inspect by hand"
  d="$(gh release view "$tag" --repo "$repo" --json isDraft 2>/dev/null | jq -r '.isDraft' 2>/dev/null || true)"
  [[ "$d" == "false" ]] || do_stop publish "hard stop: $tag is still a draft after the edit — inspect by hand"
  say "  ok  $tag published"
  mark_done publish
}

ph_verify() {
  printf '\n== phase 13: verify the published artifact (checklist step 28)\n'
  if phase_done verify-published; then say "  .. verify-published already done"; return 0; fi
  # PR #100 F2: both throwaway dirs below are cleaned on EVERY exit — success, a stop,
  # an unknown failure (the EXIT trap runs cleanup), and a signal mid-verify, where no
  # end-of-function rm could ever run: the TERM trap cleans and exits 143 (128+15).
  trap 'cleanup; exit 143' TERM
  local vdir; vdir="$(mktemp -d "${TMPDIR:-/tmp}/release-verify.XXXXXX")"
  VERIFY_TMP="$vdir"
  local z="Kai-$ver-darwin-arm64.zip" uz="updater-Kai-$ver-darwin-arm64.zip" dmg="Kai-$ver-darwin-arm64.dmg"
  ( cd "$vdir" && gh release download "$tag" --repo "$repo" >/dev/null 2>&1 ) \
    || do_stop verify-published "gh release download $tag failed"
  [[ -f "$vdir/$z" && -f "$vdir/$uz" && -f "$vdir/$dmg" && -f "$vdir/SHA256SUMS" ]] \
    || do_stop verify-published "the published assets for $tag are incomplete (wanted the zip, the updater zip, the dmg and SHA256SUMS)"
  ( cd "$vdir" && shasum -a 256 -c SHA256SUMS >/dev/null 2>&1 ) \
    || do_stop verify-published "SHA256SUMS verification failed — the published artifact is wrong; the owner decides"
  cmp -s "$vdir/$z" "$vdir/$uz" \
    || do_stop verify-published "the updater zip is not byte-identical to the release zip"
  mkdir -p "$vdir/x"
  ditto -x -k "$vdir/$z" "$vdir/x" \
    || do_stop verify-published "ditto extraction of the published zip failed"
  run_tool release-verify.sh "$vdir/x/Kai.app" "$ver" \
    || do_stop verify-published "release-verify.sh failed on the downloaded Kai.app"
  local mnt
  mnt="$(hdiutil attach -readonly -nobrowse "$vdir/$dmg" 2>/dev/null | awk -F'\t' '$3 != "" { print $3 }' | tail -1)"
  [[ -n "$mnt" ]] || do_stop verify-published "could not mount the published disk image"
  if ! diff -r "$mnt/Kai.app" "$vdir/x/Kai.app" >/dev/null 2>&1; then
    hdiutil detach "$mnt" >/dev/null 2>&1 || true
    do_stop verify-published "the dmg's Kai.app differs from the zip's"
  fi
  hdiutil detach "$mnt" >/dev/null 2>&1 || true
  local idest; idest="$(mktemp -d)"
  VERIFY_TMP="$vdir $idest"
  run_tool install-release.sh "$tag" --dest "$idest" --keep-permissions \
    || do_stop verify-published "install-release.sh against the published release failed"
  say "  ok  published artifact verified (SHA256SUMS, updater zip identical, zip == dmg, installer ran)"
  rm -rf "$vdir" "$idest"
  VERIFY_TMP=""
  trap - TERM
  mark_done verify-published
}

ph_closeout() {
  printf '\n== phase 14: closeout\n'
  if phase_done closeout; then say "  .. closeout already done"; return 0; fi
  if [[ -z "$ISSUE" ]]; then
    ISSUE="$(gh issue list --repo "$repo" --search "Release checklist: $tag" --json number,title,state 2>/dev/null \
      | jq -r -s --arg t "Release checklist: $tag" 'flatten | map(select(.title == $t)) | .[0].number // empty' 2>/dev/null || true)"
    [[ -n "$ISSUE" ]] || do_stop closeout "the checklist issue for $tag was not found to close"
    st_update '.issue = $n' --arg n "$ISSUE"
  fi
  issue_comment closeout evidence "Release $tag complete."$'\n'"- release: https://github.com/$repo/releases/$tag"$'\n'"- smoke: $(jq -r '.smoke // ""' "$state" 2>/dev/null)"$'\n'"- the published artifact was verified per checklist step 28"$'\n'"- owner nicety: link this issue from the release's notes"
  gh issue close "$ISSUE" --repo "$repo" >/dev/null 2>&1 || do_stop closeout "gh issue close #$ISSUE failed"
  say "  ok  checklist issue #$ISSUE closed"
  mark_done closeout
}

# ---------------------------------------------------------------- main flow
say "release.sh $tag — posture: $posture — repo: $repo"
ph_needs
ph_preflight
ph_changelog
ph_branch
ph_bump
ph_restructure
ph_release_pr
ph_review
ph_merge
ph_tag
ph_local

if [[ "$posture" == "dry-run" ]]; then
  say ""
  say "Dry-run posture complete for $tag: every phase through the local build ran; NO draft release was created."
  say "The branch, PR, merge and tag did happen — they are the release (only the GitHub Release object is gated)."
  say "Remaining owner steps:"
  say "  1. draft:   re-run as 'scripts/release.sh $tag' (default posture) to create the draft release"
  say "  2. smoke:   run the manual smoke matrix (docs/release-checklist.md) on $statedir/Kai.app; record with --smoke/--smoke-done"
  say "  3. publish: scripts/release.sh $tag --publish"
  say "resume: scripts/release.sh $tag"
  exit 0
fi

ph_smoke

if [[ "$posture" != "publish" ]]; then
  say ""
  say "Default posture complete for $tag: the draft release is created (not public) and the packaged build is verified."
  say "Remaining owner step: publish."
  say "resume: scripts/release.sh $tag --publish"
  exit 0
fi

ph_publish
ph_verify
ph_closeout
say ""
say "Release $tag complete: published, verified, checklist issue #$ISSUE closed."
say "Owner nicety: link the checklist issue from https://github.com/$repo/releases/$tag"
exit 0
