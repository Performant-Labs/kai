import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Issue #56: how TranslateWindow wires the back-translation. No Svelte render harness exists (#78),
// so, like contextChatWiring.test.ts, this reads the component source and pins the wiring by
// structure. The decisions (engine, cancel, history) are the backend's, tested in Go; the pure
// state is in utils/backTranslation.test.ts.

const win = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');
const script = win.slice(0, win.indexOf('</script>'));
const markup = win.slice(win.indexOf('</script>'));
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

describe('the back-translation is wired into the translate window (issue #56)', () => {
  it('has a toolbar switch, remembered, that sets the store', () => {
    expect(markup).toMatch(
      /data-testid="back-translate-checkbox"[\s\S]*checked=\{\$backTranslateOn\}/,
    );
    expect(markup).toMatch(
      /onchange=\{\(e\) => backTranslateOn\.set\(e\.currentTarget\.checked\)\}/,
    );
    expect(markup).toMatch(/aria-describedby="back-translate-tip"/);
  });

  it('asks the backend through the generated binding, with the request built by the pure module', () => {
    expect(code).toMatch(/BackTranslate[\s\S]*translatewrapper\.ts/);
    const fn = fnBody('startBack');
    expect(fn).toMatch(/await BackTranslate\(/);
    expect(fn).toMatch(/buildBackRequest\(/);
  });

  it('ignores an answer that arrives after the request was replaced or stopped', () => {
    const fn = fnBody('startBack');
    expect(fn).toMatch(/if \(backRequestId !== id\) return;/);
    expect(fn).toMatch(/acceptBack\(ans, displayed, engine, pair\.to\)/);
  });

  it('a newer request cancels the running one, and stopping cancels it and forgets the answer', () => {
    expect(fnBody('startBack')).toMatch(/CancelTranslate\(backRequestId, ''\)/);
    const stop = fnBody('stopBack');
    expect(stop).toMatch(/CancelTranslate\(backRequestId, ''\)/);
    expect(stop).toMatch(/back = null/);
  });

  it('is decided by the pure module and runs untracked, so its own state cannot re-trigger it', () => {
    expect(code).toMatch(
      /\$effect\(\(\) => \{[\s\S]*shouldBackTranslate\(\{[\s\S]*untrack\(\(\) => \{/,
    );
    expect(code).toMatch(/needsBack\(back, displayed, engine\)/);
    expect(code).toMatch(/backAsked\.displayed === displayed && backAsked\.engine === engine/);
  });

  it('shows the back-translation under the result, only for the displayed text and engine', () => {
    expect(code).toMatch(
      /const backShown = \$derived\(backShownFor\(back, activeDisplay, activeEngine\)\)/,
    );
    expect(markup).toMatch(/\{#if backShown\}[\s\S]*data-testid="back-translation"/);
    // After the result textarea, not before it.
    expect(markup.indexOf('data-testid="back-translation"')).toBeGreaterThan(
      markup.indexOf("placeholder={t('translate.noResult')}"),
    );
  });

  it('Copy still copies only the translation, never the back-translation', () => {
    expect(markup).toMatch(/onclick=\{\(\) => copy\(activeDisplay\)\}/);
    const copyUses = markup.match(/copy\([^)]*\)/g) ?? [];
    for (const use of copyUses) expect(use).not.toMatch(/back/i);
  });
});
