import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #69 + #113: the translate window's always-on-top state, with the REAL
// TranslateWindow.svelte mounted in jsdom (only the Wails boundary faked). The window starts
// from the "translate_window_on_top" setting when nothing is stored, a stored pin choice
// always wins, and the old #39 migration (forcing a stored false back to true) must never
// come back: these tests fail the moment it is reintroduced. Since #113 the stored choice
// counts only under the VERSIONED key kai:translate:pinned:v2 (an explicit-write-only store);
// the old kai:translate:pinned key is pollution that must be ignored, and the setting-changed
// broadcast + Settings-close re-pin resolve through the effectivePin decision (utils/pinDefault).

const h = vi.hoisted(() => {
  const handlers = new Map<string, Array<(e: { data: unknown }) => void>>();
  return {
    handlers,
    fire(name: string, data?: unknown) {
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
  GetEngines: async () => [{ value: 'google', name: 'Google', kind: 'translate', supported: true }],
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

  it('a stored v2 false stays false even with the setting on — the old re-pin migration must stay gone', async () => {
    // #113: the choice counts only under the versioned key; the old key saying the opposite
    // is exactly the pollution this story retires — ignored, never honored.
    localStorage.setItem('kai:translate:pinned:v2', JSON.stringify(false));
    localStorage.setItem('kai:translate:pinned', JSON.stringify(true));
    h.config.translate_window_on_top = true;
    await mountWindow();
    expect(h.setAlwaysOnTop).toHaveBeenCalledWith(false);
    expect(h.setAlwaysOnTop).not.toHaveBeenCalledWith(true);
    // Nothing re-pinned it behind the window's back either: both keys exactly as they were.
    expect(localStorage.getItem('kai:translate:pinned:v2')).toBe(JSON.stringify(false));
    expect(localStorage.getItem('kai:translate:pinned')).toBe(JSON.stringify(true));
  });

  it('a stored v2 true beats a setting default of off', async () => {
    localStorage.setItem('kai:translate:pinned:v2', JSON.stringify(true));
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
    expect(localStorage.getItem('kai:translate:pinned:v2')).toBe(JSON.stringify(true));
    expect(h.setAlwaysOnTop).toHaveBeenCalledWith(true);

    // The config lands; the default (off) must NOT overwrite the click.
    release({ ...h.config });
    await flush();
    expect(localStorage.getItem('kai:translate:pinned:v2')).toBe(JSON.stringify(true));
    expect(h.setAlwaysOnTop).not.toHaveBeenCalledWith(false);
    expect(h.setAlwaysOnTop.mock.calls[h.setAlwaysOnTop.mock.calls.length - 1][0]).toBe(true);
  });

  // ---- #113: the versioned, explicit-write-only pin store ----

  it('#113: an old-key pin is pollution, not a choice — with the setting off, the window starts NOT on top', async () => {
    localStorage.setItem('kai:translate:pinned', JSON.stringify(true)); // the pre-#69 era value
    await mountWindow();
    expect(h.setAlwaysOnTop).toHaveBeenCalledWith(false);
    expect(h.setAlwaysOnTop).not.toHaveBeenCalledWith(true);
    // The resolution never reaches storage, and the old key is left exactly as it was.
    expect(localStorage.getItem('kai:translate:pinned:v2')).toBeNull();
    expect(localStorage.getItem('kai:translate:pinned')).toBe(JSON.stringify(true));
  });

  it('a mount with nothing stored leaves localStorage empty — no auto-write anywhere (A Finding 4)', async () => {
    await mountWindow();
    expect(localStorage.getItem('kai:translate:pinned:v2')).toBeNull();
    expect(localStorage.getItem('kai:translate:pinned')).toBeNull(); // persisted's subscribe auto-writes this one today
  });

  it('an explicit 🧷 toggle is the only write: it lands under the versioned key only', async () => {
    await mountWindow();
    const pinButton = target.querySelector(
      `[aria-label="${en.translate.pin}"]`,
    ) as HTMLButtonElement;
    expect(pinButton).not.toBeNull();
    pinButton.click();
    await flush();
    expect(localStorage.getItem('kai:translate:pinned:v2')).toBe(JSON.stringify(true));
    expect(localStorage.getItem('kai:translate:pinned')).toBeNull(); // the old key is dead
  });

  it('#113 AC7(a): the on-top-changed broadcast applies the live setting, and the Settings-close re-pin does not revert it', async () => {
    await mountWindow(); // nothing stored, setting off → starts NOT on top
    expect(h.setAlwaysOnTop).not.toHaveBeenCalledWith(true);
    // The settings page saves the new value, then broadcasts. (The literal name, not the
    // constant, on purpose: importing the not-yet-existing export would module-resolution-fail
    // this whole file at RED and mask these assertions; utils/events.test.ts pins the constant
    // to exactly this string.)
    h.config.translate_window_on_top = true;
    h.fire('kai:translate-window-on-top:changed');
    await flush();
    expect(h.setAlwaysOnTop).toHaveBeenCalledWith(true);
    // Closing Settings fires the re-pin: it must resolve through the effectivePin decision
    // (nothing stored, untouched → the live setting), not the raw mount-time store value.
    h.fire('kai:window:closing', 'settings');
    await flush();
    expect(h.setAlwaysOnTop.mock.calls[h.setAlwaysOnTop.mock.calls.length - 1][0]).toBe(true);
  });

  // #113 AC7(b) — the locked rule of #69, end-to-end on the real component: an explicit 🧷
  // choice this mount wins at EVERY later application point. The direction is the one that
  // discriminates: the window starts pinned from the setting, the click unpins it (false),
  // and the setting then lands back ON — the OPPOSITE of the clicked value — so an
  // implementation that applied the live setting (or the mount-time level) instead of
  // honoring the click would visibly re-pin. The setting is toggled the way a user would,
  // off then on, and every save broadcasts.
  it('#113 AC7(b): the explicit 🧷 choice wins over the setting toggle and the Settings-close re-pin', async () => {
    h.config.translate_window_on_top = true; // nothing stored → starts pinned from the setting
    await mountWindow();
    expect(h.setAlwaysOnTop).toHaveBeenCalledWith(true);

    // The explicit choice: click 🧷 to unpin while the setting says on.
    const pinButton = target.querySelector(
      `[aria-label="${en.translate.unpin}"]`,
    ) as HTMLButtonElement;
    expect(pinButton).not.toBeNull();
    pinButton.click();
    await flush();
    expect(h.setAlwaysOnTop).toHaveBeenCalledWith(false);
    expect(localStorage.getItem('kai:translate:pinned:v2')).toBe(JSON.stringify(false));

    // From here only the post-click legs matter; the mount/click calls would blur the
    // "never re-pinned" assertions below.
    h.setAlwaysOnTop.mockClear();

    // Settings toggles the switch OFF and saves → broadcast (payload-free: the handler
    // re-fetches the config, the event carries no payload — NIT-1).
    h.config.translate_window_on_top = false;
    h.fire('kai:translate-window-on-top:changed');
    await flush();
    expect(h.setAlwaysOnTop.mock.calls.length).toBeGreaterThan(0);
    expect(h.setAlwaysOnTop.mock.calls[h.setAlwaysOnTop.mock.calls.length - 1][0]).toBe(false);
    expect(h.setAlwaysOnTop).not.toHaveBeenCalledWith(true);

    // Settings toggles it back ON and saves → broadcast again. The discriminating leg: the
    // live setting (true) is the opposite of the clicked value (false); the explicit choice
    // must still win.
    h.setAlwaysOnTop.mockClear();
    h.config.translate_window_on_top = true;
    h.fire('kai:translate-window-on-top:changed');
    await flush();
    expect(h.setAlwaysOnTop.mock.calls.length).toBeGreaterThan(0);
    expect(h.setAlwaysOnTop.mock.calls[h.setAlwaysOnTop.mock.calls.length - 1][0]).toBe(false);
    expect(h.setAlwaysOnTop).not.toHaveBeenCalledWith(true);

    // Closing Settings fires the re-pin: it must resolve through the effectivePin decision
    // again — the explicit choice still wins, the live setting must not re-pin.
    h.setAlwaysOnTop.mockClear();
    h.fire('kai:window:closing', 'settings');
    await flush();
    expect(h.setAlwaysOnTop.mock.calls.length).toBeGreaterThan(0);
    expect(h.setAlwaysOnTop.mock.calls[h.setAlwaysOnTop.mock.calls.length - 1][0]).toBe(false);
    expect(h.setAlwaysOnTop).not.toHaveBeenCalledWith(true);
    // And the stored explicit choice is exactly what the click left it: no application point
    // wrote or reverted anything (apply() never persists).
    expect(localStorage.getItem('kai:translate:pinned:v2')).toBe(JSON.stringify(false));
  });
});
