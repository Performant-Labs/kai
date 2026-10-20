import { describe, expect, it } from 'vitest';
import { initialPin } from './pinDefault';

// Issue #69: the translate window's starting pin state is a pure decision —
//   nothing stored -> the "keep the translate window on top" setting (its own default is off);
//   a stored choice   -> the stored choice, always;
//   a corrupt value   -> the setting (same as nothing stored).
// The old one-time migration (forcing a stored false back to true, #39) is gone for good:
// a stored false must stay false whatever the setting says.

describe('initialPin', () => {
  it('nothing stored: the setting is the default, both ways', () => {
    expect(initialPin(null, true)).toBe(true);
    expect(initialPin(null, false)).toBe(false);
  });

  it('a stored true beats a setting default of off', () => {
    expect(initialPin(JSON.stringify(true), false)).toBe(true);
  });

  it('a stored false beats a setting default of on — and nothing re-pins it (the old migration must stay gone)', () => {
    expect(initialPin(JSON.stringify(false), true)).toBe(false);
    expect(initialPin(JSON.stringify(false), false)).toBe(false);
  });

  it('a corrupt stored value falls back to the setting', () => {
    expect(initialPin('{oops', true)).toBe(true);
    expect(initialPin('{oops', false)).toBe(false);
    // A stored value that parses but is not a boolean is corrupt too.
    expect(initialPin(JSON.stringify('yes'), true)).toBe(true);
    expect(initialPin(JSON.stringify('yes'), false)).toBe(false);
  });
});
