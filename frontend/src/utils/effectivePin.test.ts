import { describe, expect, it } from 'vitest';
import { effectivePin } from './pinDefault';

// Issue #113 + A's round-2 ruling R2-1 (binding): every place the translate window applies its
// pin level — mount init, the on-top-changed broadcast, the Settings/History-close re-pin —
// resolves through ONE pure decision:
//
//   effectivePin(storedV2, pinTouched, thisMountChoice, liveSetting)
//     = pinTouched ? thisMountChoice : initialPin(storedV2, liveSetting)
//
// storedV2 is the RAW value of the versioned key kai:translate:pinned:v2 (null when absent);
// the old kai:translate:pinned key is never an input to it (the translate pin store owns
// ignoring the old key). initialPin composes through unchanged: a genuine stored v2 choice
// still wins over the setting (#69's locked rule), nothing stored falls to the setting.
//
// This file is separate from pinDefault.test.ts so the existing initialPin suite stays green
// while the effectivePin export does not exist yet (its RED is the missing-export failure of
// this file alone).
//
// The application-point SEQUENCES (mount -> broadcast -> re-pin) are pinned end-to-end with
// the real application points in components/translateWindowPinDefault.test.ts and
// stores/translatePin.test.ts; a pure function cannot distinguish application points, so the
// matrix here is the decision table those sequences draw from.

describe('effectivePin (R2-1: pinTouched ? thisMountChoice : initialPin(storedV2, liveSetting))', () => {
  it('nothing stored, untouched: the live setting decides, both ways', () => {
    expect(effectivePin(null, false, false, false)).toBe(false);
    expect(effectivePin(null, false, false, true)).toBe(true);
  });

  it('a stored v2 choice, untouched, keeps winning over the setting (initialPin composes through)', () => {
    expect(effectivePin(JSON.stringify(true), false, false, false)).toBe(true); // stored on beats setting off
    expect(effectivePin(JSON.stringify(false), false, true, true)).toBe(false); // stored off beats setting on
  });

  it('an explicit 🧷 choice this mount wins everywhere, over any stored value and any setting (AC7 case b)', () => {
    expect(effectivePin(JSON.stringify(false), true, true, false)).toBe(true); // chose on this mount
    expect(effectivePin(JSON.stringify(true), true, false, true)).toBe(false); // chose off this mount
    expect(effectivePin(null, true, true, false)).toBe(true); // chose on, nothing stored
  });

  it('a corrupt stored v2 value falls to the setting, exactly like initialPin', () => {
    expect(effectivePin('{oops', false, false, true)).toBe(true);
    expect(effectivePin('{oops', false, false, false)).toBe(false);
    // A stored value that parses but is not a boolean is corrupt too.
    expect(effectivePin(JSON.stringify('yes'), false, false, true)).toBe(true);
  });
});
