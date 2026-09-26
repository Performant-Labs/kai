import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { learnFromSelection } from './langLearn.ts';

// issue #53: an explicit language selection must reach the backend preference store. The selects
// live in two Svelte windows that call the Wails binding directly, and this repo has no component
// test infrastructure (mounting them would need a mocked Wails runtime, which epic #6 forbids), so
// the contract is pinned in two parts: the shared helper's behavior, and the wiring in the sources.

describe('learnFromSelection', () => {
  it('passes the picked value to the learn function, exactly once', async () => {
    const seen: string[] = [];
    await learnFromSelection(
      'es-MX',
      async (lang) => {
        seen.push(lang);
      },
      () => {},
    );
    expect(seen).toEqual(['es-MX']);
  });

  it('reports a failing learn call through onError and does not throw', async () => {
    const boom = new Error('binding failed');
    const errors: unknown[] = [];
    await expect(
      learnFromSelection(
        'pt-BR',
        async () => {
          throw boom;
        },
        (e) => errors.push(e),
      ),
    ).resolves.toBeUndefined();
    expect(errors).toEqual([boom]);
  });
});

const read = (rel: string) => readFileSync(resolve(__dirname, '..', rel), 'utf8');

// Source-level wiring contract. Deliberately blunt: it fails when a window stops calling the
// helper from a select's change handler, or starts teaching from a swap or a saved default
// ("swap consumes, never teaches").
describe('language select wiring', () => {
  const fnBody = (src: string, name: string): string => {
    const start = src.indexOf(`function ${name}(`);
    expect(start, `${name} not found`).toBeGreaterThan(-1);
    const next = src.indexOf('\n  function ', start + 1);
    const nextAsync = src.indexOf('\n  async function ', start + 1);
    const ends = [next, nextAsync].filter((i) => i > -1);
    return src.slice(start, ends.length ? Math.min(...ends) : undefined);
  };

  it('TranslateWindow: both selects use onLangPicked, which teaches through the helper', () => {
    const src = read('components/TranslateWindow.svelte');
    expect(src).toContain("from '../utils/langLearn.ts'");
    expect(src.match(/onchange=\{onLangPicked\}/g)?.length).toBe(2);
    expect(fnBody(src, 'onLangPicked')).toContain('learnLangVariant(');
  });

  it('ScreenshotWindow: the source and target selects teach; swapping and defaults do not', () => {
    const src = read('components/ScreenshotWindow.svelte');
    expect(src).toContain("from '../utils/langLearn.ts'");
    expect(src).toContain('learnLangVariant(fromLang)');
    expect(src).toContain('learnLangVariant(toLang)');
    expect(src.match(/learnLangVariant\(/g)?.length).toBe(3); // 2 call sites + the definition
    expect(fnBody(src, 'swapLangs')).not.toContain('learnLangVariant');
  });

  it('TranslateWindow: swapping and loading defaults do not teach', () => {
    const src = read('components/TranslateWindow.svelte');
    expect(src.match(/learnLangVariant\(/g)?.length).toBe(2); // onLangPicked + the definition
  });
});
