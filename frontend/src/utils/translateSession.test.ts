// issue #81: pure validator for the retained translate session (source text, per-engine results,
// requestedTo, requested). Storage itself is stores/persisted.ts; this module only validates what
// was read back.
import { describe, it, expect } from 'vitest';
import { emptySession, restoreSession, type TranslateSession } from './translateSession.ts';

const EMPTY: TranslateSession = { input: '', results: {}, requestedTo: '', requested: false };

describe('emptySession', () => {
  it('is the idle session', () => {
    expect(emptySession()).toEqual(EMPTY);
  });

  it('returns a fresh object each call (no shared mutable results)', () => {
    const a = emptySession();
    a.results['x'] = { engine: 'x' };
    expect(emptySession().results).toEqual({});
  });
});

describe('restoreSession', () => {
  it.each([
    ['null', null],
    ['undefined', undefined],
    ['a string', 'hello'],
    ['a number', 42],
    ['an array', []],
    ['an empty object', {}],
  ])('%s -> empty session', (_n, raw) => {
    expect(restoreSession(raw)).toEqual(EMPTY);
  });

  it.each([
    ['input not a string', { input: 5, results: {}, requestedTo: '', requested: false }],
    ['results not an object', { input: 'a', results: 'x', requestedTo: '', requested: false }],
    ['results null', { input: 'a', results: null, requestedTo: '', requested: false }],
    ['requestedTo not a string', { input: 'a', results: {}, requestedTo: 3, requested: false }],
    ['requested not a boolean', { input: 'a', results: {}, requestedTo: '', requested: 'yes' }],
  ])('wrong field type (%s) -> empty session', (_n, raw) => {
    expect(restoreSession(raw)).toEqual(EMPTY);
  });

  it('a valid saved session round-trips through JSON', () => {
    const s: TranslateSession = {
      input: 'hello',
      results: { google: { engine: 'google', result: 'hola', to: 'es' } },
      requestedTo: 'es',
      requested: true,
    };
    expect(restoreSession(JSON.parse(JSON.stringify(s)))).toEqual(s);
  });

  it('requested is forced false when results is empty', () => {
    const r = restoreSession({ input: 'hi', results: {}, requestedTo: 'es', requested: true });
    expect(r.requested).toBe(false);
    expect(r.input).toBe('hi');
  });

  it('requested stays true when at least one result survives', () => {
    const r = restoreSession({
      input: 'hi',
      results: { google: { engine: 'google', result: 'x' } },
      requestedTo: 'es',
      requested: true,
    });
    expect(r.requested).toBe(true);
  });

  it('requested is forced false when every result entry is dropped as invalid', () => {
    const r = restoreSession({
      input: 'hi',
      results: { google: { result: 'no engine field' } },
      requestedTo: 'es',
      requested: true,
    });
    expect(r.results).toEqual({});
    expect(r.requested).toBe(false);
  });

  it('drops results entries without an object value or a string engine, keeps the rest', () => {
    const r = restoreSession({
      input: 'hi',
      results: {
        good: { engine: 'good', result: 'ok' },
        noEngine: { result: 'x' },
        numEngine: { engine: 7, result: 'x' },
        nullEntry: null,
        strEntry: 'oops',
      },
      requestedTo: 'es',
      requested: true,
    });
    expect(Object.keys(r.results)).toEqual(['good']);
  });

  // Found by hand-testing #82: an engine failure arrives as a results entry with an Error and no
  // result text. Restoring it made the reopened window show "Translation failed" for a request
  // from a previous run, before the user had done anything.
  it('drops an engine failure payload (no result text): a failure is not a translation to restore', () => {
    const r = restoreSession({
      input: 'hi',
      results: {
        apple: { engine: 'apple', result: '', error: 'Unable to Translate', error_kind: 'x' },
        noKey: { engine: 'noKey', error: 'boom' },
      },
      requestedTo: 'en',
      requested: true,
    });
    expect(r.results).toEqual({});
    expect(r.requested).toBe(false);
    expect(r.input).toBe('hi');
  });

  it('keeps the successful engine and drops the failed one when both were stored', () => {
    const r = restoreSession({
      input: 'hi',
      results: {
        google: { engine: 'google', result: 'hola' },
        apple: { engine: 'apple', result: '', error: 'Unable to Translate' },
      },
      requestedTo: 'es',
      requested: true,
    });
    expect(Object.keys(r.results)).toEqual(['google']);
    expect(r.requested).toBe(true);
  });
});
