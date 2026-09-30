import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// The toast is CSS (jsdom does not lay anything out), so this reads the `.u-toast` rule from
// app.css. It was a fully rounded pill sized for one-word toasts ("Copied"); the long permission
// messages (double Cmd+C, missing Accessibility) wrapped inside it in half the window's width and
// turned into an ellipse with the text spilling out of its corners.

const css = readFileSync(resolve(__dirname, '../app.css'), 'utf8');
const body = css.match(/\.u-toast\s*\{([^{}]*)\}/)?.[1] ?? '';

describe('.u-toast shape', () => {
  it('has a rule', () => {
    expect(body).not.toBe('');
  });

  it('is not a fully rounded pill, which becomes an ellipse when the text wraps', () => {
    const radius = body.match(/border-radius:\s*([^;]+);/)?.[1].trim() ?? '';
    expect(radius).not.toMatch(/9999|999px|50%/);
    expect(radius).not.toBe('');
  });

  it('sizes to its text but never wider than the window, so a long message wraps at a sane width', () => {
    expect(body).toMatch(/width:\s*max-content;/);
    expect(body).toMatch(/max-width:\s*min\([^;]*(100vw|90vw|calc\(100vw)[^;]*\);/);
  });

  it('wraps its text inside the box and keeps it readable', () => {
    expect(body).toMatch(/white-space:\s*normal;/);
    expect(body).toMatch(/text-align:\s*(left|center);/);
    expect(body).toMatch(/line-height:\s*[\d.]+;/);
  });

  it('stays centred at the bottom of the window', () => {
    expect(body).toMatch(/position:\s*fixed;/);
    expect(body).toMatch(/left:\s*50%;/);
    expect(body).toMatch(/translateX\(-50%\)/);
  });
});
