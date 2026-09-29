import { get, writable } from 'svelte/store';
import { onEvent } from '../runtime';
import { EventFontSizeChanged } from '../utils/events';
import {
  GetConfig,
  SaveConfig,
} from '@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts';
import { t } from '../i18n';
import { DEFAULT_FONT_SIZE, normalizeFontSize } from '../constants/fontSize';

// Text size (issue #195). The size is a CSS variable on <html>, --kai-text-scale (1.2 = 120%);
// app.css multiplies every text size by it and leaves spacing, widths and icons alone.
// applyFontSize is the one place that writes it. Each window starts initFontSize (like initTheme)
// and follows the backend's EventFontSizeChanged, so a change in Settings reaches all windows live.

export const fontSize = writable<number>(DEFAULT_FONT_SIZE);

const CACHE_KEY = 'kai:fontSize';

// Applies a size (garbled values become the default) and returns the size applied.
export function applyFontSize(raw: unknown): number {
  const pct = normalizeFontSize(raw);
  document.documentElement.style.setProperty('--kai-text-scale', String(pct / 100));
  fontSize.set(pct);
  try {
    localStorage.setItem(CACHE_KEY, JSON.stringify(pct));
  } catch {
    /* the cache only avoids a first-paint flash; the backend stays the source of truth */
  }
  return pct;
}

let unregister: (() => void) | undefined;

export async function initFontSize(): Promise<void> {
  // Apply the last known size before the first await, so a saved size does not flash the default.
  let cached: unknown;
  try {
    cached = JSON.parse(localStorage.getItem(CACHE_KEY) ?? 'null');
  } catch {
    cached = undefined;
  }
  applyFontSize(cached);

  unregister?.();
  unregister = onEvent(EventFontSizeChanged, (data) => {
    applyFontSize(data);
  });

  try {
    const cfg = await GetConfig();
    applyFontSize(cfg?.font_size);
  } catch {
    /* keep what is applied; the default is the safe fallback */
  }
}

// Applies a size at once, then persists it; the backend broadcasts it to the other windows.
// A failed save puts the previous size back.
export async function saveFontSize(raw: unknown): Promise<void> {
  const previous = get(fontSize);
  const pct = applyFontSize(raw);
  try {
    const cfg = (await GetConfig()) ?? ({} as Record<string, unknown>);
    await SaveConfig({ ...cfg, font_size: pct } as never);
  } catch (e) {
    console.error(t('log.generalSaveFontSizeFailed'), e);
    applyFontSize(previous);
  }
}
