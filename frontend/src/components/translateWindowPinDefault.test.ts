import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #69: the translate window's always-on-top state, with the REAL TranslateWindow.svelte
// mounted in jsdom (only the Wails boundary faked). The window starts from the
// "translate_window_on_top" setting when nothing is stored, a stored pin choice always wins,
// and the old #39 migration (forcing a stored false back to true) must never come back:
// these tests fail the moment it is reintroduced.

const h = vi.hoisted(() => {
  const handlers = new Map<string, Array<(e: { data: unknown }) => void>>();
  return {
    handlers,
    fire(name: string, data: unknown) {
      for (const cb of handlers.get(name) ?? []) cb({ data });
    },
    emit: vi.fn(),
    plan: vi.fn(),
    reportSkip: vi.fn(async (_reason: string) => {}),
    translate: vi.fn(),
    back: vi.fn(),
    commit: vi.fn(),
    learn: vi.fn(),
    saveConfig: vi.fn(async (_c: unknown) => {}),
    setAlwaysOnTop: vi.fn(async (_b: boolean) => {}),
    getConfig: vi.fn(),
    config: { default_from: 'en', default_to: 'fr', auto_clipboard: false } as Record<
      string,
      unknown
    >,
  };
});

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: (name: string, cb: (e: { data: unknown }) => void) => {
      h.handlers.set(name, [...(h.handlers.get(name) ?? []), cb]);
      return () =>
        h.handlers.set(
          name,
          (h.handlers.get(name) ?? []).filter((x) => x !== cb),
        );
    },
    Emit: (...args: unknown[]) => h.emit(...(args as [string, unknown])),
  },
  Window: {
    SetAlwaysOnTop: (b: boolean) => h.setAlwaysOnTop(b),
    Name: async () => 'translate',
  },
  Clipboard: { SetText: vi.fn(async () => {}) },
  System: { IsDarkMode: async () => false },
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/translatewrapper.ts', () => ({
  TranslateMulti: (req: unknown) => h.translate(req),
  CancelTranslate: vi.fn(async () => true),
  PlanSourceSwitch: (req: unknown) => h.plan(req),
  ReportSourceSwitchSkipped: (reason: string) => h.reportSkip(reason),
  CorrectSource: vi.fn(),
  CommitAutoHistory: (id: string) => h.commit(id),
  CorrectionAvailability: async () => ({ available: true, reason: '' }),
  BackTranslate: (req: unknown) => h.back(req),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/langprefwrapper.ts', () => ({
  Learn: (l: string) => h.learn(l),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/windowwrapper.ts', () => ({
  ShowSettings: vi.fn(),
  ShowHistory: vi.fn(),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/enginewrapper.ts', () => ({
  GetEngines: async () => [
    { value: 'google', name: 'Google', kind: 'translate', supported: true },
  ],
  GetAllEngines: async () => [
    {
      id: 1,
      value: 'google',
      name: 'Google',
      kind: 'translate',
      enabled: true,
      supported: true,
      target_languages: ['en', 'fr', 'es'],
    },
  ],
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetLanguages: async () => ['auto', 'en', 'fr', 'es'].map((v) => ({ value: v, name: v })),
  GetConfig: () => h.getConfig(),
  SaveConfig: (c: unknown) => h.saveConfig(c),
  GetTheme: async () => 'light',
  SetTheme: async () => {},
}));

import TranslateWindow from './TranslateWindow.svelte';
import { en } from '../i18n/en-US.ts';

let app: Record<string, unknown> | undefined;
let target: HTMLElement;

const flush = async () => {
  for (let i = 0; i < 6; i++) await vi.advanceTimersByTimeAsync(0);
  flushSync();
};

async function mountWindow() {
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(TranslateWindow, { target });
  await flush();
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
  localStorage.clear();
  h.handlers.clear();
  h.plan.mockReset();
  h.plan.mockResolvedValue({ switched: false, from: '', to: '' });
  h.setAlwaysOnTop.mockReset();
  h.getConfig.mockReset();
  h.getConfig.mockImplementation(async () => ({ ...h.config }));
  h.config = { default_from: 'en', default_to: 'fr', auto_clipboard: false };
});

afterEach(() => {
  if (app) unmount(app);
  app = undefined;
  target?.remove();
  vi.useRealTimers();
});

describe('TranslateWindow, mounted: where the pin state starts (issue #69)', () => {
  it('nothing stored, setting absent: starts NOT on top (off is the default)', async () => {
    await mountWindow();
    expect(h.setAlwaysOnTop).toHaveBeenCalledWith(false);
    expect(h.setAlwaysOnTop).not.toHaveBeenCalledWith(true);
  });

  it('nothing stored, setting on: the setting is the default the window starts from', async () => {
    h.config.translate_window_on_top = true;
    await mountWindow();
    expect(h.setAlwaysOnTop).toHaveBeenCalledWith(true);
    expect(h.setAlwaysOnTop).not.toHaveBeenCalledWith(false);
  });

  it('a stored false stays false even with the setting on — the old re-pin migration must stay gone', async () => {
    localStorage.setItem('kai:translate:pinned', JSON.stringify(false));
    h.config.translate_window_on_top = true;
    await mountWindow();
    expect(h.setAlwaysOnTop).toHaveBeenCalledWith(false);
    expect(h.setAlwaysOnTop).not.toHaveBeenCalledWith(true);
    // Nothing re-pinned it behind the window's back either: the choice is still stored.
    expect(localStorage.getItem('kai:translate:pinned')).toBe(JSON.stringify(false));
  });

  it('a stored true beats a setting default of off', async () => {
    localStorage.setItem('kai:translate:pinned', JSON.stringify(true));
    await mountWindow();
    expect(h.setAlwaysOnTop).toHaveBeenCalledWith(true);
    expect(h.setAlwaysOnTop).not.toHaveBeenCalledWith(false);
  });

  it('the old migration flag is never written', async () => {
    await mountWindow();
    expect(localStorage.getItem('kai:translate:pinnedModalDefault')).toBeNull();
  });

  // The race (PR-Agent finding): the pin default is applied after `await GetConfig()`. A 🧷
  // click made while that config load is still pending writes an explicit choice, and an
  // explicit choice always wins — the default must not overwrite it when the await resolves.
  it('a pin click while the config is still loading survives the default being applied', async () => {
    let release!: (v: Record<string, unknown>) => void;
    const gate = new Promise<Record<string, unknown>>((r) => (release = r));
    h.getConfig.mockImplementation(() => gate); // pending until the test releases it
    h.config.translate_window_on_top = false; // the default that would be applied
    await mountWindow(); // parks inside the init at `await GetConfig()`

    // The explicit choice: click 🧷 while the config is pending (nothing stored, so this pins).
    const pinButton = target.querySelector(
      `[aria-label="${en.translate.pin}"]`,
    ) as HTMLButtonElement;
    expect(pinButton).not.toBeNull();
    pinButton.click();
    await flush();
    expect(localStorage.getItem('kai:translate:pinned')).toBe(JSON.stringify(true));
    expect(h.setAlwaysOnTop).toHaveBeenCalledWith(true);

    // The config lands; the default (off) must NOT overwrite the click.
    release({ ...h.config });
    await flush();
    expect(localStorage.getItem('kai:translate:pinned')).toBe(JSON.stringify(true));
    expect(h.setAlwaysOnTop).not.toHaveBeenCalledWith(false);
    expect(h.setAlwaysOnTop.mock.calls[h.setAlwaysOnTop.mock.calls.length - 1][0]).toBe(true);
  });
});
