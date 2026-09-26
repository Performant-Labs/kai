import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Issue #76: English is the default interface language. index.svelte.ts holds runes state that
// this svelte-free vitest process cannot compile, so this is a source-level contract in the same
// style as keys.test.ts: the initial language before settings load must be en-US, otherwise
// every window flashes Chinese first.
const src = readFileSync(resolve(__dirname, 'index.svelte.ts'), 'utf8');

describe('default interface language', () => {
  it('initialises the reactive language to English', () => {
    expect(src).toMatch(/let currentLang = \$state<ResolvedLang>\(Lang\.ENUS\)/);
  });
  it('initialises the backwards-compatible store to English', () => {
    expect(src).toMatch(/writable<ResolvedLang>\(Lang\.ENUS\)/);
  });
});
