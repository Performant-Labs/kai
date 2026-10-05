import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { en } from '../../i18n/en-US.ts';
import { zh } from '../../i18n/zh-CN.ts';

// Issue #41: the REAL GeneralTab.svelte, mounted, shows the installed version in an About card, marks
// a development build, and copies the version for pasting into an issue. Only the generated bindings
// and the Wails runtime are faked.

const h = vi.hoisted(() => ({
  version: 'v0.4.0',
  dev: false,
  getVersion: vi.fn(),
  setText: vi.fn(),
}));

vi.mock('@wailsio/runtime', () => ({
  Events: { On: () => () => {}, Emit: vi.fn() },
  Window: {},
  Clipboard: { SetText: (text: string) => h.setText(text) },
  System: { IsDarkMode: async () => false },
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/appservice.ts', () => ({
  GetVersion: () => h.getVersion(),
  IsDevBuild: async () => h.dev,
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetConfig: async () => ({ language: 'en-US', theme: 'auto', font_size: 120 }),
  SaveConfig: vi.fn(),
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
const q = (id: string) => target.querySelector(`[data-testid="${id}"]`) as HTMLElement | null;

async function mountTab() {
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(GeneralTab, { target, props: { curLang: 'en-US' } });
  await settle();
}

beforeEach(() => {
  h.version = 'v0.4.0';
  h.dev = false;
  h.getVersion.mockReset();
  h.getVersion.mockImplementation(async () => h.version);
  h.setText.mockReset();
  h.setText.mockResolvedValue(undefined);
});
afterEach(() => {
  if (app) unmount(app);
  target.remove();
  app = undefined;
});

describe('GeneralTab About card (issue #41)', () => {
  it('shows the version the backend reports', async () => {
    await mountTab();
    expect(q('app-version')?.textContent?.trim()).toBe('v0.4.0');
  });

  // The card sits above every other control, so the version is visible without scrolling.
  it('puts the About card above the language control, the first control of the tab', async () => {
    await mountTab();
    const version = q('app-version')!;
    const firstControl = target.querySelector('#lang-sel')!;
    expect(
      version.compareDocumentPosition(firstControl) & Node.DOCUMENT_POSITION_FOLLOWING,
      'the version must come before the language select in the page',
    ).toBeTruthy();
  });

  it('shows no development marker on a release build', async () => {
    await mountTab();
    expect(q('app-dev-build')).toBeNull();
  });

  it('marks a development build', async () => {
    h.dev = true;
    await mountTab();
    expect(q('app-dev-build')?.textContent).toContain(en.settings.aboutDevBuild);
  });

  it('copies "Kai <version>" and says it was copied', async () => {
    await mountTab();
    (q('app-version-copy') as HTMLButtonElement).click();
    await settle();
    expect(h.setText).toHaveBeenCalledWith('Kai v0.4.0');
    expect(q('app-version-copy')?.textContent).toContain(en.common.copied);
  });

  it('still renders the rest of the tab when the version cannot be read', async () => {
    h.getVersion.mockRejectedValue(new Error('boom'));
    await mountTab();
    expect(q('app-version')?.textContent?.trim()).toBe('');
    expect(q('font-size-value')).not.toBeNull();
  });

  it('has its strings in both locales', () => {
    for (const [name, cat] of [
      ['en', en],
      ['zh', zh],
    ] as const) {
      expect(cat.settings.about, name).toBeTruthy();
      expect(cat.settings.aboutVersion, name).toBeTruthy();
      expect(cat.settings.aboutCopy, name).toBeTruthy();
      expect(cat.settings.aboutDevBuild, name).toBeTruthy();
    }
  });
});
