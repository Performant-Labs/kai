import { describe, expect, it } from 'vitest';
import { ALL_TRANSLATE_LANGS } from './lang.ts';
import { detectedSourceLabel } from '../utils/detectedLang.ts';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// #43 end-to-end acceptance (Tester, RED phase), frontend half: the real display dictionaries
// (not injected fakes) drive the language bar and the detected label.

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

describe('detected label with the real dictionaries', () => {
  const label = (d: Record<string, string>, suffix: string, detected: string) =>
    detectedSourceLabel('auto', 'auto', detected, (c) => d[c] ?? c, suffix);

  it('en-US: a detected bare base and a qualified variant both label correctly', () => {
    expect(label(enLang, en.translate.detected, 'es')).toBe('Spanish (detected)');
    expect(label(enLang, en.translate.detected, 'pt')).toBe('Portuguese (detected)');
    expect(label(enLang, en.translate.detected, 'es-MX')).toBe('Spanish (Mexico) (detected)');
  });

  it('zh-CN: same, with the localized suffix', () => {
    expect(label(zhLang, zh.translate.detected, 'es')).toBe('西班牙语（已检测）');
    expect(label(zhLang, zh.translate.detected, 'pt-BR')).toBe('葡萄牙语（巴西）（已检测）');
  });
});
