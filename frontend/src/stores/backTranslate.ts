import { persisted } from './persisted';

// The translate window's "Show a back-translation" switch (issue #56). Off by default: it doubles
// the translation calls, which costs time and, with cloud engines, money. Remembered like the other
// window preferences.
export const backTranslateOn = persisted<boolean>('kai:translate:backTranslate', false);
