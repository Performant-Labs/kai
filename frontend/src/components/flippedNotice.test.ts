import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// Issue #44 (PR-Agent finding): the flipped-target notice is new UI in both windows, but only the
// pure helper (utils/flippedTarget.ts) had a test, so deleting the notice markup failed nothing.
// The repo has no Svelte component-render harness (#28, #78), so this pins the markup contract from
// the component sources: the notice exists where it must, is display-only, and is localised.

const read = (f: string) => readFileSync(resolve(__dirname, f), 'utf8');
const card = read('TranslateCard.svelte');
const win = read('TranslateWindow.svelte');
const shot = read('ScreenshotWindow.svelte');

// The {#if flippedLabel} ... {/if} block.
function notice(src: string): string {
  const m = src.match(/\{#if flippedLabel\}[\s\S]*?\{\/if\}/);
  return m ? m[0] : '';
}

describe('flipped-target notice markup', () => {
  for (const [name, src] of [
    ['TranslateCard (screenshot window results)', card],
    ['TranslateWindow (input window result)', win],
  ] as const) {
    it(`${name} renders the notice with the localised text`, () => {
      const block = notice(src);
      expect(block).not.toBe('');
      expect(block).toMatch(/data-testid="flipped-target"/);
      expect(block).toMatch(/t\('translate\.flippedTo', \{ lang: flippedLabel \}\)/);
    });
  }

  it('the card only computes a label for a card that has a result', () => {
    expect(card).toMatch(/tr\?\.result \? flippedTargetLabel\(/);
  });

  it('the screenshot window passes the requested target to every card', () => {
    expect(shot).toMatch(/<TranslateCard[\s\S]*?requestedTo=\{result\.to\}/);
  });

  it('the input window records the requested target when it sends', () => {
    expect(win).toMatch(/requestedTo = toLang;/);
  });
});

describe('flipped-target notice is display only', () => {
  it('never assigns the target select from a result or from the flip label', () => {
    for (const [name, src] of [['TranslateWindow', win], ['ScreenshotWindow', shot], ['TranslateCard', card]] as const) {
      // toLang / fromLang assigned from a result's `.to`, or from the label, would move the select
      // (and in ScreenshotWindow re-emit EventScreenshotRetranslate).
      expect(src, name).not.toMatch(/\btoLang\s*=\s*[^;\n]*(\.to\b|flippedLabel)/);
      expect(src, name).not.toMatch(/\bfromLang\s*=\s*[^;\n]*flippedLabel/);
    }
  });
});

describe('flipped-target notice text', () => {
  it('has distinct en-US and zh-CN strings that both take the language placeholder', () => {
    const enT = (en.translate as Record<string, string>).flippedTo;
    const zhT = (zh.translate as Record<string, string>).flippedTo;
    expect(enT).toContain('{lang}');
    expect(zhT).toContain('{lang}');
    expect(zhT).not.toBe(enT);
  });
});
