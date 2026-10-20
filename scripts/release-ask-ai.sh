#!/usr/bin/env bash
# Ask the AI for a bounded diagnosis when a release.sh phase is blocked (issue #78).
#
#   scripts/release-ask-ai.sh <phase> <command> <logfile>
#
# Sends ONE fixed prompt — the phase, the failing command, the last 200 lines of the log,
# and the phase's lines from docs/release-checklist.md — through
#   timeout 300 opencode run --format json "<prompt>"
# (the message is a single argv) and prints the assistant's LAST text part on stdout, for
# the executor (scripts/release.sh) to validate against its allowlist; the AI's answer is
# advisory only and nothing it produces is ever executed here.
#
# Fail-closed (handoff-A R9): a non-zero exit (timeout included), unparseable output, an
# envelope with no text part, or an absent binary is "no answer" — a line saying so on
# stderr and a non-zero exit. Never a hang. RED/GREEN never needs a live model: the tests
# fake `timeout` and `opencode` on PATH (bare-name dispatch, R7).
set -uo pipefail

phase="${1:-}"
command_desc="${2:-}"
logfile="${3:-}"

if [[ -z "$phase" || -z "$command_desc" || -z "$logfile" ]]; then
  echo "usage: $0 <phase> <command> <logfile>" >&2
  exit 2
fi

here="$(cd "$(dirname "$0")" && pwd)"

no_answer() { # fail-closed (R9)
  echo "release-ask-ai: no answer ($1)" >&2
  exit 1
}

command -v timeout  >/dev/null 2>&1 || no_answer "timeout is not installed"
command -v opencode >/dev/null 2>&1 || no_answer "opencode is not on PATH"
command -v jq       >/dev/null 2>&1 || no_answer "jq is not installed"

# The prompt's fixed inputs. Log: only the last 200 lines (brief item 5). Checklist lines:
# best-effort context, matched on the phase name and the wrapped script's name.
log_tail=""
[[ -f "$logfile" ]] && log_tail="$(tail -n 200 "$logfile")"

checklist=""
cl="$here/../docs/release-checklist.md"
if [[ -f "$cl" ]]; then
  first_word="$(printf '%s' "$command_desc" | awk '{print $1}')"
  first_word="${first_word#scripts/}"
  checklist="$(grep -iE "$phase|$first_word" "$cl" 2>/dev/null | head -15 || true)"
fi

msg="$(printf 'The release workflow phase "%s" is blocked.\n\nFailing command: %s\n\nLast 200 lines of its log:\n%s\n\nChecklist context (docs/release-checklist.md):\n%s\n\nReply with ONE JSON object and nothing else:\n{"diagnosis": "...", "action": "retry" | "wait" | "run-handler <name>" | "propose-fix-pr" | "stop", "argument": "...", "reason": "..."}\nThe executor validates the action against that allowlist and runs only allowlisted actions itself; any other action is refused.' \
  "$phase" "$command_desc" "$log_tail" "$checklist")"

out="$(mktemp "${TMPDIR:-/tmp}/release-ask-ai.XXXXXX")"
trap 'rm -f "$out"' EXIT

# R9's exact mechanism. The message is one argv; the fake `timeout` in the harness records
# "300 opencode run --format json <msg>" and execs the rest.
if ! timeout 300 opencode run --format json "$msg" >"$out" 2>/dev/null; then
  no_answer "opencode exited non-zero (error or timeout)"
fi

# The answer is the LAST text part of the envelope; anything else in it is dropped.
answer=""
if ! answer="$(jq -r '.parts | map(select(.type == "text")) | last | .text // empty' "$out" 2>/dev/null)"; then
  no_answer "unparseable output"
fi
[[ -n "$answer" ]] || no_answer "the envelope has no text part"

printf '%s\n' "$answer"
exit 0
