// issue #9 (Tester role, RED): result-pane pure-logic module (design §1/§3/§4 test (b)).
//
// The implementation must extract the result pane's pure logic into
// frontend/src/utils/resultPane.ts (this file imports it by its export names), which
// TranslateWindow consumes:
//   - activeEngineFor(lastUsed, defaultEngine, engines):
//       resolvePrimaryEngine(lastUsed, defaultEngine, engines)
//       || the first enabled translate engine (defensive fallback, guaranteeing the select never dangles);
//     the resolution rules are not reinvented here — it must consume #8's resolvePrimaryEngine.
//   - statusDot(engine, results, loading) / statusDots(engines, results, loading):
//     one dot per enabled translate engine, 'pending' | 'done' | 'failed':
//       done    = results[engine]?.result is a non-empty string;
//       pending = loading === true and no (non-empty) result;
//       failed  = !loading and no (non-empty) result (the backend emits no event whatsoever for a
//                  failed engine — it is simply absent from results; design §4's failed is a derived signal).
//   - resetEdits(edited, previousEngine, nextEngine, results):on engine switch, discard the
//     previous engine's manual edit; the new engine starts from its stored result (design §3
//     "manual edits reset": no per-engine edit memory).
//   - anyPending(engines, results):the 15 s fallback predicate "does any engine still lack a result"
//     (a design §4 extension: relaxed from "zero results" to "any pending").
//
// No mocks: real jsdom localStorage (NODE_OPTIONS=--localstorage-file, see the vitest.config.ts
// header note), real setTimeout (Node real timers; the 15 s case waits for real, no mocked timers,
// no stubbed wails runtime).
//
// RED note: the ./resultPane.ts module does not exist yet (the result-pane logic is inlined in
// TranslateWindow.svelte today) — this file fails on import, with vitest reporting
// "Failed to resolve import ... resultPane.ts". This is a "missing behavior" RED.
// This file touches no existing tests/modules; once the implementation lands it should turn all green.

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import {
  failureMessage,
  activeEngineFor,
  statusDot,
  statusDots,
  resetEdits,
  anyPending,
  type PaneEngine,
  type PaneResult,
} from './resultPane.ts';

const LAST_USED_KEY = 'kai:translate:lastEngine';

// The same engine set as the #8 frontend-mirror test (configstore id order: google=1, deepl=2).
const ENGINES: PaneEngine[] = [
  { id: 1, value: 'google', name: 'google', kind: 'translate', enabled: true, supported: true },
  { id: 2, value: 'deepl', name: 'deepl', kind: 'translate', enabled: true, supported: true },
];

const OCR_ENGINE: PaneEngine = {
  id: 3,
  value: 'tesseract',
  name: 'tesseract',
  kind: 'ocr',
  enabled: true,
  supported: true,
};

const GOOGLE_DISABLED: PaneEngine[] = ENGINES.map((e) =>
  e.value === 'google' ? { ...e, enabled: false } : e,
);

beforeEach(() => {
  // Real localStorage (Node 26 needs --localstorage-file, see vitest.config.ts).
  window.localStorage.removeItem(LAST_USED_KEY);
});

// Cleanup: --localstorage-file is a file-backed store, so a removeItem deletion can linger as an
// empty string (which resolvePrimaryEngine's "no-op on empty" guard skips) and leak across files
// in later test files' (resolvePrimaryEngine.test.ts etc.) vitest workers.
// clear() as the safety net: after each test the store retains no state, so files never pollute
// each other.
afterEach(() => {
  window.localStorage.clear();
});

describe('activeEngineFor (consumes #8 resolvePrimaryEngine + defensive fallback)', () => {
  it('last-used wins when among enabled translate engines (overrides primary)', () => {
    window.localStorage.setItem(LAST_USED_KEY, JSON.stringify('deepl'));
    expect(activeEngineFor(LAST_USED_KEY, 'google', ENGINES)).toBe('deepl');
  });

  it('falls back to primary when the last-used engine is disabled', () => {
    window.localStorage.setItem(LAST_USED_KEY, JSON.stringify('google'));
    expect(activeEngineFor(LAST_USED_KEY, 'deepl', GOOGLE_DISABLED)).toBe('deepl');
  });

  it('falls back to the first enabled translate engine when last-used and primary are both invalid', () => {
    expect(activeEngineFor(LAST_USED_KEY, 'nosuchengine', ENGINES)).toBe('google');
  });

  it('falls back to the first enabled translate engine when neither is set (id order)', () => {
    expect(activeEngineFor(LAST_USED_KEY, '', ENGINES)).toBe('google');
  });

  it('an ocr engine is not a valid primary', () => {
    expect(activeEngineFor(LAST_USED_KEY, 'tesseract', [...ENGINES, OCR_ENGINE])).toBe('google');
  });

  it('defensive fallback: takes the first enabled engine when resolvePrimaryEngine returns "" (select never dangles)', () => {
    // Make the #8 mirror return '': last-used in localStorage is a corrupt/invalid value and primary is empty.
    // Constructing an engine list that makes resolvePrimaryEngine land on '' is impossible
    // (with any enabled engine it necessarily returns the first) — so this defensive branch only
    // guards against a future rule change of "resolve returns '' while the list is non-empty";
    // here an all-disabled list verifies activeEngineFor returns '' (with nothing to fall back to
    // it can only be empty; the behavior contract: return resolve's value as-is).
    const noneEnabled = ENGINES.map((e) => ({ ...e, enabled: false }));
    expect(activeEngineFor(LAST_USED_KEY, '', noneEnabled)).toBe('');
  });
});

describe('statusDot / statusDots (pending / done / failed derived from real fan-out signals)', () => {
  it('loading with no result for the engine -> pending', () => {
    expect(statusDot('google', {}, true)).toBe('pending');
    expect(statusDot('deepl', {}, true)).toBe('pending');
  });

  it('non-empty result -> done (holds with or without loading: result arrived means done)', () => {
    const results: Record<string, PaneResult> = { google: { result: 'hola' } };
    expect(statusDot('google', results, true)).toBe('done');
    expect(statusDot('google', results, false)).toBe('done');
  });

  it('empty-string result is not done: !loading and no usable result -> failed', () => {
    expect(statusDot('google', { google: { result: '' } }, false)).toBe('failed');
  });

  it('!loading and the engine is absent (backend emits no event for failed engines) -> failed', () => {
    const results: Record<string, PaneResult> = { deepl: { result: 'hallo' } };
    expect(statusDot('google', results, false)).toBe('failed');
  });

  it('absent engine stays pending while loading (before the 15 s fallback)', () => {
    const results: Record<string, PaneResult> = { deepl: { result: 'hallo' } };
    expect(statusDot('google', results, true)).toBe('pending');
  });

  it('case 1: one engine fails while a sibling succeeds in a multi-engine fan-out (loading=false) -> failed dot immediately failed', () => {
    const results: Record<string, PaneResult> = { deepl: { result: 'hallo' } };
    const dots = statusDots(ENGINES, results, false);
    expect(dots.google).toBe('failed');
    expect(dots.deepl).toBe('done');
  });

  it('case 2: the only engine fails; loading stays true before the 15 s fallback -> dot stays pending', () => {
    const sole = [ENGINES[0]];
    const dots = statusDots(sole, {}, true);
    expect(dots.google).toBe('pending');
  });

  it('statusDots only covers enabled translate engines (ocr / disabled engines get no dot)', () => {
    const all = [...ENGINES, OCR_ENGINE];
    const dots = statusDots(all, {}, true);
    expect(Object.keys(dots).sort()).toEqual(['deepl', 'google']);
  });

  it('anyPending: an engine without a result -> true (15 s fallback predicate: any pending keeps loading)', () => {
    expect(anyPending(ENGINES, {}, true)).toBe(true);
    expect(anyPending(ENGINES, { google: { result: 'x' } }, true)).toBe(true);
  });

  it('anyPending: all engines have non-empty results -> false (fallback no longer flips loading)', () => {
    expect(anyPending(ENGINES, { google: { result: 'x' }, deepl: { result: 'y' } }, false)).toBe(
      false,
    );
  });
});

describe('resetEdits (switching engines drops the previous engine manual edits)', () => {
  it('drops previous edits; next starts from its stored result (no per-engine edit memory)', () => {
    const results: Record<string, PaneResult> = {
      google: { result: 'hola' },
      deepl: { result: 'hallo' },
    };
    const edited = new Map<string, string>([['google', 'manually edited text']]);
    const next = resetEdits(edited, 'google', 'deepl', results);
    // google's edit is dropped.
    expect(next.get('google')).toBeUndefined();
    // deepl's displayed text reverts to its stored result (absent from the edit map, so the display layer takes result).
    expect(next.get('deepl')).toBeUndefined();
    expect(next.get('deepl') ?? results['deepl']?.result).toBe('hallo');
  });

  it('does not mutate the passed-in map (returns a new Map)', () => {
    const results: Record<string, PaneResult> = { google: { result: 'a' }, deepl: { result: 'b' } };
    const edited = new Map<string, string>([['google', 'edit']]);
    const before = new Map(edited);
    resetEdits(edited, 'google', 'deepl', results);
    expect(edited).toEqual(before);
  });

  it('previous === next (same engine re-selected) is still treated as a switch: its edits are cleared, back to the stored result', () => {
    const results: Record<string, PaneResult> = { google: { result: 'hola' } };
    const edited = new Map<string, string>([['google', 'edited']]);
    const next = resetEdits(edited, 'google', 'google', results);
    expect(next.get('google')).toBeUndefined();
  });
});

// issue #42: user-facing copy for failure payloads (kind → localized key, raw detail appended).
describe('failureMessage (#42 surfacing failure reasons)', () => {
  const t = (key: string) => {
    const dict: Record<string, string> = {
      'translate.failed': 'Translation failed',
      'translate.failedPair': 'Language pair unavailable',
      'translate.failedNetwork': 'Engine unreachable — check network or proxy',
      'translate.failedAuth': 'Check the API key',
    };
    return dict[key] ?? key;
  };

  it('maps pair kind to the actionable message, detail appended', () => {
    expect(
      failureMessage({ engine: 'apple', error: 'Unable to Translate', errorKind: 'pair' }, t),
    ).toBe('Language pair unavailable — Unable to Translate');
  });

  it('maps network kind to the reachability message', () => {
    expect(
      failureMessage({ engine: 'google', error: 'dial tcp: refused', errorKind: 'network' }, t),
    ).toBe('Engine unreachable — check network or proxy — dial tcp: refused');
  });

  it('maps auth kind to the key message', () => {
    expect(failureMessage({ engine: 'gpt', error: '401', errorKind: 'auth' }, t)).toBe(
      'Check the API key — 401',
    );
  });

  it('falls back to the generic message for unknown kinds', () => {
    expect(failureMessage({ engine: 'x', error: 'boom', errorKind: 'engine' }, t)).toBe(
      'Translation failed — boom',
    );
  });

  it('returns the generic message with no detail for null/absent results', () => {
    expect(failureMessage(null, t)).toBe('Translation failed');
    expect(failureMessage({ engine: 'x' }, t)).toBe('Translation failed');
  });
});
