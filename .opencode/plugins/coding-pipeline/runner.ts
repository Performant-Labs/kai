#!/usr/bin/env node
// coding-pipeline plugin — executable entry point (Node 26, native TS).
//
// Usage:
//   node coding-pipeline.ts preflight [--issue N] [--rigor R] [--ui-surface]
//
// Runs EVERY pre-flight check (never fail-fast), prints one structured JSON
// payload (the full fail list) and exits:
//   0 → all checks pass (or N/A)
//   2 → one or more checks failed (payload lists every failure)
//   3 → plugin-internal error
//
// The plugin tool `pipeline_preflight` (opencode-plugin.ts) shells out to
// THIS file, so a human (`/pipeline`) and an agent get the exact same
// payload. The plugin owns pass/fail; no LLM is in the check loop.
//
// HARD CONTRACT: this process NEVER spawns the product test suite (the
// configured test commands plus the built-in signatures below) and NEVER
// invokes a commit hook (no `git commit`). Unit tests prove this with a
// spawn spy (running the hook end-to-end once pulled the full CI chain into
// pre-flight — reverted, and must not return).

import { readFileSync, accessSync } from "node:fs";
import { spawnSync, execFileSync } from "node:child_process";
import process from "node:process";

import { loadPipelineConfig, type PipelineConfig } from "./config.ts";
import { runAllChecks, RIGOR_LEVELS } from "./preflight-engine.ts";
import type { CheckContext, PreFlightInput, RunResult } from "./types.ts";

// The spawn guard lives here (not in the engine) so the engine stays pure
// and unit-testable with stubs. Any command matching a product-suite
// signature is refused BEFORE spawn: fail closed, log the attempt.
//
// Built-in signatures cover the common stacks; the project's own test commands
// (config test.unit / test.e2e) and guard.extraForbiddenSignatures are added
// on top, so a custom runner is guarded without editing this file. None of
// these match the version probes pre-flight runs (`go version`, `swift
// --version`, `node -v`).
export const FORBIDDEN_COMMAND_SIGNATURES = [
  "npm test",
  "npm run test",
  "npm run check",
  "npm run lint",
  "npm run typecheck",
  "npm run build",
  "pnpm test",
  "pnpm run test",
  "yarn test",
  "bun test",
  "deno test",
  "dotnet test",
  "phpunit",
  "rspec",
  "make test",
  "ctest",
  "vitest",
  "playwright test",
  "pytest",
  "go test",
  "go vet",
  "swift test",
  "swift build",
  "xcodebuild",
  "cargo test",
  "mvn test",
  "gradle test",
];

export function forbiddenSignaturesFor(config: PipelineConfig | null): string[] {
  if (!config) return [...FORBIDDEN_COMMAND_SIGNATURES];
  const configured = [config.test.unit.command.join(" "), config.test.e2e?.command.join(" ")].filter(
    (c): c is string => typeof c === "string" && c.length > 0,
  );
  return [...new Set([...FORBIDDEN_COMMAND_SIGNATURES, ...configured, ...config.extraForbiddenSignatures])];
}

export function assertNotForbidden(
  command: string,
  args: string[],
  log: string[],
  signatures: readonly string[] = FORBIDDEN_COMMAND_SIGNATURES,
): void {
  const joined = `${command} ${args.join(" ")}`;
  if (command === "git" && args[0] === "commit") {
    log.push(joined);
    throw new Error("pre-flight refuses `git commit` (a commit hook would fire)");
  }
  for (const signature of signatures) {
    if (joined.includes(signature)) {
      log.push(joined);
      throw new Error(`pre-flight refused to spawn the product suite: ${joined}`);
    }
  }
}

function resolveRepoRoot(cwd: string): string {
  try {
    return execFileSync("git", ["rev-parse", "--show-toplevel"], { cwd, encoding: "utf8" }).trim();
  } catch {
    return cwd;
  }
}

/** Story #727: optional seams the CLI does NOT supply (it stays a pure
 *  pre-flight runner that never provisions a worktree or runs the suite —
 *  the plugin entry supplies the real implementations). When absent, the
 *  worktree-present check degrades to the legacy "you must be in a worktree"
 *  behaviour and no suite is ever run. */
export interface BuildContextOptions {
  provision?: CheckContext["provision"];
  runSuite?: CheckContext["runSuite"];
}

export function buildContext(
  repoRoot: string,
  config: PipelineConfig | null,
  configError: string | null,
  modelBaseURL: string | undefined,
  guardLog: string[],
  opts: BuildContextOptions = {},
): CheckContext {
  const signatures = forbiddenSignaturesFor(config);
  return {
    config,
    configError,
    repoRoot,
    modelBaseURL,
    provision: opts.provision,
    runSuite: opts.runSuite,
    run(command, args, runOpts) {
      assertNotForbidden(command, args, guardLog, signatures);
      return spawn(command, args, runOpts?.cwd);
    },
    httpGet(url) {
      const controller = new AbortController();
      const timer = setTimeout(() => controller.abort(), 8000);
      return fetch(url, { signal: controller.signal })
        .then((res) => res.text().then((body) => ({ status: res.status, body })))
        .finally(() => clearTimeout(timer));
    },
    readFile(p) {
      try {
        return readFileSync(p, "utf8");
      } catch {
        return null;
      }
    },
    exists(p) {
      try {
        accessSync(p);
        return true;
      } catch {
        return false;
      }
    },
    execPath: () => process.execPath,
  };
}

// Synchronous spawn matching the (sync) CheckContext.run seam. The engine
// `await`s every seam, so a sync result is fine; sync keeps the pre-flight
// wiring simple (no promise plumbing) and is safe for the short, local
// probes pre-flight runs. A spawn error (e.g. missing binary) is reported as
// a non-zero code, never a throw, so one bad check can't abort the run.
function spawn(command: string, args: string[], cwd?: string): RunResult {
  try {
    const r = spawnSync(command, args, {
      cwd: cwd ?? process.cwd(),
      env: { ...process.env, FORCE_COLOR: "0", NO_COLOR: "1" },
      encoding: "utf8",
      timeout: 15_000,
    });
    return { code: r.status ?? -1, stdout: r.stdout ?? "", stderr: r.stderr ?? "" };
  } catch (err) {
    return { code: -1, stdout: "", stderr: err instanceof Error ? err.message : String(err) };
  }
}

async function main(): Promise<void> {
  const argv = process.argv.slice(2);
  const mode = argv[0];
  if (mode !== "preflight") {
    console.error("usage: coding-pipeline.ts preflight [--issue N] [--rigor R] [--ui-surface] [--slug S]");
    process.exit(3);
  }

  const issueIdx = argv.indexOf("--issue");
  const issueNumber =
    issueIdx !== -1 && argv[issueIdx + 1] !== undefined ? Number(argv[issueIdx + 1]) : undefined;
  const rigorIdx = argv.indexOf("--rigor");
  const requestedRigor = rigorIdx !== -1 && argv[rigorIdx + 1] !== undefined ? argv[rigorIdx + 1] : "second-opinion";
  const rigor = (RIGOR_LEVELS as readonly string[]).includes(requestedRigor)
    ? (requestedRigor as PreFlightInput["rigor"])
    : "second-opinion";
  const uiSurface = argv.includes("--ui-surface");
  // Story #727: the worktree slug. The CLI does not provision a worktree
  // (it has no `provision` seam), so this is accepted for parity with the
  // plugin tool but has no effect on the CLI's pre-flight behaviour.
  const slugIdx = argv.indexOf("--slug");
  const slug =
    slugIdx !== -1 && argv[slugIdx + 1] !== undefined ? argv[slugIdx + 1] : undefined;

  const repoRoot = resolveRepoRoot(process.cwd());
  const loaded = loadPipelineConfig(repoRoot);
  const guardLog: string[] = [];
  const ctx = buildContext(repoRoot, loaded.config, loaded.error, undefined, guardLog);

  let report;
  try {
    report = await runAllChecks(ctx, { repoRoot, issueNumber, rigor, uiSurface, slug });
  } catch (err) {
    // The engine already converts per-check throws into failed rows; a throw
    // here means the plugin itself is broken.
    console.error(`pre-flight internal error: ${err instanceof Error ? err.stack : String(err)}`);
    process.exit(3);
  }

  if (guardLog.length > 0) {
    // Defensive: the guard throws, so this is unreachable — but if it ever
    // were bypassed, the run is a fail, never a pass.
    report = {
      ...report,
      ok: false,
      combinedError: `SECURITY: pre-flight attempted to spawn the product suite: ${guardLog.join(" | ")}`,
    };
  }

  console.log(JSON.stringify(report, null, 2));
  process.exit(report.ok ? 0 : 2);
}

// Auto-run only when this file is the CLI entry point (node runner.ts
// preflight ...), never when it is imported (e.g. by the unit tests, which
// import FORBIDDEN_COMMAND_SIGNATURES and would otherwise trigger main() →
// process.exit). Node 26 sets import.meta.main === true for the entry point.
if (import.meta.main) {
  main().catch((err) => {
    console.error(err instanceof Error ? err.stack : String(err));
    process.exit(3);
  });
}
