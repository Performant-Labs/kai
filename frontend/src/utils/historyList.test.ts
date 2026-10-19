import { describe, expect, it } from 'vitest';

// Issue #60: the history screen's pure half — day grouping, the row time label, the text
// clamp, the filter options derived from the loaded rows, and the "Use" mapping (the event
// payload and what the translate window restores from it). DOM, bindings and storage free,
// like translateSession.ts / swapLangs.ts.

import {
  CLAMP_CHARS,
  canLoadMore,
  clampText,
  dayKey,
  engineOptions,
  groupDays,
  pairOptions,
  restoredPair,
  restoredResult,
  rowTimeLabel,
  usePayload,
  type HistoryRow,
} from './historyList.ts';

// A fixed "now": 2026-10-19 (a Monday) 15:00 local.
const NOW = new Date(2026, 9, 19, 15, 0, 0, 0).getTime();
const day = (y: number, m: number, d: number, h = 12, min = 0) =>
  new Date(y, m, d, h, min, 0, 0).getTime();

const row = (over: Partial<HistoryRow>): HistoryRow => ({
  id: 1,
  text: 'Hello',
  result: 'Hola',
  from_lang: 'en',
  to_lang: 'es',
  engine: 'google',
  from_ocr: false,
  created_at: NOW,
  ...over,
});

describe('day grouping (issue #60)', () => {
  it('groups rows into today / yesterday / older by local calendar day, newest first', () => {
    const groups = groupDays(
      [
        row({ id: 1, created_at: day(2026, 9, 19, 14, 2) }),
        row({ id: 2, created_at: day(2026, 9, 19, 9, 30) }),
        row({ id: 3, created_at: day(2026, 9, 18, 22, 0) }),
        row({ id: 4, created_at: day(2026, 9, 12, 8, 0) }),
        row({ id: 5, created_at: day(2026, 8, 30, 8, 0) }),
      ],
      NOW,
    );
    expect(groups.map((g) => g.kind)).toEqual(['today', 'yesterday', 'older', 'older']);
    expect(groups[0].items.map((i) => i.id)).toEqual([1, 2]);
    expect(groups[1].items.map((i) => i.id)).toEqual([3]);
    expect(groups[3].items.map((i) => i.id)).toEqual([5]);
    // The older groups carry their own date key for the heading.
    expect(groups[2].date).toBe('2026-10-12');
    expect(groups[3].date).toBe('2026-09-30');
  });

  it('starts a new group when the day changes between rows, and keeps backend order', () => {
    const groups = groupDays(
      [
        row({ id: 2, created_at: day(2026, 9, 18) }),
        row({ id: 1, created_at: day(2026, 9, 19) }), // out of order on purpose
      ],
      NOW,
    );
    // Grouping follows the rows as they come (newest first from the backend): two groups.
    expect(groups.map((g) => g.kind)).toEqual(['yesterday', 'today']);
  });

  it('an empty list groups into nothing', () => {
    expect(groupDays([], NOW)).toEqual([]);
  });

  it('dayKey is the local calendar day, not the UTC one', () => {
    // 2026-10-19 00:30 local is 2026-10-18 in UTC; the label must be the local day.
    expect(dayKey(day(2026, 9, 19, 0, 30))).toBe('2026-10-19');
  });
});

describe('row time label (issue #60)', () => {
  it('shows the clock time for today, the date for older days', () => {
    expect(rowTimeLabel(day(2026, 9, 19, 14, 2), NOW)).toBe('14:02');
    expect(rowTimeLabel(day(2026, 9, 19, 0, 5), NOW)).toBe('00:05');
    expect(rowTimeLabel(day(2026, 9, 18, 14, 2), NOW)).toBe('2026-10-18');
    expect(rowTimeLabel(day(2026, 8, 30, 14, 2), NOW)).toBe('2026-09-30');
  });
});

describe('text clamp (issue #60)', () => {
  it('clamps only texts longer than the cap, without expanding short ones', () => {
    expect(clampText('short')).toBe('short');
    expect(clampText('a'.repeat(CLAMP_CHARS))).toBe('a'.repeat(CLAMP_CHARS));
    const long = 'a'.repeat(CLAMP_CHARS + 10);
    expect(clampText(long)).toBe('a'.repeat(CLAMP_CHARS));
    expect(clampText(long).length).toBe(CLAMP_CHARS);
  });
});

describe('filter options derived from the loaded rows (issue #60)', () => {
  it('lists the distinct language pairs, newest rows first, empty pair fields skipped', () => {
    const opts = pairOptions([
      row({ from_lang: 'en', to_lang: 'es' }),
      row({ from_lang: 'en', to_lang: 'es' }),
      row({ from_lang: 'es', to_lang: 'en' }),
      row({ from_lang: '', to_lang: 'es' }),
    ]);
    expect(opts).toEqual([
      { value: 'en->es', from: 'en', to: 'es' },
      { value: 'es->en', from: 'es', to: 'en' },
    ]);
  });

  it('lists the distinct engines that produced rows', () => {
    expect(
      engineOptions([row({ engine: 'google' }), row({ engine: '' }), row({ engine: 'apple' })]),
    ).toEqual(['google', 'apple']);
  });
});

describe('the Use mapping (issue #60)', () => {
  const item = row({
    id: 7,
    text: 'Necesito los puntos clave',
    result: 'I need the key points',
    from_lang: 'es',
    to_lang: 'en',
    engine: 'google',
    from_ocr: true,
    created_at: day(2026, 9, 19, 14, 2),
  });

  it('usePayload carries exactly the wire fields', () => {
    expect(usePayload(item)).toEqual({
      text: 'Necesito los puntos clave',
      result: 'I need the key points',
      from_lang: 'es',
      to_lang: 'en',
      engine: 'google',
    });
  });

  it('restoredResult is a result entry the pane can show as-is', () => {
    expect(restoredResult(usePayload(item))).toEqual({
      engine: 'google',
      result: 'I need the key points',
      from: 'es',
      to: 'en',
    });
  });

  it('restoredPair assigns only values the selects can hold; null keeps the current one', () => {
    const source = ['auto', 'en', 'es', 'es-MX'];
    const target = ['en', 'fr', 'es-MX'];
    expect(restoredPair(usePayload(item), source, target)).toEqual({ from: 'es', to: 'en' });
    // A pair the current options cannot hold (bare es not a target option) keeps that side.
    expect(restoredPair(usePayload(row({ to_lang: 'es' })), source, target)).toEqual({
      from: 'en',
      to: null,
    });
    // Empty stored fields (an OCR row may have no source language) keep both.
    expect(restoredPair(usePayload(row({ from_lang: '', to_lang: '' })), source, target)).toEqual({
      from: null,
      to: null,
    });
  });
});

describe('paged loading (issue #60)', () => {
  it('loads more only while fewer rows than the total are shown', () => {
    expect(canLoadMore(0, 0)).toBe(false);
    expect(canLoadMore(50, 50)).toBe(false);
    expect(canLoadMore(50, 120)).toBe(true);
  });
});
