// coding-pipeline plugin — pre-flight engine (pure logic + check execution).
//
// Port of the Anthropic pipeline's Phase 1 (playbook #672/#706), rehosted on
// OpenCode: the PLUGIN executes every check and returns one structured
// payload. There is no probe-agent transcriber and no LLM in the loop.
//
// Hard contracts (plan + story #725):
//   - run EVERY check, even after a row fails; one payload lists every failure.
//   - NEVER run the product test suite or e2e, and NEVER invoke a commit hook
//     (#705): the configured test commands, `npm run check` and friends, and
//     `git commit` (which fires the hook) are forbidden in this engine.
//   - T/F model reachability is a real HTTP probe; down → fail closed.
//     There is no silent fallback to another provider.
//   - Nothing here assumes a stack: what to probe (toolchains, hooks, secret
//     scanner, handoffs dir) comes from the project's pipeline.config.json.
//     A missing or invalid config is itself a failing row.

import path from "node:path";

import {
  PIPELINE_ROLES,
  UI_ONLY_ROLES,
  findBaseURL,
  providerOf,
  readOpenCodeConfigs,
  resolveRoleModel,
  type PinSource,
  type ToolchainCheck,
} from "./config.ts";

import type {
  CheckContext,
  CheckName,
  CheckResult,
  HumanResponsibility,
  PhaseName,
  PreFlightInput,
  PreFlightReport,
} from "./types.ts";

// ---------------------------------------------------------------------------
// Phase order + rigor dial (the same phases as the Claude runner's
// coding-pipeline-logic.mjs; here the plugin is the single owner of process order).
// ---------------------------------------------------------------------------

// The Anthropic pipeline's phase order, rehosted. The plugin is the single
// owner: an LLM may not skip, reorder, or waive any of these (stories #726+
// wire the gates that enforce the transitions).
export const PHASE_ORDER: readonly PhaseName[] = [
  "preflight",
  "survey-brief",
  "design",
  "architecture-review",
  "t-red",
  "implement",
  "t-green",
  "a-dup",
  "ui-walkthrough",
  "spec-audit",
  "merge",
];

// Story #728: `pipeline_advance` refuses survey-brief unless the last
// pre-flight is pass (or a named waiver). D and U are skipped — declared,
// never silent — when there is no UI surface.
export const PHASE_CONDITIONS: Record<string, { uiSurface?: boolean }> = {
  design: { uiSurface: true },
  "ui-walkthrough": { uiSurface: true },
};

export type Phase = (typeof PHASE_ORDER)[number];
export const PHASE_INDEX = new Map<Phase, number>(PHASE_ORDER.map((p, i) => [p, i]));

export const RIGOR_LEVELS = ["direct", "in-session", "second-opinion", "panel"] as const;
export type RigorLevel = (typeof RIGOR_LEVELS)[number];

export function rigorNeedsOutsideModel(rigor: RigorLevel): boolean {
  return rigor === "second-opinion" || rigor === "panel";
}

// ---------------------------------------------------------------------------
// Check execution — every check is exit code / HTTP status / git+npm output
// parsed in code.
// ---------------------------------------------------------------------------

const CONFIG_SKIPPED = "skipped: no valid pipeline config (see the pipeline-config row)";

async function pipelineConfigCheck(ctx: CheckContext): Promise<CheckResult> {
  if (ctx.config) {
    return row("pipeline-config", "pass", `.opencode/pipeline.config.json loaded (project ${ctx.config.project}; unit suite: ${ctx.config.test.unit.command.join(" ")})`);
  }
  return row("pipeline-config", "fail", ctx.configError ?? ".opencode/pipeline.config.json is missing or invalid");
}

async function roleModelsCheck(ctx: CheckContext, input: PreFlightInput): Promise<CheckResult> {
  const configs = readOpenCodeConfigs(ctx.repoRoot, ctx.readFile);
  const missing: string[] = [];
  for (const role of PIPELINE_ROLES) {
    if (UI_ONLY_ROLES.includes(role) && input.uiSurface !== true) continue;
    const model = resolveRoleModel(role, configs, ctx.readFile(`${ctx.repoRoot}/.opencode/agents/${role}.md`));
    if (!model) missing.push(role);
  }
  if (missing.length > 0) {
    return row(
      "role-models",
      "fail",
      `no model resolved for: ${missing.join(", ")} — set "agent.<role>.model" (or a top-level "model") in opencode.json; the pipeline does not pick models for you`,
    );
  }
  return row("role-models", "pass", "every pipeline role resolves to a model in opencode.json");
}

async function toolchainCheck(ctx: CheckContext, tc: ToolchainCheck): Promise<CheckResult> {
  const name = `toolchain:${tc.name}`;
  const hint = tc.hint ? ` — ${tc.hint}` : "";
  const r = await ctx.run(tc.command[0], tc.command.slice(1));
  if (r.code !== 0) {
    return row(name, "fail", `\`${tc.command.join(" ")}\` failed: ${firstLine(r.stderr) || "not installed"}${hint}`);
  }
  // Some tools print their version on stderr.
  const out = `${r.stdout}\n${r.stderr}`;
  const match = out.match(tc.versionRegex ? new RegExp(tc.versionRegex) : /(\d+\.\d+(?:\.\d+)?)/);
  const installed = match ? (match[1] ?? match[0]) : null;
  const installedV = installed ? parseVersion(installed) : null;
  if (!installed || !installedV) {
    return row(name, "fail", `could not read a version from \`${tc.command.join(" ")}\` output: ${firstLine(out)}${hint}`);
  }
  if (!tc.pin) return row(name, "pass", `${tc.name} ${installed}`);

  const pin = await readPin(ctx, tc.pin.sources);
  if (!pin) {
    return row(name, "fail", `no version pin found in ${tc.pin.sources.map((s) => s.file).join(", ")}`);
  }
  const pinV = parseVersion(pin);
  if (!pinV) return row(name, "fail", `cannot parse the pin ${JSON.stringify(pin)}`);
  if (!satisfies(installedV, pinV, tc.pin.compare)) {
    return row(name, "fail", `${tc.name} ${installed} does not satisfy pin ${JSON.stringify(pin)} (${tc.pin.compare})${hint}`);
  }
  return row(name, "pass", `${tc.name} ${installed} satisfies ${JSON.stringify(pin)}`);
}

async function secretScanCheck(ctx: CheckContext): Promise<CheckResult> {
  const cfg = ctx.config;
  if (!cfg) return row("secret-scan", "na", CONFIG_SKIPPED);
  if (!cfg.secretScan.required) return row("secret-scan", "na", "secretScan.required is false in the pipeline config");
  const r = await ctx.run(cfg.secretScan.command[0], cfg.secretScan.command.slice(1));
  if (r.code !== 0) {
    return row("secret-scan", "fail", `\`${cfg.secretScan.command.join(" ")}\` failed — ${cfg.secretScan.hint}`);
  }
  return row("secret-scan", "pass", r.stdout.split("\n")[0]?.trim() || "installed");
}

async function gitHooksCheck(ctx: CheckContext): Promise<CheckResult> {
  const cfg = ctx.config;
  if (!cfg) return row("git-hooks-active", "na", CONFIG_SKIPPED);
  if (!cfg.hooks.required) return row("git-hooks-active", "na", "hooks.required is false in the pipeline config");
  const r = await ctx.run("git", ["config", "core.hooksPath"]);
  const hooksPath = r.stdout.trim();
  if (r.code !== 0 || !hooksPath) {
    return row("git-hooks-active", "fail", `core.hooksPath is not set — ${cfg.hooks.setupHint}`);
  }
  if (!(await ctx.exists(resolvePath(ctx, hooksPath)))) {
    return row("git-hooks-active", "fail", `core.hooksPath points at ${hooksPath} which does not exist`);
  }
  return row("git-hooks-active", "pass", `core.hooksPath = ${hooksPath}`);
}

// Story #727: the worktree-present check is now PROVISIONING, not a gate.
//
// #725's version failed when the checkout was the primary ("the pipeline runs
// in a worktree") and expected the HUMAN to hand-create the NNNN-slug worktree
// before briefing. #727 inverts that: the PLUGIN owns the worktree lifecycle,
// so when the run starts in the primary checkout the check creates the worktree
// itself (branch off the primary's HEAD, symlink node_modules, add the local
// .git/info/exclude) and then re-anchors the check context into it — so every
// later check and phase (survey-brief … t-green) executes in the worktree, and
// the run is never on the primary's branch.
//
// The provision is an injected seam (`ctx.provision`), so this engine stays
// pure and unit-testable: the tests stub it, the plugin entry supplies the real
// one. When the seam is absent (the standalone CLI, which is a pure pre-flight
// runner that must not mutate the repo), the check degrades to #725's behaviour
// — fail if in the primary — so the CLI contract is unchanged.
//
// The re-anchor (`ctx.repoRoot = <worktree>`) is load-bearing for everything AFTER
// pre-flight: the run's later phases execute in the worktree. Because it mutates
// the shared context, `runAllChecks` runs this check LAST, after every other check
// has finished: the whole report then describes ONE checkout (the primary, where
// the project's config lives and may not be committed yet), and no check races the
// change of root. (Checks run concurrently with each other, so an in-flight
// re-anchor would make each row's root depend on timing.)
async function worktreeCheck(ctx: CheckContext, input: PreFlightInput): Promise<CheckResult> {
  const [commonDir, gitDir] = [
    (await ctx.run("git", ["rev-parse", "--git-common-dir"])).stdout.trim(),
    (await ctx.run("git", ["rev-parse", "--git-dir"])).stdout.trim(),
  ];
  if (commonDir !== gitDir) {
    // Already inside a worktree: the run is provisioned. (Whether it is THIS
    // run's worktree is the caller's concern — the plugin's state tracks which
    // worktree the run is in; pre-flight just confirms a worktree is present.)
    return row("worktree-present", "pass", `inside worktree (git-dir ${gitDir})`);
  }

  // In the PRIMARY checkout. The plugin provisions the run's worktree here.
  if (!ctx.provision) {
    return row("worktree-present", "fail", "current checkout is the PRIMARY checkout and no worktree provisioner is wired; the CLI cannot provision — run the pipeline via the plugin (which creates the NNNN-slug worktree)");
  }
  if (input.issueNumber == null) {
    return row("worktree-present", "fail", "current checkout is the PRIMARY checkout and no issue number is known, so the NNNN-slug worktree cannot be derived — pass the issue to pipeline_preflight");
  }
  const slug = input.slug ?? `issue-${input.issueNumber}`;
  const result = ctx.provision(input.issueNumber, slug, ctx.repoRoot);
  if (!result.ok || !result.path) {
    return row("worktree-present", "fail", `worktree provisioning failed: ${result.error ?? "unknown error"}`);
  }
  // Re-anchor the shared context into the freshly-created worktree so every
  // later check + phase runs there (see the comment above).
  ctx.repoRoot = result.path;
  ctx.provisioned = result.info;
  return row("worktree-present", "pass", `provisioned run's worktree ${result.info?.path ?? result.path} (branch ${result.info?.branch ?? "?"}, port ${result.info?.port ?? "?"}) — all later phases run there`);
}

async function handoffsTrackedCheck(ctx: CheckContext): Promise<CheckResult> {
  const cfg = ctx.config;
  if (!cfg) return row("handoffs-tracked", "na", CONFIG_SKIPPED);
  const probe = `${cfg.handoffs.dir}/${cfg.handoffs.trackedProbe}`;
  const r = await ctx.run("git", ["check-ignore", "-q", probe]);
  // exit 0 → ignored (fail); exit 1 → tracked (pass); other → fail
  if (r.code === 1) {
    return row("handoffs-tracked", "pass", `${cfg.handoffs.dir}/ is not gitignored`);
  }
  if (r.code === 0) {
    return row("handoffs-tracked", "fail", `${probe} is gitignored — journals must be committed`);
  }
  return row("handoffs-tracked", "fail", `git check-ignore error: ${r.stderr.trim() || r.code}`);
}

async function orphanedWorktreesCheck(ctx: CheckContext): Promise<CheckResult> {
  // The default branch: origin/HEAD if the remote has one, else init.defaultBranch, else main/master
  // (remote first, then local, for a repo with no remote yet).
  const symR = await ctx.run("git", ["symbolic-ref", "--short", "refs/remotes/origin/HEAD"]);
  let defaultBranch = symR.code === 0 ? symR.stdout.trim().replace(/^origin\//, "") : "";
  if (!defaultBranch) {
    const cfgR = await ctx.run("git", ["config", "init.defaultBranch"]);
    const configured = cfgR.code === 0 ? cfgR.stdout.trim() : "";
    const refs = [...(configured ? [`origin/${configured}`, configured] : []), "origin/main", "origin/master", "main", "master"];
    for (const ref of refs) {
      const r = await ctx.run("git", ["rev-parse", "--verify", "--quiet", ref]);
      if (r.code === 0) {
        defaultBranch = ref.replace(/^origin\//, "");
        break;
      }
    }
  }
  if (!defaultBranch) {
    return row("orphaned-worktrees", "fail", "cannot determine the default branch (no origin/HEAD, and no main or master, remote or local)");
  }
  const listR = await ctx.run("git", ["worktree", "list", "--porcelain"]);
  if (listR.code !== 0) {
    return row("orphaned-worktrees", "fail", `git worktree list failed: ${listR.stderr.trim()}`);
  }
  // A worktree at a path DISTINCT from the current checkout is an orphan
  // candidate. The current worktree is excluded by its branch (the pipeline
  // branch is not the default branch, and it is never merged nor gone).
  // The root worktree is excluded by a path-prefix check: it is the current
  // checkout (repoRoot) or an ancestor of it. A SUBPATH of repoRoot is NOT
  // the root — pipeline worktrees under the configured worktree dir are
  // children of repoRoot and must still be flagged. The old
  // `current !== repoRoot` check discarded exactly those, silently.
  const root = ctx.repoRoot.replace(/[\\/]+$/, "");
  const lines = listR.stdout.split("\n");
  const orphans: string[] = [];
  let current = "";
  // Skip the current checkout and any ANCESTOR of it (the primary, when the run is inside
  // one of its worktrees). Descendants of the root are exactly where run worktrees live and
  // must be flagged.
  const isDistinctWorktree = (wt: string): boolean =>
    wt !== root && !root.startsWith(wt + path.sep);
  for (const line of lines) {
    if (line.startsWith("worktree ")) {
      current = line.slice(9);
    } else if (line.startsWith("branch ")) {
      const branchRef = line.slice(7);
      // The bare branch name (git branch --merged lists bare names; the
      // worktree line reports refs/heads/<name>). Compare the bare name so
      // "refs/heads/issue-0999-stale" matches the "--merged" line
      // "issue-0999-stale" — comparing the ref against a bare listing would
      // never match and orphans would be silently missed.
      const branchName = branchRef.replace(/^refs\/heads\//, "");
      // Skip the default branch and the current checkout (root, or an
      // ancestor of root); a distinct worktree on a non-default branch is a
      // candidate.
      if (current && branchName !== defaultBranch && isDistinctWorktree(current)) {
        const headR = await ctx.run("git", ["log", "--oneline", "-1", branchRef]);
        const mergedR = await ctx.run("git", ["branch", "--merged", defaultBranch]);
        const isMerged = mergedR.code === 0 && mergedR.stdout.includes(branchName);
        const isGone = headR.code !== 0;
        if (isMerged || isGone) orphans.push(`${current} (${branchName})`);
      }
    }
  }
  if (orphans.length > 0) {
    return row(
      "orphaned-worktrees",
      "fail",
      `${orphans.length} orphaned worktree(s): ${orphans.join("; ")} — prune before briefing`,
    );
  }
  return row("orphaned-worktrees", "pass", "no orphaned worktrees");
}

async function issueReadyCheck(ctx: CheckContext, input: PreFlightInput): Promise<CheckResult> {
  if (input.issueNumber === undefined) {
    return row("issue-ready-to-brief", "na", "no issue number supplied; nothing to verify");
  }
  const r = await ctx.run("gh", ["issue", "view", String(input.issueNumber), "--json", "title,body"]);
  if (r.code !== 0) {
    return row("issue-ready-to-brief", "fail", `gh issue view ${input.issueNumber} failed: ${r.stderr.trim()}`);
  }
  let body: string;
  try {
    body = JSON.parse(r.stdout).body ?? "";
  } catch {
    return row("issue-ready-to-brief", "fail", `issue ${input.issueNumber} body is not JSON-parseable`);
  }
  const hasHeading = /^#{1,6}\s/m.test(body);
  const linksExistingDoc = /!\[[^\]]*\]\([^)]+\.md\)/.test(body);
  const longEnough = body.length >= 200;
  if ((longEnough && hasHeading) || (longEnough && linksExistingDoc)) {
    return row("issue-ready-to-brief", "pass", `issue #${input.issueNumber} is ready to brief from`);
  }
  return row(
    "issue-ready-to-brief",
    "fail",
    `issue #${input.issueNumber} body is ${body.length} chars with ${hasHeading ? "a heading" : "no heading"}${linksExistingDoc ? " and doc links" : ""} — need ≥200 chars with a heading or an existing-doc link`,
  );
}

async function registryAuthCheck(ctx: CheckContext): Promise<CheckResult> {
  const npmrc = await ctx.readFile(`${ctx.repoRoot}/.npmrc`);
  if (!npmrc) {
    return row("private-registry-auth", "na", "no project .npmrc with scoped registries; nothing to probe");
  }
  const scopes = [...npmrc.matchAll(/^@([\w.-]+):registry=(\S+)/gm)].map((m) => m[1]);
  if (scopes.length === 0) {
    return row("private-registry-auth", "na", ".npmrc declares no @scope:registry mappings");
  }
  const byScope = new Map<string, string>();
  for (const m of npmrc.matchAll(/^@([\w.-]+):registry=(\S+)/gm)) byScope.set(m[1], m[2]);
  const failures: string[] = [];
  for (const [scope, registry] of byScope) {
    const r = await ctx.run("npm", ["whoami", `--registry=${registry}`]);
    if (r.code !== 0) failures.push(`@${scope} (${registry}): ${firstLine(r.stderr)}`);
  }
  if (failures.length > 0) {
    return row("private-registry-auth", "fail", failures.join("; "));
  }
  return row("private-registry-auth", "pass", `npm whoami ok for ${[...byScope.keys()].map((s) => `@${s}`).join(", ")}`);
}

async function dualReviewInterfaceCheck(ctx: CheckContext, input: PreFlightInput): Promise<CheckResult> {
  if (!rigorNeedsOutsideModel(input.rigor)) {
    return row("dual-review-interface", "na", `rigor is ${input.rigor}; no outside-model gate needed`);
  }
  const candidates = [
    "dual-review.sh",
    `${ctx.repoRoot}/workflow/dual-review.sh`,
    `${ctx.repoRoot}/docs/playbook/workflow/dual-review.sh`,
    `${ctx.repoRoot}/.agents/scripts/dual-review.sh`,
  ];
  for (const c of candidates) {
    const r = await ctx.run(c, ["--mode", "brief", "--brief", "/dev/null", "--out", "/dev/null", "--dump-only"]);
    // exit 0 → script resolved and accepted the interface
    // exit 1 → usage/validation error (interface mismatch)
    if (r.code === 0) {
      return row("dual-review-interface", "pass", `dual-review.sh resolved (${c}) and accepts --mode/--brief/--out`);
    }
    if (r.code === 1) {
      return row("dual-review-interface", "fail", `${c} exists but rejects the --mode/--brief/--out interface: ${firstLine(r.stderr)}`);
    }
  }
  return row(
    "dual-review-interface",
    "fail",
    `rigor is ${input.rigor} but no reachable dual-review.sh (tried: ${candidates.join("; ")})`,
  );
}

async function gateModelCheck(ctx: CheckContext, input: PreFlightInput): Promise<CheckResult> {
  if (!rigorNeedsOutsideModel(input.rigor)) {
    return row("gate-model-reachable", "na", `rigor is ${input.rigor}; no gate model to probe`);
  }
  // Which provider is the T/F model served by? The config can name it; else it
  // is the provider of the tester role's resolved model.
  const configs = readOpenCodeConfigs(ctx.repoRoot, ctx.readFile);
  const explicit = ctx.config?.gateProvider ?? null;
  const testerModel = resolveRoleModel("tester", configs, ctx.readFile(`${ctx.repoRoot}/.opencode/agents/tester.md`));
  const provider = explicit ?? (testerModel ? providerOf(testerModel) : null);
  const url = ctx.modelBaseURL ?? (provider ? findBaseURL(configs, provider) : undefined);
  if (!url) {
    if (explicit) {
      return row("gate-model-reachable", "fail", `no baseURL configured for the gate provider "${explicit}" (provider.${explicit}.options.baseURL in opencode.json)`);
    }
    if (!provider) {
      return row("gate-model-reachable", "fail", "no model resolved for the tester role, so there is no gate provider to probe");
    }
    return row("gate-model-reachable", "na", `provider "${provider}" declares no options.baseURL (hosted API); nothing to HTTP-probe — set gateProvider in the pipeline config to force a probe`);
  }
  try {
    const probe = await ctx.httpGet(url + "/models");
    if (probe.status >= 200 && probe.status < 300 && probe.body.length > 0) {
      return row("gate-model-reachable", "pass", `HTTP ${probe.status} from ${url}/models`);
    }
    return row("gate-model-reachable", "fail", `HTTP ${probe.status} from ${url}/models — fail closed, no fallback`);
  } catch (err) {
    return row("gate-model-reachable", "fail", `unreachable ${url}/models: ${err instanceof Error ? err.message : String(err)} — fail closed`);
  }
}

async function playwrightMcpCheck(ctx: CheckContext, input: PreFlightInput): Promise<CheckResult> {
  if (input.uiSurface !== true) {
    return row("playwright-mcp", "na", "N/A: uiSurface is false; Phase U not reached");
  }
  // OpenCode exposes MCP tools via the SDK client. Presence = a playwright-
  // prefixed tool id in the tool list. (Self-reported row in Claude #687.)
  // OpenCode registers MCP tools with provider-prefixed ids; presence is the
  // #687 row (self-reported in Claude, deterministically probed here).
  //
  // NB: no `as` cast with a generic type here. The Vite/oxc transformer
  // (used by Vitest) fails to parse `expr as { generic }` ("Expected `,` or
  // `>` but found `}`"); untyped locals + manual checks keep the file
  // parseable under both oxc and tsc.
  const client = (ctx as unknown as { client?: unknown }).client;
  const toolNamespace = (client as { tool?: unknown } | null)?.tool as
    | { list?: unknown }
    | undefined;
  const list = (toolNamespace as { list?: unknown } | undefined)?.list;
  if (typeof list !== "function") {
    // No SDK client (e.g. direct runner invocation) → config-level probe: is an MCP
    // server whose name mentions playwright declared in opencode.json (parsed, not grepped)?
    const declared = readOpenCodeConfigs(ctx.repoRoot, ctx.readFile).some((c) =>
      Object.keys(c.mcp ?? {}).some((k) => /playwright/i.test(k)),
    );
    if (declared) {
      return row("playwright-mcp", "pass", "playwright MCP declared in opencode config (a declaration check: whether the tool is live in the session is not probed here)");
    }
    return row("playwright-mcp", "fail", "uiSurface is true but no playwright MCP is configured in opencode.json");
  }
  const listed = (await (list as () => Promise<unknown>)()) as { data?: unknown } | null;
  const tools = (listed?.data ?? []) as { id: string }[];
  const hit = tools.find((t) => /^mcp__playwright/i.test(t.id) || /^playwright__/i.test(t.id));
  if (hit) return row("playwright-mcp", "pass", `playwright MCP tool present (${hit.id})`);
  return row("playwright-mcp", "fail", "uiSurface is true but no playwright MCP tool is registered");
}

// ---------------------------------------------------------------------------
// Engine
// ---------------------------------------------------------------------------

export function runAllChecks(ctx: CheckContext, input: PreFlightInput): Promise<PreFlightReport> {
  type Runner = () => Promise<CheckResult>;
  const toolchain = ctx.config?.toolchain ?? [];
  const runners: { name: CheckName; run: Runner }[] = [
    { name: "pipeline-config", run: () => pipelineConfigCheck(ctx) },
    { name: "role-models", run: () => roleModelsCheck(ctx, input) },
    ...toolchain.map((tc) => ({ name: `toolchain:${tc.name}`, run: () => toolchainCheck(ctx, tc) })),
    { name: "secret-scan", run: () => secretScanCheck(ctx) },
    { name: "git-hooks-active", run: () => gitHooksCheck(ctx) },
    { name: "worktree-present", run: () => worktreeCheck(ctx, input) },
    { name: "handoffs-tracked", run: () => handoffsTrackedCheck(ctx) },
    { name: "orphaned-worktrees", run: () => orphanedWorktreesCheck(ctx) },
    { name: "issue-ready-to-brief", run: () => issueReadyCheck(ctx, input) },
    { name: "private-registry-auth", run: () => registryAuthCheck(ctx) },
    { name: "dual-review-interface", run: () => dualReviewInterfaceCheck(ctx, input) },
    { name: "gate-model-reachable", run: () => gateModelCheck(ctx, input) },
    { name: "playwright-mcp", run: () => playwrightMcpCheck(ctx, input) },
  ];

  return (async () => {
    // Run every check; a throw is captured as a failed row, never aborts the rest.
    const safely = async ({ name, run }: { name: CheckName; run: Runner }): Promise<CheckResult> => {
      try {
        return await run();
      } catch (err) {
        return row(name, "fail", `check threw: ${err instanceof Error ? err.message : String(err)}`);
      }
    };
    // Everything but worktree provisioning runs concurrently against the SAME root;
    // provisioning (which re-anchors ctx.repoRoot) runs after they have all finished.
    // Rows keep their declared order in the report.
    const provisionIdx = runners.findIndex((r) => r.name === "worktree-present");
    const results: CheckResult[] = new Array(runners.length);
    await Promise.all(
      runners.map(async (r, i) => {
        if (i !== provisionIdx) results[i] = await safely(r);
      }),
    );
    // Read the model mapping from the same (primary) root as every other row, BEFORE
    // provisioning re-anchors ctx.repoRoot into a worktree that may not have the
    // project's uncommitted opencode.json / agents.
    const modelMapping = await readModelMapping(ctx);
    if (provisionIdx !== -1) results[provisionIdx] = await safely(runners[provisionIdx]);

    const humanResponsibilities = buildHumanResponsibilities(results, input);
    const failures = results.filter((r) => r.status === "fail");

    return {
      ok: failures.length === 0,
      checks: results,
      humanResponsibilities,
      modelMapping,
      combinedError:
        failures.length === 0
          ? null
          : failures.map((f) => `${f.name}: ${f.details}`).join("; "),
    };
  })();
}

function buildHumanResponsibilities(results: CheckResult[], input: PreFlightInput): HumanResponsibility[] {
  const rows: HumanResponsibility[] = [];
  const orphan = results.find((r) => r.name === "orphaned-worktrees" && r.status === "fail");
  if (orphan) {
    rows.push({ label: "Prune orphaned worktrees", detail: orphan.details });
  }
  if (input.uiSurface === true) {
    rows.push({
      label: "Approve the wireframe",
      detail: "uiSurface is true — the design stop requires human sign-off before tests/code",
    });
  }
  rows.push({
    label: "Role subagents spawn as the project's defs",
    detail: "confirm @orchestrator/@tester/etc resolve to .opencode/agents in this repo",
  });
  return rows;
}

async function readModelMapping(ctx: CheckContext): Promise<Record<string, string>> {
  // Report the configured OpenCode agent→model mapping VERBATIM (opencode.json
  // first, then agent-file frontmatter, then the global default model). Never
  // reconcile against Claude's opus/sonnet axis (#706 false positive).
  const configs = readOpenCodeConfigs(ctx.repoRoot, ctx.readFile);
  const names = new Set<string>(PIPELINE_ROLES);
  for (const file of await listMarkdownFiles(ctx, `${ctx.repoRoot}/.opencode/agents`)) {
    names.add(file.split("/").pop()!.replace(/\.md$/, ""));
  }
  const mapping: Record<string, string> = {};
  for (const name of names) {
    const model = resolveRoleModel(name, configs, ctx.readFile(`${ctx.repoRoot}/.opencode/agents/${name}.md`));
    if (model) mapping[name] = model;
  }
  return mapping;
}

// ---------------------------------------------------------------------------
// Small helpers
// ---------------------------------------------------------------------------

function row(name: CheckName, status: "pass" | "fail" | "na", details: string): CheckResult {
  return { name, status, details };
}

function firstLine(s: string): string {
  return s.split("\n").map((l) => l.trim()).find(Boolean) ?? "";
}

type Version = [number, number, number];

/** First x[.y[.z]] in the text, or null. A leading "v" is fine. */
function parseVersion(text: string): Version | null {
  const m = text.match(/(\d+)(?:\.(\d+))?(?:\.(\d+))?/);
  if (!m) return null;
  return [Number(m[1]), Number(m[2] ?? "0"), Number(m[3] ?? "0")];
}

function cmpVersion(a: Version, b: Version): number {
  for (let i = 0; i < 3; i += 1) {
    if (a[i] !== b[i]) return a[i] < b[i] ? -1 : 1;
  }
  return 0;
}

function satisfies(installed: Version, pin: Version, compare: "same-major" | "at-least"): boolean {
  if (compare === "same-major" && installed[0] !== pin[0]) return false;
  return cmpVersion(installed, pin) >= 0;
}

function jsonPath(text: string, dotted: string): string | null {
  try {
    let cur: unknown = JSON.parse(text);
    for (const key of dotted.split(".")) {
      if (typeof cur !== "object" || cur === null) return null;
      cur = (cur as Record<string, unknown>)[key];
    }
    return typeof cur === "string" ? cur : null;
  } catch {
    return null;
  }
}

/** First pin source that yields a value wins. */
async function readPin(ctx: CheckContext, sources: PinSource[]): Promise<string | null> {
  for (const src of sources) {
    const text = await ctx.readFile(`${ctx.repoRoot}/${src.file}`);
    if (text === null) continue;
    if (src.json) {
      const v = jsonPath(text, src.json);
      if (v) return v.trim();
    } else if (src.regex) {
      const m = text.match(new RegExp(src.regex, "m"));
      if (m) return (m[1] ?? m[0]).trim();
    } else if (text.trim()) {
      return text.trim();
    }
  }
  return null;
}

function resolvePath(ctx: CheckContext, path: string): string {
  if (path.startsWith("/")) return path;
  return `${ctx.repoRoot}/${path}`;
}

export async function listMarkdownFiles(ctx: CheckContext, dir: string): Promise<string[]> {
  const r = await ctx.run("find", [dir, "-name", "*.md", "-type", "f", "-maxdepth", "1"]);
  if (r.code !== 0) return [];
  return r.stdout.split("\n").filter(Boolean);
}
