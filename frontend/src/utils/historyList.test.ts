import { describe, expect, it } from 'vitest';
import {
  appendPage,
  emptyFilter,
  emptyKind,
  emptyList,
  formatRowTime,
  groupByDay,
  hasMore,
  isFiltered,
  isLongText,
  moveSelection,
  removeRow,
  replacePage,
  shouldLoadMore,
  type HistoryRow,
} from './historyList.ts';

// Issue #60: the pure half of the History screen. Dates are built in local time, the same clock the
// day grouping reads, so the tests do not depend on the machine's time zone.
const row = (id: number, at: number, extra: Partial<HistoryRow> = {}): HistoryRow => ({
  id,
  text: `source ${id}`,
  result: `result ${id}`,
  from_lang: 'es-MX',
  to_lang: 'en',
  engine: 'google',
  engine_id: 1,
  from_ocr: false,
  created_at: at,
  ...extra,
});
const NOW = new Date(2026, 9, 8, 15, 0, 0).getTime();
const at = (m: number, d: number, h = 12, min = 0, y = 2026) =>
  new Date(y, m, d, h, min, 0).getTime();

describe('filter', () => {
  it('an empty filter is not a filter', () => {
    expect(isFiltered(emptyFilter())).toBe(false);
    expect(isFiltered({ ...emptyFilter(), keyword: '   ' })).toBe(false);
  });
  it.each([
    ['keyword', { keyword: 'x' }],
    ['source language', { from_lang: 'en' }],
    ['target language', { to_lang: 'en' }],
    ['engine', { engine_id: 3 }],
  ])('a %s makes it a filter', (_n, patch) => {
    expect(isFiltered({ ...emptyFilter(), ...patch })).toBe(true);
  });
});

describe('list state', () => {
  const page = (ids: number[], total: number) => ({
    items: ids.map((i) => row(i, NOW - i)),
    total,
  });
  it('starts not loaded, with nothing more to load', () => {
    const s = emptyList();
    expect(s.loaded).toBe(false);
    expect(hasMore(s)).toBe(false);
  });
  it('the first page replaces the list and says whether more is there', () => {
    const s = replacePage(page([1, 2], 5));
    expect(s.items.map((r) => r.id)).toEqual([1, 2]);
    expect(s.loaded).toBe(true);
    expect(hasMore(s)).toBe(true);
  });
  it('a next page is appended, and a row seen twice is kept once', () => {
    const s = appendPage(replacePage(page([1, 2], 4)), page([2, 3, 4], 4));
    expect(s.items.map((r) => r.id)).toEqual([1, 2, 3, 4]);
    expect(hasMore(s)).toBe(false);
  });
  it('a page that is empty ends the list even if the total claims more', () => {
    const s = appendPage(replacePage(page([1], 9)), page([], 9));
    expect(hasMore(s)).toBe(false);
  });
  it('removing a row takes it out and lowers the total', () => {
    const s = removeRow(replacePage(page([1, 2, 3], 3)), 2);
    expect(s.items.map((r) => r.id)).toEqual([1, 3]);
    expect(s.total).toBe(2);
  });
  it('removing a row that is not there changes nothing', () => {
    const s = replacePage(page([1], 1));
    expect(removeRow(s, 99)).toEqual(s);
  });
  it('words the empty list differently with and without a filter', () => {
    expect(emptyKind(emptyList(), emptyFilter())).toBe('none');
    expect(emptyKind(replacePage(page([1], 1)), emptyFilter())).toBe('none');
    expect(emptyKind(replacePage(page([], 0)), emptyFilter())).toBe('empty');
    expect(emptyKind(replacePage(page([], 0)), { ...emptyFilter(), keyword: 'zzz' })).toBe(
      'nomatch',
    );
  });
});

describe('shouldLoadMore', () => {
  const geo = (scrollTop: number) => ({ scrollTop, clientHeight: 500, scrollHeight: 2000 });
  it('loads when the user is near the bottom', () => {
    expect(shouldLoadMore(geo(1400), { loading: false, hasMore: true })).toBe(true);
  });
  it('waits while the user is far from the bottom', () => {
    expect(shouldLoadMore(geo(100), { loading: false, hasMore: true })).toBe(false);
  });
  it('does not load while a page is loading or when nothing is left', () => {
    expect(shouldLoadMore(geo(1500), { loading: true, hasMore: true })).toBe(false);
    expect(shouldLoadMore(geo(1500), { loading: false, hasMore: false })).toBe(false);
  });
  it('loads when the rows do not fill the viewport yet', () => {
    expect(
      shouldLoadMore(
        { scrollTop: 0, clientHeight: 800, scrollHeight: 300 },
        { loading: false, hasMore: true },
      ),
    ).toBe(true);
  });
});

describe('groupByDay', () => {
  it('groups newest-first rows by local day, in order', () => {
    const rows = [
      row(1, at(9, 8, 14, 2)),
      row(2, at(9, 8, 9, 0)),
      row(3, at(9, 7, 23, 59)),
      row(4, at(9, 1)),
      row(5, at(9, 1, 8)),
    ];
    const groups = groupByDay(rows, NOW);
    expect(groups.map((g) => [g.kind, g.rows.map((r) => r.id)])).toEqual([
      ['today', [1, 2]],
      ['yesterday', [3]],
      ['date', [4, 5]],
    ]);
  });
  it('a row just after midnight is today, one just before it is yesterday', () => {
    const g = groupByDay([row(1, at(9, 8, 0, 0)), row(2, at(9, 7, 23, 59))], NOW);
    expect(g.map((x) => x.kind)).toEqual(['today', 'yesterday']);
  });
  it('gives each group a distinct key', () => {
    const g = groupByDay([row(1, at(9, 8)), row(2, at(9, 7)), row(3, at(9, 1))], NOW);
    expect(new Set(g.map((x) => x.key)).size).toBe(3);
  });
  it('is empty for no rows', () => {
    expect(groupByDay([], NOW)).toEqual([]);
  });
});

describe('formatRowTime', () => {
  it('shows the clock for today', () => {
    expect(formatRowTime(at(9, 8, 14, 2), NOW, 'en-US')).toBe('14:02');
    expect(formatRowTime(at(9, 8, 9, 5), NOW, 'en-US')).toBe('09:05');
  });
  it('shows the date for an older day, with the year only when it differs', () => {
    expect(formatRowTime(at(9, 1), NOW, 'en-US')).toBe('Oct 1');
    expect(formatRowTime(at(11, 24, 12, 0, 2025), NOW, 'en-US')).toBe('Dec 24, 2025');
  });
  it('shows the clock for yesterday too, because its group already says the day', () => {
    expect(formatRowTime(at(9, 7, 21, 30), NOW, 'en-US')).toBe('21:30');
  });
});

describe('moveSelection', () => {
  it('Down from nothing selects the first row, Up from nothing the last', () => {
    expect(moveSelection(-1, 'ArrowDown', 3)).toBe(0);
    expect(moveSelection(-1, 'ArrowUp', 3)).toBe(2);
  });
  it('moves by one and stops at the ends', () => {
    expect(moveSelection(0, 'ArrowDown', 3)).toBe(1);
    expect(moveSelection(2, 'ArrowDown', 3)).toBe(2);
    expect(moveSelection(1, 'ArrowUp', 3)).toBe(0);
    expect(moveSelection(0, 'ArrowUp', 3)).toBe(0);
  });
  it('selects nothing in an empty list', () => {
    expect(moveSelection(-1, 'ArrowDown', 0)).toBe(-1);
  });
  it('pulls a selection that is past the end back in', () => {
    expect(moveSelection(7, 'ArrowDown', 3)).toBe(2);
  });
});

describe('isLongText', () => {
  it('short single lines are not long', () => {
    expect(isLongText('hello')).toBe(false);
  });
  it('more than three lines is long', () => {
    expect(isLongText('a\nb\nc')).toBe(false);
    expect(isLongText('a\nb\nc\nd')).toBe(true);
  });
  it('a very long single line is long', () => {
    expect(isLongText('x'.repeat(400))).toBe(true);
  });
});
