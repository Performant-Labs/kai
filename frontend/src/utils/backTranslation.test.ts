import { describe, expect, it } from 'vitest';
import {
  acceptBack,
  backPair,
  backShownFor,
  buildBackRequest,
  needsBack,
  shouldBackTranslate,
  type BackAnswer,
  type BoundBack,
} from './backTranslation.ts';

// Issue #56: the pure decisions between the backend's BackTranslate answers and the result pane.
// Which engine answers, the cancel and the history rule are the backend's (tested in Go); this
// decides when a back-translation is worth asking for, which pair it uses, and when what is on
// screen no longer describes the displayed text.

const answer = (over: Partial<BackAnswer> = {}): BackAnswer => ({
  status: 'ok',
  result: 'Necesito los puntos clave',
  engine: 'google',
  request_id: 'b1',
  ...over,
});

const base = { enabled: true, displayed: 'I need the bullet points', isResult: true };

describe('shouldBackTranslate', () => {
  it('asks only with the toggle on and a finished, non-empty result on screen', () => {
    expect(shouldBackTranslate(base)).toBe(true);
    expect(shouldBackTranslate({ ...base, enabled: false })).toBe(false);
    expect(shouldBackTranslate({ ...base, isResult: false })).toBe(false);
    expect(shouldBackTranslate({ ...base, displayed: '   ' })).toBe(false);
  });

  it('never asks for a failed, cancelled or identity result', () => {
    expect(shouldBackTranslate({ ...base, failed: true })).toBe(false);
    expect(shouldBackTranslate({ ...base, cancelled: true })).toBe(false);
    expect(shouldBackTranslate({ ...base, identity: true })).toBe(false);
  });
});

describe('backPair', () => {
  it('goes from the result language back to the source language', () => {
    expect(backPair({ resultFrom: 'es-MX', resultTo: 'en', detectedFrom: '' })).toEqual({
      from: 'en',
      to: 'es-MX',
    });
  });

  it('works for any pair, not only Spanish and English', () => {
    expect(backPair({ resultFrom: 'ja', resultTo: 'fr', detectedFrom: '' })).toEqual({
      from: 'fr',
      to: 'ja',
    });
  });

  it('uses the detected language when the source was auto, and none when nothing was detected', () => {
    expect(backPair({ resultFrom: 'auto', resultTo: 'en', detectedFrom: 'es-MX' })).toEqual({
      from: 'en',
      to: 'es-MX',
    });
    expect(backPair({ resultFrom: 'auto', resultTo: 'en', detectedFrom: '' })).toBeNull();
    expect(backPair({ resultFrom: '', resultTo: 'en', detectedFrom: '' })).toBeNull();
  });

  it('is none when the result has no target, or when both sides are the same language', () => {
    expect(backPair({ resultFrom: 'es', resultTo: '', detectedFrom: '' })).toBeNull();
    expect(backPair({ resultFrom: 'en', resultTo: 'en', detectedFrom: '' })).toBeNull();
  });
});

describe('buildBackRequest', () => {
  it('carries the displayed text, the swapped pair, the engine and the request id', () => {
    expect(
      buildBackRequest({
        text: 'I need the bullet points',
        pair: { from: 'en', to: 'es-MX' },
        engine: 'google',
        requestId: 'b1',
      }),
    ).toEqual({
      text: 'I need the bullet points',
      from: 'en',
      to: 'es-MX',
      engine: 'google',
      request_id: 'b1',
    });
  });
});

describe('acceptBack and backShownFor', () => {
  it('binds an ok answer to the exact text and engine it was made for', () => {
    const b = acceptBack(answer(), 'I need the bullet points', 'google', 'es-MX');
    expect(b).toEqual({
      text: 'Necesito los puntos clave',
      engine: 'google',
      lang: 'es-MX',
      forResult: 'I need the bullet points',
      forEngine: 'google',
    });
  });

  it('accepts nothing but a non-empty ok answer', () => {
    for (const status of ['cancelled', 'skipped', 'failed'] as const) {
      expect(acceptBack(answer({ status, result: '' }), 'x', 'google', 'es'), status).toBeNull();
    }
    expect(acceptBack(answer({ result: '  ' }), 'x', 'google', 'es')).toBeNull();
    expect(acceptBack(null, 'x', 'google', 'es')).toBeNull();
  });

  const b: BoundBack = {
    text: 'Necesito',
    engine: 'google',
    lang: 'es',
    forResult: 'I need',
    forEngine: 'google',
  };
  it('is shown only for the same displayed text and the same engine', () => {
    expect(backShownFor(b, 'I need', 'google')).toBe(b);
    expect(backShownFor(b, 'I need it', 'google')).toBeNull(); // the result was edited, or a new text arrived
    expect(backShownFor(b, 'I need', 'deepl')).toBeNull(); // another engine
    expect(backShownFor(null, 'I need', 'google')).toBeNull();
  });

  it('needsBack says whether a request is due: nothing shown for this text and engine yet', () => {
    expect(needsBack(b, 'I need', 'google')).toBe(false);
    expect(needsBack(b, 'I need it', 'google')).toBe(true);
    expect(needsBack(null, 'I need', 'google')).toBe(true);
  });
});
