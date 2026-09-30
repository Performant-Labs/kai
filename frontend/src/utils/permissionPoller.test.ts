import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPoller } from './permissionPoller';

// Issue #14: one timer that re-runs a check every 3 s, never two checks at once.

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

describe('createPoller', () => {
  it('runs the check every 3 seconds after start, not before', async () => {
    const run = vi.fn(async () => {});
    const p = createPoller(run);
    p.start();
    await vi.advanceTimersByTimeAsync(2999);
    expect(run).toHaveBeenCalledTimes(0);
    await vi.advanceTimersByTimeAsync(1);
    expect(run).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(3000);
    expect(run).toHaveBeenCalledTimes(2);
    p.stop();
  });

  it('start(true) checks at once, then every 3 seconds', async () => {
    const run = vi.fn(async () => {});
    const p = createPoller(run);
    p.start(true);
    await vi.advanceTimersByTimeAsync(0);
    expect(run).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(3000);
    expect(run).toHaveBeenCalledTimes(2);
    p.stop();
  });

  it('stop cancels the timer, and a check in flight does not schedule another', async () => {
    let release: () => void = () => {};
    const run = vi.fn(() => new Promise<void>((r) => (release = r)));
    const p = createPoller(run);
    p.start(true);
    await vi.advanceTimersByTimeAsync(0);
    p.stop();
    release();
    await vi.advanceTimersByTimeAsync(20000);
    expect(run).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(0);
  });

  it('never overlaps: a slow check delays the next one until it finishes', async () => {
    let release: () => void = () => {};
    const run = vi.fn(() => new Promise<void>((r) => (release = r)));
    const p = createPoller(run);
    p.start();
    await vi.advanceTimersByTimeAsync(3000);
    expect(run).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(12000); // 4 more intervals while still running
    expect(run).toHaveBeenCalledTimes(1);
    release();
    await vi.advanceTimersByTimeAsync(2999);
    expect(run).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(1);
    expect(run).toHaveBeenCalledTimes(2);
    p.stop();
  });

  it('start twice keeps one timer', async () => {
    const run = vi.fn(async () => {});
    const p = createPoller(run);
    p.start();
    p.start();
    expect(vi.getTimerCount()).toBe(1);
    await vi.advanceTimersByTimeAsync(3000);
    expect(run).toHaveBeenCalledTimes(1);
    p.stop();
  });

  it('a check that throws does not end the polling', async () => {
    const run = vi.fn(async () => {
      throw new Error('boom');
    });
    const p = createPoller(run);
    p.start();
    await vi.advanceTimersByTimeAsync(3000);
    await vi.advanceTimersByTimeAsync(3000);
    expect(run).toHaveBeenCalledTimes(2);
    p.stop();
  });
});

describe('createPoller restart', () => {
  it('a restart while a check is still in flight does not start a second check', async () => {
    const releases: (() => void)[] = [];
    const run = vi.fn(() => new Promise<void>((r) => releases.push(r)));
    const p = createPoller(run);
    p.start(true);
    await vi.advanceTimersByTimeAsync(0);
    p.stop();
    p.start(true); // e.g. hidden then shown again during a slow check
    await vi.advanceTimersByTimeAsync(0);
    expect(run).toHaveBeenCalledTimes(1);
    releases[0]();
    await vi.advanceTimersByTimeAsync(3000);
    expect(run).toHaveBeenCalledTimes(2);
    expect(vi.getTimerCount()).toBe(0); // second check still in flight, nothing else scheduled
    releases[1]();
    await vi.advanceTimersByTimeAsync(0);
    p.stop();
  });
});
