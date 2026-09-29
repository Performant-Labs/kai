import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// Issue #161 (T, RED), frontend half. (Issue #200 later reversed "never changes" for a pinned source:
// see autoSourceSwitch.test.ts; what stays is that the Auto entry is a constant label and this note is
// display only.) The source dropdown never changes: its Auto entry always
// reads the constant translate.sourceAuto ("Detected"), and #11's relabeling helper is deleted.
// What the engine actually translated from is shown instead by a persistent, muted note in the
// result pane, rendered from the backend-decided activeResult.detected_from whenever it is
// non-empty and the result is not an identity result. The frontend compares nothing itself, and
// the note never teaches the variant store, never persists the pair, never toasts and never
// assigns a select. The repo has no Svelte render harness (#28), so the markup and wiring are
// pinned from the component source, as identityResult.test.ts / swapWindow.test.ts do.

const read = (f: string) => readFileSync(resolve(__dirname, f), 'utf8');
const win = read('TranslateWindow.svelte');

// A top-level function's text, from `function name(` to the next top-level function.
function fnBody(name: string): string {
  const start = win.indexOf(`function ${name}(`);
  expect(start, `${name} not found`).toBeGreaterThan(-1);
  const ends = [
    win.indexOf('\n  function ', start + 1),
    win.indexOf('\n  async function ', start + 1),
    win.indexOf('\n  const ', start + 1),
    win.indexOf('\n  let ', start + 1),
    win.indexOf('\n  //', start + 1),
  ].filter((i) => i > -1);
  return win.slice(start, ends.length ? Math.min(...ends) : undefined);
}

// The {#if <cond>} ... {/if} block whose condition reads detected_from.
function noteBlock(): { cond: string; body: string; at: number } {
  const m = /\{#if ([^}]*\bdetected_from\b[^}]*)\}([\s\S]*?)\{\/if\}/.exec(win);
  return m ? { cond: m[1], body: m[2], at: m.index } : { cond: '', body: '', at: -1 };
}

const TEACH_OR_TOAST = [
  'learnLangVariant',
  'LearnLangVariant',
  'learnFromSelection',
  'persistLangs',
  'onLangPicked',
  'showToast',
  'default_from',
];

describe('source dropdown: the Auto entry is a constant label', () => {
  it('fromOptionLabel maps Auto to translate.sourceAuto and anything else to langName, whatever the result state', () => {
    const body = fnBody('fromOptionLabel');
    // Evaluate the real function with stubs. It must be a pure function of its argument: a
    // reference to result state (detectedFrom, activeResult, fromLang) is a ReferenceError here.
    const js = body.replace(
      /function fromOptionLabel\(\s*(\w+)\s*:\s*string\s*\)\s*:\s*string/,
      'function fromOptionLabel($1)',
    );
    const make = new Function('TRANSLATE_LANG', 't', 'langName', `${js}\nreturn fromOptionLabel;`);
    const fromOptionLabel = make(
      { Auto: 'auto' },
      (k: string) => `T(${k})`,
      (c: string) => `N(${c})`,
    ) as (v: string) => string;
    expect(fromOptionLabel('auto')).toBe('T(translate.sourceAuto)');
    expect(fromOptionLabel('es-MX')).toBe('N(es-MX)');
    expect(fromOptionLabel('en')).toBe('N(en)');
  });

  it('fromOptionLabel reads no detection state and no suffix key', () => {
    const body = fnBody('fromOptionLabel');
    for (const s of [
      'detectedFrom',
      'activeResult',
      'detected_from',
      'fromLang',
      "'translate.detected'",
      "'lang.auto'",
    ]) {
      expect(body, s).not.toContain(s);
    }
  });

  it('the source select still renders every option through fromOptionLabel', () => {
    expect(win).toMatch(/<option value=\{l\.value\}>\{fromOptionLabel\(l\.value\)\}<\/option>/);
  });

  it('the shared lang.auto key keeps serving the interface-language setting', () => {
    expect(read('settings/GeneralTab.svelte')).toContain("t('lang.auto')");
  });
});

describe('the relabeling helper is deleted', () => {
  // Assembled at run time so this file does not itself mention the retired names.
  const retired = [
    ['detected', 'Lang'],
    ['detected', 'SourceLabel'],
  ].map((p) => p.join(''));

  function walk(dir: string, out: string[] = []): string[] {
    for (const e of readdirSync(dir)) {
      const p = join(dir, e);
      if (statSync(p).isDirectory()) walk(p, out);
      else if (/\.(ts|svelte|js)$/.test(e)) out.push(p);
    }
    return out;
  }

  it('the module and its test are gone', () => {
    expect(existsSync(resolve(__dirname, '../utils/', `${retired[0]}.ts`))).toBe(false);
    expect(existsSync(resolve(__dirname, '../utils/', `${retired[0]}.test.ts`))).toBe(false);
  });

  it('no source under frontend/src names it (imports and the three precedent comments included)', () => {
    const self = resolve(__dirname, 'detectedSourceNote.test.ts');
    const hits: string[] = [];
    for (const f of walk(resolve(__dirname, '..'))) {
      if (f === self) continue;
      const body = readFileSync(f, 'utf8');
      for (const s of retired) if (body.includes(s)) hits.push(`${f}: ${s}`);
    }
    expect(hits).toEqual([]);
  });

  it('the superseded toast/relabel keys are not used', () => {
    for (const k of ['translate.detected', 'translate.sourceCorrected', 'translate.autoDetected']) {
      expect(win, k).not.toContain(`'${k}'`);
    }
  });
});

describe('the existing detectedFrom still feeds the swap (unchanged)', () => {
  it('detectedFrom is still derived from the active result From', () => {
    expect(win).toMatch(
      /const detectedFrom = \$derived\(String\(activeResult\?\.from \?\? ''\)\);/,
    );
  });

  it('swapPair still passes detectedFrom to swapLanguages', () => {
    const m = win.match(/const swapPair = \$derived\([\s\S]*?\n {2}\);/);
    expect(m, 'swapPair derivation not found').not.toBeNull();
    expect(m![0]).toMatch(/swapLanguages\(\{[\s\S]*\bdetectedFrom,/);
    expect(m![0]).not.toContain('detected_from');
  });
});

describe('result-pane note: translated from the auto-detected language', () => {
  it('exists, gated on detected_from and not identity', () => {
    const { cond } = noteBlock();
    expect(cond, 'no {#if …detected_from…} block').not.toBe('');
    expect(cond).toMatch(/activeResult\??\.detected_from/);
    expect(cond).toMatch(/!\s*activeResult\??\.identity/);
  });

  it('makes no frontend comparison and applies no length floor', () => {
    const { cond } = noteBlock();
    expect(cond, 'no {#if …detected_from…} block').not.toBe('');
    for (const s of [
      'fromLang',
      'toLang',
      'detectedFrom',
      'TRANSLATE_LANG',
      '.length',
      '.text',
      '===',
      '!==',
    ]) {
      expect(cond, s).not.toContain(s);
    }
  });

  it('renders the localized sentence naming the language, muted and small, with a test id', () => {
    const { body } = noteBlock();
    expect(body).toMatch(/data-testid="detected-from-note"/);
    expect(body).toMatch(
      /t\(\s*'translate\.translatedFromDetected'\s*,\s*\{\s*lang:\s*langName\(\s*activeResult\??\.detected_from\s*\)\s*,?\s*\}\s*\)/,
    );
    const cls = body.match(/class="([^"]*)"/);
    expect(cls, 'note element has no class').not.toBeNull();
    for (const c of ['u-muted', 'px-4', 'text-2xs', 'first:pt-4']) {
      expect(cls![1].split(/\s+/), c).toContain(c);
    }
  });

  it('sits first in the result pane note stack, outside and before the result textarea', () => {
    const branch = win.indexOf("pane === 'result' && activeResult");
    const { at } = noteBlock();
    expect(branch).toBeGreaterThan(-1);
    expect(at).toBeGreaterThan(branch);
    const phonetic = win.indexOf('{#if activeResult.phonetic}', branch);
    const textarea = win.indexOf('<textarea', branch);
    expect(phonetic).toBeGreaterThan(-1);
    expect(at, 'note must come before the phonetic line (first in the stack)').toBeLessThan(
      phonetic,
    );
    expect(at, 'note must come before the result textarea').toBeLessThan(textarea);
    expect(win.match(/data-testid="detected-from-note"/g) ?? []).toHaveLength(1);
  });

  it('is display only: no teaching, persisting, toast or select assignment', () => {
    const { body, cond } = noteBlock();
    expect(cond, 'no {#if …detected_from…} block').not.toBe('');
    for (const s of TEACH_OR_TOAST) {
      expect(body + cond, s).not.toContain(s);
    }
    // Nowhere in the component does detected_from reach a teach/persist/toast call or a select.
    for (const line of win.split('\n').filter((l) => l.includes('detected_from'))) {
      for (const s of TEACH_OR_TOAST) expect(line, s).not.toContain(s);
      expect(line).not.toMatch(/\b(fromLang|toLang)\s*=[^=]/);
    }
    for (const fn of ['onLangPicked', 'learnLangVariant', 'persistLangs', 'swap', 'copy']) {
      expect(fnBody(fn), fn).not.toContain('detected_from');
    }
  });

  it('is never part of the copied or displayed result text', () => {
    const m = win.match(/const activeDisplay = \$derived\([^\n]*\);/);
    expect(m, 'activeDisplay derivation not found').not.toBeNull();
    expect(m![0]).not.toContain('detected_from');
  });
});

describe('i18n', () => {
  const enT = en.translate as Record<string, string>;
  const zhT = zh.translate as Record<string, string>;

  it('the Auto entry reads "Detected" / "自动检测"', () => {
    expect(enT.sourceAuto).toBe('Detected');
    expect(zhT.sourceAuto).toBe('自动检测');
  });

  it('the note reads "Translated from auto-detected {lang}" / "译自自动识别的{lang}"', () => {
    expect(enT.translatedFromDetected).toBe('Translated from auto-detected {lang}');
    expect(zhT.translatedFromDetected).toBe('译自自动识别的{lang}');
  });

  it('translate.detected is removed from both catalogs and the Dict type', () => {
    expect('detected' in enT).toBe(false);
    expect('detected' in zhT).toBe(false);
    expect(readFileSync(resolve(__dirname, '../i18n/keys.ts'), 'utf8')).not.toMatch(
      /^\s+detected:\s*string;/m,
    );
  });

  it('the shared lang.auto is unchanged', () => {
    expect((en.lang as Record<string, string>).auto).toBe('Auto');
    expect((zh.lang as Record<string, string>).auto).toBe('自动');
  });
});
