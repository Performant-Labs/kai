// coding-pipeline plugin — plugin-owned test verdict (Story #727).
//
// The RED/GREEN verdict is the PLUGIN's exit code from running the suite,
// never a model self-report. This is the whole point of the T-red → F →
// T-green loop being code rather than model judgment:
//
//   T-red   → run the suite → the plugin observes exit>0  → RED  (the contract)
//   F       → writes production code ONLY (it is permission-denied from
//              writing or editing tests, and denied from Task-spawning)
//   T-green → re-run the suite → the plugin observes exit 0  → GREEN
//
// Because F is denied from touching the test files (see
// .opencode/agents/feature-implementor.md: edit deny-catch-all with test-path
// allows), the only thing that can turn the suite RED→GREEN is F's production
// code — and the only authority that can SAY it is green is the plugin re-
// running the suite and reading the exit code. A test F edits to pass silently
// is exactly the fraud the deny exists to block; the plugin re-run is what
// catches it, because F cannot edit the file the plugin re-runs.
//
// Verdict mapping (deterministic, in code):
//   exit code > 0  → RED    (something failed — a valid RED pre-F, a failure post-F)
//   exit code == 0 → GREEN  (the suite passes)
//   spawn error    → ERROR  (the run itself blew up; not a verdict, a stop)
//
// The suites are the project's own commands (pipeline.config.json test.unit /
// test.e2e) — the plugin never guesses a stack. The unit suite always runs;
// the e2e suite runs ONLY when one is configured AND the story has a UI surface
// (the `uiSurface` flag gates the Phase U / e2e path, mirroring pre-flight's
// playwright-mcp row).
//
// This is the ONE sanctioned exception to the "never run the product suite"
// rule that binds PRE-FLIGHT (see runner.ts FORBIDDEN_COMMAND_SIGNATURES):
// pre-flight is Phase 1 (wiring + reachability only, #705) and must not run
// the suite; T-red / T-green are LATER phases whose entire job IS to run it.
// The pre-flight guard is scoped to pre-flight; this module is scoped to the
// t-red / t-green phases. The two never share a code path.

import type { SuiteConfig } from "./config.ts";
import type { RunResult, TestRun, Verdict } from "./types.ts";

export interface TestRunArgs {
  /** Absolute path to the run's worktree root (the cwd the suite runs in). */
  worktreePath: string;
  /** Whether the story has a UI surface. Gates the e2e half of the verdict. */
  uiSurface: boolean;
}

export interface RunTestsOptions {
  /** May be async: a suite can run for minutes and must not block the host process. */
  run(command: string, args: string[], options?: { cwd?: string; timeoutMs?: number }): RunResult | Promise<RunResult>;
}

export interface TestVerdict {
  runs: TestRun[];
  /** Overall: GREEN only if every run that ran is GREEN; RED if any is RED;
   *  ERROR if any errored (an error outranks a red — the suite didn't get to
   *  produce a clean signal). */
  verdict: Verdict;
  /** One line per run, for the handoff + the journal. */
  summary: string;
  /** The command(s) the plugin spawned, verbatim. */
  commands: string[];
}

export function mapVerdict(code: number, errored: boolean): Verdict {
  if (errored) return "ERROR";
  if (code === 0) return "GREEN";
  return "RED";
}

function tailLines(stdout: string, stderr: string, max = 20): string {
  const all = [stdout, stderr].filter(Boolean).join("\n");
  const lines = all.split("\n");
  return lines.slice(-max).join("\n").trim() || "(no output)";
}

function display(command: string[]): string {
  return command.join(" ");
}

export async function runTests(
  opts: RunTestsOptions,
  args: TestRunArgs,
  suites: { unit: SuiteConfig; e2e: SuiteConfig | null },
): Promise<TestVerdict> {
  const runs: TestRun[] = [];
  const commands: string[] = [];

  // The unit suite is ALWAYS the core of the verdict.
  const unitCmd = display(suites.unit.command);
  const unit = await opts.run(suites.unit.command[0], suites.unit.command.slice(1), {
    cwd: args.worktreePath,
    timeoutMs: suites.unit.timeoutMs,
  });
  runs.push({
    suite: "unit",
    command: unitCmd,
    verdict: mapVerdict(unit.code, unit.code === -1),
    code: unit.code,
    tail: tailLines(unit.stdout, unit.stderr),
    at: new Date().toISOString(),
  });
  commands.push(unitCmd);

  // The e2e suite is run only for UI-surface stories that configure one (the
  // t-green Phase 6 runs the e2e/visual half for those). Others skip it —
  // declared, never silent (it is simply not part of their verdict).
  if (args.uiSurface && suites.e2e) {
    const e2eCmd = display(suites.e2e.command);
    const e2e = await opts.run(suites.e2e.command[0], suites.e2e.command.slice(1), {
      cwd: args.worktreePath,
      timeoutMs: suites.e2e.timeoutMs,
    });
    runs.push({
      suite: "e2e",
      command: e2eCmd,
      verdict: mapVerdict(e2e.code, e2e.code === -1),
      code: e2e.code,
      tail: tailLines(e2e.stdout, e2e.stderr),
      at: new Date().toISOString(),
    });
    commands.push(e2eCmd);
  }

  // Overall verdict: ERROR outranks RED outranks GREEN.
  let verdict: Verdict = "GREEN";
  if (runs.some((r) => r.verdict === "ERROR")) verdict = "ERROR";
  else if (runs.some((r) => r.verdict === "RED")) verdict = "RED";

  const summary = runs
    .map((r) => `${r.suite} (${r.command}): ${r.verdict} [exit ${r.code}]`)
    .join(" · ");

  return { runs, verdict, summary, commands };
}
