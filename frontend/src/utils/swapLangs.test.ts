import { describe, expect, it } from 'vitest';
import { swapLanguages } from './swapLangs.ts';

// Issue #13: dialect-aware swap. Pure helper, predicates injected (no bindings import).
const AUTO = 'auto';
// The target select's options in display order (dialects follow their family, like the backend).
const OPTIONS = ['en', 'es-MX', 'es-ES', 'pt-BR', 'zh'];
const SELECTABLE = new Set(OPTIONS);
const isSelectable = (c: string) => SELECTABLE.has(c);

const run = (from: string, to: string, detectedFrom: string) =>
  swapLanguages({ from, to, detectedFrom, autoCode: AUTO, isSelectable, options: OPTIONS });

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

  // Hand test of #82 (2026-09-26): after a restart the variant preference is gone, so Spanish comes
  // back detected as bare es, which is not a target option, and the swap button stayed grey. A
  // bare base now lands on the first selectable variant of its family, the same rule as the
  // backend's model.Language.SelectableOr (es -> es-MX, pt -> pt-BR).
  it('auto + bare es detection: target becomes the first selectable Spanish variant', () => {
    expect(run(AUTO, 'en', 'es')).toEqual({ from: 'en', to: 'es-MX' });
  });

  it('auto + bare pt detection: target becomes the first selectable Portuguese variant', () => {
    expect(run(AUTO, 'en', 'pt')).toEqual({ from: 'en', to: 'pt-BR' });
  });

  it('a family variant that is not selectable (capability-disabled) is skipped', () => {
    const r = swapLanguages({
      from: AUTO,
      to: 'en',
      detectedFrom: 'es',
      autoCode: AUTO,
      isSelectable: (c) => c !== 'es-MX' && SELECTABLE.has(c),
      options: OPTIONS,
    });
    expect(r).toEqual({ from: 'en', to: 'es-ES' });
  });

  it('auto + bare base with no selectable variant -> null', () => {
    expect(run(AUTO, 'en', 'de')).toBeNull();
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
