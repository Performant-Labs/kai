import { writable, get } from 'svelte/store';
import { onEvent } from '../runtime';
import { EventLocaleChanged, type LocaleChangedPayload } from '../utils/events';
import { locale, setLocale, resolveLang, type LangCode } from '../i18n';
import { Lang } from '../constants/lang';
import { GetConfig } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';

export const activeWindow = writable<string>('');

// userLang holds the user-selected UI language mode (auto/zh-CN/en-US), distinct from
// locale (the resolved, actually effective language). The settings dropdown must bind
// userLang, otherwise auto users would be shown as the resolved concrete language.
export const userLang = writable<LangCode>(Lang.Auto);

let unregister: Array<() => void> = [];

export async function initWindow(): Promise<void> {
  try {
    // Read the UI language config (Settings.Language is the UI language: auto/zh-CN/en-US).
    const cfg = await GetConfig();
    if (cfg?.language) {
      userLang.set(cfg.language as LangCode);
      setLocale(resolveLang(cfg.language as LangCode));
    }
  } catch {
    // ignore
  }

  unregister.forEach((u) => u());
  unregister = [];
  unregister.push(
    onEvent(EventLocaleChanged, (payload: LocaleChangedPayload) => {
      // Language is the UI language mode (auto/zh-CN/en-US).
      if (payload?.language) {
        userLang.set(payload.language as LangCode);
        setLocale(resolveLang(payload.language as LangCode));
      }
    }),
  );
}

export function currentLang(): LangCode {
  return get(locale);
}
