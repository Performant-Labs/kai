// issue #8 (Tester role, RED): the translate window's first-render primary-engine resolution
// rules (frontend mirror).
//
// Rules (aligned with the design doc §4's Go authoritative implementation PrimaryTranslateEngine;
// test d's divergence check guarantees both sides agree):
//
//   Resolution order last-used ?? primary ?? first-enabled:
//   1. lastUsed (the last-used engine persisted in localStorage), if among enabled translate engines -> it;
//   2. otherwise settings' default_engine if valid (in the list, kind=translate, enabled) -> it;
//   3. otherwise the first enabled translate engine (list id order);
//   4. no enabled translate engines -> ''.
//
// The "enabled translate" predicate = kind === 'translate' && enabled && supported
// (consistent with the Go-side GetAllEngines shape; see design §3).
//
// RED note: resolvePrimaryEngine does not exist yet (the design §4 frontend-mirror module), so
// this file fails to resolve on import. This is a "missing behavior" RED, not an environment or
// typo problem.
// The engine list uses the same dataset as the Go side's (b)/(c); the Go-side tests must resolve
// to the same results as this file.
import { describe, it, expect, beforeEach } from 'vitest';
import { resolvePrimaryEngine } from './resolvePrimaryEngine.ts';

const LAST_USED_KEY = 'kai:translate:lastEngine';

// Same engine list as the Go side's (b)/(c) (configstore id order: google=1, deepl=2).
const ENGINES = [
  {
    id: 1,
    value: 'google',
    name: 'google',
    kind: 'translate',
    enabled: true,
    supported: true,
    builtin: false,
  },
  {
    id: 2,
    value: 'deepl',
    name: 'deepl',
    kind: 'translate',
    enabled: true,
    supported: true,
    builtin: false,
  },
];

// The list after disabling google (the mid-state of the Go side's test c).
const ENGINES_GOOGLE_DISABLED = ENGINES.map((e) =>
  e.value === 'google' ? { ...e, enabled: false } : e,
);

const ocrItem = {
  id: 3,
  value: 'tesseract',
  name: 'tesseract',
  kind: 'ocr',
  enabled: true,
  supported: true,
  builtin: false,
};

beforeEach(() => {
  window.localStorage.removeItem(LAST_USED_KEY);
});

describe('resolvePrimaryEngine (last-used ?? primary ?? first-enabled, real localStorage)', () => {
  it('last-used wins when it is among enabled translate engines', () => {
    // Write to localStorage for real (same shape as the persisted store's write path).
    window.localStorage.setItem(LAST_USED_KEY, JSON.stringify('deepl'));
    expect(resolvePrimaryEngine(LAST_USED_KEY, 'google', ENGINES)).toBe('deepl');
  });

  it('falls back to primary when the last-used engine is disabled', () => {
    window.localStorage.setItem(LAST_USED_KEY, JSON.stringify('google'));
    // google is disabled (same list state as the Go side's test c); primary is still deepl.
    expect(resolvePrimaryEngine(LAST_USED_KEY, 'deepl', ENGINES_GOOGLE_DISABLED)).toBe('deepl');
  });

  it('falls back to the first enabled translate engine when primary is invalid (not in the list)', () => {
    expect(resolvePrimaryEngine(LAST_USED_KEY, 'nosuchengine', ENGINES)).toBe('google');
  });

  it('falls back to the first enabled translate engine when last-used and primary are both unset', () => {
    expect(resolvePrimaryEngine(LAST_USED_KEY, '', ENGINES)).toBe('google');
  });

  it('an ocr engine is not a valid primary; falls back to first-enabled', () => {
    const withOcr = [...ENGINES, ocrItem];
    expect(resolvePrimaryEngine(LAST_USED_KEY, 'tesseract', withOcr)).toBe('google');
  });

  it('returns an empty string when no translate engine is enabled', () => {
    const noneEnabled = ENGINES.map((e) => ({ ...e, enabled: false }));
    expect(resolvePrimaryEngine(LAST_USED_KEY, '', noneEnabled)).toBe('');
  });

  it('reads real localStorage: an empty-string last-used does not override primary', () => {
    // Read back for real (simulating the persisted store's initialize-from-localStorage path).
    window.localStorage.setItem(LAST_USED_KEY, JSON.stringify(''));
    expect(resolvePrimaryEngine(LAST_USED_KEY, 'deepl', ENGINES)).toBe('deepl');
  });
});
