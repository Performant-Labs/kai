import { readFileSync, existsSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Issue #60: one list, one set of rules. The Settings tab renders the same component as the History
// window instead of a second implementation, and the window has its own page.
const read = (p: string) => readFileSync(resolve(__dirname, p), 'utf8');

describe('History: one list for the window and the Settings tab (issue #60)', () => {
  it('the Settings tab renders HistoryList and talks to no binding itself', () => {
    const tab = read('HistoryTab.svelte');
    expect(tab).toMatch(/import HistoryList from '\.\.\/HistoryList\.svelte'/);
    expect(tab).toMatch(/<HistoryList\b/);
    expect(tab).not.toMatch(/historywrapper/);
  });

  it('the History window renders the same HistoryList and closes itself on Esc', () => {
    const win = read('../History.svelte');
    expect(win).toMatch(/import HistoryList from '\.\/HistoryList\.svelte'/);
    expect(win).toMatch(/<HistoryList[^>]*onescape=\{close\}/);
  });

  it('the History window has its page, its entry file and its build input', () => {
    const root = resolve(__dirname, '../../..');
    expect(existsSync(resolve(root, 'history.html'))).toBe(true);
    expect(read('../../windows/history/main.ts')).toMatch(/mount\(History,/);
    expect(readFileSync(resolve(root, 'history.html'), 'utf8')).toContain(
      '/src/windows/history/main.ts',
    );
    expect(readFileSync(resolve(root, 'vite.config.ts'), 'utf8')).toMatch(
      /history:\s*resolve\(__dirname,\s*'history\.html'\)/,
    );
  });

  it('the old paged tab strings are gone from both catalogs', () => {
    for (const f of ['en-US.ts', 'zh-CN.ts', 'keys.ts']) {
      const src = read(`../../i18n/${f}`);
      expect(src, f).not.toMatch(/historyPageInfo|historyPrev|historyNext/);
    }
  });
});
