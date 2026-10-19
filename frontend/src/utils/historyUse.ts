// "Use" on a History entry (issue #60): put the entry back into the translate window, with its
// source, its result, its two languages and its engine. The History window sends the payload
// (EventHistoryUse); the translate window maps it onto its own state with useMapping and applies
// it. Nothing is translated: the entry is shown the way a retained session is (issue #81), and the
// "Translate as I type" and back-translation switches are not touched.
//
// Pure: no bindings, no storage, no DOM.

import type { PaneResult } from './resultPane.ts';
import type { HistoryRow } from './historyList.ts';

/** What travels between the windows. */
export type HistoryUsePayload = {
  text: string;
  result: string;
  from_lang: string;
  to_lang: string;
  engine: string;
};

export function toUsePayload(row: HistoryRow): HistoryUsePayload {
  return {
    text: row.text,
    result: row.result,
    from_lang: row.from_lang,
    to_lang: row.to_lang,
    engine: row.engine,
  };
}

/** What the translate window knows when the payload arrives. */
export type UseContext = {
  /** The language codes the selects hold (auto included). */
  languages: string[];
  /** The translate engines that can be selected. */
  engines: string[];
  /** The engine selected now. */
  active: string;
};

export type UseMapping = {
  input: string;
  /** The source language to select, or null to leave the select as it is. */
  from: string | null;
  /** The target language to select, or null to leave the select as it is. */
  to: string | null;
  /** The engine to select ('' when there is none). */
  engine: string;
  results: Record<string, PaneResult>;
  requestedTo: string;
};

export function useMapping(p: HistoryUsePayload, ctx: UseContext): UseMapping {
  const known = new Set(ctx.languages);
  const from = p.from_lang !== '' && known.has(p.from_lang) ? p.from_lang : null;
  // The target select never holds auto.
  const to = p.to_lang !== '' && p.to_lang !== 'auto' && known.has(p.to_lang) ? p.to_lang : null;
  // An engine that is gone (removed, disabled) cannot show the result: the one selected now does.
  const engine = ctx.engines.includes(p.engine) ? p.engine : ctx.active;
  const results: Record<string, PaneResult> = {};
  if (engine !== '' && p.result !== '') {
    results[engine] = { engine, result: p.result, from: p.from_lang, to: p.to_lang };
  }
  return { input: p.text, from, to, engine, results, requestedTo: p.to_lang };
}
