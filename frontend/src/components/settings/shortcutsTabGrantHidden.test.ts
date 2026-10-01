import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #23: a row's Grant button shows only while that permission is missing (on the owner's Mac
// Input Monitoring is already covered by Device Control and Data Access, and a Grant button next to
// "Granted" asked for something already there). Screen Recording tells the user how to add Kai by
// hand and to restart Kai, because macOS does not list the app on its own for every build.

const h = vi.hoisted(() => ({ acc: true, sr: true, im: true }));

vi.mock('@wailsio/runtime', () => ({
  Events: { On: () => () => {}, Emit: vi.fn() },
  Window: { Name: async () => 'settings' },
  Dialogs: { Info: vi.fn(), Error: vi.fn() },
  System: { IsMac: () => true, IsDarkMode: async () => false },
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetConfig: async () => ({ double_copy_translate: false }),
  SaveConfig: vi.fn(async () => {}),
  GetDoubleCopyStatus: async () => 'off',
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/appservice.ts', () => ({
  CheckAccessibility: async () => h.acc,
  OpenAccessibilitySettings: vi.fn(async () => {}),
  CheckScreenRecording: async () => h.sr,
  OpenScreenRecordingSettings: vi.fn(async () => {}),
  CheckInputMonitoring: async () => h.im,
  OpenInputMonitoringSettings: vi.fn(async () => {}),
}));
vi.mock('../../utils/analytics', () => ({ track: vi.fn() }));

import ShortcutsTab from './ShortcutsTab.svelte';

let app: Record<string, unknown> | undefined;
let target: HTMLElement;
const tick = async (ms = 0) => {
  await vi.advanceTimersByTimeAsync(ms);
  flushSync();
};
const q = (id: string) => target.querySelector(`[data-testid="${id}"]`);

async function render() {
  vi.useFakeTimers();
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => 'visible' });
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(ShortcutsTab, { target });
  await tick();
}
afterEach(() => {
  if (app) unmount(app);
  app = undefined;
  target?.remove();
  vi.useRealTimers();
});
beforeEach(() => {
  h.acc = h.sr = h.im = true;
});

describe('ShortcutsTab Grant buttons only while the permission is missing', () => {
  it('shows no Grant button and no Screen Recording note when all three are granted', async () => {
    await render();
    // All granted collapses the block; open it so the rows are in the page.
    (target.querySelector('button') as HTMLButtonElement | null)?.click();
    const details = [...target.querySelectorAll('button')].find((b) =>
      /Details/.test(b.textContent ?? ''),
    );
    details?.click();
    await tick();
    expect(q('grant-accessibility')).toBeNull();
    expect(q('grant-screen-recording')).toBeNull();
    expect(q('grant-input-monitoring')).toBeNull();
    expect(q('screen-recording-missing')).toBeNull();
  });

  it('shows only the Screen Recording button, with the add-by-hand-and-restart note, when only it is missing', async () => {
    h.sr = false;
    await render();
    expect(q('grant-screen-recording')).not.toBeNull();
    expect(q('grant-accessibility')).toBeNull();
    expect(q('grant-input-monitoring')).toBeNull();
    expect(q('screen-recording-missing')?.textContent).toMatch(/click \+ to add it.*restart Kai/);
  });

  it('names the first row Device Control and Data Access', async () => {
    h.acc = false;
    await render();
    expect(target.textContent).toContain('Device Control and Data Access');
  });
});
