// A single repeating check that never overlaps itself (issue #14).
//
// The next run is scheduled only after the current one has finished, so a check that takes longer
// than the interval is waited for, not stacked: one timer exists at any moment, and at most one
// check is in flight. A check that throws is swallowed here (the caller owns its own error
// handling) so one bad read never ends the polling.

export const PERMISSION_POLL_MS = 3000;

export interface Poller {
  /** Start polling; with `immediate` the first check runs now instead of after one interval. No-op when already started. */
  start(immediate?: boolean): void;
  /** Stop polling: cancels the timer; a check in flight finishes but schedules nothing after it. */
  stop(): void;
  readonly active: boolean;
}

export function createPoller(run: () => Promise<unknown>, intervalMs = PERMISSION_POLL_MS): Poller {
  let active = false;
  let running = false; // a check is in flight (possibly begun before a stop)
  let timer: ReturnType<typeof setTimeout> | null = null;

  function schedule() {
    timer = setTimeout(tick, intervalMs);
  }

  async function tick() {
    timer = null;
    running = true;
    try {
      await run();
    } catch {
      /* the check reports its own failures */
    }
    running = false;
    if (active) schedule();
  }

  return {
    start(immediate = false) {
      if (active) return;
      active = true;
      // A check still in flight from before a stop schedules the next one when it finishes.
      if (running) return;
      if (immediate) void tick();
      else schedule();
    },
    stop() {
      active = false;
      if (timer !== null) clearTimeout(timer);
      timer = null;
    },
    get active() {
      return active;
    },
  };
}
