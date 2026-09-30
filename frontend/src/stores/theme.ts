import { writable, derived, get } from 'svelte/store';
import { onEvent, System, Window } from '../runtime';
import { EventLocaleChanged, EventThemeChanged, type ThemeChangedPayload } from '../utils/events';
import { GetTheme, SetTheme } from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
import { THEME, type ThemeMode, type ResolvedTheme } from '../constants/theme';

export const themeMode = writable<ThemeMode>(THEME.Auto);
export const systemDark = writable<boolean>(false);

export const resolvedTheme = derived([themeMode, systemDark], ([mode, sysDark]): ResolvedTheme => {
  if (mode === THEME.Auto) return sysDark ? THEME.Dark : THEME.Light;
  return mode as ResolvedTheme;
});

export const isDark = derived(resolvedTheme, ($r) => $r === THEME.Dark);

const lightVars: Record<string, string> = {
  '--app-bg': '#ffffff',
  '--app-fg': '#1f2329',
  '--app-muted': '#8a8f99',
  '--app-border': '#e5e6eb',
  '--app-card': '#f7f8fa',
  '--app-accent': '#3b82f6',
  '--app-accent-fg': '#ffffff',
  '--app-input-bg': '#ffffff',
};

const darkVars: Record<string, string> = {
  '--app-bg': '#18181c',
  '--app-fg': '#e6e6e6',
  '--app-muted': '#8a8f99',
  '--app-border': '#2c2c32',
  '--app-card': '#232328',
  '--app-accent': '#3b82f6',
  '--app-accent-fg': '#ffffff',
  '--app-input-bg': '#232328',
};

export const rootStyle = derived(isDark, ($dark) => ($dark ? darkVars : lightVars));

let unregister: Array<() => void> = [];

export async function initTheme(): Promise<void> {
  try {
    const mode = await GetTheme();
    if (mode === THEME.Auto || mode === THEME.Light || mode === THEME.Dark)
      themeMode.set(mode as ThemeMode);
    // Use the native @wailsio/runtime API to get the system dark mode, reducing RPC calls.
    const isDarkMode = await System.IsDarkMode();
    systemDark.set(isDarkMode);
  } catch {
    // ignore
  }

  applyClass();
  applyNativeTheme();

  unregister.forEach((u) => u());
  unregister = [];
  unregister.push(
    onEvent(EventThemeChanged, (data: ThemeChangedPayload) => {
      // Single theme event: mode = user-configured mode, theme = actual system appearance.
      // Non-auto uses mode; auto uses theme to follow the system.
      if (data?.mode === THEME.Auto || data?.mode === THEME.Light || data?.mode === THEME.Dark) {
        themeMode.set(data.mode as ThemeMode);
      }
      if (data?.theme === THEME.Dark || data?.theme === THEME.Light) {
        systemDark.set(data.theme === THEME.Dark);
      }
      applyClass();
      applyNativeTheme();
    }),
  );
  unregister.push(onEvent(EventLocaleChanged, () => applyClass()));
}

// Issue #15: the pages' inline <head> script reads this cache to pick the colour before the first
// paint. The store stays the source of truth: it rewrites the cache on every theme change.
export const THEME_CACHE_MODE_KEY = 'kai.theme.mode';
export const THEME_CACHE_RESOLVED_KEY = 'kai.theme.resolved';

function writeThemeCache(): void {
  try {
    localStorage.setItem(THEME_CACHE_MODE_KEY, get(themeMode));
    localStorage.setItem(THEME_CACHE_RESOLVED_KEY, get(resolvedTheme));
  } catch {
    // localStorage can be unavailable; the cache is only an optimisation.
  }
}

function applyClass(): void {
  const dark = get(isDark);
  document.documentElement.classList.toggle('dark', dark);
  document.documentElement.style.backgroundColor = dark ? darkVars['--app-bg'] : lightVars['--app-bg'];
  writeThemeCache();
}

export async function setTheme(mode: ThemeMode): Promise<void> {
  themeMode.set(mode);
  applyClass();
  try {
    await SetTheme(mode);
  } catch {
    // ignore
  }
}

// Applies the in-app theme to the native title bar (macOS traffic lights/title, Windows title bar).
// Note: Wails beta.15 declares no public API (neither Go nor frontend runtime) for switching the
// theme of a persistent window at runtime, so this tries the window.runtime.Window theme methods:
// if the runtime supports them it follows in real time; if not, it silently skips (the initial
// Theme set at window creation is the fallback).
function applyNativeTheme(): void {
  const mode = get(themeMode);
  const w = Window as unknown as {
    SetSystemDefaultTheme?: () => void | Promise<void>;
    SetDarkTheme?: () => void | Promise<void>;
    SetLightTheme?: () => void | Promise<void>;
  };
  try {
    if (mode === THEME.Auto) {
      w.SetSystemDefaultTheme?.();
    } else if (mode === THEME.Dark) {
      w.SetDarkTheme?.();
    } else if (mode === THEME.Light) {
      w.SetLightTheme?.();
    }
  } catch {
    // ignore
  }
}
