// Issue #109: the pure rules of a translation request that has no time limit.
//
// Nothing in the frontend decides by a timer that a translation is over any more. A request is open
// from the moment the window sends it until every engine the backend started has reported (a
// translation, a failure or a cancel), or the call itself failed; the user can end it earlier with
// Cancel. This module holds what that needs and nothing else: naming a request, deciding when it
// has settled (requestSettled), and turning what is known about a running engine into the one
// progress line the window shows (progressLine).
//
// Everything here is a pure function of its arguments. The clock is an argument (nowMs) and is the
// frontend's own: an engine is timed from the moment the window received its started event, never
// from the backend's started_at_ms, so a skew between the two clocks cannot show. The 1 s tick
// that re-reads the clock, and the i18n copy the line is rendered with, live in the components.

import { allReported, type PaneResult } from './resultPane.ts';

/**
 * How long an engine may stay silent before its progress line says "still waiting". It is not a
 * limit: nothing is cancelled or failed when it passes, the line only changes wording.
 */
export const STILL_WAITING_AFTER_MS = 30_000;

let requestCounter = 0;

/**
 * A new request id (a plain string the backend echoes on every event of the request). Unique within
 * and across windows without any platform random source: the time, a counter that never repeats in
 * this page, and a random suffix. The prefix keeps it apart from the ids the backend generates for
 * callers that send none.
 */
export function newRequestID(): string {
  requestCounter += 1;
  const random = Math.random().toString(36).slice(2, 10);
  return `req-${Date.now().toString(36)}-${requestCounter.toString(36)}-${random}`;
}

/**
 * Whether a request has settled: the backend call has returned (started is the list of engines it
 * says it started; null while the call is still pending) and every one of those engines has an
 * entry in results, whatever its outcome. An empty list settles at once (nothing was started).
 * Only the started list counts, never the window's own idea of which engines are enabled, so a
 * registry and a config that disagree cannot leave a request open forever.
 */
export function requestSettled(
  started: string[] | null,
  results: Record<string, PaneResult>,
): boolean {
  return started !== null && allReported(started, results);
}

/** What the window knows about one running engine. */
export type ProgressState = {
  engine: string;
  /** When the started event was received (the frontend's own clock, ms). */
  startedAt: number;
  /** When something from the engine was last received (its started event or a chunk), ms. */
  lastActivityAt: number;
  /** The last chunk progress, when the engine reports any. */
  chunk?: { done: number; total: number } | null;
};

/** The engine has just started: elapsed time and the silence clock both begin now. */
export function startProgress(engine: string, nowMs: number): ProgressState {
  return { engine, startedAt: nowMs, lastActivityAt: nowMs, chunk: null };
}

/**
 * A chunk event arrived (done of total parts finished): the state now carries it and the silence
 * clock restarts. Returns a new state; the input is not changed.
 */
export function applyChunk(
  state: ProgressState,
  chunk: { done: number; total: number },
  nowMs: number,
): ProgressState {
  return { ...state, chunk: { done: chunk.done, total: chunk.total }, lastActivityAt: nowMs };
}

/** The progress line's facts; the component renders them with its own copy. */
export interface ProgressLine {
  /**
   * working: the engine is being worked on; stalled: nothing was received from it for
   * STILL_WAITING_AFTER_MS; chunk: it reports parts (done of total).
   */
  kind: 'working' | 'stalled' | 'chunk';
  engine: string;
  /** Time since the started event was received, ms. */
  elapsedMs: number;
  /** Only for kind 'chunk'. */
  done?: number;
  total?: number;
}

/**
 * The single place the progress rules live: silence for STILL_WAITING_AFTER_MS or more is
 * 'stalled' (measured from the last thing received, so each chunk restarts it), otherwise 'chunk'
 * when the engine reports parts, otherwise 'working'.
 */
export function progressLine(state: ProgressState, nowMs: number): ProgressLine {
  const elapsedMs = Math.max(0, nowMs - state.startedAt);
  if (nowMs - state.lastActivityAt >= STILL_WAITING_AFTER_MS) {
    return { kind: 'stalled', engine: state.engine, elapsedMs };
  }
  if (state.chunk) {
    return {
      kind: 'chunk',
      engine: state.engine,
      elapsedMs,
      done: state.chunk.done,
      total: state.chunk.total,
    };
  }
  return { kind: 'working', engine: state.engine, elapsedMs };
}

/** Whole minutes and the remaining whole seconds of a duration in ms (negative counts as 0). */
export function splitElapsed(ms: number): { minutes: number; seconds: number } {
  const total = Math.floor(Math.max(0, ms) / 1000);
  return { minutes: Math.floor(total / 60), seconds: total % 60 };
}

/** A duration in ms as a zero-padded mm:ss clock. */
export function formatClock(ms: number): string {
  const { minutes, seconds } = splitElapsed(ms);
  return `${String(minutes).padStart(2, '0')}:${String(seconds).padStart(2, '0')}`;
}
