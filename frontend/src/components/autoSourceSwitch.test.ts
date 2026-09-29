import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// Issue #200 (T, RED): text that arrives in another language than the pinned source switches the
// source dropdown, replaces the target with the old source and translates, for EVERY way text
// arrives. The repo has no Svelte render harness (#28), so the wiring is pinned from the component
// source, as detectedSourceNote.test.ts does. The decision itself is the backend's
// (PlanSourceSwitch, internal/translate/source_switch_test.go); this pins that every path asks
// through the ONE shared function, that the switch cannot reach the learn/persist code, and that
// the cue reuses the result pane's note styling with the swap button as undo.

const read = (f: string) => readFileSync(resolve(__dirname, f), 'utf8');
const win = read('TranslateWindow.svelte');
const general = read('settings/GeneralTab.svelte');

function fnBody(name: string): string {
  const m = new RegExp(`\\n  (?:async )?function ${name}\\(`).exec(win);
  expect(m, `${name} not found`).not.toBeNull();
  const start = m!.index + 1;
  const ends = [
    win.indexOf('\n  function ', start + 1),
    win.indexOf('\n  async function ', start + 1),
    win.indexOf('\n  const ', start + 1),
    win.indexOf('\n  let ', start + 1),
  ].filter((i) => i > -1);
  return win.slice(start, ends.length ? Math.min(...ends) : undefined);
}

const flow = read('sourceSwitchFlow.ts');

// The behaviour of the flow is executed in sourceSwitchFlow.test.ts (and the whole window in
// mountedSourceSwitch.test.ts where it can be mounted). What is pinned from source here is only the
// wiring that a behaviour test of the module cannot see: that the window builds its switcher from
// the real binding and routes each arrival path into it.
describe('the window is wired to the one flow', () => {
  it('asks the backend planner from exactly one place, through the binding', () => {
    expect(win.match(/PlanSourceSwitch\(/g)?.length).toBe(1);
    expect(win).toMatch(/PlanSourceSwitch[\s\S]*translatewrapper\.ts/);
    expect(win).toContain('createSourceSwitcher(');
  });

  it('the flow module cannot reach the learn or persist code', () => {
    for (const s of [
      'learnLangVariant',
      'LearnLangVariant',
      'learnFromSelection',
      'persistLangs',
      'SaveConfig',
      'localStorage',
      'default_from',
      'default_to',
    ]) {
      expect(flow, s).not.toContain(s);
    }
    // ...and the deps the window gives it assign the selects only.
    const i = win.indexOf('createSourceSwitcher(');
    const deps = win.slice(i, win.indexOf('\n  });', i));
    for (const s of ['learnLangVariant', 'persistLangs', 'onLangPicked', 'SaveConfig']) {
      expect(deps, s).not.toContain(s);
    }
  });

  it('the fill (hotkey, tray, auto-clipboard) marks a new arrival, then translates with the switch', () => {
    const i = win.indexOf('onEvent(EventInputFill');
    const body = win.slice(i, win.indexOf('\n    });', i));
    expect(body.search(/setSource\(\s*text\s*,\s*'program'\s*\)/)).toBeLessThan(
      body.indexOf('switcher.newArrival()'),
    );
    expect(body.indexOf('switcher.newArrival()')).toBeLessThan(
      body.indexOf('translateWithSwitch('),
    );
    expect(body).not.toMatch(/\bdoTranslate\(/);
  });

  it('the Translate button and Cmd+Enter translate with the switch', () => {
    expect(win).toMatch(/onclick=\{translateWithSwitch\}/);
    expect(fnBody('handleTranslateShortcut')).toContain('translateWithSwitch(');
    expect(fnBody('handleTranslateShortcut')).not.toMatch(/\bdoTranslate\(/);
  });

  it('only a paste or drop, never typing or a cut, reaches translateIfSwitched', () => {
    const b = fnBody('onSourceInput');
    const at = b.indexOf('translateIfSwitched(');
    expect(at).toBeGreaterThan(-1);
    expect(b.slice(Math.max(0, at - 200), at)).toMatch(/insertFromPaste/);
    expect(b.slice(Math.max(0, at - 200), at)).not.toMatch(/deleteByCut/);
  });

  it('a result that still carries the engine detection (#162) retries through the same flow, once', () => {
    const i = win.indexOf('onEvent(EventTranslateResult');
    const body = win.slice(i, win.indexOf('\n    });', i));
    expect(body).toContain('translateIfSwitched(');
    expect(body).toContain('detected_from');
    expect(body).toContain('hintedRequestId');
  });

  it('swap asks the flow to undo first', () => {
    expect(fnBody('swap')).toMatch(/switcher\.undo\(\)/);
  });
});

describe('the cue', () => {
  it('renders as a muted note in the result pane, with the languages named', () => {
    const m = /\{#if cueShown\}([\s\S]*?)\{\/if\}/.exec(win);
    expect(m, 'cue block').not.toBeNull();
    expect(m![1]).toContain('u-muted');
    expect(m![1]).toContain('data-testid="source-switched-note"');
    expect(m![1]).toContain("t('translate.sourceSwitched'");
  });

  it('sits inside the result branch, next to the other notes', () => {
    const branch = win.indexOf("{:else if pane === 'result' && activeResult}");
    const cue = win.indexOf('{#if cueShown}');
    expect(cue).toBeGreaterThan(branch);
    expect(cue).toBeLessThan(win.indexOf('{#if activeResult.detected_from'));
  });
});

describe('the setting', () => {
  it('has a switch with a tooltip in the general settings that saves auto_switch_source', () => {
    expect(general).toContain('auto_switch_source');
    expect(general).toContain("t('settings.autoSwitchSource')");
    expect(general).toMatch(/title=\{t\('settings\.autoSwitchSourceHint'\)\}/);
    expect(general).toMatch(/cfg\.auto_switch_source\s*\?\?\s*true/);
  });

  it('has its strings in both locales', () => {
    for (const [name, cat] of [
      ['en', en],
      ['zh', zh],
    ] as const) {
      expect(cat.settings.autoSwitchSource, name).toBeTruthy();
      expect(cat.settings.autoSwitchSourceHint, name).toBeTruthy();
      expect(cat.translate.sourceSwitched, name).toBeTruthy();
      expect(cat.translate.sourceSwitched, name).toContain('{from}');
      expect(cat.translate.sourceSwitched, name).toContain('{to}');
    }
  });

  it('the English tooltip stays short, like the other tooltips', () => {
    const words = en.settings.autoSwitchSourceHint.trim().split(/\s+/).length;
    expect(words).toBeGreaterThan(2);
    expect(words).toBeLessThanOrEqual(25);
  });
});
