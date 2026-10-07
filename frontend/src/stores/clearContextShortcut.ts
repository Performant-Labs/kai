import { persisted } from './persisted';
import { DEFAULT_CLEAR_SHORTCUT } from '../utils/contextChat';

// The translate window's "clear context" shortcut (issue #48). Persisted like the other window
// preferences; the Settings > Shortcuts tab writes it (and broadcasts EventClearContextShortcutChanged),
// the translate window reads it. An empty string means no shortcut.
export const CLEAR_CONTEXT_SHORTCUT_KEY = 'kai:translate:clearContextShortcut';
export const clearContextShortcut = persisted<string>(
  CLEAR_CONTEXT_SHORTCUT_KEY,
  DEFAULT_CLEAR_SHORTCUT,
);
