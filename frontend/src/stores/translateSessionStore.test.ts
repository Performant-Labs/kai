// issue #81: the translate session is retained through the existing persisted() store and
// validated on the way back in by restoreSession. Real jsdom localStorage, no mocks.
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { get } from 'svelte/store';
import { persisted } from './persisted.ts';
import { emptySession, restoreSession, type TranslateSession } from '../utils/translateSession.ts';

const KEY = 'kai:translate:session';

beforeEach(() => window.localStorage.removeItem(KEY));
afterEach(() => window.localStorage.clear());

describe('persisted translate session', () => {
  it('reads a saved session back through restoreSession', () => {
    const saved: TranslateSession = {
      input: 'good morning',
      results: { google: { engine: 'google', result: 'buenos dias' } },
      requestedTo: 'es',
      requested: true,
    };
    window.localStorage.setItem(KEY, JSON.stringify(saved));
    const store = persisted<TranslateSession>(KEY, emptySession());
    expect(restoreSession(get(store))).toEqual(saved);
  });

  it('a written session survives a fresh store instance (close/reopen or restart)', () => {
    const a = persisted<TranslateSession>(KEY, emptySession());
    a.set({ input: 'hi', results: { g: { engine: 'g', result: 'x' } }, requestedTo: 'fr', requested: true });
    const b = persisted<TranslateSession>(KEY, emptySession());
    expect(restoreSession(get(b)).input).toBe('hi');
  });

  it('falls back to the empty session on corrupt JSON', () => {
    window.localStorage.setItem(KEY, '{not json');
    const store = persisted<TranslateSession>(KEY, emptySession());
    expect(restoreSession(get(store))).toEqual(emptySession());
  });

  it('falls back to the empty session when the stored JSON has the wrong shape', () => {
    window.localStorage.setItem(KEY, JSON.stringify({ input: 12, results: [] }));
    const store = persisted<TranslateSession>(KEY, emptySession());
    expect(restoreSession(get(store))).toEqual(emptySession());
  });
});
