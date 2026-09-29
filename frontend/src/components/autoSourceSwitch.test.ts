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

const LEARN_OR_PERSIST = [
  'learnLangVariant',
  'LearnLangVariant',
  'learnFromSelection',
  'persistLangs',
  'onLangPicked',
  'SaveConfig',
  'default_from',
  'default_to',
];

describe('one shared switch function', () => {
  it('asks the backend planner from exactly one place', () => {
    expect(win).toContain('PlanSourceSwitch');
    expect(win.match(/PlanSourceSwitch\(/g)?.length).toBe(1);
    expect(fnBody('autoSwitchSource')).toContain('PlanSourceSwitch(');
  });

  it('imports the planner from the translate wrapper bindings and the gate from the pure module', () => {
    expect(win).toMatch(/PlanSourceSwitch[\s\S]*translatewrapper\.ts/);
    expect(win).toContain("from '../utils/sourceSwitch.ts'");
  });

  it('applies the pair by assigning the two selects and nothing else', () => {
    const b = fnBody('autoSwitchSource');
    expect(b).toMatch(/\bfromLang\s*=/);
    expect(b).toMatch(/\btoLang\s*=/);
    expect(b).toMatch(/acceptSwitch\(/);
  });

  it('never reaches the learn or persist code (an automatic switch is not a choice)', () => {
    for (const fn of [
      'autoSwitchSource',
      'restoreBeforeSwitch',
      'translateWithSwitch',
      'translateIfSwitched',
    ]) {
      const b = fnBody(fn);
      for (const s of LEARN_OR_PERSIST) expect(b, `${fn} reaches ${s}`).not.toContain(s);
    }
  });

  it('does not switch an Auto source and skips text the user undid', () => {
    const b = fnBody('autoSwitchSource');
    expect(b).toContain('TRANSLATE_LANG.Auto');
    expect(b).toMatch(/isDeclined\(/);
  });

  it('checks the setting through the backend: no frontend copy of the decision', () => {
    const b = fnBody('autoSwitchSource');
    for (const s of ['utf8', '.length <', 'confidence', 'SameAs', 'Covers']) {
      expect(b, s).not.toContain(s);
    }
  });

  it('drops a plan that arrives after the text or the pair changed', () => {
    const b = fnBody('autoSwitchSource');
    expect(b).toMatch(/input\s*!==\s*text/);
  });
});

describe('every arrival path goes through it', () => {
  it('the hotkey / tray / auto-clipboard fill (EventInputFill) fills, then translates with the switch', () => {
    const i = win.indexOf('onEvent(EventInputFill');
    expect(i).toBeGreaterThan(-1);
    const body = win.slice(i, win.indexOf('\n    });', i));
    const fill = body.search(/setSource\(\s*text\s*,\s*'program'\s*\)/);
    expect(fill).toBeGreaterThan(-1);
    expect(fill).toBeLessThan(body.indexOf('translateWithSwitch('));
    expect(body).not.toMatch(/\bdoTranslate\(/);
    // A new arrival is a new text: an earlier undo of another text does not carry over.
    expect(body).toMatch(/declinedText\s*=\s*''/);
  });

  it('Translate on typed text (button and Cmd+Enter) goes through it too', () => {
    expect(win).toMatch(/onclick=\{translateWithSwitch\}/);
    const shortcut = fnBody('handleTranslateShortcut');
    expect(shortcut).toContain('translateWithSwitch(');
    expect(shortcut).not.toMatch(/\bdoTranslate\(/);
  });

  it('a paste into the source pane checks the pair and translates only when it switched', () => {
    const b = fnBody('onSourceInput');
    expect(b).toMatch(/insertFromPaste/);
    expect(b).toContain('translateIfSwitched(');
    expect(b).not.toMatch(/\bdoTranslate\(/);
    const h = fnBody('translateIfSwitched');
    expect(h).toContain('autoSwitchSource(');
    expect(h).toContain('doTranslate()');
  });

  it('typing checks nothing per keystroke', () => {
    const b = fnBody('onSourceInput');
    // The check is only ever reached through the paste / drop branch (cut and undo check nothing).
    const at = b.indexOf('translateIfSwitched(');
    expect(b.slice(Math.max(0, at - 200), at)).toMatch(/insertFromPaste/);
    expect(b.slice(Math.max(0, at - 200), at)).not.toMatch(/deleteByCut/);
  });

  it('a result that still carries the engine detection (#162) retries through the same helper', () => {
    const i = win.indexOf('onEvent(EventTranslateResult');
    const body = win.slice(i, win.indexOf('\n    });', i));
    expect(body).toContain('translateIfSwitched(');
    expect(body).toContain('detected_from');
  });

  it('every doTranslate caller is a switch-aware function or the deliberate no-switch swap/undo', () => {
    // Anything else calling doTranslate() directly would be an arrival path that forgot the switch.
    const allowed = ['swap', 'restoreBeforeSwitch', 'translateWithSwitch', 'translateIfSwitched'];
    const stray: string[] = [];
    const code = win
      .replace(/<!--[\s\S]*?-->/g, '')
      .replace(/^\s*\/\/.*$/gm, '')
      .replace(/\s\/\/ .*$/gm, '');
    for (const m of code.matchAll(/(?<![\w.])doTranslate\(\)/g)) {
      const before = code.slice(0, m.index!);
      const fn =
        [...before.matchAll(/\n  (?:async )?function (\w+)\(/g)].pop()?.[1] ?? '(top level)';
      if (!allowed.includes(fn)) stray.push(fn);
    }
    expect(stray).toEqual([]);
  });
});

describe('cue and undo', () => {
  it('renders the cue as a muted note in the result pane, with the languages named', () => {
    const m = /\{#if cueShown\}([\s\S]*?)\{\/if\}/.exec(win);
    expect(m, 'cue block').not.toBeNull();
    expect(m![1]).toContain('u-muted');
    expect(m![1]).toContain('data-testid="source-switched-note"');
    expect(m![1]).toContain("t('translate.sourceSwitched'");
  });

  it('the cue sits inside the result branch, next to the other notes', () => {
    const branch = win.indexOf("{:else if pane === 'result' && activeResult}");
    const cue = win.indexOf('{#if cueShown}');
    const detected = win.indexOf('{#if activeResult.detected_from');
    expect(branch).toBeGreaterThan(-1);
    expect(cue).toBeGreaterThan(branch);
    expect(cue).toBeLessThan(detected);
  });

  it('swap restores the previous pair while the cue is active, and keeps the source text', () => {
    const b = fnBody('swap');
    expect(b).toContain('undoPair(');
    expect(b).toContain('restoreBeforeSwitch(');
    const r = fnBody('restoreBeforeSwitch');
    expect(r).toMatch(/declinedText\s*=\s*input/);
    expect(r).not.toContain('setSource(');
    expect(r).toContain('doTranslate()');
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
