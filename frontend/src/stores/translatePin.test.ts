import { get } from 'svelte/store';
import { beforeEach, describe, expect, it } from 'vitest';
import { createTranslatePinStore, TRANSLATE_PIN_KEY } from './translatePin';
import { effectivePin } from '../utils/pinDefault';

// Issue #113 / AC6 (A Findings 3 + 4, binding): the translate window's 🧷 state moves to a
// translate-specific VERSIONED key (kai:translate:pinned:v2) in an explicit-write-only store —
// closing the #113 defect where persisted's subscribe auto-writes the initial pin and
// initialPin then honors that pollution as a "choice". The contract:
//
//   - reads the versioned key ONLY (the old kai:translate:pinned key is ignored entirely —
//     pinKey() itself is untouched, so the screenshot window's pin is unaffected);
//   - a subscribe (or a mere read) writes NOTHING;
//   - apply(v) is the runtime-only update the mount init / broadcast / re-pin resolutions use —
//     it never reaches localStorage (the mount-path no-write test, A Finding 4);
//   - set(v) is the ONE explicit path that persists, and only under the versioned key.
//
// The last two cases wire the store to the effectivePin decision (R2-1): the #113 owner
// migration (old key true, v2 absent, setting off -> starts NOT on top, resolution writes
// nothing) and the AC8 re-pin with the versioned key present (stored choice survives the
// re-pin; no write).

const OLD_KEY = 'kai:translate:pinned'; // pinKey('translate') — deliberately never read or written here again

beforeEach(() => {
  localStorage.clear();
});

describe('createTranslatePinStore (the explicit-choice-only pin store, #113)', () => {
  it('reads the versioned key only — an old-key pin is ignored, not honored', () => {
    localStorage.setItem(OLD_KEY, JSON.stringify(true));
    expect(get(createTranslatePinStore())).toBe(false); // the old key's value is pollution, not a choice
  });

  it('reads a boolean from the versioned key, both ways', () => {
    localStorage.setItem(TRANSLATE_PIN_KEY, JSON.stringify(true));
    expect(get(createTranslatePinStore())).toBe(true);
    localStorage.setItem(TRANSLATE_PIN_KEY, JSON.stringify(false));
    expect(get(createTranslatePinStore())).toBe(false);
  });

  it('a corrupt versioned value reads as the initial false and is not rewritten', () => {
    localStorage.setItem(TRANSLATE_PIN_KEY, '{oops');
    const s = createTranslatePinStore();
    expect(get(s)).toBe(false);
    expect(localStorage.getItem(TRANSLATE_PIN_KEY)).toBe('{oops'); // byte-identical: no "repair" write
  });

  it('creating and subscribing writes nothing (no subscribe auto-write — the #113 defect must stay dead)', () => {
    localStorage.setItem(TRANSLATE_PIN_KEY, JSON.stringify(true));
    const s = createTranslatePinStore();
    const unsub = s.subscribe(() => {});
    unsub();
    expect(localStorage.getItem(TRANSLATE_PIN_KEY)).toBe(JSON.stringify(true)); // byte-identical, no rewrite
  });

  it('a mount with nothing stored leaves localStorage EMPTY (A Finding 4)', () => {
    const s = createTranslatePinStore();
    const unsub = s.subscribe(() => {});
    s.apply(false); // the mount-init resolution — runtime only
    unsub();
    expect(localStorage.getItem(TRANSLATE_PIN_KEY)).toBeNull();
    expect(localStorage.getItem(OLD_KEY)).toBeNull();
  });

  it('apply() changes the live value without ever writing', () => {
    const s = createTranslatePinStore();
    let v: boolean | undefined;
    const unsub = s.subscribe((x) => (v = x));
    s.apply(true);
    expect(v).toBe(true);
    expect(localStorage.getItem(TRANSLATE_PIN_KEY)).toBeNull();
    unsub();
  });

  it('set() is the explicit 🧷 write: it persists under the versioned key only', () => {
    const s = createTranslatePinStore();
    s.set(true);
    expect(localStorage.getItem(TRANSLATE_PIN_KEY)).toBe(JSON.stringify(true));
    expect(localStorage.getItem(OLD_KEY)).toBeNull(); // the old key is never written again
    s.set(false);
    expect(localStorage.getItem(TRANSLATE_PIN_KEY)).toBe(JSON.stringify(false));
  });

  it('#113 owner migration, end to end: old key true, v2 absent, setting off → starts NOT on top, and the resolution writes nothing', () => {
    localStorage.setItem(OLD_KEY, JSON.stringify(true)); // the pre-#69 pollution
    const rawV2 = localStorage.getItem(TRANSLATE_PIN_KEY); // null — the only key that counts
    const s = createTranslatePinStore();
    s.apply(effectivePin(rawV2, false, false, false)); // the mount init resolves through the decision
    expect(get(s)).toBe(false); // the old key's true must NOT pin the window
    expect(localStorage.getItem(TRANSLATE_PIN_KEY)).toBeNull(); // the resolution never reached storage
    expect(localStorage.getItem(OLD_KEY)).toBe(JSON.stringify(true)); // the old key is left as-is
  });

  it('AC8: with a versioned choice present, the Settings-close re-pin resolves through effectivePin and does not revert it — and does not write', () => {
    localStorage.setItem(TRANSLATE_PIN_KEY, JSON.stringify(true)); // a genuine stored choice (W-2: versioned key present)
    const rawV2 = localStorage.getItem(TRANSLATE_PIN_KEY);
    const s = createTranslatePinStore();
    let v: boolean | undefined;
    const unsub = s.subscribe((x) => (v = x));
    // mount: stored true wins over setting off
    s.apply(effectivePin(rawV2, false, false, false));
    expect(v).toBe(true);
    // Settings closes (the re-pin): same decision, same inputs -> still true (not the raw
    // mount-time store value, not the setting)
    s.apply(effectivePin(rawV2, false, false, false));
    expect(v).toBe(true);
    expect(localStorage.getItem(TRANSLATE_PIN_KEY)).toBe(JSON.stringify(true)); // no write on either apply
    unsub();
  });
});
