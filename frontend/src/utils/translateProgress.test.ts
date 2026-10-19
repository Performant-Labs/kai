// issue #109 (Tester, RED): the pure request-lifecycle and progress rules, in the new module
// utils/translateProgress.ts (imported here by its export names). No Svelte, no wails runtime, no
// timers: the clock is an argument (nowMs) everywhere.
//
// State contract (F implements exactly this shape; the field names are the contract):
//   ProgressState = { engine: string; startedAt: number; lastActivityAt: number;
//                     chunk?: { done: number; total: number } | null }
//   startProgress(engine, nowMs)            -> a fresh state (startedAt = lastActivityAt = nowMs)
//   applyChunk(state, {done,total}, nowMs)  -> new state, lastActivityAt restarts at nowMs
//   progressLine(state, nowMs)              -> { kind: 'working' | 'stalled' | 'chunk', engine, elapsedMs, ... }
//   elapsedMs is always nowMs - startedAt (the frontend's own receive time, never started_at_ms).
import { describe, it, expect } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import {
  STILL_WAITING_AFTER_MS,
  newRequestID,
  requestSettled,
  progressLine,
  startProgress,
  applyChunk,
  formatClock,
} from './translateProgress.ts';

describe('newRequestID', () => {
  it('returns a non-empty string, unique across many calls', () => {
    const seen = new Set<string>();
    for (let i = 0; i < 2000; i++) {
      const id = newRequestID();
      expect(typeof id).toBe('string');
      expect(id.length).toBeGreaterThan(0);
      seen.add(id);
    }
    expect(seen.size).toBe(2000);
  });
  it('has no crypto dependency (works when globalThis.crypto is absent)', () => {
    const saved = Object.getOwnPropertyDescriptor(globalThis, 'crypto');
    Object.defineProperty(globalThis, 'crypto', { value: undefined, configurable: true });
    try {
      expect(newRequestID().length).toBeGreaterThan(0);
    } finally {
      if (saved) Object.defineProperty(globalThis, 'crypto', saved);
      else delete (globalThis as { crypto?: unknown }).crypto;
    }
    const src = readFileSync(resolve(__dirname, 'translateProgress.ts'), 'utf8');
    expect(src).not.toMatch(/\bcrypto\b/);
  });
});

describe('requestSettled (D9: what replaces the timer)', () => {
  it('is false while TranslateMulti has not returned (started is null)', () => {
    expect(requestSettled(null, {})).toBe(false);
    expect(requestSettled(null, { google: { engine: 'google', result: 'x' } })).toBe(false);
  });
  it('is false until every started engine has an entry in results', () => {
    expect(requestSettled(['apple', 'google'], { google: { engine: 'google', result: 'x' } })).toBe(
      false,
    );
  });
  it('is true once every started engine reported, whatever the outcome', () => {
    const results = {
      google: { engine: 'google', result: 'x' },
      apple: { engine: 'apple', result: '', error: 'boom' },
      deepl: { engine: 'deepl', result: '', cancelled: true },
    };
    expect(requestSettled(['apple', 'google', 'deepl'], results)).toBe(true);
  });
  it('an engine enabled in the list but not started does not block settling', () => {
    // Only the started list is consulted: results has exactly the started engines.
    expect(requestSettled(['google'], { google: { engine: 'google', result: 'x' } })).toBe(true);
  });
  it('count == 0 (an empty started list) settles at once', () => {
    expect(requestSettled([], {})).toBe(true);
  });
});

describe('progressLine', () => {
  it('working: names the engine and the elapsed time from the frontend receive time', () => {
    const s = startProgress('google', 1_000);
    const line = progressLine(s, 1_000 + 7_000);
    expect(line.kind).toBe('working');
    expect(line.engine).toBe('google');
    expect(line.elapsedMs).toBe(7_000);
  });
  it('the still-waiting threshold is 30 s', () => {
    expect(STILL_WAITING_AFTER_MS).toBe(30_000);
  });
  it('stays working just below the threshold and turns stalled at it', () => {
    const s = startProgress('openai', 0);
    expect(progressLine(s, STILL_WAITING_AFTER_MS - 1).kind).toBe('working');
    const stalled = progressLine(s, STILL_WAITING_AFTER_MS);
    expect(stalled.kind).toBe('stalled');
    expect(stalled.engine).toBe('openai');
    expect(stalled.elapsedMs).toBe(STILL_WAITING_AFTER_MS);
  });
  it('a chunk event switches to chunk progress with done/total', () => {
    const s = applyChunk(startProgress('openai', 0), { done: 2, total: 5 }, 10_000);
    const line = progressLine(s, 12_000);
    expect(line.kind).toBe('chunk');
    expect(line).toMatchObject({ engine: 'openai', done: 2, total: 5 });
    expect(line.elapsedMs).toBe(12_000);
  });
  it('the threshold clock restarts on each chunk event', () => {
    const s0 = startProgress('openai', 0);
    // 29 s in, a chunk arrives: 30 s after the START is not stalled, 30 s after the CHUNK is.
    const s1 = applyChunk(s0, { done: 1, total: 4 }, 29_000);
    expect(progressLine(s1, 30_000 + 5_000).kind).toBe('chunk');
    expect(progressLine(s1, 29_000 + STILL_WAITING_AFTER_MS - 1).kind).toBe('chunk');
    expect(progressLine(s1, 29_000 + STILL_WAITING_AFTER_MS).kind).toBe('stalled');
    const s2 = applyChunk(s1, { done: 2, total: 4 }, 50_000);
    expect(progressLine(s2, 50_000 + STILL_WAITING_AFTER_MS - 1).kind).toBe('chunk');
  });
  it('applyChunk does not mutate its input', () => {
    const s0 = startProgress('g', 0);
    const frozen = JSON.stringify(s0);
    applyChunk(s0, { done: 1, total: 2 }, 5);
    expect(JSON.stringify(s0)).toBe(frozen);
  });
});

describe('formatClock', () => {
  it('renders mm:ss, zero padded', () => {
    expect(formatClock(0)).toBe('00:00');
    expect(formatClock(7_400)).toBe('00:07');
    expect(formatClock(65_000)).toBe('01:05');
    expect(formatClock(600_000)).toBe('10:00');
  });
});

// Issue #10: time remaining on the progress line. Two sources, in order of trust: the parts
// already finished (a proportional ETA, which reflects whatever mode is actually running), and,
// until the first part lands, the backend's measured-rate estimate from the started event.
describe('time remaining (issue #10)', () => {
  it('working: the started estimate yields estimate - elapsed, and disappears once spent', () => {
    // Started at t=1000; the 20 s read is inside the 30 s stall threshold, so it is 'working'.
    const s = startProgress('apple', 1_000, 180_000);
    const line = progressLine(s, 21_000);
    expect(line.kind).toBe('working');
    expect(line.elapsedMs).toBe(20_000);
    expect(line.remainingMs).toBe(160_000);
    // The estimate is a hint, not a promise: once elapsed passes it, no time-left is shown.
    expect(progressLine(s, 181_000).remainingMs).toBeUndefined();
    expect(progressLine(s, 301_000).remainingMs).toBeUndefined();
  });
  it('working without an estimate (every engine but the System engine): no time-left', () => {
    const s = startProgress('google', 0);
    expect(progressLine(s, 10_000).remainingMs).toBeUndefined();
  });
  it('chunk: proportional from the parts done — 3 of 12 after 60 s leaves 180 s', () => {
    let s = startProgress('apple', 0, 999_000);
    s = applyChunk(s, { done: 3, total: 12 }, 55_000);
    const line = progressLine(s, 60_000);
    expect(line.kind).toBe('chunk');
    expect(line.remainingMs).toBe(180_000); // 60 s * (12-3)/3
  });
  it('chunk: the proportional ETA replaces the started estimate (it outranks it once parts land)', () => {
    let s = startProgress('apple', 0, 50_000);
    s = applyChunk(s, { done: 1, total: 2 }, 30_000);
    expect(progressLine(s, 30_000).remainingMs).toBe(30_000); // 30 s * (2-1)/1, not 50-30
  });
  it('chunk with every part done: no time-left', () => {
    let s = startProgress('apple', 0, 10_000);
    s = applyChunk(s, { done: 4, total: 4 }, 40_000);
    expect(progressLine(s, 41_000).remainingMs).toBeUndefined();
  });
  it('stalled keeps the time-left (a long System translation goes silent between parts; the ETA is what makes the wait legible)', () => {
    let s = startProgress('apple', 0, 180_000);
    s = applyChunk(s, { done: 1, total: 4 }, 10_000);
    const read = 10_000 + STILL_WAITING_AFTER_MS + 1;
    const line = progressLine(s, read);
    expect(line.kind).toBe('stalled');
    // Proportional from ALL the elapsed time (1 part in read ms): read * (4-1)/1.
    expect(line.remainingMs).toBe(read * 3);
  });
  it('applyChunk keeps the started estimate for the next state', () => {
    const s0 = startProgress('apple', 0, 90_000);
    const s1 = applyChunk(s0, { done: 1, total: 3 }, 10_000);
    expect(s1.estimatedMs).toBe(90_000);
  });
  it('a zero/absent estimate never shows a time-left', () => {
    const s = startProgress('apple', 0, 0);
    expect(progressLine(s, 5_000).remainingMs).toBeUndefined();
  });
});

describe('module hygiene', () => {
  it('is pure: no wails runtime, bindings, DOM or timers', () => {
    const src = readFileSync(resolve(__dirname, 'translateProgress.ts'), 'utf8');
    expect(src).not.toMatch(/@wailsio|bindings\/|localStorage|setTimeout|setInterval|document\./);
  });
});
