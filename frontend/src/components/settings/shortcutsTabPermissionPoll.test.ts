import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #14: the REAL ShortcutsTab.svelte, mounted with fake timers and only the Wails boundary
// faked. While the Settings window is visible the three macOS permissions are re-checked every 3 s
// so a change made in System Settings shows up; polling pauses when the window is hidden.

const h = vi.hoisted(() => ({
  acc: true as boolean,
  sr: true as boolean,
  im: true as boolean,
  checkAcc: vi.fn(),
  checkSr: vi.fn(),
  checkIm: vi.fn(),
  handlers: new Map<string, (e: { data: unknown }) => void>(),
  doubleCopy: false,
  dcStatus: 'off',
}));

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: (name: string, cb: (e: { data: unknown }) => void) => {
      h.handlers.set(name, cb);
      return () => h.handlers.delete(name);
    },
    Emit: vi.fn(),
  },
  Window: { Name: async () => 'settings' },
  Dialogs: { Info: vi.fn(), Error: vi.fn() },
  System: { IsMac: () => true, IsDarkMode: async () => false },
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetConfig: async () => ({ double_copy_translate: h.doubleCopy }),
  SaveConfig: vi.fn(async () => {}),
  GetDoubleCopyStatus: async () => h.dcStatus,
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/appservice.ts', () => ({
  CheckAccessibility: () => h.checkAcc(),
  OpenAccessibilitySettings: vi.fn(),
  CheckScreenRecording: () => h.checkSr(),
  OpenScreenRecordingSettings: vi.fn(),
  OpenInputMonitoringSettings: async () => {},
  CheckInputMonitoring: () => h.checkIm(),
}));
vi.mock('../../utils/analytics', () => ({ track: vi.fn() }));

import ShortcutsTab from './ShortcutsTab.svelte';

let app: Record<string, unknown> | undefined;
let target: HTMLElement;
let visibility: 'visible' | 'hidden' = 'visible';

// Lets pending promises and Svelte's effects run, and moves the fake clock by ms.
const tick = async (ms = 0) => {
  await vi.advanceTimersByTimeAsync(ms);
  flushSync();
};
const setVisibility = (v: 'visible' | 'hidden') => {
  visibility = v;
  document.dispatchEvent(new Event('visibilitychange'));
};
const text = () => target.textContent ?? '';
// The status text of one permission row: its title's parent row.
const row = (title: string) => {
  const t = [...target.querySelectorAll('.text-sm.font-medium')].find(
    (n) => n.textContent?.trim() === title,
  );
  return t?.closest('.flex.items-center.justify-between')?.textContent ?? '';
};

async function mountTab() {
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(ShortcutsTab, { target });
  await tick();
}

beforeEach(() => {
  vi.useFakeTimers();
  h.acc = true;
  h.sr = true;
  h.im = true;
  h.doubleCopy = false;
  h.dcStatus = 'off';
  h.handlers.clear();
  visibility = 'visible';
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => visibility });
  h.checkAcc = vi.fn(async () => h.acc);
  h.checkSr = vi.fn(async () => h.sr);
  h.checkIm = vi.fn(async () => h.im);
});
afterEach(() => {
  if (app) unmount(app);
  app = undefined;
  target?.remove();
  vi.useRealTimers();
});

describe('ShortcutsTab permission polling (issue #14)', () => {
  it('checks once on load, then every 3 seconds: all three permissions', async () => {
    await mountTab();
    for (const c of [h.checkAcc, h.checkSr, h.checkIm]) expect(c).toHaveBeenCalledTimes(1);
    await tick(2999);
    expect(h.checkAcc).toHaveBeenCalledTimes(1);
    await tick(1);
    for (const c of [h.checkAcc, h.checkSr, h.checkIm]) expect(c).toHaveBeenCalledTimes(2);
    await tick(3000);
    for (const c of [h.checkAcc, h.checkSr, h.checkIm]) expect(c).toHaveBeenCalledTimes(3);
  });

  it('pauses while the window is hidden and checks again at once when it is shown', async () => {
    await mountTab();
    setVisibility('hidden');
    await tick(20000);
    expect(h.checkAcc).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(0);
    setVisibility('visible');
    await tick();
    expect(h.checkAcc).toHaveBeenCalledTimes(2);
    await tick(3000);
    expect(h.checkAcc).toHaveBeenCalledTimes(3);
  });

  it('pauses when the Settings window is closed (its close hook only hides it)', async () => {
    await mountTab();
    h.handlers.get('kai:window:closing')?.({ data: 'translate' }); // another window: ignored
    await tick(3000);
    expect(h.checkAcc).toHaveBeenCalledTimes(2);
    h.handlers.get('kai:window:closing')?.({ data: 'settings' });
    await tick(20000);
    expect(h.checkAcc).toHaveBeenCalledTimes(2);
    window.dispatchEvent(new Event('focus')); // shown again
    await tick();
    expect(h.checkAcc).toHaveBeenCalledTimes(3);
  });

  it('stops for good when the tab is unmounted', async () => {
    await mountTab();
    unmount(app!);
    app = undefined;
    await tick(20000);
    expect(h.checkAcc).toHaveBeenCalledTimes(1);
    expect(vi.getTimerCount()).toBe(0);
  });

  it('a false-to-true flip updates the card without reopening the page', async () => {
    h.acc = false;
    h.sr = false;
    h.im = false;
    await mountTab();
    expect(row('Accessibility')).toContain('Not granted');
    expect(row('Screen Recording')).toContain('Not granted');
    expect(row('Input Monitoring')).toContain('Not granted');
    expect(target.querySelector('[data-testid="accessibility-missing"]')).not.toBeNull();
    h.acc = true;
    await tick(3000);
    expect(row('Accessibility')).toContain('Granted');
    expect(row('Accessibility')).not.toContain('Not granted');
    expect(target.querySelector('[data-testid="accessibility-missing"]')).toBeNull();
    expect(row('Screen Recording')).toContain('Not granted');
    h.sr = true;
    h.im = true;
    await tick(3000);
    expect(row('Screen Recording')).not.toContain('Not granted');
    expect(row('Input Monitoring')).not.toContain('Not granted');
  });

  it('a true-to-false flip shows up too', async () => {
    h.sr = false; // keeps the block open so the rows are on screen
    await mountTab();
    expect(row('Accessibility')).not.toContain('Not granted');
    h.acc = false;
    await tick(3000);
    expect(row('Accessibility')).toContain('Not granted');
  });

  it('keeps the open-once rule: a block opened on load stays open when all become granted', async () => {
    h.acc = false;
    await mountTab();
    expect(text()).toContain('Permissions Required');
    expect(text()).not.toContain('Collapse');
    h.acc = true;
    await tick(3000);
    // Still expanded (its Collapse button is offered); it did not collapse by itself.
    expect(text()).toContain('Collapse');
    expect(row('Accessibility')).toContain('Granted');
  });

  it('keeps the open-once rule: a block collapsed on load is not forced open by polling', async () => {
    await mountTab();
    expect(text()).not.toContain('Collapse');
    expect(text()).not.toContain('Screen Recording'); // collapsed: only the summary line
    await tick(6000);
    expect(text()).not.toContain('Screen Recording');
  });

  it('never overlaps: a slow check is waited for, not stacked', async () => {
    let release: (v: boolean) => void = () => {};
    await mountTab();
    h.checkAcc = vi.fn(() => new Promise<boolean>((r) => (release = r)));
    await tick(3000); // tick starts the slow check
    expect(h.checkAcc).toHaveBeenCalledTimes(1);
    await tick(14000);
    expect(h.checkAcc).toHaveBeenCalledTimes(1);
    release(true);
    await tick(3000);
    expect(h.checkAcc).toHaveBeenCalledTimes(2);
  });

  it('a failed check keeps the last known state instead of blanking the card', async () => {
    h.sr = false;
    await mountTab();
    expect(row('Accessibility')).toContain('Granted');
    h.checkAcc = vi.fn(async () => {
      throw new Error('bridge gone');
    });
    await tick(3000);
    expect(row('Accessibility')).toContain('Granted');
  });

  it('the double Cmd+C note follows the live Input Monitoring check, not the cached listener state', async () => {
    h.doubleCopy = true;
    h.dcStatus = 'missing_permission'; // the listener cached this when it started
    h.im = false;
    await mountTab();
    expect(target.querySelector('[data-testid="double-copy-permission"]')).not.toBeNull();
    h.im = true;
    await tick(3000);
    expect(target.querySelector('[data-testid="double-copy-permission"]')).toBeNull();
  });
});
