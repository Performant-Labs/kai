import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

// Issue #56: the back-translation switch persists in the real localStorage (jsdom), is OFF by
// default (it doubles the translation calls), and a stored value from an earlier run is read back.
const KEY = 'kai:translate:backTranslate';

beforeEach(() => {
  window.localStorage.removeItem(KEY);
  vi.resetModules();
});

describe('backTranslateOn (jsdom + real localStorage)', () => {
  it('is off when nothing was stored', async () => {
    const { backTranslateOn } = await import('./backTranslate.ts');
    expect(get(backTranslateOn)).toBe(false);
  });

  it('writes the switch to localStorage and reads it back in a fresh module', async () => {
    const first = await import('./backTranslate.ts');
    first.backTranslateOn.set(true);
    expect(window.localStorage.getItem(KEY)).toBe('true');
    vi.resetModules();
    const second = await import('./backTranslate.ts');
    expect(get(second.backTranslateOn)).toBe(true);
  });

  it('survives a corrupt stored value by falling back to off', async () => {
    window.localStorage.setItem(KEY, '{not json');
    const { backTranslateOn } = await import('./backTranslate.ts');
    expect(get(backTranslateOn)).toBe(false);
  });
});
