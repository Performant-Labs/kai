import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { en } from '../../i18n/en-US.ts';
import { zh } from '../../i18n/zh-CN.ts';

// Issue #199: the REAL GeneralTab.svelte, mounted, with only the generated bindings faked. The
// double-copy switch is off by default, saves through SaveConfig without touching other fields,
// and shows an actionable Input Monitoring message when the backend says the permission is missing.

const h = vi.hoisted(() => ({
  config: { language: 'en-US', double_copy_translate: false } as Record<string, unknown>,
  status: 'off',
  saveConfig: vi.fn(),
  getStatus: vi.fn(),
}));

vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetConfig: async () => ({ ...h.config }),
  SaveConfig: (c: unknown) => h.saveConfig(c),
  GetDoubleCopyStatus: () => h.getStatus(),
  GetTheme: async () => 'light',
  SetTheme: async () => {},
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
  target.querySelector('label[aria-label="Translate on double Cmd+C"] input') as HTMLInputElement;
const note = () => target.querySelector('[data-testid="double-copy-permission"]');

async function mountTab() {
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(GeneralTab, { target, props: { curLang: 'en-US' } });
  await settle();
}

beforeEach(() => {
  h.config = { language: 'en-US', theme: 'auto', double_copy_translate: false };
  h.status = 'off';
  h.saveConfig.mockReset();
  h.saveConfig.mockResolvedValue(undefined);
  h.getStatus.mockReset();
  h.getStatus.mockImplementation(async () => h.status);
});
afterEach(() => {
  if (app) unmount(app);
  target?.remove();
});

describe('GeneralTab double-copy switch, mounted', () => {
  it('is off when the config says off, and has a tooltip', async () => {
    await mountTab();
    expect(toggle()).not.toBeNull();
    expect(toggle().checked).toBe(false);
    const label = toggle().closest('label')!;
    expect(label.getAttribute('title')).toBe(en.settings.doubleCopyHint);
    expect(note()).toBeNull();
  });

  it('is on when the config says on', async () => {
    h.config.double_copy_translate = true;
    h.status = 'running';
    await mountTab();
    expect(toggle().checked).toBe(true);
    expect(note()).toBeNull();
  });

  it('switching it on saves double_copy_translate: true and keeps the rest of the config', async () => {
    await mountTab();
    toggle().click();
    await settle();
    expect(h.saveConfig).toHaveBeenCalledTimes(1);
    expect(h.saveConfig.mock.calls[0][0]).toMatchObject({
      language: 'en-US',
      theme: 'auto',
      double_copy_translate: true,
    });
  });

  it('switching it off saves double_copy_translate: false', async () => {
    h.config.double_copy_translate = true;
    h.status = 'running';
    await mountTab();
    toggle().click();
    await settle();
    expect(h.saveConfig.mock.calls[0][0]).toMatchObject({ double_copy_translate: false });
  });

  it('shows the permission message, naming the pane, when the backend reports it missing', async () => {
    await mountTab();
    h.status = 'missing_permission';
    toggle().click();
    await settle();
    const n = note();
    expect(n, 'no permission message after switching on without the grant').not.toBeNull();
    expect(n!.textContent).toContain('Privacy & Security');
    expect(n!.textContent).toContain('Input Monitoring');
    expect(n!.textContent).toContain('Kai');
  });

  it('shows the message on open when the feature is on and the grant is missing', async () => {
    h.config.double_copy_translate = true;
    h.status = 'missing_permission';
    await mountTab();
    expect(note()).not.toBeNull();
  });

  it('hides the message again when the feature is switched off', async () => {
    h.config.double_copy_translate = true;
    h.status = 'missing_permission';
    await mountTab();
    h.status = 'off';
    toggle().click();
    await settle();
    expect(note()).toBeNull();
  });

  it('a failed status read shows no message and does not break the switch', async () => {
    h.getStatus.mockRejectedValue(new Error('boom'));
    await mountTab();
    toggle().click();
    await settle();
    expect(h.saveConfig).toHaveBeenCalledTimes(1);
    expect(note()).toBeNull();
  });
});

describe('double-copy copy', () => {
  it('exists in both locales with the same keys, differing text', () => {
    for (const k of ['doubleCopy', 'doubleCopyHint', 'doubleCopyPermission'] as const) {
      expect(en.settings[k], `en ${k}`).toBeTruthy();
      expect(zh.settings[k], `zh ${k}`).toBeTruthy();
      expect(en.settings[k]).not.toBe(zh.settings[k]);
    }
    expect(en.translate.doubleCopyPermission).toBeTruthy();
    expect(zh.translate.doubleCopyPermission).toBeTruthy();
  });

  it('the permission text names the panes a user has to open', () => {
    for (const s of [en.settings.doubleCopyPermission, en.translate.doubleCopyPermission]) {
      expect(s).toContain('Privacy & Security');
      expect(s).toContain('Input Monitoring');
    }
    for (const s of [zh.settings.doubleCopyPermission, zh.translate.doubleCopyPermission]) {
      expect(s).toContain('输入监控');
    }
  });
});
