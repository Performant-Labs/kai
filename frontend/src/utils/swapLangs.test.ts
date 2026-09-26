import { describe, expect, it } from 'vitest';
import { swapLanguages } from './swapLangs.ts';

// Issue #13: dialect-aware swap. Pure helper, predicates injected (no bindings import).
const AUTO = 'auto';
const SELECTABLE = new Set(['en', 'zh', 'es-MX', 'es-ES', 'pt-BR']);
const isSelectable = (c: string) => SELECTABLE.has(c);

const run = (from: string, to: string, detectedFrom: string) =>
  swapLanguages({ from, to, detectedFrom, autoCode: AUTO, isSelectable });

describe('swapLanguages', () => {
  it('exchanges two pinned languages', () => {
    expect(run('en', 'zh', '')).toEqual({ from: 'zh', to: 'en' });
  });

  it('ignores detection when the source is pinned', () => {
    expect(run('en', 'zh', 'es-MX')).toEqual({ from: 'zh', to: 'en' });
  });

  it('auto + qualified detection: target becomes the detected variant, source the old target', () => {
    expect(run(AUTO, 'en', 'es-MX')).toEqual({ from: 'en', to: 'es-MX' });
  });

  it('auto + bare es detection (recognized but not selectable) -> null', () => {
    expect(run(AUTO, 'en', 'es')).toBeNull();
  });

  it('auto + no detection (empty) -> null', () => {
    expect(run(AUTO, 'en', '')).toBeNull();
  });

  it('auto + detection that is still the request value auto -> null', () => {
    expect(run(AUTO, 'en', AUTO)).toBeNull();
  });

  it('auto + detected language outside the selectable set -> null', () => {
    expect(run(AUTO, 'en', 'xx')).toBeNull();
  });

  it('never returns auto as the new target', () => {
    for (const d of ['', AUTO, 'es', 'es-MX', 'en', 'xx']) {
      const r = run(AUTO, 'zh', d);
      if (r) expect(r.to).not.toBe(AUTO);
    }
    expect(run('en', 'zh', '')?.to).not.toBe(AUTO);
  });

  it('never leaves auto as the new source', () => {
    expect(run(AUTO, 'en', 'es-MX')?.from).not.toBe(AUTO);
  });

  it('identical pinned pair still returns the (identity) exchange', () => {
    expect(run('es-MX', 'es-MX', '')).toEqual({ from: 'es-MX', to: 'es-MX' });
  });

  it('a pinned source is never gated by isSelectable', () => {
    expect(run('xx', 'en', '')).toEqual({ from: 'en', to: 'xx' });
  });

  it('auto is excluded even when the predicate would accept anything', () => {
    const permissive = (f: string, d: string) =>
      swapLanguages({ from: f, to: 'en', detectedFrom: d, autoCode: AUTO, isSelectable: () => true });
    expect(permissive(AUTO, '')).toBeNull();
    expect(permissive(AUTO, AUTO)).toBeNull();
  });
});
