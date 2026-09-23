// coding-pipeline plugin — worktree provisioning (Story #727).
//
// The plugin OWNS the NNNN-slug worktree the run executes in ("Worktree-per-
// run"). It is deterministic in the issue number — never hand-picked — so two
// concurrent stories on the same repo can never collide by hand:
//
//   path   = <primary>/<worktree.dir>/NNNN-<slug>
//   branch = issue-NNNN-<slug>
//   port   = worktree.basePort + (NNNN % 100)
//   db     = <worktree.dbNamePrefix>_<NNNN>
//
// Each directory in worktree.linkDirs (e.g. node_modules for a Node project;
// empty for Go or Swift) is SYMLINKED from the primary checkout (the literal
// same files, so no native-binary ABI mismatch and no fresh install), and a
// LOCAL exclude line is appended to .git/info/exclude for each so the symlink
// is never untracked noise (.gitignore patterns ending in `/` do not match a
// symlink). .git/info/exclude resolves to the shared gitdir, so it is local-
// only: never committed, never seen by a clone.
//
// The engine is pure over the WorktreeContext seams so the tests drive it
// against stubbed git output (no live repo). The only thing that differs
// between "already in a worktree" and "in the primary checkout" is the
// --git-common-dir vs --git-dir comparison (the same test pre-flight uses).
//
// The worktree is created off the PRIMARY's current HEAD (the story's start
// base), not off whatever branch the primary happens to sit on — the run
// moves on its own branch from there.
//
// HARD BOUNDARIES:
//   - `git commit` is never spawned here (a commit hook could fire).
//     The worktree is never committed by the plugin.
//   - Dependencies are never installed by the plugin. The symlink is the
//     default; a story that changes the dependency manifest installs itself.

import path from "node:path";

import type { PipelineConfig } from "./config.ts";
import type { RunResult, WorktreeContext, WorktreeInfo } from "./types.ts";

export type WorktreeSettings = PipelineConfig["worktree"];

/** Used only when a caller supplies no settings (unit tests, ad-hoc use). */
export const DEFAULT_WORKTREE_SETTINGS: WorktreeSettings = {
  dir: ".worktrees",
  basePort: 9000,
  dbNamePrefix: "pipeline",
  linkDirs: [],
};

export interface ProvisionResult {
  ok: boolean;
  /** The worktree path the run must execute in — `info.path` when ok, null
   *  when the operation failed. Carried flat (not only via `info`) so the
   *  pre-flight seam can re-anchor its context with a single, null-checked
   *  field rather than unwrapping the nested info. */
  path: string | null;
  info: WorktreeInfo | null;
  error: string | null;
}

function pad4(n: number): string {
  return String(n).padStart(4, "0");
}

function sanitizeSlug(slug: string): string {
  // Lowercase, collapse runs of non-alphanumerics to a single dash, trim.
  // A slug is the human-facing NNNN-<slug> token; it must be filesystem- and
  // branch-safe. Empty-after-sanitize is refused by the caller.
  return slug.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "");
}

export function deriveWorktree(
  issueNumber: number,
  slug: string,
  primaryRoot: string,
  settings: WorktreeSettings = DEFAULT_WORKTREE_SETTINGS,
) {
  const nnnn = pad4(issueNumber);
  const clean = sanitizeSlug(slug);
  if (!clean) {
    throw new Error(`slug ${JSON.stringify(slug)} sanitizes to an empty token; supply a non-empty slug`);
  }
  const relPath = `${settings.dir}/${nnnn}-${clean}`;
  return {
    issueNumber,
    nnnn,
    slug: clean,
    branch: `issue-${nnnn}-${clean}`,
    path: path.join(primaryRoot, relPath),
    relPath,
    port: settings.basePort + (issueNumber % 100),
    dbName: `${settings.dbNamePrefix}_${nnnn}`,
  };
}

// ---------------------------------------------------------------------------
// Provisioning
// ---------------------------------------------------------------------------

/** Provision the run's worktree when the invocation directory is the PRIMARY
 *  checkout. This is the Story #727 pre-flight path: the plugin calls this
 *  from the worktree-present check while it is still in the primary, creates
 *  the NNNN-slug worktree off the primary's current HEAD, symlinks
 *  node_modules, and adds the local exclude — then the check re-anchors the
 *  context into the new worktree so every later phase (incl. t-red / t-green)
 *  executes there. `slug` is required (the run's token). */
export function provisionWorktreeFromPrimary(
  ctx: WorktreeContext,
  issueNumber: number,
  slug: string,
  settings: WorktreeSettings = DEFAULT_WORKTREE_SETTINGS,
): ProvisionResult {
  const top = ctx.run("git", ["rev-parse", "--show-toplevel"]);
  if (top.code !== 0) {
    return { ok: false, path: null, info: null, error: `cannot resolve the primary checkout root: ${firstLine(top.stderr)}` };
  }
  const primaryRoot = top.stdout.trim();
  return provisionWorktree(ctx, primaryRoot, issueNumber, slug, settings);
}

/** Provision (or resume) the run's worktree. `primaryRoot` is the canonical
 *  primary checkout root — the base the run's branch is created from and the
 *  source of the node_modules symlink. It is passed explicitly (resolved from
 *  the invocation dir by the caller) rather than derived here, so a caller
 *  that already lives inside the worktree is never misread as the primary. */
export function provisionWorktree(
  ctx: WorktreeContext,
  primaryRoot: string,
  issueNumber: number,
  slug: string,
  settings: WorktreeSettings = DEFAULT_WORKTREE_SETTINGS,
): ProvisionResult {
  // 1. Resolve the primary's current HEAD (the base to branch from). The run
  //    is always provisioned from the primary (see provisionWorktreeFromPrimary),
  //    so git commands here are run with the invocation dir as cwd — which is
  //    the primary at provision time.
  const head = ctx.run("git", ["rev-parse", "--verify", "--quiet", "HEAD"]);
  if (head.code !== 0) {
    return { ok: false, path: null, info: null, error: "the primary checkout has no HEAD to branch the run's worktree from" };
  }
  const baseRef = head.stdout.trim();

  // 2. Derive the deterministic identity.
  let derived;
  try {
    derived = deriveWorktree(issueNumber, slug, primaryRoot, settings);
  } catch (err) {
    return { ok: false, path: null, info: null, error: err instanceof Error ? err.message : String(err) };
  }

  const { branch, path: wtPath, port, dbName } = derived;

  // 3. Is this run's branch ALREADY a checked-out worktree? (resume path)
  //    `git worktree list --porcelain` → blocks of "worktree <path>" +
  //    "branch refs/heads/<name>". If <path> is bound to our branch, the run
  //    was interrupted and is resuming — reuse it rather than create a
  //    duplicate. (The "am I already inside the worktree" short-circuit is the
  //    check's job, not the provisioner's: the provisioner is only ever called
  //    when the context is the primary, so it always creates or resumes.)
  const listR = ctx.run("git", ["worktree", "list", "--porcelain"], { cwd: ctx.invokedIn });
  // Let, not const: the resume path may null it out after pruning a stale
  // record, which then falls through to the create path.
  let existingPath = listR.code === 0 ? findWorktreeForBranch(listR.stdout, branch) : null;

  let source: WorktreeInfo["source"] = "created";
  let reused = false;
  if (existingPath) {
    if (ctx.exists(existingPath)) {
      // A worktree for this branch already exists and is present on disk:
      // refresh the local excludes + dependency symlinks idempotently.
      ensureExclude(ctx, existingPath, excludeEntries(settings));
      ensureLinkDirs(ctx, existingPath, primaryRoot, settings.linkDirs);
      reused = true;
      source = "existing";
    } else {
      // A stale `git worktree` record points at a path that is gone (a
      // crashed run). Prune it so we can re-create cleanly below.
      const prune = ctx.run("git", ["worktree", "prune"], { cwd: ctx.invokedIn });
      if (prune.code !== 0) {
        return { ok: false, path: null, info: null, error: `stale worktree record for ${branch} could not be pruned: ${firstLine(prune.stderr)}` };
      }
      existingPath = null;
      source = "created";
    }
  }

  if (!existingPath) {
    // Create the worktree off the primary's current HEAD (the base), on a
    // fresh branch. --no-checkout is not used: we want the full checkout so
    // the run can read the tree immediately.
    const addR = ctx.run(
      "git",
      ["worktree", "add", wtPath, "-b", branch, baseRef],
      { cwd: ctx.invokedIn },
    );
    if (addR.code !== 0) {
      return {
        ok: false,
        path: null,
        info: null,
        error: `git worktree add failed: ${firstLine(addR.stderr)} (branch ${branch} at ${wtPath})`,
      };
    }
    // Symlink the dependency dirs from the primary + add the local excludes.
    ensureLinkDirs(ctx, wtPath, primaryRoot, settings.linkDirs);
    ensureExclude(ctx, wtPath, excludeEntries(settings));
    source = "created";
  }

  const info: WorktreeInfo = {
    path: wtPath,
    branch,
    primaryRoot,
    port,
    dbName,
    reused,
    source,
  };
  return { ok: true, path: wtPath, info, error: null };
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function firstLine(s: string): string {
  return s.split("\n").map((l) => l.trim()).find(Boolean) ?? "";
}

/** Given `git worktree list --porcelain` output, return the path of the
 *  worktree bound to `branch`, or null. */
function findWorktreeForBranch(porcelain: string, branch: string): string | null {
  const wanted = `refs/heads/${branch}`;
  let current = "";
  for (const line of porcelain.split("\n")) {
    if (line.startsWith("worktree ")) {
      current = line.slice(9);
    } else if (line.startsWith("branch ")) {
      if (line.slice(7) === wanted) return current;
    }
  }
  return null;
}

/** What the local exclude must hide: each linked dependency dir, and the
 *  worktree dir itself (it lives inside the primary checkout, so without this
 *  every run shows up as untracked noise and `git add -A` could sweep it in). */
function excludeEntries(settings: WorktreeSettings): string[] {
  return [...settings.linkDirs, `/${settings.dir}/`];
}

/** Idempotently append each entry to the worktree's .git/info/exclude so
 *  a dependency symlink never shows as untracked. info/exclude resolves to the
 *  shared gitdir: local-only, never committed, never seen by a clone. */
function ensureExclude(ctx: WorktreeContext, worktreeRoot: string, entries: string[]): void {
  if (entries.length === 0) return;
  const excludePath = ctx.run("git", ["rev-parse", "--git-path", "info/exclude"], { cwd: worktreeRoot }).stdout.trim();
  if (!excludePath) return;
  const abs = excludePath.startsWith("/") ? excludePath : path.join(worktreeRoot, excludePath);
  const existing = ctx.readFile(abs);
  const have = new Set((existing ?? "").split(/\r?\n/).map((l) => l.trim()));
  const missing = entries.filter((d) => !have.has(d));
  if (missing.length === 0) return;
  const next = (existing ?? "") + (existing && !existing.endsWith("\n") ? "\n" : "") + missing.join("\n") + "\n";
  ctx.writeFile(abs, next);
}

/** Idempotently symlink each linked dir from the primary checkout into the
 *  worktree (replace a stale/real one). The symlink is the literal same files;
 *  a dir missing from the primary is skipped (nothing to link). */
function ensureLinkDirs(ctx: WorktreeContext, worktreeRoot: string, primaryRoot: string, linkDirs: string[]): void {
  for (const dir of linkDirs) {
    const linkPath = path.join(worktreeRoot, dir);
    const source = path.join(primaryRoot, dir);
    if (!ctx.exists(source)) continue;
    // `ln -s` overwrites neither a symlink nor a directory: remove the old
    // entry first (never the primary's, which is a distinct path).
    if (ctx.exists(linkPath)) {
      const rm = ctx.run("rm", ["-rf", linkPath]);
      if (rm.code !== 0) continue;
    }
    ctx.run("mkdir", ["-p", path.dirname(linkPath)]);
    ctx.run("ln", ["-s", source, linkPath]);
  }
}

export type { RunResult };
