import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// PR #67 review (307d666): clearAll and deleteRow change rows/total locally without advancing
// loadToken, so a page append that left before the user's action resolves afterwards and
// writes its stale rows and count back over the emptied or shortened list. This file pins the
// class rule — every local list mutation invalidates in-flight loads — with a GetHistory whose
// page-2 promise is resolved BY HAND after the user's action: (a) clear all, (b) deleting a
// page-1 row, (c) a keyword change (which reloads, and so must already invalidate). The REAL
// HistoryScreen is mounted; only the Wails boundary is faked.

const h = vi.hoisted(() => ({
  items: [] as Array<Record<string, unknown>>,
  getHistory: vi.fn(),
  countHistory: vi.fn(),
  del: vi.fn(async (_id: number) => {}),
  clear: vi.fn(async () => {}),
  // The gate: while set, the page-2 GetHistory call parks and hands back its resolver.
  gateOffset: -1,
  release: null as null | (() => void),
}));

vi.mock('@wailsio/runtime', () => ({
  Events: { On: () => () => {}, Emit: vi.fn() },
  Window: { Name: async () => 'history', Close: vi.fn(async () => {}) },
  Clipboard: { SetText: vi.fn(async () => {}) },
  Dialogs: {
    Info: vi.fn(async () => {}),
    Error: vi.fn(async () => {}),
    // The confirming button (Buttons[0]) is the component's "clear" answer.
    Question: async (o: { Buttons: string[] }) => o.Buttons[0],
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

const flush = async () => {
  for (let i = 0; i < 6; i++) await vi.advanceTimersByTimeAsync(0);
  flushSync();
};
const advance = async (ms: number) => {
  await vi.advanceTimersByTimeAsync(ms);
  flushSync();
};

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

function resetStore(n: number) {
  h.items = Array.from({ length: n }, (_, i) =>
    row({ id: i + 1, text: `Hello ${i + 1}`, created_at: Date.now() - (i + 1) * 60_000 }),
  );
  h.getHistory.mockReset();
  h.countHistory.mockReset();
  h.del.mockClear().mockResolvedValue(undefined);
  h.clear.mockClear().mockResolvedValue(undefined);
  h.gateOffset = -1;
  h.release = null;
  h.getHistory.mockImplementation(
    (kw: string, _from: string, _to: string, _engine: string, offset: number, limit: number) => {
      if (offset === h.gateOffset) {
        return new Promise((resolve) => {
          h.release = () => resolve(h.items.slice(offset, offset + limit));
        });
      }
      const k = kw.trim().toLowerCase();
      return Promise.resolve(
        h.items
          .filter((r) => !k || String(r.text).toLowerCase().includes(k))
          .slice(offset, offset + limit),
      );
    },
  );
  h.countHistory.mockImplementation((kw: string) => {
    const k = kw.trim().toLowerCase();
    return Promise.resolve(
      h.items.filter((r) => !k || String(r.text).toLowerCase().includes(k)).length,
    );
  });
}

async function mountScreen() {
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(HistoryScreen, { target });
  await flush();
}

const rows = () => [...target.querySelectorAll('[data-testid="history-row"]')];
const loadMore = () =>
  target.querySelector('[data-testid="history-load-more"]') as HTMLButtonElement;

// Mounts 120 entries, loads page 1, then starts the gated page-2 append (rows stay 50 until
// the gate is released by hand).
async function mountWithGatedPage2() {
  resetStore(120);
  h.gateOffset = 50;
  await mountScreen();
  expect(rows()).toHaveLength(50);
  loadMore().click();
  await flush();
  expect(h.release).not.toBeNull(); // the append is parked mid-flight
  expect(rows()).toHaveLength(50);
}

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
  localStorage.clear();
});

afterEach(() => {
  if (app) unmount(app);
  app = undefined;
  target?.remove();
  vi.useRealTimers();
});

describe('HistoryList: a late append must not undo the user (PR #67 review)', () => {
  it('(a) clear all: the emptied list stays empty', async () => {
    await mountWithGatedPage2();
    (target.querySelector('[data-testid="history-clear"]') as HTMLButtonElement).click();
    await flush();
    expect(h.clear).toHaveBeenCalledTimes(1);
    expect(rows()).toHaveLength(0);
    expect(target.querySelector('[data-testid="history-empty"]')).not.toBeNull();
    // The parked page 2 resolves NOW, after the clear emptied the list: it must be dropped.
    h.release!();
    await flush();
    expect(rows()).toHaveLength(0);
    expect(target.querySelector('[data-testid="history-empty"]')).not.toBeNull();
    expect(target.querySelector('[data-testid="history-load-more"]')).toBeNull();
  });

  it('(b) deleting a page-1 row: the shortened list stays shortened', async () => {
    await mountWithGatedPage2();
    const first = rows()[0];
    (first.querySelector('[data-testid="history-delete"]') as HTMLButtonElement).click();
    await flush();
    expect(rows()).toHaveLength(49); // page 1 minus the deleted row
    // The parked page 2 resolves NOW, after the deletion shortened the list: it must be
    // dropped.
    h.release!();
    await flush();
    expect(rows()).toHaveLength(49);
    expect(target.textContent).not.toContain('Hello 51');
  });

  it('(c) a keyword change: the reload wins, the parked append is dropped', async () => {
    await mountWithGatedPage2();
    const search = target.querySelector('[data-testid="history-search"]') as HTMLInputElement;
    search.value = 'zzz-nothing';
    search.dispatchEvent(new Event('input', { bubbles: true }));
    await advance(300); // the debounce fires the reload (a fresh load, its own token)
    await flush();
    expect(target.querySelector('[data-testid="history-no-match"]')).not.toBeNull();
    // The parked page 2 resolves now, after the reload answered: it must be dropped.
    h.release!();
    await flush();
    expect(rows()).toHaveLength(0);
    expect(target.querySelector('[data-testid="history-no-match"]')).not.toBeNull();
  });
});
