import { describe, expect, it } from 'vitest';
import {
  acceptCorrection,
  correctionShown,
  textToSend,
  unavailableKey,
  type CorrectionAnswer,
} from './sourceCorrection.ts';

// Issue #208: the small pure gate between the backend's correction answer and the window. The
// decisions (setting, language, guards, diff) are the backend's, tested in Go; this only refuses
// an answer that cannot be applied and says what to send.

const ORIGINAL = 'Ellos no sabe donde esta la biblioteca';
const answer = (over: Partial<CorrectionAnswer> = {}): CorrectionAnswer => ({
  corrected: true,
  status: 'corrected',
  reason: '',
  text: 'Ellos no saben dónde está la biblioteca',
  original: ORIGINAL,
  language: 'es-MX',
  changes: [{ before: 'no sabe donde esta', after: 'no saben dónde está' }],
  ...over,
});

describe('acceptCorrection', () => {
  it('takes a corrected answer for the text that was sent', () => {
    const c = acceptCorrection(answer(), ORIGINAL);
    expect(c).toEqual({
      original: ORIGINAL,
      text: 'Ellos no saben dónde está la biblioteca',
      language: 'es-MX',
      changes: [{ before: 'no sabe donde esta', after: 'no saben dónde está' }],
    });
  });

  it('refuses everything that is not a usable correction', () => {
    expect(acceptCorrection(null, ORIGINAL)).toBeNull();
    expect(acceptCorrection(undefined, ORIGINAL)).toBeNull();
    expect(acceptCorrection(answer({ corrected: false }), ORIGINAL)).toBeNull();
    expect(acceptCorrection(answer({ text: '   ' }), ORIGINAL)).toBeNull();
    expect(acceptCorrection(answer({ text: ORIGINAL }), ORIGINAL)).toBeNull();
    // An answer for another text than the one on screen is not this text's correction.
    expect(acceptCorrection(answer({ original: 'otro texto distinto' }), ORIGINAL)).toBeNull();
    // Nothing to show is nothing corrected.
    expect(acceptCorrection(answer({ changes: [] }), ORIGINAL)).toBeNull();
    expect(acceptCorrection(answer({ changes: null }), ORIGINAL)).toBeNull();
  });
});

describe('what is shown and sent', () => {
  const c = acceptCorrection(answer(), ORIGINAL)!;

  it('the correction describes the window only while the text is the one it was made for', () => {
    expect(correctionShown(c, ORIGINAL)).toBe(true);
    expect(correctionShown(c, ORIGINAL + ' más')).toBe(false);
    expect(correctionShown(null, ORIGINAL)).toBe(false);
  });

  it('sends the corrected text while shown, and the text on screen otherwise', () => {
    expect(textToSend(c, ORIGINAL)).toBe('Ellos no saben dónde está la biblioteca');
    expect(textToSend(c, 'texto nuevo')).toBe('texto nuevo');
    expect(textToSend(null, ORIGINAL)).toBe(ORIGINAL);
  });
});

describe('unavailableKey', () => {
  it('words every reason the backend can give, and any other one generically', () => {
    expect(unavailableKey('apple_intelligence_off')).toBe(
      'translate.correctUnavailableAppleIntelligence',
    );
    expect(unavailableKey('model_not_ready')).toBe('translate.correctUnavailableNotReady');
    expect(unavailableKey('unsupported_hardware')).toBe('translate.correctUnavailableHardware');
    expect(unavailableKey('unsupported_platform')).toBe('translate.correctUnavailablePlatform');
    expect(unavailableKey('unavailable')).toBe('translate.correctUnavailable');
    expect(unavailableKey('something new')).toBe('translate.correctUnavailable');
    expect(unavailableKey('')).toBe('translate.correctUnavailable');
  });
});
