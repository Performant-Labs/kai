// The History shortcut (issue #60), configured like the clear-context one (issue #48): the same
// "Mod+Key" grammar, the same recorder, the same matcher (isClearShortcut reads any combo).
import { validateClearShortcut, type ShortcutCheck } from './contextChat.ts';

/** Cmd+Y, the Mac convention for history. */
export const DEFAULT_HISTORY_SHORTCUT = 'Cmd+Y';

export type HistoryShortcutCheck = ShortcutCheck | { ok: false; reason: 'same_as_clear' };

/**
 * Checks the shortcut the user typed or recorded. Empty means "no shortcut". It may not be the
 * clear-context shortcut, since one key cannot do both.
 */
export function validateHistoryShortcut(raw: string, clearCombo: string): HistoryShortcutCheck {
  const check = validateClearShortcut(raw);
  if (!check.ok || check.value === '') return check;
  const clear = validateClearShortcut(clearCombo);
  if (clear.ok && clear.value !== '' && clear.value === check.value) {
    return { ok: false, reason: 'same_as_clear' };
  }
  return check;
}
