// issue #9: result-pane pure logic (the DOM-free part extracted from TranslateWindow.svelte so
// tests can verify it independently).
//
// The resolution rules are not reinvented here: activeEngineFor directly consumes #8's
// resolvePrimaryEngine (last-used ?? primary(default_engine) ?? first-enabled) and adds a single
// defensive fallback on top — '' → the first enabled translate engine — guaranteeing the result
// pane's engine select never dangles.
//
// The three status-dot states are a pure function of the fan-out's real output (design §4;
// backend unchanged):
//   done    = results[engine].result is a non-empty string (done as soon as a result arrives, regardless of loading);
//   pending = loading and that engine has no (non-empty) result yet;
//   failed  = !loading and that engine has no (non-empty) result — the backend sends no event
//             whatsoever for a failed engine, it is simply absent from results; failed is the
//             derived signal of "absent + loading finished".
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

/** A single engine's dot state (the state table of design §4). */
export type DotState = 'pending' | 'done' | 'failed';

/**
 * A single engine's dot state: done (non-empty result) > pending (loading and no result) > failed
 * (!loading and no result — a failed engine is absent from results).
 */
export function statusDot(
  engine: string,
  results: Record<string, PaneResult>,
  loading: boolean,
): DotState {
  if (results[engine]?.result) return 'done';
  if (loading) return 'pending';
  return 'failed';
}

/**
 * One dot per enabled translate engine (activeEngines order, i.e. id order);
 * ocr / disabled engines get no dot (design §4).
 */
export function statusDots(
  engines: PaneEngine[],
  results: Record<string, PaneResult>,
  loading: boolean,
): Record<string, DotState> {
  const dots: Record<string, DotState> = {};
  for (const e of engines) {
    if (!isEnabledTranslate(e)) continue;
    dots[e.value] = statusDot(e.value, results, loading);
  }
  return dots;
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
