import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { en } from '../../i18n/en-US.ts';
import { zh } from '../../i18n/zh-CN.ts';

// Issue #199 / ADR-0003 (#122): the REAL ShortcutsTab.svelte, mounted, with only the generated bindings faked. The
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
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/appservice.ts', () => ({
  CheckAccessibility: async () => true,
  OpenAccessibilitySettings: async () => {},
  CheckScreenRecording: async () => true,
  OpenScreenRecordingSettings: async () => {},
}));
vi.mock('@wailsio/runtime', () => ({
  Dialogs: { Info: async () => {}, Error: async () => {} },
  Events: { On: () => () => {}, Off: () => {}, Emit: () => {} },
}));
vi.mock('../../utils/analytics', () => ({ track: vi.fn() }));

import ShortcutsTab from './ShortcutsTab.svelte';
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

async function mountTab(Comp: any = ShortcutsTab, props: Record<string, unknown> = {}) {
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(Comp, { target, props });
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
  app = undefined;
  target?.remove();
});

describe('ShortcutsTab double-copy switch, mounted', () => {
  it('is off when the config says off, and has a tooltip', async () => {
    await mountTab();
    expect(toggle()).not.toBeNull();
    expect(toggle().checked).toBe(false);
    const label = toggle().closest('label')!;
    expect(label.getAttribute('title')).toBe(en.settings.doubleCopyHint);
    expect(note()).toBeNull();
  });

  it('is off when the config has no value for it (the default)', async () => {
    delete h.config.double_copy_translate;
    await mountTab();
    expect(toggle().checked).toBe(false);
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

describe('double-copy switch location', () => {
  it('GeneralTab no longer renders the switch, its hint or the permission note', async () => {
    h.config.double_copy_translate = true;
    h.status = 'missing_permission';
    await mountTab(GeneralTab, { curLang: 'en-US' });
    expect(toggle()).toBeNull();
    expect(note()).toBeNull();
    expect(target.textContent).not.toContain(en.settings.doubleCopy);
    expect(target.textContent).not.toContain(en.settings.doubleCopyHint);
  });

  it('a failed save is logged, does not throw, and the permission status is still re-read', async () => {
    const err = vi.spyOn(console, 'error').mockImplementation(() => {});
    h.saveConfig.mockRejectedValue(new Error('disk full'));
    await mountTab();
    const before = h.getStatus.mock.calls.length;
    toggle().click();
    await settle();
    expect(err).toHaveBeenCalled();
    expect(h.getStatus.mock.calls.length).toBeGreaterThan(before);
    err.mockRestore();
  });
});

describe('double-copy copy', () => {
  it('exists in both locales with the same keys, differing text', () => {
    for (const k of ['doubleCopy', 'doubleCopyHint', 'doubleCopyPermission'] as const) {
      expect(en.settings[k], `en ${k}`).toBeTruthy();
      expect(zh.settings[k], `zh ${k}`).toBeTruthy();
      expect(en.settings[k]).not.toBe(zh.settings[k]);
    }
    expect(en.log.shortcutSaveDoubleCopyFailed).toBeTruthy();
    expect(zh.log.shortcutSaveDoubleCopyFailed).toBeTruthy();
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

  it('every text that points at the switch says Settings > Shortcuts, never General', () => {
    expect(en.translate.doubleCopyPermission).toContain('Settings > Shortcuts');
    expect(zh.translate.doubleCopyPermission).toContain('设置 > 快捷键');
    for (const s of [en.translate.doubleCopyPermission, en.settings.doubleCopyPermission]) {
      expect(s).not.toContain('Settings > General');
    }
    expect(zh.translate.doubleCopyPermission).not.toContain('通用');
  });
});
