import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Issue #95: the translate window's result pane drops the multi-engine card chrome (the
// u-result-card frame and the engine badge) and renders flat, like the source pane. There is no
// Svelte render harness (#78), so this pins the markup contract from the component source; the
// visual alignment is a manual check.

const src = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');
const css = readFileSync(resolve(__dirname, '../app.css'), 'utf8');

// The result pane body: the wrapper <div> that opens right before the paneState chain, through the
// end of the result <section>.
function body(): string {
  const start = src.indexOf("{#if pane === 'no-engine'}");
  expect(start, "paneState chain '{#if pane === 'no-engine'}'").toBeGreaterThan(-1);
  const end = src.indexOf('</section>', start);
  expect(end).toBeGreaterThan(start);
  return src.slice(start, end);
}

// The opening tag of the scrolling wrapper directly above the chain.
function wrapperTag(): string {
  const idx = src.indexOf("{#if pane === 'no-engine'}");
  const before = src.slice(0, idx);
  const tags = [...before.matchAll(/<div\b[^>]*>/g)];
  return tags[tags.length - 1]?.[0] ?? '';
}

function branch(name: string): string {
  const b = body();
  const re = new RegExp(`\\{:else if pane === '${name}'[^}]*\\}([\\s\\S]*?)(?=\\{:else if pane ===|\\{/if\\}\\s*$)`);
  return b.match(re)?.[1] ?? '';
}

function openTag(html: string, marker: string): string {
  const i = html.indexOf(marker);
  if (i < 0) return '';
  const start = html.lastIndexOf('<', i);
  return html.slice(start, html.indexOf('>', i) + 1);
}

describe('flat result pane (#95)', () => {
  it('has no u-result-card wrapper anywhere and the css class is removed', () => {
    expect(src).not.toMatch(/u-result-card/);
    expect(css).not.toMatch(/u-result-card/);
  });

  it('has no engine badge in the result pane body', () => {
    const b = body();
    expect(b).not.toMatch(/rounded-full\s+bg-\[var\(--app-accent\)\]/);
    expect(b).not.toMatch(/engineName\(/);
  });

  it('the pane body wrapper does not pad the text (no double padding) and is a flex column', () => {
    const tag = wrapperTag();
    expect(tag).toMatch(/overflow-y-auto/);
    expect(tag).not.toMatch(/(^|[\s"'])p-4([\s"']|$)/);
    expect(tag).toMatch(/(^|[\s"'])flex([\s"']|$)/); // display:flex itself; \bflex\b also matches flex-1 / flex-col
    expect(tag).toMatch(/(^|[\s"'])flex-col([\s"']|$)/);
    expect(tag).toMatch(/(^|[\s"'])flex-1([\s"']|$)/); // fills the section: the textarea and the centred states size from it
  });

  it('result display div and edit textarea use the source pane text classes', () => {
    const r = branch('result');
    expect(r.length).toBeGreaterThan(0);
    const ta = openTag(r, '<textarea');
    const div = openTag(r, 'onclick={enterResultEdit}');
    for (const tag of [ta, div]) {
      expect(tag).toMatch(/[\s"']p-4[\s"']/);
      expect(tag).toMatch(/\btext-base\b/);
      expect(tag).toMatch(/\bleading-relaxed\b/);
      expect(tag).not.toMatch(/min-h-\[120px\]/);
    }
    expect(div).toMatch(/\bcursor-text\b/);
    expect(div).not.toMatch(/overflow-y-auto/); // the wrapper is the one scroller
    expect(ta).toMatch(/\bmin-h-0\b/);
    expect(ta).toMatch(/\bflex-1\b/);
    expect(ta).toMatch(/\bresize-none\b/);
    expect(ta).toMatch(/\bbg-transparent\b/);
    expect(ta).toMatch(/\boutline-none\b/);
  });

  it('phonetic and the identity notice are small muted lines above the text', () => {
    const r = branch('result');
    const phon = r.indexOf('activeResult.phonetic');
    const ident = r.indexOf('data-testid="identity-result"');
    const text = r.indexOf('<textarea');
    expect(phon).toBeGreaterThan(-1);
    expect(ident).toBeGreaterThan(-1);
    expect(phon).toBeLessThan(ident); // phonetic first (approved wireframe)
    expect(ident).toBeLessThan(text);
    expect(openTag(r, 'data-testid="identity-result"')).toMatch(/u-muted/);
    expect(openTag(r, 'data-testid="identity-result"')).toMatch(/text-xs/);
    expect(openTag(r, '{activeResult.phonetic}')).toMatch(/u-muted/);
    expect(openTag(r, '{activeResult.phonetic}')).toMatch(/text-xs/);
    // Notes align with the text's p-4 inset (px-4); the first note adds the top inset.
    expect(openTag(r, 'data-testid="identity-result"')).toMatch(/[\s"']px-4[\s"']/);
    expect(openTag(r, 'data-testid="identity-result"')).toMatch(/first:pt-4/);
    expect(openTag(r, '{activeResult.phonetic}')).toMatch(/[\s"']px-4[\s"']/);
    expect(openTag(r, '{activeResult.phonetic}')).toMatch(/first:pt-4/);
  });

  it('result display div fills the pane so the whole area is the click-to-edit target', () => {
    const r = branch('result');
    expect(openTag(r, 'onclick={enterResultEdit}')).toMatch(/[\s"']flex-1[\s"']/);
  });

  it('loading is flat: no card, keeps loading copy, dots and bar, and pads itself', () => {
    const l = branch('loading');
    expect(l.length).toBeGreaterThan(0);
    expect(l).toMatch(/t\('common\.loading'\)/);
    expect(l).toMatch(/class="([^"]*\s)?kai-dots(\s[^"]*)?"/); // a class, not the word in the branch comment
    expect(l).toMatch(/class="([^"]*\s)?kai-loading-bar(\s[^"]*)?"/);
    expect(l).toMatch(/[\s"']p-4[\s"']/);
  });

  it('keeps behaviour hooks and the failed / no-engine copy', () => {
    const b = body();
    expect(b).toMatch(/enterResultEdit/);
    expect(b).toMatch(/setEdited\(activeEngine/);
    expect(b).toMatch(/SpanText/);
    // #96: the failed copy is rendered through failureMessage (resultPane.ts), whose generic
    // fallback is still t('translate.failed'); the markup reads failure.headline, not the key.
    expect(b).toMatch(/failure\.headline/);
    expect(b).toMatch(/t\('translate\.noActiveEngine'\)/);
    expect(b).toMatch(/var\(--app-danger\)/);
  });
});
