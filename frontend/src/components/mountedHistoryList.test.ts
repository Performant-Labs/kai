import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #60: the History list, with the REAL HistoryList.svelte mounted in jsdom (the component both
// the History window and the Settings tab render). Only the Wails boundary is faked: the runtime
// (events, clipboard, dialogs) and the generated bindings. The fake backend applies the same filter
// rules as the SQL (tested in Go), so what is pinned here is what the list asks for and shows.

type Row = {
  id: number;
  text: string;
  result: string;
  from_lang: string;
  to_lang: string;
  engine: string;
  engine_id: number;
  from_ocr: boolean;
  created_at: number;
};

const h = vi.hoisted(() => ({
  rows: [] as Row[],
  query: vi.fn(),
  facets: vi.fn(),
  del: vi.fn(),
  clear: vi.fn(),
  showTranslate: vi.fn(async (): Promise<void> => {}),
  emit: vi.fn(),
  copy: vi.fn(async (_s: string): Promise<void> => {}),
  question: vi.fn(),
  error: vi.fn(async (_o: unknown): Promise<void> => {}),
}));

vi.mock('@wailsio/runtime', () => ({
  Events: { On: () => () => {}, Emit: (...a: unknown[]) => h.emit(...a) },
  Window: { Close: vi.fn(async () => {}), Name: async () => 'history' },
  Clipboard: { SetText: (s: string) => h.copy(s) },
  Dialogs: { Question: (o: unknown) => h.question(o), Error: (o: unknown) => h.error(o) },
  System: { IsDarkMode: async () => false },
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/historywrapper.ts', () => ({
  QueryHistory: (f: unknown, offset: number, limit: number) => h.query(f, offset, limit),
  HistoryFacets: () => h.facets(),
  DeleteHistory: (id: number) => h.del(id),
  ClearHistory: () => h.clear(),
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/windowwrapper.ts', () => ({
  ShowTranslateWindow: () => h.showTranslate(),
}));

import HistoryList from './HistoryList.svelte';
import { setLocale } from '../i18n';

const NOW = new Date(2026, 9, 8, 15, 0, 0).getTime();
const at = (m: number, d: number, hh: number, mm = 0) => new Date(2026, m, d, hh, mm).getTime();
const mk = (id: number, extra: Partial<Row> = {}): Row => ({
  id,
  text: `source ${id}`,
  result: `result ${id}`,
  from_lang: 'es-MX',
  to_lang: 'en',
  engine: 'google',
  engine_id: 1,
  from_ocr: false,
  created_at: NOW - id * 60_000,
  ...extra,
});
const SEED = (): Row[] => [
  mk(1, {
    text: 'Necesito los puntos clave de tus cambios',
    result: 'I need the key points of your changes',
    created_at: at(9, 8, 14, 2),
  }),
  mk(2, {
    text: 'Hello there',
    result: 'Hola',
    from_lang: 'en',
    to_lang: 'es-MX',
    engine: 'system',
    engine_id: 2,
    from_ocr: true,
    created_at: at(9, 8, 13, 40),
  }),
  mk(3, { created_at: at(9, 7, 21, 30) }),
  mk(4, { created_at: at(9, 1, 9, 0), text: 'old one', result: 'viejo' }),
];

let app: Record<string, unknown> | undefined;
let target: HTMLElement;
let onescape: ReturnType<typeof vi.fn>;

const flush = async () => {
  for (let i = 0; i < 8; i++) await vi.advanceTimersByTimeAsync(0);
  flushSync();
};
const advance = async (ms: number) => {
  await vi.advanceTimersByTimeAsync(ms);
  await flush();
};
const rowEls = () => [...target.querySelectorAll('li[data-history-id]')] as HTMLElement[];
const ids = () => rowEls().map((r) => Number(r.dataset.historyId));
const search = () => target.querySelector('input[type="search"]') as HTMLInputElement;
const btn = (row: HTMLElement, label: string) =>
  [...row.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === label,
  ) as HTMLButtonElement;
const top = (label: string) =>
  [...target.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === label,
  ) as HTMLButtonElement;
const select = (label: string) =>
  target.querySelector(`select[aria-label="${label}"]`) as HTMLSelectElement;
const key = (k: string, init: KeyboardEventInit = {}, on: EventTarget = window) =>
  on.dispatchEvent(
    new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true, ...init }),
  );
const lastFilter = () => h.query.mock.calls[h.query.mock.calls.length - 1][0];

function install(rows: Row[]) {
  h.rows = rows;
  h.query.mockImplementation(async (f: any, offset: number, limit: number) => {
    const kw = (f.keyword ?? '').toLowerCase();
    const hit = h.rows.filter(
      (r) =>
        (kw === '' || r.text.toLowerCase().includes(kw) || r.result.toLowerCase().includes(kw)) &&
        (!f.from_lang || r.from_lang === f.from_lang) &&
        (!f.to_lang || r.to_lang === f.to_lang) &&
        (!f.engine_id || r.engine_id === f.engine_id),
    );
    return { items: hit.slice(offset, offset + limit), total: hit.length };
  });
  h.facets.mockImplementation(async () => {
    const pairs = new Map<string, { from_lang: string; to_lang: string }>();
    const engines = new Map<number, { id: number; engine: string }>();
    for (const r of h.rows) {
      pairs.set(`${r.from_lang}|${r.to_lang}`, { from_lang: r.from_lang, to_lang: r.to_lang });
      if (r.engine_id) engines.set(r.engine_id, { id: r.engine_id, engine: r.engine });
    }
    return { pairs: [...pairs.values()], engines: [...engines.values()] };
  });
  h.del.mockImplementation(async (id: number) => {
    h.rows = h.rows.filter((r) => r.id !== id);
  });
  h.clear.mockImplementation(async () => {
    h.rows = [];
  });
}

async function open(rows: Row[] = SEED(), props: Record<string, unknown> = {}) {
  install(rows);
  app = mount(HistoryList, { target, props: { title: 'History', onescape, ...props } });
  await flush();
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
  vi.setSystemTime(NOW);
  setLocale('en-US');
  for (const m of [h.query, h.facets, h.del, h.clear, h.showTranslate, h.emit, h.copy, h.error])
    m.mockReset();
  h.showTranslate.mockResolvedValue(undefined);
  h.copy.mockResolvedValue(undefined);
  h.error.mockResolvedValue(undefined);
  h.question.mockReset();
  onescape = vi.fn();
  target = document.createElement('div');
  document.body.appendChild(target);
});

afterEach(() => {
  if (app) unmount(app);
  app = undefined;
  target.remove();
  vi.useRealTimers();
});

describe('HistoryList, mounted: the rows', () => {
  it('lists the newest first, grouped by day, from the first page', async () => {
    await open();
    expect(ids()).toEqual([1, 2, 3, 4]);
    const heads = [...target.querySelectorAll('h2')].map((x) => x.textContent?.trim());
    expect(heads).toEqual(['Today', 'Yesterday', 'Oct 1']);
    expect(h.query).toHaveBeenCalledTimes(1);
    expect(h.query.mock.calls[0].slice(1)).toEqual([0, 50]);
  });

  it('shows on every row the time, the pair, the engine and the source and result', async () => {
    await open();
    const first = rowEls()[0].textContent ?? '';
    expect(first).toContain('14:02');
    expect(first).toContain('Spanish (Mexico)');
    expect(first).toContain('English');
    expect(first).toContain('Google');
    expect(first).toContain('Necesito los puntos clave de tus cambios');
    expect(first).toContain('I need the key points of your changes');
  });

  it('marks an entry that came from a screenshot', async () => {
    await open();
    expect(rowEls()[0].textContent).not.toContain('OCR');
    expect(rowEls()[1].textContent).toContain('OCR');
  });

  it('shows the date, not the clock, for an older entry', async () => {
    await open();
    expect(rowEls()[3].textContent).toContain('Oct 1');
    expect(rowEls()[3].textContent).not.toContain('09:00');
  });

  it('clamps a long text behind "Show more", which opens and closes it', async () => {
    const long = Array.from({ length: 9 }, (_, i) => `line ${i}`).join('\n');
    await open([mk(1, { text: long })]);
    const row = rowEls()[0];
    const para = [...row.querySelectorAll('p')].find((p) => p.textContent?.includes('line 0'))!;
    expect(para.className).toContain('line-clamp-3');
    btn(row, 'Show more').click();
    await flush();
    expect(para.className).not.toContain('line-clamp-3');
    btn(row, 'Show less').click();
    await flush();
    expect(para.className).toContain('line-clamp-3');
  });

  it('offers no "Show more" for short texts', async () => {
    await open();
    expect(btn(rowEls()[0], 'Show more')).toBeUndefined();
  });
});

describe('HistoryList, mounted: Use and Copy', () => {
  it('Use brings the translate window forward, then sends the entry; nothing else', async () => {
    await open();
    btn(rowEls()[0], 'Use').click();
    await flush();
    expect(h.showTranslate).toHaveBeenCalledTimes(1);
    expect(h.emit).toHaveBeenCalledWith('kai:history:use', {
      text: 'Necesito los puntos clave de tus cambios',
      result: 'I need the key points of your changes',
      from_lang: 'es-MX',
      to_lang: 'en',
      engine: 'google',
    });
    expect(h.showTranslate.mock.invocationCallOrder[0]).toBeLessThan(
      h.emit.mock.invocationCallOrder[0],
    );
    expect(h.del).not.toHaveBeenCalled();
    expect(ids()).toEqual([1, 2, 3, 4]);
  });

  it('Use still sends the entry if the window cannot be brought forward', async () => {
    h.showTranslate.mockRejectedValue(new Error('no window'));
    await open();
    btn(rowEls()[1], 'Use').click();
    await flush();
    expect(h.emit).toHaveBeenCalledWith(
      'kai:history:use',
      expect.objectContaining({ text: 'Hello there' }),
    );
  });

  it('a double click on a row uses it', async () => {
    await open();
    rowEls()[2].dispatchEvent(new MouseEvent('dblclick', { bubbles: true }));
    await flush();
    expect(h.emit).toHaveBeenCalledWith(
      'kai:history:use',
      expect.objectContaining({ text: 'source 3' }),
    );
  });

  it('Copy puts the result on the clipboard and says so', async () => {
    await open();
    btn(rowEls()[0], 'Copy').click();
    await flush();
    expect(h.copy).toHaveBeenCalledWith('I need the key points of your changes');
    expect(target.textContent).toContain('Copied');
  });
});

describe('HistoryList, mounted: search and filters', () => {
  it('searches after a pause, from the top, over source and result', async () => {
    await open();
    search().value = 'viejo';
    search().dispatchEvent(new Event('input', { bubbles: true }));
    await advance(299);
    expect(h.query).toHaveBeenCalledTimes(1);
    await advance(1);
    expect(h.query).toHaveBeenCalledTimes(2);
    expect(lastFilter().keyword).toBe('viejo');
    expect(h.query.mock.calls[1].slice(1)).toEqual([0, 50]);
    expect(ids()).toEqual([4]);
  });

  it('a burst of typing is one search', async () => {
    await open();
    for (const v of ['h', 'he', 'hel']) {
      search().value = v;
      search().dispatchEvent(new Event('input', { bubbles: true }));
      await advance(100);
    }
    await advance(300);
    expect(h.query).toHaveBeenCalledTimes(2);
    expect(lastFilter().keyword).toBe('hel');
  });

  it('filters by language pair, from the pairs that exist', async () => {
    await open();
    const pair = select('Languages');
    const labels = [...pair.options].map((o) => o.textContent?.trim());
    expect(labels).toEqual([
      'Languages: All',
      'Spanish (Mexico) → English',
      'English → Spanish (Mexico)',
    ]);
    pair.value = 'en|es-MX';
    pair.dispatchEvent(new Event('change', { bubbles: true }));
    await flush();
    expect(lastFilter()).toMatchObject({ from_lang: 'en', to_lang: 'es-MX' });
    expect(ids()).toEqual([2]);
  });

  it('filters by engine', async () => {
    await open();
    const engine = select('Engine');
    expect([...engine.options].map((o) => o.value)).toEqual(['0', '1', '2']);
    engine.value = '2';
    engine.dispatchEvent(new Event('change', { bubbles: true }));
    await flush();
    expect(lastFilter().engine_id).toBe(2);
    expect(ids()).toEqual([2]);
  });

  it('combines the filters with the keyword', async () => {
    await open();
    const engine = select('Engine');
    engine.value = '1';
    engine.dispatchEvent(new Event('change', { bubbles: true }));
    await flush();
    search().value = 'viejo';
    search().dispatchEvent(new Event('input', { bubbles: true }));
    await advance(300);
    expect(lastFilter()).toMatchObject({ keyword: 'viejo', engine_id: 1 });
    expect(ids()).toEqual([4]);
  });

  it('words "nothing matches" apart from "nothing saved", and clears the filters from there', async () => {
    await open();
    search().value = 'zzz';
    search().dispatchEvent(new Event('input', { bubbles: true }));
    await advance(300);
    expect(ids()).toEqual([]);
    expect(target.textContent).toContain('Nothing matches');
    expect(target.textContent).not.toContain('No translations saved yet');
    top('Clear filters').click();
    await flush();
    expect(search().value).toBe('');
    expect(ids()).toEqual([1, 2, 3, 4]);
  });

  it('words the empty history on its own', async () => {
    await open([]);
    expect(target.textContent).toContain('No translations saved yet');
    expect(target.textContent).not.toContain('Nothing matches');
    expect(top('Clear all').disabled).toBe(true);
  });

  it('drops the answer to a search that was replaced by a newer one', async () => {
    await open();
    let release!: (v: unknown) => void;
    const slow = new Promise((r) => (release = r));
    h.query.mockImplementationOnce(async () => {
      await slow;
      return { items: [mk(99, { text: 'stale' })], total: 1 };
    });
    search().value = 'a';
    search().dispatchEvent(new Event('input', { bubbles: true }));
    await advance(300);
    search().value = 'viejo';
    search().dispatchEvent(new Event('input', { bubbles: true }));
    await advance(300);
    release(null);
    await flush();
    expect(ids()).toEqual([4]);
  });
});

describe('HistoryList, mounted: delete and clear', () => {
  it('deletes one entry and updates the list without reloading it', async () => {
    await open();
    btn(rowEls()[1], 'Delete').click();
    await flush();
    expect(h.del).toHaveBeenCalledWith(2);
    expect(ids()).toEqual([1, 3, 4]);
    expect(h.query).toHaveBeenCalledTimes(1);
    expect(target.textContent).toContain('3 entries');
  });

  it('keeps the entry and says so when the delete fails', async () => {
    await open();
    h.del.mockRejectedValueOnce(new Error('db'));
    btn(rowEls()[0], 'Delete').click();
    await flush();
    expect(ids()).toEqual([1, 2, 3, 4]);
    expect(h.error).toHaveBeenCalledWith(
      expect.objectContaining({ Message: 'Failed to delete the entry' }),
    );
  });

  it('clears everything only after a confirmation', async () => {
    await open();
    h.question.mockResolvedValue('Clear all');
    top('Clear all').click();
    await flush();
    expect(h.question).toHaveBeenCalledTimes(1);
    expect(h.question.mock.calls[0][0]).toMatchObject({
      Title: 'Clear all history?',
      Buttons: [{ Label: 'Cancel', IsCancel: true, IsDefault: true }, { Label: 'Clear all' }],
    });
    expect(h.clear).toHaveBeenCalledTimes(1);
    expect(ids()).toEqual([]);
    expect(target.textContent).toContain('No translations saved yet');
  });

  it('keeps everything when the confirmation is cancelled', async () => {
    await open();
    h.question.mockResolvedValue('Cancel');
    top('Clear all').click();
    await flush();
    expect(h.clear).not.toHaveBeenCalled();
    expect(ids()).toEqual([1, 2, 3, 4]);
  });

  it('clears everything, not only what the filter shows', async () => {
    await open();
    const engine = select('Engine');
    engine.value = '2';
    engine.dispatchEvent(new Event('change', { bubbles: true }));
    await flush();
    h.question.mockResolvedValue('Clear all');
    top('Clear all').click();
    await flush();
    expect(h.clear).toHaveBeenCalledTimes(1);
    expect(h.rows).toEqual([]);
    expect(lastFilter()).toMatchObject({ engine_id: 0, keyword: '' });
  });
});

describe('HistoryList, mounted: paging', () => {
  const many = (n: number) =>
    Array.from({ length: n }, (_, i) => mk(i + 1, { created_at: NOW - i * 1000 }));
  const geometry = (
    el: HTMLElement,
    g: { scrollTop: number; clientHeight: number; scrollHeight: number },
  ) => {
    for (const [k, v] of Object.entries(g))
      Object.defineProperty(el, k, { value: v, configurable: true });
  };
  const scroller = () => target.querySelector('[data-testid="history-scroller"]') as HTMLElement;

  it('loads the next page when the user scrolls near the bottom, and not before', async () => {
    await open(many(120));
    expect(ids()).toHaveLength(50);
    geometry(scroller(), { scrollTop: 0, clientHeight: 500, scrollHeight: 5000 });
    scroller().dispatchEvent(new Event('scroll'));
    await flush();
    expect(h.query).toHaveBeenCalledTimes(1);

    // Rows have height: the list grows as pages arrive, so the user is no longer near the bottom.
    Object.defineProperty(scroller(), 'scrollHeight', {
      configurable: true,
      get: () => rowEls().length * 100,
    });
    Object.defineProperty(scroller(), 'scrollTop', { configurable: true, value: 4400 });
    scroller().dispatchEvent(new Event('scroll'));
    await flush();
    expect(h.query).toHaveBeenCalledTimes(2);
    expect(h.query.mock.calls[1].slice(1)).toEqual([50, 50]);
    expect(ids()).toHaveLength(100);
  });

  it('shows all of a long history in the end, once each, and stops asking', async () => {
    await open(many(120));
    geometry(scroller(), { scrollTop: 4900, clientHeight: 500, scrollHeight: 5000 });
    for (let i = 0; i < 4; i++) {
      scroller().dispatchEvent(new Event('scroll'));
      await flush();
    }
    expect(ids()).toHaveLength(120);
    expect(new Set(ids()).size).toBe(120);
    expect(h.query).toHaveBeenCalledTimes(3);
  });

  it('does not ask for a second page while one is on its way', async () => {
    await open(many(120));
    let release!: (v: unknown) => void;
    h.query.mockImplementationOnce(() => new Promise((r) => (release = r)));
    geometry(scroller(), { scrollTop: 4900, clientHeight: 500, scrollHeight: 5000 });
    scroller().dispatchEvent(new Event('scroll'));
    scroller().dispatchEvent(new Event('scroll'));
    await flush();
    expect(h.query).toHaveBeenCalledTimes(2);
    release({ items: [], total: 120 });
    await flush();
  });
});

describe('HistoryList, mounted: the keyboard', () => {
  it('Down and Up move through the rows, and stop at the ends', async () => {
    await open();
    const sel = () => rowEls().findIndex((r) => r.getAttribute('aria-current') === 'true');
    expect(sel()).toBe(-1);
    key('ArrowDown');
    await flush();
    expect(sel()).toBe(0);
    key('ArrowDown');
    key('ArrowDown');
    await flush();
    expect(sel()).toBe(2);
    for (let i = 0; i < 6; i++) key('ArrowDown');
    await flush();
    expect(sel()).toBe(3);
    key('ArrowUp');
    await flush();
    expect(sel()).toBe(2);
  });

  it('Enter uses the selected entry', async () => {
    await open();
    key('ArrowDown');
    key('ArrowDown');
    await flush();
    key('Enter');
    await flush();
    expect(h.emit).toHaveBeenCalledWith(
      'kai:history:use',
      expect.objectContaining({ text: 'Hello there' }),
    );
  });

  it('Enter with nothing selected does nothing', async () => {
    await open();
    key('Enter');
    await flush();
    expect(h.emit).not.toHaveBeenCalled();
  });

  it('Delete removes the selected entry and selects the one that took its place', async () => {
    await open();
    key('ArrowDown');
    await flush();
    key('Delete');
    await flush();
    expect(h.del).toHaveBeenCalledWith(1);
    expect(ids()).toEqual([2, 3, 4]);
    expect(rowEls()[0].getAttribute('aria-current')).toBe('true');
  });

  it('Delete in the search box edits the text and removes nothing', async () => {
    await open();
    key('ArrowDown');
    await flush();
    key('Delete', {}, search());
    await flush();
    expect(h.del).not.toHaveBeenCalled();
  });

  it('Enter on a button presses that button, not the row', async () => {
    await open();
    key('ArrowDown');
    await flush();
    const copy = btn(rowEls()[0], 'Copy');
    key('Enter', {}, copy);
    await flush();
    expect(h.emit).not.toHaveBeenCalled();
  });

  it('Cmd+F focuses the search box', async () => {
    await open();
    const ev = new KeyboardEvent('keydown', { key: 'f', metaKey: true, cancelable: true });
    window.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(search());
  });

  it('Esc closes the screen where it has a window to close', async () => {
    await open();
    key('Escape');
    expect(onescape).toHaveBeenCalledTimes(1);
  });

  it('Esc does nothing where there is no window to close (the Settings tab)', async () => {
    await open(SEED(), { onescape: undefined });
    const ev = new KeyboardEvent('keydown', { key: 'Escape', cancelable: true });
    window.dispatchEvent(ev);
    expect(ev.defaultPrevented).toBe(false);
  });

  it('leaves the keys alone while an IME composition is open', async () => {
    await open();
    key('ArrowDown', { isComposing: true });
    await flush();
    expect(rowEls().some((r) => r.getAttribute('aria-current') === 'true')).toBe(false);
  });
});

describe('HistoryList, mounted: staying current and speaking Chinese', () => {
  it('re-reads the list when the window gets the focus back, keeping what was loaded', async () => {
    await open();
    h.rows = [mk(9, { text: 'brand new', created_at: NOW }), ...h.rows];
    window.dispatchEvent(new Event('focus'));
    await flush();
    expect(ids()).toEqual([9, 1, 2, 3, 4]);
  });

  it('words everything in Chinese when the interface is Chinese', async () => {
    setLocale('zh-CN');
    await open();
    expect(search().placeholder).toBe('搜索原文或译文…');
    expect(top('全部清空')).toBeDefined();
    expect(btn(rowEls()[0], '使用')).toBeDefined();
    expect(btn(rowEls()[0], '复制')).toBeDefined();
    expect(btn(rowEls()[0], '删除')).toBeDefined();
    expect([...target.querySelectorAll('h2')][0].textContent?.trim()).toBe('今天');
  });
});
