// Teaches the backend variant-preference store from an explicit language selection (issue #53).
//
// Both translate windows call the Wails `Learn` binding from a language select's change handler.
// The call is injected (not imported here) so this module stays free of the generated bindings and
// vitest can run it directly, the same seam detectedLang.ts uses. A failing binding must never
// break the select itself, so the error goes to the caller's own logger instead of being thrown.

/**
 * @param lang the language value the user just picked (any select value; the backend ignores
 *             bases and languages without dialects, so callers pass every explicit pick through)
 * @param learn the backend binding that records the selection
 * @param onError called with the failure when `learn` rejects; this function never throws
 */
export async function learnFromSelection(
  lang: string,
  learn: (lang: string) => Promise<void>,
  onError: (e: unknown) => void,
): Promise<void> {
  try {
    await learn(lang);
  } catch (e) {
    onError(e);
  }
}
