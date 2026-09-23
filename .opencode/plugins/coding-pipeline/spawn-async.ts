// coding-pipeline plugin — non-blocking process runner.
//
// The test suite and the outside-model review can each run for minutes. In OpenCode
// the plugin shares a process with the UI, so a synchronous spawn would freeze the
// session for the whole run. This runs the command asynchronously, keeps the tail of
// each stream, and on timeout kills the WHOLE process group (a shell wrapper such as
// `sh -c "build && go test"` must not leave its children running).
//
// A spawn failure or a timeout is reported as code -1 (mapped to an ERROR verdict by
// the callers), never as a test failure.

import { spawn } from "node:child_process";

import type { RunResult } from "./types.ts";

const MAX_CAPTURE_BYTES = 2_000_000;

export function spawnAsync(
  command: string,
  args: string[],
  options: { cwd?: string; timeoutMs?: number } = {},
): Promise<RunResult> {
  return new Promise((resolve) => {
    let stdout = "";
    let stderr = "";
    let settled = false;
    let timedOut = false;
    let timer: ReturnType<typeof setTimeout> | undefined;

    const finish = (result: RunResult): void => {
      if (settled) return;
      settled = true;
      if (timer) clearTimeout(timer);
      resolve(result);
    };

    let child: ReturnType<typeof spawn>;
    try {
      child = spawn(command, args, {
        cwd: options.cwd,
        stdio: ["ignore", "pipe", "pipe"],
        env: { ...process.env, FORCE_COLOR: "0", NO_COLOR: "1" },
        // Own process group, so a timeout can kill the command and everything it started.
        detached: process.platform !== "win32",
      });
    } catch (err) {
      return finish({ code: -1, stdout: "", stderr: err instanceof Error ? err.message : String(err) });
    }

    const keep = (current: string, chunk: Buffer): string => (current + chunk.toString("utf8")).slice(-MAX_CAPTURE_BYTES);
    child.stdout?.on("data", (d: Buffer) => {
      stdout = keep(stdout, d);
    });
    child.stderr?.on("data", (d: Buffer) => {
      stderr = keep(stderr, d);
    });

    if (options.timeoutMs && options.timeoutMs > 0) {
      timer = setTimeout(() => {
        timedOut = true;
        try {
          if (child.pid !== undefined && process.platform !== "win32") process.kill(-child.pid, "SIGKILL");
          else child.kill("SIGKILL");
        } catch {
          child.kill("SIGKILL");
        }
      }, options.timeoutMs);
    }

    child.on("error", (err) => finish({ code: -1, stdout, stderr: stderr || err.message }));
    child.on("close", (code, signal) => {
      const note = timedOut ? `\n[killed after ${options.timeoutMs}ms]` : signal ? `\n[terminated by ${signal}]` : "";
      finish({ code: timedOut ? -1 : (code ?? -1), stdout, stderr: stderr + note });
    });
  });
}
