import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// Issue #10: the System engine's speed mode, frontend wiring. No Svelte render harness exists
// (#78), so this pins the wiring by reading the component source like the other wiring-pin
// tests in this directory (see copyKeyFailedToast.test.ts); the pure rules it builds on are
// unit-tested in utils/translateProgress.test.ts.

const src = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');
const enginesTab = readFileSync(resolve(__dirname, 'settings/EnginesTab.svelte'), 'utf8');

describe('progress time-left wiring (issue #10)', () => {
  it('feeds the started event estimate into the progress state', () => {
    expect(src).toMatch(/startProgress\(payload\.engine, now, payload\.estimated_ms\)/);
  });

  it('words the time-left after every progress line that has one', () => {
    expect(src).toMatch(/translate\.timeLeft/);
    expect(src).toMatch(/line\.remainingMs !== undefined/);
    // All three lines append it: the composition is shared (`left`), and the two named kinds
    // (chunk, stalled) plus the workingOn fallback all end with it.
    const m = src.match(/function progressText\(line: ProgressLine\): string \{[\s\S]*?\n  \}/);
    expect(m, 'progressText not found').not.toBeNull();
    expect(m![0]).toContain(`'chunk'`);
    expect(m![0]).toContain(`'stalled'`);
    expect(m![0]).toContain('workingOn');
    expect(m![0]).toMatch(/return t\('translate\.workingOn'[\s\S]*\+ left;/);
  });
});

describe('faster-setting suggestion wiring (issue #10)', () => {
  it('shows the suggestion toast once per request, never a silent switch', () => {
    expect(src).toMatch(/payload\.suggest_fast && payload\.estimated_ms && !suggestedFast/);
    expect(src).toMatch(/suggestedFast = true/);
    // Reset with the rest of the per-request state, in doTranslate and in clearInput.
    expect((src.match(/suggestedFast = false/g) ?? []).length).toBeGreaterThanOrEqual(2);
  });

  it('uses the fastSuggestion copy with the estimated time', () => {
    expect(src).toMatch(
      /translate\.fastSuggestion', \{ time: formatClock\(payload\.estimated_ms\)/,
    );
  });
});

describe('result notice wiring (issue #10)', () => {
  it('renders the backend notice as a muted note in the result pane', () => {
    expect(src).toMatch(/\{#if activeResult\.notice\}/);
    expect(src).toMatch(/data-testid="result-notice"/);
    expect(src).toMatch(/\{activeResult\.notice\}/);
  });
});

describe('settings toggle wiring (issue #10)', () => {
  it('persists fast_system_translate through GetConfig/SaveConfig', () => {
    expect(enginesTab).toMatch(/cfg\?\.fast_system_translate \?\? false/);
    expect(enginesTab).toMatch(/fast_system_translate: enabled/);
    expect(enginesTab).toMatch(/data-testid="fast-system-translate"/);
  });

  it('sits in the System engine config panel (the Translation Engines tab)', () => {
    const m = enginesTab.match(
      /schema\?\.kind === 'translate' && schema\?\.builtin[\s\S]{0,2500}fastSystemTranslate/,
    );
    expect(m, 'the toggle must live in the builtin translate engine config card').not.toBeNull();
  });
});

describe('issue #10 copy exists and differs between locales', () => {
  it('timeLeft', () => {
    expect(en.translate.timeLeft).toBeTruthy();
    expect(zh.translate.timeLeft).toBeTruthy();
    expect(en.translate.timeLeft).not.toBe(zh.translate.timeLeft);
    expect(en.translate.timeLeft).toContain('{time}');
    expect(zh.translate.timeLeft).toContain('{time}');
  });
  it('fastSuggestion', () => {
    expect(en.translate.fastSuggestion).toBeTruthy();
    expect(zh.translate.fastSuggestion).toBeTruthy();
    expect(en.translate.fastSuggestion).not.toBe(zh.translate.fastSuggestion);
    expect(en.translate.fastSuggestion).toContain('{time}');
    expect(zh.translate.fastSuggestion).toContain('{time}');
  });
  it('settings labels and hints', () => {
    expect(en.settings.fastSystemTranslate).toBeTruthy();
    expect(zh.settings.fastSystemTranslate).toBeTruthy();
    expect(en.settings.fastSystemTranslateHint).toBeTruthy();
    expect(zh.settings.fastSystemTranslateHint).toBeTruthy();
  });
});
