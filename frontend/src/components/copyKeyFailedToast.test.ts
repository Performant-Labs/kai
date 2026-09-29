import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// Issue #175 item 5: a failed simulated copy (Go's TriggerInput copy-key branch captured
// nothing) used to be silent on the frontend — the translate window still comes to the front,
// but with no EventInputFill the input box keeps showing whatever text a previous session left
// in it (issue #81's retained session), reading exactly like the old text being the new,
// correct selection. The fix is EventCopyKeyFailed (Go) → a toast (frontend) so a failed
// capture is visibly a failure. No Svelte render harness exists (#78, see
// screenshotRecaptureTooltip.test.ts), so this reads the component source like the other
// wiring-pin tests in this directory.

const src = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');

describe('EventCopyKeyFailed wiring (issue #175 item 5)', () => {
  it('is imported from the shared events module', () => {
    expect(src).toMatch(/EventCopyKeyFailed/);
    // Imported alongside the other event constants, not a local re-declaration.
    expect(src).toMatch(/from\s+'\.\.\/utils\/events'/);
  });

  it('registers a listener that shows a toast with the copyKeyFailed copy', () => {
    const m = src.match(/onEvent\(EventCopyKeyFailed,\s*\(\)\s*=>\s*\{[\s\S]*?\}\);/);
    expect(m, 'no onEvent(EventCopyKeyFailed, ...) handler found').not.toBeNull();
    expect(m![0]).toMatch(/showToast\(\s*t\('translate\.copyKeyFailed'\)/);
  });

  it('unsubscribes the listener on cleanup, like every other onEvent subscription here', () => {
    const offDecl = src.match(/const\s+(offCopyKeyFailed)\s*=\s*onEvent\(EventCopyKeyFailed/);
    expect(offDecl, 'listener must be captured into an off* unsubscribe variable').not.toBeNull();
    const name = offDecl![1];
    // The cleanup block (`return () => { ... }`) inside onMount must call it, or the listener
    // leaks/re-registers across (un)mounts.
    const cleanup = src.match(/return\s*\(\)\s*=>\s*\{[\s\S]*?\};/);
    expect(cleanup, 'no onMount cleanup block found').not.toBeNull();
    expect(cleanup![0]).toContain(`${name}();`);
  });
});

describe('translate.copyKeyFailed copy (issue #175 item 5)', () => {
  it('exists, is non-empty, and differs between locales', () => {
    expect(en.translate.copyKeyFailed).toBeTruthy();
    expect(zh.translate.copyKeyFailed).toBeTruthy();
    expect(en.translate.copyKeyFailed).not.toBe(zh.translate.copyKeyFailed);
  });
});
