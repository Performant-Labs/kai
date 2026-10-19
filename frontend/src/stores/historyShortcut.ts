import { persisted } from './persisted';
import { DEFAULT_HISTORY_SHORTCUT } from '../utils/historyShortcut';

// The translate window's "open History" shortcut (issue #60). Persisted like the clear-context one
// (stores/clearContextShortcut): the Settings > Shortcuts tab writes it (and broadcasts
// EventHistoryShortcutChanged), the translate window reads it. An empty string means no shortcut.
export const HISTORY_SHORTCUT_KEY = 'kai:translate:historyShortcut';
export const historyShortcut = persisted<string>(HISTORY_SHORTCUT_KEY, DEFAULT_HISTORY_SHORTCUT);
