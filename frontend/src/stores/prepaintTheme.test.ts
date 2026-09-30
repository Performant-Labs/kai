import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #15: every window flashed white before the dark colour was applied. The pages now carry an
// inline script (and a background rule) in <head> that applies the last saved theme before the first
// paint; the theme store writes the cache that script reads.

vi.mock('../runtime', () => ({
  onEvent: () => () => {},
  System: { IsDarkMode: async () => false },
  Window: {},
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/configwrapper.ts', () => ({
  GetTheme: async () => 'auto',
  SetTheme: async () => {},
}));

import { get } from 'svelte/store';
import { initTheme, setTheme, themeMode, systemDark } from './theme';

const PAGES = ['index.html', 'translate.html', 'settings.html', 'screenshot.html'];
const root = resolve(__dirname, '../..');
const read = (p: string) => readFileSync(resolve(root, p), 'utf8');

function inlineScript(html: string): string {
  const head = html.slice(0, html.indexOf('</head>'));
  const m = head.match(/<script(?![^>]*\bsrc=)(?![^>]*type="module")[^>]*>([\s\S]*?)<\/script>/);
  return m?.[1] ?? '';
}

function mockMedia(dark: boolean) {
  window.matchMedia = ((q: string) => ({
    matches: q.includes('dark') ? dark : !dark,
    media: q,
    addEventListener() {},
    removeEventListener() {},
  })) as unknown as typeof window.matchMedia;
}

function run(page: string) {
  // eslint-disable-next-line @typescript-eslint/no-implied-eval
  new Function(inlineScript(read(page)))();
}

beforeEach(() => {
  document.documentElement.className = '';
  document.documentElement.removeAttribute('style');
  localStorage.clear();
});
afterEach(() => vi.restoreAllMocks());

describe.each(PAGES)('%s', (page) => {
  it('has an inline script in <head> before the module script', () => {
    const html = read(page);
    const inline = html.search(/<script(?![^>]*\bsrc=)(?![^>]*type="module")/);
    const mod = html.search(/<script[^>]*type="module"/);
    expect(inline).toBeGreaterThan(-1);
    if (mod > -1) expect(inline).toBeLessThan(mod);
    expect(inline).toBeLessThan(html.indexOf('</head>'));
  });

  it('has a background style in <head>', () => {
    const head = read(page).slice(0, read(page).indexOf('</head>'));
    expect(head).toMatch(/<style>[\s\S]*background[\s\S]*<\/style>/);
  });

  it('applies dark from the cached resolved theme', () => {
    mockMedia(false);
    localStorage.setItem('kai.theme.resolved', 'dark');
    run(page);
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    expect(document.documentElement.style.backgroundColor).toMatch(/#18181c|rgb\(24, 24, 28\)/);
  });

  it('applies light from the cached resolved theme even if the system is dark', () => {
    mockMedia(true);
    localStorage.setItem('kai.theme.resolved', 'light');
    run(page);
    expect(document.documentElement.classList.contains('dark')).toBe(false);
    expect(document.documentElement.style.backgroundColor).toMatch(/#fff|rgb\(255, 255, 255\)/);
  });

  it('falls back to prefers-color-scheme with no cache', () => {
    mockMedia(true);
    run(page);
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    document.documentElement.className = '';
    mockMedia(false);
    run(page);
    expect(document.documentElement.classList.contains('dark')).toBe(false);
  });

  it('survives unavailable localStorage', () => {
    mockMedia(true);
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('denied');
    });
    expect(() => run(page)).not.toThrow();
    expect(document.documentElement.classList.contains('dark')).toBe(true);
  });
});

describe('theme store cache', () => {
  it('writes mode and resolved value on setTheme', async () => {
    systemDark.set(false);
    await setTheme('dark');
    expect(localStorage.getItem('kai.theme.mode')).toBe('dark');
    expect(localStorage.getItem('kai.theme.resolved')).toBe('dark');
    await setTheme('light');
    expect(localStorage.getItem('kai.theme.resolved')).toBe('light');
  });

  it('writes the resolved value after initTheme, following the system in auto', async () => {
    await initTheme();
    expect(get(themeMode)).toBe('auto');
    expect(localStorage.getItem('kai.theme.mode')).toBe('auto');
    expect(localStorage.getItem('kai.theme.resolved')).toBe('light');
  });

  it('does not throw when localStorage is unavailable', async () => {
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new Error('denied');
    });
    await expect(setTheme('dark')).resolves.toBeUndefined();
  });
});
