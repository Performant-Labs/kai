import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Issue #60: the history screen, with the REAL HistoryScreen.svelte (wrapping the real
// HistoryList.svelte) mounted in jsdom. Only the Wails boundary is faked: the runtime
// (events, window, clipboard, dialogs) and the generated bindings. What the backend does
// with a filter is tested in Go (internal/historystore); this pins what the screen asks for
// and what it shows: open, search, filters, use, delete, clear, empty states, keyboard, Esc.

const h = vi.hoisted(() => {
  const handlers = new Map<string, Array<(e: { data: unknown }) => void>>();
  return {
    handlers,
    fire(name: string, data: unknown) {
      for (const cb of handlers.get(name) ?? []) cb({ data });
    },
    emit: vi.fn(),
    close: vi.fn(async () => {}),
    copy: vi.fn(async (_t: string) => {}),
    info: vi.fn(async (_o: unknown) => {}),
    error: vi.fn(async (_o: unknown) => {}),
    question: 0, // index of the button the confirm dialog answers: 0 = clear, 1 = cancel
    // The store the bindings read: rows + total, filtered the way the backend would.
    items: [] as Array<Record<string, unknown>>,
    getHistory: vi.fn(),
    countHistory: vi.fn(),
    del: vi.fn(async (_id: number) => {}),
    clear: vi.fn(async () => {}),
  };
});

vi.mock('@wailsio/runtime', () => ({
  Events: {
    On: (name: string, cb: (e: { data: unknown }) => void) => {
      h.handlers.set(name, [...(h.handlers.get(name) ?? []), cb]);
      return () =>
        h.handlers.set(
          name,
          (h.handlers.get(name) ?? []).filter((x) => x !== cb),
        );
    },
    Emit: (...args: unknown[]) => h.emit(...(args as [string, unknown])),
  },
  Window: { Name: async () => 'history', Close: () => h.close() },
  Clipboard: { SetText: (t: string) => h.copy(t) },
  Dialogs: {
    Info: (o: unknown) => h.info(o),
    Error: (o: unknown) => h.error(o),
    Question: async (o: { Buttons: string[] }) => o.Buttons[h.question],
  },
  System: { IsDarkMode: async () => false },
}));
vi.mock('@bindings/cnb.cool/dtapp/kai/internal/service/historywrapper.ts', () => ({
  GetHistory: (
    kw: string,
    from: string,
    to: string,
    engine: string,
    offset: number,
    limit: number,
  ) => h.getHistory(kw, from, to, engine, offset, limit),
  CountHistory: (kw: string, from: string, to: string, engine: string) =>
    h.countHistory(kw, from, to, engine),
  DeleteHistory: (id: number) => h.del(id),
  ClearHistory: () => h.clear(),
}));

import HistoryScreen from './HistoryScreen.svelte';

let app: Record<string, unknown> | undefined;
let target: HTMLElement;

// Fake timers (the search debounce) with a frozen Date, so "today/yesterday" fixtures are
// stable. Microtasks are flushed by advancing by zero.
const flush = async () => {
  for (let i = 0; i < 6; i++) await vi.advanceTimersByTimeAsync(0);
  flushSync();
};
const advance = async (ms: number) => {
  await vi.advanceTimersByTimeAsync(ms);
  flushSync();
};

// The backend's filter, restated for the mock: keyword over text and result, the pair and
// the engine compared exactly, newest first, offset/limit.
function filterItems(
  items: Array<Record<string, unknown>>,
  kw: string,
  from: string,
  to: string,
  engine: string,
): Array<Record<string, unknown>> {
  const k = kw.trim().toLowerCase();
  return items
    .filter(
      (r) =>
        (!k ||
          String(r.text).toLowerCase().includes(k) ||
          String(r.result).toLowerCase().includes(k)) &&
        (!from || r.from_lang === from) &&
        (!to || r.to_lang === to) &&
        (!engine || r.engine === engine),
    )
    .sort((a, b) => Number(b.created_at) - Number(a.created_at));
}

const row = (over: Record<string, unknown>): Record<string, unknown> => ({
  id: 1,
  text: 'Hello there',
  result: 'Hola amigo',
  from_lang: 'en',
  to_lang: 'es',
  engine: 'google',
  from_ocr: false,
  created_at: Date.now() - 60_000,
  ...over,
});

let NOW = 0;
function resetStore(items: Array<Record<string, unknown>>) {
  h.items = items;
  h.getHistory.mockReset();
  h.countHistory.mockReset();
  h.getHistory.mockImplementation(
    (kw: string, from: string, to: string, engine: string, offset: number, limit: number) =>
      Promise.resolve(filterItems(h.items, kw, from, to, engine).slice(offset, offset + limit)),
  );
  h.countHistory.mockImplementation((kw: string, from: string, to: string, engine: string) =>
    Promise.resolve(filterItems(h.items, kw, from, to, engine).length),
  );
}

async function mountScreen() {
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(HistoryScreen, { target });
  await flush();
}

const rows = () => [...target.querySelectorAll('[data-testid="history-row"]')];
const text = () => target.textContent ?? '';
const key = (k: string, opts: KeyboardEventInit = {}) => {
  window.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, ...opts }));
};

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
  // Pin the clock to local midday (issue #96): the day headings depend on the local calendar day,
  // so between 01:00 and 02:00 local time the "26 hours ago" fixture fell two days back, under a date heading,
  // and the "Yesterday" heading was missing (CI failed at 01:07 UTC).
  vi.setSystemTime(new Date(2026, 9, 14, 12, 0, 0));
  NOW = Date.now();
  localStorage.clear();
  h.handlers.clear();
  h.emit.mockReset();
  h.close.mockReset();
  h.copy.mockReset();
  h.info.mockReset();
  h.error.mockReset();
  h.question = 0;
  h.del.mockReset();
  h.del.mockResolvedValue(undefined);
  h.clear.mockReset();
  h.clear.mockResolvedValue(undefined);
});

afterEach(() => {
  if (app) unmount(app);
  app = undefined;
  target?.remove();
  vi.useRealTimers();
});

describe('HistoryScreen, mounted (issue #60)', () => {
  it('opens loading the first page, grouped by day, with the time, pair, engine and OCR mark', async () => {
    resetStore([
      row({ id: 1, created_at: NOW - 60_000 }),
      row({ id: 2, created_at: NOW - 3_600_000 }),
      row({
        id: 3,
        text: 'Bonjour',
        result: 'Hola',
        from_lang: 'fr',
        to_lang: 'es',
        engine: 'apple',
        from_ocr: true,
        created_at: NOW - 26 * 3_600_000,
      }),
      row({ id: 4, created_at: NOW - 8 * 24 * 3_600_000 }),
    ]);
    await mountScreen();
    expect(rows()).toHaveLength(4);
    // Day headings: today, yesterday, and the date for older.
    expect(text()).toContain('Today');
    expect(text()).toContain('Yesterday');
    // The row meta: pair names, engine name, the OCR mark.
    expect(text()).toContain('English → Spanish');
    expect(text()).toContain('Google');
    expect(target.querySelector('[data-testid="history-row-ocr"]')?.textContent).toContain('OCR');
    // The first load asks the backend with empty filters and the first page.
    expect(h.getHistory).toHaveBeenCalledWith('', '', '', '', 0, 50);
    expect(h.countHistory).toHaveBeenCalledWith('', '', '', '');
  });

  it('searches over source and result after the debounce, and asks with the keyword', async () => {
    resetStore([
      row({ id: 1, text: 'Necesito los puntos clave', result: 'I need the key points' }),
      row({ id: 2, text: 'Goodbye', result: 'Adiós' }),
    ]);
    await mountScreen();
    const search = target.querySelector('[data-testid="history-search"]') as HTMLInputElement;
    search.value = 'puntos';
    search.dispatchEvent(new Event('input', { bubbles: true }));
    await advance(299);
    expect(h.getHistory).toHaveBeenCalledTimes(1); // still only the mount load
    await advance(1);
    expect(h.getHistory).toHaveBeenLastCalledWith('puntos', '', '', '', 0, 50);
    expect(rows()).toHaveLength(1);
    expect(text()).toContain('Necesito los puntos clave');
    // A keyword that matches nothing shows the "nothing matches" state, not the empty state.
    search.value = 'zzz-nothing';
    search.dispatchEvent(new Event('input', { bubbles: true }));
    await advance(300);
    expect(target.querySelector('[data-testid="history-no-match"]')).not.toBeNull();
  });

  it('shows the plain empty state when there is no history at all', async () => {
    resetStore([]);
    await mountScreen();
    expect(target.querySelector('[data-testid="history-empty"]')).not.toBeNull();
    expect(target.querySelector('[data-testid="history-no-match"]')).toBeNull();
  });

  it('filters by language pair and by engine, from the pairs and engines that are there', async () => {
    resetStore([
      row({ id: 1, from_lang: 'en', to_lang: 'es', engine: 'google' }),
      row({ id: 2, from_lang: 'es', to_lang: 'en', engine: 'google' }),
      row({ id: 3, from_lang: 'en', to_lang: 'es', engine: 'apple' }),
    ]);
    await mountScreen();
    // The filter options come from the data: the engines that produced rows, the pairs stored.
    const engineSel = target.querySelector(
      '[data-testid="history-engine-filter"]',
    ) as HTMLSelectElement;
    expect([...engineSel.options].map((o) => o.value)).toEqual(['', 'google', 'apple']);
    const pairSel = target.querySelector(
      '[data-testid="history-pair-filter"]',
    ) as HTMLSelectElement;
    expect([...pairSel.options].map((o) => o.value)).toEqual(['', 'en->es', 'es->en']);
    pairSel.value = 'es->en';
    pairSel.dispatchEvent(new Event('change', { bubbles: true }));
    await flush();
    expect(h.getHistory).toHaveBeenLastCalledWith('', 'es', 'en', '', 0, 50);
    expect(rows()).toHaveLength(1);

    pairSel.value = '';
    pairSel.dispatchEvent(new Event('change', { bubbles: true }));
    await flush();
    engineSel.value = 'apple';
    engineSel.dispatchEvent(new Event('change', { bubbles: true }));
    await flush();
    expect(h.getHistory).toHaveBeenLastCalledWith('', '', '', 'apple', 0, 50);
    expect(rows()).toHaveLength(1);
    expect(text()).toContain('System'); // engineName('apple')
  });

  it('Use broadcasts the entry with the exact wire payload and translates nothing', async () => {
    const item = row({
      id: 7,
      text: 'Necesito los puntos clave',
      result: 'I need the key points',
      from_lang: 'es',
      to_lang: 'en',
      engine: 'google',
      from_ocr: true,
    });
    resetStore([item]);
    await mountScreen();
    (rows()[0].querySelector('[data-testid="history-use"]') as HTMLButtonElement).click();
    await flush();
    expect(h.emit).toHaveBeenCalledWith('kai:history:use', {
      text: 'Necesito los puntos clave',
      result: 'I need the key points',
      from_lang: 'es',
      to_lang: 'en',
      engine: 'google',
    });
    // The list stays open (the user may bring back several entries), and nothing else is asked.
    expect(rows()).toHaveLength(1);
  });

  it('Copy puts the result on the clipboard', async () => {
    resetStore([row({ id: 1, result: 'Hola amigo' })]);
    await mountScreen();
    (rows()[0].querySelector('[data-testid="history-copy"]') as HTMLButtonElement).click();
    await flush();
    expect(h.copy).toHaveBeenCalledWith('Hola amigo');
  });

  it('Delete removes the one entry from the list without refetching', async () => {
    resetStore([row({ id: 1 }), row({ id: 2, created_at: NOW - 2 * 60_000 })]);
    await mountScreen();
    expect(rows()).toHaveLength(2);
    h.getHistory.mockClear();
    (rows()[1].querySelector('[data-testid="history-delete"]') as HTMLButtonElement).click();
    await flush();
    expect(h.del).toHaveBeenCalledWith(2);
    expect(rows()).toHaveLength(1);
    expect(h.getHistory).not.toHaveBeenCalled(); // updated locally, no reload
  });

  // Coverage ported from the WIP history-screen branch (710cd7b): the delete-failure branch —
  // the row stays and the error dialog says so.
  it('a failed delete keeps the entry and says so', async () => {
    resetStore([row({ id: 1 }), row({ id: 2, created_at: NOW - 2 * 60_000 })]);
    await mountScreen();
    h.del.mockImplementation(() => Promise.reject(new Error('db gone')));
    (rows()[1].querySelector('[data-testid="history-delete"]') as HTMLButtonElement).click();
    await flush();
    expect(rows()).toHaveLength(2); // nothing removed
    expect(h.error).toHaveBeenCalledTimes(1);
  });

  // Coverage ported from the WIP history-screen branch (710cd7b): keys pressed inside the
  // search box are the box's — Delete edits the text and removes no row.
  it('Delete in the search box edits the text and removes nothing', async () => {
    resetStore([row({ id: 1 }), row({ id: 2, created_at: NOW - 2 * 60_000 })]);
    await mountScreen();
    const search = target.querySelector('[data-testid="history-search"]') as HTMLInputElement;
    search.focus();
    // Bubbles up to the window listener, whose guard must see the input as the target.
    search.dispatchEvent(new KeyboardEvent('keydown', { key: 'Delete', bubbles: true }));
    await flush();
    expect(h.del).not.toHaveBeenCalled();
    expect(rows()).toHaveLength(2);
  });

  it('Clear all asks for a confirmation and only clears on the clearing answer', async () => {
    resetStore([row({ id: 1 })]);
    await mountScreen();
    const clearBtn = () =>
      target.querySelector('[data-testid="history-clear"]') as HTMLButtonElement;
    // Answer "cancel": nothing is cleared, the list stays.
    h.question = 1;
    clearBtn().click();
    await flush();
    expect(h.clear).not.toHaveBeenCalled();
    expect(rows()).toHaveLength(1);
    // Answer "clear": the store empties, the list shows the empty state.
    h.question = 0;
    clearBtn().click();
    await flush();
    expect(h.clear).toHaveBeenCalledTimes(1);
    expect(rows()).toHaveLength(0);
    expect(target.querySelector('[data-testid="history-empty"]')).not.toBeNull();
  });

  it('keyboard: up/down move the selection, Enter uses, Delete removes, Cmd+F focuses search', async () => {
    resetStore([
      row({ id: 1, created_at: NOW - 60_000 }),
      row({ id: 2, created_at: NOW - 2 * 60_000 }),
      row({ id: 3, created_at: NOW - 3 * 60_000 }),
    ]);
    await mountScreen();
    const selected = () => rows().find((r) => r.getAttribute('aria-selected') === 'true');
    key('ArrowDown');
    await flush();
    expect(selected()?.getAttribute('data-id')).toBe('1');
    key('ArrowDown');
    await flush();
    expect(selected()?.getAttribute('data-id')).toBe('2');
    key('ArrowUp');
    await flush();
    expect(selected()?.getAttribute('data-id')).toBe('1');
    // Enter uses the selected entry.
    h.emit.mockClear();
    key('Enter');
    await flush();
    expect(h.emit).toHaveBeenCalledTimes(1);
    expect(h.emit.mock.calls[0][0]).toBe('kai:history:use');
    // Delete removes the selected entry.
    key('Delete');
    await flush();
    expect(h.del).toHaveBeenCalledWith(1);
    expect(rows()).toHaveLength(2);
    // Cmd+F focuses the search box (and the keys inside it are the box's, not the list's).
    const search = target.querySelector('[data-testid="history-search"]') as HTMLInputElement;
    key('f', { metaKey: true });
    expect(document.activeElement).toBe(search);
  });

  it('Esc closes the screen (the window), not the list', async () => {
    resetStore([row({ id: 1 })]);
    await mountScreen();
    key('Escape');
    await flush();
    expect(h.close).toHaveBeenCalledTimes(1);
  });

  it('long texts clamp behind a show more that unfolds and folds them', async () => {
    const long = 'a'.repeat(400);
    resetStore([row({ id: 1, text: long, result: long })]);
    await mountScreen();
    const textEl = target.querySelector('[data-testid="history-row-text"]') as HTMLElement;
    expect(textEl.textContent).toBe('a'.repeat(300));
    const more = target.querySelector('[data-testid="history-show-more"]') as HTMLButtonElement;
    more.click();
    await flush();
    expect(target.querySelector('[data-testid="history-row-text"]')?.textContent).toBe(long);
    expect(more.textContent).toContain('Show less');
  });

  it('loads the next page as the user asks for more, not all at once', async () => {
    const many = Array.from({ length: 120 }, (_, i) =>
      row({ id: i + 1, created_at: NOW - (i + 1) * 60_000 }),
    );
    resetStore(many);
    await mountScreen();
    // First page only.
    expect(rows()).toHaveLength(50);
    expect(h.getHistory).toHaveBeenLastCalledWith('', '', '', '', 0, 50);
    (target.querySelector('[data-testid="history-load-more"]') as HTMLButtonElement).click();
    await flush();
    expect(h.getHistory).toHaveBeenLastCalledWith('', '', '', '', 50, 50);
    expect(rows()).toHaveLength(100);
    // No more button once everything is loaded.
    (target.querySelector('[data-testid="history-load-more"]') as HTMLButtonElement).click();
    await flush();
    expect(rows()).toHaveLength(120);
    expect(target.querySelector('[data-testid="history-load-more"]')).toBeNull();
  });
});
