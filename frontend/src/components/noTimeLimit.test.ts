import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// issue #109 (Tester, RED): source-contract tests for the no-time-limit rework. No Svelte render
// harness exists (#78), so, like swapWindow.test.ts and sessionRetention.test.ts, these read the
// component sources. The wiring is pinned by what it must contain, not by variable names, wherever
// a name is F's choice (`requestId` in the translate window is the exception: the brief names it).
const dir = __dirname;
const read = (f: string) => readFileSync(resolve(dir, f), 'utf8');
const translateWindow = read('TranslateWindow.svelte');
const screenshotWindow = read('ScreenshotWindow.svelte');
const card = read('TranslateCard.svelte');
const enginesTab = read('settings/EnginesTab.svelte');

// Needles are assembled at run time so that this file itself never contains the literals the
// criterion says must not appear anywhere under frontend/src.
const FIFTEEN_SECONDS = '150' + '00';
const OLD_FALLBACK = 'any' + 'Pending';

function walk(d: string, out: string[] = []): string[] {
  for (const n of readdirSync(d)) {
    if (n === 'node_modules') continue;
    const p = join(d, n);
    if (statSync(p).isDirectory()) walk(p, out);
    else if (/\.(ts|svelte)$/.test(n)) out.push(p);
  }
  return out;
}

// The body of `const <name> = onEvent(...)`, from its declaration to the closing `});` at the given
// indent (the offClosing test below found its handler the same way). An assertion made on this slice
// cannot be satisfied by code elsewhere in the file: the loose whole-file patterns this replaces
// still matched with the behaviour they name deleted (issue #109, S rework).
function handlerBody(src: string, name: string, indent: number): string {
  const m = src.match(new RegExp(`const ${name} = onEvent\\([\\s\\S]*?\\n {${indent}}\\}\\);`));
  expect(m, `${name} handler not found`).not.toBeNull();
  return m![0];
}
// The body of `function <name>() {...}`, closed by the `}` at the given indent.
function functionBody(src: string, name: string, indent: number): string {
  const m = src.match(new RegExp(`function ${name}\\([^)]*\\) \\{[\\s\\S]*?\\n {${indent}}\\}`));
  expect(m, `${name}() not found`).not.toBeNull();
  return m![0];
}

describe('no timer decides a translation is over (criterion 12)', () => {
  it('neither the 15 s timer literal nor the old fallback predicate exists anywhere under frontend/src', () => {
    const offenders: string[] = [];
    for (const f of walk(resolve(dir, '..'))) {
      const s = readFileSync(f, 'utf8');
      if (s.includes(FIFTEEN_SECONDS) || s.includes(OLD_FALLBACK)) offenders.push(f);
    }
    expect(offenders).toEqual([]);
  });
});

describe('translate window request lifecycle (criteria 12, 14)', () => {
  it('doTranslate names the request itself and sends it as request_id', () => {
    expect(translateWindow).toMatch(/requestId = newRequestID\(\)/);
    expect(translateWindow).toMatch(/TranslateMulti\(\{[\s\S]*?request_id: requestId/);
  });
  it('events from another request are ignored, for results and for progress', () => {
    expect(handlerBody(translateWindow, 'offResult', 4)).toMatch(/if \(payload\.request_id !== requestId\) return;/);
    expect(handlerBody(translateWindow, 'offProgress', 4)).toMatch(/payload\.request_id !== requestId\) return;/);
  });
  it('a rejected TranslateMulti closes the wait at once', () => {
    expect(translateWindow).toMatch(
      /console\.error\(t\('log\.translateRequestFailed'\), e\);[\s\S]{0,300}awaiting = false/,
    );
  });
  it('the Translate button reads the open request (awaiting), not the first-result loading flag', () => {
    expect(translateWindow).toMatch(/disabled=\{awaiting \|\| !input\.trim\(\)\}/);
    expect(translateWindow).toMatch(/\{awaiting \? t\('common\.loading'\) : t\('translate\.button'\)\}/);
    expect(translateWindow).not.toMatch(/disabled=\{loading \|\| !input\.trim\(\)\}/);
  });
  it('the request-level Cancel calls CancelTranslate(requestId, "") and only shows while the request is open', () => {
    expect(translateWindow).toMatch(/CancelTranslate\(requestId, ''\)/);
    expect(translateWindow).toMatch(/\{#if awaiting\}[\s\S]{0,600}t\('translate\.cancel'\)/);
  });
  it('the active engine placeholder carries a per-engine Cancel', () => {
    expect(translateWindow).toMatch(/CancelTranslate\(requestId, (activeEngine|[A-Za-z.]*[eE]ngine[A-Za-z.]*)\)/);
    expect(translateWindow).toContain("t('translate.cancelEngine'");
  });
  it('the progress line comes from the pure progressLine and a 1 s interval that is cleared', () => {
    expect(translateWindow).toContain('progressLine(');
    expect(translateWindow).toContain("t('translate.workingOn'");
    expect(translateWindow).toContain("t('translate.stillWaiting'");
    expect(translateWindow).toContain("t('translate.part'");
    expect(translateWindow).toMatch(/setInterval\([\s\S]*?1000\)/);
    expect(translateWindow).toContain('clearInterval(');
  });
  it('closing the translate window does not cancel the request (D10)', () => {
    // Any cancel path counts, not only the literal binding call: cancelRequest() and cancelEngine()
    // reach the backend just the same.
    expect(handlerBody(translateWindow, 'offClosing', 4)).not.toMatch(/CancelTranslate|cancelRequest|cancelEngine/);
  });
});

describe('cancelled is not failed, on every surface (criterion 13)', () => {
  it('the translate pane has a cancelled branch with muted copy, never the failure copy or danger colour', () => {
    const m = translateWindow.match(/pane === 'cancelled'\}?[\s\S]*?(?=\{:else if|\{\/if\})/);
    expect(m, "no pane === 'cancelled' branch").not.toBeNull();
    expect(m![0]).toContain("t('translate.cancelled')");
    expect(m![0]).not.toContain('translate.failed');
    expect(m![0]).not.toContain('--app-danger');
  });
  it('the cancelled dot has its own tooltip copy', () => {
    expect(translateWindow).toContain("t('translate.engineCancelled')");
  });
  it('TranslateCard shows a muted cancelled badge instead of the failed badge', () => {
    const m = card.match(/\{#if [^}]*cancelled[^}]*\}([\s\S]*?)\{:else/);
    expect(m, 'no tr.cancelled branch in TranslateCard').not.toBeNull();
    expect(m![1]).toContain("t('screenshot.cancelled')");
    expect(m![1]).not.toContain('--app-danger');
    expect(m![1]).not.toContain('screenshot.translateFailed');
  });
});

describe('screenshot window (criteria 11, 14, 16)', () => {
  it('adopts its request id from its own pushes and ignores foreign progress events', () => {
    expect(handlerBody(screenshotWindow, 'offProgress', 2)).toMatch(/payload\.request_id !== requestId\) return;/);
  });
  it('has a per-engine Cancel and a Cancel-all control', () => {
    expect(screenshotWindow).toMatch(/CancelTranslate\([^,)]+, [^')][^)]*\)/); // (requestId, engine)
    expect(screenshotWindow).toMatch(/CancelTranslate\([^,)]+, ''\)/); // (requestId, '')
    expect(screenshotWindow).toContain("t('translate.cancel'");
  });
  it('closing the screenshot window also cancels its in-flight request', () => {
    // The close handler abandons the run, and abandoning the run is what cancels it.
    expect(handlerBody(screenshotWindow, 'offClosing', 2)).toMatch(/abandonRun\(\);/);
    expect(functionBody(screenshotWindow, 'abandonRun', 2)).toMatch(/CancelTranslate\(requestId, ''\)/);
  });
});

describe('LLM timeout setting removed (criterion 3)', () => {
  it('EnginesTab has no llm_timeout widget and no llmTimeoutSec state', () => {
    expect(enginesTab).not.toContain('llm_timeout');
    expect(enginesTab).not.toContain('llmTimeoutSec');
  });
  it('the catalogs drop both llm_timeout strings', () => {
    const flat = (o: unknown, p = ''): string[] =>
      o && typeof o === 'object'
        ? Object.entries(o as Record<string, unknown>).flatMap(([k, v]) => flat(v, p ? `${p}.${k}` : k))
        : [p];
    for (const cat of [en, zh]) {
      expect(flat(cat).filter((k) => k.endsWith('llm_timeout'))).toEqual([]);
    }
  });
});

describe('i18n copy for the new surface (criterion 17)', () => {
  const get = (o: unknown, path: string): unknown =>
    path.split('.').reduce<unknown>((a, k) => (a as Record<string, unknown> | undefined)?.[k], o);
  const keys = [
    'translate.cancel',
    'translate.cancelEngine',
    'translate.cancelled',
    'translate.workingOn',
    'translate.stillWaiting',
    'translate.part',
    'translate.engineCancelled',
    'screenshot.cancelled',
  ];
  it.each(keys)('%s exists, non-empty, in en-US and zh-CN', (k) => {
    for (const cat of [en, zh]) {
      const v = get(cat, k);
      expect(typeof v).toBe('string');
      expect((v as string).length).toBeGreaterThan(0);
    }
  });
  it('the parameterised strings keep their placeholders in both languages', () => {
    for (const cat of [en, zh]) {
      expect(get(cat, 'translate.cancelEngine') as string).toContain('{engine}');
      expect(get(cat, 'translate.workingOn') as string).toContain('{engine}');
      expect(get(cat, 'translate.stillWaiting') as string).toContain('{engine}');
      expect(get(cat, 'translate.part') as string).toContain('{done}');
      expect(get(cat, 'translate.part') as string).toContain('{total}');
    }
  });
});

describe('events mirror (criterion 10)', () => {
  it('utils/events.ts mirrors the progress event name', () => {
    const src = readFileSync(resolve(dir, '../utils/events.ts'), 'utf8');
    expect(src).toMatch(/export const EventTranslateProgress = 'kai:translate:progress'/);
  });
});
