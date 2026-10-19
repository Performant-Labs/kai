// The history screen's pure half (issue #60).
//
// The screen itself (the list, the filters, the keyboard) is a component; what lives here is
// every rule that can be stated without the DOM, the bindings or storage, so vitest covers it
// directly (the same split as translateSession.ts / swapLangs.ts):
//   - the day grouping of the list (Today / Yesterday / a date heading);
//   - the per-row time label (the clock time today, the date for older ones);
//   - the text clamp behind a row's "show more";
//   - the language-pair and engine filter options, derived from the rows that are loaded
//     ("from the data that is there");
//   - the "Use" mapping: the event payload a row produces, and what the translate window
//     restores from it — the pane result, and the pair the two selects can actually hold.
//
// Nothing here translates anything: "Use" only maps data; the translate window owns what
// happens on receipt.

/** One history entry as the screen holds it (the HistoryItem wire shape, structural). */
export type HistoryRow = {
  id: number;
  text: string;
  result: string;
  from_lang: string;
  to_lang: string;
  engine: string;
  from_ocr: boolean;
  created_at: number;
};

/** The wire payload of a "Use" (aligned with internal/events HistoryUsePayload). */
export type HistoryUse = {
  text: string;
  result: string;
  from_lang: string;
  to_lang: string;
  engine: string;
};

/** A group of rows under one day heading. */
export type DayGroup = {
  /** 'YYYY-MM-DD' of the group's day (local calendar day). */
  key: string;
  /** Which heading the group gets: a word for today/yesterday, the date otherwise. */
  kind: 'today' | 'yesterday' | 'older';
  /** The date as 'YYYY-MM-DD', for the 'older' headings. */
  date: string;
  items: HistoryRow[];
};

// pad2: 4 -> '04'.
const pad2 = (n: number) => String(n).padStart(2, '0');

/** The local calendar day of a timestamp, as 'YYYY-MM-DD'. */
export function dayKey(ts: number): string {
  const d = new Date(ts);
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
}

function startOfDay(ts: number): number {
  const d = new Date(ts);
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}

// The day-start of the local calendar day BEFORE ts's. A calendar-day step (setDate), never
// `ts - 86400000`: on the day after a DST transition a local day is 23 or 25 hours, so a
// fixed 24-hour subtraction lands an hour off midnight and mislabels yesterday's rows as
// older (PR #67 review).
function startOfPrevDay(ts: number): number {
  const d = new Date(ts);
  d.setDate(d.getDate() - 1);
  d.setHours(0, 0, 0, 0);
  return d.getTime();
}

/**
 * Groups rows into day headings, following the order they arrive in (the backend's newest
 * first): a row opens a new group whenever its calendar day differs from the previous row's.
 * `now` decides which days are "today" and "yesterday" (calendar-day comparisons, so a DST
 * day is labelled by its date, not by being exactly 24 hours long).
 */
export function groupDays(items: HistoryRow[], now: number): DayGroup[] {
  const groups: DayGroup[] = [];
  const today = startOfDay(now);
  const yesterday = startOfPrevDay(now);
  for (const item of items) {
    const key = dayKey(item.created_at);
    const last = groups[groups.length - 1];
    if (last && last.key === key) {
      last.items.push(item);
      continue;
    }
    const start = startOfDay(item.created_at);
    const kind: DayGroup['kind'] =
      start === today ? 'today' : start === yesterday ? 'yesterday' : 'older';
    groups.push({ key, kind, date: key, items: [item] });
  }
  return groups;
}

/**
 * A row's time label: the clock time ('HH:MM') for rows of today, the date ('YYYY-MM-DD') for
 * older ones — the clock time of another day is useless next to its day heading.
 */
export function rowTimeLabel(ts: number, now: number): string {
  if (dayKey(ts) === dayKey(now)) {
    const d = new Date(ts);
    return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
  }
  return dayKey(ts);
}

/** The clamp cap: rows longer than this fold behind "show more". */
export const CLAMP_CHARS = 300;

/** The text a clamped row shows. Short texts pass through unchanged. */
export function clampText(text: string): string {
  return text.length > CLAMP_CHARS ? text.slice(0, CLAMP_CHARS) : text;
}

/** One language-pair filter option, derived from the loaded rows. */
export type PairOption = {
  /** The select's value ('from->to'), stable and unique per pair. */
  value: string;
  from: string;
  to: string;
};

/**
 * The distinct language pairs of the loaded rows, in first-seen order (the backend's newest
 * first). Rows without a pair (an OCR entry may have stored none) are skipped.
 */
export function pairOptions(rows: HistoryRow[]): PairOption[] {
  const seen = new Set<string>();
  const out: PairOption[] = [];
  for (const r of rows) {
    if (!r.from_lang || !r.to_lang) continue;
    const value = `${r.from_lang}->${r.to_lang}`;
    if (seen.has(value)) continue;
    seen.add(value);
    out.push({ value, from: r.from_lang, to: r.to_lang });
  }
  return out;
}

/** The distinct engines of the loaded rows, first-seen order; empty names are skipped. */
export function engineOptions(rows: HistoryRow[]): string[] {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const r of rows) {
    if (!r.engine || seen.has(r.engine)) continue;
    seen.add(r.engine);
    out.push(r.engine);
  }
  return out;
}

/** The "Use" event payload of a row: exactly the fields the translate window restores. */
export function usePayload(item: HistoryRow): HistoryUse {
  return {
    text: item.text,
    result: item.result,
    from_lang: item.from_lang,
    to_lang: item.to_lang,
    engine: item.engine,
  };
}

/**
 * The result entry the pane shows after a "Use": the stored translation as a finished result
 * of its own engine, in its own pair. It satisfies the retained session's entry rule (an
 * object naming its engine with non-empty result text), so it survives a window close like
 * any result.
 */
export function restoredResult(p: HistoryUse): {
  engine: string;
  result: string;
  from: string;
  to: string;
} {
  return { engine: p.engine, result: p.result, from: p.from_lang, to: p.to_lang };
}

/**
 * The pair the two selects take from a "Use": a side is assigned only when the stored value is
 * non-empty and the select actually lists it (the loaded language list recognizes bare
 * es / pt but does not offer them); null keeps whatever that select shows now, so a restore
 * never points a select at an option it does not have.
 */
export function restoredPair(
  p: HistoryUse,
  sourceOptions: string[],
  targetOptions: string[],
): { from: string | null; to: string | null } {
  const from = p.from_lang && sourceOptions.includes(p.from_lang) ? p.from_lang : null;
  const to = p.to_lang && targetOptions.includes(p.to_lang) ? p.to_lang : null;
  return { from, to };
}

/** Whether another page is worth asking for: shown rows below the matching total. */
export function canLoadMore(loaded: number, total: number): boolean {
  return loaded < total;
}
