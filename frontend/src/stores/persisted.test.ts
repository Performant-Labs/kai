// issue #7 (Tester role, RED): example frontend vitest suite.
//
// Why the persisted store was chosen (rather than lang.ts's lang-name fallback):
//  - lang.ts re-exports the wails-generated enum `Language` from
//    @bindings/.../model/models.ts as TRANSLATE_LANG / ALL_TRANSLATE_LANGS /
//    TARGET_TRANSLATE_LANGS; the module's identity depends on wails-generated
//    artifacts, and with no wails runtime in the test environment the import fails
//    to resolve.
//  - persisted.ts only imports svelte/store — pure logic plus real localStorage,
//    exactly matching the issue's stipulation of "frontend logic in jsdom with
//    real localStorage".
//
// Constraints:
//  - jsdom provides real window.localStorage, not mocked;
//  - no component-testing framework;
//  - each test case uses a unique key for isolation, no interference between cases.
import { describe, it, expect, beforeEach } from 'vitest';
import { persisted, pinKey } from './persisted.ts';

const KEY = 'kai:test:persisted';

beforeEach(() => {
  // Clean up localStorage written by the previous case so every case starts from an empty state.
  window.localStorage.removeItem(KEY);
  window.localStorage.removeItem(pinKey('translate'));
});

describe('persisted store (jsdom + real localStorage)', () => {
  it('falls back to initial when localStorage has no value', () => {
    const s = persisted<number>(KEY, 42);
    let got = 0;
    const unsub = s.subscribe((v) => (got = v));
    expect(got).toBe(42);
    unsub();
  });

  it('reads back the last persisted value from localStorage on init', () => {
    // Simulate a write from a "previous run"
    window.localStorage.setItem(KEY, JSON.stringify(7));
    const s = persisted<number>(KEY, 42);
    let got = 0;
    const unsub = s.subscribe((v) => (got = v));
    expect(got).toBe(7);
    unsub();
  });

  it('syncs localStorage on set; a new store reads the value back', () => {
    const s1 = persisted<number>(KEY, 1);
    s1.set(99);
    // A new store re-initializes with the same key -> should read 99, not initial 1
    const s2 = persisted<number>(KEY, 1);
    let got = 0;
    const unsub = s2.subscribe((v) => (got = v));
    expect(got).toBe(99);
    expect(JSON.parse(window.localStorage.getItem(KEY)!)).toBe(99);
    unsub();
  });

  it('falls back to initial when the localStorage value is corrupt (invalid JSON)', () => {
    window.localStorage.setItem(KEY, '{not-json');
    const s = persisted<number>(KEY, 5);
    let got = 0;
    const unsub = s.subscribe((v) => (got = v));
    expect(got).toBe(5);
    unsub();
  });

  it('pinKey generates a distinct key per window name', () => {
    expect(pinKey('translate')).toBe('kai:translate:pinned');
    expect(pinKey('settings')).toBe('kai:settings:pinned');
    expect(pinKey('translate')).not.toBe(pinKey('settings'));
  });
});
