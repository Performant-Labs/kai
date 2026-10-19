import { describe, expect, it } from 'vitest';
import { isSettingsShortcut } from './settingsShortcut.ts';

// Issue #8 (collector item, quoted): "Cmd+, opens the Settings window, the standard macOS
// shortcut for an app's settings." The predicate mirrors the Cmd+Y history handler's semantics
// (TranslateWindow.svelte): Mac-only — metaKey, never ctrlKey — no Alt/Shift, matched on the
// final key string so a press of Cmd alone (key === 'Meta') or a shifted comma ('<') is not one.
// The IME-composition guard (e.isComposing) stays in each window's keydown handler, exactly
// like the undo/redo path.

describe('isSettingsShortcut', () => {
  const ev = (over: Partial<KeyboardEvent>) =>
    ({
      key: ',',
      metaKey: true,
      shiftKey: false,
      ctrlKey: false,
      altKey: false,
      ...over,
    }) as KeyboardEvent;

  it('matches Cmd+, exactly', () => {
    expect(isSettingsShortcut(ev({}))).toBe(true);
  });

  it('ignores a plain comma with no modifier', () => {
    expect(isSettingsShortcut(ev({ metaKey: false }))).toBe(false);
  });

  it('is Mac-only: Ctrl+, and Ctrl+Cmd+, do nothing', () => {
    expect(isSettingsShortcut(ev({ metaKey: false, ctrlKey: true }))).toBe(false);
    expect(isSettingsShortcut(ev({ ctrlKey: true }))).toBe(false);
  });

  it('ignores Alt and Shift combinations', () => {
    expect(isSettingsShortcut(ev({ altKey: true }))).toBe(false);
    expect(isSettingsShortcut(ev({ shiftKey: true }))).toBe(false);
  });

  it('does not fire on the Cmd key itself or a shifted comma', () => {
    expect(isSettingsShortcut(ev({ key: 'Meta' }))).toBe(false);
    expect(isSettingsShortcut(ev({ key: '<' }))).toBe(false);
  });

  it('tolerates a non-string key without throwing', () => {
    expect(isSettingsShortcut(ev({ key: undefined as unknown as string }))).toBe(false);
  });
});
