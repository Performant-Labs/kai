import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Issue #81: idle pane + retained session. No Svelte render harness (#28), so this pins the
// contract from the component source, in the style of swapWindow.test.ts.

const src = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');

function fnBody(name: string): string {
  const start = src.indexOf(`function ${name}(`);
  expect(start, `${name} not found`).toBeGreaterThan(-1);
  const ends = [src.indexOf('\n  function ', start + 1), src.indexOf('\n  async function ', start + 1)].filter(
    (i) => i > -1,
  );
  return src.slice(start, ends.length ? Math.min(...ends) : undefined);
}

/** The EventWindowClosing handler's body (from its onEvent call to the matching `});`). */
function closingHandler(): string {
  const start = src.indexOf('onEvent(EventWindowClosing');
  expect(start, 'closing handler not found').toBeGreaterThan(-1);
  const end = src.indexOf('\n    });', start);
  expect(end).toBeGreaterThan(start);
  return src.slice(start, end);
}

/** The translate-window branch: everything after the `name !== WindowTranslate` guard. */
function afterTranslateGuard(): string {
  const h = closingHandler();
  const i = h.indexOf('name !== WindowTranslate');
  expect(i, 'WindowTranslate guard not found').toBeGreaterThan(-1);
  return h.slice(i);
}

describe('closing the translate window retains the session', () => {
  it('keeps the WindowSettings pin branch and the WindowTranslate guard', () => {
    const h = closingHandler();
    expect(h).toContain('name === WindowSettings');
    expect(h).toMatch(/if\s*\(\s*name\s*!==\s*WindowTranslate\s*\)\s*return/);
  });

  it('no longer clears input / results / edited on close', () => {
    const tail = afterTranslateGuard();
    expect(tail).not.toMatch(/\binput\s*=/);
    expect(tail).not.toMatch(/\bresults\s*=/);
    expect(tail).not.toMatch(/\bedited\s*=/);
  });

  it('no code path resets fromLang / toLang on close', () => {
    expect(closingHandler()).not.toMatch(/\b(fromLang|toLang)\s*=/);
  });
});

describe('session retention wiring', () => {
  it('uses the persisted store with the session key and validates via restoreSession', () => {
    expect(src).toMatch(/persisted<TranslateSession>\(\s*'kai:translate:session'\s*,\s*emptySession\(\)\s*\)/);
    expect(src).toContain('restoreSession(');
    expect(src).toContain("from '../utils/translateSession.ts'");
  });

  it('does not touch localStorage directly for the session', () => {
    expect(src).toContain('persisted<TranslateSession>');
    // the key appears once, as the persisted store's key
    expect(src.match(/kai:translate:session/g)?.length).toBe(1);
    // the only raw localStorage calls are the pre-existing #39 pin migration
    for (const m of src.matchAll(/localStorage\s*\.\s*\w+\(([^)]*)\)/g)) {
      expect(m[1]).not.toMatch(/session/i);
    }
  });

  it('writes the session back from an $effect over input, results, requestedTo and requested', () => {
    const effects = [...src.matchAll(/\$effect\(\(\)\s*=>\s*\{([\s\S]*?)\n {2}\}\);/g)].map((x) => x[1]);
    const writer = effects.find((b) => /sessionStore\.set\(|\$sessionStore\s*=/.test(b));
    expect(writer, 'no $effect writes the session store').toBeDefined();
    for (const f of ['input', 'results', 'requestedTo', 'requested']) {
      expect(writer!, `effect does not include ${f}`).toMatch(new RegExp(`\\b${f}\\b`));
    }
  });

  it('never persists loading or edited', () => {
    const effects = [...src.matchAll(/\$effect\(\(\)\s*=>\s*\{([\s\S]*?)\n {2}\}\);/g)].map((x) => x[1]);
    const writer = effects.find((b) => /sessionStore\.set\(|\$sessionStore\s*=/.test(b)) ?? '';
    expect(writer, 'no $effect writes the session store').not.toBe('');
    expect(writer).not.toMatch(/\bloading\b/);
    expect(writer).not.toMatch(/\bedited\b/);
  });
});

describe('requested flag and Clear', () => {
  it('doTranslate sets requested = true', () => {
    expect(fnBody('doTranslate')).toMatch(/\brequested\s*=\s*true/);
  });

  it('clearInput resets requested, input, results and edited', () => {
    const b = fnBody('clearInput');
    expect(b).toMatch(/\brequested\s*=\s*false/);
    expect(b).toMatch(/setSource\(\s*''\s*,\s*'program'\s*\)/);
    expect(b).toMatch(/\bresults\s*=\s*\{\}/);
    expect(b).toMatch(/\bedited\s*=\s*new Map/);
  });

  it('EventInputFill still replaces the text and translates', () => {
    const i = src.indexOf('onEvent(EventInputFill');
    expect(i).toBeGreaterThan(-1);
    const body = src.slice(i, src.indexOf('\n    });', i));
    expect(body).toMatch(/setSource\(\s*text\s*,\s*'program'\s*\)/);
    // Since #200 the fill translates through the switch-aware wrapper, which calls doTranslate.
    expect(body).toMatch(/translateWithSwitch\(\)/);
  });
});

describe('result pane chain', () => {
  it('imports and uses paneState', () => {
    expect(src).toMatch(/import\s*\{[^}]*\bpaneState\b[^}]*\}\s*from '\.\.\/utils\/resultPane\.ts'/);
    expect(src).toMatch(/paneState\(/);
  });

  it('the failure headline is rendered only in the failed branch, never as an unconditional else', () => {
    // #96: the generic copy lives in failureMessage; the branch renders failure.headline.
    const idx = src.indexOf('failure.headline');
    expect(idx).toBeGreaterThan(-1);
    const before = src.slice(Math.max(0, idx - 900), idx);
    // the nearest preceding branch marker must be a paneState 'failed' test, not a bare {:else}
    const markers = [...before.matchAll(/\{:else if[^}]*\}|\{:else\}|\{#if[^}]*\}/g)];
    const last = markers[markers.length - 1]?.[0] ?? '';
    expect(last).toMatch(/failed/);
  });

  it('passes requested to the status dots', () => {
    expect(src).toMatch(/statusDots\([^)]*requested[^)]*\)/);
  });
});

describe('languages are retained (unchanged mechanism)', () => {
  it('loadDefaults restores fromLang/toLang from default_from/default_to and runs on mount', () => {
    const b = fnBody('loadDefaults');
    expect(b).toMatch(/fromLang\s*=\s*cfg\.default_from/);
    expect(b).toMatch(/toLang\s*=\s*cfg\.default_to/);
    expect(src).toMatch(/Promise\.all\(\[\s*loadDefaults\(\)/);
  });

  it('persistLangs saves both languages to default_from/default_to', () => {
    const b = fnBody('persistLangs');
    expect(b).toMatch(/default_from:\s*fromLang/);
    expect(b).toMatch(/default_to:\s*toLang/);
  });
});
