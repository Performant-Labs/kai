## Brief Review (Round 1)

### BLOCK findings
None.

### Needs-verification findings
- **[NV-1]** That each wrapped script (`release-preflight.sh`, `changelog-check.sh`, `release-bump.sh`, `release-gates.sh`, `release-local.sh`, `release-notes.sh`, `install-release.sh`) actually supports non-interactive invocation and observably exposes the postconditions `release.sh` needs (in particular that `release-local.sh` accepts `--yes` for its draft ask, and that a "verified draft" state is externally observable) — the shell sources of those scripts, especially `release-local.sh`'s flag parsing.
- **[NV-2]** That `opencode run` exists as a non-interactive, single-shot CLI that returns stdout suitable for JSON parsing — the `opencode` CLI documentation or a `--help` excerpt.
- **[NV-3]** That `<!-- changelog-skip: ... -->` is a token `changelog-check.sh` honors (the "#74/#75 precedent") — the changelog-check script source and the #74/#75 commits.
- **[NV-4]** That the `release-preflight.test.sh` harness shape (`mkrepo`/`run`/`has`, no network, PATH-faked scripts) generalizes to driving a stateful multi-phase workflow with kill/resume — the existing test file.
- **[NV-5]** That `.aftersight/pipeline.config.json`'s `test.unit.command` can be extended to include new harnesses without other pipeline assumptions breaking — the current config file contents.
- **[NV-6]** That `gh` CLI can comment on / open / close an issue with the semantics assumed in item 8 — `gh` issue subcommand documentation.
- **[NV-7]** That the #74 rule as stated ("merged tip = leftover; unmerged = release in progress") matches the actual rule in the repo docs/scripts it cites — `docs/releasing.md` / the branch-deletion logic in the wrapped scripts.

### WARN findings
- **[W-1]** Internal inconsistency on the PR-Agent threshold: item 6 hard-stops on "PR-Agent < 90 … with unresolved findings," while "For A to settle" frames it as the "PR-Agent > 90 evaluation rule." The comparison direction and the unresolved-findings qualifier are stated differently in two places; A must settle one canonical rule before T-red, or tests will pin the wrong predicate.
- **[W-2]** AC1 is environment-ambiguous: `--dry-run` "on a clean `master`" sounds live, but the Non-goals say the live rehearsal is post-merge and this run delivers only fake-script tests. State explicitly that AC1 is verified via the fake-script harness (and how "verified, packaged draft posture" is asserted there), or AC1 risks being unverifiable within this run.
- **[W-3]** AC3 pins "exactly one" consultation for an unknown failure, while item 5 permits "≤ 2 per phase." If a retry-after-consultation then fails again, is a second consultation allowed? Tests will diverge from spec unless this is pinned.
- **[W-4]** AC6's "encodes the hard-stop list verbatim" is not a testable criterion by itself; the parenthetical test pins cover only 3 of the listed hard stops. Add explicit pins for the remaining hard stops (missing signing identity; deleting/moving a published tag) or drop "verbatim."
- **[W-5]** Missing error path: `bin/release/vX.Y.Z/state.json` corruption, partial writes (kill mid-phase), and concurrent invocations of `release.sh` are unaddressed. A cheap atomic-write + lock convention should be specified before implementation.
- **[W-6]** Item 5 says the AI gets "read-only tools" yet may answer `run-handler <name>` / `propose-fix-pr`; clarify that the executor, not the AI session, performs any mutating action, and that `propose-fix-pr` cannot push to the release branch or move tags — otherwise this is a hole in the "hard stops no AI can override" guarantee.

### NIT findings
- **[NIT-1]** AC2 "killing it at any phase" should say "at any phase boundary" (postcondition-first resume cannot resume a half-completed side effect inside a wrapped script).
- **[NIT-2]** The needs-the-owner list (item 3) includes "codesign key-access prompt allowed once," which contradicts "no questions on the happy path" in spirit — worth one sentence acknowledging it's an OS-level prompt, not a workflow prompt.

### Verdict
PASS — no BLOCK findings; implementation may proceed.