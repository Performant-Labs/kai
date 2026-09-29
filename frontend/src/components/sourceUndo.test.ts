import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { en } from '../i18n/en-US.ts';
import { zh } from '../i18n/zh-CN.ts';

// Issue #118 (Tester, RED): the wiring of the source pane's undo/redo in TranslateWindow.svelte.
// The pure history rules are pinned in utils/sourceHistory.test.ts. No Svelte render harness
// exists (#78), so this reads the component source, like swapWindow.test.ts and
// resultPaneSettle.test.ts.
//
// Per handoff-A: nothing here depends on the name of the component's history $state or on import
// aliases of record/undo/redo/breakTyping/emptyHistory (the local names are read off the import).
// The two decision helpers are named functions, sliced by name: shortcutAction(e) and
// changeKindOf(e). Button placement is pinned by source order only, not by footer classes.

const src = readFileSync(resolve(__dirname, 'TranslateWindow.svelte'), 'utf8');
const scriptEnd = src.indexOf('</script>');
const script = src.slice(0, scriptEnd);
const markup = src.slice(scriptEnd);

/** Comments blanked to spaces (offsets preserved), so a comment never satisfies or breaks a rule. */
const code = script.replace(/\/\*[\s\S]*?\*\/|\/\/[^\n]*/g, (m) => m.replace(/[^\n]/g, ' '));

/** The import statement from utils/sourceHistory.ts ('' when missing). */
function historyImport(): string {
  const m = code.match(/import\s*\{([^}]*)\}\s*from\s*'\.\.\/utils\/sourceHistory\.ts'/);
  return m ? m[1] : '';
}

/** The local name an export of sourceHistory.ts is bound to (handles `x as y`). */
function local(exported: string): string {
  const imp = historyImport();
  const m = imp.match(new RegExp(`\\b${exported}\\b(?:\\s+as\\s+(\\w+))?`));
  expect(m, `${exported} is not imported from ../utils/sourceHistory.ts`).not.toBeNull();
  return m![1] ?? exported;
}

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

/** Start offset of a named function in the script: `function name(` or `const name = (…) =>`. */
function fnStart(name: string): number {
  const decl = code.search(new RegExp(`(?:async\\s+)?function\\s+${name}\\s*\\(`));
  if (decl > -1) return decl;
  return code.search(new RegExp(`const\\s+${name}\\s*=\\s*(?:async\\s*)?\\([^)]*\\)[^=]*=>`));
}

/** [start, end) of a named function in `code`, header included, body brace-matched. */
function fnRange(name: string): [number, number] {
  const start = fnStart(name);
  expect(start, `${name} not found`).toBeGreaterThan(-1);
  const arrow = code.indexOf('=>', start);
  const paren = code.indexOf(')', start);
  const from = code.startsWith('const', start) && arrow > -1 ? arrow : paren;
  return [start, from + braceBlock(code, from).length];
}

/** The text of a named function (so the slice never runs into the next statement). */
function fnBody(name: string): string {
  const [s, e] = fnRange(name);
  return code.slice(s, e);
}

/** Every opening tag `<name …>` in the markup; a `>` inside `{…}` (an `=>`) does not end the tag. */
function openTags(name: string): string[] {
  const tags: string[] = [];
  let at = markup.indexOf(`<${name}`);
  while (at > -1) {
    let depth = 0;
    let end = at;
    for (; end < markup.length; end++) {
      const c = markup[end];
      if (c === '{') depth++;
      else if (c === '}') depth--;
      else if (c === '>' && depth === 0) break;
    }
    tags.push(markup.slice(at, end + 1));
    at = markup.indexOf(`<${name}`, end);
  }
  return tags;
}

/** The source textarea's opening tag (the one bound to sourceEl). */
function sourceTextarea(): string {
  return openTags('textarea').find((t) => t.includes('bind:this={sourceEl}')) ?? '';
}

/** The result textarea's opening tag (the one showing activeDisplay; #144 dropped resultEl). */
function resultTextarea(): string {
  return openTags('textarea').find((t) => t.includes('value={activeDisplay}')) ?? '';
}

/** The value of an `attr={…}` attribute in a tag (brace-matched), '' when absent. */
function attr(tag: string, name: string): string {
  const i = tag.search(new RegExp(`\\s${name}=\\{`));
  if (i < 0) return '';
  const block = braceBlock(tag, tag.indexOf('{', i));
  return block.slice(1, -1).trim();
}

/** The code an event attribute runs: the inline arrow itself, or the body of the named handler. */
function handlerCode(value: string): string {
  if (/^\w+$/.test(value)) return fnBody(value);
  const call = value.match(/^\(?[^)]*\)?\s*=>\s*(\w+)\(/);
  return call && fnStart(call[1]) > -1 ? value + '\n' + fnBody(call[1]) : value;
}

/** The first `<button …>…</button>` whose text contains `needle`, with its offset in src. */
function button(needle: string): { html: string; at: number } {
  for (const m of src.matchAll(/<button[\s\S]*?<\/button>/g))
    if (m[0].includes(needle)) return { html: m[0], at: m.index! };
  return { html: '', at: -1 };
}

describe('the history module is used, and the history is plain component state', () => {
  it('imports the history functions from ../utils/sourceHistory.ts', () => {
    expect(historyImport(), 'no import from ../utils/sourceHistory.ts').not.toBe('');
    for (const f of ['emptyHistory', 'record', 'breakTyping', 'undo', 'redo', 'canUndo', 'canRedo'])
      local(f);
  });

  it('canUndo and canRedo are imported unaliased (the button contract names them)', () => {
    expect(local('canUndo')).toBe('canUndo');
    expect(local('canRedo')).toBe('canRedo');
  });

  it('the history is a $state that starts empty, not seeded from the restored session', () => {
    const empty = local('emptyHistory');
    expect(code).toMatch(
      new RegExp(`let\\s+\\w+\\s*=\\s*\\$state(?:<[^>]*>)?\\(\\s*${empty}\\(\\)\\s*\\)`),
    );
  });

  it('the history state is not named `history` (it would shadow window.history)', () => {
    expect(code).not.toMatch(/\blet\s+history\b/);
  });

  it('the stored session gets no history field (sessionStore.set mentions none)', () => {
    const empty = local('emptyHistory');
    const decl = code.match(
      new RegExp(`let\\s+(\\w+)\\s*=\\s*\\$state(?:<[^>]*>)?\\(\\s*${empty}\\(\\)`),
    );
    expect(decl, 'history state not found').not.toBeNull();
    const at = code.indexOf('sessionStore.set(');
    expect(at).toBeGreaterThan(-1);
    const set = braceBlock(code, at);
    expect(set).not.toMatch(new RegExp(`\\b${decl![1]}\\b`));
    expect(set).not.toMatch(/history|undo|redo/i);
    const session = readFileSync(resolve(__dirname, '../utils/translateSession.ts'), 'utf8');
    expect(session).not.toMatch(/history|undo|redo/i);
  });
});

describe('one writer: setSource (criterion 5)', () => {
  it('setSource records the change, then assigns input', () => {
    const b = fnBody('setSource');
    const rec = local('record');
    const recAt = b.search(new RegExp(`\\b${rec}\\(`));
    const assignAt = b.search(/\binput\s*=(?!=)/);
    expect(recAt, 'setSource does not call record').toBeGreaterThan(-1);
    expect(assignAt, 'setSource does not assign input').toBeGreaterThan(-1);
    expect(recAt).toBeLessThan(assignAt);
  });

  it('setSource passes the current input as the text before the change', () => {
    const rec = local('record');
    expect(fnBody('setSource')).toMatch(new RegExp(`\\b${rec}\\(\\s*\\w+\\s*,\\s*input\\s*,`));
  });

  it('clearInput goes through setSource and writes input nowhere else', () => {
    const b = fnBody('clearInput');
    expect(b).toMatch(/setSource\(\s*''\s*,\s*'program'\s*\)/);
    expect(b).not.toMatch(/\binput\s*=(?!=)/);
  });

  it('swap goes through setSource before translating', () => {
    const b = fnBody('swap');
    const at = b.search(/setSource\(\s*activeDisplay\s*,\s*'program'\s*\)/);
    expect(at).toBeGreaterThan(-1);
    expect(at).toBeLessThan(b.indexOf('doTranslate()'));
    expect(b).not.toMatch(/\binput\s*=(?!=)/);
  });

  it('the EventInputFill handler goes through setSource before translating', () => {
    const i = code.indexOf('onEvent(EventInputFill');
    expect(i).toBeGreaterThan(-1);
    const body = code.slice(i, code.indexOf('\n    });', i));
    const at = body.search(/setSource\(\s*text\s*,\s*'program'\s*\)/);
    expect(at).toBeGreaterThan(-1);
    expect(at).toBeLessThan(body.indexOf('doTranslate()'));
    expect(body).not.toMatch(/\binput\s*=(?!=)/);
  });

  it('the only assignments to input are in setSource, applyUndo and applyRedo', () => {
    const ranges = ['setSource', 'applyUndo', 'applyRedo'].map(fnRange);
    const stray: string[] = [];
    for (const m of code.matchAll(/(?<![.\w])input\s*=(?!=)/g)) {
      const at = m.index!;
      const line = code.slice(code.lastIndexOf('\n', at) + 1, code.indexOf('\n', at));
      if (/let\s+input\s*=\s*\$state\(restored\.input\)/.test(line)) continue;
      if (!ranges.some(([s, e]) => at >= s && at < e)) stray.push(line.trim());
    }
    expect(stray).toEqual([]);
  });

  it('applyUndo and applyRedo each assign input', () => {
    expect(fnBody('applyUndo')).toMatch(/\binput\s*=(?!=)/);
    expect(fnBody('applyRedo')).toMatch(/\binput\s*=(?!=)/);
  });
});

describe('the source textarea (criterion 5, Decision B)', () => {
  it('is controlled: value={input} + oninput, no bind:value={input} anywhere', () => {
    const ta = sourceTextarea();
    expect(ta, 'source textarea not found').not.toBe('');
    expect(src).not.toMatch(/bind:value=\{\s*input\s*\}/);
    expect(ta).toMatch(/\svalue=\{\s*input\s*\}/);
    expect(attr(ta, 'oninput')).not.toBe('');
  });

  it('oninput goes through setSource with the kind from changeKindOf', () => {
    const run = handlerCode(attr(sourceTextarea(), 'oninput'));
    expect(run).toMatch(/setSource\(/);
    expect(run).toMatch(/changeKindOf\(/);
  });

  it('changeKindOf maps paste, drop, cut and drag to paste; composition to compose; else typing', () => {
    const b = fnBody('changeKindOf');
    for (const t of [
      'insertFromPaste',
      'insertFromDrop',
      'deleteByCut',
      'deleteByDrag',
      'insertCompositionText',
    ]) {
      expect(b, `changeKindOf does not name ${t}`).toContain(`'${t}'`);
    }
    expect(b).toMatch(/isComposing/);
    for (const k of ['paste', 'compose', 'typing']) expect(b).toContain(`'${k}'`);
    expect(b).not.toContain("'program'");
  });

  // #144 (Decision E4): there is no edit mode left to leave; blur only ends the typing run.
  it('onblur ends the typing run (breakTyping)', () => {
    const run = handlerCode(attr(sourceTextarea(), 'onblur'));
    expect(run).toMatch(new RegExp(`\\b${local('breakTyping')}\\(`));
    expect(run).not.toMatch(/editingSource/);
  });

  it('onbeforeinput cancels native historyUndo / historyRedo and applies Kai undo / redo', () => {
    const value = attr(sourceTextarea(), 'onbeforeinput');
    expect(value, 'source textarea has no onbeforeinput').not.toBe('');
    const run = handlerCode(value);
    expect(run).toContain("'historyUndo'");
    expect(run).toContain("'historyRedo'");
    expect(run).toMatch(/\.preventDefault\(\)/);
    expect(run).toMatch(/applyUndo\(/);
    expect(run).toMatch(/applyRedo\(/);
  });

  it('a 500 ms dedupe window guards the beforeinput path against a keydown-applied undo', () => {
    expect(code).toMatch(/\b500\b/);
  });

  // Phase 7 (T-green): the dedupe was pinned only by the `500` literal, so removing the check or
  // never stamping the keydown time survived (F's mutants M1 / M16). Pinned here as a data flow:
  // the keydown handler stamps a time before applying, and the beforeinput handler compares
  // Date.now() minus that stamp against 500 (literal or a const holding it) before it applies.
  it('the keydown handler stamps the time it applied a shortcut, and beforeinput dedupes against it', () => {
    const keydown = handlerCode(attr(openTags('svelte:window')[0] ?? '', 'onkeydown'));
    const stamp = keydown.match(/(\w+)\s*=\s*Date\.now\(\)/);
    expect(stamp, 'the keydown handler stamps no time').not.toBeNull();
    expect(keydown.indexOf(stamp![0])).toBeLessThan(keydown.search(/applyUndo\(/));

    const before = handlerCode(attr(sourceTextarea(), 'onbeforeinput'));
    const window500 = [...code.matchAll(/const\s+(\w+)\s*=\s*500\b/g)].map((m) => m[1]);
    const limit = `(?:500|${[...window500, '__none__'].join('|')})`;
    expect(before, 'beforeinput never reads the keydown stamp').toMatch(
      new RegExp(`Date\\.now\\(\\)\\s*-\\s*${stamp![1]}\\b`),
    );
    const guard = before.search(new RegExp(`<\\s*${limit}\\b[^\\n]*\\breturn\\b`));
    expect(guard, 'no `< 500 … return` guard in the beforeinput handler').toBeGreaterThan(-1);
    expect(guard).toBeLessThan(before.search(/applyUndo\(/));
    expect(before.search(/\.preventDefault\(\)/)).toBeLessThan(guard);
  });

  // Phase 7: F's Deviation 2. A native historyUndo / historyRedo input that still gets through is
  // put back and never recorded ("A historyUndo never reaches record", Decision B). Mutant M4.
  it('oninput puts a native historyUndo / historyRedo back and returns before setSource', () => {
    const run = handlerCode(attr(sourceTextarea(), 'oninput'));
    const hist = run.search(/'historyUndo'/);
    expect(hist, 'oninput does not name historyUndo').toBeGreaterThan(-1);
    expect(run).toContain("'historyRedo'");
    const tail = run.slice(hist);
    const back = tail.search(/\.value\s*=\s*input\b/);
    const ret = tail.search(/\breturn\b/);
    const rec = tail.search(/setSource\(/);
    expect(back, 'the textarea text is not put back').toBeGreaterThan(-1);
    expect(ret).toBeGreaterThan(back);
    expect(rec).toBeGreaterThan(ret);
  });

  // Phase 7: F's Deviation 1. WebKit (Input Events Level 1) reports an IME commit as
  // deleteCompositionText / insertFromComposition after compositionend, with isComposing false; as
  // typing, a candidate chosen 1 s after the last keystroke would leave a half-composed step
  // (criterion 2). Both must sit in the branch that returns 'compose'. Mutant M2.
  it("changeKindOf classes WebKit's IME commit types as compose", () => {
    const b = fnBody('changeKindOf');
    const ret = b.indexOf("return 'compose'");
    expect(ret).toBeGreaterThan(-1);
    const branch = b.slice(b.lastIndexOf('return', ret - 1), ret);
    for (const t of ['insertCompositionText', 'deleteCompositionText', 'insertFromComposition'])
      expect(branch, `${t} is not in the compose branch`).toContain(`'${t}'`);
    expect(branch).toMatch(/isComposing/);
  });

  it('the result-edit textarea keeps its native undo (no history hooks on it)', () => {
    const ta = resultTextarea();
    expect(ta, 'result textarea not found').not.toBe('');
    expect(ta).not.toMatch(/onbeforeinput/);
    expect(ta).not.toMatch(/setSource|applyUndo|applyRedo/);
  });
});

describe('keyboard shortcuts (criterion 7, Decision B)', () => {
  const windowTag = () => openTags('svelte:window')[0] ?? '';

  it('one <svelte:window onkeydown> handler exists', () => {
    expect(markup.match(/<svelte:window\b/g)?.length ?? 0).toBe(1);
    expect(attr(windowTag(), 'onkeydown')).not.toBe('');
  });

  it('the handler skips IME composition, prevents the default and applies undo / redo via shortcutAction', () => {
    const run = handlerCode(attr(windowTag(), 'onkeydown'));
    expect(run).toMatch(/isComposing/);
    expect(run).toMatch(/shortcutAction\(/);
    expect(run).toMatch(/\.preventDefault\(\)/);
    expect(run).toMatch(/applyUndo\(/);
    expect(run).toMatch(/applyRedo\(/);
  });

  it('the handler is scoped: it compares the target with the source textarea and skips selects and other editables', () => {
    const run = handlerCode(attr(windowTag(), 'onkeydown')) + '\n' + fnBody('shortcutAction');
    expect(run).toMatch(/\bsourceEl\b/);
    expect(run).toMatch(/select/i);
    expect(run).toMatch(/isContentEditable|textarea|INPUT/i);
  });

  it('shortcutAction matches Cmd/Ctrl+Z, Shift+Cmd/Ctrl+Z and Ctrl+Y on the lower-cased key, never with Alt', () => {
    const b = fnBody('shortcutAction');
    expect(b).toMatch(/\.key\.toLowerCase\(\)/);
    expect(b).toContain("'z'");
    expect(b).toContain("'y'");
    for (const k of ['metaKey', 'ctrlKey', 'shiftKey', 'altKey'])
      expect(b, `shortcutAction ignores ${k}`).toContain(k);
    expect(b).toContain("'undo'");
    expect(b).toContain("'redo'");
  });

  it('no per-element keydown handler duplicates the shortcut', () => {
    expect(sourceTextarea()).not.toMatch(/onkeydown/);
  });
});

describe('undo never translates and never cancels (criterion 6, Decision F)', () => {
  for (const name of ['applyUndo', 'applyRedo']) {
    it(`${name} only restores the text`, () => {
      const b = fnBody(name);
      for (const bad of [
        /doTranslate/,
        /TranslateMulti/,
        /cancelRequest/,
        /CancelTranslate/,
        /persistLangs/,
      ]) {
        expect(b, `${name} must not call ${bad.source}`).not.toMatch(bad);
      }
      for (const bad of [/\bresults\s*=(?!=)/, /\bfromLang\s*=(?!=)/, /\btoLang\s*=(?!=)/]) {
        expect(b, `${name} must not assign ${bad.source}`).not.toMatch(bad);
      }
    });
  }

  it('applyUndo calls the history undo and applyRedo calls the history redo', () => {
    expect(fnBody('applyUndo')).toMatch(new RegExp(`\\b${local('undo')}\\(`));
    expect(fnBody('applyRedo')).toMatch(new RegExp(`\\b${local('redo')}\\(`));
  });
});

describe('Undo and Redo buttons (criterion 8, Decision E)', () => {
  for (const [kind, can, apply] of [
    ['undo', 'canUndo', 'applyUndo'],
    ['redo', 'canRedo', 'applyRedo'],
  ] as const) {
    it(`the ${kind} button is an icon button labelled translate.${kind} and disabled by !${can}(…)`, () => {
      const { html } = button(`t('translate.${kind}')`);
      expect(html, `${kind} button not found`).not.toBe('');
      expect(html).toMatch(/class="[^"]*\bu-icon-btn\b[^"]*\bu-no-drag\b/);
      expect(html).toContain(`aria-label={t('translate.${kind}')}`);
      expect(html).toContain(`title={t('translate.${kind}')}`);
      expect(html).toMatch(new RegExp(`disabled=\\{\\s*!${can}\\(`));
      expect(html).toMatch(new RegExp(`onclick=\\{[^}]*${apply}`));
      expect(html).toContain('<svg');
    });
  }

  it('sits in the source footer in the order Clear, Undo, Redo, Copy', () => {
    const clear = button('onclick={clearInput}').at;
    const u = button(`t('translate.undo')`).at;
    const r = button(`t('translate.redo')`).at;
    const copy = button('copy(input)').at;
    expect(clear).toBeGreaterThan(-1);
    expect(copy).toBeGreaterThan(-1);
    expect(u).toBeGreaterThan(clear);
    expect(r).toBeGreaterThan(u);
    expect(copy).toBeGreaterThan(r);
  });

  it('the catalogs carry non-empty, distinct undo/redo tooltips per language (issue #173: expanded to be more descriptive)', () => {
    const enT = en.translate as unknown as Record<string, string>;
    const zhT = zh.translate as unknown as Record<string, string>;
    expect(enT.undo).toBeTruthy();
    expect(enT.redo).toBeTruthy();
    expect(enT.undo).not.toBe(enT.redo);
    expect(zhT.undo).toBeTruthy();
    expect(zhT.redo).toBeTruthy();
    expect(zhT.undo).not.toBe(zhT.redo);
    expect(zhT.undo).not.toBe(enT.undo);
    expect(zhT.redo).not.toBe(enT.redo);
  });
});
