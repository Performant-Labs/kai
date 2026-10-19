import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #60: the History shortcut is configured in Settings > Shortcuts like the clear-context one
// (#48): validated before anything is saved, kept in localStorage, broadcast to the translate window.

const h = vi.hoisted(() => ({
  emit: vi.fn(),
  error: vi.fn(async (_o: unknown): Promise<void> => {}),
}));

vi.mock('@wailsio/runtime', () => ({
  Events: { On: () => () => {}, Emit: (...a: unknown[]) => h.emit(...a) },
  Window: { Name: async () => 'settings' },
  Dialogs: { Info: vi.fn(), Error: (o: unknown) => h.error(o) },
  System: { IsMac: () => true, IsDarkMode: async () => false },
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetConfig: async () => ({ double_copy_translate: false }),
  SaveConfig: vi.fn(async () => {}),
  GetDoubleCopyStatus: async () => 'off',
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/appservice.ts', () => ({
  CheckAccessibility: async () => true,
  OpenAccessibilitySettings: vi.fn(async () => {}),
  CheckScreenRecording: async () => true,
  OpenScreenRecordingSettings: vi.fn(async () => {}),
  CheckInputMonitoring: async () => true,
  OpenInputMonitoringSettings: vi.fn(async () => {}),
}));
vi.mock('../../utils/analytics', () => ({ track: vi.fn() }));

import ShortcutsTab from './ShortcutsTab.svelte';
import { historyShortcut } from '../../stores/historyShortcut';
import { clearContextShortcut } from '../../stores/clearContextShortcut';

let app: Record<string, unknown> | undefined;
let target: HTMLElement;
const flush = async () => {
  for (let i = 0; i < 6; i++) await vi.advanceTimersByTimeAsync(0);
  flushSync();
};
const field = () => target.querySelector('[data-testid="history-key"]') as HTMLInputElement;
const type = (v: string) => {
  field().value = v;
  field().dispatchEvent(new Event('input', { bubbles: true }));
};
const save = async () => {
  const b = [...target.querySelectorAll('button')].find((x) =>
    x.className.includes('u-btn--primary'),
  ) as HTMLButtonElement;
  b.click();
  await flush();
};

beforeEach(async () => {
  vi.useFakeTimers();
  localStorage.clear();
  h.emit.mockClear();
  h.error.mockClear();
  historyShortcut.set('Cmd+Y');
  clearContextShortcut.set('Cmd+Shift+K');
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'visible' });
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(ShortcutsTab, { target });
  await flush();
});
afterEach(() => {
  if (app) unmount(app);
  app = undefined;
  target.remove();
  vi.useRealTimers();
});

describe('Settings > Shortcuts: the History shortcut (issue #60)', () => {
  it('shows the shortcut in use, and Reset brings back Cmd+Y', async () => {
    expect(field().value).toBe('Cmd+Y');
    type('Cmd+Shift+H');
    (target.querySelector('[data-testid="history-reset"]') as HTMLButtonElement).click();
    await flush();
    expect(field().value).toBe('Cmd+Y');
  });

  it('saves a new shortcut, writes it canonically, and tells the translate window', async () => {
    type('cmd+shift+h');
    await save();
    expect(h.error).not.toHaveBeenCalled();
    let stored = '';
    historyShortcut.subscribe((v) => (stored = v))();
    expect(stored).toBe('Cmd+Shift+H');
    expect(h.emit).toHaveBeenCalledWith('kai:history:shortcut-changed', 'Cmd+Shift+H');
  });

  it('allows no shortcut at all', async () => {
    type('');
    await save();
    expect(h.error).not.toHaveBeenCalled();
    expect(h.emit).toHaveBeenCalledWith('kai:history:shortcut-changed', '');
  });

  it.each([
    ['y', 'needs Cmd, Ctrl or Alt'],
    ['Cmd+', 'needs a key'],
    ['Cmd+Z', 'already used'],
    ['Cmd+Shift+K', 'clear-context shortcut'],
  ])('refuses %s and saves nothing (%s)', async (combo) => {
    type(combo);
    await save();
    expect(h.error).toHaveBeenCalledTimes(1);
    expect(h.emit).not.toHaveBeenCalled();
    let stored = '';
    historyShortcut.subscribe((v) => (stored = v))();
    expect(stored).toBe('Cmd+Y');
  });
});
