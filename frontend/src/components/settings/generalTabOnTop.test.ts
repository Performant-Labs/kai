import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { en } from '../../i18n/en-US.ts';
import { zh } from '../../i18n/zh-CN.ts';

// Issue #69 + #113: the REAL GeneralTab.svelte, mounted, with only the generated bindings and
// the Wails runtime faked. The "keep the translate window on top of other apps" switch is off by
// default, mirrors the config field, and saves through SaveConfig without dropping other fields.
// Since #113, toggling it also broadcasts kai:translate-window-on-top:changed so an OPEN
// translate window picks the change up (AC7, the EventEnginesChanged settings->translate shape).

const h = vi.hoisted(() => ({
  config: { language: 'en-US', theme: 'auto' } as Record<string, unknown>,
  saveConfig: vi.fn(),
  emit: vi.fn(),
}));

vi.mock('@wailsio/runtime', () => ({
  Events: { On: () => () => {}, Emit: (name: string, ...rest: unknown[]) => h.emit(name, ...rest) },
  Window: {},
  Clipboard: { SetText: vi.fn(async () => {}) },
  System: { IsDarkMode: async () => false },
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetConfig: async () => ({ ...h.config }),
  SaveConfig: async (c: Record<string, unknown>) => {
    await h.saveConfig(c);
    h.config = { ...c };
  },
  GetTheme: async () => 'light',
  SetTheme: async () => {},
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/appservice.ts', () => ({
  GetVersion: async () => '0.5.0',
  IsDevBuild: async () => false,
}));
vi.mock('../../utils/analytics', () => ({ track: vi.fn() }));

import GeneralTab from './GeneralTab.svelte';

let app: Record<string, unknown> | undefined;
let target: HTMLElement;
const settle = async () => {
  for (let i = 0; i < 6; i++) {
    await Promise.resolve();
    await new Promise((r) => setTimeout(r, 0));
  }
  flushSync();
};
const toggle = () =>
  target.querySelector(
    `label[aria-label="${en.settings.translateWindowOnTop}"] input`,
  ) as HTMLInputElement;

async function mountTab() {
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(GeneralTab, { target, props: { curLang: 'en-US' } });
  await settle();
}

beforeEach(() => {
  h.config = { language: 'en-US', theme: 'auto' };
  h.saveConfig.mockReset();
  h.saveConfig.mockResolvedValue(undefined);
  h.emit.mockReset();
});
afterEach(() => {
  if (app) unmount(app);
  app = undefined;
  target?.remove();
});

describe('GeneralTab translate-window on-top switch, mounted', () => {
  it('renders the row with the en label and hint, off by default (config has no value)', async () => {
    await mountTab();
    expect(toggle()).not.toBeNull();
    expect(toggle().checked).toBe(false);
    const label = toggle().closest('label')!;
    expect(label.getAttribute('title')).toBe(en.settings.translateWindowOnTopHint);
    expect(target.textContent).toContain(en.settings.translateWindowOnTop);
    expect(target.textContent).toContain(en.settings.translateWindowOnTopHint);
  });

  it('is on when the config says on', async () => {
    h.config.translate_window_on_top = true;
    await mountTab();
    expect(toggle().checked).toBe(true);
  });

  it('switching it on saves translate_window_on_top: true and keeps the rest of the config', async () => {
    await mountTab();
    toggle().click();
    await settle();
    expect(h.saveConfig).toHaveBeenCalledTimes(1);
    expect(h.saveConfig.mock.calls[0][0]).toMatchObject({
      language: 'en-US',
      theme: 'auto',
      translate_window_on_top: true,
    });
  });

  it('switching it off saves translate_window_on_top: false', async () => {
    h.config.translate_window_on_top = true;
    await mountTab();
    toggle().click();
    await settle();
    expect(h.saveConfig.mock.calls[0][0]).toMatchObject({ translate_window_on_top: false });
  });

  it('#113 AC7: toggling the switch broadcasts kai:translate-window-on-top:changed', async () => {
    await mountTab();
    toggle().click();
    await settle();
    // The literal name, not the constant, on purpose: importing the not-yet-existing export
    // would module-resolution-fail this whole file at RED and mask these assertions;
    // utils/events.test.ts pins the constant to exactly this string. The payload is
    // deliberately not asserted — it is not part of the contract (the translate window
    // re-fetches the config; see events.test.ts).
    const names = h.emit.mock.calls.map((c) => c[0]);
    expect(names).toContain('kai:translate-window-on-top:changed');
  });
});

describe('translate-window on-top copy', () => {
  it('exists in both locales, differing text', () => {
    for (const k of ['translateWindowOnTop', 'translateWindowOnTopHint'] as const) {
      expect(en.settings[k], `en ${k}`).toBeTruthy();
      expect(zh.settings[k], `zh ${k}`).toBeTruthy();
      expect(en.settings[k]).not.toBe(zh.settings[k]);
    }
    expect(en.log.generalSaveTranslateWindowOnTopFailed).toBeTruthy();
    expect(zh.log.generalSaveTranslateWindowOnTopFailed).toBeTruthy();
  });
});
