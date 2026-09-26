// "Flipped target" feedback for the same-language guard (issue #44): when the backend finds that a
// result's detected source already IS the requested target (Mandarin text into a Chinese target),
// it re-runs that engine toward English and reports the target it actually used in the result's
// own `to`. The target select still shows what was requested, so each result says where it really
// went: "Target matches the source language — translated into English instead".
//
// Display only, and that is the point. This module is a pure function of three plain values and
// takes no setters, so it cannot assign toLang / fromLang, persist a language, teach the variant
// store, or (in the screenshot window, where a toLang change re-emits EventScreenshotRetranslate)
// trigger a second full run. The backend decides the flip; the frontend only reads it.
//
// nameOf (language code → display name) is injected by the caller (i18n's langName), like
// detectedLang.ts, so this stays free of the generated bindings and vitest can cover it directly.
// Both windows share it: the translate window reads its own result card, the screenshot window
// reads each translation card.

/**
 * Returns the display name of the target a result was actually translated into, when that differs
 * from the target it was requested with; null when there is nothing to say.
 *
 * @param requestedTo the target the request was sent with (the translate window captures it when
 *   it sends; the screenshot window reads the backend's ScreenshotResult.to, which keeps meaning
 *   the requested target). Empty means unknown, and an unknown request never claims a flip.
 * @param resultTo the result's own target (TranslateResult.to); empty when there is no result yet
 * @param nameOf language code → display name
 * @returns the flipped target's display name; null when the result went to the requested target,
 *   or when either side is unknown
 */
export function flippedTargetLabel(
  requestedTo: string,
  resultTo: string,
  nameOf: (code: string) => string,
): string | null {
  if (requestedTo === '' || resultTo === '' || resultTo === requestedTo) return null;
  return nameOf(resultTo);
}
