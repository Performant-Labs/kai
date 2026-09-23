// coding-pipeline plugin — dual-review gate (Story #728).
//
// The review-rigor dial's outside-model gate (`second-opinion` / `panel`) runs
// playbook's `dual-review.sh` (the canonical, project-agnostic second-opinion
// reviewer) via the plugin's spawn seam, then READS THE VERDICT FROM THE WRITTEN
// ARTIFACT. The verdict is code, not a model judgment:
//
//   PASS   → the gate passes (no BLOCK findings).
//   BLOCK  → the gate fails (one or more BLOCK findings; O respawns / amends).
//   UNVERIFIED / ABSENT → a NAMED STOP, never a PASS.
//
// The "never a looks-like-PASS" guarantee is the whole point of this module.
// dual-review.sh has THREE ways to not produce a trustworthy review, and each
// must land in a named stop:
//
//   exit ≠ 0            → the gate did not run (provider/transport/auth error, or
//                          a usage error). fail_gate writes a stub that says
//                          "FAILED (the gate did NOT run)". The script's own
//                          contract: a downstream consumer must not mistake a gate
//                          that never ran for one that ran clean.
//   exit 0 + no "### Verdict" heading  → the script's own sanity check flagged
//     (sanity-flagged)          the response as a non-review (e.g. the measured
//                          qwen38 degradation: a 178-byte "I'll start by reading
//                          the source..." narration with no review structure).
//                          The script prepends a marker line to --out; we parse it.
//   exit 0, well-formed, but the verdict section contains neither PASS nor BLOCK
//   (unparseable)           → the script ran clean but the model produced a
//                          "verdict" section we cannot read as a PASS/BLOCK. That
//                          is NOT a pass.
//
// A PASS is only ever returned when the artifact is present AND clean (no marker,
// no stub) AND the verdict section explicitly contains PASS and no BLOCK.
//
// The seam is injectable (run) so the unit tests script each exit code / artifact
// combination deterministically — the real spawnSync is wired in by
// opencode-plugin.ts, exactly like test-verdict.ts's run seam.

export type DualReviewMode = "brief" | "diff";

export type DualReviewOutcome = "PASS" | "BLOCK" | "UNVERIFIED" | "ABSENT";

export interface DualReviewRunResult {
  code: number;
  stdout: string;
  stderr: string;
}

export interface DualReviewSpawn {
  /** Run the gate: `dual-review.sh <args...>` and report the raw exit + streams. */
  run(command: string, args: string[], options?: { cwd?: string; timeoutMs?: number }): DualReviewRunResult | Promise<DualReviewRunResult>;
}

export interface DualReviewOptions {
  /** May be async: the review calls a model and can take minutes. */
  run(command: string, args: string[], options?: { cwd?: string; timeoutMs?: number }): DualReviewRunResult | Promise<DualReviewRunResult>;
  readFile(path: string): string | null;
  exists(path: string): boolean;
}

export interface DualReviewArgs {
  mode: DualReviewMode;
  /** Absolute path to the brief.md the review is taken against. */
  brief: string;
  /** Absolute path the gate writes the review artifact to. */
  out: string;
  /** diff mode only: the handoff file. */
  handoff?: string;
  /** diff mode only: the base git ref the diff is taken against. */
  base?: string;
  /** diff mode only: the diff git ref range (e.g. "HEAD~3"). */
  diff?: string;
  /** Re-review round: 2 requires a `response` path. */
  round?: number;
  /** Round-2 response file (the operator's answer to the prior BLOCK). */
  response?: string;
  /** panel mode: select the panel model arm instead of the default. */
  panel?: boolean;
}

export interface DualReviewReport {
  outcome: DualReviewOutcome;
  /** The review artifact path (the --out), if the file exists. */
  outPath: string | null;
  /** One line for the journal / handoff: what the gate did + why. */
  detail: string;
  /** The raw tail of the gate's stdout+stderr, for triage on a named stop. */
  tail: string;
}

// The script's own failure stub is headed by this exact line (see dual-review.sh
// fail_gate). Matching the heading — not the prose after it — keeps the check
// robust to the message text that follows.
const STUB_HEADING = "# Dual Review — FAILED (the gate did NOT run)";
// The script's sanity-check marker is prepended to --out as this markdown block
// quote when the response looks like a non-review (see mark_output_if_flagged).
const SANITY_MARKER = "SANITY CHECK FAILED";
// The prompt templates require a "### Verdict" heading; a review lacking it is,
// by the script's own definition, not a review. (Case-insensitive on the heading
// shape, mirroring the script's sanity_check_output grep. Note the JS regex
// form: bash's [[:space:]] is \s in JS, and \s covers the CRLF newline so a
// "\r"-terminated line still matches, as it does under the script's grep.)
const VERDICT_HEADING_JS = /^#{1,6}\s*verdict\s*$/im;

function tailLines(stdout: string, stderr: string, max = 20): string {
  const all = [stdout, stderr].filter(Boolean).join("\n");
  const lines = all.split("\n");
  return lines.slice(-max).join("\n").trim() || "(no output)";
}

// Extract the "### Verdict" section of a review artifact: from the heading line
// (a markdown heading whose text is exactly "verdict") to the next heading of the
// same-or-higher level, or end of file. Returns null when no verdict heading
// exists. This mirrors dual-review.sh's sanity_check_output: it matches the
// HEADING SHAPE, not the bare word "verdict" in prose, so a narrated-plan reply
// that merely says "I'll determine a verdict once I've read..." does not count.
export function extractVerdictSection(text: string): string | null {
  const lines = text.split("\n");
  let start = -1;
  let startLevel = 0;
  for (let i = 0; i < lines.length; i += 1) {
    const m = lines[i].match(/^(#{1,6})\s*verdict\s*$/i);
    if (m) {
      start = i;
      startLevel = m[1].length;
      break;
    }
  }
  if (start < 0) return null;
  let end = lines.length;
  for (let i = start + 1; i < lines.length; i += 1) {
    const h = lines[i].match(/^(#{1,6})\s/);
    if (h && h[1].length <= startLevel) {
      end = i;
      break;
    }
  }
  return lines.slice(start, end).join("\n");
}

export function hasVerdictHeading(text: string): boolean {
  return VERDICT_HEADING_JS.test(text);
}

// The script's exit codes (see the "Documented exit codes" block in
// dual-review.sh):
//   0  review written
//   1  usage/validation error
//   10 provider returned an error object (auth, quota, bad model, …)
//   11 transport failure (network/DNS/timeout) — no response at all
//   12 HTTP 200 but no review text in the response
// Every non-zero code is a gate that did NOT run a trustworthy review.
export function mapOutcomeForExit(code: number, artifact: string | null): { outcome: DualReviewOutcome; reason: string } {
  if (code === 0) {
    if (artifact == null) {
      // Exit 0 but the artifact the gate was told to write is missing. The script
      // writes --out on every success path, so a missing file here is
      // inconsistent — treat as a named stop, never a pass.
      return { outcome: "ABSENT", reason: "gate exited 0 but the --out artifact is missing" };
    }
    if (artifact.includes(STUB_HEADING)) {
      return { outcome: "UNVERIFIED", reason: "the --out artifact is the script's failure stub, not a review" };
    }
    if (artifact.includes(SANITY_MARKER)) {
      // The script flagged the response as a non-review (sanity check) but still
      // wrote it out with a marker. A flagged review is UNVERIFIED — not a pass.
      return { outcome: "UNVERIFIED", reason: "the script's sanity check flagged the response as not a real review (sanity marker in the artifact)" };
    }
    if (!hasVerdictHeading(artifact)) {
      return { outcome: "UNVERIFIED", reason: "no '### Verdict' heading in the artifact — not a real review" };
    }
    // Scope the decision to the lines AFTER the "### Verdict" heading. The
    // script's templates put the literal words "### BLOCK findings" / "### WARN
    // findings" in EARLIER headings and the decision on the verdict line itself
    // (a clean PASS reads "### Verdict" then "PASS — …"; a real BLOCK reads
    // "### Verdict" then "BLOCK — …"). A naive whole-artifact substring test
    // would see the "### BLOCK findings" heading and misread every clean PASS as
    // a BLOCK.
    const headingMatch = artifact.match(/^#{1,6}[ \t]*verdict[ \t]*$/im);
    if (!headingMatch || headingMatch.index === undefined) {
      return { outcome: "UNVERIFIED", reason: "verdict heading matched the presence check but not on its own line" };
    }
    const after = artifact.slice(headingMatch.index + headingMatch[0].length);
    // Only the verdict's own decision lines count: stop at the next markdown
    // heading so a later "### Notes" section (or the embedded brief) cannot
    // reintroduce the words.
    const decisionBlock = after
      .split(/\r?\n/)
      .reduce<string[]>((acc, line) => {
        if (/^#{1,6}[ \t]/.test(line) && acc.length > 0) return acc;
        acc.push(line);
        return acc;
      }, []);
    // The script's PASS verdict wording NEGATES the word — "PASS — no BLOCK
    // findings…". So a positive BLOCK statement ("BLOCK — N blocking finding(s)")
    // is what marks a BLOCK; a bare substring "BLOCK" would trip on the
    // negated "no BLOCK findings" in a clean PASS. Match the positive form
    // ("BLOCK —" / "BLOCK:" / "BLOCK:"). A PASS's "no BLOCK" must not count.
    const decision = decisionBlock.join("\n");
    const isPass = /\bPASS\b/.test(decision.toUpperCase());
    const isBlock = /\bBLOCK\b[ \t]*[:—–]/.test(decision.toUpperCase());
    const hasPass = isPass;
    const hasBlock = isBlock;
    if (hasBlock) {
      return { outcome: "BLOCK", reason: "the review's Verdict section contains BLOCK finding(s)" };
    }
    if (hasPass) {
      return { outcome: "PASS", reason: "the review's Verdict section is PASS (no BLOCK findings)" };
    }
    return { outcome: "UNVERIFIED", reason: "the Verdict section contains neither PASS nor BLOCK — unparseable" };
  }
  return { outcome: "ABSENT", reason: `the gate exited ${code} (did not run: ${code} = ${exitName(code)})` };
}

function exitName(code: number): string {
  switch (code) {
    case 1:
      return "usage/validation error";
    case 10:
      return "provider returned an error object (auth, quota, bad model)";
    case 11:
      return "transport failure (network/DNS/timeout) — no response";
    case 12:
      return "HTTP 200 but no review text in the response";
    default:
      return "unrecognized non-zero exit";
  }
}

export async function runDualReview(opts: DualReviewOptions, args: DualReviewArgs): Promise<DualReviewReport> {
  // Assemble the canonical argv. This is the interface pre-flight already
  // verified (--mode/--brief/--out), plus the diff/round/panel extras the
  // gate's own usage documents.
  const script = "dual-review.sh";
  const scriptArgs = [
    "--mode",
    args.mode,
    "--brief",
    args.brief,
    "--out",
    args.out,
  ];
  if (args.mode === "diff") {
    if (args.handoff) scriptArgs.push("--handoff", args.handoff);
    if (args.base) scriptArgs.push("--base", args.base);
    if (args.diff) scriptArgs.push("--diff", args.diff);
  }
  if (args.round != null && args.round > 1) {
    scriptArgs.push("--round", String(args.round));
    if (args.response) scriptArgs.push("--response", args.response);
  }
  if (args.panel) scriptArgs.push("--panel");

  const r = await opts.run(script, scriptArgs);
  const artifact = r.code === 0 ? opts.readFile(args.out) : null;
  const { outcome, reason } = mapOutcomeForExit(r.code, artifact);
  return {
    outcome,
    outPath: artifact != null ? args.out : null,
    detail: `${outcome} — ${reason}`,
    tail: tailLines(r.stdout, r.stderr),
  };
}
