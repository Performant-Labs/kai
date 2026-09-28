import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Issue #165 (Tester, RED): the General tab's interface-language <select id="lang-sel"> was
// stretched with `w-full max-w-[240px]`, an outlier not used by any other <select> in the app
// (the language-bar and engine selects just use u-select/u-lang-select/u-engine-select and size
// to content) — the concrete, in-code candidate for the issue's "System dropdown looks out of
// proportion" (there is no control literally labeled "System"; the theme picker's "System"
// option is a segmented button, not a <select>). This pins the select to the same sizing
// convention as the rest of the app instead of the one-off stretch.

const src = readFileSync(resolve(__dirname, '../../components/settings/GeneralTab.svelte'), 'utf8');

function selectTag(): string {
  const m = src.match(/<select[\s\S]*?id="lang-sel"[\s\S]*?>/);
  expect(m, 'lang-sel <select> not found').not.toBeNull();
  return m![0];
}

describe('the General tab interface-language select matches the app-wide select sizing (issue #165)', () => {
  it('uses u-lang-select like every other language <select> in the app, not a one-off stretch', () => {
    expect(selectTag()).toMatch(/class="[^"]*\bu-lang-select\b[^"]*"/);
  });

  it('no longer force-stretches to a fixed max width', () => {
    expect(selectTag()).not.toMatch(/w-full/);
    expect(selectTag()).not.toMatch(/max-w-\[240px\]/);
  });
});
