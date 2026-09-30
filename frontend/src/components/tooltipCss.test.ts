import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// The tooltip's geometry is CSS, and jsdom does not compute the cascade for pseudo-elements
// (getComputedStyle(el, '::after') is not implemented), so this parses app.css and applies the
// cascade rule by hand: among rules that match the same element/state, the higher specificity
// wins, and for equal specificity the LATER one wins.

const css = readFileSync(resolve(__dirname, '../app.css'), 'utf8');

interface Rule {
  selector: string;
  body: string;
  order: number;
  spec: [number, number, number];
}

function specificity(sel: string): [number, number, number] {
  const pseudoEls = (sel.match(/::[\w-]+/g) ?? []).length;
  const rest = sel.replace(/::[\w-]+/g, '');
  const classes = (rest.match(/\.[\w-]+|:[\w-]+(\([^)]*\))?|\[[^\]]*\]/g) ?? []).length;
  const types = (rest.match(/(^|[\s>+~])[a-zA-Z][\w-]*/g) ?? []).length;
  return [(rest.match(/#[\w-]+/g) ?? []).length, classes, types + pseudoEls];
}

function rules(): Rule[] {
  const out: Rule[] = [];
  const re = /([^{}]+)\{([^{}]*)\}/g;
  let m: RegExpExecArray | null;
  let order = 0;
  while ((m = re.exec(css))) {
    const body = m[2];
    for (const sel of m[1].replace(/\/\*[\s\S]*?\*\//g, '').split(',')) {
      const selector = sel.trim();
      if (selector) out.push({ selector, body, order: order++, spec: specificity(selector) });
    }
  }
  return out;
}

const cmp = (a: Rule, b: Rule) =>
  a.spec[0] - b.spec[0] || a.spec[1] - b.spec[1] || a.spec[2] - b.spec[2] || a.order - b.order;

/** Winning `transform` on a `.u-tooltip.u-tooltip--start` element in the given state. */
function winningTransform(state: ':hover' | ':focus-visible'): string | undefined {
  const matching = rules().filter(
    (r) =>
      /^\.u-tooltip(--start)?(:hover|:focus-visible)?::after$/.test(r.selector) &&
      (!/:(hover|focus-visible)/.test(r.selector) || r.selector.includes(state)) &&
      /transform:/.test(r.body),
  );
  const winner = matching.sort(cmp).at(-1);
  return winner?.body.match(/transform:\s*([^;]+);/)?.[1].trim();
}

describe('tooltip CSS (duplicate/clipped tooltip fix)', () => {
  for (const state of [':hover', ':focus-visible'] as const) {
    it(`the start variant is not shifted left by translateX(-50%) on ${state}`, () => {
      const t = winningTransform(state);
      expect(t, 'no transform rule found').toBeDefined();
      expect(t).not.toMatch(/translateX\(-50%\)/);
      expect(t).toMatch(/translateY\(0\)/);
    });
  }

  it('the centred variant still centres on hover (translateX(-50%) is applied to plain .u-tooltip)', () => {
    const plain = rules()
      .filter((r) => /^\.u-tooltip(:hover|:focus-visible)::after$/.test(r.selector))
      .map((r) => r.body)
      .join(' ');
    expect(plain).toMatch(/translateX\(-50%\)\s*translateY\(0\)/);
  });

  it('opens below, wraps at a max-width, and uses the text-scale font size', () => {
    const base = css.match(/\.u-tooltip::after\s*\{([^}]*)\}/)![1];
    expect(base).toMatch(/top:\s*calc\(100% \+ 6px\)/);
    expect(base).toContain('font-size: max(0.625rem, calc(0.6875rem * var(--kai-text-scale)))');
    const start = css.match(/\.u-tooltip--start::after\s*\{([^}]*)\}/)![1];
    expect(start).toMatch(/left:\s*0/);
    expect(start).toMatch(/white-space:\s*normal/);
    expect(start).toMatch(/max-width:\s*min\(24rem, 90vw\)/);
  });
});
