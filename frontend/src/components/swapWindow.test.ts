import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Issue #13: the translate window's swap is dialect-aware and reverse-translates. The repo has no
// Svelte component-render harness (#28), so this pins the contract from the component source.

const src = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');

function fnBody(name: string): string {
  const start = src.indexOf(`function ${name}(`);
  expect(start, `${name} not found`).toBeGreaterThan(-1);
  const ends = [src.indexOf('\n  function ', start + 1), src.indexOf('\n  async function ', start + 1)].filter(
    (i) => i > -1,
  );
  return src.slice(start, ends.length ? Math.min(...ends) : undefined);
}

function swapButton(): string {
  for (const m of src.matchAll(/<button[\s\S]*?<\/button>/g)) {
    if (m[0].includes('onclick={swap}')) return m[0];
  }
  return '';
}

describe('translate window swap', () => {
  const swap = () => fnBody('swap');

  it('imports and uses the swapLanguages helper', () => {
    expect(src).toContain("from '../utils/swapLangs.ts'");
    expect(swap()).toMatch(/swapLanguages\(|swapPair/);
  });

  it('no longer hardcodes Chinese as the auto fallback', () => {
    expect(swap()).not.toContain('TRANSLATE_LANG.ZH');
  });

  it('moves the displayed result text into the input', () => {
    expect(swap()).toMatch(/setSource\(\s*activeDisplay\s*,\s*'program'\s*\)/);
  });

  it('triggers the reverse translation through the existing doTranslate', () => {
    expect(swap()).toMatch(/doTranslate\(\)/);
  });

  it('persists the new pair and never teaches', () => {
    expect(swap()).toContain('persistLangs()');
    expect(swap()).not.toContain('learnLangVariant');
    expect(swap()).not.toContain('learnFromSelection');
  });

  it('the swap button is disabled when no swap is possible, keeping label and title', () => {
    const btn = swapButton();
    expect(btn).not.toBe('');
    expect(btn).toMatch(/disabled=\{/);
    expect(btn).toMatch(/aria-label=\{t\('translate\.swap'\)\}/);
    expect(btn).toMatch(/title=\{t\('translate\.swap'\)\}/);
  });

  it('moves the result into the input only when there is text on screen', () => {
    expect(swap()).toMatch(
      /if\s*\(\s*activeDisplay\s*!==\s*''\s*\)\s*setSource\(\s*activeDisplay\s*,\s*'program'\s*\)/,
    );
  });

  it('swapPair gates the detection on the loaded target list and engine capability (#52)', () => {
    const m = src.match(/const swapPair = \$derived\([\s\S]*?\n {2}\);/);
    expect(m, 'swapPair derivation not found').not.toBeNull();
    expect(m![0]).toContain('targetLanguages');
    expect(m![0]).toContain('isTargetDisabled');
    expect(m![0]).not.toMatch(/isSelectable:\s*\(\w*\)\s*=>\s*true/);
  });
});
