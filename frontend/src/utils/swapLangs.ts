// Dialect-aware language swap for the translate window (issue #13).
//
// The swap button used to trade the two selects and, for an auto source, substitute a hardcoded
// Chinese for it, ignoring what the engine had actually detected. What is exchanged now is the
// EFFECTIVE source language: the pinned source, or, when the source is auto, the language the
// active engine detected. Since issue #53 that detection already carries the user's learned
// variant (a detected es arrives as es-MX once the user has picked es-MX), so the swap lands on
// the variant the user works in without any lookup of its own.
//
// Auto does not survive a swap. The new source is the old target, which is always a concrete
// language (the target select has no auto option), and the new target is the effective source.
// When the source is auto and nothing usable was detected there is nothing to exchange: this
// returns null and the caller disables the button instead of guessing a language (the old
// hardcoded zh fallback could never be right). A detection is usable only if the target select
// can hold it. Bare es / pt are recognized but not selectable (epic #51); a bare detection lands on
// the first selectable variant of its family, the backend's SelectableOr rule (es -> es-MX), so
// a Spanish text detected before any variant was learned still swaps (hand test of #82).
//
// Pure function of plain values, like detectedLang.ts / flippedTarget.ts / targetCapability.ts:
// the auto code and the "can the target select hold this code" predicate are injected by the
// caller, so this stays free of the generated bindings and vitest can cover it directly. It only
// computes the new pair. It never persists, never teaches the variant store and never translates;
// the caller assigns the pair, persists it and re-runs the translation.
//
// Not related to ScreenshotWindow's local swapLangs(): that window has no single source text and
// no per-window detection, keeps its own swap, and is out of scope for #13.

/** A source/target language pair as plain language codes. */
export interface LangPair {
  from: string;
  to: string;
}

/** The plain values swapLanguages decides from (all injected by the caller). */
export interface SwapLanguagesArgs {
  /** The current source-language selection (autoCode or a concrete language code). */
  from: string;
  /**
   * The current target-language selection. Expected to be a concrete language, because the target
   * select offers no auto option; that is what keeps auto from surviving a swap.
   */
  to: string;
  /**
   * The source language the active engine recognized: empty when there is no result yet, the auto
   * code when the engine reported no detection. Only read while `from` is auto. Since issue #53
   * the backend qualifies it with the learned variant preference, so it is used as it arrives.
   */
  detectedFrom: string;
  /** The language code for auto (TRANSLATE_LANG.Auto). */
  autoCode: string;
  /**
   * Whether the target select can hold a language code: offered by the loaded language list and
   * not capability-disabled (issue #52). Decides whether a detection can become the new target.
   * Never consulted for a pinned source, which is always exchanged.
   */
  isSelectable: (code: string) => boolean;
  /**
   * The target select's option codes in display order. When a detection is a bare family base the
   * select does not offer (es, pt), the first selectable option of that family (code starting
   * with `<base>-`) is used instead: the same rule as the backend's model.Language.SelectableOr
   * (es -> es-MX, pt -> pt-BR). Optional; without it a bare base is unusable.
   */
  options?: string[];
}

/**
 * Returns the pair the swap should apply: the old target becomes the source and the effective
 * source becomes the target. A pinned pair always exchanges (an identical pair returns itself).
 *
 * @returns the exchanged pair; null when the source is auto and there is no usable detection
 *   (empty, still the auto code, or not selectable as a target), i.e. nothing to exchange
 */
export function swapLanguages({
  from,
  to,
  detectedFrom,
  autoCode,
  isSelectable,
  options = [],
}: SwapLanguagesArgs): LangPair | null {
  let source = from;
  if (from === autoCode) {
    // No result yet (''), or the engine reported no detection and the backend fell back to the
    // request value, auto (issue #53): nothing was detected. A detection the target select cannot
    // hold (a language no enabled engine can target, or a bare base with no usable variant) is
    // unusable too.
    if (detectedFrom === '' || detectedFrom === autoCode) return null;
    // A bare family base (es, pt) is recognized but not a target option: land on its first
    // selectable variant, so a Spanish text detected without a learned preference still swaps.
    const usable = isSelectable(detectedFrom)
      ? detectedFrom
      : options.find((c) => c.startsWith(`${detectedFrom}-`) && isSelectable(c));
    if (!usable) return null;
    source = usable;
  }
  return { from: to, to: source };
}
