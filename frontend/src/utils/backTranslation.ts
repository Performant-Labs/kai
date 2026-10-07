// The frontend half of the back-translation (issue #56).
//
// With the toggle on, the displayed result is translated back into the source language and shown
// under the result, so the user can check it in their own language. The backend
// (translate.Service.BackTranslate) does the translating: a cancellable single-engine request that
// writes no history and never raises an error. What lives here is the small pure state between
// its answers and the window, so vitest covers it without the bindings (like sourceCorrection.ts):
// when one is worth asking for, which pair it uses, and when what is on screen no longer describes
// the displayed text.
//
// Nothing here persists or teaches anything.

/** The backend's answer (model.BackTranslateResult). */
export interface BackAnswer {
  status: 'ok' | 'cancelled' | 'skipped' | 'failed';
  result: string;
  engine: string;
  request_id: string;
}

/** A back-translation, bound to the exact displayed text and engine it was made for. */
export interface BoundBack {
  text: string;
  engine: string;
  /** The language it is in (the source language of the request), for the label. */
  lang: string;
  forResult: string;
  forEngine: string;
}

/** The pair of a back-translation: from the result's language to the source language. */
export interface BackPair {
  from: string;
  to: string;
}

/**
 * Whether a back-translation is worth asking for: the toggle is on, a finished, non-empty result is
 * on screen, and it is not a failure, a cancelled result or an identity result (the same language on
 * both sides, #80, where there is nothing to check).
 */
export function shouldBackTranslate(i: {
  enabled: boolean;
  displayed: string;
  isResult: boolean;
  failed?: boolean;
  cancelled?: boolean;
  identity?: boolean;
}): boolean {
  if (!i.enabled || !i.isResult) return false;
  if (i.failed || i.cancelled || i.identity) return false;
  return i.displayed.trim() !== '';
}

/**
 * The pair for a result: `resultTo` is the language the result is in, `resultFrom` the language it was
 * translated from (the pin, or `auto`). On auto the detected language stands in. Null when there is
 * no usable source language or both sides are the same language.
 */
export function backPair(r: {
  resultFrom: string;
  resultTo: string;
  detectedFrom: string;
}): BackPair | null {
  const source = r.resultFrom !== '' && r.resultFrom !== 'auto' ? r.resultFrom : r.detectedFrom;
  if (source === '' || source === 'auto' || r.resultTo === '' || r.resultTo === 'auto') return null;
  if (source === r.resultTo) return null;
  return { from: r.resultTo, to: source };
}

/** The request BackTranslate takes (model.TranslateRequest): the displayed text and the swapped pair. */
export function buildBackRequest(r: {
  text: string;
  pair: BackPair;
  engine: string;
  requestId: string;
}) {
  return {
    text: r.text,
    from: r.pair.from,
    to: r.pair.to,
    engine: r.engine,
    request_id: r.requestId,
  };
}

/** The back-translation to keep from an answer, bound to what it was asked for; null for anything but a non-empty ok. */
export function acceptBack(
  a: BackAnswer | null | undefined,
  displayed: string,
  engine: string,
  lang: string,
): BoundBack | null {
  if (!a || a.status !== 'ok') return null;
  const text = a.result.trim();
  if (text === '') return null;
  return { text, engine: a.engine || engine, lang, forResult: displayed, forEngine: engine };
}

/** The back-translation to show for this displayed text and engine, or null: it belongs to one text, one engine. */
export function backShownFor(
  b: BoundBack | null,
  displayed: string,
  engine: string,
): BoundBack | null {
  return b !== null && b.forResult === displayed && b.forEngine === engine ? b : null;
}

/** Whether a request is due: nothing is shown yet for this displayed text and engine. */
export function needsBack(b: BoundBack | null, displayed: string, engine: string): boolean {
  return backShownFor(b, displayed, engine) === null;
}
