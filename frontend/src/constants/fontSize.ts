// Text-size setting (issue #195). Percent of the pre-#195 size. The Go side owns the setting
// (internal/settings/font_size.go); this is its copy, needed to apply a size before any Go call
// returns. internal/settings/font_size_test.go pins the ladder and the default to these.
export const FONT_SIZE_STEPS = [90, 105, 120, 135, 150, 165] as const;
export const DEFAULT_FONT_SIZE = 120;

// The one place a stored, received or submitted value becomes a size: one of the six, else 120.
export function normalizeFontSize(v: unknown): number {
  return typeof v === 'number' && (FONT_SIZE_STEPS as readonly number[]).includes(v)
    ? v
    : DEFAULT_FONT_SIZE;
}

// One step up (dir 1) or down (dir -1) the ladder; stays put at the ends.
export function stepFontSize(current: number, dir: 1 | -1): number {
  const i = FONT_SIZE_STEPS.indexOf(normalizeFontSize(current) as (typeof FONT_SIZE_STEPS)[number]);
  return FONT_SIZE_STEPS[Math.min(FONT_SIZE_STEPS.length - 1, Math.max(0, i + dir))];
}
