import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// Issue #173 item 6, added in response to a PR review finding: the recapture button's
// descriptive tooltip (title={t('screenshot.recaptureHint')}) had no test pinning the
// binding — tooltipLength.test.ts only checks the copy's word count, so removing the
// `title` attribute entirely would leave every existing test green. No Svelte render
// harness exists (#78), so this reads the component source, like sourceUndo.test.ts /
// translateWindowGear.test.ts.

const src = readFileSync(resolve(__dirname, 'ScreenshotWindow.svelte'), 'utf8');

// The <button ...> opening tag through its closing </button> that calls onclick={recapture}.
function recaptureButton(): string | null {
  for (const m of src.matchAll(/<button[\s\S]*?<\/button>/g)) {
    if (m[0].includes('onclick={recapture}')) return m[0];
  }
  return null;
}

describe('screenshot recapture button tooltip (issue #173)', () => {
  it('exists and is titled with the descriptive screenshot.recaptureHint key, not the bare label', () => {
    const btn = recaptureButton();
    expect(btn, 'a <button onclick={recapture}> not found').not.toBeNull();
    expect(btn).toMatch(/title=\{t\('screenshot\.recaptureHint'\)\}/);
    // The visible label stays the short screenshot.recapture key — recaptureHint must not
    // also be used as the button's text, or the button would show a full sentence.
    expect(btn).toContain(`{t('screenshot.recapture')}`);
  });

  it('recaptureHint is non-empty and distinct from the short recapture label, in both locales', () => {
    expect(en.screenshot.recaptureHint).toBeTruthy();
    expect(en.screenshot.recaptureHint).not.toBe(en.screenshot.recapture);
    expect(zh.screenshot.recaptureHint).toBeTruthy();
    expect(zh.screenshot.recaptureHint).not.toBe(zh.screenshot.recapture);
  });
});
