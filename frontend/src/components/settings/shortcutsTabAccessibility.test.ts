import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #194: the REAL ShortcutsTab.svelte, mounted, with only the Wails boundary faked. When macOS
// says the Accessibility permission is not granted, the Accessibility row must say what to enable
// and where (both names of the pane) and that Kai must be removed and added again after a rebuild;
// when it is granted, no such message.

const h = vi.hoisted(() => ({
  accessibility: true,
  screenRecording: true,
  openAccessibility: vi.fn(),
}));

vi.mock('@wailsio/runtime', () => ({
  Events: { On: () => () => {}, Emit: vi.fn() },
  Window: { Name: async () => 'settings' },
  Dialogs: { Info: vi.fn(), Error: vi.fn() },
  System: { IsMac: () => true, IsDarkMode: async () => false },
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetConfig: async () => ({ hotkeys: {}, exec_keys: {}, auto_clipboard: false }),
  SaveConfig: vi.fn(async () => {}),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/appservice.ts', () => ({
  CheckAccessibility: async () => h.accessibility,
  OpenAccessibilitySettings: () => h.openAccessibility(),
  CheckScreenRecording: async () => h.screenRecording,
  CheckInputMonitoring: async () => true,
  OpenScreenRecordingSettings: vi.fn(),
  OpenInputMonitoringSettings: async () => {},
}));

import ShortcutsTab from './ShortcutsTab.svelte';

let app: Record<string, unknown> | undefined;
let target: HTMLElement;
const settle = async () => {
  for (let i = 0; i < 6; i++) {
    await Promise.resolve();
    await new Promise((r) => setTimeout(r, 0));
  }
  flushSync();
};
const note = () => target.querySelector('[data-testid="accessibility-missing"]');

async function mountTab() {
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(ShortcutsTab, { target });
  await settle();
}

beforeEach(() => {
  h.accessibility = true;
  h.screenRecording = true;
  h.openAccessibility.mockReset();
});
afterEach(() => {
  if (app) unmount(app);
  app = undefined;
  target?.remove();
});

describe('ShortcutsTab Accessibility row, mounted (issue #194)', () => {
  it('says what to enable and where when the permission is not granted', async () => {
    h.accessibility = false;
    await mountTab();
    const n = note();
    expect(n, 'no message in the Accessibility row for a missing permission').not.toBeNull();
    const text = n!.textContent ?? '';
    expect(text).toContain('Privacy & Security');
    expect(text).toContain('Device Control and Data Access');
    expect(text).toContain('Kai');
  });

  it('keeps the note to one short line: the status and the button already say the rest', async () => {
    // It was a four-line paragraph repeating what the toast says, which cluttered the row.
    h.accessibility = false;
    await mountTab();
    const text = (note()?.textContent ?? '').trim();
    expect(text.length).toBeGreaterThan(0);
    expect(text.length).toBeLessThanOrEqual(90);
    expect(text).not.toContain('add it again');
  });

  it('shows no message when the permission is granted', async () => {
    // Screen Recording not granted keeps the permission block expanded, so the Accessibility row
    // is really on screen (all granted collapses the whole block).
    h.screenRecording = false;
    await mountTab();
    expect(target.textContent).toContain('Accessibility');
    expect(note()).toBeNull();
  });

  it('opening the pane still needs a click: mounting alone never requests it', async () => {
    h.accessibility = false;
    await mountTab();
    expect(h.openAccessibility).not.toHaveBeenCalled();
  });
});
