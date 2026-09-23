// coding-pipeline plugin (OpenCode). Registers the pipeline's machine-facing
// tools. The Anthropic pipeline is rehosted, not wrapped: phase order, the
// pre-flight gate, worktree naming, RED/GREEN and the rigor dial all live in
// this plugin's code — an LLM cannot skip, reorder, or waive them.
//
// Tools: `pipeline_preflight` (run every check, one structured payload, fail
// closed on the T/F model) + `pipeline_status` + `pipeline_advance` (the hard
// gate: no survey-brief without a passing pre-flight or a named waiver) +
// `pipeline_worktree` + `pipeline_dual_review`.
//
// Project-specific behaviour (test commands, toolchains, handoffs dir, worktree
// naming) comes from <repo>/.opencode/pipeline.config.json — see config.ts. A
// missing or invalid config fails closed.

import { spawnSync } from "node:child_process";
import path from "node:path";
import fs from "node:fs";

// OpenCode installs @opencode-ai/plugin into .opencode/ on startup (it manages
// .opencode/package.json), so a local plugin imports its tool factory from there.
// OpenCode calls a plugin as (input, options?) — it never passes a tool factory.
import { tool as opencodeTool } from "@opencode-ai/plugin";

import { RIGOR_LEVELS, runAllChecks, PHASE_ORDER, PHASE_INDEX, PHASE_CONDITIONS, rigorNeedsOutsideModel } from "./coding-pipeline/preflight-engine.ts";
import { buildContext } from "./coding-pipeline/runner.ts";
import { loadPipelineConfig } from "./coding-pipeline/config.ts";
import { DEFAULT_WORKTREE_SETTINGS, deriveWorktree, provisionWorktree as provisionWorktreeImpl } from "./coding-pipeline/worktree.ts";
import { runTests, type TestVerdict } from "./coding-pipeline/test-verdict.ts";
import { spawnAsync } from "./coding-pipeline/spawn-async.ts";
import { runDualReview, type DualReviewArgs, type DualReviewMode, type DualReviewOptions } from "./coding-pipeline/dual-review.ts";
import type { PhaseName, PreFlightInput, PreFlightReport, TestRun, WorktreeContext, WorktreeInfo } from "./coding-pipeline/types.ts";

// Story #727: the rework-loop cap. The T-red → F → T-green loop re-enters F on
// every RED; without a cap a non-converging implementation burns unbounded
// F↔T-green cycles. The cap is code (not a model decision) and is surfaced to
// the human at the stop, never silently exceeded.
const REWORK_CAP = 5;

// The tool factory and the arg-schema helper are OpenCode's own (`tool` and
// `tool.schema` from @opencode-ai/plugin). The shapes the factory consumes and
// produces are typed structurally here so the plugin's unit tests can build
// fakes against them (the optional `wiring` argument exists for that only; a
// real OpenCode never passes one). ToolDefinition is generic over the parsed
// arg object so each tool's execute() reads strongly-typed args instead of
// `unknown`.
export type ToolDefinition<TArgs extends Record<string, unknown>> = {
  description: string;
  args: Record<string, unknown>;
  execute: (args: TArgs, context: { directory: string }) => Promise<string>;
};
export type ToolFactory = <TArgs extends Record<string, unknown>>(def: ToolDefinition<TArgs>) => ToolDefinition<TArgs>;
interface ZodBase {
  describe(d: string): this;
}
export interface ZodNum extends ZodBase {
  int(): ZodNum;
  positive(): ZodNum;
  optional(): ZodNum;
  default(v: number): ZodNum;
}
export interface ZodStr extends ZodBase {
  optional(): ZodStr;
}
export interface ZodBool extends ZodBase {
  optional(): ZodBool;
}
export interface ZodEnum extends ZodBase {
  default(v: string): ZodEnum;
}
export type ZodLike = {
  number: () => ZodNum;
  string: () => ZodStr;
  boolean: () => ZodBool;
  enum: (values: readonly string[]) => ZodEnum;
};
export type PluginInput = {
  directory: string;
  worktree?: string;
  client?: unknown;
};
export type PluginWiring = { tool?: ToolFactory; zod?: ZodLike };

const STATE_FILE = "state.json";

export interface PipelineState {
  version: 1;
  issueNumber: number | null;
  rigor: (typeof RIGOR_LEVELS)[number];
  uiSurface: boolean;
  /** #727: the run's worktree slug (NNNN-<slug>). Carried from pre-flight. */
  slug: string | null;
  currentPhase: PhaseName;
  preflight: {
    status: "pending" | "pass" | "fail";
    at: string | null;
    report: PreFlightReport | null;
    waiver: string | null;
  };
  // Story #727: the run's NNNN-slug worktree (created when the run first
  // advances past pre-flight). `active` flips true once the worktree exists;
  // from then on the run's repoRoot is the WORKTREE, not the primary checkout.
  worktree: (WorktreeInfo & { active: boolean }) | null;
  // Story #727: the plugin-owned test verdict. `red` is set when the suite is
  // confirmed RED at T-red (the contract F implements against); `green` when
  // it is confirmed GREEN at T-green. Both are the plugin's exit codes, never
  // a model self-report. `rework` counts F↔T-green re-entries (capped).
  test: {
    red: TestRun[] | null;
    green: TestRun[] | null;
    rework: number;
    /** A NAMED, recorded waiver allowing a failed T-red verdict to proceed to
     *  F anyway (the equivalent of the pre-flight waiver; recorded, never
     *  silent). The default is fail-closed: a RED that is not a contract
     *  failure blocks the loop. */
    waiver: string | null;
  };
  // Story #728: the review-rigor dial's outside-model gates. For
  // `second-opinion` / `panel` the plugin runs playbook's dual-review.sh
  // (brief gate + diff gate) and READS the verdict from the written artifact —
  // never a model self-report, never a "looks like PASS". `brief` / `diff` are
  // the two gates (null until run); `lastOutcome` / `lastReason` are the named
  // verdict the advance gates consult; a non-PASS outcome is a NAMED STOP the
  // human resolves with a named waiver, exactly like the pre-flight and T-red
  // waivers.
  dualReview: {
    brief: { outcome: "PASS" | "BLOCK" | "UNVERIFIED" | "ABSENT"; at: string; out: string | null; reason: string } | null;
    diff: { outcome: "PASS" | "BLOCK" | "UNVERIFIED" | "ABSENT"; at: string; out: string | null; reason: string } | null;
    lastOutcome: "PASS" | "BLOCK" | "UNVERIFIED" | "ABSENT" | null;
    lastReason: string | null;
    /** Named waivers recorded per dual-review gate. Kept apart from the
     *  pre-flight waiver so accepting a review verdict never rewrites what was
     *  accepted at pre-flight. */
    waivers?: Partial<Record<"brief" | "diff", string>>;
  };
  phasesCompleted: string[];
  updates: { phase: PhaseName; note: string; at: string }[];
}

const DEFAULT_HANDOFFS_DIR = "docs/handoffs";

function stateFile(repoRoot: string, handoffsDir: string): string {
  return path.join(repoRoot, ...handoffsDir.split("/"), STATE_FILE);
}

function loadState(repoRoot: string, handoffsDir: string): PipelineState | null {
  try {
    const text = fs.readFileSync(stateFile(repoRoot, handoffsDir), "utf8");
    const parsed = JSON.parse(text) as PipelineState;
    if (parsed.version !== 1) return null;
    return parsed;
  } catch {
    return null;
  }
}

function saveState(repoRoot: string, handoffsDir: string, state: PipelineState): void {
  const file = stateFile(repoRoot, handoffsDir);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, JSON.stringify(state, null, 2) + "\n");
}

function newState(input: PreFlightInput): PipelineState {
  return {
    version: 1,
    issueNumber: input.issueNumber ?? null,
    rigor: input.rigor,
    uiSurface: input.uiSurface ?? false,
    slug: input.slug ?? null,
    currentPhase: "preflight",
    preflight: { status: "pending", at: null, report: null, waiver: null },
    worktree: null,
    test: { red: null, green: null, rework: 0, waiver: null },
    dualReview: { brief: null, diff: null, lastOutcome: null, lastReason: null },
    phasesCompleted: [],
    updates: [],
  };
}

// OpenCode loads a local plugin by invoking this exported function with
// (input, options?) and reading the returned hooks. `wiring` is a test seam.
export default async function codingPipelinePlugin(input: PluginInput, wiring?: PluginWiring) {
  const repoRoot = input.directory;
  // Per-project configuration. Null (with an error) when missing/invalid: the
  // pre-flight `pipeline-config` row then fails and the suite gates refuse.
  const loaded = loadPipelineConfig(repoRoot);
  const config = loaded.config;
  const handoffsDir = config?.handoffs.dir ?? DEFAULT_HANDOFFS_DIR;
  const worktreeSettings = config?.worktree ?? DEFAULT_WORKTREE_SETTINGS;

  // Story #727: the run's worktree is the unit of work after pre-flight —
  // T-red, F, and T-green all read and write inside it, so the PLUGIN resolves
  // "where does this run execute" (never a model, never the primary branch).
  // The root is resolved from the persisted state (worktree.path once active),
  // so a re-run / resume in a different cwd still lands in the right place.
  // Defined here (not at module scope) because it closes over `repoRoot`.
  const worktreeRootFor = (state: PipelineState): string =>
    state.worktree?.active ? state.worktree.path : repoRoot;
  const isWorktreeProvisioned = (state: PipelineState): boolean => state.worktree?.active === true;
  // The plugin-owned test verdict: the plugin runs the suite in the run's
  // worktree and reads the exit code — F cannot self-report RED/GREEN (it is
  // permission-denied from the test files and from Task-spawning), so the
  // plugin's spawn is the only authority. This is the ONE sanctioned exception
  // to the "never run the product suite" rule; it is wired ONLY to the
  // t-red / t-green advance gates, never to pre-flight.
  const runSuiteIn = async (worktreePath: string, uiSurface: boolean): Promise<TestVerdict> => {
    if (!config) {
      // Fail closed: never guess the test command. ERROR is a stop, not a verdict.
      return { runs: [], verdict: "ERROR", summary: `pipeline config unavailable — ${loaded.error}`, commands: [] };
    }
    // Async: a suite can run for minutes and OpenCode shares this process with its UI.
    return runTests({
      run: (cmd, args, o) => spawnAsync(cmd, args, { cwd: o?.cwd ?? worktreePath, timeoutMs: o?.timeoutMs ?? 300_000 }),
    }, { worktreePath, uiSurface }, config.test);
  };
  // #728: the dual-review gate seam. The plugin resolves dual-review.sh and runs
  // it (the pre-flight `dual-review-interface` check already confirmed it
  // resolves + accepts --mode/--brief/--out under outside-model rigor). The
  // plugin READS the verdict from the written artifact — it never trusts a model
  // self-report and never treats a missing/flagged/unparseable review as a pass
  // (that parsing lives in coding-pipeline/dual-review.ts). The seam is injected
  // so unit tests can script every exit code / artifact shape; the real
  // spawnSync is wired here.
  const dualReviewIn = (args: DualReviewArgs) =>
    runDualReview(
      {
        run: (cmd, a, o) => spawnAsync(cmd, a, { cwd: o?.cwd, timeoutMs: o?.timeoutMs ?? 900_000 }),
        readFile: (p) => {
          try {
            return fs.readFileSync(p, "utf8");
          } catch {
            return null;
          }
        },
        exists: (p) => {
          try {
            fs.accessSync(p);
            return true;
          } catch {
            return false;
          }
        },
      } satisfies DualReviewOptions,
      args,
    );
  // The worktree provisioner seam handed to the pre-flight context: build a
  // WorktreeContext rooted at the primary (the run is always provisioned FROM
  // the primary, never from inside an existing worktree) and call the
  // provisioner. The seam is synchronous (the CheckContext.provision contract)
  // and so is the underlying provisioner — every git op is a fast local
  // spawnSync — so no await is needed and none is legal here.
  const provisionWorktree = (issueNumber: number | null, slug: string | undefined, primaryRoot: string) => {
    if (issueNumber == null) {
      return { ok: false as const, path: null, info: null, error: "no issue number" };
    }
    const wtCtx: WorktreeContext = {
      invokedIn: primaryRoot,
      run: (cmd, args, o) => {
        const r = spawnSync(cmd, args, { cwd: o?.cwd ?? primaryRoot, encoding: "utf8", timeout: 60_000 });
        return { code: r.status ?? -1, stdout: r.stdout ?? "", stderr: r.stderr || (r.error ? r.error.message : "") };
      },
      readFile: (p) => { try { return fs.readFileSync(p, "utf8"); } catch { return null; } },
      writeFile: (p, content) => fs.writeFileSync(p, content),
      exists: (p) => { try { fs.accessSync(p); return true; } catch { return false; } },
    };
    return provisionWorktreeImpl(wtCtx, primaryRoot, issueNumber, slug ?? `issue-${issueNumber}`, worktreeSettings);
  };

  const tool: ToolFactory = wiring?.tool ?? (opencodeTool as unknown as ToolFactory);
  const z: ZodLike = wiring?.zod ?? (opencodeTool.schema as unknown as ZodLike);
  type PreFlightArgs = { issue?: number; rigor?: (typeof RIGOR_LEVELS)[number]; uiSurface?: boolean; slug?: string };
  type AdvanceArgs = { target?: string; waiver?: string; note?: string };
  // #727: the worktree slug is carried from pre-flight (the story's token).
  // `pipeline_worktree` also accepts an explicit slug for re-provisioning.
  type WorktreeArgs = { issue?: number; slug?: string; reprovision?: boolean };
  // #728: pipeline_dual_review's arg shape. The canonical DualReviewArgs (imported)
  // is what the seam consumes; this is the tool-facing view (mode/brief optional
  // with defaults, the rest optional).
  type DualReviewArgsInput = {
    mode?: DualReviewMode;
    brief?: string;
    out?: string;
    handoff?: string;
    base?: string;
    diff?: string;
    round?: number;
    response?: string;
    panel?: boolean;
  };

  const preflightArgs = {
    issue: z
      .number()
      .int()
      .positive()
      .optional()
      .describe("GitHub issue number the run is briefing from (optional)"),
    rigor: z.enum(RIGOR_LEVELS).default("second-opinion").describe("Review-rigor dial"),
    uiSurface: z.boolean().optional().describe("Does the story have a UI surface? (drives D/U + Playwright MCP)"),
    slug: z.string().optional().describe(`Short token for the run's worktree: ${worktreeSettings.dir}/NNNN-<slug>, branch issue-NNNN-<slug>`),
  };

  return {
    tool: {
      pipeline_preflight: tool({
        description:
          "Run the pipeline's Phase 1 pre-flight. Deterministic: the plugin executes every check " +
          "(pipeline config, role models, the project's toolchains, secret scanner and git hooks when required, " +
          "worktree, handoffs tracking, orphaned worktrees, issue readiness, private-registry auth, " +
          "dual-review interface, T/F model HTTP probe, Playwright MCP when uiSurface). " +
          "It never runs the product test suite, e2e, or a commit hook. Every check runs even after a " +
          "failure; one structured payload returns every failure. Exit code 2 (or an unreachable T/F model) " +
          "means FAIL CLOSED: fix or explicitly waive before briefing.",
         args: preflightArgs,
         execute: async (args: PreFlightArgs) => {
          const issueNumber = args.issue ?? detectIssueFromBranch(repoRoot);
           const rigor = args.rigor ?? "second-opinion";
           const uiSurface = args.uiSurface ?? false;
           const slug = args.slug;

          const guardLog: string[] = [];
           // Story #727: wire the real provision seam (the CLI does not) and
           // pass the slug. The worktree-present check calls provision when the
           // run is in the primary and re-anchors ctx.repoRoot into the new
           // worktree — so every later check + phase executes in the worktree.
           const ctx = buildContext(repoRoot, config, loaded.error, undefined, guardLog, { provision: provisionWorktree });

           const report = await runAllChecks(ctx, { repoRoot, issueNumber, rigor, uiSurface, slug });

           const state = loadState(repoRoot, handoffsDir);
           const base =
             state && (state.issueNumber === issueNumber || !state.preflight.at)
               ? state
               : newState({ repoRoot, issueNumber, rigor, uiSurface, slug });
           base.preflight = {
             status: report.ok ? "pass" : "fail",
             at: new Date().toISOString(),
             report,
             waiver: base.preflight.waiver,
           };
           // Story #727: if the worktree-present check provisioned a worktree
           // (ctx.repoRoot moved from the primary into it), persist that so the
           // later phases — and the t-red/t-green verdicts — run in the
           // worktree, not the primary. The branch/port/DB name are re-derived
           // deterministically from (issueNumber, slug, primaryRoot) via
           // deriveWorktree at read time, so they always match the worktree
           // that was actually created.
           if (ctx.repoRoot !== repoRoot && base.issueNumber != null) {
             const slugForDerive = slug ?? `issue-${base.issueNumber}`;
             base.slug = slugForDerive;
             const derived = deriveWorktree(base.issueNumber, slugForDerive, repoRoot, worktreeSettings);
             base.worktree = {
               path: ctx.repoRoot,
               branch: derived.branch,
               primaryRoot: repoRoot,
               port: derived.port,
               dbName: derived.dbName,
               reused: ctx.provisioned?.reused ?? false,
               source: ctx.provisioned?.source ?? "created",
               active: true,
             };
           }
           if (guardLog.length > 0) {
             base.preflight.status = "fail";
             base.preflight.report = {
               ...report,
               ok: false,
               combinedError: `SECURITY: pre-flight attempted the product suite: ${guardLog.join(" | ")}`,
             };
           }
           saveState(repoRoot, handoffsDir, base);

           return renderReport(base.preflight, uiSurface);
        },
      }),

      pipeline_status: tool({
        description: "Show the pipeline's current position: phase, pre-flight result, rigor, waivers, and updates.",
        args: {},
        execute: async () => {
          const state = loadState(repoRoot, handoffsDir);
          if (!state) {
            return "No pipeline run in flight. Start one with `pipeline_preflight`.";
          }
          const lines = [
            `Pipeline — issue ${state.issueNumber ?? "?"}, rigor ${state.rigor}, uiSurface ${state.uiSurface}`,
            `Phase: ${state.currentPhase} (index ${PHASE_INDEX.get(state.currentPhase) ?? "?"} of ${PHASE_ORDER.length})`,
            `Pre-flight: ${state.preflight.status}${state.preflight.at ? ` at ${state.preflight.at}` : ""}${state.preflight.waiver ? ` (waived: ${state.preflight.waiver})` : ""}`,
          ];
          for (const row of state.preflight.report?.checks ?? []) {
            lines.push(`  ${row.status.toUpperCase().padEnd(4)} ${row.name}: ${row.details}`);
          }
          if (state.preflight.report?.humanResponsibilities.length) {
            lines.push("Human responsibilities:");
            for (const h of state.preflight.report.humanResponsibilities) lines.push(`  - ${h.label}: ${h.detail}`);
          }
          if (Object.keys(state.preflight.report?.modelMapping ?? {}).length) {
            lines.push("Model mapping (configured verbatim):");
            for (const [agent, model] of Object.entries(state.preflight.report?.modelMapping ?? {})) {
              lines.push(`  @${agent} → ${model}`);
            }
          }
          // Story #727: surface the worktree (the run's unit of work) and the
          // plugin-owned RED/GREEN verdict so a human resuming the run can see
          // where it executes and whether the loop is converging.
          if (state.worktree?.active) {
            lines.push(`Worktree: ${state.worktree.path} (branch ${state.worktree.branch}, port ${state.worktree.port}, db ${state.worktree.dbName}) — ${state.worktree.source}${state.worktree.reused ? " (resumed)" : ""}`);
          }
          if (state.test.red || state.test.green || state.test.rework > 0) {
            const redSum = state.test.red?.[0] ? `T-red ${state.test.red[0].verdict}` : "T-red —";
            const greenSum = state.test.green?.[0] ? `T-green ${state.test.green[0].verdict}` : "T-green —";
            lines.push(`Verdict (plugin-owned): ${redSum}, ${greenSum}, rework ${state.test.rework}/${REWORK_CAP}${state.test.waiver ? `, T-red waived: ${state.test.waiver}` : ""}`);
          }
          if (state.phasesCompleted.length) {
            lines.push(`Completed: ${state.phasesCompleted.join(" → ")}`);
          }
          const gates = (["brief", "diff"] as const).flatMap((g) => {
            const r = state.dualReview[g];
            const w = state.dualReview.waivers?.[g];
            return r || w ? [`  ${g}: ${r ? r.outcome : "not run"}${w ? ` (waived: ${w})` : ""}`] : [];
          });
          if (gates.length) lines.push("Review gates:", ...gates);
          if (state.updates.length) {
            lines.push("Updates:");
            for (const u of state.updates) lines.push(`  [${u.at}] ${u.phase}: ${u.note}`);
          }
          return lines.join("\n");
        },
      }),

      pipeline_advance: tool({
        description:
          "Advance the pipeline to the next phase. The plugin is the gate: survey-brief is refused unless the " +
          "last pre-flight is pass or carries a named waiver; design / ui-walkthrough are skipped (and the skip is " +
          "RECORDED, never silent) when uiSurface is false; the phase order is code, not a model decision.",
        args: {
          target: z
            .string()
            .optional()
            .describe(
              `Phase to advance to. Defaults to the phase after the current one. Valid: ${PHASE_ORDER.join(", ")}.`,
            ),
          waiver: z
            .string()
            .optional()
            .describe("Named waiver required to advance past a FAILED pre-flight (recorded in state)."),
          note: z.string().optional().describe("What changed / why (recorded as an update in the journal)."),
        },
        execute: async (args: AdvanceArgs) => {
          const state = loadState(repoRoot, handoffsDir);
          if (!state) {
            return "STOP: no pipeline run exists. Run `pipeline_preflight` first — the plugin refuses to brief from a missing pre-flight.";
          }
          const currentIndex = PHASE_INDEX.get(state.currentPhase) ?? 0;
          const rawTarget = args.target ?? PHASE_ORDER[currentIndex + 1];
          const target = rawTarget as PhaseName | undefined;
          if (!target || !PHASE_ORDER.includes(target)) {
            return `STOP: unknown phase ${JSON.stringify(target)}. Valid phases: ${PHASE_ORDER.join(", ")}.`;
          }
          const targetIndex = PHASE_INDEX.get(target)!;
          if (targetIndex <= currentIndex) {
            return `STOP: phase order is code. Current phase is ${state.currentPhase} (index ${currentIndex}); ${target} (index ${targetIndex}) does not move forward. Backward moves are not permitted.`;
          }

          // Forward moves may not skip phases. The only phases that can be passed
          // over are the UI-conditional ones (design, ui-walkthrough) when the story
          // has no UI surface — each is then recorded N/A, declared and never silent.
          // Anything else (survey-brief, implement, …) must be completed first: this
          // is what keeps a failed pre-flight, the RED→implement→GREEN loop and the
          // review gates from being bypassed by jumping ahead.
          const skipped = PHASE_ORDER.slice(currentIndex + 1, targetIndex) as PhaseName[];
          const blocking = skipped.find((p) => !(PHASE_CONDITIONS[p]?.uiSurface && !state.uiSurface));
          if (blocking) {
            return `STOP: phase order is code. ${blocking} must be completed before ${target} (current phase: ${state.currentPhase}); phases cannot be skipped. Advance to ${blocking} first.`;
          }
          for (const p of skipped) {
            const marker = `${p} (N/A: uiSurface is false)`;
            if (!state.phasesCompleted.includes(marker)) {
              state.phasesCompleted.push(marker);
              state.updates.push({ phase: p, note: "N/A — declared by plugin: uiSurface is false", at: new Date().toISOString() });
            }
          }

          // The hard gate: briefing is blocked behind a PASSING pre-flight, and
          // a waiver does not change that. A named waiver is an explicitly
          // RECORDED human authorization (it lands in state and is visible to
          // every later gate and to the human); the refusal stays in place until
          // a separate advance carries that waiver, so the decision and its
          // rationale are auditable and reversible, never a silent skip.
          if (target === "survey-brief" && state.currentPhase === "preflight") {
            if (state.preflight.status === "fail") {
              const waiver = args.waiver ?? state.preflight.waiver;
              if (!waiver) {
                return `ADVANCE REFUSED: pre-flight FAILED — ${state.preflight.report?.combinedError ?? "see pipeline_status"}. Fix the failures and re-run pipeline_preflight, or pass a named waiver (recorded) if the failure is accepted.`;
              }
              if (args.waiver) {
                state.preflight.waiver = args.waiver;
                saveState(repoRoot, handoffsDir, state);
                return `ADVANCE REFUSED — waiver "${args.waiver}" recorded. The failed pre-flight still blocks survey-brief; re-run pipeline_advance to survey-brief with the waiver to proceed.`;
              }
              // A waiver is already on record: the advance proceeds, visibly.
              state.phasesCompleted.push(state.currentPhase);
              state.currentPhase = target;
              saveState(repoRoot, handoffsDir, state);
              return `Advanced: ${target} (index ${targetIndex} of ${PHASE_ORDER.length}). Waived pre-flight on record: ${state.preflight.waiver}.`;
            }
            if (state.preflight.status !== "pass") {
              return "ADVANCE REFUSED: no pre-flight result on record. Run pipeline_preflight first.";
            }
          }

          // D / U: declared N/A when there is no UI surface — never silent.
          if (target === "design" || target === "ui-walkthrough") {
            if (!state.uiSurface) {
              state.phasesCompleted.push(`${target} (N/A: uiSurface is false)`);
              state.updates.push({ phase: target, note: "N/A — declared by plugin: uiSurface is false", at: new Date().toISOString() });
              state.currentPhase = target;
              saveState(repoRoot, handoffsDir, state);
              return `Phase ${target} recorded N/A (uiSurface is false). The skip is declared in state.json and the journal — it was never silent.`;
            }
          }

          // Story #728: the review-rigor dial is PLUGIN-ENFORCED, not a model
          // decision. Under `direct` / `in-session` the gate is declared N/A
          // (no outside-model review; the in-session reviewer is O itself, and it
          // is a human, not a model that can be fooled into "looks like PASS").
          // Under `second-opinion` / `panel` the plugin REQUIRES a PASSING
          // outside-model review at the two gates — the BRIEF gate (after
          // survey-brief) and the DIFF gate (after t-green, before a-dup). A
          // BLOCK / UNVERIFIED / ABSENT verdict is a NAMED STOP: the phase does
          // not advance until the human resolves it with a NAMED waiver (recorded
          // in state.json), exactly like the pre-flight and T-red waivers. The
          // dual-review.sh invocation itself is `pipeline_dual_review` — the
          // orchestrator never shells out to it from memory.
          const gateFor = (to: PhaseName): "brief" | "diff" | null => {
            if (to === "architecture-review") return "brief";
            if (to === "a-dup") return "diff";
            return null;
          };
          const rig = state.rigor;
          const gate = gateFor(target);
          if (gate && rig && PHASE_ORDER.includes(target)) {
            if (!rigorNeedsOutsideModel(rig)) {
              if (!state.phasesCompleted.some((p) => p.startsWith(`${gate} (declared N/A`))) {
                state.phasesCompleted.push(`${gate} (declared N/A: rigor is ${rig})`);
                state.updates.push({ phase: target, note: `dual-review ${gate} declared N/A: rigor is ${rig}; no outside-model gate`, at: new Date().toISOString() });
                saveState(repoRoot, handoffsDir, state);
              }
              state.phasesCompleted.push(state.currentPhase + (args.note ? ` (${args.note})` : ""));
              state.currentPhase = target;
              if (args.note) state.updates.push({ phase: target, note: args.note, at: new Date().toISOString() });
              saveState(repoRoot, handoffsDir, state);
              return `Advanced: ${target} (index ${targetIndex} of ${PHASE_ORDER.length}). The ${gate} dual-review gate is DECLARED N/A — rigor is "${rig}" (no outside-model review). The N/A is recorded in state.json; it was never silent.`;
            }
            const gateState = state.dualReview[gate];
            if (!gateState || gateState.outcome !== "PASS") {
              // A NAMED waiver (passed to THIS advance) records the operator's
              // acceptance of the non-PASS verdict and unblocks the gate — exactly
              // like the pre-flight and T-red waivers. The waiver is recorded in
              // state.json; the original outcome is preserved in the gate's reason.
              if (args.waiver) {
                state.dualReview.waivers = { ...(state.dualReview.waivers ?? {}), [gate]: args.waiver };
                state.updates.push({ phase: target, note: `NAMED WAIVER for ${gate} dual-review gate: "${args.waiver}" (outcome was ${gateState ? gateState.outcome : "no review run"}; accepted by the operator)`, at: new Date().toISOString() });
                state.phasesCompleted.push(state.currentPhase + (args.note ? ` (${args.note})` : ""));
                state.currentPhase = target;
                if (args.note) state.updates.push({ phase: target, note: args.note, at: new Date().toISOString() });
                saveState(repoRoot, handoffsDir, state);
                return `Advanced: ${target} (index ${targetIndex} of ${PHASE_ORDER.length}). The ${gate} dual-review gate did not return a PASS (it was ${gateState ? gateState.outcome : "never run"}); a NAMED WAIVER "${args.waiver}" was recorded to proceed anyway. The waiver is visible in state.json — this advance was never a silent pass.`;
              }
              const why = gateState?.reason ?? "no dual-review has been run for this phase yet";
              const modeHint = gate === "diff" ? ", with --handoff/--base/--diff" : "";
              return `ADVANCE REFUSED (${gate} dual-review gate): rigor is "${rig}" (an outside-model gate) but the ${gate} review has no PASS on record (${why}). Run pipeline_dual_review (mode '${gate}'${modeHint}) and it must return a PASS before ${target} is reached. A BLOCK / UNVERIFIED / ABSENT verdict is a NAMED STOP, never a 'looks like PASS' — resolve it with a NAMED waiver (pass 'waiver' to pipeline_advance) if the failure is accepted (recorded).`;
            }
          }

          // Story #727: the plugin OWNS the T-red → F → T-green loop and the
          // RED/GREEN verdict. F cannot self-report — it is permission-denied
          // from the test files and from Task-spawning, so the plugin runs the
          // suite in the run's worktree and reads the exit code. These two
          // gates are the ONE sanctioned exception to the "never run the
          // product suite" rule (which binds PRE-FLIGHT); they are scoped to
          // t-red / t-green and never touch pre-flight.
          if (target === "t-red") {
            if (!isWorktreeProvisioned(state)) {
              return "STOP: cannot enter t-red — the run's worktree is not provisioned. Run pipeline_preflight (it creates the NNNN-slug worktree) before the T-red → F → T-green loop.";
            }
            const verdict = await runSuiteIn(worktreeRootFor(state), state.uiSurface);
            state.test.red = verdict.runs;
            state.updates.push({ phase: "t-red", note: `plugin-owned verdict: ${verdict.verdict} — ${verdict.summary}`, at: new Date().toISOString() });
            saveState(repoRoot, handoffsDir, state);
            if (verdict.verdict === "ERROR") {
              return `T-RED ERROR: the suite could not be run (${verdict.summary}). Fix the harness and re-run — a crash is not a RED, so the loop does not proceed.`;
            }
            if (verdict.verdict === "RED") {
              state.phasesCompleted.push(state.currentPhase + (args.note ? ` (${args.note})` : ""));
              state.currentPhase = "t-red";
              saveState(repoRoot, handoffsDir, state);
              return `T-RED confirmed RED (plugin-owned verdict, exit≠0) — ${verdict.summary}. The failing tests are the contract F implements against. Advance to implement (F) when the @tester has written them.`;
            }
            if (args.waiver) {
              state.test.waiver = args.waiver;
              state.phasesCompleted.push(state.currentPhase + (args.note ? ` (${args.note})` : ""));
              state.currentPhase = "t-red";
              saveState(repoRoot, handoffsDir, state);
              return `T-RED is GREEN, not RED — ${verdict.summary}. The contract is already green, so there is nothing for F to implement. A NAMED waiver "${args.waiver}" was recorded to proceed to F anyway (the run is now deliberately off the red→green contract); without a waiver the loop stops here.`;
            }
            // Fail-closed: a GREEN at t-red does NOT advance the phase. The
            // RED→GREEN loop is the whole point of the story; a contract that
            // is already green is off-contract, so the plugin holds the phase
            // and makes the human either write a failing test (then re-run) or
            // pass a NAMED waiver to proceed deliberately off-contract.
            saveState(repoRoot, handoffsDir, state);
            return `T-RED REFUSED (fail-closed): the plugin ran the suite and it is GREEN — ${verdict.summary}. T-red requires a RED contract. The phase did NOT advance. Either write a failing test (then re-run pipeline_advance to t-red) or pass a NAMED waiver to proceed off-contract.`;
          }

          if (target === "t-green") {
            if (!isWorktreeProvisioned(state)) {
              return "STOP: cannot enter t-green — the run's worktree is not provisioned.";
            }
            if (state.test.rework >= REWORK_CAP) {
              return `STOP: the rework loop has hit its cap (${REWORK_CAP} F↔T-green re-entries). The implementation is not converging to green; stop and hand back to the human rather than burning unbounded F↔T-green cycles.`;
            }
            const verdict = await runSuiteIn(worktreeRootFor(state), state.uiSurface);
            state.test.green = verdict.runs;
            state.updates.push({ phase: "t-green", note: `plugin-owned verdict: ${verdict.verdict} (rework ${state.test.rework}) — ${verdict.summary}`, at: new Date().toISOString() });
            if (verdict.verdict === "ERROR") {
              saveState(repoRoot, handoffsDir, state);
              return `T-GREEN ERROR: the suite could not be run (${verdict.summary}). Fix the harness and re-run — a crash is not a GREEN, so the loop does not advance.`;
            }
            if (verdict.verdict === "GREEN") {
              state.phasesCompleted.push(state.currentPhase + (args.note ? ` (${args.note})` : ""));
              state.currentPhase = target;
              if (args.note) state.updates.push({ phase: target, note: args.note, at: new Date().toISOString() });
              saveState(repoRoot, handoffsDir, state);
              return `T-GREEN confirmed GREEN (plugin-owned verdict, exit 0) — ${verdict.summary}. Advanced to ${target} (index ${targetIndex} of ${PHASE_ORDER.length}).`;
            }
            // RED (or a red that should now be green): rework. Bump the counter.
            state.test.rework += 1;
            saveState(repoRoot, handoffsDir, state);
            return `T-GREEN is still RED (plugin-owned verdict, exit≠0) — ${verdict.summary}. Rework ${state.test.rework}/${REWORK_CAP}: dispatch @feature-implementor to fix the production code, then re-run pipeline_advance to t-green. The verdict is the plugin's exit code, not F's claim.`;
          }

          state.phasesCompleted.push(state.currentPhase + (args.note ? ` (${args.note})` : ""));
          state.currentPhase = target;
          if (args.note) {
            state.updates.push({ phase: target, note: args.note, at: new Date().toISOString() });
          }
          saveState(repoRoot, handoffsDir, state);
          return `Advanced: ${target} (index ${targetIndex} of ${PHASE_ORDER.length}).${args.note ? ` Note: ${args.note}` : ""}`;
         },
       }),

      pipeline_worktree: tool({
        description:
          "Inspect or (re)provision the run's NNNN-slug git worktree. The worktree is the unit of work for the " +
          "whole run (T-red, F, T-green all execute in it), and the plugin — never a model, never the primary " +
          "checkout's branch — owns where the run executes. Report mode shows the current worktree (path, branch, " +
          "port, DB name, created-vs-resumed) or states that none is active yet. Re-provision mode (pass `issue` " +
          "and/or `slug`) creates the worktree if missing or resumes it if present, the same way pre-flight does.",
        args: {
          issue: z.number().optional().describe("Issue number for the worktree (NNNN). Defaults to the run's issue."),
          slug: z.string().optional().describe(`Short token: ${worktreeSettings.dir}/NNNN-<slug>, branch issue-NNNN-<slug>.`),
          reprovision: z.boolean().optional().describe("If true, (re)create/resume the worktree now. Defaults to false (report only)."),
        },
        execute: async (args: WorktreeArgs) => {
          const state = loadState(repoRoot, handoffsDir);
          if (!state) {
            return "No pipeline run in flight. Start one with `pipeline_preflight` (it provisions the worktree).";
          }
          const issue = args.issue ?? state.issueNumber ?? null;
          const slug = args.slug ?? state.slug ?? (issue != null ? `issue-${issue}` : undefined);

          if (!args.reprovision) {
            // Report mode.
            if (!state.worktree?.active) {
              return `No worktree is active for issue ${issue ?? "?"}. The run is still anchored to the primary checkout (${repoRoot}). Run pipeline_preflight (or pipeline_worktree with reprovision=true) to provision ${worktreeSettings.dir}/${issue != null ? String(issue).padStart(4, "0") : "????"}-${slug ?? "?"}.`;
            }
            const w = state.worktree;
            return [
              `Worktree (issue ${issue ?? "?"}):`,
              `  path:    ${w.path}`,
              `  branch:  ${w.branch}`,
              `  port:    ${w.port}`,
              `  dbName:  ${w.dbName}`,
              `  source:  ${w.source}${w.reused ? " (resumed)" : " (created)"}`,
              `  primary: ${w.primaryRoot}`,
            ].join("\n");
          }

          // Re-provision mode.
          if (issue == null) {
            return "Cannot re-provision: no issue number. Pass `issue` (NNNN) and an optional `slug`.";
          }
          if (!slug) {
            return "Cannot re-provision: no slug. Pass `slug` (or ensure the run has one from pre-flight).";
          }
          const result = provisionWorktree(issue, slug, repoRoot);
          if (!result.ok) {
            return `WORKTREE PROVISION FAILED: ${result.error}`;
          }
          state.worktree = {
            path: result.path!,
            branch: result.info?.branch ?? state.worktree?.branch ?? deriveWorktree(issue, slug, repoRoot, worktreeSettings).branch,
            primaryRoot: repoRoot,
            port: result.info?.port ?? (worktreeSettings.basePort + (issue % 100)),
            dbName: result.info?.dbName ?? `${worktreeSettings.dbNamePrefix}_${String(issue).padStart(4, "0")}`,
            reused: result.info?.reused ?? state.worktree?.reused ?? false,
            source: result.info?.source ?? (state.worktree ? "existing" : "created"),
            active: true,
          };
          state.slug = slug;
          saveState(repoRoot, handoffsDir, state);
          return `Worktree ${state.worktree.source === "existing" ? "resumed" : "created"} at ${state.worktree.path} (branch ${state.worktree.branch}, port ${state.worktree.port}, db ${state.worktree.dbName}). The run now executes there.`;
        },
      }),

      pipeline_dual_review: tool({
        description:
          "Run the review-rigor dial's outside-model gate (playbook's dual-review.sh) and read the verdict from the " +
          "written artifact. Use for the BRIEF gate (mode \"brief\", after survey-brief) and the DIFF gate (mode " +
          "\"diff\", after t-green, before a-dup). Only meaningful under second-opinion / panel rigor — under " +
          "direct / in-session the gate is declared N/A by pipeline_advance. The verdict is the plugin's reading of " +
          "the artifact (PASS / BLOCK / UNVERIFIED / ABSENT), NEVER a model self-report: a missing, flag-marked, or " +
          "unparseable review is a NAMED STOP, never a 'looks like PASS'. A non-PASS verdict blocks the corresponding " +
          "advance until resolved with a NAMED waiver.",
        args: {
          mode: z.enum(["brief", "diff"]).describe("brief = the brief gate (after survey-brief); diff = the diff gate (after t-green, before a-dup)."),
          brief: z.string().optional().describe(`Path to the brief.md the review is taken against. Defaults to ${handoffsDir}/brief.md in the run's root.`),
          out: z.string().optional().describe(`Where to write the review artifact. Defaults to ${handoffsDir}/dual-review-<mode>.md in the run's root.`),
          handoff: z.string().optional().describe("diff mode: the F handoff path."),
          base: z.string().optional().describe("diff mode: the base git ref the diff is taken against."),
          diff: z.string().optional().describe("diff mode: the diff git ref range (e.g. HEAD~3)."),
          round: z.number().int().positive().optional().describe("Re-review round. Round 2 requires `response` (the operator's answer to the prior BLOCK)."),
          response: z.string().optional().describe("Round-2 response file (the operator's answer to the prior BLOCK)."),
          panel: z.boolean().optional().describe("panel: select the panel model arm instead of the default reviewer."),
        },
        execute: async (input: DualReviewArgsInput) => {
          const state = loadState(repoRoot, handoffsDir);
          if (!state) {
            return "STOP: no pipeline run exists. Run `pipeline_preflight` first.";
          }
          const phaseIdx = PHASE_INDEX.get(state.currentPhase ?? "preflight") ?? 0;
          // The diff gate runs after t-green (before a-dup); everything earlier is the brief gate.
          const tGreenIdx = PHASE_INDEX.get("t-green") ?? 1;
          const mode: DualReviewMode = input.mode ?? (phaseIdx >= tGreenIdx ? "diff" : "brief");
          const runRoot = isWorktreeProvisioned(state) ? state.worktree!.path : repoRoot;
          // Under direct / in-session the gate is a DECLARED N/A — it never runs
          // and needs no brief. Short-circuit here (before the brief check) so the
          // N/A is honest and the script is never invoked.
          if (!rigorNeedsOutsideModel(state.rigor)) {
            state.updates.push({ phase: state.currentPhase, note: `dual-review(${mode}) skipped: rigor is "${state.rigor}" (no outside-model gate)`, at: new Date().toISOString() });
            saveState(repoRoot, handoffsDir, state);
            return `DECLARED N/A: rigor is "${state.rigor}" — the ${mode} gate needs no outside-model review. Nothing was run; the ${mode} advance will proceed (the N/A is declared in state.json, never silent).`;
          }
          const out = input.out ?? path.join(runRoot, ...handoffsDir.split("/"), `dual-review-${mode}.md`);
          const brief = input.brief ?? path.join(runRoot, ...handoffsDir.split("/"), "brief.md");
          if (!input.brief && !fs.existsSync(brief)) {
            return `STOP: no brief found at the default path (${brief}). Pass 'brief' (the brief.md path) explicitly.`;
          }
          // Convert the tool-facing (all-optional) args to the canonical
          // DualReviewArgs the seam consumes; the defaults are already resolved
          // above.
          const canonical: DualReviewArgs = {
            mode,
            brief,
            out,
            handoff: mode === "diff" ? input.handoff : undefined,
            base: mode === "diff" ? input.base : undefined,
            diff: mode === "diff" ? input.diff : undefined,
            round: input.round,
            response: input.response,
            panel: input.panel ?? state.rigor === "panel",
          };
          const report = await dualReviewIn(canonical);
          state.dualReview[mode] = { outcome: report.outcome, at: new Date().toISOString(), out: report.outPath, reason: report.detail };
          state.dualReview.lastOutcome = report.outcome;
          state.dualReview.lastReason = report.detail;
          state.updates.push({ phase: state.currentPhase, note: `dual-review(${mode}) → ${report.outcome}: ${report.detail}`, at: new Date().toISOString() });
          saveState(repoRoot, handoffsDir, state);
          const lines = [`dual-review(${mode}) → ${report.outcome} — ${report.detail}`, report.outPath ? `  artifact: ${report.outPath}` : "  artifact: (none written)"];
          if (report.tail && report.outcome !== "PASS") lines.push(`  tail: ${report.tail}`);
          if (report.outcome === "PASS") {
            lines.push(`  PASS: the ${mode} gate is satisfied; the corresponding advance (${mode === "brief" ? "to architecture-review" : "to a-dup"}) is now unblocked.`);
          } else if (report.outcome === "BLOCK") {
            lines.push(`  BLOCK: NAMED STOP. The ${mode === "brief" ? "architecture-review" : "a-dup"} advance is blocked until the BLOCK finding(s) are resolved (fix and re-run, or a NAMED waiver via pipeline_advance).`);
          } else {
            lines.push(`  ${report.outcome} (NAMED STOP, never a pass): the ${mode === "brief" ? "architecture-review" : "a-dup"} advance is blocked. Fix the cause (provider/transport/auth for ABSENT; the model not producing a review for UNVERIFIED) and re-run, or record a NAMED waiver.`);
          }
          return lines.join("\n");
        },
      }),
    },
  };
}

// --- helpers (kept local so the plugin file stays self-contained) ----------

function detectIssueFromBranch(repoRoot: string): number | undefined {
  try {
    const branch = spawnSync("git", ["rev-parse", "--abbrev-ref", "HEAD"], { cwd: repoRoot, encoding: "utf8" }).stdout ?? "";
    const m = branch.match(/(\d{3,4})/);
    return m ? Number(m[1]) : undefined;
  } catch {
    return undefined;
  }
}

function renderReport(preflight: PipelineState["preflight"], uiSurface: boolean): string {
  const report = preflight.report!;
  const lines: string[] = [];
  lines.push(`PRE-FLIGHT: ${report.ok ? "PASS" : "FAIL"} (${report.checks.filter((c) => c.status === "fail").length} failing of ${report.checks.length})`);
  for (const row of report.checks) {
    const marker = row.status === "pass" ? "PASS" : row.status === "na" ? "N/A " : "FAIL";
    lines.push(`  ${marker}  ${row.name}: ${row.details}`);
  }
  if (report.humanResponsibilities.length) {
    lines.push("Human responsibilities (you own these — the plugin cannot):");
    for (const h of report.humanResponsibilities) lines.push(`  - ${h.label}: ${h.detail}`);
  }
  if (Object.keys(report.modelMapping).length) {
    lines.push("Model mapping (configured verbatim — do NOT reconcile against Claude's opus/sonnet axis):");
    for (const [agent, model] of Object.entries(report.modelMapping)) lines.push(`  @${agent} → ${model}`);
  }
  if (uiSurface) {
    lines.push("uiSurface: true — the design stop requires human wireframe approval before tests/code.");
  }
  if (!report.ok) {
    lines.push("");
    lines.push("FAIL CLOSED: fix the failures above and re-run pipeline_preflight, or record a named waiver via pipeline_advance before briefing.");
  } else {
    lines.push("");
    lines.push("Proceed: pipeline_advance to survey-brief when ready.");
  }
  return lines.join("\n");
}
