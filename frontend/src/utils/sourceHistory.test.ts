// issue #118 (Tester, RED): the pure undo/redo history of the translate window's source pane, in
// the new module utils/sourceHistory.ts (brief Decision A; imported here by its export names).
// No Svelte, no DOM, no timers: the clock is the `now` argument everywhere, so no test sleeps.
//
// Contract (brief Decision A, the field names are the contract):
//   SourceHistory = { undo: string[] (oldest first); redo: string[] (most recent last);
//                     chars: number (total length of undo + redo); typingAt: number | null }
//   record(h, prev, next, kind, now, limits?) / breakTyping(h) / undo(h, current) / redo(h, current)
//   canUndo(h) / canRedo(h) / emptyHistory() / DEFAULT_LIMITS
import { describe, expect, it } from 'vitest';
import {
  DEFAULT_LIMITS,
  breakTyping,
  canRedo,
  canUndo,
  emptyHistory,
  record,
  redo,
  undo,
  type ChangeKind,
  type HistoryLimits,
  type SourceHistory,
} from './sourceHistory.ts';

const T0 = 1_000_000; // an arbitrary non-zero clock origin

/** Sum of the lengths of every undo and redo entry: what `chars` must always equal. */
const heldChars = (h: SourceHistory) => [...h.undo, ...h.redo].reduce((n, s) => n + s.length, 0);

/** Records a chain of texts, one step per text (program changes never coalesce). */
function programChain(texts: string[], limits?: HistoryLimits): SourceHistory {
  let h = emptyHistory();
  for (let i = 1; i < texts.length; i++)
    h = record(h, texts[i - 1], texts[i], 'program', T0 + i, limits);
  return h;
}

/** Deep-freezes a history so any in-place mutation throws (strict-mode ESM). */
function frozen(h: SourceHistory): SourceHistory {
  Object.freeze(h.undo);
  Object.freeze(h.redo);
  return Object.freeze(h);
}

describe('defaults and the empty history', () => {
  it('DEFAULT_LIMITS are 100 steps, 2,000,000 characters and a 1000 ms typing window', () => {
    expect(DEFAULT_LIMITS).toEqual({ maxSteps: 100, maxChars: 2_000_000, coalesceMs: 1000 });
  });

  it('an empty history holds nothing and can neither undo nor redo', () => {
    const h = emptyHistory();
    expect(h.undo).toEqual([]);
    expect(h.redo).toEqual([]);
    expect(h.chars).toBe(0);
    expect(h.typingAt).toBeNull();
    expect(canUndo(h)).toBe(false);
    expect(canRedo(h)).toBe(false);
  });

  it('undo and redo on an empty history return null', () => {
    expect(undo(emptyHistory(), 'text')).toBeNull();
    expect(redo(emptyHistory(), 'text')).toBeNull();
  });

  it('each call to emptyHistory returns an independent value', () => {
    const a = emptyHistory();
    const b = record(a, '', 'x', 'program', T0);
    expect(canUndo(b)).toBe(true);
    expect(canUndo(emptyHistory())).toBe(false);
  });
});

describe('typing coalesces into one step (criterion 2)', () => {
  it('the first typing record pushes the text before it and opens a run', () => {
    const h = record(emptyHistory(), '', 'a', 'typing', T0);
    expect(h.undo).toEqual(['']);
    expect(h.typingAt).toBe(T0);
    expect(canUndo(h)).toBe(true);
  });

  it('typing records with gaps under 1000 ms are one step', () => {
    let h = record(emptyHistory(), 'Hi', 'Hi ', 'typing', T0);
    h = record(h, 'Hi ', 'Hi t', 'typing', T0 + 200);
    h = record(h, 'Hi t', 'Hi th', 'typing', T0 + 999 + 200);
    expect(h.undo).toEqual(['Hi']);
  });

  it('the window slides: a sentence typed without a 1 s pause is one step however long it takes', () => {
    const text = 'Typing a whole sentence';
    let h = emptyHistory();
    for (let i = 0; i < text.length; i++)
      h = record(h, text.slice(0, i), text.slice(0, i + 1), 'typing', T0 + i * 900);
    expect(h.undo).toEqual(['']);
    expect(h.typingAt).toBe(T0 + (text.length - 1) * 900);
  });

  it('a gap of exactly 1000 ms starts a new step', () => {
    let h = record(emptyHistory(), '', 'a', 'typing', T0);
    h = record(h, 'a', 'ab', 'typing', T0 + 1000);
    expect(h.undo).toEqual(['', 'a']);
  });

  it('a gap of 999 ms does not start a new step', () => {
    let h = record(emptyHistory(), '', 'a', 'typing', T0);
    h = record(h, 'a', 'ab', 'typing', T0 + 999);
    expect(h.undo).toEqual(['']);
  });

  it('typing, a pause, then more typing is two steps; undo removes the second run whole', () => {
    let h = record(emptyHistory(), '', 'O', 'typing', T0);
    h = record(h, 'O', 'On', 'typing', T0 + 100);
    h = record(h, 'On', 'One', 'typing', T0 + 200);
    h = record(h, 'One', 'One ', 'typing', T0 + 2000);
    h = record(h, 'One ', 'One t', 'typing', T0 + 2100);
    h = record(h, 'One t', 'One two', 'typing', T0 + 2200);
    expect(h.undo).toEqual(['', 'One']);
    const u = undo(h, 'One two');
    expect(u?.text).toBe('One');
  });

  it('the coalescing window comes from the limits argument', () => {
    const limits: HistoryLimits = { ...DEFAULT_LIMITS, coalesceMs: 50 };
    let h = record(emptyHistory(), '', 'a', 'typing', T0, limits);
    h = record(h, 'a', 'ab', 'typing', T0 + 49, limits);
    h = record(h, 'ab', 'abc', 'typing', T0 + 49 + 50, limits);
    expect(h.undo).toEqual(['', 'ab']);
  });

  it('breakTyping (blur) ends the run: the next keystroke is a new step even within 1000 ms', () => {
    let h = record(emptyHistory(), '', 'a', 'typing', T0);
    h = breakTyping(h);
    expect(h.typingAt).toBeNull();
    h = record(h, 'a', 'ab', 'typing', T0 + 10);
    expect(h.undo).toEqual(['', 'a']);
  });

  it('breakTyping leaves both stacks alone', () => {
    const h = record(record(emptyHistory(), '', 'a', 'program', T0), 'a', 'ab', 'typing', T0 + 1);
    const b = breakTyping(h);
    expect(b.undo).toEqual(h.undo);
    expect(b.redo).toEqual(h.redo);
    expect(b.chars).toBe(h.chars);
  });
});

describe('IME composition never splits on a pause (criterion 2)', () => {
  it('compose records inside an open run never push on a time gap', () => {
    let h = record(emptyHistory(), '', 'n', 'compose', T0);
    h = record(h, 'n', 'ni', 'compose', T0 + 5000);
    h = record(h, 'ni', '你', 'compose', T0 + 60_000);
    expect(h.undo).toEqual(['']);
  });

  it('a compose record continuing a typing run does not split on a gap either', () => {
    let h = record(emptyHistory(), '', 'a ', 'typing', T0);
    h = record(h, 'a ', 'a n', 'compose', T0 + 3000);
    expect(h.undo).toEqual(['']);
  });

  it('compose with no open run pushes the text before it and opens a run', () => {
    const h = record(
      breakTyping(record(emptyHistory(), '', 'x', 'typing', T0)),
      'x',
      'xn',
      'compose',
      T0 + 1,
    );
    expect(h.undo).toEqual(['', 'x']);
    expect(h.typingAt).not.toBeNull();
  });
});

describe('paste and program changes are always their own step (criterion 2, Decision C)', () => {
  for (const kind of ['paste', 'program'] as ChangeKind[]) {
    it(`${kind} pushes even inside an open typing run`, () => {
      let h = record(emptyHistory(), '', 'a', 'typing', T0);
      h = record(h, 'a', 'a pasted', kind, T0 + 10);
      expect(h.undo).toEqual(['', 'a']);
    });

    it(`${kind} closes the run: typing right after it is a new step`, () => {
      let h = record(emptyHistory(), '', 'a', 'typing', T0);
      h = record(h, 'a', 'a pasted', kind, T0 + 10);
      expect(h.typingAt).toBeNull();
      h = record(h, 'a pasted', 'a pasted!', 'typing', T0 + 20);
      expect(h.undo).toEqual(['', 'a', 'a pasted']);
    });

    it(`two ${kind} records in a row are two steps`, () => {
      let h = record(emptyHistory(), '', 'one', kind, T0);
      h = record(h, 'one', 'two', kind, T0 + 1);
      expect(h.undo).toEqual(['', 'one']);
    });
  }
});

describe('an unchanged text records nothing (criterion 2, Decision C)', () => {
  for (const kind of ['typing', 'compose', 'paste', 'program'] as ChangeKind[]) {
    it(`${kind} with prev === next is a no-op and keeps redo`, () => {
      const base = programChain(['a', 'b', 'c']);
      const u = undo(base, 'c')!;
      expect(canRedo(u.history)).toBe(true);
      const after = record(u.history, 'b', 'b', kind, T0 + 50);
      expect(after.undo).toEqual(u.history.undo);
      expect(after.redo).toEqual(u.history.redo);
      expect(after.chars).toBe(u.history.chars);
      expect(canRedo(after)).toBe(true);
    });
  }

  it('Clear on an empty pane records nothing', () => {
    const h = record(emptyHistory(), '', '', 'program', T0);
    expect(canUndo(h)).toBe(false);
  });
});

describe('undo / redo order (criterion 3)', () => {
  it('walks three steps back and forward in order', () => {
    const h = programChain(['', 'one', 'two', 'three']);
    const u1 = undo(h, 'three')!;
    expect(u1.text).toBe('two');
    const u2 = undo(u1.history, u1.text)!;
    expect(u2.text).toBe('one');
    const u3 = undo(u2.history, u2.text)!;
    expect(u3.text).toBe('');
    expect(undo(u3.history, u3.text)).toBeNull();
    expect(canUndo(u3.history)).toBe(false);
    expect(canRedo(u3.history)).toBe(true);

    const r1 = redo(u3.history, u3.text)!;
    expect(r1.text).toBe('one');
    const r2 = redo(r1.history, r1.text)!;
    expect(r2.text).toBe('two');
    const r3 = redo(r2.history, r2.text)!;
    expect(r3.text).toBe('three');
    expect(redo(r3.history, r3.text)).toBeNull();
    expect(canRedo(r3.history)).toBe(false);
    expect(canUndo(r3.history)).toBe(true);
  });

  it('undo moves the current text onto redo; redo moves it back onto undo', () => {
    const h = programChain(['a', 'b']);
    const u = undo(h, 'b')!;
    expect(u.history.undo).toEqual([]);
    expect(u.history.redo).toEqual(['b']);
    const r = redo(u.history, 'a')!;
    expect(r.history.undo).toEqual(['a']);
    expect(r.history.redo).toEqual([]);
  });

  it('undo stores the current text given by the caller, not a recorded one', () => {
    // The caller passes what is on screen; that is what redo must bring back.
    const h = programChain(['a', 'b']);
    const u = undo(h, 'b edited')!;
    expect(redo(u.history, u.text)!.text).toBe('b edited');
  });

  it('a new record after an undo clears redo', () => {
    const h = programChain(['', 'one', 'two']);
    const u = undo(h, 'two')!;
    expect(canRedo(u.history)).toBe(true);
    const n = record(u.history, 'one', 'one!', 'typing', T0 + 100);
    expect(n.redo).toEqual([]);
    expect(canRedo(n)).toBe(false);
    expect(n.undo).toEqual(['', 'one']);
  });

  it('undo closes the typing run: typing right after an undo is a new step', () => {
    let h = record(emptyHistory(), '', 'a', 'typing', T0);
    h = record(h, 'a', 'ab', 'typing', T0 + 100);
    const u = undo(h, 'ab')!;
    expect(u.text).toBe('');
    expect(u.history.typingAt).toBeNull();
    const n = record(u.history, '', 'x', 'typing', T0 + 200);
    expect(n.undo).toEqual(['']);
    expect(canUndo(n)).toBe(true);
  });

  it('redo closes the typing run too', () => {
    const h = programChain(['a', 'b']);
    const u = undo(h, 'b')!;
    const r = redo(u.history, u.text)!;
    expect(r.history.typingAt).toBeNull();
  });
});

describe('undo after Clear and after swap (Decision G)', () => {
  it('undo after Clear returns the cleared text; redo empties it again', () => {
    let h = record(emptyHistory(), '', 'abc', 'typing', T0);
    h = record(h, 'abc', '', 'program', T0 + 5000);
    const u = undo(h, '')!;
    expect(u.text).toBe('abc');
    const r = redo(u.history, u.text)!;
    expect(r.text).toBe('');
  });

  it('undo after a swap returns the source text from before the swap', () => {
    const h = record(emptyHistory(), 'Good morning', 'Buenos días', 'program', T0);
    expect(undo(h, 'Buenos días')!.text).toBe('Good morning');
  });

  it('undo after a hotkey fill returns the earlier text', () => {
    let h = record(emptyHistory(), '', 'draft', 'typing', T0);
    h = record(h, 'draft', 'filled from the clipboard', 'program', T0 + 10);
    expect(undo(h, 'filled from the clipboard')!.text).toBe('draft');
  });
});

describe('caps (criterion 4)', () => {
  it('101 records leave 100 undo steps, the oldest dropped', () => {
    const texts = Array.from({ length: 102 }, (_, i) => `t${i}`);
    const h = programChain(texts); // 101 records: prevs t0..t100
    expect(h.undo.length).toBe(100);
    expect(h.undo[0]).toBe('t1');
    expect(h.undo[99]).toBe('t100');
  });

  it('the step cap comes from the limits argument', () => {
    const texts = Array.from({ length: 12 }, (_, i) => `t${i}`);
    const h = programChain(texts, { ...DEFAULT_LIMITS, maxSteps: 5 });
    expect(h.undo).toEqual(['t6', 't7', 't8', 't9', 't10']);
  });

  it('25 records of 100,000 characters keep at most 2,000,000 characters (20 steps), oldest dropped', () => {
    const texts = Array.from({ length: 26 }, (_, i) => String.fromCharCode(65 + i).repeat(100_000));
    const h = programChain(texts);
    expect(h.chars).toBeLessThanOrEqual(2_000_000);
    expect(h.chars).toBe(heldChars(h));
    expect(h.undo.length).toBe(20);
    expect(h.undo[h.undo.length - 1]).toBe(texts[24]); // newest kept
    expect(h.undo[0]).toBe(texts[5]); // the five oldest are gone
  });

  it('the newest entry is kept even when it alone exceeds the character cap', () => {
    const limits: HistoryLimits = { ...DEFAULT_LIMITS, maxChars: 10 };
    const huge = 'x'.repeat(50);
    let h = record(emptyHistory(), 'short', huge, 'program', T0, limits);
    h = record(h, huge, '', 'program', T0 + 1, limits); // Clear of a huge document
    expect(h.undo).toEqual([huge]);
    expect(undo(h, '')!.text).toBe(huge);
  });

  it('Clear of a document larger than the default cap is still undoable', () => {
    const doc = 'y'.repeat(2_500_000);
    let h = record(emptyHistory(), '', doc, 'paste', T0);
    h = record(h, doc, '', 'program', T0 + 1);
    expect(canUndo(h)).toBe(true);
    expect(undo(h, '')!.text).toBe(doc);
  });
});

describe('chars bookkeeping', () => {
  it('chars equals the total length of undo + redo entries through records, undo and redo', () => {
    let h = programChain(['', 'ab', 'abcd', 'abcdef']);
    expect(h.chars).toBe(heldChars(h));
    const u1 = undo(h, 'abcdef')!;
    expect(u1.history.chars).toBe(heldChars(u1.history));
    const u2 = undo(u1.history, u1.text)!;
    expect(u2.history.chars).toBe(heldChars(u2.history));
    const r = redo(u2.history, u2.text)!;
    expect(r.history.chars).toBe(heldChars(r.history));
    h = record(r.history, r.text, 'new', 'program', T0 + 99);
    expect(h.chars).toBe(heldChars(h));
    expect(breakTyping(h).chars).toBe(heldChars(h));
  });
});

describe('purity: no function mutates its input', () => {
  it('record, breakTyping, undo and redo leave a frozen input untouched', () => {
    const base = frozen(programChain(['', 'a', 'b', 'c']));
    const snapshot = JSON.stringify(base);
    const u = undo(base, 'c')!;
    frozen(u.history);
    const uSnap = JSON.stringify(u.history);
    record(base, 'c', 'd', 'typing', T0 + 5);
    record(base, 'c', 'd', 'program', T0 + 5);
    breakTyping(record(base, 'c', 'd', 'typing', T0 + 5));
    breakTyping(base);
    redo(u.history, u.text);
    record(u.history, u.text, 'z', 'paste', T0 + 6);
    expect(JSON.stringify(base)).toBe(snapshot);
    expect(JSON.stringify(u.history)).toBe(uSnap);
  });

  it('record returns a new object for a real change', () => {
    const h = emptyHistory();
    const n = record(h, '', 'a', 'typing', T0);
    expect(n).not.toBe(h);
    expect(h.undo).toEqual([]);
  });
});
