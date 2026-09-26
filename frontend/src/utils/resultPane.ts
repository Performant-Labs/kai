// issue #9: result-pane pure logic (the DOM-free part extracted from TranslateWindow.svelte so
// tests can verify it independently).
//
// The resolution rules are not reinvented here: activeEngineFor directly consumes #8's
// resolvePrimaryEngine (last-used ?? primary(default_engine) ?? first-enabled) and adds a single
// defensive fallback on top — '' → the first enabled translate engine — guaranteeing the result
// pane's engine select never dangles.
//
// The status-dot states are a pure function of the fan-out's real output (design §4; backend
// unchanged) plus one flag, whether a translation was requested at all (issue #81):
//   done    = results[engine].result is a non-empty string (done as soon as a result arrives, regardless of loading);
//   pending = loading and that engine has no (non-empty) result yet;
//   failed  = a request was made, !loading and that engine has no (non-empty) result — the backend
//             sends no event whatsoever for a failed engine, it is simply absent from results;
//             failed is the derived signal of "absent + loading finished";
//   idle    = nothing was requested (a fresh or cleared window): there is nothing to report, so it
//             must not read as failed.
// paneState below applies the same rules to the whole result pane.
//
// This module never touches localStorage / wails runtime / DOM: TranslateWindow holds lastUsed's
// persisted store and hands the resolution inputs to activeEngineFor; the result-pane template
// consumes statusDots and resetEdits.

import { resolvePrimaryEngine } from './resolvePrimaryEngine.ts';

/** Result-pane engine entry (GetAllEngines shape): id order defines "first". */
export type PaneEngine = {
  id: number;
  value: string;
  name: string;
  kind: string;
  enabled: boolean;
  supported: boolean;
  builtin?: boolean;
};

/** Single-engine fan-out result entry (TranslateResult shape; result is the translation; empty string / absent means no result). */
export type PaneResult = {
  engine?: string;
  result?: string;
  phonetic?: string;
  /** issue #42: the failure payload's raw error and category (pair/network/auth/engine); absent on success. */
  error?: string;
  errorKind?: string;
  [key: string]: unknown;
};

/**
 * User-facing copy for a failure payload (issue #42): kind → localized key (pair/network/auth get
 * actionable copy; everything else falls back to translate.failed), with the raw error detail
 * appended after " — ".
 * Pure function: t (i18n lookup) is injected by the caller.
 */
export function failureMessage(
  result: PaneResult | null | undefined,
  t: (key: string) => string,
): string {
  const generic = t('translate.failed');
  if (!result?.error) return generic;
  const key =
    result.errorKind === 'pair'
      ? 'translate.failedPair'
      : result.errorKind === 'network'
        ? 'translate.failedNetwork'
        : result.errorKind === 'auth'
          ? 'translate.failedAuth'
          : 'translate.failed';
  return `${t(key)} — ${result.error}`;
}

/**
 * The "enabled translate engine" predicate: kind=translate and enabled and platform-supported.
 * Exported so the target-language capability gating (issue #52, utils/targetCapability.ts)
 * shares this one definition instead of restating it.
 */
export function isEnabledTranslate(e: Pick<PaneEngine, 'kind' | 'enabled' | 'supported'>): boolean {
  return e.kind === 'translate' && e.enabled && e.supported;
}

/**
 * Resolves the name of the engine the result pane is currently bound to (= #8's resolvedPrimary
 * derivation + a defensive fallback).
 *
 * @param lastUsedKey   localStorage key (e.g. `kai:translate:lastEngine`); resolvePrimaryEngine
 *                      reads it itself (real localStorage; corrupt/empty values fall to the next layer).
 * @param defaultEngine settings' default_engine (primary, the middle layer of the resolution chain).
 * @param engines       engine list in the GetAllEngines shape (id order).
 * @returns the resolved engine name; when resolvePrimaryEngine returns '' but the list still has
 *          enabled translate engines (a defense against future rule changes), falls back to the
 *          first enabled translate engine so the select never dangles; returns '' when the list
 *          has no usable engine at all (the pane shows the existing "no active engine" empty state).
 */
export function activeEngineFor(
  lastUsedKey: string,
  defaultEngine: string,
  engines: PaneEngine[],
): string {
  const resolved = resolvePrimaryEngine(lastUsedKey, defaultEngine, engines);
  if (resolved) return resolved;
  for (const e of engines) {
    if (isEnabledTranslate(e)) return e.value;
  }
  return '';
}

/** A single engine's dot state (the state table of design §4, plus idle from issue #81). */
export type DotState = 'pending' | 'done' | 'failed' | 'idle';

/**
 * A single engine's dot state: done (non-empty result) > pending (loading and no result) > failed
 * (a request was made, !loading and no result — a failed engine is absent from results) > idle
 * (nothing was requested, so there is nothing to have failed).
 *
 * @param requested whether a translation was requested and not cleared since (issue #81); passed
 *   explicitly by every caller, so an idle window can never be drawn as failed by omission.
 */
export function statusDot(
  engine: string,
  results: Record<string, PaneResult>,
  loading: boolean,
  requested: boolean,
): DotState {
  if (results[engine]?.result) return 'done';
  if (loading) return 'pending';
  return requested ? 'failed' : 'idle';
}

/**
 * One dot per enabled translate engine (activeEngines order, i.e. id order);
 * ocr / disabled engines get no dot (design §4).
 */
export function statusDots(
  engines: PaneEngine[],
  results: Record<string, PaneResult>,
  loading: boolean,
  requested: boolean,
): Record<string, DotState> {
  const dots: Record<string, DotState> = {};
  for (const e of engines) {
    if (!isEnabledTranslate(e)) continue;
    dots[e.value] = statusDot(e.value, results, loading, requested);
  }
  return dots;
}

/** What the result pane renders (issue #81). */
export type PaneState = 'no-engine' | 'loading' | 'result' | 'idle' | 'failed';

/** The plain values paneState decides from. */
export interface PaneStateArgs {
  /** Whether there is at least one enabled translate engine to show (the pane's engine list is non-empty). */
  hasEngines: boolean;
  /** The engine the pane is bound to (activeEngineFor). */
  engine: string;
  /** The fan-out's results so far, keyed by engine name. */
  results: Record<string, PaneResult>;
  loading: boolean;
  /**
   * Whether a translation was requested and not cleared since. A fresh window, a cleared window
   * and a restored session that holds no result are all "nothing requested": the pane is idle,
   * not failed.
   */
  requested: boolean;
}

/**
 * Which state the result pane is in, one of:
 *   no-engine — no enabled translate engine at all (wins over everything);
 *   loading   — the request is in flight and the active engine has no entry in results yet;
 *   result    — the active engine has a non-empty result;
 *   failed    — a request was made and the active engine has no usable result (absent, empty, or
 *               an error payload with no result);
 *   idle      — nothing was requested: the pane stays blank.
 *
 * The two tests on results are deliberately not the same, exactly as the template's chain always
 * was: loading looks at entry presence (an error payload that arrives while loading ends the
 * wait, so it falls through to failed), result looks at a non-empty `.result`. Manual edits are
 * not an input: an edit exists only inside a rendered result card, so an edit without a result
 * never selects `result` (the card would have nothing to render).
 */
export function paneState({
  hasEngines,
  engine,
  results,
  loading,
  requested,
}: PaneStateArgs): PaneState {
  if (!hasEngines) return 'no-engine';
  if (loading && !results[engine]) return 'loading';
  if (results[engine]?.result) return 'result';
  return requested ? 'failed' : 'idle';
}

/**
 * Whether every enabled translate engine has reported for the current request: a result or a
 * failure payload (an entry in results either way). This is the settle rule of the result pane:
 * the active engine keeps showing its loading placeholder until it has reported itself or the
 * request has settled, so a fast engine answering first no longer makes a slower active engine
 * read as failed. True when there is nothing to wait for.
 */
export function allReported(engines: PaneEngine[], results: Record<string, PaneResult>): boolean {
  for (const e of engines) {
    if (isEnabledTranslate(e) && !results[e.value]) return false;
  }
  return true;
}

/**
 * Whether a result-pane dropdown option renders disabled. The options come from GetEngines, whose
 * items carry no `enabled` field, so the answer is read from the GetAllEngines list; an engine
 * missing from that list (not loaded yet, or unknown) is not disabled, so the dropdown never
 * shows every engine as disabled.
 */
export function isEngineOptionDisabled(value: string, all: PaneEngine[]): boolean {
  const found = all.find((e) => e.value === value);
  return found ? !found.enabled : false;
}

/** The dropdown option label: the engine name, plus " (disabled)" (localized word) when disabled. */
export function engineOptionLabel(name: string, disabled: boolean, disabledWord: string): string {
  return disabled ? `${name} (${disabledWord})` : name;
}

/**
 * The 15 s fallback predicate (a design §4 extension): whether the fan-out is still in progress —
 * i.e. some enabled translate engine is "still pending" (loading and no (non-empty) result yet).
 * The old logic only cleared loading on "zero results"; relaxed to "any pending", sibling engines
 * arriving each flip loading to false themselves, and the moment loading clears with a failed
 * engine absent, its dot flips from pending to failed (case 1 is immediate; case 2 converges at
 * this predicate's 15 s fallback point).
 * With `loading === false` no engine can be pending -> always false (a fallback would be meaningless).
 */
export function anyPending(
  engines: PaneEngine[],
  results: Record<string, PaneResult>,
  loading: boolean,
): boolean {
  if (!loading) return false;
  for (const e of engines) {
    if (isEnabledTranslate(e) && !results[e.value]?.result) return true;
  }
  return false;
}

/**
 * Edit reset on engine switch (or re-selecting the same engine) (design §3 "manual edits reset"):
 * returns a new Map, discarding previousEngine's manual edit (no per-engine edit memory);
 * nextEngine's displayed text reverts to its stored result (absent from the map, so the display
 * layer's ?? takes over).
 * The input Map is not mutated.
 */
export function resetEdits(
  edited: Map<string, string>,
  previousEngine: string,
  nextEngine: string,
  _results: Record<string, PaneResult>,
): Map<string, string> {
  const next = new Map(edited);
  next.delete(previousEngine);
  return next;
}
