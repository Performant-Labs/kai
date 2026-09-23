// coding-pipeline plugin — shared types (forked from the Anthropic pipeline's
// pre-flight payload; OpenCode-owned, no import from the Claude tree).

import type { PipelineConfig } from "./config.ts";

export type CheckStatus = "pass" | "fail" | "na";

/** A pre-flight row name. Fixed rows: pipeline-config, role-models,
 *  secret-scan, git-hooks-active, worktree-present, handoffs-tracked,
 *  orphaned-worktrees, issue-ready-to-brief, private-registry-auth,
 *  dual-review-interface, gate-model-reachable, playwright-mcp. Plus one
 *  `toolchain:<name>` row per entry in the project's config, so the set is open. */
export type CheckName = string;

export interface CheckResult {
  name: CheckName;
  status: CheckStatus;
  details: string;
}

/** A pipeline phase. Distinct from CheckName: phases are the ordered states the
 *  pipeline moves through (PHASE_ORDER); checks are the pre-flight rows. They
 *  happen to share some words ("design", "ui-walkthrough", "architecture-
 *  review") but are different domains — a check name is never a phase name and
 *  vice-versa ("node-version", "merge", "t-red" are phases-or-checks, not both). */
export type PhaseName =
  | "preflight"
  | "survey-brief"
  | "design"
  | "architecture-review"
  | "t-red"
  | "implement"
  | "t-green"
  | "a-dup"
  | "ui-walkthrough"
  | "spec-audit"
  | "merge";

export interface PreFlightInput {
  repoRoot: string;
  /** Issue number for the story being briefed (e.g. 725). Optional. */
  issueNumber?: number;
  /** Playbook rigor dial: direct / in-session / second-opinion / panel. */
  rigor: "direct" | "in-session" | "second-opinion" | "panel";
  /** Whether the story has a UI surface (drives the Playwright MCP row + D/U). */
  uiSurface?: boolean;
  /** #727: short token for the run's worktree (NNNN-<slug>). Optional. */
  slug?: string;
}

export interface HumanResponsibility {
  label: string;
  detail: string;
}

export interface PreFlightReport {
  ok: boolean;
  checks: CheckResult[];
  /** Human-owned rows: printed, never silently skipped, never gate the run. */
  humanResponsibilities: HumanResponsibility[];
  /** Configured OpenCode agent→model mapping, reported verbatim.
   *  NEVER reconciled against Claude's opus/sonnet axis (#706 false positive). */
  modelMapping: Record<string, string>;
  combinedError: string | null;
}

/** Injectable seams so unit tests can run the engine against fixtures
 *  (stubbed git/npm/curl outputs) without a live machine. */
export interface CheckContext {
  /** The project's validated pipeline config, or null when it is missing or
   *  invalid (the `pipeline-config` row then fails and the config-dependent
   *  rows fail closed rather than guessing). */
  config: PipelineConfig | null;
  /** Why `config` is null (shown verbatim in the `pipeline-config` row). */
  configError?: string | null;
  /** Repo root the run targets (worktree or primary checkout). Checks resolve
   *  file paths against this. Carried on the context so the engine stays pure
   *  and testable (the CLI's buildContext and the plugin entry both supply it). */
  repoRoot: string;
  /** Run a command, capture stdout/stderr/exit. Default: node:child_process
   *  (spawnSync). Tests may spy on this to PROVE the suite/hook are never
   *  spawned. Synchronous: the engine `await`s every seam, so a sync value is
   *  fine and keeps the default implementation simple. `options.cwd` (Story
   *  #727) lets a check run a command in a DIFFERENT directory than
   *  `repoRoot` — the worktree-present check re-anchors the context into the
   *  freshly-provisioned worktree and the verdict runs the suite there. */
  run(command: string, args: string[], options?: { timeoutMs?: number; cwd?: string }): RunResult;
  /** HTTP GET returning status + body (default: fetch with a timeout). The
   *  only genuinely async seam. */
  httpGet(url: string): Promise<{ status: number; body: string }>;
  /** Read a file; null when absent. Synchronous (fs.readFileSync). */
  readFile(path: string): string | null;
  /** Existence check. Synchronous (fs.accessSync). */
  exists(path: string): boolean;
  /** Current process's executable path (for the node-version check). */
  execPath(): string;
  /** Override for the T/F model endpoint's base URL (tests). Normally the
   *  engine resolves it from opencode.json via the gate provider. */
  modelBaseURL?: string;
  /** Story #727: create (or resume) the run's NNNN-slug worktree from the
   *  PRIMARY checkout and return it. The plugin entry supplies a real
   *  implementation (provisionWorktree); the CLI and the pre-flight engine
   *  tests supply a no-op / stub. Optional so a context that does not provision
   *  (e.g. the standalone CLI) still type-checks: the worktree-present check
   *  degrades to the old "you must be in a worktree" behaviour when absent. */
  provision?: (issueNumber: number | null, slug: string | undefined, primaryRoot: string) => { ok: boolean; path: string | null; info: WorktreeInfo | null; error: string | null };
  /** Set by the worktree-present check when it provisioned (or resumed) the run's worktree, so
   *  the caller can record whether it was created or resumed instead of assuming "created". */
  provisioned?: WorktreeInfo | null;
  /** Story #727: run the product suite in a given directory and return the
   *  plugin-owned verdict (exit>0 → RED, exit 0 → GREEN, crash → ERROR). The
   *  plugin entry supplies a real implementation (runTests); the pre-flight
   *  tests never invoke it (pre-flight must NOT run the suite — #705), so a
   *  stub suffices there. This is the ONE sanctioned exception to the
   *  "never run the product suite" rule, and ONLY the t-red / t-green advance
   *  gates (not pre-flight) call it. */
  runSuite?: (worktreePath: string, uiSurface: boolean) => { verdict: Verdict; summary: string; runs: TestRun[]; commands: string[] } | Promise<{ verdict: Verdict; summary: string; runs: TestRun[]; commands: string[] }>;
}

export interface RunResult {
  code: number;
  stdout: string;
  stderr: string;
}

// ---------------------------------------------------------------------------
// Story #727 — the NNNN-slug worktree the run executes in.
//
// The worktree is the UNIT OF WORK for every phase after pre-flight: T-red,
// F, and T-green all read and write inside it (it is `repoRoot` once the run
// has advanced past pre-flight), so concurrent stories on the same repo never
// share a checkout. Its identity is DERIVED, never hand-picked, so two runs
// can never collide by hand (CLAUDE.md "Worktree-per-run"):
//   - NNNN is the zero-padded 4-digit issue number,
//   - branch is issue-NNNN-<slug>,
//   - the port and throwaway database name are deterministic in NNNN.
// A `.gitignore` pattern ending in `/` (e.g. `node_modules/`) does NOT match a
// symlink, so a symlinked dependency directory (config worktree.linkDirs)
// shows as untracked; the worktree adds a LOCAL exclude line for each to
// .git/info/exclude (shared gitdir, never committed, never seen by a clone).
// ---------------------------------------------------------------------------

/** How `pipeline_worktree` created or found the run's worktree. */
export type WorktreeSource = "created" | "existing" | "in-place";

export interface WorktreeInfo {
  /** Absolute path to the worktree's root directory (its own toplevel). */
  path: string;
  /** The run's branch, `issue-NNNN-<slug>`. */
  branch: string;
  /** Absolute path to the PRIMARY checkout's root (source of the linked dirs). */
  primaryRoot: string;
  /** Port = worktree.basePort + (NNNN % 100): distinct per story, no hand coordination. */
  port: number;
  /** Throwaway database name `<dbNamePrefix>_<NNNN>`, deterministic in NNNN. */
  dbName: string;
  /** Whether this run's branch is already a checked-out worktree (vs. fresh). */
  reused: boolean;
  /** How the run got its worktree. */
  source: WorktreeSource;
}

// ---------------------------------------------------------------------------
// Story #727 — the plugin-owned test verdict.
//
// The RED/GREEN verdict is the PLUGIN's exit code from running the suite,
// never a model self-report. F (the implementor) is forbidden from writing or
// editing tests (see .opencode/agents/feature-implementor.md: edit deny-catch-
// all + test-path allow-list) and is forbidden from Task-spawning, so the only
// thing that can turn RED into GREEN is the plugin re-running the suite and
// observing exit 0. A test that F edits to pass silently is exactly the
// fraud the deny exists to block, and the plugin re-run is what catches it.
// ---------------------------------------------------------------------------

/** Which suite the verdict was taken from. */
export type SuiteKind = "unit" | "e2e";

/** The deterministic outcome the plugin observed by running the suite. */
export type Verdict = "RED" | "GREEN" | "ERROR";

export interface TestRun {
  suite: SuiteKind;
  /** The exact command that was spawned (for the handoff + the journal). */
  command: string;
  /** The plugin-owned verdict: exit>0 → RED, exit 0 → GREEN, crash → ERROR. */
  verdict: Verdict;
  /** exit code (or -1 on spawn error). */
  code: number;
  /** Last ~20 lines of stdout+stderr, so the handoff shows the real output. */
  tail: string;
  at: string;
}

/** Injectable seams so unit tests can drive the worktree + verdict without a
 *  live git repo (stubbed `run`, in-memory `fs` for .git/info/exclude). */
export interface WorktreeContext {
  /** The directory the plugin/check was invoked from. The PRIMARY root is
   *  derived from this via `git rev-parse --show-toplevel` (see
   *  provisionWorktreeFromPrimary) — it is not carried separately, so a caller
   *  that already lives inside the run's worktree is not misread as the
   *  primary. */
  invokedIn: string;
  run(command: string, args: string[], options?: { cwd?: string }): RunResult;
  readFile(path: string): string | null;
  writeFile(path: string, content: string): void;
  exists(path: string): boolean;
}
