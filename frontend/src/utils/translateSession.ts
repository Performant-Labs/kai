// The retained translate session (issue #81).
//
// Closing the translate window used to wipe the source text and the results, so reopening it (or
// restarting the app) showed an empty window. The window now keeps the last translation: the
// source text, each engine's result and the request marker. This module is the pure half of that:
// the shape of what is kept, the empty value, and the validation of a value read back from
// storage. Storage itself is stores/persisted.ts (the same localStorage-backed store that keeps
// the pin and the last-used engine); TranslateWindow.svelte owns the wiring (seed at init, write
// back from one $effect).
//
// Kept: input, the per-engine results, requestedTo (the target the results were requested with;
// nothing reads it back since issue #80, it stays part of the stored shape) and requested (whether
// a request was made, which is what tells an idle pane from a failed one, see paneState). Not kept:
// loading (always false after a restore: nothing is in flight in a fresh process) and manual
// result edits (design §3: edits are discarded). The two languages are not part of it either;
// they already persist through the settings file (persistLangs / loadDefaults).
//
// Whatever storage hands back is untrusted: a previous version, a hand edit or a truncated write
// can leave anything there (persisted() only parses the JSON, it validates nothing). So
// restoreSession never trusts its argument and falls back to the empty session instead of
// throwing or half-applying. Pure function of a plain value, like swapLangs.ts / detectedLang.ts:
// no bindings import (the result type is the structural PaneResult, imported as a type only), no
// storage access, no DOM.

import type { PaneResult } from './resultPane.ts';

/** The translation on screen, as kept between window closes and app restarts. */
export type TranslateSession = {
  /** The source text. */
  input: string;
  /** Per-engine results, keyed by engine name. */
  results: Record<string, PaneResult>;
  /** The target the results were requested with; '' when unknown. */
  requestedTo: string;
  /** Whether a translation was requested (false on an idle window, so the pane stays blank). */
  requested: boolean;
};

/** The idle session: no text, no results, nothing requested. A fresh object on every call. */
export function emptySession(): TranslateSession {
  return { input: '', results: {}, requestedTo: '', requested: false };
}

// A JSON object: not null, not an array.
function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

// A results entry worth keeping: an object that names its engine and carries translated text, or
// that records the user's cancel of that engine (issue #109): a cancel is a plain fact about the
// request the user made, so it is restored as such and reads "Cancelled", never as a failure. An
// engine failure arrives as an entry too (Error set, no result text); it is not a translation and
// restoring it would show "Translation failed" for a request from a previous run.
function isResultEntry(v: unknown): v is PaneResult {
  return (
    isRecord(v) &&
    typeof v.engine === 'string' &&
    ((typeof v.result === 'string' && v.result !== '') || v.cancelled === true)
  );
}

/**
 * Validates a value read back from storage and returns the session to restore.
 *
 * - Anything that is not a well-formed session (not an object, or a field of the wrong type,
 *   including a missing one) yields the empty session.
 * - `results` keeps only the entries that are objects with a string `engine` and either a
 *   non-empty `result` or `cancelled: true` (issue #109); the rest are dropped, engine failure
 *   payloads included.
 * - `requested` is forced false when no result survives: a request that never produced anything
 *   reads as an idle window after a restore, not as a failed one.
 *
 * @param raw whatever storage returned (already JSON-parsed, or the store's initial value)
 */
export function restoreSession(raw: unknown): TranslateSession {
  if (!isRecord(raw)) return emptySession();
  const { input, results, requestedTo, requested } = raw;
  if (
    typeof input !== 'string' ||
    !isRecord(results) ||
    typeof requestedTo !== 'string' ||
    typeof requested !== 'boolean'
  ) {
    return emptySession();
  }
  // fromEntries defines own properties, so a stored key such as "__proto__" stays plain data.
  const kept: Record<string, PaneResult> = Object.fromEntries(
    Object.entries(results).filter((e): e is [string, PaneResult] => isResultEntry(e[1])),
  );
  return {
    input,
    results: kept,
    requestedTo,
    requested: requested && Object.keys(kept).length > 0,
  };
}
