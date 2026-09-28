import { describe, expect, it } from 'vitest';
import { ALL_TRANSLATE_LANGS } from './lang.ts';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// #43 end-to-end acceptance (Tester, RED phase), frontend half: the real display dictionaries
// (not injected fakes) drive the language bar. (#161 retired the detected-label half, #11's
// relabeling of the Auto entry: it now always reads translate.sourceAuto.)

const enLang = en.lang as Record<string, string>;
const zhLang = zh.lang as Record<string, string>;

describe('every selectable language has a display name in both locales', () => {
  it('en-US and zh-CN name every option, and each variant differs from its base', () => {
    for (const code of ALL_TRANSLATE_LANGS) {
      expect(enLang[code], `en-US lang.${code}`).toBeTruthy();
      expect(zhLang[code], `zh-CN lang.${code}`).toBeTruthy();
    }
    for (const [variant, base] of [['es-MX', 'es'], ['pt-BR', 'pt'], ['pt-PT', 'pt']]) {
      expect(enLang[variant]).not.toBe(enLang[base]);
      expect(zhLang[variant]).not.toBe(zhLang[base]);
    }
    expect(enLang['pt-BR']).not.toBe(enLang['pt-PT']);
    expect(zhLang['pt-BR']).not.toBe(zhLang['pt-PT']);
  });

  it('bare es/pt are not options but keep names so a detected base still labels', () => {
    expect(ALL_TRANSLATE_LANGS).not.toContain('es');
    expect(ALL_TRANSLATE_LANGS).not.toContain('pt');
    expect(enLang['es'] && enLang['pt'] && zhLang['es'] && zhLang['pt']).toBeTruthy();
  });
});
