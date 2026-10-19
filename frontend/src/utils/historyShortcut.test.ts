import { describe, expect, it } from 'vitest';

// Issue #60: the History shortcut is configured like the clear-context one (#48): the same
// "Mod+Key" grammar, the same recorder, the same matcher — plus one rule of its own: it may
// not be the clear-context shortcut, since one key cannot do both. Ported from the WIP
// history-screen branch (710cd7b) onto master's code.

import { DEFAULT_HISTORY_SHORTCUT, validateHistoryShortcut } from './historyShortcut.ts';
import { isClearShortcut } from './contextChat.ts';

describe('history shortcut', () => {
  it('defaults to Cmd+Y, the Mac convention for history', () => {
    expect(DEFAULT_HISTORY_SHORTCUT).toBe('Cmd+Y');
    const e = new KeyboardEvent('keydown', { key: 'y', metaKey: true });
    expect(isClearShortcut(e, DEFAULT_HISTORY_SHORTCUT)).toBe(true);
  });
  it('accepts the default and writes it canonically', () => {
    expect(validateHistoryShortcut('cmd+y', '')).toEqual({ ok: true, value: 'Cmd+Y' });
  });
  it('allows no shortcut', () => {
    expect(validateHistoryShortcut('', 'Cmd+Shift+K')).toEqual({ ok: true, value: '' });
  });
  it('needs a modifier and a key', () => {
    expect(validateHistoryShortcut('y', '')).toEqual({ ok: false, reason: 'needs_modifier' });
    expect(validateHistoryShortcut('Cmd+', '')).toEqual({ ok: false, reason: 'no_key' });
  });
  it('refuses a key the window already uses', () => {
    expect(validateHistoryShortcut('Cmd+Z', '')).toEqual({ ok: false, reason: 'reserved' });
  });
  it('refuses the clear-context shortcut', () => {
    expect(validateHistoryShortcut('Cmd+Shift+K', 'Cmd+Shift+K')).toEqual({
      ok: false,
      reason: 'same_as_clear',
    });
  });
});
