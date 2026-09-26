import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { resolve, join } from 'node:path';
import { describe, expect, it } from 'vitest';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// Issue #80: same source and target language shows the source text plus one muted line. The repo
// has no Svelte render harness (#28, #78), so this pins the markup contract from the sources.

const read = (f: string) => readFileSync(resolve(__dirname, f), 'utf8');
const card = read('TranslateCard.svelte');
const win = read('TranslateWindow.svelte');
const shot = read('ScreenshotWindow.svelte');

// The {#if ... identity} ... {/if} block carrying the identity paragraph.
function identityBlock(src: string): string {
  const m = src.match(/\{#if [^}]*\bidentity\b[^}]*\}[\s\S]*?\{\/if\}/);
  return m ? m[0] : '';
}

describe('identity-result line markup', () => {
  for (const [name, src] of [
    ['TranslateCard (screenshot window results)', card],
    ['TranslateWindow (input window result)', win],
  ] as const) {
    it(`${name} renders the muted line with the localised text`, () => {
      const block = identityBlock(src);
      expect(block).not.toBe('');
      expect(block).toMatch(/data-testid="identity-result"/);
      expect(block).toMatch(/t\('translate\.identity'\)/);
    });
  }

  it('the card gates the line on a rendered result', () => {
    expect(card).toMatch(/\{#if [^}]*tr\?\.identity[^}]*tr\?\.result[^}]*\}|\{#if [^}]*tr\?\.result[^}]*tr\?\.identity[^}]*\}/);
  });

  it('the window line sits in the pane === result branch', () => {
    const branch = win.indexOf("pane === 'result' && activeResult");
    expect(branch).toBeGreaterThan(-1);
    const line = win.indexOf('data-testid="identity-result"');
    expect(line).toBeGreaterThan(branch);
    // and before the next top-level branch of the pane chain
    const after = win.slice(branch).search(/\{:else/);
    if (after > -1) expect(line - branch).toBeLessThan(after);
  });

  it('is display only: nothing assigns the selects from an identity flag', () => {
    for (const [name, src] of [['TranslateWindow', win], ['ScreenshotWindow', shot], ['TranslateCard', card]] as const) {
      expect(src, name).not.toMatch(/\b(toLang|fromLang)\s*=\s*[^;\n]*identity/);
    }
  });
});

describe('identity copy', () => {
  it('has the agreed en-US and zh-CN strings', () => {
    expect((en.translate as Record<string, string>).identity).toBe(
      'Source and target language are the same — showing the source text',
    );
    expect((zh.translate as Record<string, string>).identity).toBe('源语言与目标语言相同，已显示原文');
  });
});

// The retired #44 flip. Symbol names are assembled at run time so this file does not contain them.
describe('flip removal sweep', () => {
  const symbols = [
    ['flipped', 'To'],
    ['flipped', 'Target'],
    ['flipped', 'Label'],
    ['flipped', '-target'],
  ].map((p) => p.join(''));

  function walk(dir: string, out: string[] = []): string[] {
    for (const e of readdirSync(dir)) {
      const p = join(dir, e);
      if (statSync(p).isDirectory()) walk(p, out);
      else if (/\.(ts|svelte|js)$/.test(e)) out.push(p);
    }
    return out;
  }

  it('no source under frontend/src mentions a flip symbol', () => {
    const src = resolve(__dirname, '..');
    const self = resolve(__dirname, 'identityResult.test.ts');
    const hits: string[] = [];
    for (const f of walk(src)) {
      if (f === self) continue;
      const body = readFileSync(f, 'utf8');
      for (const s of symbols) if (body.includes(s)) hits.push(`${f}: ${s}`);
    }
    expect(hits).toEqual([]);
  });

  it('the flip helper module is gone', () => {
    expect(existsSync(resolve(__dirname, '../utils/', ['flipped', 'Target.ts'].join('')))).toBe(false);
  });

  it('TranslateCard takes no requestedTo prop and ScreenshotWindow passes none', () => {
    expect(card).not.toMatch(/requestedTo/);
    expect(shot).not.toMatch(/requestedTo/);
  });
});
