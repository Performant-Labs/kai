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
  paneState,
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
    expect(statusDot('google', {}, true, true)).toBe('pending');
    expect(statusDot('deepl', {}, true, true)).toBe('pending');
  });

  it('non-empty result -> done (holds with or without loading: result arrived means done)', () => {
    const results: Record<string, PaneResult> = { google: { result: 'hola' } };
    expect(statusDot('google', results, true, true)).toBe('done');
    expect(statusDot('google', results, false, true)).toBe('done');
  });

  it('empty-string result is not done: !loading and no usable result -> failed', () => {
    expect(statusDot('google', { google: { result: '' } }, false, true)).toBe('failed');
  });

  it('!loading and the engine is absent (backend emits no event for failed engines) -> failed', () => {
    const results: Record<string, PaneResult> = { deepl: { result: 'hallo' } };
    expect(statusDot('google', results, false, true)).toBe('failed');
  });

  it('absent engine stays pending while loading (before the 15 s fallback)', () => {
    const results: Record<string, PaneResult> = { deepl: { result: 'hallo' } };
    expect(statusDot('google', results, true, true)).toBe('pending');
  });

  it('case 1: one engine fails while a sibling succeeds in a multi-engine fan-out (loading=false) -> failed dot immediately failed', () => {
    const results: Record<string, PaneResult> = { deepl: { result: 'hallo' } };
    const dots = statusDots(ENGINES, results, false, true);
    expect(dots.google).toBe('failed');
    expect(dots.deepl).toBe('done');
  });

  it('case 2: the only engine fails; loading stays true before the 15 s fallback -> dot stays pending', () => {
    const sole = [ENGINES[0]];
    const dots = statusDots(sole, {}, true, true);
    expect(dots.google).toBe('pending');
  });

  it('statusDots only covers enabled translate engines (ocr / disabled engines get no dot)', () => {
    const all = [...ENGINES, OCR_ENGINE];
    const dots = statusDots(all, {}, true, true);
    expect(Object.keys(dots).sort()).toEqual(['deepl', 'google']);
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

// issue #96 (supersedes #42's string-returning version): failureMessage(result, t, engineLabel)
// returns { headline, detail, action }. The wire field is the generated binding name `error_kind`
// (snake case), never `errorKind`. t is injected; new copy takes {engine}.
describe('failureMessage (#96 surfacing failure reasons)', () => {
  // t echoes "key" and, when an engine param is passed, "key[engine]" so both the key choice and
  // the {engine} parameter are observable without depending on final copy.
  const t = (key: string, params?: Record<string, string | number>): string =>
    params && 'engine' in params ? `${key}[${params.engine}]` : key;

  const kinds: Array<[string, string, string | null]> = [
    // [error_kind, expected headline key, expected action]
    ['not_configured', 'translate.failedNotConfigured', 'settings'],
    ['auth', 'translate.failedAuth', 'settings'],
    ['quota', 'translate.failedQuota', null],
    ['rate_limit', 'translate.failedRateLimit', null],
    ['unavailable', 'translate.failedUnavailable', null],
    ['network', 'translate.failedNetwork', null],
    ['too_long', 'translate.failedTooLong', null],
    ['engine', 'translate.failed', null],
  ];

  for (const [kind, key, action] of kinds) {
    it(`maps ${kind} to ${key} with {engine}, detail = raw error, action = ${action}`, () => {
      const r = failureMessage({ engine: 'google', error: 'raw detail', error_kind: kind }, t, 'Google');
      if (key === 'translate.failed') {
        expect(r.headline).toMatch(/^translate\.failed(\[Google\])?$/);
      } else {
        expect(r.headline).toBe(`${key}[Google]`);
      }
      expect(r.detail).toBe('raw detail');
      expect(r.action).toBe(action);
    });
  }

  it('pair on apple keeps the existing system-settings copy (failedPair)', () => {
    const r = failureMessage({ engine: 'apple', error: 'Unable to Translate', error_kind: 'pair' }, t, 'Apple');
    expect(r.headline.startsWith('translate.failedPair')).toBe(true);
    expect(r.detail).toBe('Unable to Translate');
    expect(r.action).toBeNull();
  });

  it('pair on any other engine uses failedUnsupported with {engine}', () => {
    const r = failureMessage({ engine: 'deepl', error: 'no pair', error_kind: 'pair' }, t, 'DeepL');
    expect(r.headline).toBe('translate.failedUnsupported[DeepL]');
    expect(r.action).toBeNull();
  });

  it('an unknown kind falls back to the generic headline but keeps the detail', () => {
    const r = failureMessage({ engine: 'x', error: 'boom', error_kind: 'something_new' }, t, 'X');
    expect(r.headline).toMatch(/^translate\.failed(\[X\])?$/);
    expect(r.detail).toBe('boom');
    expect(r.action).toBeNull();
  });

  it('only not_configured and auth carry the settings action', () => {
    for (const [kind, , action] of kinds) {
      const r = failureMessage({ engine: 'e', error: 'x', error_kind: kind }, t, 'E');
      expect(r.action === 'settings').toBe(action === 'settings');
    }
  });

  it('ignores the retired camelCase field', () => {
    const legacy = { engine: 'e', error: 'x', errorKind: 'not_configured' } as unknown as PaneResult;
    const r = failureMessage(legacy, t, 'E');
    expect(r.headline).toMatch(/^translate\.failed(\[E\])?$/);
    expect(r.action).toBeNull();
  });

  it('returns the generic headline with empty detail for null/absent results', () => {
    for (const input of [null, undefined, { engine: 'x' }]) {
      const r = failureMessage(input as PaneResult | null | undefined, t, 'X');
      expect(r.headline).toMatch(/^translate\.failed(\[X\])?$/);
      expect(r.detail).toBe('');
      expect(r.action).toBeNull();
    }
  });

  it('accepts a bindings-shaped TranslateResult (interface, no index signature) without a cast', () => {
    // Type-level contract (AC/A finding 5): the first parameter is structural and narrow.
    const bindingsShaped: { engine?: string; error?: string; error_kind?: string } = {
      engine: 'google',
      error: 'e',
      error_kind: 'rate_limit',
    };
    expect(failureMessage(bindingsShaped, t, 'Google').headline).toBe('translate.failedRateLimit[Google]');
  });
});

// issue #81: idle pane vs failed pane. paneState takes ONE object argument:
//   { hasEngines, engine, results, loading, requested } -> 'no-engine' | 'loading' | 'result' | 'idle' | 'failed'
// Order: no-engine > loading (loading && engine absent from results) > result (non-empty .result)
// > failed (requested) > idle. Manual edits are NOT an input (an edit alone never yields 'result').
// statusDot/statusDots gain an explicit 4th `requested` argument; DotState gains 'idle'.
describe('paneState (#81 idle vs failed)', () => {
  const base = { hasEngines: true, engine: 'google', results: {}, loading: false, requested: false };

  it('no active engine -> no-engine (wins over everything)', () => {
    expect(paneState({ ...base, hasEngines: false, loading: true, requested: true })).toBe('no-engine');
  });

  it('nothing requested, not loading -> idle (not failed)', () => {
    expect(paneState(base)).toBe('idle');
  });

  it('loading with the engine absent from results -> loading', () => {
    expect(paneState({ ...base, loading: true, requested: true })).toBe('loading');
  });

  it('non-empty result -> result', () => {
    expect(
      paneState({ ...base, requested: true, results: { google: { result: 'hola' } } }),
    ).toBe('result');
  });

  it('result present also after a restore where loading is false and requested is true', () => {
    expect(
      paneState({ ...base, requested: true, results: { google: { result: 'x' } }, loading: false }),
    ).toBe('result');
  });

  it('requested, not loading, engine absent from results -> failed', () => {
    expect(paneState({ ...base, requested: true })).toBe('failed');
  });

  it('requested, not loading, engine returned an empty result -> failed', () => {
    expect(paneState({ ...base, requested: true, results: { google: { result: '' } } })).toBe('failed');
  });

  it('an error payload (no result) after a request -> failed', () => {
    expect(
      paneState({ ...base, requested: true, results: { google: { error: 'boom', errorKind: 'network' } } }),
    ).toBe('failed');
  });

  it('a result for ANOTHER engine does not make the active engine a result', () => {
    expect(paneState({ ...base, requested: true, results: { deepl: { result: 'x' } } })).toBe('failed');
  });

  it('idle stays idle when a stale empty results map is present and nothing was requested', () => {
    expect(paneState({ ...base, results: { google: { result: '' } } })).toBe('idle');
  });
});

describe('statusDot requested rule (#81)', () => {
  it('idle window (not requested, not loading, no result) -> never failed', () => {
    expect(statusDot('google', {}, false, false)).not.toBe('failed');
    expect(statusDot('google', {}, false, false)).toBe('idle');
  });

  it('requested and not loading with no result -> failed', () => {
    expect(statusDot('google', {}, false, true)).toBe('failed');
  });

  it('a result is done regardless of requested', () => {
    expect(statusDot('google', { google: { result: 'x' } }, false, false)).toBe('done');
  });

  it('loading is pending', () => {
    expect(statusDot('google', {}, true, true)).toBe('pending');
  });

  it('statusDots on an idle window has no failed dot', () => {
    const dots = statusDots(ENGINES, {}, false, false);
    expect(Object.values(dots)).not.toContain('failed');
    expect(Object.keys(dots).sort()).toEqual(['deepl', 'google']);
  });
});

// issue #109: cancelled is not failed. A cancel flag rides on the result entry (payload
// `cancelled: true`, no error); the dot, the pane state and the settle rule all read it.
describe('cancelled (#109)', () => {
  const cancelledEntry: PaneResult = { engine: 'google', result: '', cancelled: true };
  const partial: PaneResult = { engine: 'google', result: 'partial text', cancelled: true };

  it('statusDot: a cancelled entry is cancelled, not failed, whether or not the request is still open', () => {
    expect(statusDot('google', { google: cancelledEntry }, false, true)).toBe('cancelled');
    expect(statusDot('google', { google: cancelledEntry }, true, true)).toBe('cancelled');
  });
  it('statusDot precedence: cancelled first, then done, pending, failed, idle', () => {
    expect(statusDot('google', { google: partial }, false, true)).toBe('cancelled');
    expect(statusDot('google', { google: { result: 'x' } }, false, true)).toBe('done');
    expect(statusDot('google', {}, true, true)).toBe('pending');
    expect(statusDot('google', {}, false, true)).toBe('failed');
    expect(statusDot('google', {}, false, false)).toBe('idle');
  });
  it('a failure payload (error set, not cancelled) is still failed', () => {
    expect(statusDot('google', { google: { result: '', error: 'boom' } }, false, true)).toBe('failed');
  });
  it('statusDots reports cancelled per engine', () => {
    const dots = statusDots(ENGINES, { google: cancelledEntry, deepl: { result: 'y' } }, false, true);
    expect(dots.google).toBe('cancelled');
    expect(dots.deepl).toBe('done');
  });
  const base = { hasEngines: true, engine: 'google', results: {}, loading: false, requested: true };
  it('paneState: a cancelled entry with an empty result is cancelled', () => {
    expect(paneState({ ...base, results: { google: cancelledEntry } })).toBe('cancelled');
  });
  it('paneState: a cancelled entry with a partial result stays result (#84 keeps the parts on screen)', () => {
    expect(paneState({ ...base, results: { google: partial } })).toBe('result');
  });
  it('paneState: a cancelled entry is never failed, even after the wait ends', () => {
    expect(paneState({ ...base, results: { google: cancelledEntry }, loading: true })).not.toBe('failed');
    expect(paneState({ ...base, results: { google: cancelledEntry } })).not.toBe('failed');
  });
  it('paneState: an error entry with no result is still failed', () => {
    expect(paneState({ ...base, results: { google: { result: '', error: 'boom' } } })).toBe('failed');
  });
});

// Issue #80: an identity result (the source text, flagged by the service) is an ordinary result
// for the pane and the dot; only the muted line differs.
describe('identity result (#80)', () => {
  const results: Record<string, PaneResult> = { apple: { engine: 'apple', result: 'src', identity: true } };

  it('paneState is result', () => {
    expect(paneState({ hasEngines: true, engine: 'apple', results, loading: false, requested: true })).toBe('result');
  });

  it('statusDot is done', () => {
    expect(statusDot('apple', results, false, true)).toBe('done');
  });
});
