import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Issue #165, item 6 (Tester, RED; re-investigated after a wrong first guess at #lang-sel — the
// principal clarified "System" is the target-side ENGINE dropdown, not a language picker:
// `t('engine.apple')` displays the on-device engine as "System").
//
// Root cause (verified against the actual built CSS, `pnpm --dir frontend run build:dev` then
// grepping dist/assets/service-*.css for `@layer`): `.u-select` (app.css, `@layer components`)
// reserves `padding-right: 2.25rem` for its custom dropdown-arrow icon
// (`background-position: right .75rem center`). Tailwind v4 emits `@layer properties, theme,
// base, components, utilities;` — utilities always cascade-wins over components regardless of
// source order — so a plain Tailwind `px-3` utility (`padding-inline: .75rem`, i.e. both sides)
// on the same element collapsed that reserved padding to .75rem, letting option text (including
// "System") sit under the arrow: the "out of proportion" dropdown. Fix: `pl-3` (left only)
// instead of `px-3`, leaving `.u-select`'s own right padding uncontested.

const src = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');
const css = readFileSync(resolve(__dirname, '../app.css'), 'utf8');

/** Every opening `<select …>` tag; a `>` inside `{…}` (an `=>`) does not end the tag. */
function selectTags(): string[] {
  const tags: string[] = [];
  let at = src.indexOf('<select');
  while (at > -1) {
    let depth = 0;
    let end = at;
    for (; end < src.length; end++) {
      const c = src[end];
      if (c === '{') depth++;
      else if (c === '}') depth--;
      else if (c === '>' && depth === 0) break;
    }
    tags.push(src.slice(at, end + 1));
    at = src.indexOf('<select', end);
  }
  return tags;
}

function engineSelectTag(): string {
  const tag = selectTags().find((t) => t.includes('u-engine-select'));
  expect(tag, 'the u-engine-select <select> was not found').toBeDefined();
  return tag!;
}

describe('the result-pane engine dropdown ("System" + whichever engines are enabled) is not squeezed under its arrow (issue #165)', () => {
  it('no longer carries a px-3 utility that would collapse u-select’s reserved arrow padding', () => {
    expect(engineSelectTag()).not.toMatch(/\bpx-3\b/);
  });

  it('supplies its own left padding (pl-3), leaving the right side to u-select', () => {
    expect(engineSelectTag()).toMatch(/\bpl-3\b/);
  });

  it('app.css still reserves padding-right: 2.25rem on .u-select for the arrow icon', () => {
    const m = css.match(/\.u-select\s*\{([^}]*)\}/);
    expect(m, '.u-select rule not found in app.css').not.toBeNull();
    expect(m![1]).toMatch(/padding-right:\s*2\.25rem/);
  });
});
