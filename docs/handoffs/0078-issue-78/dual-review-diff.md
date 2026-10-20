## Implementation Review (Round 1)

### BLOCK findings

**[B-1] frontend/package.json (claimed) — the handoff's Post-GATE addendum asserts the branch carries a `frontend/package.json` edit (`NODE_OPTIONS=--no-experimental-webstorage` in the test script), but that file appears nowhere in the diff, and the diff's stated exclusions are only `docs/handoffs/**` and `*.prompt.txt` — why it blocks — the F self-check's "FULL SUITE EXIT=0" claim depends on it: the addendum's own evidence says that without the env var the frontend fails 284/870, and `pnpm --dir frontend test` is in `test.unit.command` (AC5). Either the diff is incomplete (the review is not reviewing what will be merged) or the claim is false and the suite is red. The two parts of this prompt contradict each other — remediation: include the `frontend/package.json` change in the diff (or show the base already carries it via merged #99), and re-verify the full `test.unit.command` chain on the exact reviewed tree.

### Needs-verification findings

**[NV-1] scripts/release.sh:~ph_review/ph_merge (`ci_green_on`) — the claim that CI on a just-created PR head and on a just-merged merge commit is already `completed` with a green conclusion when checked microseconds after `gh pr create` / `gh pr merge` — the evidence that would settle it is real GitHub check-runs timing behavior (documentation or a live observation) plus any wait/poll code; none is present in the diff, and the fakes return instant `completed success`, so the hazard is untested. If CI is queued, this takes the hard-stop path on every real run.
**[NV-2] scripts/release-ask-ai.sh:70 — the claim that `opencode run --format json` emits an envelope whose text parts look like `.parts[] | select(.type=="text") | .text` and that the model's last text part is parseable JSON — the evidence is `opencode` CLI documentation/source for the `--format json` output schema; only the fake shape is exercised.
**[NV-3] scripts/release.sh `merge_sha_of` — the claim that real `gh pr view --json mergeCommit,headRefOid` reports `mergeCommit.oid` for a `--merge` merge and that `release-gates.sh`'s on-master check holds for it — the evidence is gh's documented JSON shape plus `scripts/release-gates.sh` source, neither shown.
**[NV-4] .aftersight/pipeline.config.json:pluginSuite/pluginTypecheck — the claim that `null` "never runs" and that the npm-run defaults would otherwise exit 254 in a repo with no root package.json — the evidence is the aftersight pipeline config schema/`suite-environment.ts` source, not shown.
**[NV-5] scripts/release.sh `agent_gate` — the claim that real PR-Agent comments carry the `pr-agent-review-state:v1` machine block and the `<strong>Score</strong>: N</td>` HTML shape parsed here — the evidence is a real PR-Agent review comment excerpt from this repo.

### WARN findings

**[W-1] scripts/release.sh `ph_needs` — the printed needs-the-owner list omits "the codesign key-access prompt allowed once", which brief item 3 explicitly includes in the static pre-start list (it appears only in docs/releasing.md prose) — recommendation: add the line to the printed list, or journal an A amendment; as written it is spec drift against the brief.
**[W-2] .aftersight/pipeline.config.json — `pluginSuite: null`, `pluginTypecheck: null` are a second `.aftersight/` change beyond brief item 9's sanctioned single amendment ("This is the only `.aftersight/` change in the diff"); disclosed in the addendum but never amended into the brief per the brief's own amendment procedure — recommendation: land the decisions.md entry as a brief amendment before merge.
**[W-3] scripts/release.sh `issue_comment` — all comment failures are swallowed (`|| true`), including the do_stop checklist comment; a `gh` auth failure means a "loud stop" is silent on GitHub and nobody is told — recommendation: at least print a warning line when the comment fails.
**[W-4] scripts/release.sh `phase_run` — the `handlers` counter is shared across blocker kinds, so one quit-kai attempt plus one bindings regeneration already hits the "≤ 2" ceiling for unrelated blockers; also `run-handler quit-kai` invoked by the AI bypasses the handler-count cap — recommendation: count per handler.
**[W-5] scripts/release.sh `--smoke` in `--dry-run` posture — `ph_smoke` is never reached (main exits at the dry-run boundary first), so `--dry-run --smoke` silently ignores the smoke request; the AC4 `--smoke` test only covers the default posture — recommendation: reject the combination or print that smoke recording happens after the draft.
**[W-6] scripts/release-ask-ai.sh:54 — `grep -iE "$phase|$first_word"` interpolates `first_word` (derived from the command description) unescaped into an ERE; a `.`/`*`-containing script name silently mis-matches or matches nothing — recommendation: `grep -F` for the fixed string portion.

### NIT findings

**[NIT-1] scripts/release.sh `ph_verify` — `rm -rf "$vdir"` is skipped on every stop path (no trap), leaving temp verification dirs behind.
**[NIT-2] scripts/release.sh `-h|--help` — the `sed -n '2,/^set -uo/p'` help slice assumes the header comment layout; fragile but harmless.
**[NIT-3] scripts/release.sh `ph_branch` — `set -- $info` repurposes the positional parameters inside a function, which also clobbers the caller-visible `$@` semantics if ever refactored; a local array would be safer.

### Verdict

BLOCK — 1 blocking finding ([B-1], the claimed-but-absent `frontend/package.json` change underlying the AC5 green claim); must resolve before testing starts.