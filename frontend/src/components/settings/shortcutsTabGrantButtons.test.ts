import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #23: each of the three permission rows has a Grant button that calls its own binding, and
// the buttons do no polling of their own (the 3-second re-check from #14 updates the card).

const h = vi.hoisted(() => ({
  openAcc: vi.fn(async () => {}),
  openSr: vi.fn(async () => {}),
  openIm: vi.fn(async () => {}),
  checkIm: vi.fn(async () => false),
}));

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
  CheckAccessibility: async () => false,
  OpenAccessibilitySettings: h.openAcc,
  CheckScreenRecording: async () => false,
  OpenScreenRecordingSettings: h.openSr,
  CheckInputMonitoring: () => h.checkIm(),
  OpenInputMonitoringSettings: h.openIm,
}));
vi.mock('../../utils/analytics', () => ({ track: vi.fn() }));

import ShortcutsTab from './ShortcutsTab.svelte';

let app: Record<string, unknown> | undefined;
let target: HTMLElement;
const tick = async (ms = 0) => {
  await vi.advanceTimersByTimeAsync(ms);
  flushSync();
};
const btn = (id: string) => target.querySelector<HTMLButtonElement>(`[data-testid="${id}"]`);

beforeEach(async () => {
  vi.useFakeTimers();
  Object.defineProperty(document, 'visibilityState', {
    configurable: true,
    get: () => 'visible',
  });
  Object.values(h).forEach((f) => f.mockClear());
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(ShortcutsTab, { target });
  await tick();
});
afterEach(() => {
  if (app) unmount(app);
  app = undefined;
  target.remove();
  vi.useRealTimers();
});

describe('ShortcutsTab Grant buttons (issue #23)', () => {
  it('the Input Monitoring row has a Grant button with the same label and style as the others', () => {
    const im = btn('grant-input-monitoring');
    expect(im).not.toBeNull();
    expect(im!.textContent?.trim()).toBe('Grant access');
    expect(im!.className).toBe(btn('grant-screen-recording')!.className);
    expect(im!.className).toBe(btn('grant-accessibility')!.className);
  });

  it('each button calls its own binding once', async () => {
    btn('grant-input-monitoring')!.click();
    await tick();
    expect(h.openIm).toHaveBeenCalledTimes(1);
    expect(h.openSr).not.toHaveBeenCalled();
    expect(h.openAcc).not.toHaveBeenCalled();
    btn('grant-screen-recording')!.click();
    btn('grant-accessibility')!.click();
    await tick();
    expect(h.openSr).toHaveBeenCalledTimes(1);
    expect(h.openAcc).toHaveBeenCalledTimes(1);
  });

  it('the Input Monitoring button adds no polling of its own: only the 3 s re-check runs', async () => {
    const before = h.checkIm.mock.calls.length;
    btn('grant-input-monitoring')!.click();
    await tick(2999);
    // At most the single short re-check the other two buttons already do; never a fast loop.
    expect(h.checkIm.mock.calls.length - before).toBeLessThanOrEqual(1);
    await tick(1);
    expect(h.checkIm.mock.calls.length - before).toBeLessThanOrEqual(2);
  });
});
