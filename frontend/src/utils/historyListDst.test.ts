import { afterEach, beforeAll, describe, expect, it } from 'vitest';

// PR #67 review: groupDays decided "yesterday" with `startOfDay(now) - 86400000`, a fixed
// 24-hour step. On the day after a DST spring-forward the local day is 23 hours, so
// midnight-to-midnight is NOT 24h apart and a row genuinely from yesterday was labelled with
// an "older" date heading instead of "Yesterday". This file pins the calendar-day rule in a
// timezone that really transitions (America/New_York, spring forward 2026-03-08 02:00),
// because in a zone without transitions the 24h arithmetic is accidentally correct.

process.env.TZ = 'America/New_York';

import { groupDays, type HistoryRow } from './historyList.ts';

const row = (created_at: number): HistoryRow => ({
  id: 1,
  text: 'Hello',
  result: 'Hola',
  from_lang: 'en',
  to_lang: 'es',
  engine: 'google',
  from_ocr: false,
  created_at,
});

// The zone the runner started in, restored after this file so any test that runs later in the
// same worker process sees it again.
const hadTZ = process.env.TZ;

describe('groupDays across a DST transition (PR #67 review)', () => {
  beforeAll(() => {
    // The fixture is only meaningful where the zone transitions; say so instead of silently
    // testing nothing.
    const before = new Date(2026, 2, 1, 12).getTimezoneOffset(); // Mar 1, EST
    const after = new Date(2026, 2, 8, 12).getTimezoneOffset(); // Mar 8, EDT
    expect(after).not.toBe(before);
  });

  afterEach(() => {
    process.env.TZ = hadTZ;
  });

  it('labels the spring-forward day before now as Yesterday, not as an older date', () => {
    // Now: Monday 2026-03-09 12:00 EDT. Yesterday is Sunday 2026-03-08 — the 23-hour day.
    const now = new Date(2026, 2, 9, 12, 0, 0, 0).getTime();
    const sunday = new Date(2026, 2, 8, 12, 0, 0, 0).getTime();
    const groups = groupDays([row(sunday)], now);
    expect(groups).toHaveLength(1);
    expect(groups[0].kind).toBe('yesterday');
    expect(groups[0].key).toBe('2026-03-08');
  });

  it('keeps Today across the transition night, and keeps older days older', () => {
    const now = new Date(2026, 2, 9, 12, 0, 0, 0).getTime();
    const groups = groupDays(
      [
        row(new Date(2026, 2, 9, 1, 30, 0, 0).getTime()), // after the earlier transition night
        row(new Date(2026, 2, 7, 18, 0, 0, 0).getTime()), // two calendar days back
      ],
      now,
    );
    expect(groups.map((g) => g.kind)).toEqual(['today', 'older']);
    expect(groups[1].key).toBe('2026-03-07');
  });
});
