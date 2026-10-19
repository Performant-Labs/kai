import { mount, unmount, flushSync } from 'svelte';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// PR #67 review (8189ccb): the auto-scroll pagination never triggered. The IntersectionObserver
// was registered once in onMount, when the sentinel is not rendered yet (canLoadMore is false
// with the still-empty list), so the null check skipped registration forever and later pages
// only loaded through the Load more button. This file pins the real rule with the REAL
// HistoryScreen (HistoryList inside), the Wails boundary faked and a controllable
// IntersectionObserver stubbed in: the sentinel is observed as soon as it appears, an
// intersection loads the next page, a still-visible sentinel re-arms (the observer is rebuilt
// per appended page), and the observer is disconnected once everything is loaded.

const h = vi.hoisted(() => ({
  items: [] as Array<Record<string, unknown>>,
  getHistory: vi.fn(),
  countHistory: vi.fn(),
}));

vi.mock('@wailsio/runtime', () => ({
  Events: { On: () => () => {}, Emit: vi.fn() },
  Window: { Name: async () => 'history', Close: vi.fn(async () => {}) },
  Clipboard: { SetText: vi.fn(async () => {}) },
  Dialogs: { Info: vi.fn(async () => {}), Error: vi.fn(async () => {}) },
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
  DeleteHistory: vi.fn(async () => {}),
  ClearHistory: vi.fn(async () => {}),
}));

import HistoryScreen from './HistoryScreen.svelte';

// A controllable IntersectionObserver: instances are recorded so a test can fire one, and
// observe/disconnect are recorded so the wiring itself is what is asserted.
class MockIntersectionObserver {
  static instances: MockIntersectionObserver[] = [];
  callback: (entries: Array<{ isIntersecting: boolean }>) => void;
  observed: unknown[] = [];
  disconnected = false;

  constructor(
    cb: (entries: Array<{ isIntersecting: boolean }>) => void,
    _opts?: { root?: Element | null; [k: string]: unknown },
  ) {
    this.callback = cb;
    MockIntersectionObserver.instances.push(this);
  }

  observe(el: unknown) {
    this.observed.push(el);
  }

  unobserve() {}

  disconnect() {
    this.disconnected = true;
  }
}

let app: Record<string, unknown> | undefined;
let target: HTMLElement;

const flush = async () => {
  for (let i = 0; i < 6; i++) await vi.advanceTimersByTimeAsync(0);
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
    row({ id: i + 1, created_at: Date.now() - (i + 1) * 60_000 }),
  );
  h.getHistory.mockReset();
  h.countHistory.mockReset();
  h.getHistory.mockImplementation(
    (_kw: string, _from: string, _to: string, _engine: string, offset: number, limit: number) =>
      Promise.resolve(h.items.slice(offset, offset + limit)),
  );
  h.countHistory.mockImplementation(() => Promise.resolve(h.items.length));
}

async function mountScreen() {
  target = document.createElement('div');
  document.body.appendChild(target);
  app = mount(HistoryScreen, { target });
  await flush();
}

const rows = () => [...target.querySelectorAll('[data-testid="history-row"]')];
const lastIO = () =>
  MockIntersectionObserver.instances[MockIntersectionObserver.instances.length - 1];
const intersect = (io: MockIntersectionObserver) => io.callback([{ isIntersecting: true }]);

beforeEach(() => {
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'Date'] });
  localStorage.clear();
  MockIntersectionObserver.instances = [];
  vi.stubGlobal('IntersectionObserver', MockIntersectionObserver);
});

afterEach(() => {
  if (app) unmount(app);
  app = undefined;
  target?.remove();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

describe('HistoryList scroll pagination (PR #67 review)', () => {
  it('observes the sentinel once the first page leaves more to load, and an intersection loads the next page', async () => {
    resetStore(120);
    await mountScreen();
    expect(rows()).toHaveLength(50);
    // The sentinel appears with the first page; it must have been observed (not only on
    // mount, when it did not exist yet).
    expect(MockIntersectionObserver.instances.length).toBeGreaterThan(0);
    const io = MockIntersectionObserver.instances[0];
    expect(io.observed).toHaveLength(1);

    intersect(io);
    await flush();
    expect(h.getHistory).toHaveBeenLastCalledWith('', '', '', '', 50, 50);
    expect(rows()).toHaveLength(100);
  });

  it('re-arms while the sentinel stays visible, and disconnects once everything is loaded', async () => {
    resetStore(120);
    await mountScreen();
    intersect(MockIntersectionObserver.instances[0]);
    await flush();
    // The appended page re-armed the observer (a fresh one watches the sentinel again), so a
    // still-visible sentinel keeps loading instead of waiting for a scroll event.
    expect(MockIntersectionObserver.instances.length).toBeGreaterThan(1);
    const second = MockIntersectionObserver.instances[1];
    expect(second.observed).toHaveLength(1);
    expect(MockIntersectionObserver.instances[0].disconnected).toBe(true);

    intersect(second);
    await flush();
    expect(rows()).toHaveLength(120);
    // All loaded: the sentinel is gone and its observer was disconnected; no new one appears.
    const last = lastIO();
    expect(last.disconnected).toBe(true);
    const count = MockIntersectionObserver.instances.length;
    await flush();
    expect(MockIntersectionObserver.instances.length).toBe(count);
    expect(target.querySelector('[data-testid="history-load-more"]')).toBeNull();
  });

  it('creates no observer while everything fits one page', async () => {
    resetStore(10);
    await mountScreen();
    expect(rows()).toHaveLength(10);
    expect(MockIntersectionObserver.instances).toHaveLength(0);
  });
});
