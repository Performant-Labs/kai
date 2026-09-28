import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Issue #165 (Tester, RED):
// - the source/result pane title-area headers share a fixed height (u-pane-header), so they
//   match regardless of which controls each one happens to render.
// - the pin and auto-clipboard buttons get an instant, visible tooltip (u-tooltip/data-tooltip),
//   not just the native `title` (long hover delay in the Wails webview) — see app.css's
//   .u-tooltip rule.
// No Svelte render harness exists (#78), so this reads the component source, like
// sourceUndo.test.ts / swapWindow.test.ts.

const src = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');
const css = readFileSync(resolve(__dirname, '../app.css'), 'utf8');

/** Every opening tag `<name …>` in `text`; a `>` inside `{…}` (an `=>`) does not end the tag. */
function openTags(text: string, name: string): string[] {
  const tags: string[] = [];
  let at = text.indexOf(`<${name}`);
  while (at > -1) {
    let depth = 0;
    let end = at;
    for (; end < text.length; end++) {
      const c = text[end];
      if (c === '{') depth++;
      else if (c === '}') depth--;
      else if (c === '>' && depth === 0) break;
    }
    tags.push(text.slice(at, end + 1));
    at = text.indexOf(`<${name}`, end);
  }
  return tags;
}

describe('source/result pane headers share a fixed height (issue #165)', () => {
  it('exactly two header divs carry u-pane-header (the source and result panes)', () => {
    const headers = openTags(src, 'div').filter((t) => t.includes('u-pane-header'));
    expect(headers).toHaveLength(2);
    for (const h of headers) expect(h).toMatch(/u-border-b\s+u-pane-header/);
  });

  it('app.css defines .u-pane-header with a fixed height', () => {
    const m = css.match(/\.u-pane-header\s*\{([^}]*)\}/);
    expect(m, '.u-pane-header rule not found in app.css').not.toBeNull();
    expect(m![1]).toMatch(/height:\s*[\d.]+(rem|px)/);
  });
});

describe('pin and auto-clipboard buttons get an instant custom tooltip (issue #165)', () => {
  it('app.css defines .u-tooltip driven by a data-tooltip attribute', () => {
    expect(css).toMatch(/\.u-tooltip::after\s*\{[^}]*content:\s*attr\(data-tooltip\)/);
  });

  it('the pin button carries u-tooltip and a data-tooltip matching its title', () => {
    const btn = openTags(src, 'button').find((t) => t.includes('onclick={togglePin}'));
    expect(btn, 'pin button not found').toBeDefined();
    expect(btn).toMatch(/\bu-tooltip\b/);
    expect(btn).toMatch(
      /data-tooltip=\{pinned \? t\('translate\.unpin'\) : t\('translate\.pin'\)\}/,
    );
  });

  it('the auto-clipboard button carries u-tooltip and a data-tooltip matching its title', () => {
    const btn = openTags(src, 'button').find((t) =>
      t.includes('onclick={() => applyAutoClipboard(!autoClipboard)}'),
    );
    expect(btn, 'auto-clipboard button not found').toBeDefined();
    expect(btn).toMatch(/\bu-tooltip\b/);
    expect(btn).toMatch(/data-tooltip=\{t\('translate\.autoClipboard'\)\}/);
  });
});
