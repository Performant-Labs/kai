import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// Issue #69: the translate window gets a gear that opens Settings. The repo has no Svelte
// component-render harness, so this pins the markup contract from the component source (the
// visible placement/centering is a manual check, #28).

const src = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');

// The <button ...> opening tag through its closing </button> that mentions titlebar.settings.
function gearButton(): string | null {
  for (const m of src.matchAll(/<button[\s\S]*?<\/button>/g)) {
    if (m[0].includes('titlebar.settings')) return m[0];
  }
  return null;
}

describe('translate window gear button', () => {
  it('exists, labelled and titled with titlebar.settings', () => {
    const btn = gearButton();
    expect(btn, "a <button> using t('titlebar.settings')").not.toBeNull();
    expect(btn).toMatch(/aria-label=\{t\('titlebar\.settings'\)\}/);
    expect(btn).toMatch(/title=\{t\('titlebar\.settings'\)\}/);
  });

  it('calls the existing ShowSettings binding and nothing else opens Settings', () => {
    expect(src).toMatch(
      /import\s*\{[^}]*\bShowSettings\b[^}]*\}\s*from\s*'@bindings\/cnb\.cool\/dtapp\/kai\/internal\/service\/windowwrapper\.ts'/,
    );
    const btn = gearButton() ?? '';
    expect(btn).toMatch(/onclick=\{[^}]*ShowSettings/);
    expect(btn).not.toMatch(/emitEvent/);
  });

  it('is a non-draggable icon button at the swap button size (not --sm)', () => {
    const btn = gearButton() ?? '';
    expect(btn).toMatch(/u-icon-btn/);
    expect(btn).toMatch(/u-no-drag/);
    expect(btn).not.toMatch(/u-icon-btn--sm/);
    expect(btn).toMatch(/width="16"/);
  });

  it('has non-empty, distinct en-US and zh-CN labels', () => {
    const enT = (en.titlebar as Record<string, string>).settings;
    const zhT = (zh.titlebar as Record<string, string>).settings;
    expect(enT).toBeTruthy();
    expect(zhT).toBeTruthy();
    expect(zhT).not.toBe(enT);
  });
});

// Issue #69, found in manual testing: a pinned (always-on-top) translate window covered a freshly
// opened Settings window. Go lowers the translate window when Settings opens; when Settings
// closes it broadcasts EventWindowClosing with the Settings window name, and the translate window
// must put its persisted pin back (Wails has no getter, so the pin lives here, not in Go).
describe('translate window pin after Settings closes', () => {
  // The EventWindowClosing handler body.
  const handler = (() => {
    const m = src.match(/onEvent\(EventWindowClosing,[\s\S]*?\n    \}\);/);
    return m ? m[0] : '';
  })();

  it('handles the Settings window closing event', () => {
    expect(handler).toMatch(/name === WindowSettings/);
  });

  it('re-applies the persisted pin, not a hard-coded level', () => {
    expect(handler).toMatch(/SetAlwaysOnTop\(\$pinnedStore\)/);
    expect(handler).not.toMatch(/SetAlwaysOnTop\((true|false)\)/);
  });

  it('does not clear the translation when Settings closes', () => {
    // The Settings branch must return before the translate-window reset below it.
    const settingsBranch = handler.slice(0, handler.indexOf('name !== WindowTranslate'));
    expect(settingsBranch).toMatch(/return;/);
  });
});
