// 语言码统一常量：前端复用后端 model 的 enum 定义（wails 自动生成），
// 避免在各组件散落 'auto' / 'zh-CN' / 'en-US' 等裸字符串，便于前后端同步修改。
//
// 两套语义（互不混用，这是翻译软件的本质约束）：
//  - Translation language (TranslateLang / TRANSLATE_LANG): the engines' source/target language,
//    incl. auto (auto-detect); aligned with the backend model.Language (auto/zh/en/ja/es-MX/
//    pt-BR/...), fully independent of the UI language. The RECOGNIZED set (the Language enum,
//    incl. the bare es / pt bases that detection and aliasing use) is larger than the SELECTABLE
//    set: dropdowns only get SelectableLanguages(), never bare es / pt.
//  - 界面语言（Lang）：应用界面显示语言（auto / zh-CN / en-US，auto 由系统语言解析），
//    对齐后端 settings.Language，与主题（theme）平级，就叫"语言"不额外加前缀。

import { Language } from '@bindings/cnb.cool/dtapp/kai/internal/model/models.ts';

// 翻译语言别名，组件里用 TRANSLATE_LANG.Auto / TRANSLATE_LANG.ZH / TRANSLATE_LANG.EN 等。
export const TRANSLATE_LANG = Language;

// A translation language code (any recognized value, incl. auto and the bare es / pt bases).
export type TranslateLang = Language;

// 界面语言码（auto / zh-CN / en-US），对齐后端 settings.Language。就叫"语言"，不加 UI 前缀。
export type LangCode = 'auto' | 'zh-CN' | 'en-US';
// 界面语言码常量。组件里用 Lang.Auto / Lang.ZHCN / Lang.ENUS。
export const Lang = {
  Auto: 'auto',
  ZHCN: 'zh-CN',
  ENUS: 'en-US',
} as const;

// The SELECTABLE translation languages (incl. auto), for dropdowns to iterate directly. Mirrors
// the backend model.SelectableLanguages() (same order, dialects adjacent to their family): bare
// es / pt are recognized but deliberately not offered. This static list is only the offline
// fallback — the language bar loads the same set from the backend (ConfigWrapper.GetLanguages);
// which engine can translate into which option is backend-owned too (AllEngineItem.target_languages).
export const ALL_TRANSLATE_LANGS: TranslateLang[] = [
  Language.Auto,
  Language.EN,
  Language.JA,
  Language.KO,
  Language.FR,
  Language.DE,
  Language.ESMX,
  Language.PTBR,
  Language.PTPT,
  Language.RU,
  // Chinese last, matching the backend order (principal, 2026-09-26).
  Language.ZH,
];

// 目标翻译语言需排除 auto（引擎不支持自动检测目标语言）。
export const TARGET_TRANSLATE_LANGS: TranslateLang[] = ALL_TRANSLATE_LANGS.filter(
  (c) => c !== Language.Auto,
);
