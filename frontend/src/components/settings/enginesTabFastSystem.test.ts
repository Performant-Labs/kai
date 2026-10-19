import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #10, mounted: the "Faster System translation" toggle in the Engines tab, with the REAL
// EnginesTab.svelte in jsdom. The engine schema is the real one the backend serves for the
// System engine (builtin translate, NO fields — nothing to configure), which matters: the panel
// the toggle lives in must render for exactly that shape. Only the Wails boundary is faked.

const h = vi.hoisted(() => {
  return {
    saveConfig: vi.fn(),
    config: { default_from: 'en', default_to: 'fr', fast_system_translate: false } as Record<
      string,
      unknown
    >,
  };
});

vi.mock('@wailsio/runtime', () => ({
  Events: { On: vi.fn(() => () => {}), Emit: vi.fn() },
  Window: { SetAlwaysOnTop: vi.fn(async () => {}), Name: async () => 'settings' },
  Clipboard: { SetText: vi.fn(async () => {}) },
  System: { IsDarkMode: async () => false },
  Dialogs: { Info: vi.fn(async () => {}), Error: vi.fn(async () => {}) },
}));
vi.mock('../../runtime', () => ({
  emitEvent: vi.fn(),
  onEvent: vi.fn(() => () => {}),
  getWindowName: vi.fn(async () => 'settings'),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/enginewrapper.ts', () => ({
  // The System engine exactly as GetAllEngines serves it on macOS.
  GetAllEngines: async () => [
    {
      id: 1,
      value: 'apple',
      name: 'System',
      kind: 'translate',
      enabled: true,
      supported: true,
      builtin: true,
    },
  ],
  GetKnownEngines: async () => [],
  // The real apple schema: builtin translate, Fields nil — "this engine needs no extra config".
  GetEngineSchema: async () => ({ builtin: true, kind: 'translate', fields: null }),
  GetEngineConfig: async () => ({}),
  AddEngine: vi.fn(),
  UpdateEngineConfig: vi.fn(),
  ToggleEngineEnabled: vi.fn(),
  RemoveEngine: vi.fn(),
  CheckTesseract: vi.fn(),
  GetOcrLangs: vi.fn(async () => []),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  SystemLanguages: async () => ['en', 'es-MX'],
  GetConfig: async () => ({ ...h.config }),
  SaveConfig: (c: unknown) => h.saveConfig(c),
}));

import EnginesTab from './EnginesTab.svelte';

let app: Record<string, unknown> | undefined;
let target: HTMLElement;

const flush = async () => {
  for (let i = 0; i < 6; i++) await Promise.resolve();
  flushSync();
};
const toggle = () =>
  target.querySelector('[data-testid="fast-system-translate"]') as HTMLInputElement | null;

beforeEach(async () => {
  localStorage.clear();
  h.saveConfig.mockReset();
  h.config = { default_from: 'en', default_to: 'fr', fast_system_translate: false };
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(EnginesTab, { target });
  await flush();
});

afterEach(() => {
  if (app) unmount(app);
  target.remove();
});

describe('EnginesTab, mounted: the faster-translation toggle (issue #10)', () => {
  it('renders in the System engine panel although the engine has no config fields', async () => {
    // The one thing the source-grep test could not pin: the panel must appear for the real
    // apple schema (builtin translate, fields null), where the "no extra config" placeholder
    // is what fills the rest of the card.
    expect(toggle()).not.toBeNull();
  });

  it('reads its initial state from the settings', async () => {
    expect(toggle()!.checked).toBe(false);
    h.config = { ...h.config, fast_system_translate: true };
    unmount(app!);
    target.remove();
    target = document.createElement('div');
    document.body.appendChild(target);
    app = mount(EnginesTab, { target });
    await flush();
    expect(toggle()!.checked).toBe(true);
  });

  it('flipping it persists fast_system_translate through SaveConfig, merged into the config', async () => {
    const box = toggle()!;
    box.checked = true;
    box.dispatchEvent(new Event('change', { bubbles: true }));
    await flush();
    expect(h.saveConfig).toHaveBeenCalledTimes(1);
    const saved = h.saveConfig.mock.calls[0][0] as Record<string, unknown>;
    expect(saved.fast_system_translate).toBe(true);
    expect(saved.default_to).toBe('fr'); // read-modify-write, not a bare key
    box.checked = false;
    box.dispatchEvent(new Event('change', { bubbles: true }));
    await flush();
    expect(h.saveConfig).toHaveBeenCalledTimes(2);
    const off = h.saveConfig.mock.calls[1][0] as Record<string, unknown>;
    expect(off.fast_system_translate).toBe(false);
  });
});
