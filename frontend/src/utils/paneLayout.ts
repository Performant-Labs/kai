// Divider math for the two-pane layout (issue #10): the "drag point → left-pane width ratio"
// mapping is collapsed into pure functions so vitest can unit-test them (real dragging can't be
// automated inside WKWebView, see issue #28).
// Components only bind events; all the clamping logic lives here.

/** Minimum left-pane ratio (no matter how narrow the window, the source pane never drops below 1/4 of the content area). */
export const MIN_RATIO = 0.25;
/** Maximum left-pane ratio (the results pane is likewise guaranteed at least 1/4). */
export const MAX_RATIO = 0.75;

/**
 * Clamps any ratio to [min, max]. NaN (malformed coordinates in drag events) falls back to the
 * midpoint 0.5, guaranteeing the divider always sits at a legal position instead of vanishing.
 */
export function clampRatio(
  ratio: number,
  min: number = MIN_RATIO,
  max: number = MAX_RATIO,
): number {
  if (Number.isNaN(ratio)) return 0.5;
  return Math.min(max, Math.max(min, ratio));
}

/**
 * Converts "the pointer's horizontal position within the layout row" into a left-pane ratio.
 * x outside the row bounds clamps to the ends; width of 0 (layout not yet measured) falls back
 * to the midpoint.
 */
export function ratioFromPoint(
  x: number,
  left: number,
  width: number,
  min: number = MIN_RATIO,
  max: number = MAX_RATIO,
): number {
  if (width <= 0) return 0.5;
  return clampRatio((x - left) / width, min, max);
}
