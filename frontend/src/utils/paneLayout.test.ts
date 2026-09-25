import { describe, expect, it } from 'vitest';
import { MAX_RATIO, MIN_RATIO, clampRatio, ratioFromPoint } from './paneLayout.ts';

describe('clampRatio', () => {
  it('keeps an in-range ratio unchanged', () => {
    expect(clampRatio(0.5)).toBe(0.5);
    expect(clampRatio(MIN_RATIO)).toBe(MIN_RATIO);
    expect(clampRatio(MAX_RATIO)).toBe(MAX_RATIO);
  });

  it('clamps below the minimum', () => {
    expect(clampRatio(0.1)).toBe(MIN_RATIO);
    expect(clampRatio(-1)).toBe(MIN_RATIO);
  });

  it('clamps above the maximum', () => {
    expect(clampRatio(0.9)).toBe(MAX_RATIO);
    expect(clampRatio(2)).toBe(MAX_RATIO);
  });

  it('handles NaN by returning the midpoint', () => {
    expect(clampRatio(NaN)).toBe(0.5);
  });
});

describe('ratioFromPoint', () => {
  const left = 100;
  const width = 800;

  it('maps a point inside the row to its ratio', () => {
    expect(ratioFromPoint(500, left, width)).toBe(0.5);
    expect(ratioFromPoint(300, left, width)).toBe(0.25);
    expect(ratioFromPoint(700, left, width)).toBe(0.75);
  });

  it('clamps points outside the row', () => {
    expect(ratioFromPoint(0, left, width)).toBe(MIN_RATIO);
    expect(ratioFromPoint(2000, left, width)).toBe(MAX_RATIO);
  });

  it('never divides by zero on a zero-width row', () => {
    expect(ratioFromPoint(50, left, 0)).toBe(0.5);
  });
});
