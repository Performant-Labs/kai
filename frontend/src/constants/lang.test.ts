import { describe, expect, it } from 'vitest';
import { ALL_TRANSLATE_LANGS, TARGET_TRANSLATE_LANGS } from './lang.ts';
import { Language } from '@bindings/cnb.cool/dtapp/kai/internal/model/models.ts';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// issue #52 (Tester, RED): the static fallback lists mirror backend SelectableLanguages()
// (recognized bare es/pt are NOT selectable); display names exist in en + zh.

const SELECTABLE = ['auto', 'zh', 'en', 'ja', 'ko', 'fr', 'de', 'es-MX', 'pt-BR', 'pt-PT', 'ru'];

describe('static language lists', () => {
  it('equals the generated Language enum minus $zero, bare es and pt', () => {
    const derived = Object.values(Language).filter((c) => c !== '' && c !== 'es' && c !== 'pt');
    expect([...ALL_TRANSLATE_LANGS]).toEqual(derived);
  });

  it('ALL_TRANSLATE_LANGS mirrors backend SelectableLanguages() order', () => {
    expect([...ALL_TRANSLATE_LANGS]).toEqual(SELECTABLE);
  });

  it('TARGET_TRANSLATE_LANGS is the same minus auto', () => {
    expect([...TARGET_TRANSLATE_LANGS]).toEqual(SELECTABLE.filter((c) => c !== 'auto'));
  });
});

// langName(code) resolves `lang.<code>` in the active dict (index.svelte.ts needs the svelte
// compiler for runes, so the dicts are asserted directly).
const enLang = en.lang as Record<string, string>;
const zhLang = zh.lang as Record<string, string>;

describe('lang display names', () => {
  it('en: variants named, bare labels kept for detection', () => {
    expect(enLang['es-MX']).toBe('Spanish (Mexico)');
    expect(enLang['pt-BR']).toBe('Portuguese (Brazil)');
    expect(enLang['pt-PT']).toBe('Portuguese (Portugal)');
    expect(enLang['es']).toBe('Spanish');
    expect(enLang['pt']).toBe('Portuguese');
  });

  it('zh: variants named', () => {
    expect(zhLang['es-MX']).toBe('西班牙语（墨西哥）');
    expect(zhLang['pt-BR']).toBe('葡萄牙语（巴西）');
    expect(zhLang['pt-PT']).toBe('葡萄牙语（葡萄牙）');
  });
});
