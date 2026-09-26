import { existsSync, readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { en } from './en-US.ts';
import { zh } from './zh-CN.ts';

// issue #58 (Tester, RED): the Dict type contract lives in a neutral keys module and the en/zh
// catalogs must carry identical key trees. vitest does not type-check, so parity is asserted at
// runtime here (the tsc annotation is only a second net in `make check`).

const dir = __dirname;
const read = (f: string) => readFileSync(resolve(dir, f), 'utf8');

// Flatten a nested catalog into dotted leaf paths (a leaf is any non-plain-object value).
function flatten(obj: unknown, prefix = ''): string[] {
  if (obj === null || typeof obj !== 'object' || Array.isArray(obj)) return [prefix];
  return Object.entries(obj as Record<string, unknown>).flatMap(([k, v]) =>
    flatten(v, prefix ? `${prefix}.${k}` : k),
  );
}

describe('neutral Dict type source', () => {
  it('provides frontend/src/i18n/keys.ts exporting the Dict type', () => {
    expect(existsSync(resolve(dir, 'keys.ts'))).toBe(true);
    expect(read('keys.ts')).toMatch(/export\s+(type|interface)\s+Dict\b/);
  });

  it('keys.ts is a pure contract: no value imports of a catalog', () => {
    expect(existsSync(resolve(dir, 'keys.ts'))).toBe(true);
    expect(read('keys.ts')).not.toMatch(/from\s+['"]\.\/(en-US|zh-CN)/);
  });

  it('zh-CN.ts no longer exports the Dict type', () => {
    expect(read('zh-CN.ts')).not.toMatch(/export\s+type\s+Dict\b/);
  });

  it('en-US.ts and zh-CN.ts both take Dict from ./keys, not from each other', () => {
    expect(read('en-US.ts')).toMatch(/from\s+['"]\.\/keys(\.ts)?['"]/);
    expect(read('en-US.ts')).not.toMatch(/from\s+['"]\.\/zh-CN/);
    expect(read('zh-CN.ts')).toMatch(/from\s+['"]\.\/keys(\.ts)?['"]/);
  });

  it('index.svelte.ts imports Dict from ./keys', () => {
    const src = read('index.svelte.ts');
    expect(src).toMatch(/from\s+['"]\.\/keys(\.ts)?['"]/);
    expect(src).not.toMatch(/type\s+Dict[^;]*from\s+['"]\.\/zh-CN/);
  });
});

describe('en-US / zh-CN key parity', () => {
  const enKeys = flatten(en).sort();
  const zhKeys = flatten(zh).sort();

  it('catalogs are non-empty (guards a vacuous pass)', () => {
    expect(enKeys.length).toBeGreaterThan(50);
  });

  it('every zh-CN key exists in en-US', () => {
    expect(zhKeys.filter((k) => !enKeys.includes(k))).toEqual([]);
  });

  it('every en-US key exists in zh-CN', () => {
    expect(enKeys.filter((k) => !zhKeys.includes(k))).toEqual([]);
  });
});
