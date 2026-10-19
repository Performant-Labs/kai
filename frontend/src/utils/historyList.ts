// The pure half of the History screen (issue #60): the list state, the day grouping, the row
// time, the paging trigger and the keyboard selection. HistoryList.svelte owns the wiring (the
// bindings, the events, the DOM); nothing here touches storage, the runtime or the DOM, so the
// rules are tested as plain functions.

/** One saved translation, as the backend reports it (service.HistoryItem). */
export type HistoryRow = {
  id: number;
  text: string;
  result: string;
  from_lang: string;
  to_lang: string;
  /** The engine's name, resolved by the backend; '' when it is no longer configured. */
  engine: string;
  engine_id: number;
  from_ocr: boolean;
  /** Epoch milliseconds. */
  created_at: number;
};

/** What the list is narrowed to (service.HistoryFilter); '' / 0 mean "any". */
export type HistoryFilter = {
  keyword: string;
  from_lang: string;
  to_lang: string;
  engine_id: number;
};

/** How many rows a page asks for. */
export const PAGE_SIZE = 50;

export function emptyFilter(): HistoryFilter {
  return { keyword: '', from_lang: '', to_lang: '', engine_id: 0 };
}

/** Whether the filter narrows the list at all (a keyword of spaces does not). */
export function isFiltered(f: HistoryFilter): boolean {
  return f.keyword.trim() !== '' || f.from_lang !== '' || f.to_lang !== '' || f.engine_id !== 0;
}

/** The rows loaded so far, and how many exist under the filter. */
export type ListState = { items: HistoryRow[]; total: number; loaded: boolean };

export function emptyList(): ListState {
  return { items: [], total: 0, loaded: false };
}

type Page = { items: HistoryRow[]; total: number };

/** The first page: it replaces whatever was shown. */
export function replacePage(page: Page): ListState {
  return { items: [...page.items], total: page.total, loaded: true };
}

/** A later page: appended, a row already shown is kept once (a new entry shifts the offsets). */
export function appendPage(state: ListState, page: Page): ListState {
  const seen = new Set(state.items.map((r) => r.id));
  const fresh = page.items.filter((r) => !seen.has(r.id));
  // An empty page ends the list whatever the total says, so a stale total cannot loop the paging.
  const total = page.items.length === 0 ? state.items.length : page.total;
  return { items: [...state.items, ...fresh], total, loaded: true };
}

/** A deleted row leaves the list and the total, without reloading it. */
export function removeRow(state: ListState, id: number): ListState {
  if (!state.items.some((r) => r.id === id)) return state;
  return {
    items: state.items.filter((r) => r.id !== id),
    total: Math.max(0, state.total - 1),
    loaded: state.loaded,
  };
}

export function hasMore(state: ListState): boolean {
  return state.loaded && state.items.length < state.total;
}

/** Which empty state to word: none (rows, or still loading), nothing saved yet, or nothing matches. */
export function emptyKind(state: ListState, filter: HistoryFilter): 'none' | 'empty' | 'nomatch' {
  if (!state.loaded || state.items.length > 0) return 'none';
  return isFiltered(filter) ? 'nomatch' : 'empty';
}

/** How close to the bottom (px) the next page is asked for. */
const LOAD_MARGIN = 200;

/**
 * Whether the next page should be asked for: the list is near its bottom, or its rows do not even
 * fill the viewport, and no page is on its way and one exists.
 */
export function shouldLoadMore(
  g: { scrollTop: number; clientHeight: number; scrollHeight: number },
  s: { loading: boolean; hasMore: boolean },
): boolean {
  if (s.loading || !s.hasMore) return false;
  return g.scrollTop + g.clientHeight >= g.scrollHeight - LOAD_MARGIN;
}

export type DayGroup = {
  /** Unique within a list: the local date, YYYY-MM-DD. */
  key: string;
  kind: 'today' | 'yesterday' | 'date';
  /** Local midnight of the day. */
  date: Date;
  rows: HistoryRow[];
};

function startOfDay(ts: number): Date {
  const d = new Date(ts);
  return new Date(d.getFullYear(), d.getMonth(), d.getDate());
}

function dayKey(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** Groups newest-first rows by local day (the order is kept: a group is a run of the same day). */
export function groupByDay(rows: HistoryRow[], now: number): DayGroup[] {
  const today = startOfDay(now);
  const yesterday = new Date(today.getFullYear(), today.getMonth(), today.getDate() - 1);
  const groups: DayGroup[] = [];
  for (const r of rows) {
    const day = startOfDay(r.created_at);
    const key = dayKey(day);
    const last = groups[groups.length - 1];
    if (last && last.key === key) {
      last.rows.push(r);
      continue;
    }
    const kind = key === dayKey(today) ? 'today' : key === dayKey(yesterday) ? 'yesterday' : 'date';
    groups.push({ key, kind, date: day, rows: [r] });
  }
  return groups;
}

/** The label of a day group that is neither today nor yesterday. */
export function formatDayLabel(date: Date, now: number, locale?: string): string {
  const sameYear = date.getFullYear() === new Date(now).getFullYear();
  return new Intl.DateTimeFormat(locale, {
    month: 'short',
    day: 'numeric',
    ...(sameYear ? {} : { year: 'numeric' }),
  }).format(date);
}

/**
 * The time shown on a row: the clock for today and yesterday (their group header says the day),
 * the date for anything older, with the year only when it is not this year's.
 */
export function formatRowTime(ts: number, now: number, locale?: string): string {
  const today = startOfDay(now).getTime();
  const day = startOfDay(ts).getTime();
  const dayMs = 24 * 60 * 60 * 1000;
  // Yesterday is "one calendar day back", which is 23 or 25 hours across a clock change.
  const recent = day === today || day === startOfDay(today - dayMs / 2).getTime();
  if (recent) {
    return new Intl.DateTimeFormat(locale, {
      hour: '2-digit',
      minute: '2-digit',
      hourCycle: 'h23',
    }).format(ts);
  }
  return formatDayLabel(new Date(ts), now, locale);
}

/** The next selected index after an arrow key; -1 means nothing is selected. */
export function moveSelection(
  current: number,
  key: 'ArrowUp' | 'ArrowDown',
  length: number,
): number {
  if (length <= 0) return -1;
  if (current < 0) return key === 'ArrowDown' ? 0 : length - 1;
  const next = key === 'ArrowDown' ? current + 1 : current - 1;
  return Math.min(length - 1, Math.max(0, next));
}

const CLAMP_LINES = 3;
const CLAMP_CHARS = 240;

/** Whether a text is long enough to be clamped behind "show more". */
export function isLongText(text: string): boolean {
  return text.split('\n').length > CLAMP_LINES || text.length > CLAMP_CHARS;
}
