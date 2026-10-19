// Issue #8 (collector item): "Cmd+, opens the Settings window, the standard macOS shortcut for
// an app's settings." This is the pure predicate every Kai window's keydown handler asks, so
// the semantics live in one place: Mac-only — metaKey, never ctrlKey, matching this repo's
// shortcut conventions (see TranslateWindow.svelte's handleTranslateShortcut / Cmd+Y comments)
// — with no Alt and no Shift, matched on the final key string so Cmd alone (key === 'Meta') or
// a shifted comma ('<' on a US layout) never counts. The IME-composition guard (e.isComposing)
// stays with the callers, exactly like the undo/redo path: while a composition is open the key
// belongs to the IME, not to the window.

export function isSettingsShortcut(e: KeyboardEvent): boolean {
  if (typeof e.key !== 'string' || e.key !== ',') return false;
  return e.metaKey && !e.ctrlKey && !e.altKey && !e.shiftKey;
}
