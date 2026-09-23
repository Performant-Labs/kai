// coding-pipeline plugin — per-project configuration.
//
// Everything that differs between projects (how to run the tests, which
// toolchains must be installed, where handoffs live, how worktrees are named)
// is data in <repo>/.opencode/pipeline.config.json. The plugin code carries no
// project or stack assumptions: a missing or invalid config FAILS CLOSED
// (pre-flight fails, t-red / t-green refuse) — it never guesses `npm test`.

import fs from "node:fs";
import os from "node:os";
import path from "node:path";

export const CONFIG_RELATIVE_PATH = ".opencode/pipeline.config.json";

export interface SuiteConfig {
  /** argv, e.g. ["go", "test", "./..."]. No shell: element 0 is the binary. */
  command: string[];
  timeoutMs: number;
}

export interface PinSource {
  file: string;
  /** Capture group 1 is the pinned version. */
  regex?: string;
  /** Dotted path into a JSON file, e.g. "engines.node". */
  json?: string;
}

export interface ToolchainCheck {
  name: string;
  /** argv that prints the installed version. */
  command: string[];
  /** Capture group 1 is the installed version. Default: first x.y[.z]. */
  versionRegex?: string;
  pin?: { sources: PinSource[]; compare: "same-major" | "at-least" };
  hint?: string;
}

export interface PipelineConfig {
  project: string;
  /** One line about the project, shown to the orchestrator. Optional. */
  description: string;
  /** Path globs the role agents' edit permissions are generated from
   *  (install.mjs --sync-agents). T may edit `test`; F may edit `production`
   *  but is denied `test`; every other role edits only the handoffs dir. */
  paths: { test: string[]; production: string[] };
  test: { unit: SuiteConfig; e2e: SuiteConfig | null };
  toolchain: ToolchainCheck[];
  hooks: { required: boolean; setupHint: string };
  secretScan: { required: boolean; command: string[]; hint: string };
  handoffs: { dir: string; trackedProbe: string };
  worktree: { dir: string; basePort: number; dbNamePrefix: string; linkDirs: string[] };
  /** OpenCode provider whose options.baseURL is HTTP-probed for the gate. null
   *  = derive it from the tester role's resolved model. */
  gateProvider: string | null;
  /** Extra command signatures pre-flight must never spawn (added to the
   *  built-in list and to the configured test commands). */
  extraForbiddenSignatures: string[];
}

export type ConfigResult = { config: PipelineConfig; error: null } | { config: null; error: string };

const DEFAULT_TIMEOUT_MS = 300_000;

function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

function stringArray(v: unknown, label: string): string[] | string {
  if (!Array.isArray(v) || v.length === 0 || !v.every((x) => typeof x === "string" && x.length > 0)) {
    return `${label} must be a non-empty array of non-empty strings (argv, no shell)`;
  }
  return v as string[];
}

/** Repo-relative directory: no absolute paths, no `..` segments. */
function relativeDir(v: unknown, label: string, fallback: string): string {
  const value = v === undefined ? fallback : v;
  if (typeof value !== "string" || value.trim() === "") throw new Error(`${label} must be a non-empty string`);
  const normalized = path.posix.normalize(value.replace(/\\/g, "/")).replace(/\/+$/, "");
  if (normalized.startsWith("/") || normalized === ".." || normalized.startsWith("../") || normalized === "." || normalized === "") {
    throw new Error(`${label} must be a repo-relative directory without ".." (got ${JSON.stringify(value)})`);
  }
  return normalized;
}

function sanitizeIdentifier(s: string): string {
  return s.toLowerCase().replace(/[^a-z0-9]+/g, "_").replace(/^_+|_+$/g, "");
}

function parseSuite(v: unknown, label: string): SuiteConfig {
  if (!isRecord(v)) throw new Error(`${label} must be an object with a "command" array`);
  const cmd = stringArray(v.command, `${label}.command`);
  if (typeof cmd === "string") throw new Error(cmd);
  const timeoutMs = v.timeoutMs === undefined ? DEFAULT_TIMEOUT_MS : v.timeoutMs;
  if (typeof timeoutMs !== "number" || !Number.isFinite(timeoutMs) || timeoutMs <= 0) {
    throw new Error(`${label}.timeoutMs must be a positive number`);
  }
  return { command: cmd, timeoutMs };
}

function parseToolchain(v: unknown): ToolchainCheck[] {
  if (v === undefined) return [];
  if (!Array.isArray(v)) throw new Error("toolchain must be an array");
  return v.map((raw, i) => {
    const label = `toolchain[${i}]`;
    if (!isRecord(raw)) throw new Error(`${label} must be an object`);
    if (typeof raw.name !== "string" || !raw.name) throw new Error(`${label}.name is required`);
    const command = stringArray(raw.command, `${label}.command`);
    if (typeof command === "string") throw new Error(command);
    const out: ToolchainCheck = { name: raw.name, command };
    if (raw.versionRegex !== undefined) {
      if (typeof raw.versionRegex !== "string") throw new Error(`${label}.versionRegex must be a string`);
      try {
        new RegExp(raw.versionRegex);
      } catch {
        throw new Error(`${label}.versionRegex is not a valid regular expression`);
      }
      out.versionRegex = raw.versionRegex;
    }
    if (raw.hint !== undefined) {
      if (typeof raw.hint !== "string") throw new Error(`${label}.hint must be a string`);
      out.hint = raw.hint;
    }
    if (raw.pin !== undefined) {
      if (!isRecord(raw.pin) || !Array.isArray(raw.pin.sources) || raw.pin.sources.length === 0) {
        throw new Error(`${label}.pin.sources must be a non-empty array`);
      }
      const compare = raw.pin.compare === undefined ? "at-least" : raw.pin.compare;
      if (compare !== "same-major" && compare !== "at-least") {
        throw new Error(`${label}.pin.compare must be "same-major" or "at-least"`);
      }
      const sources = raw.pin.sources.map((s: unknown, j: number) => {
        if (!isRecord(s) || typeof s.file !== "string" || !s.file) throw new Error(`${label}.pin.sources[${j}].file is required`);
        if (s.regex === undefined && s.json === undefined && !s.file.endsWith(".nvmrc") && !s.file.endsWith("-version")) {
          throw new Error(`${label}.pin.sources[${j}] needs "regex" or "json" (only a plain version file like .nvmrc may omit both)`);
        }
        const src: PinSource = { file: s.file };
        if (typeof s.regex === "string") {
          try {
            new RegExp(s.regex, "m");
          } catch {
            throw new Error(`${label}.pin.sources[${j}].regex is not a valid regular expression`);
          }
          src.regex = s.regex;
        }
        if (typeof s.json === "string") src.json = s.json;
        return src;
      });
      out.pin = { sources, compare };
    }
    return out;
  });
}

/** Validate + default a parsed config object. Never throws: returns an error string. */
export function parseConfig(raw: unknown): ConfigResult {
  try {
    if (!isRecord(raw)) throw new Error("config must be a JSON object");
    if (typeof raw.project !== "string" || !sanitizeIdentifier(raw.project)) {
      throw new Error('"project" is required (a short name; it prefixes throwaway database names)');
    }
    if (!isRecord(raw.test)) throw new Error('"test.unit.command" is required — the plugin never guesses how to run your tests');
    const unit = parseSuite(raw.test.unit, "test.unit");
    const e2e = raw.test.e2e === undefined ? null : parseSuite(raw.test.e2e, "test.e2e");

    const hooks = isRecord(raw.hooks) ? raw.hooks : {};
    const secret = isRecord(raw.secretScan) ? raw.secretScan : {};
    let secretCommand: string[] = ["gitleaks", "version"];
    if (secret.command !== undefined) {
      const c = stringArray(secret.command, "secretScan.command");
      if (typeof c === "string") throw new Error(c);
      secretCommand = c;
    }
    const handoffs = isRecord(raw.handoffs) ? raw.handoffs : {};
    const wt = isRecord(raw.worktree) ? raw.worktree : {};
    const basePort = wt.basePort === undefined ? 9000 : wt.basePort;
    if (typeof basePort !== "number" || !Number.isInteger(basePort) || basePort < 1 || basePort > 65_000) {
      throw new Error("worktree.basePort must be an integer between 1 and 65000");
    }
    const linkDirs = wt.linkDirs === undefined ? [] : wt.linkDirs;
    if (!Array.isArray(linkDirs) || !linkDirs.every((d) => typeof d === "string" && d)) {
      throw new Error("worktree.linkDirs must be an array of directory names");
    }
    const linkChecked = (linkDirs as string[]).map((d) => relativeDir(d, "worktree.linkDirs[]", d));

    let extra: string[] = [];
    if (isRecord(raw.guard) && raw.guard.extraForbiddenSignatures !== undefined) {
      const e = raw.guard.extraForbiddenSignatures;
      if (!Array.isArray(e) || !e.every((x) => typeof x === "string" && x)) {
        throw new Error("guard.extraForbiddenSignatures must be an array of non-empty strings");
      }
      extra = e as string[];
    }
    if (raw.gateProvider !== undefined && raw.gateProvider !== null && typeof raw.gateProvider !== "string") {
      throw new Error("gateProvider must be a string or null");
    }

    const paths = isRecord(raw.paths) ? raw.paths : {};
    const globList = (v: unknown, label: string): string[] => {
      if (v === undefined) return [];
      if (!Array.isArray(v) || !v.every((g) => typeof g === "string" && g && !g.includes("\n") && !g.includes('"'))) {
        throw new Error(`${label} must be an array of glob strings (no quotes or newlines)`);
      }
      return v as string[];
    };

    const config: PipelineConfig = {
      project: raw.project,
      description: typeof raw.description === "string" ? raw.description.trim() : "",
      paths: { test: globList(paths.test, "paths.test"), production: globList(paths.production, "paths.production") },
      test: { unit, e2e },
      toolchain: parseToolchain(raw.toolchain),
      hooks: {
        required: hooks.required === true,
        setupHint: typeof hooks.setupHint === "string" ? hooks.setupHint : "set core.hooksPath to your hooks directory",
      },
      secretScan: {
        required: secret.required === true,
        command: secretCommand,
        hint: typeof secret.hint === "string" ? secret.hint : "install the secret scanner (e.g. brew install gitleaks)",
      },
      handoffs: {
        dir: relativeDir(handoffs.dir, "handoffs.dir", "docs/handoffs"),
        trackedProbe: typeof handoffs.trackedProbe === "string" && handoffs.trackedProbe ? handoffs.trackedProbe : "decisions.md",
      },
      worktree: {
        dir: relativeDir(wt.dir, "worktree.dir", ".worktrees"),
        basePort,
        dbNamePrefix: typeof wt.dbNamePrefix === "string" && sanitizeIdentifier(wt.dbNamePrefix) ? sanitizeIdentifier(wt.dbNamePrefix) : sanitizeIdentifier(raw.project),
        linkDirs: linkChecked,
      },
      gateProvider: typeof raw.gateProvider === "string" && raw.gateProvider ? raw.gateProvider : null,
      extraForbiddenSignatures: extra,
    };
    return { config, error: null };
  } catch (err) {
    return { config: null, error: err instanceof Error ? err.message : String(err) };
  }
}

export function loadPipelineConfig(
  repoRoot: string,
  readFile: (p: string) => string | null = (p) => {
    try {
      return fs.readFileSync(p, "utf8");
    } catch {
      return null;
    }
  },
): ConfigResult {
  const file = path.join(repoRoot, CONFIG_RELATIVE_PATH);
  const text = readFile(file);
  if (text === null) {
    return {
      config: null,
      error: `${CONFIG_RELATIVE_PATH} not found in ${repoRoot} — copy a preset from the playbook (workflow/opencode/templates/presets/) or run install.mjs; the pipeline will not guess your test command`,
    };
  }
  let raw: unknown;
  try {
    raw = JSON.parse(text);
  } catch (err) {
    return { config: null, error: `${CONFIG_RELATIVE_PATH} is not valid JSON: ${err instanceof Error ? err.message : String(err)}` };
  }
  const parsed = parseConfig(raw);
  return parsed.error === null ? parsed : { config: null, error: `${CONFIG_RELATIVE_PATH}: ${parsed.error}` };
}

// ---------------------------------------------------------------------------
// OpenCode config (opencode.json) helpers — the source of truth for models.
// ---------------------------------------------------------------------------

export interface OpenCodeConfigFile {
  model?: string;
  provider?: Record<string, { options?: { baseURL?: string } }>;
  agent?: Record<string, { model?: string }>;
  mcp?: Record<string, unknown>;
}

/** Repo opencode.json first, then the user's global one. Unreadable files are skipped. */
export function readOpenCodeConfigs(repoRoot: string, readFile: (p: string) => string | null): OpenCodeConfigFile[] {
  const out: OpenCodeConfigFile[] = [];
  for (const file of [path.join(repoRoot, "opencode.json"), path.join(os.homedir(), ".config/opencode/opencode.json")]) {
    const text = readFile(file);
    if (text === null) continue;
    try {
      const parsed = JSON.parse(text) as unknown;
      if (isRecord(parsed)) out.push(parsed as OpenCodeConfigFile);
    } catch {
      // not JSON — try the next candidate
    }
  }
  return out;
}

/** The role's model as OpenCode would resolve it: the role's `agent.<role>.model`
 *  in opencode.json, else its agent-file frontmatter, else the top-level `model`. */
export function resolveRoleModel(
  role: string,
  configs: OpenCodeConfigFile[],
  agentFileText: string | null,
): string | null {
  for (const c of configs) {
    const m = c.agent?.[role]?.model;
    if (typeof m === "string" && m) return m;
  }
  const fm = agentFileText?.match(/^model:\s*["']?([\w./:@-]+)/m);
  if (fm) return fm[1];
  for (const c of configs) {
    if (typeof c.model === "string" && c.model) return c.model;
  }
  return null;
}

export function providerOf(model: string): string {
  return model.includes("/") ? model.slice(0, model.indexOf("/")) : model;
}

export function findBaseURL(configs: OpenCodeConfigFile[], provider: string): string | undefined {
  for (const c of configs) {
    const url = c.provider?.[provider]?.options?.baseURL;
    if (typeof url === "string" && url) return url;
  }
  return undefined;
}

/** The roles the pipeline spawns. D and U are only needed for UI stories. */
export const PIPELINE_ROLES = [
  "orchestrator",
  "tester",
  "feature-implementor",
  "architecture-reviewer",
  "spec-auditor",
  "designer",
  "playwright-ui-walkthrough",
] as const;
export const UI_ONLY_ROLES: readonly string[] = ["designer", "playwright-ui-walkthrough"];
