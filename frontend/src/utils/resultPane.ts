// issue #9: result-pane pure logic (the DOM-free part extracted from TranslateWindow.svelte so
// tests can verify it independently).
//
// The resolution rules are not reinvented here: activeEngineFor directly consumes #8's
// resolvePrimaryEngine (last-used ?? primary(default_engine) ?? first-enabled) and adds a single
// defensive fallback on top — '' → the first enabled translate engine — guaranteeing the result
// pane's engine select never dangles.
//
// The status-dot states are a pure function of the fan-out's real output (design §4) plus one
// flag, whether a translation was requested at all (issue #81):
//   cancelled = results[engine].cancelled is set (issue #109): the user cancelled that engine. It is
//             not a failure and wins over everything else, a partial result included;
//   done    = results[engine].result is a non-empty string (done as soon as a result arrives, regardless of loading);
//   pending = loading and that engine has no (non-empty) result yet;
//   failed  = a request was made, !loading and that engine has no (non-empty) result: it reported a
//             failure payload (error set) or never reported at all;
//   idle    = nothing was requested (a fresh or cleared window): there is nothing to report, so it
//             must not read as failed.
// paneState below applies the same rules to the whole result pane.
//
// This module never touches localStorage / wails runtime / DOM: TranslateWindow holds lastUsed's
// persisted store and hands the resolution inputs to activeEngineFor; the result-pane template
// consumes statusDots and resetEdits.
//
// failureMessage (issues #42, #96) is the one renderer of a failure payload: the translate window's
// failed pane, its failed-dot tooltip and the screenshot window's TranslateCard all read it, so the
// copy table exists once. It reads the payload's `error_kind`, the generated binding's own field
// name, with no mapping layer; the values come from internal/model (ErrorKind*).

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
  /** issues #42, #96: the failure payload's sanitized error and its category (`error_kind`, an ErrorKind); absent on success. */
  error?: string;
  error_kind?: string;
  /**
   * issue #109: the user cancelled this engine. Beside `error`, not a kind of it: a cancelled entry
   * has no error, and result may hold a partial translation.
   */
  cancelled?: boolean;
  [key: string]: unknown;
};

/**
 * The failure categories the backend sends as `error_kind`. The source of truth is internal/model
 * (ErrorKind*); a value this union does not list is treated as `engine`, so a newer backend never
 * breaks an older window.
 */
export type ErrorKind =
  | 'not_configured'
  | 'auth'
  | 'quota'
  | 'rate_limit'
  | 'unavailable'
  | 'network'
  | 'pair'
  | 'too_long'
  | 'engine';

/**
 * The parts of a result failureMessage reads. Structural and narrow on purpose: both PaneResult and
 * the generated TranslateResult (an interface, so with no index signature) satisfy it without a cast.
 */
export type FailureSource = { engine?: string; error?: string; error_kind?: string };

/** What a failed result renders as: a headline, a muted detail line (may be empty) and an optional action button. */
export type FailureMessage = {
  headline: string;
  detail: string;
  /** 'settings' = offer the Settings button (a credential problem the user fixes there); null = no button. */
  action: 'settings' | null;
};

/**
 * User-facing copy for a failure payload (issues #42, #96): the category picks a localized
 * headline, the payload's (already sanitized) error is the detail, and only the two credential
 * categories (not_configured, auth) carry the Settings action.
 *
 * `error_kind` picks the key: every category has its own copy taking the engine's display name as
 * `{engine}`; `pair` reads differently for Apple, whose fix is a language download in System Settings
 * (failedPair, no {engine}), than for any other engine (failedUnsupported); an unknown or absent
 * kind falls back to the generic `translate.failed`. A null result, or one with no `error`,
 * is the generic headline with no detail: an engine that reported nothing.
 * Pure function: t (the i18n lookup) is injected by the caller, and the caller owns the side
 * effect of the action (opening Settings).
 */
export function failureMessage(
  result: FailureSource | null | undefined,
  t: (key: string, params?: Record<string, string | number>) => string,
  engineLabel: string,
): FailureMessage {
  if (!result?.error) return { headline: t('translate.failed'), detail: '', action: null };
  const p = { engine: engineLabel };
  let headline: string;
  // Typed as the union so a typo in a case label below fails the compile; an unknown wire value
  // still reaches `default`.
  const kind = result.error_kind as ErrorKind | undefined;
  switch (kind) {
    case 'not_configured':
      headline = t('translate.failedNotConfigured', p);
      break;
    case 'auth':
      headline = t('translate.failedAuth', p);
      break;
    case 'quota':
      headline = t('translate.failedQuota', p);
      break;
    case 'rate_limit':
      headline = t('translate.failedRateLimit', p);
      break;
    case 'unavailable':
      headline = t('translate.failedUnavailable', p);
      break;
    case 'network':
      headline = t('translate.failedNetwork', p);
      break;
    case 'too_long':
      headline = t('translate.failedTooLong', p);
      break;
    case 'pair':
      headline =
        result.engine === 'apple' ? t('translate.failedPair') : t('translate.failedUnsupported', p);
      break;
    default:
      headline = t('translate.failed');
  }
  const action = kind === 'not_configured' || kind === 'auth' ? 'settings' : null;
  return { headline, detail: result.error, action };
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

/**
 * A single engine's dot state (the state table of design §4, plus idle from issue #81 and
 * cancelled from issue #109).
 */
export type DotState = 'pending' | 'done' | 'failed' | 'idle' | 'cancelled';

/**
 * A single engine's dot state: cancelled (the user cancelled it, issue #109) > done (non-empty
 * result) > pending (loading and no result) > failed (a request was made, !loading and no result:
 * it failed or never reported) > idle (nothing was requested, so there is nothing to have failed).
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
  if (results[engine]?.cancelled) return 'cancelled';
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

/** What the result pane renders (issue #81; cancelled from issue #109). */
export type PaneState = 'no-engine' | 'loading' | 'result' | 'idle' | 'failed' | 'cancelled';

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
 *   result    — the active engine has a non-empty result (a cancelled engine's partial result
 *               included: cancelling keeps the parts already translated on screen);
 *   cancelled — the user cancelled the active engine and it has no result (issue #109); never
 *               failed;
 *   failed    — a request was made and the active engine has no usable result (absent, empty, or
 *               an error payload with no result);
 *   idle      — nothing was requested: the pane stays blank.
 *
 * The two tests on results are deliberately not the same, exactly as the template's chain always
 * was: loading looks at entry presence (an error payload that arrives while loading ends the
 * wait, so it falls through to failed), result looks at a non-empty `.result`. Manual edits are
 * not an input: an edit exists only inside a rendered result, so an edit without a result
 * never selects `result` (there would be nothing to render).
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
  if (results[engine]?.cancelled) return 'cancelled';
  return requested ? 'failed' : 'idle';
}

/**
 * Whether every engine the backend started has reported for the current request: a result, a
 * failure payload or a cancel (an entry in results either way). This is the settle rule of the
 * result pane: the active engine keeps showing its loading placeholder until it has reported
 * itself or the request has settled, so a fast engine answering first no longer makes a slower
 * active engine read as failed. It reads the started list the backend returned, not the enabled
 * engines, so an enabled engine that was never started cannot hold a request open (issue #109).
 * True when there is nothing to wait for.
 */
export function allReported(started: string[], results: Record<string, PaneResult>): boolean {
  for (const e of started) {
    if (!results[e]) return false;
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
