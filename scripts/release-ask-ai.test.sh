#!/usr/bin/env bash
# Tests for scripts/release-ask-ai.sh: the consultation client (issue #78). Fakes `timeout`
# and `opencode` on PATH — no network, no model, no live AI (handoff-A R9: RED/GREEN never
# needs a live model). Contract: timeout 300 opencode run --format json "<fixed prompt>",
# message a single argv; the answer is the LAST text part of the JSON envelope; any non-zero
# exit, timeout, or unparseable output, and any absent binary, is no-answer => fail-closed.
# Part of test.unit.command (issue #78, brief item 9).
#   bash scripts/release-ask-ai.test.sh
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
au="$here/release-ask-ai.sh"
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
val_is() { local v=""; [[ -f "$2" ]] && v="$(tr -d '[:space:]' <"$2")"
  if [[ "$v" == "$3" ]]; then echo "ok   $1"; else echo "FAIL $1 ($(basename "$2") is '$v', wanted '$3')"; fail=1; fi; }

SD="$tmp/state"; mkdir -p "$SD" "$tmp/wd"
T2="$tmp/tbin"; O2="$tmp/obin"; mkdir -p "$T2" "$O2"

cat >"$T2/timeout" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$*" >>"$FAKE_STATE_DIR/timeout.log"
if [[ -e "$FAKE_STATE_DIR/timeout-kill" ]]; then exit 124; fi
shift
exec "$@"
EOF

cat >"$O2/opencode" <<'EOF'
#!/usr/bin/env bash
printf '%s\n' "$#" >"$FAKE_STATE_DIR/oc.argc"
i=0
for a in "$@"; do i=$((i + 1)); printf 'ARG%d:%s\n' "$i" "$a" >>"$FAKE_STATE_DIR/oc.args"; done
case "${FAKE_OC:-ok}" in
  ok) cat <<'J'
{"type":"message","parts":[{"type":"text","text":"thinking about it"},{"type":"text","text":"{\"diagnosis\":\"d\",\"action\":\"retry\",\"argument\":\"\",\"reason\":\"r\"}"}]}
J
    ;;
  bad)     printf 'this is not JSON {{{\n' ;;
  err)     printf '{"type":"error","timestamp":1792454525968,"sessionID":"","error":{"type":"unknown","message":"UnexpectedStatus: 500"}}\n'; exit 1 ;;
  notext)  printf '{"type":"message","parts":[{"type":"status","text":"x"}]}\n' ;;
esac
exit 0
EOF

chmod +x "$T2/timeout" "$O2/opencode"

# The fixed prompt's inputs: a phase, its command, and a log longer than 200 lines.
LOG="$SD/phase.log"
i=1; while [[ "$i" -le 250 ]]; do printf 'LINE%03d\n' "$i" >>"$LOG"; i=$((i + 1)); done

arun() { # <PATH-prepend> [FAKE_OC via env] — invoke release-ask-ai.sh as the executor would
  out="$(cd "$tmp/wd" && env PATH="$1:$PATH" FAKE_STATE_DIR="$SD" \
    ${ar_env[@]+"${ar_env[@]}"} "$au" bump 'release-bump.sh 0.5.1' "$LOG" </dev/null 2>&1)"
  code=$?
}
ar_env=()

# A01 — the script under test must exist (the RED gate).
if [[ -x "$au" ]]; then
  echo "ok   A01 scripts/release-ask-ai.sh exists and is executable"
else
  echo "FAIL A01 scripts/release-ask-ai.sh exists and is executable (feature absent: F implements it)"
  fail=1
fi

# A02 — happy path: the answer is the LAST text part only.
rm -f "$SD"/oc.* "$SD"/timeout.log
arun "$T2:$O2"
exit_is     "A02/R9: a good envelope exits 0"                                   0
has         "A02/R9: the assistant's answer is printed"                          '"action":"retry"'
hasnt       "A02/R9: only the LAST text part is taken (earlier parts dropped)"   'thinking about it'

# A03/A04 — the invocation shape and the prompt.
val_is      "A03/R9: the message is a single argv (run --format json + 1 arg)"   "$SD/oc.argc" '4'
fhas        "A03/R9: subcommand is 'run'"                                       "$SD/oc.args" '^ARG1:run$'
fhas        "A03/R9: output format is json"                                     "$SD/oc.args" '^ARG2:--format$'
fhas        "A03/R9: --format's value is json"                                  "$SD/oc.args" '^ARG3:json$'
fhas        "A03: the prompt carries the phase"                                  "$SD/oc.args" 'bump'
fhas        "A03: the prompt carries the failing command"                        "$SD/oc.args" 'release-bump\.sh 0\.5\.1'
fhas        "A03/R9: the prompt carries the log TAIL"                            "$SD/oc.args" 'LINE250'
fhasnt      "A03/R9: the prompt does not carry lines older than the last 200"    "$SD/oc.args" 'LINE001'
fhas        "A04/R9: the call is wrapped in timeout"                             "$SD/timeout.log" '^300 '
fhas        "A04/R9: the timeout wraps opencode run --format json"               "$SD/timeout.log" 'opencode run --format json'

# A05-A09 — every failure mode is fail-closed: no answer, non-zero exit (R9).
rm -f "$SD"/oc.* "$SD"/timeout.log
ar_env=(FAKE_OC=bad)
arun "$T2:$O2"
exit_nonzero "A05/R9: unparseable output is no-answer (fail-closed)"
has         "A05/R9: the no-answer condition is reported"                        '[Nn]o answer'

rm -f "$SD"/oc.* "$SD"/timeout.log
ar_env=(FAKE_OC=err)
arun "$T2:$O2"
exit_nonzero "A06/R9: opencode exiting non-zero (the live 500) is no-answer"
has         "A06/R9: the no-answer condition is reported"                        '[Nn]o answer'

rm -f "$SD"/oc.* "$SD"/timeout.log
ar_env=()
: >"$SD/timeout-kill"
arun "$T2:$O2"
rm -f "$SD/timeout-kill"
exit_nonzero "A07/R9: a timeout is no-answer"
has         "A07/R9: the no-answer condition is reported"                        '[Nn]o answer'

rm -f "$SD"/oc.* "$SD"/timeout.log
ar_env=()
arun "$T2"        # PATH without the opencode dir: the binary is absent
exit_nonzero "A08/R9: an absent binary is no-answer (fail-closed, not a hang)"
has         "A08/R9: the no-answer condition is reported"                        '[Nn]o answer'

rm -f "$SD"/oc.* "$SD"/timeout.log
ar_env=(FAKE_OC=notext)
arun "$T2:$O2"
exit_nonzero "A09/R9: an envelope with no text part is no-answer"
has         "A09/R9: the no-answer condition is reported"                        '[Nn]o answer'

[[ $fail -eq 0 ]] && echo "PASS" || { echo "FAILED"; exit 1; }
