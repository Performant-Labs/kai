## Implementation Review (Round 1)

### BLOCK findings

**[B-1] `.aftersight/pipeline.config.json` (diff hunk, added `"pluginSuite": null, "pluginTypecheck": null`) — the diff contains `.aftersight/` changes beyond the one the brief authorizes.** The brief (item 9) says: "`.aftersight/pipeline.config.json`'s `test.unit.command` is amended to run them so RED/GREEN are observable. **This is the only `.aftersight/` change in the diff.**" The diff adds two new keys that null out other suite configurations. Why it blocks: explicit brief constraint violated by visible diff content; nulling `pluginSuite`/`pluginTypecheck` could silently disable other pipeline stages, and neither the handoff nor the brief discloses or justifies it. Remediation: revert to the single-line `test.unit.command` amendment, or land a brief amendment (decisions.md) explaining exactly what these nulls do and why they are safe.

**[B-2] `frontend/package.json` (`"test": "NODE_OPTIONS=--no-experimental-webstorage vitest --run"`) — an undisclosed functional change to the frontend test command riding in a release-tooling diff.** The handoff's change table lists exactly: `scripts/release.sh`, `scripts/release-ask-ai.sh`, `docs/releasing.md`, `docs/release-checklist.md`, `CHANGELOG.md` — "New production code (2 files), docs (2 files), CHANGELOG (1 line)." The diff contradicts this: a sixth file is modified, changing how every frontend test invocation behaves (injecting a Node env var). Why it blocks: scope drift demonstrable from the diff vs. the handoff's own inventory; nothing in the brief or issue #78 authorizes touching the frontend test command, and it hides an environment workaround inside unrelated work. Remediation: revert, or split into its own issue/PR with its own justification (and disclose it in the handoff).

### Needs-verification findings

**[NV-1] `scripts/release-ask-ai.sh:60-70` — claim: `opencode run --format json` emits an envelope whose last text part is at `.parts[] | select(.type == "text") | .text`.** Evidence that would settle it: the `opencode` CLI documentation/source for `run --format json` output schema (or a captured real invocation). Only the fake `opencode` shape is exercised by the harness; if the real envelope nests differently, every real consultation is fail-closed (safe, but the feature never works).

**[NV-2] `scripts/release.sh` (R7 bare-name dispatch, `run_tool`) — claim: invoking wrapped scripts and `release-ask-ai.sh` by bare PATH lookup is safe in the real release environment (no shadowing binaries earlier on PATH).** Evidence: the actual PATH on the release Mac / CI, or a pinned-resolution scheme. PATH-first dispatch is the test interposition mechanism, but in production an unexpected `release-bump.sh` earlier on PATH would be silently executed.

**[NV-3] `scripts/release.sh` `ph_merge` — claim: after `gh pr merge --delete-branch`, `gh pr list --head release/vX.Y.Z` still returns the MERGED PR.** Evidence: `gh` documentation/behavior for `pr list --head` after branch deletion (the code does `set -- $info` on the possibly-empty result and indexes `$1`/`$3` unguarded; if empty, `merge_sha_of ""` runs and the failure surfaces only via the "could not determine the merge commit" stop). Real-`gh` behavior is not shown in this prompt.

**[NV-4] `scripts/release-ask-ai.sh:30-32` — claim: GNU `timeout` will exist on the release Mac.** The handoff itself discloses macOS ships no `timeout` (brew coreutils needed); without it every consultation is fail-closed stop. Evidence: the release Mac's PATH inventory. (Disclosed, but it gates the entire AI-consultation feature.)

**[NV-5] AC5's second half — "the pre-existing `scripts/*.test.sh` stay green (nothing wrapped broke)."** No run of the pre-existing harnesses appears in the handoff's self-check (the quoted full-suite command runs only Go/frontend plus the two new harnesses). Evidence that would settle it: a full-suite run including every pre-existing `scripts/*.test.sh`, or their addition to `test.unit.command`. Note also the brief's wiring decision (item 9) does not execute them, so AC5's "stay green" has no automated enforcement.

### WARN findings

**[W-1] `scripts/release.sh` dry-run posture vs. brief item 2 — the brief defines `--dry-run` as "to the draft, nothing public," but the implementation's dry-run pushes the release branch, opens the PR, **merges it**, pushes the tag, and creates **no** draft; the summary even says "The branch, PR, merge and tag did happen — they are the release."** A merged PR and a pushed tag are public, irreversible actions taken under a flag the brief describes as non-public. The harness comment cites ruling "A1" as settling this, but no such ruling text is in this prompt. Recommendation: confirm the A amendment in decisions.md genuinely redefines `--dry-run` this way; if it does not, dry-run must stop before the merge/tag.

**[W-2] `scripts/release.sh` `phase_run` — the `handlers` retry counter is shared between the Kai and bindings handlers (cap 2 total, not 2 each), and the AI `wait` action sleeps an unconditional 60 s in unit tests' PATH (fine) but also in real runs, where a single `wait` burns a consultation slot for a fixed pause.** Recommendation: separate counters per handler; acceptable as-is if R10 says otherwise (ruling text not shown).

**[W-3] `scripts/release.sh` `ph_verify` — the `mktemp -d` verification dir leaks on every `do_stop` path (only the success path `rm -rf`s it), and stop paths leave the dmg mounted if `diff -r` fails is handled but `release-verify.sh`/`shasum` failures detach nothing (no mount yet at those points — ok) — main leak is the temp dir.** Recommendation: trap-cleanup the verify dir.

**[W-4] `scripts/release.sh` `notify()` — only double quotes are escaped (`${1//\"/\'}`); backslashes and other shell metacharacters in the message would still be interpolated inside the `osascript -e` double-quoted string.** Messages are currently static phase names, so low risk. Recommendation: pass the message via `on run argv` or `printf %s` into osascript rather than string interpolation.

**[W-5] Handoff deviation 7 — CHANGELOG.md was edited via `sed -i` because the edit tool "does not list CHANGELOG.md" in its path permissions; the visible CHANGELOG hunk looks correct (one skip line, nothing else), but a tool-permission workaround to edit a file the tool was not permitted to touch is worth an explicit owner sign-off.** Recommendation: owner confirms the one-line result manually.

### NIT findings

**[NIT-1] `scripts/release.sh` `find_pr`/`ph_branch`/`ph_release_pr` — `set -- $info` relies on unquoted word splitting of gh output; a title containing spaces in a future format change would misparse (currently only number/state/sha, so safe).**

**[NIT-2] `scripts/release.sh` `ph_smoke` with `--smoke` calls `exit 0` from inside a phase function, bypassing the shared "posture complete" summary printing — the smoke instructions are printed instead, which is the intent, but the asymmetry is easy to break in future edits.**

**[NIT-3] `scripts/release.sh` `ph_changelog` builds `cands` (two `gh issue list` calls) even on the strict-failure path where it is used only in the stop message — harmless, just wasted calls before an inevitable stop.**

### Verdict
BLOCK — 2 blocking finding(s); must resolve before testing starts.