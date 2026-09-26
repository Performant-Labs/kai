import { existsSync, readdirSync, readFileSync, statSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { describe, expect, it } from 'vitest';

// Issue #144 (Tester, RED): both panes of the translate window show the text in one real
// <textarea> that is never swapped for a per-word display (SpanText is deleted), with the same
// text look and whitespace-pre-wrap on both sides, and undo/redo write the restored text into the
// existing source textarea with a computed caret (brief Decisions A, B, C, D; criteria 1-6).
// There is no Svelte render harness (#78), so this pins the markup and wiring contract from the
// component source, in the style of sourceUndo.test.ts: script comments and markup comments are
// blanked (offsets kept) so a comment never satisfies or breaks a rule. The pure caret rule is
// pinned in utils/sourceHistory.test.ts; layout, scroll and WebKit caret painting are hand-test
// items (criterion 9).

const componentPath = resolve(__dirname, 'TranslateWindow.svelte');
const src = readFileSync(componentPath, 'utf8');
const scriptEnd = src.indexOf('</script>');
const blank = (m: string) => m.replace(/[^\n]/g, ' ');
/** The script with JS comments blanked. */
const code = src.slice(0, scriptEnd).replace(/\/\*[\s\S]*?\*\/|\/\/[^\n]*/g, blank);
/** The markup with HTML comments blanked. */
const markup = src.slice(scriptEnd).replace(/<!--[\s\S]*?-->/g, blank);

/** The one class token set both textareas must carry (criterion 4). */
const TEXT_CLASSES = [
  'min-h-0',
  'flex-1',
  'resize-none',
  'bg-transparent',
  'p-4',
  'text-base',
  'leading-relaxed',
  'outline-none',
  'whitespace-pre-wrap',
];

/** From the `{` at or after `from`, the text up to and including its matching `}`. */
function braceBlock(text: string, from: number): string {
  const open = text.indexOf('{', from);
  expect(open, 'no opening brace').toBeGreaterThan(-1);
  let depth = 0;
  for (let i = open; i < text.length; i++) {
    if (text[i] === '{') depth++;
    else if (text[i] === '}' && --depth === 0) return text.slice(from, i + 1);
  }
  return text.slice(from);
}

/** The text of a named function in the script (`function name(` or `const name = (…) =>`). */
function fnBody(name: string): string {
  let start = code.search(new RegExp(`(?:async\\s+)?function\\s+${name}\\s*\\(`));
  if (start < 0) start = code.search(new RegExp(`const\\s+${name}\\s*=\\s*(?:async\\s*)?\\([^)]*\\)[^=]*=>`));
  expect(start, `${name} not found`).toBeGreaterThan(-1);
  const arrow = code.indexOf('=>', start);
  const paren = code.indexOf(')', start);
  const from = code.startsWith('const', start) && arrow > -1 ? arrow : paren;
  return code.slice(start, from + braceBlock(code, from).length);
}

/** Every opening tag `<name …>` in `html` with its offset; a `>` inside `{…}` does not end it. */
function openTags(html: string, name: string): { tag: string; at: number }[] {
  const out: { tag: string; at: number }[] = [];
  let at = html.indexOf(`<${name}`);
  while (at > -1) {
    let depth = 0;
    let end = at;
    for (; end < html.length; end++) {
      const c = html[end];
      if (c === '{') depth++;
      else if (c === '}') depth--;
      else if (c === '>' && depth === 0) break;
    }
    out.push({ tag: html.slice(at, end + 1), at });
    at = html.indexOf(`<${name}`, end);
  }
  return out;
}

/** The value of an `attr={…}` attribute in a tag (brace-matched), '' when absent. */
function attr(tag: string, name: string): string {
  const i = tag.search(new RegExp(`\\s${name}=\\{`));
  if (i < 0) return '';
  return braceBlock(tag, tag.indexOf('{', i)).slice(1, -1).trim();
}

/** The class="…" tokens of a tag, as a sorted list. */
function classTokens(tag: string): string[] {
  const m = tag.match(/\sclass="([^"]*)"/);
  return m ? m[1].split(/\s+/).filter(Boolean).sort() : [];
}

/** Net `{#…}` / `{/…}` block depth of a markup slice (`{:else}` does not change it). */
function blockDepth(html: string): number {
  return (html.match(/\{#/g)?.length ?? 0) - (html.match(/\{\//g)?.length ?? 0);
}

/** The source textareas (bound to sourceEl). */
const sourceTextareas = () => openTags(markup, 'textarea').filter((t) => t.tag.includes('bind:this={sourceEl}'));

/** The result branch of the paneState chain: `{:else if pane === 'result' …}` up to the next branch. */
function resultBranch(): { html: string; at: number } {
  const open = markup.search(/\{:else if pane === 'result'[^}]*\}/);
  expect(open, "result branch '{:else if pane === 'result'' not found").toBeGreaterThan(-1);
  const start = markup.indexOf('}', open) + 1;
  const next = markup.indexOf('{:else if pane ===', start);
  expect(next, 'no branch after the result branch').toBeGreaterThan(start);
  return { html: markup.slice(start, next), at: start };
}

/** Every non-test source file under frontend/src. */
function sourceFiles(dir: string): string[] {
  const out: string[] = [];
  for (const name of readdirSync(dir)) {
    if (name === 'node_modules') continue;
    const p = join(dir, name);
    if (statSync(p).isDirectory()) out.push(...sourceFiles(p));
    else if (/\.(ts|svelte|js)$/.test(name) && !/\.test\.ts$/.test(name)) out.push(p);
  }
  return out;
}

describe('no per-word display (criterion 1, Decision C)', () => {
  it('SpanText.svelte is deleted', () => {
    expect(existsSync(resolve(__dirname, 'SpanText.svelte'))).toBe(false);
  });

  it('no non-test source file under frontend/src mentions SpanText, comments included', () => {
    const hits = sourceFiles(resolve(__dirname, '..')).filter((f) => /spantext/i.test(readFileSync(f, 'utf8')));
    expect(hits.map((f) => f.slice(f.indexOf('frontend/src')))).toEqual([]);
  });
});

describe('the source pane is always one textarea (criterion 2, Decision A)', () => {
  it('there is exactly one textarea bound to sourceEl', () => {
    expect(sourceTextareas()).toHaveLength(1);
  });

  it('the source textarea is inside no {#if} / {:else} of its own', () => {
    const [{ at }] = sourceTextareas();
    const section = markup.lastIndexOf('<section', at);
    expect(section, 'source <section> not found').toBeGreaterThan(-1);
    // Every block opened inside the source section before the textarea is closed before it.
    expect(blockDepth(markup.slice(section, at))).toBe(0);
    expect(markup).not.toMatch(/\{#if\s+input\s*===\s*''\s*\|\|\s*editingSource\s*\}/);
  });

  it('editingSource and enterSourceEdit are gone from the code and the markup', () => {
    for (const name of ['editingSource', 'enterSourceEdit']) {
      expect(code, `script still names ${name}`).not.toMatch(new RegExp(`\\b${name}\\b`));
      expect(markup, `markup still names ${name}`).not.toMatch(new RegExp(`\\b${name}\\b`));
    }
  });

  it('the source textarea has no onfocus, and keeps value, oninput and onbeforeinput unchanged', () => {
    const [{ tag }] = sourceTextareas();
    expect(tag).not.toMatch(/\sonfocus=/);
    expect(attr(tag, 'value')).toBe('input');
    expect(attr(tag, 'oninput')).toBe('onSourceInput');
    expect(attr(tag, 'onbeforeinput')).toBe('onSourceBeforeInput');
    expect(attr(tag, 'onblur'), 'the typing run must still end on blur').not.toBe('');
  });

  it('the tick import is dropped with the two click-to-edit functions (no unused import)', () => {
    const m = code.match(/import\s*\{([^}]*)\}\s*from\s*'svelte'/);
    expect(m, "no import from 'svelte'").not.toBeNull();
    expect(m![1]).not.toMatch(/\btick\b/);
  });
});

describe('the result is always one real, editable textarea (criterion 3, Decision B)', () => {
  it('the result branch holds exactly one textarea and no onclick', () => {
    const { html } = resultBranch();
    expect(openTags(html, 'textarea')).toHaveLength(1);
    expect(html).not.toMatch(/\sonclick=/);
  });

  it('the result textarea is inside no {#if} of its own within the branch', () => {
    const { html } = resultBranch();
    const [{ at }] = openTags(html, 'textarea');
    expect(blockDepth(html.slice(0, at))).toBe(0);
  });

  it('keeps value={activeDisplay}, the per-engine onchange commit and the noResult placeholder', () => {
    const [{ tag }] = openTags(resultBranch().html, 'textarea');
    expect(attr(tag, 'value')).toBe('activeDisplay');
    expect(attr(tag, 'onchange')).toMatch(
      /^\(\s*ev\s*\)\s*=>\s*setEdited\(\s*activeEngine\s*,\s*ev\.currentTarget\.value\s*\)$/,
    );
    expect(attr(tag, 'placeholder')).toBe("t('translate.noResult')");
  });

  it('the result textarea has no onclick, no onblur and no bind:this', () => {
    const [{ tag }] = openTags(resultBranch().html, 'textarea');
    expect(tag).not.toMatch(/\sonclick=/);
    expect(tag).not.toMatch(/\sonblur=/);
    expect(tag).not.toMatch(/bind:this/);
  });

  it('editingResult, enterResultEdit and resultEl are gone from the code and the markup', () => {
    for (const name of ['editingResult', 'enterResultEdit', 'resultEl']) {
      expect(code, `script still names ${name}`).not.toMatch(new RegExp(`\\b${name}\\b`));
      expect(markup, `markup still names ${name}`).not.toMatch(new RegExp(`\\b${name}\\b`));
    }
  });
});

describe('same text look on both sides, breaks kept (criterion 4)', () => {
  it('the source textarea carries exactly the shared class set, whitespace-pre-wrap included', () => {
    const [{ tag }] = sourceTextareas();
    expect(classTokens(tag)).toEqual([...TEXT_CLASSES].sort());
  });

  it('the result textarea carries exactly the shared class set, whitespace-pre-wrap included', () => {
    const [{ tag }] = openTags(resultBranch().html, 'textarea');
    expect(classTokens(tag)).toEqual([...TEXT_CLASSES].sort());
  });
});

describe('the exact engine string reaches the result textarea (criterion 5)', () => {
  // Regression guard: holds on master and must keep holding.
  it('activeDisplay is edited ?? result ?? empty, with no trim / replace / split', () => {
    const line = code.split('\n').find((l) => /\bconst\s+activeDisplay\s*=/.test(l)) ?? '';
    expect(line).toMatch(
      /const\s+activeDisplay\s*=\s*\$derived\(\s*edited\.get\(\s*activeEngine\s*\)\s*\?\?\s*activeResult\?\.result\s*\?\?\s*''\s*\)/,
    );
    expect(line).not.toMatch(/\.trim\(|\.replace\(|\.split\(/);
  });
});

describe('undo/redo keep the element and place the caret (criterion 6, Decision D)', () => {
  it('showRestored saves scrollTop, writes the value, sets a collapsed caret, then restores scrollTop', () => {
    const b = fnBody('showRestored');
    const read = b.search(/\.scrollTop\b(?!\s*=[^=])/);
    const write = b.search(/\.value\s*=(?!=)/);
    const caret = b.search(/\.setSelectionRange\(/);
    const restore = b.search(/\.scrollTop\s*=(?!=)/);
    expect(read, 'scrollTop is not saved').toBeGreaterThan(-1);
    expect(write, 'the value is not written').toBeGreaterThan(-1);
    expect(caret, 'no setSelectionRange').toBeGreaterThan(-1);
    expect(restore, 'scrollTop is not restored').toBeGreaterThan(-1);
    expect(read).toBeLessThan(write);
    expect(write).toBeLessThan(caret);
    expect(caret).toBeLessThan(restore);
  });

  it('showRestored places the caret from caretAfterRestore(shown, restored), collapsed', () => {
    const imp = code.match(/import\s*\{([^}]*)\}\s*from\s*'\.\.\/utils\/sourceHistory\.ts'/)?.[1] ?? '';
    const m = imp.match(/\bcaretAfterRestore\b(?:\s+as\s+(\w+))?/);
    expect(m, 'caretAfterRestore is not imported from ../utils/sourceHistory.ts').not.toBeNull();
    const fn = m![1] ?? 'caretAfterRestore';
    const b = fnBody('showRestored');
    const params = b.match(/showRestored\s*\(\s*(\w+)\s*(?::[^,]+)?,\s*(\w+)/);
    expect(params, 'showRestored takes (shown, restored)').not.toBeNull();
    const [, shown, restored] = params!;
    expect(b).toMatch(new RegExp(`\\b${fn}\\(\\s*${shown}\\s*,\\s*${restored}\\s*\\)`));
    expect(b).toMatch(new RegExp(`\\.value\\s*=\\s*${restored}\\b`));
    const range = b.match(/\.setSelectionRange\(\s*([^,()]+?)\s*,\s*([^,()]+?)\s*\)/);
    expect(range, 'setSelectionRange is not a collapsed (c, c) call').not.toBeNull();
    expect(range![1]).toBe(range![2]);
  });

  it('showRestored writes no input, never focuses, and does nothing without sourceEl', () => {
    const b = fnBody('showRestored');
    expect(b).not.toMatch(/(?<![.\w])input\s*=(?!=)/);
    expect(b).not.toMatch(/\.focus\(/);
    expect(b).toMatch(/\bsourceEl\b/);
    expect(b, 'no early return when the textarea is missing').toMatch(
      /if\s*\(\s*!\s*\w+\s*\)\s*return\b|if\s*\(\s*\w+\s*={2,3}\s*null\s*\)\s*return\b/,
    );
  });

  for (const name of ['applyUndo', 'applyRedo']) {
    it(`${name} calls showRestored(input, step.text) before it assigns input`, () => {
      const b = fnBody(name);
      const show = b.search(/\bshowRestored\(\s*input\s*,\s*\w+\.text\s*\)/);
      const assign = b.search(/(?<![.\w])input\s*=(?!=)/);
      expect(show, `${name} does not call showRestored(input, step.text)`).toBeGreaterThan(-1);
      expect(assign, `${name} does not assign input`).toBeGreaterThan(-1);
      expect(show).toBeLessThan(assign);
    });
  }
});
