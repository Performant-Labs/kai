import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Issue #165 (Tester, RED): Cmd+Enter translates, only while this window has focus. No Svelte
// render harness exists (#78), so — like sourceUndo.test.ts / swapWindow.test.ts — this reads the
// component source and pins the wiring by structure, not by rendering it.

const src = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');
const scriptEnd = src.indexOf('</script>');
const script = src.slice(0, scriptEnd);
const markup = src.slice(scriptEnd);

/** Comments blanked to spaces (offsets preserved), so a comment never satisfies or breaks a rule. */
const code = script.replace(/\/\*[\s\S]*?\*\/|\/\/[^\n]*/g, (m) => m.replace(/[^\n]/g, ' '));

/** [start, end) brace-matched block starting at the first `{` at or after `from`. */
function braceBlock(text: string, from: number): string {
  const open = text.indexOf('{', from);
  expect(open, 'no opening brace').toBeGreaterThan(-1);
  let depth = 0;
  for (let i = open; i < text.length; i++) {
    if (text[i] === '{') depth++;
    else if (text[i] === '}' && --depth === 0) return text.slice(from, i + 1);
  }
  return text.slice(from);
}

/** Start offset of a named function in the script: `function name(`. */
function fnStart(name: string): number {
  return code.search(new RegExp(`(?:async\\s+)?function\\s+${name}\\s*\\(`));
}

/** The full text of a named `function name(...) { ... }` declaration. */
function fnBody(name: string): string {
  const start = fnStart(name);
  expect(start, `${name} not found`).toBeGreaterThan(-1);
  const paren = code.indexOf(')', start);
  const block = braceBlock(code, paren);
  return code.slice(start, paren + block.length);
}

describe('Cmd+Enter translates while the window has focus (issue #165)', () => {
  it('handleTranslateShortcut checks metaKey + Enter, never ctrlKey (Mac-only, not Ctrl+Enter)', () => {
    const fn = fnBody('handleTranslateShortcut');
    expect(fn).toMatch(/e\.key\s*!==\s*'Enter'/);
    expect(fn).toMatch(/!e\.metaKey/);
    expect(fn).toMatch(/e\.ctrlKey/);
  });

  it('always prevents default (Enter must not insert a newline in the source textarea)', () => {
    const fn = fnBody('handleTranslateShortcut');
    // preventDefault happens before the awaiting/input guard, i.e. unconditionally once matched.
    const guardAt = fn.search(/if\s*\(\s*!awaiting/);
    const preventAt = fn.indexOf('preventDefault');
    expect(preventAt).toBeGreaterThan(-1);
    expect(guardAt).toBeGreaterThan(-1);
    expect(preventAt).toBeLessThan(guardAt);
  });

  it('only calls doTranslate when not already awaiting and there is input (matches the button)', () => {
    const fn = fnBody('handleTranslateShortcut');
    expect(fn).toMatch(/!awaiting\s*&&\s*input\.trim\(\)/);
    expect(fn).toMatch(/doTranslate\(\)/);
  });

  it('onWindowKeydown consults handleTranslateShortcut before the undo/redo shortcut', () => {
    const fn = fnBody('onWindowKeydown');
    const shortcutAt = fn.indexOf('handleTranslateShortcut');
    const actionAt = fn.indexOf('shortcutAction');
    expect(shortcutAt).toBeGreaterThan(-1);
    expect(actionAt).toBeGreaterThan(-1);
    expect(shortcutAt).toBeLessThan(actionAt);
  });

  it('onWindowKeydown is wired to the window (only fires while this window has focus)', () => {
    expect(markup).toMatch(/<svelte:window\s+onkeydown=\{onWindowKeydown\}/);
  });
});
