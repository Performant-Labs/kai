import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Issue #48: how TranslateWindow wires the result pane's context chat. No Svelte render harness
// exists (#78), so, like translateShortcut.test.ts, this reads the component sources and pins the
// wiring by structure. The decisions (engine choice, fallback order, prompt) are the backend's and
// are tested in Go; contextChat.test.ts covers the pure state.

const win = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');
const panel = readFileSync(resolve(__dirname, 'ContextChatPanel.svelte'), 'utf8');
const script = win.slice(0, win.indexOf('</script>'));
const code = script.replace(/\/\*[\s\S]*?\*\/|\/\/[^\n]*/g, (m) => m.replace(/[^\n]/g, ' '));

function fnBody(name: string): string {
  const start = code.search(new RegExp(`(?:async\\s+)?function\\s+${name}\\s*\\(`));
  expect(start, `${name} not found`).toBeGreaterThan(-1);
  const open = code.indexOf('{', code.indexOf(')', start));
  let depth = 0;
  for (let i = open; i < code.length; i++) {
    if (code[i] === '{') depth++;
    else if (code[i] === '}' && --depth === 0) return code.slice(start, i + 1);
  }
  return code.slice(start);
}

describe('the context chat is wired into the translate window (issue #48)', () => {
  it('asks the backend through the generated binding', () => {
    expect(code).toMatch(/RetranslateWithContext[\s\S]*translatewrapper\.ts/);
    expect(fnBody('runContext')).toMatch(/await RetranslateWithContext\(/);
  });

  it('keeps the context across texts: only the clear action resets the chat', () => {
    for (const fn of ['doTranslate', 'clearInput', 'swap']) {
      expect(fnBody(fn), `${fn} must not reset the chat`).not.toMatch(/chat\s*=\s*clearChat\(\)/);
    }
    expect(fnBody('clearContext')).toMatch(/chat\s*=\s*clearChat\(\)/);
  });

  it('applies a kept context to the next translation, and drops the old retranslation', () => {
    const fn = fnBody('doTranslate');
    expect(fn).toMatch(/retrans\s*=\s*null/);
    expect(fn).toMatch(/if\s*\(\s*hasContext\(chat\)\s*\)\s*void runContext\(''\)/);
  });

  it('clearing cancels a request in flight and forgets the retranslation', () => {
    const fn = fnBody('clearContext');
    expect(fn).toMatch(/if\s*\(\s*chatBusy\s*\)\s*cancelContext\(\)/);
    expect(fn).toMatch(/retrans\s*=\s*null/);
    // The retranslation is also shown through the edited map: clearing must take it out of there.
    expect(fn).toMatch(/edited\s*=\s*dropRetranslation\(edited,\s*retrans\)/);
    expect(fn.indexOf('dropRetranslation')).toBeLessThan(fn.indexOf('retrans = null'));
    expect(fnBody('cancelContext')).toMatch(/CancelTranslate\(chatRequestId,\s*''\)/);
  });

  it('has a keyboard shortcut that clears the context, at window level', () => {
    const fn = fnBody('onWindowKeydown');
    expect(fn).toMatch(/isClearShortcut\(e,\s*clearShortcut\)/);
    expect(fn).toMatch(/clearContext\(\)/);
    expect(fn.indexOf('isClearShortcut')).toBeLessThan(fn.indexOf('shortcutAction'));
  });

  it('shows a retranslation the way a manual edit is shown, and can go back to the engine result', () => {
    expect(fnBody('runContext')).toMatch(/edited\s*=\s*new Map\(edited\)\.set\(forEngine,/);
    const back = fnBody('showOriginal');
    expect(back).toMatch(/next\.delete\(activeEngine\)/);
    expect(back).toMatch(/retrans\s*=\s*null/);
  });

  it('renders the panel with the clear handler', () => {
    expect(win).toMatch(/<ContextChatPanel[\s\S]*onclear=\{clearContext\}/);
    expect(win).toMatch(/onsend=\{sendContext\}/);
  });
});

describe('the chat panel (issue #48)', () => {
  it('has a clear button, only while a context is active, with the shortcut in its tooltip', () => {
    expect(panel).toMatch(
      /\{#if active\}[\s\S]*data-testid="context-clear"[\s\S]*onclick=\{onclear\}/,
    );
    expect(panel).toMatch(/translate\.contextClearHint',\s*\{\s*shortcut:\s*shortcutLabel\s*\}/);
  });

  it('shows that a context is active even while the chat is closed', () => {
    const activeAt = panel.indexOf('data-testid="context-active"');
    const openAt = panel.indexOf('{#if open}');
    expect(activeAt).toBeGreaterThan(-1);
    expect(activeAt).toBeLessThan(openAt);
  });

  it('sends on Enter (not Shift+Enter, not Cmd+Enter which translates) and can cancel a running request', () => {
    expect(panel).toMatch(/e\.key === 'Enter' && !e\.shiftKey && !e\.metaKey/);
    expect(panel).toMatch(/data-testid="context-cancel"/);
    expect(panel).toMatch(/onclick=\{oncancel\}/);
  });
});

describe('the clear-context shortcut is configurable in Settings > Shortcuts (issue #48)', () => {
  const tab = readFileSync(resolve(__dirname, 'settings/ShortcutsTab.svelte'), 'utf8');
  const tabScript = tab.slice(0, tab.indexOf('</script>'));
  const tabCode = tabScript.replace(/\/\*[\s\S]*?\*\/|\/\/[^\n]*/g, (m) =>
    m.replace(/[^\n]/g, ' '),
  );

  it('has a field, a Record button and a Reset button for it', () => {
    expect(tab).toMatch(/data-testid="clear-context-key"[\s\S]*bind:value=\{clearContextKey\}/);
    expect(tab).toMatch(/data-testid="clear-context-record"[\s\S]*startRecord\('clearContext'\)/);
    expect(tab).toMatch(
      /data-testid="clear-context-reset"[\s\S]*clearContextKey = DEFAULT_CLEAR_SHORTCUT/,
    );
  });

  it('records into the field like the other shortcuts', () => {
    expect(tabCode).toMatch(/k === 'clearContext'\)\s*\{\s*clearContextKey = combo/);
  });

  it('validates before saving anything, refuses a bad combo with its own message, then stores and broadcasts', () => {
    const start = tabCode.indexOf('async function saveShortcuts');
    const body = tabCode.slice(start, tabCode.indexOf('await SaveConfig', start));
    expect(body).toMatch(/validateClearShortcut\(clearContextKey\)/);
    expect(body).toMatch(/if \(!check\.ok\)[\s\S]*Dialogs\.Error[\s\S]*return;/);
    for (const key of [
      'hkClearContextNeedsModifier',
      'hkClearContextNoKey',
      'hkClearContextReserved',
    ]) {
      expect(body).toContain(key);
    }
    expect(body).toMatch(/clearContextShortcut\.set\(check\.value\)/);
    expect(body).toMatch(/emitEvent\(EventClearContextShortcutChanged, check\.value\)/);
    expect(body.indexOf('validateClearShortcut')).toBeLessThan(
      body.indexOf('clearContextShortcut.set'),
    );
  });

  it('the translate window picks up a new shortcut without being reopened', () => {
    expect(code).toMatch(
      /onEvent\(EventClearContextShortcutChanged,[\s\S]*clearContextShortcut\.set\(v\)/,
    );
    expect(code).toMatch(/offClearShortcut\(\)/);
    expect(code).toMatch(/const clearShortcut = \$derived\(\$clearContextShortcut\)/);
  });
});
