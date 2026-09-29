import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { en } from '../../i18n/en-US.ts';
import { zh } from '../../i18n/zh-CN.ts';

// Issue #195: the REAL GeneralTab.svelte, mounted, with the real font-size store and the real
// documentElement. Only the generated bindings and the Wails runtime are faked.

const h = vi.hoisted(() => ({
  config: { language: 'en-US', font_size: 120 } as Record<string, unknown>,
  saveConfig: vi.fn(),
}));

vi.mock('@wailsio/runtime', () => ({
  Events: { On: () => () => {}, Emit: vi.fn() },
  Window: {},
  System: { IsDarkMode: async () => false },
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetConfig: async () => ({ ...h.config }),
  SaveConfig: async (c: Record<string, unknown>) => {
    await h.saveConfig(c);
    h.config = { ...c };
  },
  GetDoubleCopyStatus: async () => 'off',
  GetTheme: async () => 'light',
  SetTheme: async () => {},
}));
vi.mock('../../utils/analytics', () => ({ track: vi.fn() }));

import GeneralTab from './GeneralTab.svelte';
import { applyFontSize } from '../../stores/fontSize';

let app: Record<string, unknown> | undefined;
let target: HTMLElement;
const settle = async () => {
  for (let i = 0; i < 6; i++) {
    await Promise.resolve();
    await new Promise((r) => setTimeout(r, 0));
  }
  flushSync();
};
const q = (id: string) => target.querySelector(`[data-testid="${id}"]`) as HTMLButtonElement;
const shown = () => q('font-size-value').textContent!.trim();
const scale = () => Number(document.documentElement.style.getPropertyValue('--kai-text-scale'));

async function mountTab(size = 120) {
  h.config = { language: 'en-US', theme: 'auto', font_size: size };
  applyFontSize(size);
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(GeneralTab, { target, props: { curLang: 'en-US' } });
  await settle();
}

beforeEach(() => {
  h.saveConfig.mockReset();
  h.saveConfig.mockResolvedValue(undefined);
});
afterEach(() => {
  if (app) unmount(app);
  app = undefined;
  target?.remove();
});

describe('GeneralTab text size control, mounted', () => {
  it('shows the current percentage, 120% by default', async () => {
    await mountTab();
    expect(shown()).toBe('120%');
  });

  it('larger saves the next step, shows it and applies it to the document at once', async () => {
    await mountTab();
    q('font-size-larger').click();
    await settle();
    expect(h.saveConfig).toHaveBeenCalledTimes(1);
    expect(h.saveConfig.mock.calls[0][0]).toMatchObject({
      language: 'en-US',
      theme: 'auto',
      font_size: 140,
    });
    expect(shown()).toBe('140%');
    expect(scale()).toBeCloseTo(1.4, 5);
  });

  it('smaller saves the previous step', async () => {
    await mountTab();
    q('font-size-smaller').click();
    await settle();
    expect(h.saveConfig.mock.calls[0][0]).toMatchObject({ font_size: 100 });
    expect(shown()).toBe('100%');
    expect(scale()).toBeCloseTo(1.0, 5);
  });

  it('walks the whole ladder and stops: smaller is disabled at 80, larger at 180', async () => {
    await mountTab(100);
    expect(q('font-size-smaller').disabled).toBe(false);
    q('font-size-smaller').click();
    await settle();
    expect(shown()).toBe('80%');
    expect(q('font-size-smaller').disabled).toBe(true);
    expect(q('font-size-larger').disabled).toBe(false);
    for (let i = 0; i < 5; i++) {
      q('font-size-larger').click();
      await settle();
    }
    expect(shown()).toBe('180%');
    expect(q('font-size-larger').disabled).toBe(true);
    expect(q('font-size-smaller').disabled).toBe(false);
    expect(h.saveConfig).toHaveBeenCalledTimes(6);
    expect(scale()).toBeCloseTo(1.8, 5);
  });

  it('a disabled button saves nothing', async () => {
    await mountTab(180);
    q('font-size-larger').click();
    await settle();
    expect(h.saveConfig).not.toHaveBeenCalled();
  });

  it('reset goes back to 120 and saves it; it is disabled when already there', async () => {
    await mountTab(160);
    expect(q('font-size-reset').disabled).toBe(false);
    q('font-size-reset').click();
    await settle();
    expect(h.saveConfig.mock.calls[0][0]).toMatchObject({ font_size: 120 });
    expect(shown()).toBe('120%');
    expect(scale()).toBeCloseTo(1.2, 5);
    expect(q('font-size-reset').disabled).toBe(true);
  });

  it('the buttons carry the #165 tooltip (instant data-tooltip plus title and aria-label)', async () => {
    await mountTab();
    const want: Record<string, string> = {
      'font-size-smaller': en.settings.fontSizeSmaller,
      'font-size-larger': en.settings.fontSizeLarger,
      'font-size-reset': en.settings.fontSizeReset,
    };
    for (const [id, text] of Object.entries(want)) {
      const b = q(id);
      expect(b.classList.contains('u-tooltip'), id).toBe(true);
      expect(b.getAttribute('data-tooltip'), id).toBe(text);
      expect(b.getAttribute('title'), id).toBe(text);
      expect(b.getAttribute('aria-label'), id).toBe(text);
    }
  });

  it('sits with the language and theme controls, before the analytics switch', async () => {
    await mountTab();
    const html = target.innerHTML;
    expect(html.indexOf('font-size-value')).toBeGreaterThan(html.indexOf('lang-sel'));
    expect(html.indexOf('font-size-value')).toBeLessThan(html.indexOf('u-switch'));
  });
});

describe('text size copy', () => {
  it('exists in both locales, differing text', () => {
    for (const k of [
      'fontSize',
      'fontSizeHint',
      'fontSizeSmaller',
      'fontSizeLarger',
      'fontSizeReset',
    ] as const) {
      expect(en.settings[k], `en ${k}`).toBeTruthy();
      expect(zh.settings[k], `zh ${k}`).toBeTruthy();
      expect(en.settings[k]).not.toBe(zh.settings[k]);
    }
    expect(en.log.generalSaveFontSizeFailed).toBeTruthy();
    expect(zh.log.generalSaveFontSizeFailed).toBeTruthy();
  });
});
