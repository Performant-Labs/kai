// "Auto-detect" feedback for the source-language label (issue #11): when the source language is
// auto and the engine has returned a detected language, the language bar no longer shows a bare
// "detect language" option but an "English (detected)"-style label — so the user can see what the
// system recognized. As soon as the user manually pins a concrete language, the feedback disappears
// (returns null; the caller falls back to langName(fromLang)).
//
// nameOf (language code → display name) is injected by the caller (i18n's langName), and the auto
// language code is likewise passed in by the caller (TRANSLATE_LANG.Auto) — this module stays a pure
// function with no dependency on the generated bindings, so vitest can cover it directly.

/**
 * Returns the display label for the auto option in the source-language dropdown.
 *
 * @param fromLang the current source-language selection (autoCode or a concrete language code)
 * @param autoCode the language code for auto (TRANSLATE_LANG.Auto)
 * @param detectedFrom the source language actually recognized in the engine result (empty string when there is no result; the auto code when the engine reported no detection). Since issue #53 the backend qualifies it with the user's learned variant preference (a detected es arrives as es-MX once the user has picked es-MX), so it is shown as it arrives and keeps the suffix.
 * @param nameOf language code → display name
 * @param suffix detected suffix (i18n, e.g. ' (detected)' in en-US, its full-width-punctuation counterpart in zh-CN; may include a leading space)
 * @returns the override label; null when no override is needed (language pinned, or nothing detected)
 */
export function detectedSourceLabel(
  fromLang: string,
  autoCode: string,
  detectedFrom: string,
  nameOf: (code: string) => string,
  suffix: string,
): string | null {
  if (fromLang !== autoCode) return null;
  // Nothing detected: no result yet (''), or the engine reported no detection and the backend
  // fell back to the request value, auto (issue #53) — "(detected)" would claim otherwise.
  if (detectedFrom === '' || detectedFrom === autoCode) return null;
  return nameOf(detectedFrom) + suffix;
}
