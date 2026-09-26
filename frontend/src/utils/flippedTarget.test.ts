import { describe, expect, it } from 'vitest';
import { flippedTargetLabel } from './flippedTarget.ts';

// Issue #44: per-result, display-only label for a target the backend auto-flipped (X->X guard).
// Contract: flippedTargetLabel(requestedTo, resultTo, nameOf) returns the display name of the
// result's own target when it differs from the requested one, else null. Pure: it takes no
// setters, so it cannot assign toLang/fromLang or trigger a retranslate.
const nameOf = (code: string) => ({ en: 'English', zh: 'Chinese' })[code] ?? code;

describe('flippedTargetLabel', () => {
  it('returns null when the result target is the requested one', () => {
    expect(flippedTargetLabel('en', 'en', nameOf)).toBeNull();
  });

  it('names the flipped target when the result target differs', () => {
    expect(flippedTargetLabel('zh', 'en', nameOf)).toBe('English');
  });

  it('returns null while there is no result target yet', () => {
    expect(flippedTargetLabel('zh', '', nameOf)).toBeNull();
  });
});
