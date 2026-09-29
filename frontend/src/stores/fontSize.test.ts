import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #195: the text-size setting. The REAL store and constants run in jsdom against the real
// documentElement and localStorage; only the Wails boundary (events + generated bindings) is faked.

const h = vi.hoisted(() => {
  const handlers = new Map<string, Array<(d: unknown) => void>>();
  return {
    handlers,
    fire(name: string, data: unknown) {
      for (const cb of handlers.get(name) ?? []) cb(data);
    },
    config: { font_size: 120, language: 'en-US' } as Record<string, unknown> | null,
    getConfigFails: false,
    saveConfig: vi.fn(),
  };
});

vi.mock('../runtime', () => ({
  onEvent: (name: string, cb: (d: unknown) => void) => {
    h.handlers.set(name, [...(h.handlers.get(name) ?? []), cb]);
    return () =>
      h.handlers.set(
        name,
        (h.handlers.get(name) ?? []).filter((x) => x !== cb),
      );
  },
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetConfig: async () => {
    if (h.getConfigFails) throw new Error('no backend');
    return h.config ? { ...h.config } : h.config;
  },
  SaveConfig: (c: unknown) => h.saveConfig(c),
}));

import {
  DEFAULT_FONT_SIZE,
  FONT_SIZE_STEPS,
  normalizeFontSize,
  stepFontSize,
} from '../constants/fontSize';
import { EventFontSizeChanged } from '../utils/events';
import { applyFontSize, fontSize, initFontSize, saveFontSize } from './fontSize';
import { get } from 'svelte/store';

const root = resolve(__dirname, '..');
const scale = () => document.documentElement.style.getPropertyValue('--kai-text-scale');
const settle = async () => {
  for (let i = 0; i < 4; i++) await new Promise((r) => setTimeout(r, 0));
};

beforeEach(() => {
  h.handlers.clear();
  h.config = { font_size: 120, language: 'en-US' };
  h.getConfigFails = false;
  h.saveConfig.mockReset();
  h.saveConfig.mockResolvedValue(undefined);
  document.documentElement.style.removeProperty('--kai-text-scale');
  localStorage.clear();
  applyFontSize(DEFAULT_FONT_SIZE);
  document.documentElement.style.removeProperty('--kai-text-scale');
});
afterEach(() => localStorage.clear());

describe('the ladder', () => {
  it('is the six even 20-point steps, default 120', () => {
    expect([...FONT_SIZE_STEPS]).toEqual([80, 100, 120, 140, 160, 180]);
    expect(DEFAULT_FONT_SIZE).toBe(120);
  });

  it('normalizeFontSize keeps the six and turns everything else into 120', () => {
    for (const p of FONT_SIZE_STEPS) expect(normalizeFontSize(p)).toBe(p);
    for (const bad of [
      0,
      90, // the unreleased 15-point ladder: 90 105 135 150 165
      105,
      121,
      135,
      150,
      165,
      -140,
      180.5,
      NaN,
      Infinity,
      null,
      undefined,
      '140',
      'big',
      {},
      [160],
      true,
    ]) {
      expect(normalizeFontSize(bad), String(bad)).toBe(120);
    }
  });

  it('stepFontSize moves one step and stops at the ends', () => {
    expect(stepFontSize(120, 1)).toBe(140);
    expect(stepFontSize(120, -1)).toBe(100);
    expect(stepFontSize(100, -1)).toBe(80);
    expect(stepFontSize(80, -1)).toBe(80);
    expect(stepFontSize(160, 1)).toBe(180);
    expect(stepFontSize(180, 1)).toBe(180);
  });
});

describe('applyFontSize: the one place the scale reaches the document', () => {
  it('sets --kai-text-scale to percent/100 for each size', () => {
    for (const p of FONT_SIZE_STEPS) {
      applyFontSize(p);
      expect(Number(scale())).toBeCloseTo(p / 100, 5);
      expect(get(fontSize)).toBe(p);
    }
  });

  it('a garbled value applies the 120 default', () => {
    for (const bad of [105, 0, null, 'x', NaN, 999]) {
      applyFontSize(160);
      applyFontSize(bad);
      expect(Number(scale()), String(bad)).toBeCloseTo(1.2, 5);
      expect(get(fontSize)).toBe(120);
    }
  });
});

describe('initFontSize: every window applies the saved size, live', () => {
  it('applies the size from the config', async () => {
    h.config = { font_size: 80 };
    await initFontSize();
    expect(Number(scale())).toBeCloseTo(0.8, 5);
  });

  it('a config without the value, or a failing read, is 120', async () => {
    h.config = { language: 'en-US' };
    await initFontSize();
    expect(Number(scale())).toBeCloseTo(1.2, 5);
    localStorage.clear();
    h.getConfigFails = true;
    await initFontSize();
    expect(Number(scale())).toBeCloseTo(1.2, 5);
  });

  it('follows the change event without a reload', async () => {
    await initFontSize();
    expect(Number(scale())).toBeCloseTo(1.2, 5);
    h.fire(EventFontSizeChanged, 180);
    expect(Number(scale())).toBeCloseTo(1.8, 5);
    h.fire(EventFontSizeChanged, 80);
    expect(Number(scale())).toBeCloseTo(0.8, 5);
    h.fire(EventFontSizeChanged, 'garbage');
    expect(Number(scale())).toBeCloseTo(1.2, 5);
  });

  it('re-initialising does not stack listeners', async () => {
    await initFontSize();
    await initFontSize();
    expect(h.handlers.get(EventFontSizeChanged)?.length).toBe(1);
  });

  it('remembers the last size in localStorage so the next start applies it before the config loads', async () => {
    h.config = { font_size: 160 };
    await initFontSize();
    document.documentElement.style.removeProperty('--kai-text-scale');
    h.getConfigFails = true;
    // Same document, new start: the cached size is applied synchronously, before any await.
    const p = initFontSize();
    expect(Number(scale())).toBeCloseTo(1.6, 5);
    await p;
  });

  it('a corrupt cached value reads as 120', async () => {
    localStorage.setItem('kai:fontSize', '"huge"');
    h.getConfigFails = true;
    await initFontSize();
    expect(Number(scale())).toBeCloseTo(1.2, 5);
  });
});

describe('saveFontSize', () => {
  it('applies at once, then saves through SaveConfig keeping the rest of the config', async () => {
    await saveFontSize(160);
    expect(Number(scale())).toBeCloseTo(1.6, 5);
    expect(h.saveConfig).toHaveBeenCalledTimes(1);
    expect(h.saveConfig.mock.calls[0][0]).toMatchObject({ language: 'en-US', font_size: 160 });
  });

  it('saves a garbled value as 120', async () => {
    await saveFontSize(101);
    expect(h.saveConfig.mock.calls[0][0]).toMatchObject({ font_size: 120 });
    expect(Number(scale())).toBeCloseTo(1.2, 5);
  });

  it('puts the previous size back when the save fails', async () => {
    applyFontSize(140);
    h.saveConfig.mockRejectedValue(new Error('disk full'));
    const err = vi.spyOn(console, 'error').mockImplementation(() => {});
    await saveFontSize(180);
    err.mockRestore();
    expect(get(fontSize)).toBe(140);
    expect(Number(scale())).toBeCloseTo(1.4, 5);
  });
});

describe('the wiring that makes it reach every window', () => {
  it.each(['translate', 'settings', 'screenshot'])('the %s window starts the font size', (w) => {
    const src = readFileSync(join(root, 'windows', w, 'main.ts'), 'utf8');
    expect(src).toMatch(
      /import\s*\{[^}]*\binitFontSize\b[^}]*\}\s*from\s*'\.\.\/\.\.\/stores\/fontSize'/,
    );
    expect(src).toMatch(/^initFontSize\(\);/m);
  });

  it('the event name matches the Go side', () => {
    const go = readFileSync(resolve(root, '..', '..', 'internal', 'events', 'events.go'), 'utf8');
    expect(go).toContain(`EventFontSizeChanged = "${EventFontSizeChanged}"`);
  });
});

describe('app.css: text follows the scale, layout does not', () => {
  const css = readFileSync(join(root, 'app.css'), 'utf8');

  it('never divides one length by another in calc() (typed division is not valid in every WebKit)', () => {
    // e.g. calc(1rem / 0.6875rem): the declaration is dropped where unsupported, so the line-height
    // silently falls back. A line-height ratio must be a plain number: calc(1 / 0.6875).
    const bad = css.match(/calc\([^)]*\/\s*[\d.]+(rem|em|px|%)/g) ?? [];
    expect(bad).toEqual([]);
  });

  it('starts at the default scale, so the first paint matches a fresh install', () => {
    const m = css.match(/:root\s*\{[^}]*--kai-text-scale:\s*([0-9.]+)\s*;/);
    expect(m, 'no --kai-text-scale on :root').not.toBeNull();
    expect(Number(m![1])).toBeCloseTo(DEFAULT_FONT_SIZE / 100, 5);
  });

  it('every Tailwind text size the app uses is scaled', () => {
    for (const tok of ['xs', 'sm', 'base', 'lg', 'xl', '2xl', '2xs', '3xs']) {
      const m = css.match(new RegExp(`--text-${tok}:\\s*([^;]+);`));
      expect(m, `--text-${tok} not defined`).not.toBeNull();
      expect(m![1], `--text-${tok}`).toContain('var(--kai-text-scale)');
    }
  });

  it('the two smallest sizes never drop below 10px, so 80% stays readable', () => {
    for (const tok of ['2xs', '3xs']) {
      const m = css.match(new RegExp(`--text-${tok}:\\s*([^;]+);`));
      expect(m![1], tok).toMatch(/^max\(0\.625rem,\s*calc\(/);
    }
    // The next size up is 12px * 0.8 = 9.6px at 80%; nothing else needs a floor.
    expect(FONT_SIZE_STEPS[0] / 100).toBeGreaterThanOrEqual(0.8);
  });

  it('the text that inherits (no text-* class) is scaled too', () => {
    expect(css).toMatch(/body\s*\{[^}]*font-size:\s*calc\([^;]*var\(--kai-text-scale\)/);
  });

  it('no hand-written font-size in app.css ignores the scale', () => {
    const decls = css.replace(/\/\*[\s\S]*?\*\//g, '').match(/font-size:[^;]+;/g) ?? [];
    expect(decls.length).toBeGreaterThan(5);
    for (const d of decls) expect(d, d).toContain('var(--kai-text-scale)');
  });

  it('the root font size is left alone (rem spacing and widths must not scale)', () => {
    const code = css.replace(/\/\*[\s\S]*?\*\//g, '');
    expect(code).not.toMatch(/(^|\})\s*html\s*\{[^}]*font-size/);
    expect(code).not.toMatch(/:root\s*\{[^}]*font-size/);
  });

  it('no component sizes text in fixed pixels', () => {
    const offenders: string[] = [];
    const walk = (dir: string) => {
      for (const f of readdirSync(dir)) {
        const p = join(dir, f);
        if (statSync(p).isDirectory()) walk(p);
        else if (
          p.endsWith('.svelte') &&
          /text-\[\d+(\.\d+)?px\]|font-size:\s*\d+px/.test(readFileSync(p, 'utf8'))
        )
          offenders.push(p);
      }
    };
    walk(join(root, 'components'));
    expect(offenders).toEqual([]);
  });
});
