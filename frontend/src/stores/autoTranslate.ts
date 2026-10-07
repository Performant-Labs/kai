import { persisted } from './persisted';

// The translate window's "Translate as I type" switch (issue #57). On by default: the user asked for
// translation without a click. It matters with cloud engines, where every request costs, so it can be
// turned off. Remembered like the other window preferences.
export const autoTranslateOn = persisted<boolean>('kai:translate:autoTranslate', true);
