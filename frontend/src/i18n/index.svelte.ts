import { writable } from 'svelte/store';
import type { Dict } from './keys';
import { zh } from './zh-CN';
import { en } from './en-US';
import { Lang, type LangCode } from '../constants/lang';

// Re-export the LangCode type (UI language: auto/zh-CN/en-US) for index.ts and components to reuse.
export type { LangCode };

// The actually effective i18n language can only be zh-CN/en-US (auto is resolved by
// resolveLang), so dicts has exactly two keys.
type ResolvedLang = typeof Lang.ZHCN | typeof Lang.ENUS;
const dicts: Record<ResolvedLang, Dict> = { [Lang.ZHCN]: zh, [Lang.ENUS]: en };

// Hold the current language in runes state so t() refreshes reactively in any component.
let currentLang = $state<ResolvedLang>(Lang.ZHCN);

// Backwards compatibility with the old writable usage (ui.ts etc. still operate via setLocale/get).
const localeStore = writable<ResolvedLang>(Lang.ZHCN);
localeStore.subscribe((l) => {
  if (l !== currentLang) currentLang = l;
});

export const locale = localeStore;

type Path = string;

function lookup(dict: unknown, path: string): string {
  const parts = path.split('.');
  let cur: any = dict;
  for (const p of parts) {
    if (cur == null) return path;
    cur = cur[p];
  }
  return typeof cur === 'string' ? cur : path;
}

export function t(path: Path, params?: Record<string, string | number>): string {
  let str = lookup(dicts[currentLang], path);
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      str = str.replace(new RegExp(`\\{${k}\\}`, 'g'), String(v));
    }
  }
  return str;
}

export function getSystemLocale() {
  const lang = navigator.language || '';
  return lang.startsWith('zh') ? Lang.ZHCN : Lang.ENUS;
}

// Resolve the UI language (Lang: auto/zh-CN/en-US) into the actually effective language (zh-CN/en-US).
// For auto, follow the system language (getSystemLocale) instead of the backend system-language API.
export function resolveLang(lang: LangCode): typeof Lang.ZHCN | typeof Lang.ENUS {
  if (lang === Lang.ZHCN || lang === Lang.ENUS) return lang;
  return getSystemLocale();
}

export function setLocale(lang: LangCode): void {
  const resolved = resolveLang(lang);
  currentLang = resolved;
  localeStore.set(resolved);
}

export function langName(code: string): string {
  if (!code) return '';
  const key = `lang.${code.trim()}`;
  const val = lookup(dicts[currentLang], key);
  return val === key ? code : val;
}

export function engineName(code: string): string {
  if (!code) return '';
  const key = `engine.${code.trim().toLowerCase()}`;
  const val = lookup(dicts[currentLang], key);
  return val === key ? code : val;
}
